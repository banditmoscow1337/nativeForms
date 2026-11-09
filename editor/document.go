// Package editor provides an editable text document for code-oriented widgets.
// It does not own a caret or a selection; those belong to the view.
package editor

import (
	"errors"
	"sort"
	"strings"
	"unicode/utf8"
	"unsafe"
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

type historyEntry struct {
	before, after *piece
	forward       []Edit
	backward      []Edit
	fwd1, bwd1    [1]Edit
	retired       []*piece
	created       []*piece
	retireBuf     [32]*piece
	createBuf     [32]*piece
}

func (e *historyEntry) reset() {
	e.before, e.after = nil, nil
	e.forward, e.backward = nil, nil
	e.fwd1[0], e.bwd1[0] = Edit{}, Edit{}
	clear(e.retired)
	clear(e.created)
	e.retired = e.retireBuf[:0]
	e.created = e.createBuf[:0]
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
	freeHistory  []*historyEntry
	undoRing     []*historyEntry
	undoHead     int
	undoCount    int
	redoStack    []*historyEntry
	scratchEntry historyEntry
	editRing     [64]Edit
	editRingPos  int
}

func NewDocument(text string) (*Document, error) {
	if !utf8.ValidString(text) {
		return nil, ErrInvalidUTF8
	}
	d := &Document{version: 1, seed: 0x243f6a8885a308d3, curTx: 1, HistoryLimit: 256}
	d.scratchEntry.retired = d.scratchEntry.retireBuf[:0]
	d.scratchEntry.created = d.scratchEntry.createBuf[:0]
	d.root = d.makeInitialPieces(text)
	if len(text) > pieceSize*16 {
		d.ensurePools()
	}
	return d, nil
}

func (d *Document) ensurePools() {
	if d.historySlots != nil {
		return
	}
	limit := max(16, d.HistoryLimit)
	ringCap := 1
	for ringCap <= limit+2 {
		ringCap <<= 1
	}
	d.historySlots = make([]historyEntry, limit+4)
	d.freeHistory = make([]*historyEntry, 0, len(d.historySlots))
	for i := len(d.historySlots) - 1; i >= 0; i-- {
		entry := &d.historySlots[i]
		entry.retired = entry.retireBuf[:0]
		entry.created = entry.createBuf[:0]
		d.freeHistory = append(d.freeHistory, entry)
	}
	d.undoRing = make([]*historyEntry, ringCap)
	d.redoStack = make([]*historyEntry, 0, limit+2)
	if len(d.nodeSlab) == cap(d.nodeSlab) {
		d.nodeSlab = make([]piece, 0, 4096)
	}
	if d.freePieces == nil {
		d.freePieces = make([]*piece, 0, 512)
	}
}

func (d *Document) singleEditSlice(edit Edit) []Edit {
	pos := d.editRingPos & (len(d.editRing) - 1)
	d.editRingPos++
	d.editRing[pos] = edit
	return d.editRing[pos : pos+1 : pos+1]
}

func (d *Document) allocHistoryEntry() *historyEntry {
	d.ensurePools()
	if n := len(d.freeHistory); n > 0 {
		entry := d.freeHistory[n-1]
		d.freeHistory = d.freeHistory[:n-1]
		entry.reset()
		return entry
	}
	entry := &historyEntry{}
	entry.reset()
	return entry
}

func (d *Document) releaseHistoryEntry(entry *historyEntry) {
	if entry == nil || entry == &d.scratchEntry {
		return
	}
	entry.reset()
	d.freeHistory = append(d.freeHistory, entry)
}

func (d *Document) pushUndo(entry *historyEntry) {
	d.ensurePools()
	if d.undoCount == len(d.undoRing) {
		newCap := len(d.undoRing) * 2
		grown := make([]*historyEntry, newCap)
		mask := len(d.undoRing) - 1
		for i := 0; i < d.undoCount; i++ {
			grown[i] = d.undoRing[(d.undoHead+i)&mask]
		}
		d.undoRing = grown
		d.undoHead = 0
	}
	mask := len(d.undoRing) - 1
	d.undoRing[(d.undoHead+d.undoCount)&mask] = entry
	d.undoCount++
}

func (d *Document) popOldestUndo() *historyEntry {
	if d.undoCount == 0 {
		return nil
	}
	mask := len(d.undoRing) - 1
	entry := d.undoRing[d.undoHead]
	d.undoRing[d.undoHead] = nil
	d.undoHead = (d.undoHead + 1) & mask
	d.undoCount--
	return entry
}

func (d *Document) popNewestUndo() *historyEntry {
	if d.undoCount == 0 {
		return nil
	}
	mask := len(d.undoRing) - 1
	d.undoCount--
	idx := (d.undoHead + d.undoCount) & mask
	entry := d.undoRing[idx]
	d.undoRing[idx] = nil
	return entry
}

func (d *Document) trimUndoTo(limit int) {
	for d.undoCount > limit {
		oldest := d.popOldestUndo()
		if oldest != nil {
			d.freePieces = append(d.freePieces, oldest.retired...)
			d.releaseHistoryEntry(oldest)
		}
	}
}

func (d *Document) clearRedo() {
	for i := len(d.redoStack) - 1; i >= 0; i-- {
		entry := d.redoStack[i]
		d.redoStack[i] = nil
		if entry != nil {
			d.freePieces = append(d.freePieces, entry.created...)
			d.releaseHistoryEntry(entry)
		}
	}
	d.redoStack = d.redoStack[:0]
}

func (d *Document) Version() uint64 { return d.version }
func (d *Document) Len() int        { return pieceBytes(d.root) }
func (d *Document) LineCount() int  { return pieceLines(d.root) + 1 }

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

// LineAtOffset returns the zero-based line number for a valid text offset,
// rejecting offsets inside UTF-8 runes or between CR and LF.
func (d *Document) LineAtOffset(offset int) (int, error) {
	if !validBoundary(d.root, offset) {
		return 0, ErrRange
	}
	if offset > 0 && offset < pieceBytes(d.root) && byteAt(d.root, offset) == '\n' && byteAt(d.root, offset-1) == '\r' {
		return 0, ErrPosition
	}
	return newlinesBefore(d.root, offset), nil
}

// LineRange returns byte offsets for the text on a line, excluding its LF or
// CRLF terminator. Files with lone CR line endings are not split into lines.
func (d *Document) LineRange(line int) (start, end int, err error) {
	count := d.LineCount()
	if line < 0 || line >= count {
		return 0, 0, ErrPosition
	}
	if line == 0 {
		end = d.Len()
		if count > 1 {
			end = nthNewline(d.root, 0)
			if end > 0 && byteAt(d.root, end-1) == '\r' {
				end--
			}
		}
		return 0, end, nil
	}
	if line < count-1 {
		if s, e, ok := lineRangePair(d.root, line-1); ok {
			return s, e, nil
		}
	}
	start = nthNewline(d.root, line-1) + 1
	end = d.Len()
	if line < count-1 {
		end = nthNewline(d.root, line)
		if end > start && byteAt(d.root, end-1) == '\r' {
			end--
		}
	}
	return start, end, nil
}

func lineRangePair(root *piece, firstNth int) (start, end int, ok bool) {
	offset := 0
	for root != nil {
		leftLines := pieceLines(root.left)
		if firstNth < leftLines {
			root = root.left
			continue
		}
		firstNth -= leftLines
		offset += pieceBytes(root.left)
		own := int(root.ownLines)
		if firstNth >= own {
			firstNth -= own
			offset += len(root.text)
			root = root.right
			continue
		}
		if firstNth+1 >= own {
			return 0, 0, false
		}
		text := root.text
		pos := 0
		for n := firstNth; n >= 0; n-- {
			idx := strings.IndexByte(text[pos:], '\n')
			if idx < 0 {
				return 0, 0, false
			}
			pos += idx + 1
		}
		start = offset + pos
		nextIdx := strings.IndexByte(text[pos:], '\n')
		if nextIdx < 0 {
			return 0, 0, false
		}
		end = start + nextIdx
		if nextIdx > 0 && text[pos+nextIdx-1] == '\r' {
			end--
		}
		return start, end, true
	}
	return 0, 0, false
}

// ScanLinesFrom visits lines in-order starting at fromLine without per-line
// tree lookups or heap allocations. The string passed to fn is valid only for
// the duration of that callback invocation.
func (d *Document) ScanLinesFrom(fromLine int, scratch []byte, fn func(line int, text string) bool) []byte {
	if d == nil || d.root == nil {
		if fromLine == 0 {
			fn(0, "")
		}
		return scratch
	}
	count := d.LineCount()
	if fromLine < 0 || fromLine >= count {
		return scratch
	}
	startOffset := 0
	if fromLine > 0 {
		startOffset = nthNewline(d.root, fromLine-1) + 1
	}
	line := fromLine
	spanning := false
	scratch = scratch[:0]

	var stackBuf [64]*piece
	stack := stackBuf[:0]
	cur := d.root
	baseOffset := 0

	// Descend to the piece containing startOffset while pushing right-ancestors.
	for cur != nil {
		leftBytes := pieceBytes(cur.left)
		if startOffset < baseOffset+leftBytes {
			stack = append(stack, cur)
			cur = cur.left
		} else if startOffset < baseOffset+leftBytes+len(cur.text) {
			break
		} else {
			baseOffset += leftBytes + len(cur.text)
			cur = cur.right
		}
	}

	firstWithin := 0
	if cur != nil {
		firstWithin = startOffset - (baseOffset + pieceBytes(cur.left))
	}

	for cur != nil {
		text := cur.text
		if firstWithin > 0 {
			text = text[firstWithin:]
			firstWithin = 0
		}
		for len(text) > 0 {
			nl := strings.IndexByte(text, '\n')
			if nl < 0 {
				scratch = append(scratch, text...)
				spanning = true
				break
			}
			part := text[:nl]
			text = text[nl+1:]
			var seg string
			if !spanning {
				if len(part) > 0 && part[len(part)-1] == '\r' {
					part = part[:len(part)-1]
				}
				seg = part
			} else {
				scratch = append(scratch, part...)
				if len(scratch) > 0 && scratch[len(scratch)-1] == '\r' {
					scratch = scratch[:len(scratch)-1]
				}
				if len(scratch) > 0 {
					seg = unsafe.String(unsafe.SliceData(scratch), len(scratch))
				}
				spanning = false
			}
			scratch = scratch[:0]
			if !fn(line, seg) {
				return scratch
			}
			line++
			if line >= count {
				return scratch
			}
		}
		if cur.right != nil {
			cur = cur.right
			for cur.left != nil {
				stack = append(stack, cur)
				cur = cur.left
			}
		} else if len(stack) > 0 {
			cur = stack[len(stack)-1]
			stack = stack[:len(stack)-1]
		} else {
			cur = nil
		}
	}

	if line < count {
		var seg string
		if len(scratch) > 0 {
			seg = unsafe.String(unsafe.SliceData(scratch), len(scratch))
		}
		scratch = scratch[:0]
		fn(line, seg)
	}
	return scratch
}

// UTF16Position converts an offset at a rune boundary to an LSP position.
// Offsets within a CRLF terminator are not valid text positions.
func (d *Document) UTF16Position(offset int) (Position, error) {
	if !validBoundary(d.root, offset) {
		return Position{}, ErrRange
	}
	if offset > 0 && offset < pieceBytes(d.root) && byteAt(d.root, offset) == '\n' && byteAt(d.root, offset-1) == '\r' {
		return Position{}, ErrPosition
	}
	line := newlinesBefore(d.root, offset)
	start := 0
	if line > 0 {
		start = nthNewline(d.root, line-1) + 1
	}
	if s, ok := sliceSinglePieceNode(d.root, start, offset); ok {
		return Position{Line: line, Character: utf16Count(s)}, nil
	}
	return Position{Line: line, Character: utf16Before(d.root, offset) - utf16Before(d.root, start)}, nil
}

// UTF16Offset returns the absolute UTF-16 code-unit offset at a UTF-8 rune
// boundary. It is used by native text input APIs whose ranges cover the whole
// document rather than one line.
func (d *Document) UTF16Offset(offset int) (int, error) {
	if !validBoundary(d.root, offset) {
		return 0, ErrRange
	}
	return utf16Before(d.root, offset), nil
}

func utf16Before(n *piece, offset int) int {
	count := 0
	for n != nil {
		left := pieceBytes(n.left)
		if offset < left {
			n = n.left
			continue
		}
		count += pieceUTF16(n.left)
		offset -= left
		if offset == 0 {
			break
		}
		if offset < len(n.text) {
			count += utf16Count(n.text[:offset])
			break
		}
		count += int(n.ownUTF16)
		offset -= len(n.text)
		if offset == 0 {
			break
		}
		n = n.right
	}
	return count
}

// ByteOffset converts an LSP position to an offset. A column inside a UTF-16
// surrogate pair or beyond the line's text is rejected.
func (d *Document) ByteOffset(pos Position) (int, error) {
	if pos.Character < 0 {
		return 0, ErrPosition
	}
	start, end, err := d.LineRange(pos.Line)
	if err != nil {
		return 0, err
	}
	column, offset := 0, start
	for _, r := range readRange(d.root, start, end) {
		if column == pos.Character {
			return offset, nil
		}
		units := 1
		if r > 0xffff {
			units = 2
		}
		if pos.Character < column+units {
			return 0, ErrPosition
		}
		column += units
		offset += utf8.RuneLen(r)
	}
	if column == pos.Character {
		return end, nil
	}
	return 0, ErrPosition
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
	d.clearRedo()
	var entry *historyEntry
	if d.HistoryLimit > 0 {
		d.trimUndoTo(d.HistoryLimit - 1)
		entry = d.allocHistoryEntry()
	} else {
		d.trimUndoTo(0)
		entry = &d.scratchEntry
		entry.reset()
	}
	d.curTx++
	if d.curTx == 0 {
		d.curTx = 1
	}
	before := d.root
	if updated, ok := d.replaceExactPiece(before, edit.Start, edit.End, edit.Text, entry); ok {
		d.root = updated
	} else {
		entry.fwd1[0] = edit
		d.root = d.applySorted(before, entry.fwd1[:1], entry)
	}
	d.version++
	if d.HistoryLimit > 0 {
		entry.before = before
		entry.after = d.root
		entry.fwd1[0] = edit
		entry.bwd1[0] = Edit{
			Start: edit.Start,
			End:   edit.Start + len(edit.Text),
			Text:  removed,
		}
		entry.forward = entry.fwd1[:1]
		entry.backward = entry.bwd1[:1]
		d.pushUndo(entry)
	} else {
		d.freePieces = append(d.freePieces, entry.retired...)
		entry.reset()
	}
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
	d.clearRedo()
	var entry *historyEntry
	if d.HistoryLimit > 0 {
		d.trimUndoTo(d.HistoryLimit - 1)
		entry = d.allocHistoryEntry()
	} else {
		d.trimUndoTo(0)
		entry = &d.scratchEntry
		entry.reset()
	}
	d.curTx++
	if d.curTx == 0 {
		d.curTx = 1
	}
	before := d.root
	d.root = d.applySorted(d.root, forward, entry)
	d.version++
	if d.HistoryLimit > 0 {
		entry.before = before
		entry.after = d.root
		entry.forward = forward
		entry.backward = backward
		d.pushUndo(entry)
	} else {
		d.freePieces = append(d.freePieces, entry.retired...)
		entry.reset()
	}
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
	if len(entry.backward) == 1 {
		return Change{BeforeVersion: d.version - 1, Version: d.version, Edits: d.singleEditSlice(entry.backward[0])}, true
	}
	return Change{BeforeVersion: d.version - 1, Version: d.version, Edits: copyEdits(entry.backward)}, true
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
	if len(entry.forward) == 1 {
		return Change{BeforeVersion: d.version - 1, Version: d.version, Edits: d.singleEditSlice(entry.forward[0])}, true
	}
	return Change{BeforeVersion: d.version - 1, Version: d.version, Edits: copyEdits(entry.forward)}, true
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

// replaceExactPiece replaces a single piece in-place along the root-to-node
// path when [start, end) matches the exact boundaries of an existing piece and
// the replacement text fits in one piece. Because the node's priority and
// children are unchanged, no treap splits or joins are needed.
func (d *Document) replaceExactPiece(root *piece, start, end int, text string, entry *historyEntry) (*piece, bool) {
	if root == nil || start >= end || len(text) == 0 || len(text) > pieceSize {
		return nil, false
	}
	var path [64]*piece
	var dir [64]uint8
	depth := 0
	cur := root
	s, e := start, end
	for cur != nil {
		if depth >= len(path) {
			return nil, false
		}
		leftBytes := pieceBytes(cur.left)
		if e <= leftBytes {
			path[depth] = cur
			dir[depth] = 0
			depth++
			cur = cur.left
		} else if s >= leftBytes+len(cur.text) {
			path[depth] = cur
			dir[depth] = 1
			depth++
			s -= leftBytes + len(cur.text)
			e -= leftBytes + len(cur.text)
			cur = cur.right
		} else if s == leftBytes && e == leftBytes+len(cur.text) {
			child := d.allocPiece(entry)
			initPiece(child, text, cur.priority, d.curTx)
			child.left = cur.left
			child.right = cur.right
			update(child)
			entry.retired = append(entry.retired, cur)
			for i := depth - 1; i >= 0; i-- {
				parent := path[i]
				entry.retired = append(entry.retired, parent)
				cloned := d.allocPiece(entry)
				*cloned = *parent
				cloned.tx = d.curTx
				if dir[i] == 0 {
					cloned.left = child
				} else {
					cloned.right = child
				}
				update(cloned)
				child = cloned
			}
			return child, true
		} else {
			return nil, false
		}
	}
	return nil, false
}

func (d *Document) applySorted(root *piece, edits []Edit, entry *historyEntry) *piece {
	for i := len(edits) - 1; i >= 0; i-- {
		edit := edits[i]
		left, tail := d.split(root, edit.Start, entry)
		deleted, right := d.split(tail, edit.End-edit.Start, entry)
		d.releaseSubtree(deleted, entry)
		root = d.join(d.join(left, d.makePiecesTx(edit.Text, entry), entry), right, entry)
	}
	return root
}

type piece struct {
	left, right        *piece
	text               string
	priority           uint64
	bytes, lines       int32
	utf16              int32
	ownLines, ownUTF16 int16
	tx                 uint32
	ownLex, subLex     uint8
}

func pieceBytes(n *piece) int {
	if n == nil {
		return 0
	}
	return int(n.bytes)
}

func pieceLines(n *piece) int {
	if n == nil {
		return 0
	}
	return int(n.lines)
}

func pieceUTF16(n *piece) int {
	if n == nil {
		return 0
	}
	return int(n.utf16)
}

func utf16Count(text string) int {
	hasMulti := false
	for i := 0; i < len(text); i++ {
		if text[i] >= 0x80 {
			hasMulti = true
			break
		}
	}
	if !hasMulti {
		return len(text)
	}
	count := 0
	for _, r := range text {
		count++
		if r > 0xffff {
			count++
		}
	}
	return count
}

func computePieceMetrics(text string) (lines, utf16 int, lex uint8) {
	hasMulti := false
	for i := 0; i < len(text); i++ {
		b := text[i]
		if b >= 0x80 {
			hasMulti = true
			continue
		}
		switch b {
		case '\n':
			lines++
		case '`':
			lex |= lexFlagBacktick
		case '/':
			lex |= lexFlagSlash
		case '*':
			lex |= lexFlagStar
		}
	}
	if !hasMulti {
		return lines, len(text), lex
	}
	for _, r := range text {
		utf16++
		if r > 0xffff {
			utf16++
		}
	}
	return lines, utf16, lex
}

func initPiece(n *piece, text string, priority uint64, tx uint32) {
	lines, u16, lex := computePieceMetrics(text)
	*n = piece{
		text:     text,
		priority: priority,
		bytes:    int32(len(text)),
		lines:    int32(lines),
		utf16:    int32(u16),
		ownLines: int16(lines),
		ownUTF16: int16(u16),
		tx:       tx,
		ownLex:   lex,
		subLex:   lex,
	}
}

func update(n *piece) {
	l, r := n.left, n.right
	b := int32(len(n.text))
	ln := int32(n.ownLines)
	u := int32(n.ownUTF16)
	lx := n.ownLex
	if l != nil {
		b += l.bytes
		ln += l.lines
		u += l.utf16
		lx |= l.subLex
	}
	if r != nil {
		b += r.bytes
		ln += r.lines
		u += r.utf16
		lx |= r.subLex
	}
	n.bytes = b
	n.lines = ln
	n.utf16 = u
	n.subLex = lx
}

func (d *Document) allocPiece(entry *historyEntry) *piece {
	var n *piece
	if freeCount := len(d.freePieces); freeCount > 0 {
		n = d.freePieces[freeCount-1]
		d.freePieces = d.freePieces[:freeCount-1]
	} else {
		if len(d.nodeSlab) == cap(d.nodeSlab) {
			d.nodeSlab = make([]piece, 0, 1024)
		}
		idx := len(d.nodeSlab)
		d.nodeSlab = d.nodeSlab[:idx+1]
		n = &d.nodeSlab[idx]
	}
	if entry != nil {
		entry.created = append(entry.created, n)
	}
	return n
}

func (d *Document) freeCreatedPiece(n *piece, entry *historyEntry) {
	if entry != nil {
		for i := len(entry.created) - 1; i >= 0; i-- {
			if entry.created[i] == n {
				last := len(entry.created) - 1
				entry.created[i] = entry.created[last]
				entry.created[last] = nil
				entry.created = entry.created[:last]
				break
			}
		}
	}
	*n = piece{}
	d.freePieces = append(d.freePieces, n)
}

func (d *Document) editablePiece(n *piece, entry *historyEntry) *piece {
	if n.tx == d.curTx {
		return n
	}
	if entry != nil {
		entry.retired = append(entry.retired, n)
	}
	c := d.allocPiece(entry)
	*c = *n
	c.tx = d.curTx
	return c
}

func (d *Document) newPieceTx(text string, priority uint64, entry *historyEntry) *piece {
	n := d.allocPiece(entry)
	initPiece(n, text, priority, d.curTx)
	return n
}

func (d *Document) releaseSubtree(n *piece, entry *historyEntry) {
	if n == nil {
		return
	}
	left, right := n.left, n.right
	if n.tx == d.curTx {
		d.freeCreatedPiece(n, entry)
	} else if entry != nil {
		entry.retired = append(entry.retired, n)
	}
	d.releaseSubtree(left, entry)
	d.releaseSubtree(right, entry)
}

func (d *Document) nextPriority() uint64 {
	d.seed += 0x9e3779b97f4a7c15
	z := d.seed
	z = (z ^ (z >> 30)) * 0xbf58476d1ce4e5b9
	z = (z ^ (z >> 27)) * 0x94d049bb133111eb
	return z ^ (z >> 31)
}

func countTextPieces(text string) int {
	count := 0
	for len(text) > 0 {
		size := min(pieceSize, len(text))
		for size < len(text) && text[size]&0xc0 == 0x80 {
			size--
		}
		count++
		text = text[size:]
	}
	return count
}

// makeInitialPieces builds the initial treap in O(N) using a single contiguous
// slice allocation and a Cartesian tree right-spine stack.
func (d *Document) makeInitialPieces(text string) *piece {
	if len(text) == 0 {
		d.nodeSlab = make([]piece, 0, 64)
		return nil
	}
	count := countTextPieces(text)
	extra := 128
	if count > 64 {
		extra = 4096
	}
	slab := make([]piece, count, count+extra)
	d.nodeSlab = slab

	var spineBuf [128]*piece
	spine := spineBuf[:0]
	for i := 0; i < count; i++ {
		size := min(pieceSize, len(text))
		for size < len(text) && text[size]&0xc0 == 0x80 {
			size--
		}
		cur := &slab[i]
		initPiece(cur, text[:size], d.nextPriority(), d.curTx)
		text = text[size:]

		var lastPopped *piece
		for len(spine) > 0 && spine[len(spine)-1].priority < cur.priority {
			top := spine[len(spine)-1]
			spine = spine[:len(spine)-1]
			update(top)
			lastPopped = top
		}
		cur.left = lastPopped
		if len(spine) > 0 {
			spine[len(spine)-1].right = cur
		}
		spine = append(spine, cur)
	}
	for i := len(spine) - 1; i >= 0; i-- {
		update(spine[i])
	}
	return spine[0]
}

func (d *Document) makePiecesTx(text string, entry *historyEntry) *piece {
	var root *piece
	for len(text) > 0 {
		size := min(pieceSize, len(text))
		for size < len(text) && text[size]&0xc0 == 0x80 {
			size--
		}
		root = d.join(root, d.newPieceTx(text[:size], d.nextPriority(), entry), entry)
		text = text[size:]
	}
	return root
}

// join keeps the heap invariant while sharing all untouched subtrees.
func (d *Document) join(left, right *piece, entry *historyEntry) *piece {
	if left == nil {
		return right
	}
	if right == nil {
		return left
	}
	if left.priority >= right.priority {
		n := d.editablePiece(left, entry)
		n.right = d.join(n.right, right, entry)
		update(n)
		return n
	}
	n := d.editablePiece(right, entry)
	n.left = d.join(left, n.left, entry)
	update(n)
	return n
}

func (d *Document) split(root *piece, offset int, entry *historyEntry) (*piece, *piece) {
	if root == nil {
		return nil, nil
	}
	leftBytes := pieceBytes(root.left)
	if offset < leftBytes {
		left, middle := d.split(root.left, offset, entry)
		tail := d.editablePiece(root, entry)
		tail.left = nil
		update(tail)
		return left, d.join(middle, tail, entry)
	}
	end := leftBytes + len(root.text)
	if offset > end {
		middle, right := d.split(root.right, offset-end, entry)
		head := d.editablePiece(root, entry)
		head.right = nil
		update(head)
		return d.join(head, middle, entry), right
	}
	if offset == leftBytes {
		left := root.left
		tail := d.editablePiece(root, entry)
		tail.left = nil
		update(tail)
		return left, tail
	}
	if offset == end {
		right := root.right
		head := d.editablePiece(root, entry)
		head.right = nil
		update(head)
		return head, right
	}
	within := offset - leftBytes
	leftChild, rightChild := root.left, root.right
	text := root.text
	if root.tx == d.curTx {
		d.freeCreatedPiece(root, entry)
	} else if entry != nil {
		entry.retired = append(entry.retired, root)
	}
	return d.join(leftChild, d.newPieceTx(text[:within], d.nextPriority(), entry), entry),
		d.join(d.newPieceTx(text[within:], d.nextPriority(), entry), rightChild, entry)
}

func validBoundary(root *piece, offset int) bool {
	return offset >= 0 && offset <= pieceBytes(root) &&
		(offset == pieceBytes(root) || byteAt(root, offset)&0xc0 != 0x80)
}

func validRange(root *piece, start, end int) bool {
	return start <= end && validBoundary(root, start) && validBoundary(root, end)
}

func byteAt(root *piece, offset int) byte {
	for root != nil {
		left := pieceBytes(root.left)
		if offset < left {
			root = root.left
		} else if offset < left+len(root.text) {
			return root.text[offset-left]
		} else {
			offset -= left + len(root.text)
			root = root.right
		}
	}
	return 0
}

func sliceSinglePieceNode(n *piece, start, end int) (string, bool) {
	if start == end {
		return "", true
	}
	for n != nil {
		left := pieceBytes(n.left)
		if end <= left {
			n = n.left
		} else if start >= left+len(n.text) {
			start -= left + len(n.text)
			end -= left + len(n.text)
			n = n.right
		} else if start >= left && end <= left+len(n.text) {
			return n.text[start-left : end-left], true
		} else {
			return "", false
		}
	}
	return "", false
}

func readRange(root *piece, start, end int) string {
	if start >= end {
		return ""
	}
	if s, ok := sliceSinglePieceNode(root, start, end); ok {
		return s
	}
	buf := appendRangeBytes(make([]byte, 0, end-start), root, start, end)
	return string(buf)
}

// SliceSinglePiece returns a substring without allocating when [start, end)
// resides completely inside one piece node.
func (d *Document) SliceSinglePiece(start, end int) (string, bool) {
	if !validRange(d.root, start, end) {
		return "", false
	}
	return sliceSinglePieceNode(d.root, start, end)
}

// AppendRange appends the byte range [start, end) to dst without intermediate
// heap allocations when capacity is sufficient.
func (d *Document) AppendRange(dst []byte, start, end int) ([]byte, error) {
	if !validRange(d.root, start, end) {
		return dst, ErrRange
	}
	return appendRangeBytes(dst, d.root, start, end), nil
}

func appendRangeBytes(dst []byte, root *piece, start, end int) []byte {
	if root == nil || start >= end {
		return dst
	}
	left := pieceBytes(root.left)
	if start < left {
		dst = appendRangeBytes(dst, root.left, start, min(end, left))
	}
	if start < left+len(root.text) && end > left {
		dst = append(dst, root.text[max(0, start-left):min(len(root.text), end-left)]...)
	}
	if end > left+len(root.text) {
		dst = appendRangeBytes(dst, root.right, max(0, start-left-len(root.text)), end-left-len(root.text))
	}
	return dst
}

func nthNewline(root *piece, nth int) int {
	offset := 0
	for root != nil {
		leftLines := pieceLines(root.left)
		if nth < leftLines {
			root = root.left
			continue
		}
		nth -= leftLines
		offset += pieceBytes(root.left)
		own := int(root.ownLines)
		if nth >= own {
			nth -= own
			offset += len(root.text)
			root = root.right
			continue
		}
		text := root.text
		pos := 0
		for nth >= 0 {
			idx := strings.IndexByte(text[pos:], '\n')
			if idx < 0 {
				break
			}
			if nth == 0 {
				return offset + pos + idx
			}
			pos += idx + 1
			nth--
		}
		offset += len(root.text)
		root = root.right
	}
	return -1
}

func newlinesBefore(root *piece, offset int) int {
	count := 0
	for root != nil {
		left := pieceBytes(root.left)
		if offset <= left {
			root = root.left
			continue
		}
		count += pieceLines(root.left)
		offset -= left
		if offset < len(root.text) {
			count += strings.Count(root.text[:offset], "\n")
			break
		}
		count += int(root.ownLines)
		offset -= len(root.text)
		if offset == 0 {
			break
		}
		root = root.right
	}
	return count
}
