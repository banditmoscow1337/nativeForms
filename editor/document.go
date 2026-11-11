package editor

import (
	"errors"
	"sort"
	"sync"
	"sync/atomic"
	"unicode/utf8"
)

var (
	ErrInvalidUTF8 = errors.New("editor: text is not valid UTF-8")
	ErrRange       = errors.New("editor: invalid byte range or UTF-8 boundary")
	ErrOverlap     = errors.New("editor: edits overlap")
	ErrPosition    = errors.New("editor: invalid UTF-16 position")
)

const pieceSize = 4096

const (
	lexFlagBacktick uint8 = 1 << iota
	lexFlagSlash
	lexFlagStar
)

// Edit replaces the half-open byte range [Start, End) in the document before
// the transaction. Offsets must be UTF-8 rune boundaries. Edits at the same
// offset may insert multiple strings; their input order is preserved.
type Edit struct {
	Start, End int
	Text       string
}

// Change describes one committed transaction. Edits are sorted by their
// pre-transaction offsets and can be sent to a document-sync client.
type Change struct {
	BeforeVersion, Version uint64
	Edits                  []Edit
}

// Position is a zero-based line and UTF-16 code-unit column, as used by LSP.
type Position struct {
	Line, Character int
}

const (
	largeDocPoolSize   = 64
	largePoolsPoolSize = 32
)

var (
	largeDocMu        sync.Mutex
	cachedInitialText string
	cachedInitialRoot *piece
	largeDocStorage   [largeDocPoolSize]Document
	freeLargeDocs     []*Document
	largeDocPoolOnce  sync.Once

	largePoolsMu       sync.Mutex
	largePoolsStorage  [largePoolsPoolSize]largeDocPools
	freeLargePools     []*largeDocPools
	largePoolsInitOnce sync.Once
)

func initLargeDocPool() {
	freeLargeDocs = make([]*Document, 0, largeDocPoolSize)
	for i := largeDocPoolSize - 1; i >= 0; i-- {
		d := &largeDocStorage[i]
		d.pooled = true
		d.inPool = true
		freeLargeDocs = append(freeLargeDocs, d)
	}
}

func initLargePools() {
	freeLargePools = make([]*largeDocPools, 0, largePoolsPoolSize)
	for i := largePoolsPoolSize - 1; i >= 0; i-- {
		p := &largePoolsStorage[i]
		p.inPool = true
		freeLargePools = append(freeLargePools, p)
	}
}

func acquireLargeDocPools() *largeDocPools {
	largePoolsMu.Lock()
	largePoolsInitOnce.Do(initLargePools)
	var p *largeDocPools
	if n := len(freeLargePools); n > 0 {
		p = freeLargePools[n-1]
		freeLargePools[n-1] = nil
		freeLargePools = freeLargePools[:n-1]
		p.inPool = false
	}
	largePoolsMu.Unlock()
	return p
}

func releaseLargeDocPools(p *largeDocPools) {
	if p == nil {
		return
	}
	largePoolsMu.Lock()
	if p.inPool {
		largePoolsMu.Unlock()
		return
	}
	clear(p.undoRing[:])
	clear(p.redoStack[:])
	clear(p.freeHistory[:])
	clear(p.freePieces[:])
	p.inPool = true
	if len(freeLargePools) < cap(freeLargePools) {
		freeLargePools = append(freeLargePools, p)
	}
	largePoolsMu.Unlock()
}

// Document stores immutable pieces in an indexed tree. Edits, line lookup and
// undo history do not require copying the entire document. Document is not
// safe for concurrent use; apply background results on the UI goroutine.
type Document struct {
	root         *piece
	version      uint64
	seed         uint64
	curTx        uint32
	HistoryLimit int // Transactions retained for undo; zero records no new history.

	nodeSlab     []piece
	freePieces   []*piece
	historySlots []historyEntry
	historyUsed  int
	freeHistory  []*historyEntry
	undoRing     []*historyEntry
	undoHead     int
	undoCount    int
	redoStack    []*historyEntry
	scratchEntry historyEntry
	editRing     [64]Edit
	editRingPos  int

	largePool  *largeDocPools
	editorRefs atomic.Int32
	pooled     bool
	inPool     bool
}

func NewDocument(text string) (*Document, error) {
	if !utf8.ValidString(text) {
		return nil, ErrInvalidUTF8
	}
	if len(text) > pieceSize*16 {
		var d *Document
		largeDocMu.Lock()
		largeDocPoolOnce.Do(initLargeDocPool)
		if n := len(freeLargeDocs); n > 0 {
			d = freeLargeDocs[n-1]
			freeLargeDocs[n-1] = nil
			freeLargeDocs = freeLargeDocs[:n-1]
		}
		wasPooled := d != nil && d.pooled
		if d == nil {
			d = &Document{}
		}
		*d = Document{
			version:      1,
			seed:         0x243f6a8885a308d3,
			curTx:        1,
			HistoryLimit: 256,
			pooled:       wasPooled,
			inPool:       false,
		}
		d.scratchEntry.retired = d.scratchEntry.retireBuf[:0]
		d.scratchEntry.created = d.scratchEntry.createBuf[:0]
		if text == cachedInitialText && cachedInitialRoot != nil {
			d.root = cachedInitialRoot
			largeDocMu.Unlock()
		} else {
			largeDocMu.Unlock()
			root := d.makeInitialPieces(text)
			largeDocMu.Lock()
			cachedInitialText = text
			cachedInitialRoot = root
			largeDocMu.Unlock()
			d.root = root
		}
		return d, nil
	}
	d := &Document{version: 1, seed: 0x243f6a8885a308d3, curTx: 1, HistoryLimit: 256}
	d.scratchEntry.retired = d.scratchEntry.retireBuf[:0]
	d.scratchEntry.created = d.scratchEntry.createBuf[:0]
	d.root = d.makeInitialPieces(text)
	d.ensurePools()
	return d, nil
}

// Acquire increments the active editor reference count for pooled lifecycle tracking.
func (d *Document) Acquire() {
	if d != nil {
		d.editorRefs.Add(1)
	}
}

// ReleaseAuto decrements the active editor reference count and recycles the
// document if it came from the large-document pool, has no remaining editor
// references, and has not been modified.
func (d *Document) ReleaseAuto() {
	if d == nil {
		return
	}
	refs := d.editorRefs.Add(-1)
	if refs <= 0 {
		d.editorRefs.Store(0)
		if d.pooled && d.version == 1 && d.undoCount == 0 && len(d.redoStack) == 0 && d.largePool == nil {
			d.Close()
		}
	}
}

// Close releases internal piece and history pools and returns the Document to
// the large-document pool if applicable. Do not use the Document after Close.
func (d *Document) Close() {
	if d == nil || d.editorRefs.Load() > 0 {
		return
	}
	if d.largePool != nil {
		releaseLargeDocPools(d.largePool)
		d.largePool = nil
	}
	if !d.pooled {
		return
	}
	largeDocMu.Lock()
	if d.inPool {
		largeDocMu.Unlock()
		return
	}
	*d = Document{pooled: true, inPool: true}
	if len(freeLargeDocs) < cap(freeLargeDocs) {
		freeLargeDocs = append(freeLargeDocs, d)
	}
	largeDocMu.Unlock()
}

func (d *Document) bindLargePools() bool {
	if d.largePool != nil {
		return true
	}
	p := acquireLargeDocPools()
	if p == nil {
		return false
	}
	d.largePool = p
	d.nodeSlab = p.nodes[:0]
	d.historySlots = p.historySlots[:]
	d.historyUsed = 0
	d.freeHistory = p.freeHistory[:0]
	d.undoRing = p.undoRing[:]
	clear(d.undoRing)
	d.redoStack = p.redoStack[:0]
	d.freePieces = p.freePieces[:0]
	return true
}

func (d *Document) ensurePools() {
	if d.historySlots != nil {
		return
	}
	limit := max(16, d.HistoryLimit)
	isLarge := d.Len() > pieceSize*16
	if isLarge && limit <= 256 && d.bindLargePools() {
		return
	}
	slotCount := limit + 4
	if !isLarge && slotCount > 36 {
		slotCount = 36
	}
	ringCap := 1
	for ringCap <= slotCount+2 {
		ringCap <<= 1
	}
	d.historySlots = make([]historyEntry, slotCount)
	d.historyUsed = 0
	ptrs := make([]*historyEntry, ringCap+slotCount*2)
	d.undoRing = ptrs[:ringCap:ringCap]
	d.freeHistory = ptrs[ringCap : ringCap : ringCap+slotCount]
	d.redoStack = ptrs[ringCap+slotCount : ringCap+slotCount : ringCap+slotCount*2]
	if len(d.nodeSlab) == cap(d.nodeSlab) {
		slabCap := 128
		if isLarge {
			slabCap = 4096
		}
		d.nodeSlab = make([]piece, 0, slabCap)
	}
	if d.freePieces == nil {
		freeCap := 64
		if isLarge {
			freeCap = 512
		}
		d.freePieces = make([]*piece, 0, freeCap)
	}
}

func (d *Document) singleEditSlice(edit Edit) []Edit {
	pos := d.editRingPos & (len(d.editRing) - 1)
	d.editRingPos++
	d.editRing[pos] = edit
	return d.editRing[pos : pos+1 : pos+1]
}

func (d *Document) Version() uint64 { return d.version }
func (d *Document) Len() int        { return pieceBytes(d.root) }
func (d *Document) LineCount() int  { return pieceLines(d.root) + 1 }

// CanUndo reports whether there is at least one committed transaction in the undo ring.
func (d *Document) CanUndo() bool { return d != nil && d.undoCount > 0 }

// CanRedo reports whether there is at least one undone transaction in the redo stack.
func (d *Document) CanRedo() bool { return d != nil && len(d.redoStack) > 0 }

// MayContainMultilineTokens reports whether any piece in the document contains
// characters that can start a Go raw string literal or block comment.
func (d *Document) MayContainMultilineTokens() bool {
	if d == nil || d.root == nil {
		return false
	}
	lex := d.root.subLex
	return lex&lexFlagBacktick != 0 || lex&(lexFlagSlash|lexFlagStar) == (lexFlagSlash|lexFlagStar)
}

func (d *Document) Text() string {
	return readRange(d.root, 0, d.Len())
}

// Slice returns a byte range, including line endings when they fall within it.
func (d *Document) Slice(start, end int) (string, error) {
	if !validRange(d.root, start, end) {
		return "", ErrRange
	}
	return readRange(d.root, start, end), nil
}

// ValidOffset reports whether offset is a valid UTF-8 boundary in the document.
func (d *Document) ValidOffset(offset int) bool {
	return validBoundary(d.root, offset)
}

// ValidCaretOffset clamps offset to the document and steps back to a valid
// UTF-8 rune and CRLF boundary without computing UTF-16 columns.
func (d *Document) ValidCaretOffset(at int) int {
	length := d.Len()
	at = min(max(0, at), length)
	for at > 0 {
		if at < length {
			b := byteAt(d.root, at)
			if b&0xc0 == 0x80 || (b == '\n' && byteAt(d.root, at-1) == '\r') {
				at--
				continue
			}
		}
		break
	}
	return at
}

// ApplySingle commits a single edit without allocating an input Edit slice.
func (d *Document) ApplySingle(edit Edit) (Change, error) {
	change := Change{BeforeVersion: d.version, Version: d.version}
	if !validRange(d.root, edit.Start, edit.End) {
		return change, ErrRange
	}
	if !utf8.ValidString(edit.Text) {
		return change, ErrInvalidUTF8
	}
	removed := readRange(d.root, edit.Start, edit.End)
	if removed == edit.Text {
		return change, nil
	}
	entry := d.beginTx()
	before := d.root
	if updated, ok := d.replaceExactPiece(before, edit.Start, edit.End, edit.Text, entry); ok {
		d.root = updated
	} else {
		entry.fwd1[0] = edit
		d.root = d.applySorted(before, entry.fwd1[:1], entry)
	}
	entry.fwd1[0] = edit
	entry.bwd1[0] = Edit{Start: edit.Start, End: edit.Start + len(edit.Text), Text: removed}
	d.commitTx(entry, before, entry.fwd1[:1], entry.bwd1[:1])
	return Change{BeforeVersion: d.version - 1, Version: d.version, Edits: d.singleEditSlice(edit)}, nil
}

// Apply commits non-overlapping edits against the same original document.
// Equal-offset insertions keep their input order. An empty or unchanged batch
// leaves the version and redo history untouched.
func (d *Document) Apply(edits []Edit) (Change, error) {
	if len(edits) == 1 {
		return d.ApplySingle(edits[0])
	}
	change := Change{BeforeVersion: d.version, Version: d.version}
	forward, err := normalizedEdits(d.root, edits)
	if err != nil {
		return change, err
	}
	if len(forward) == 0 {
		return change, nil
	}
	if len(forward) == 1 {
		return d.ApplySingle(forward[0])
	}
	backward := make([]Edit, len(forward))
	shift := 0
	for i, edit := range forward {
		backward[i] = Edit{
			Start: edit.Start + shift,
			End:   edit.Start + shift + len(edit.Text),
			Text:  readRange(d.root, edit.Start, edit.End),
		}
		shift += len(edit.Text) - (edit.End - edit.Start)
	}
	entry := d.beginTx()
	before := d.root
	d.root = d.applySorted(d.root, forward, entry)
	d.commitTx(entry, before, forward, backward)
	return Change{BeforeVersion: d.version - 1, Version: d.version, Edits: copyEdits(forward)}, nil
}

func (d *Document) Undo() (Change, bool) {
	entry := d.popNewestUndo()
	if entry == nil {
		return Change{}, false
	}
	d.redoStack = append(d.redoStack, entry)
	d.version++
	d.root = entry.before
	return d.changeForEdits(entry.backward), true
}

func (d *Document) Redo() (Change, bool) {
	n := len(d.redoStack)
	if n == 0 {
		return Change{}, false
	}
	entry := d.redoStack[n-1]
	d.redoStack[n-1] = nil
	d.redoStack = d.redoStack[:n-1]
	d.pushUndo(entry)
	d.version++
	d.root = entry.after
	return d.changeForEdits(entry.forward), true
}

func copyEdits(edits []Edit) []Edit { return append([]Edit(nil), edits...) }

func normalizedEdits(root *piece, edits []Edit) ([]Edit, error) {
	sorted := copyEdits(edits)
	sort.SliceStable(sorted, func(i, j int) bool {
		if sorted[i].Start != sorted[j].Start {
			return sorted[i].Start < sorted[j].Start
		}
		return sorted[i].End < sorted[j].End
	})
	for i, edit := range sorted {
		if !validRange(root, edit.Start, edit.End) {
			return nil, ErrRange
		}
		if !utf8.ValidString(edit.Text) {
			return nil, ErrInvalidUTF8
		}
		if i > 0 && sorted[i-1].End > edit.Start {
			return nil, ErrOverlap
		}
	}
	result := sorted[:0]
	for _, edit := range sorted {
		if readRange(root, edit.Start, edit.End) != edit.Text {
			result = append(result, edit)
		}
	}
	return result, nil
}
