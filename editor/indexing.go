package editor

import (
	"strings"
	"unicode/utf8"
	"unsafe"
)

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
