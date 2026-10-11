// Package editor provides an editable text document for code-oriented widgets.
// It does not own a caret or a selection; those belong to the view.
package editor

import (
	"errors"
	"sort"
	"strings"
	"unicode/utf8"
)

var (
	ErrInvalidUTF8 = errors.New("editor: text is not valid UTF-8")
	ErrRange       = errors.New("editor: invalid byte range or UTF-8 boundary")
	ErrOverlap     = errors.New("editor: edits overlap")
	ErrPosition    = errors.New("editor: invalid UTF-16 position")
)

const pieceSize = 4096

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
	before, after     *piece
	forward, backward []Edit
}

// Document stores immutable pieces in an indexed tree. Edits, line lookup and
// undo history do not require copying the entire document. Document is not
// safe for concurrent use; apply background results on the UI goroutine.
type Document struct {
	root         *piece
	version      uint64
	seed         uint64
	undo, redo   []historyEntry
	HistoryLimit int // Transactions retained for undo; zero records no new history.
}

func NewDocument(text string) (*Document, error) {
	if !utf8.ValidString(text) {
		return nil, ErrInvalidUTF8
	}
	d := &Document{version: 1, seed: 0x243f6a8885a308d3, HistoryLimit: 256}
	d.root = d.makePieces(text)
	return d, nil
}

func (d *Document) Version() uint64 { return d.version }
func (d *Document) Len() int        { return pieceBytes(d.root) }
func (d *Document) LineCount() int  { return pieceLines(d.root) + 1 }

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

// LineRange returns byte offsets for the text on a line, excluding its LF or
// CRLF terminator. Files with lone CR line endings are not split into lines.
func (d *Document) LineRange(line int) (start, end int, err error) {
	if line < 0 || line >= d.LineCount() {
		return 0, 0, ErrPosition
	}
	if line > 0 {
		start = nthNewline(d.root, line-1) + 1
	}
	end = d.Len()
	if line < d.LineCount()-1 {
		end = nthNewline(d.root, line)
		if end > start && byteAt(d.root, end-1) == '\r' {
			end--
		}
	}
	return start, end, nil
}

// UTF16Position converts an offset at a rune boundary to an LSP position.
// Offsets within a CRLF terminator are not valid text positions.
func (d *Document) UTF16Position(offset int) (Position, error) {
	if !validBoundary(d.root, offset) {
		return Position{}, ErrRange
	}
	line := newlinesBefore(d.root, offset)
	start, end, _ := d.LineRange(line)
	if offset > end {
		return Position{}, ErrPosition
	}
	column := 0
	for _, r := range readRange(d.root, start, offset) {
		column++
		if r > 0xffff {
			column++
		}
	}
	return Position{Line: line, Character: column}, nil
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

// Apply commits non-overlapping edits against the same original document.
// Equal-offset insertions keep their input order. An empty or unchanged batch
// leaves the version and redo history untouched.
func (d *Document) Apply(edits []Edit) (Change, error) {
	change := Change{BeforeVersion: d.version, Version: d.version}
	forward, err := normalizedEdits(d.root, edits)
	if err != nil {
		return change, err
	}
	if len(forward) == 0 {
		return change, nil
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
	before := d.root
	d.root = d.applySorted(d.root, forward)
	d.version++
	d.redo = nil
	if d.HistoryLimit > 0 {
		d.undo = append(d.undo, historyEntry{before, d.root, forward, backward})
		if len(d.undo) > d.HistoryLimit {
			copy(d.undo, d.undo[len(d.undo)-d.HistoryLimit:])
			clear(d.undo[d.HistoryLimit:])
			d.undo = d.undo[:d.HistoryLimit]
		}
	} else {
		d.undo = nil
	}
	return Change{BeforeVersion: d.version - 1, Version: d.version, Edits: copyEdits(forward)}, nil
}

func (d *Document) Undo() (Change, bool) {
	if len(d.undo) == 0 {
		return Change{}, false
	}
	i := len(d.undo) - 1
	entry := d.undo[i]
	d.undo[i] = historyEntry{}
	d.undo = d.undo[:i]
	d.root = entry.before
	d.redo = append(d.redo, entry)
	d.version++
	return Change{BeforeVersion: d.version - 1, Version: d.version, Edits: copyEdits(entry.backward)}, true
}

func (d *Document) Redo() (Change, bool) {
	if len(d.redo) == 0 {
		return Change{}, false
	}
	i := len(d.redo) - 1
	entry := d.redo[i]
	d.redo[i] = historyEntry{}
	d.redo = d.redo[:i]
	d.root = entry.after
	d.undo = append(d.undo, entry)
	d.version++
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

func (d *Document) applySorted(root *piece, edits []Edit) *piece {
	for i := len(edits) - 1; i >= 0; i-- {
		edit := edits[i]
		left, tail := d.split(root, edit.Start)
		_, right := d.split(tail, edit.End-edit.Start)
		root = join(join(left, d.makePieces(edit.Text)), right)
	}
	return root
}

type piece struct {
	left, right *piece
	text        string
	priority    uint64
	bytes, lines int
}

func pieceBytes(n *piece) int {
	if n == nil {
		return 0
	}
	return n.bytes
}

func pieceLines(n *piece) int {
	if n == nil {
		return 0
	}
	return n.lines
}

func update(n *piece) {
	n.bytes = pieceBytes(n.left) + len(n.text) + pieceBytes(n.right)
	n.lines = pieceLines(n.left) + strings.Count(n.text, "\n") + pieceLines(n.right)
}

func node(text string, priority uint64) *piece {
	n := &piece{text: text, priority: priority}
	update(n)
	return n
}

func (d *Document) nextPriority() uint64 {
	d.seed += 0x9e3779b97f4a7c15
	z := d.seed
	z = (z ^ (z >> 30)) * 0xbf58476d1ce4e5b9
	z = (z ^ (z >> 27)) * 0x94d049bb133111eb
	return z ^ (z >> 31)
}

func (d *Document) makePieces(text string) *piece {
	var root *piece
	for len(text) > 0 {
		size := min(pieceSize, len(text))
		for size < len(text) && text[size]&0xc0 == 0x80 {
			size--
		}
		root = join(root, node(text[:size], d.nextPriority()))
		text = text[size:]
	}
	return root
}

// join keeps the heap invariant while sharing all untouched subtrees.
func join(left, right *piece) *piece {
	if left == nil {
		return right
	}
	if right == nil {
		return left
	}
	if left.priority >= right.priority {
		n := *left
		n.right = join(n.right, right)
		update(&n)
		return &n
	}
	n := *right
	n.left = join(left, n.left)
	update(&n)
	return &n
}

func (d *Document) split(root *piece, offset int) (*piece, *piece) {
	if root == nil {
		return nil, nil
	}
	leftBytes := pieceBytes(root.left)
	if offset < leftBytes {
		left, middle := d.split(root.left, offset)
		tail := *root
		tail.left = nil
		update(&tail)
		return left, join(middle, &tail)
	}
	end := leftBytes + len(root.text)
	if offset > end {
		middle, right := d.split(root.right, offset-end)
		head := *root
		head.right = nil
		update(&head)
		return join(&head, middle), right
	}
	if offset == leftBytes {
		tail := *root
		tail.left = nil
		update(&tail)
		return root.left, &tail
	}
	if offset == end {
		head := *root
		head.right = nil
		update(&head)
		return &head, root.right
	}
	within := offset - leftBytes
	return join(root.left, node(root.text[:within], d.nextPriority())),
		join(node(root.text[within:], d.nextPriority()), root.right)
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

func readRange(root *piece, start, end int) string {
	var b strings.Builder
	b.Grow(end - start)
	appendRange(&b, root, start, end)
	return b.String()
}

func appendRange(b *strings.Builder, root *piece, start, end int) {
	if root == nil || start >= end {
		return
	}
	left := pieceBytes(root.left)
	if start < left {
		appendRange(b, root.left, start, min(end, left))
	}
	if start < left+len(root.text) && end > left {
		b.WriteString(root.text[max(0, start-left):min(len(root.text), end-left)])
	}
	if end > left+len(root.text) {
		appendRange(b, root.right, max(0, start-left-len(root.text)), end-left-len(root.text))
	}
}

func nthNewline(root *piece, nth int) int {
	offset := 0
	for root != nil {
		if nth < pieceLines(root.left) {
			root = root.left
			continue
		}
		nth -= pieceLines(root.left)
		offset += pieceBytes(root.left)
		for i := 0; i < len(root.text); i++ {
			if root.text[i] == '\n' {
				if nth == 0 {
					return offset + i
				}
				nth--
			}
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
		within := min(offset, len(root.text))
		count += strings.Count(root.text[:within], "\n")
		if offset <= len(root.text) {
			break
		}
		offset -= len(root.text)
		root = root.right
	}
	return count
}
