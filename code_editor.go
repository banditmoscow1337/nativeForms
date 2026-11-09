package nativeforms

import (
	"sort"
	"strconv"
	"time"
	"unicode/utf8"
	"unsafe"

	"github.com/banditmoscow1337/nativeForms/editor"
)

// CodeEditor is a focusable, unwrapped source editor backed by editor.Document.
// It paints only visible lines. Byte offsets, breakpoint line numbers and
// callbacks are zero based; displayed line numbers start at one.
type CodeEditor struct {
	State
	Document           *editor.Document
	Font               FontSpec
	TabWidth           int
	ReadOnly           bool
	Caret, Anchor      int
	Palette            map[CodeStyle]Color
	OnChange           func(editor.Change)
	OnBreakpointChange func(line int, enabled bool)

	breakpoints      map[int]bool
	styles, semantic []StyledRange
	lexStates        []goLexState
	lastVersion      uint64
	scrollX, scrollY float32
	desiredX         float32
	retainX          bool
	composing        bool
	preedit          string
	preeditCaret     int
	caretVisible     bool
	lastClick        time.Time
	lastPoint        Vec2
	clicks           int
	completionPopup  PopupID

	scratchDisplay   []rune
	scratchOffsets   []int
	scratchColumns   []int
	scratchCarets    []float32
	scratchStyles    []CodeStyle
	scratchSpans     []StyledRange
	scratchLineBytes []byte
	numBuf           [20]byte
	crossPieceLines  map[int]string
	lineNumberCache  []string
	gutterDigits     int
	gutterSpec       FontSpec
	gutterFontGen    uint64
	gutterCachedW    float32
}

var _ Component = (*CodeEditor)(nil)
var _ TextInputClient = (*CodeEditor)(nil)

func NewCodeEditor(document *editor.Document) *CodeEditor {
	if document == nil {
		document, _ = editor.NewDocument("")
	}
	lineCount := document.LineCount()
	e := &CodeEditor{
		Document:         document,
		Font:             FontSpec{Family: "monospace", Size: 14},
		TabWidth:         4,
		lexStates:        make([]goLexState, lineCount+1, max(16, lineCount+1)),
		lastVersion:      document.Version(),
		caretVisible:     true,
		scratchDisplay:   make([]rune, 0, 256),
		scratchOffsets:   make([]int, 0, 256),
		scratchColumns:   make([]int, 0, 256),
		scratchCarets:    make([]float32, 0, 256),
		scratchStyles:    make([]CodeStyle, 0, 256),
		scratchSpans:     make([]StyledRange, 0, 32),
		scratchLineBytes: make([]byte, 0, 256),
	}
	e.owner = e
	e.focusable = true
	e.clip = true
	e.minimum = Vec2{X: 300, Y: 180}
	if document.MayContainMultilineTokens() {
		e.rebuildLexStates(0)
	}
	return e
}

func (e *CodeEditor) SetDocument(document *editor.Document) {
	e.HideCompletions()
	if document == nil {
		document, _ = editor.NewDocument("")
	}
	e.Document = document
	e.Caret, e.Anchor = 0, 0
	e.scrollX, e.scrollY = 0, 0
	e.styles, e.semantic = nil, nil
	clear(e.crossPieceLines)
	e.lexStates = append(e.lexStates[:0], goNormal)
	e.rebuildLexStates(0)
	e.lastVersion = document.Version()
	clear(e.breakpoints)
	e.composing, e.preedit = false, ""
	e.invalidateLayout()
}

func (e *CodeEditor) Text() string { e.syncDocument(); return e.Document.Text() }

func (e *CodeEditor) SetBreakpoint(line int, enabled bool) bool {
	e.syncDocument()
	if line < 0 || line >= e.Document.LineCount() || e.breakpoints[line] == enabled {
		return false
	}
	if e.breakpoints == nil {
		e.breakpoints = make(map[int]bool)
	}
	e.breakpoints[line] = enabled
	if !enabled {
		delete(e.breakpoints, line)
	}
	if e.OnBreakpointChange != nil {
		e.OnBreakpointChange(line, enabled)
	}
	e.invalidatePaint()
	return true
}

func (e *CodeEditor) Breakpoint(line int) bool { return e.breakpoints[line] }

func (e *CodeEditor) BreakpointLines() []int {
	lines := make([]int, 0, len(e.breakpoints))
	for line := range e.breakpoints {
		lines = append(lines, line)
	}
	sort.Ints(lines)
	return lines
}

func (e *CodeEditor) syncDocument() {
	if e.Document == nil {
		e.Document, _ = editor.NewDocument("")
	}
	if e.Document.Version() == e.lastVersion {
		return
	}
	e.lastVersion = e.Document.Version()
	clear(e.crossPieceLines)
	e.lexStates = append(e.lexStates[:0], goNormal)
	e.rebuildLexStates(0)
	e.styles, e.semantic = nil, nil
	e.Caret = e.validCaret(e.Caret)
	e.Anchor = e.validCaret(e.Anchor)
	e.scrollY = clampFloat32(e.scrollY, 0, e.maxScrollY())
	e.invalidatePaint()
}

func (e *CodeEditor) validCaret(at int) int {
	return e.Document.ValidCaretOffset(at)
}

func (e *CodeEditor) fontSpec() FontSpec {
	spec := e.Font
	if spec.Size <= 0 {
		spec.Size = 14
	}
	return spec
}

func (e *CodeEditor) rowHeight() float32 {
	if fonts := e.fontContext(); fonts != nil {
		return maxFloat32(1, fonts.layoutShared("", e.fontSpec()).Height())
	}
	return e.fontSpec().Size * 1.25
}

const nineDigits = "99999999999999999999"

func countDecimalDigits(n int) int {
	digits := 1
	for n >= 10 {
		n /= 10
		digits++
	}
	return min(digits, len(nineDigits))
}

func (e *CodeEditor) gutterWidth() float32 {
	digits := countDecimalDigits(max(1, e.Document.LineCount()))
	spec := e.fontSpec()
	gen := e.fontContext().layoutGeneration()
	if e.gutterDigits == digits && e.gutterSpec == spec && e.gutterFontGen == gen && e.gutterCachedW > 0 {
		return e.gutterCachedW
	}
	w := maxFloat32(48, e.textWidth(nineDigits[:digits], spec)+24)
	e.gutterDigits = digits
	e.gutterSpec = spec
	e.gutterFontGen = gen
	e.gutterCachedW = w
	return w
}

func (e *CodeEditor) contentRect() Rect {
	gutter := e.gutterWidth()
	return Rect{X: e.rect.X + gutter + 8, Y: e.rect.Y + 4,
		W: maxFloat32(0, e.rect.W-gutter-12), H: maxFloat32(0, e.rect.H-8)}
}

func (e *CodeEditor) maxScrollY() float32 {
	return maxFloat32(0, float32(e.Document.LineCount())*e.rowHeight()-e.contentRect().H)
}

func (e *CodeEditor) Measure(c Constraints) Vec2 { return e.State.Measure(c) }

func (e *CodeEditor) Arrange(r Rect) {
	e.rect = r
	e.syncDocument()
	e.scrollY = clampFloat32(e.scrollY, 0, e.maxScrollY())
}

// VisibleLines reports the half-open range rendered for the current viewport.
func (e *CodeEditor) VisibleLines() (first, last int) {
	e.syncDocument()
	view := e.contentRect()
	if view.H <= 0 {
		return 0, 0
	}
	h := e.rowHeight()
	first = min(e.Document.LineCount()-1, max(0, int(e.scrollY/h)))
	last = min(e.Document.LineCount(), int((e.scrollY+view.H)/h)+2)
	return first, last
}

func (e *CodeEditor) ScrollToLine(line int) {
	e.syncDocument()
	line = min(max(0, line), e.Document.LineCount()-1)
	e.scrollY = clampFloat32(float32(line)*e.rowHeight(), 0, e.maxScrollY())
	e.invalidatePaint()
}

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
	return Rect{X: x, Y: view.Y + float32(line)*e.rowHeight() - e.scrollY,
		W: 1, H: e.rowHeight()}
}

func (e *CodeEditor) TextInputSnapshot(maxBytes int) (TextInputState, bool) {
	e.syncDocument()
	if e.ReadOnly {
		return TextInputState{}, false
	}
	a, b := min(e.Caret, e.Anchor), max(e.Caret, e.Anchor)
	length := e.Document.Len()
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
	return TextInputState{Text: text, Cursor: e.Caret - start, Anchor: e.Anchor - start,
		Base: start, UTF16Base: utf16Base, Multiline: true}, true
}

func (e *CodeEditor) TextInputComposing() bool { return e.composing }
func (e *CodeEditor) ResetCaretBlink()         { e.caretVisible = true; e.invalidatePaint() }
func (e *CodeEditor) BlinkCaret()              { e.caretVisible = !e.caretVisible; e.invalidatePaint() }

func (e *CodeEditor) goStateAt(line int) goLexState {
	if len(e.lexStates) == 0 {
		e.lexStates = []goLexState{goNormal}
	}
	if len(e.lexStates) <= line {
		e.rebuildLexStates(len(e.lexStates) - 1)
	}
	return e.lexStates[line]
}

// Carry states are prepared once per version, outside the paint loop. A cold
// jump then only tokenizes the visible lines.
func (e *CodeEditor) rebuildLexStates(fromLine int) {
	if e.Document == nil {
		return
	}
	count := e.Document.LineCount()
	if fromLine < 0 || fromLine >= len(e.lexStates) {
		fromLine = 0
		if len(e.lexStates) == 0 {
			e.lexStates = make([]goLexState, count+1)
		} else {
			e.lexStates[0] = goNormal
		}
	}
	if cap(e.lexStates) < count+1 {
		grown := make([]goLexState, count+1, max(count+1, cap(e.lexStates)*2))
		copy(grown, e.lexStates[:fromLine+1])
		e.lexStates = grown
	} else {
		e.lexStates = e.lexStates[:count+1]
	}
	if fromLine >= count {
		return
	}
	state := e.lexStates[fromLine]
	if state == goNormal && !e.Document.MayContainMultilineTokens() {
		clear(e.lexStates[fromLine+1 : count+1])
		return
	}
	e.scratchLineBytes = e.Document.ScanLinesFrom(fromLine, e.scratchLineBytes[:0], func(line int, segment string) bool {
		state = advanceGoLexState(segment, state)
		e.lexStates[line+1] = state
		return true
	})
}

func (e *CodeEditor) updateLexStatesFrom(fromLine, removedLines, insertedLines int) {
	if e.Document == nil {
		return
	}
	count := e.Document.LineCount()
	if fromLine < 0 || len(e.lexStates) == 0 || fromLine >= len(e.lexStates) {
		e.rebuildLexStates(0)
		return
	}
	delta := insertedLines - removedLines
	if len(e.lexStates)+delta != count+1 {
		e.rebuildLexStates(fromLine)
		return
	}
	if delta > 0 {
		if cap(e.lexStates) < count+1 {
			grown := make([]goLexState, len(e.lexStates)+delta, max(count+1, cap(e.lexStates)*2))
			copy(grown, e.lexStates[:fromLine+1])
			copy(grown[fromLine+1+insertedLines:], e.lexStates[fromLine+1+removedLines:])
			e.lexStates = grown
		} else {
			oldLen := len(e.lexStates)
			e.lexStates = e.lexStates[:count+1]
			copy(e.lexStates[fromLine+1+insertedLines:], e.lexStates[fromLine+1+removedLines:oldLen])
		}
	} else if delta < 0 {
		copy(e.lexStates[fromLine+1+insertedLines:], e.lexStates[fromLine+1+removedLines:])
		e.lexStates = e.lexStates[:count+1]
	}
	state := e.lexStates[fromLine]
	minScanEnd := min(count, fromLine+max(1, insertedLines+1))
	if state == goNormal && !e.Document.MayContainMultilineTokens() {
		for line := fromLine; line < count; line++ {
			if line+1 >= minScanEnd && e.lexStates[line+1] == goNormal {
				return
			}
			e.lexStates[line+1] = goNormal
		}
		return
	}
	e.scratchLineBytes = e.Document.ScanLinesFrom(fromLine, e.scratchLineBytes[:0], func(line int, segment string) bool {
		state = advanceGoLexState(segment, state)
		if line+1 >= minScanEnd && e.lexStates[line+1] == state {
			return false
		}
		e.lexStates[line+1] = state
		return true
	})
}

func (e *CodeEditor) applyStyleRanges(dst []CodeStyle, layout codeLineLayout, lineStart int, ranges []StyledRange) {
	if len(dst) == 0 {
		return
	}
	lineEnd := lineStart + layout.offsets[len(layout.offsets)-1]
	index := sort.Search(len(ranges), func(i int) bool { return ranges[i].End > lineStart })
	for ; index < len(ranges) && ranges[index].Start < lineEnd; index++ {
		r := ranges[index]
		start := sort.SearchInts(layout.offsets, max(0, r.Start-lineStart))
		end := sort.SearchInts(layout.offsets, min(lineEnd-lineStart, r.End-lineStart))
		for i := start; i < end && i < len(dst); i++ {
			dst[i] = r.Style
		}
	}
}

func (e *CodeEditor) rowStyles(text string, layout codeLineLayout, line, start int) []CodeStyle {
	n := max(0, len(layout.offsets)-1)
	var styles []CodeStyle
	if cap(e.scratchStyles) < n {
		styles = make([]CodeStyle, n)
	} else {
		styles = e.scratchStyles[:n]
		clear(styles)
	}
	e.scratchStyles = styles
	local, _ := scanGoLineInto(text, e.goStateAt(line), true, e.scratchSpans[:0])
	e.scratchSpans = local
	e.applyStyleRanges(styles, layout, 0, local)
	e.applyStyleRanges(styles, layout, start, e.styles)
	e.applyStyleRanges(styles, layout, start, e.semantic)
	return styles
}

func (e *CodeEditor) lineNumberString(num int) string {
	if num < 100 {
		return strconv.Itoa(num)
	}
	for len(e.lineNumberCache) <= num {
		if cap(e.lineNumberCache) <= num {
			grown := make([]string, len(e.lineNumberCache), max(num+64, cap(e.lineNumberCache)*2))
			copy(grown, e.lineNumberCache)
			e.lineNumberCache = grown
		}
		e.lineNumberCache = e.lineNumberCache[:num+1]
	}
	if s := e.lineNumberCache[num]; s != "" {
		return s
	}
	s := strconv.Itoa(num)
	e.lineNumberCache[num] = s
	return s
}

func (e *CodeEditor) Paint(c *Canvas) {
	e.syncDocument()
	if e.rect.Empty() {
		return
	}
	c.Rect(e.rect, c.theme.Panel)
	c.PushClip(e.rect)
	defer c.PopClip()
	gutter := e.gutterWidth()
	c.Rect(Rect{X: e.rect.X, Y: e.rect.Y, W: gutter, H: e.rect.H}, c.theme.PanelAlt)
	view := e.contentRect()
	first, last := e.VisibleLines()
	h := e.rowHeight()
	c.PushClip(view)
	for line := first; line < last; line++ {
		start, end, _ := e.Document.LineRange(line)
		text := e.sliceLine(line, start, end, true)
		layout := e.layoutLine(text)
		y := view.Y + float32(line)*h - e.scrollY
		e.paintCodeLine(c, layout, text, line, start, end, view.X-e.scrollX, y, h)
	}
	if e.Focused() && e.caretVisible {
		caret := e.CaretRect()
		c.Line(Vec2{X: caret.X, Y: caret.Y + 2}, Vec2{X: caret.X, Y: caret.Y + h - 2}, 1, c.theme.AccentHover)
	}
	c.PopClip()
	spec := e.fontSpec()
	atlasOnly := c.fonts == nil && !hasTextShaper()
	for line := first; line < last; line++ {
		y := view.Y + float32(line)*h - e.scrollY
		if e.breakpoints[line] {
			c.RoundedRect(Rect{X: e.rect.X + 7, Y: y + (h-10)/2, W: 10, H: 10}, 5, c.theme.Danger)
		}
		var label string
		if atlasOnly {
			n := line + 1
			pos := len(e.numBuf)
			for n > 0 {
				pos--
				e.numBuf[pos] = byte('0' + n%10)
				n /= 10
			}
			label = unsafe.String(&e.numBuf[pos], len(e.numBuf)-pos)
		} else {
			label = e.lineNumberString(line + 1)
		}
		c.TextStyled(label, Rect{X: e.rect.X + 19, Y: y, W: gutter - 25, H: h},
			spec, TextRight, c.theme.MutedText)
	}
}

func (e *CodeEditor) paintCodeLine(c *Canvas, layout codeLineLayout, text string, line, start, end int, x, y, h float32) {
	a, b := min(e.Caret, e.Anchor), max(e.Caret, e.Anchor)
	from, to := max(start, a), min(end, b)
	if from < to || b > end && a <= end {
		left := x + layout.xAt(max(0, from-start))
		right := x + layout.xAt(max(0, to-start))
		if b > end {
			right += 6
		}
		c.Rect(Rect{X: left, Y: y, W: maxFloat32(0, right-left), H: h}, c.theme.Selection)
	}
	styles := e.rowStyles(text, layout, line, start)
	spec := e.fontSpec()
	for from := 0; from < len(styles); {
		to := from + 1
		for to < len(styles) && styles[to] == styles[from] {
			to++
		}
		color := codeStyleColor(styles[from], *c.theme)
		if e.Palette != nil {
			if custom, ok := e.Palette[styles[from]]; ok {
				color = custom
			}
		}
		left := layout.carets[layout.columns[from]]
		right := layout.carets[layout.columns[to]]
		var fragment string
		if !layout.hasTabs {
			fragment = text[layout.offsets[from]:layout.offsets[to]]
		} else {
			fragment = string(layout.display[layout.columns[from]:layout.columns[to]])
		}
		c.TextStyled(fragment, Rect{X: x + left, Y: y, W: maxFloat32(1, right-left+2), H: h},
			spec, TextLeft, color)
		from = to
	}
	preeditAt := min(e.Caret, e.Anchor)
	if e.composing && e.preedit != "" && preeditAt >= start && preeditAt <= end {
		left := x + layout.xAt(preeditAt-start)
		width := e.textWidth(e.preedit, e.fontSpec())
		c.Rect(Rect{X: left, Y: y, W: width + 2, H: h}, c.theme.Panel)
		c.TextStyled(e.preedit, Rect{X: left, Y: y, W: width + 2, H: h}, e.fontSpec(), TextLeft, c.theme.Text)
		c.Line(Vec2{X: left, Y: y + h - 2}, Vec2{X: left + width, Y: y + h - 2}, 1, c.theme.Accent)
	}
}
