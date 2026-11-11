package nativeforms

import (
	"sort"
	"unicode/utf8"
	"unsafe"
)

type codeLineLayout struct {
	display          []rune
	offsets, columns []int
	carets           []float32
	hasTabs          bool
}

func (e *CodeEditor) sliceLine(line, start, end int, transient bool) string {
	if start >= end {
		return ""
	}
	if s, ok := e.Document.SliceSinglePiece(start, end); ok {
		return s
	}
	if transient && e.fontContext() == nil && !hasTextShaper() {
		buf, err := e.Document.AppendRange(e.scratchLineBytes[:0], start, end)
		if err == nil {
			e.scratchLineBytes = buf
			return unsafe.String(unsafe.SliceData(buf), len(buf))
		}
	}
	if s, ok := e.crossPieceLines[line]; ok {
		return s
	}
	s, _ := e.Document.Slice(start, end)
	if e.crossPieceLines == nil {
		e.crossPieceLines = make(map[int]string, 16)
	}
	e.crossPieceLines[line] = s
	return s
}

func (e *CodeEditor) layoutLine(line string) codeLineLayout {
	offsets := append(e.scratchOffsets[:0], 0)
	columns := append(e.scratchColumns[:0], 0)
	display := e.scratchDisplay[:0]
	width := e.TabWidth
	if width <= 0 {
		width = 4
	}
	hasTabs := false
	column := 0
	for at, r := range line {
		if r == '\t' {
			hasTabs = true
			spaces := width - column%width
			for i := 0; i < spaces; i++ {
				display = append(display, ' ')
			}
			column += spaces
		} else {
			display = append(display, r)
			column++
		}
		offsets = append(offsets, at+utf8.RuneLen(r))
		columns = append(columns, len(display))
	}
	e.scratchOffsets = offsets
	e.scratchColumns = columns
	e.scratchDisplay = display
	var carets []float32
	spec := e.fontSpec()
	if e.fontContext() == nil && !hasTextShaper() {
		carets = textCaretPositionsRunesInto(display, spec.Size, e.scratchCarets[:0])
	} else if !hasTabs {
		carets = e.caretPositionsInto(line, spec, e.scratchCarets[:0])
	} else {
		carets = e.caretPositionsInto(string(display), spec, e.scratchCarets[:0])
	}
	e.scratchCarets = carets
	return codeLineLayout{
		display: display,
		offsets: offsets,
		columns: columns,
		carets:  carets,
		hasTabs: hasTabs,
	}
}

func (l codeLineLayout) xAt(byteOffset int) float32 {
	index := sort.SearchInts(l.offsets, byteOffset)
	if index == len(l.offsets) || l.offsets[index] != byteOffset {
		index = max(0, index-1)
	}
	return l.carets[l.columns[index]]
}

func (l codeLineLayout) byteAtX(line string, x float32) int {
	var runes []rune
	if !l.hasTabs {
		runes = l.display
	} else {
		runes = []rune(line)
	}
	best, distance := 0, float32(1e30)
	for i, offset := range l.offsets {
		if !graphemeBreak(runes, i) {
			continue
		}
		if d := absFloat32(x - l.carets[l.columns[i]]); d < distance {
			best, distance = offset, d
		}
	}
	return best
}

func (e *CodeEditor) lineAtCaret() (line, start, end int) {
	line, err := e.Document.LineAtOffset(e.Caret)
	if err != nil {
		return 0, 0, 0
	}
	start, end, _ = e.Document.LineRange(line)
	return line, start, end
}

func (e *CodeEditor) CaretRect() Rect {
	e.syncDocument()
	spec := e.effectiveFontSpec(e.fontSpec())
	gen := e.fontContext().layoutGeneration()
	if e.caretRectValid &&
		e.caretRectVersion == e.lastVersion &&
		e.caretRectCaret == e.Caret &&
		e.caretRectAnchor == e.Anchor &&
		e.caretRectComposing == e.composing &&
		e.caretRectPreedit == e.preedit &&
		e.caretRectPreeditCaret == e.preeditCaret &&
		e.caretRectScrollX == e.scrollX &&
		e.caretY == e.scrollY &&
		e.caretRectBounds == e.rect &&
		e.caretRectFontGen == gen &&
		e.caretRectSpec == spec {
		return e.cachedCaretRect
	}
	at := e.Caret
	if e.composing && e.preedit != "" {
		at = min(e.Caret, e.Anchor)
	}
	line, _ := e.Document.LineAtOffset(at)
	start, end, _ := e.Document.LineRange(line)
	text := e.sliceLine(line, start, end, true)
	layout := e.layoutLine(text)
	view := e.contentRect()
	x := view.X + layout.xAt(at-start) - e.scrollX
	if e.composing && e.preedit != "" {
		prefix := []rune(e.preedit)
		prefix = prefix[:min(max(0, e.preeditCaret), len(prefix))]
		x += e.textWidth(string(prefix), e.fontSpec())
	}
	h := e.rowHeight()
	res := Rect{X: x, Y: view.Y + float32(line)*h - e.scrollY, W: 2, H: h}
	e.caretRectValid = true
	e.caretRectVersion = e.lastVersion
	e.caretRectCaret = e.Caret
	e.caretRectAnchor = e.Anchor
	e.caretRectComposing = e.composing
	e.caretRectPreedit = e.preedit
	e.caretRectPreeditCaret = e.preeditCaret
	e.caretRectScrollX = e.scrollX
	e.caretY = e.scrollY
	e.caretRectBounds = e.rect
	e.caretRectFontGen = gen
	e.caretRectSpec = spec
	e.cachedCaretRect = res
	return res
}

func (e *CodeEditor) TextInputSnapshot(maxBytes int) (TextInputState, bool) {
	e.syncDocument()
	if e.ReadOnly {
		return TextInputState{}, false
	}
	if e.snapValid && e.snapVersion == e.lastVersion && e.snapCaret == e.Caret &&
		e.snapAnchor == e.Anchor && e.snapMax == maxBytes {
		return e.snapState, true
	}
	a, b := min(e.Caret, e.Anchor), max(e.Caret, e.Anchor)
	length := e.Document.Len()
	reqMax := maxBytes
	if maxBytes <= 0 {
		maxBytes = textInputWindowBytes
	}
	maxBytes = min(length, max(maxBytes, b-a))
	start := min(max(0, a-(maxBytes-(b-a))/2), length-maxBytes)
	end := start + maxBytes
	for start > 0 && !e.Document.ValidOffset(start) {
		start--
	}
	for end < length && !e.Document.ValidOffset(end) {
		end++
	}
	text, _ := e.Document.Slice(start, end)
	utf16Base, _ := e.Document.UTF16Offset(start)
	state := TextInputState{Text: text, Cursor: e.Caret - start, Anchor: e.Anchor - start,
		Base: start, UTF16Base: utf16Base, Multiline: true}
	e.snapValid = true
	e.snapVersion = e.lastVersion
	e.snapCaret = e.Caret
	e.snapAnchor = e.Anchor
	e.snapMax = reqMax
	e.snapState = state
	return state, true
}

func (e *CodeEditor) TextInputComposing() bool { return e.composing }
func (e *CodeEditor) ResetCaretBlink()         { e.caretVisible = true; e.InvalidatePaint() }
func (e *CodeEditor) BlinkCaret()              { e.caretVisible = !e.caretVisible; e.InvalidatePaint() }
