package nativeforms

import (
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/banditmoscow1337/nativeForms/editor"
)

// CodeEditor is a focusable, unwrapped source editor backed by editor.Document.
// It paints only visible lines. Byte offsets, breakpoint line numbers and
// callbacks are zero based; displayed line numbers start at one.
type CodeEditor struct {
	State
	Document *editor.Document
	Font FontSpec
	TabWidth int
	ReadOnly bool
	Caret, Anchor int
	Palette map[CodeStyle]Color
	OnChange func(editor.Change)
	OnBreakpointChange func(line int, enabled bool)

	breakpoints map[int]bool
	styles, semantic []StyledRange
	lexStates []goLexState
	lastVersion uint64
	scrollX, scrollY float32
	desiredX float32
	retainX bool
	composing bool
	preedit string
	preeditCaret int
	caretVisible bool
	lastClick time.Time
	lastPoint Vec2
	clicks int
	completionPopup PopupID
}

var _ Component = (*CodeEditor)(nil)
var _ TextInputClient = (*CodeEditor)(nil)

func NewCodeEditor(document *editor.Document) *CodeEditor {
	if document == nil {
		document, _ = editor.NewDocument("")
	}
	e := &CodeEditor{
		Document: document, Font: FontSpec{Family: "monospace", Size: 14},
		TabWidth: 4, breakpoints: make(map[int]bool), lexStates: []goLexState{goNormal},
		lastVersion: document.Version(), caretVisible: true,
	}
	e.owner = e
	e.focusable = true
	e.clip = true
	e.minimum = Vec2{X: 300, Y: 180}
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
	e.lexStates = []goLexState{goNormal}
	e.lastVersion = document.Version()
	e.breakpoints = make(map[int]bool)
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
	e.lexStates = []goLexState{goNormal}
	e.styles, e.semantic = nil, nil
	e.Caret = e.validCaret(e.Caret)
	e.Anchor = e.validCaret(e.Anchor)
	e.scrollY = clampFloat32(e.scrollY, 0, e.maxScrollY())
	e.invalidatePaint()
}

func (e *CodeEditor) validCaret(at int) int {
	at = min(max(0, at), e.Document.Len())
	for at > 0 {
		if _, err := e.Document.UTF16Position(at); err == nil {
			break
		}
		at--
	}
	return at
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
		return maxFloat32(1, fonts.Layout("", e.fontSpec()).Height())
	}
	return e.fontSpec().Size * 1.25
}

func (e *CodeEditor) gutterWidth() float32 {
	digits := len(strconv.Itoa(max(1, e.Document.LineCount())))
	return maxFloat32(48, e.textWidth(strings.Repeat("9", digits), e.fontSpec())+24)
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
	display []rune
	offsets, columns []int
	carets []float32
}

func (e *CodeEditor) layoutLine(line string) codeLineLayout {
	layout := codeLineLayout{offsets: []int{0}, columns: []int{0}}
	width := e.TabWidth
	if width <= 0 {
		width = 4
	}
	column := 0
	for at, r := range line {
		if r == '\t' {
			spaces := width - column%width
			for i := 0; i < spaces; i++ {
				layout.display = append(layout.display, ' ')
			}
			column += spaces
		} else {
			layout.display = append(layout.display, r)
			column++
		}
		layout.offsets = append(layout.offsets, at+utf8.RuneLen(r))
		layout.columns = append(layout.columns, len(layout.display))
	}
	layout.carets = e.caretPositions(string(layout.display), e.fontSpec())
	return layout
}

func (l codeLineLayout) xAt(byteOffset int) float32 {
	index := sort.SearchInts(l.offsets, byteOffset)
	if index == len(l.offsets) || l.offsets[index] != byteOffset {
		index = max(0, index-1)
	}
	return l.carets[l.columns[index]]
}

func (l codeLineLayout) byteAtX(line string, x float32) int {
	runes := []rune(line)
	best, distance := 0, float32(1e30)
	for i, offset := range l.offsets {
		if !graphemeBreak(runes, i) {
			continue
		}
		if d := absFloat32(x-l.carets[l.columns[i]]); d < distance {
			best, distance = offset, d
		}
	}
	return best
}

func (e *CodeEditor) lineAtCaret() (line, start, end int) {
	pos, err := e.Document.UTF16Position(e.Caret)
	if err != nil {
		return 0, 0, 0
	}
	start, end, _ = e.Document.LineRange(pos.Line)
	return pos.Line, start, end
}

func (e *CodeEditor) CaretRect() Rect {
	e.syncDocument()
	at := e.Caret
	if e.composing && e.preedit != "" {
		at = min(e.Caret, e.Anchor)
	}
	pos, _ := e.Document.UTF16Position(at)
	line := pos.Line
	start, end, _ := e.Document.LineRange(line)
	text, _ := e.Document.Slice(start, end)
	layout := e.layoutLine(text)
	view := e.contentRect()
	x := view.X + layout.xAt(at-start) - e.scrollX
	if e.composing && e.preedit != "" {
		prefix := []rune(e.preedit)
		prefix = prefix[:min(max(0, e.preeditCaret), len(prefix))]
		x += e.textWidth(string(prefix), e.fontSpec())
	}
	return Rect{X: x, Y: view.Y + float32(line)*e.rowHeight()-e.scrollY,
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
	end := start+maxBytes
	for start > 0 {
		if _, err := e.Document.Slice(start, start); err == nil {
			break
		}
		start--
	}
	for end < length {
		if _, err := e.Document.Slice(end, end); err == nil {
			break
		}
		end++
	}
	text, _ := e.Document.Slice(start, end)
	utf16Base, _ := e.Document.UTF16Offset(start)
	return TextInputState{Text: text, Cursor: e.Caret-start, Anchor: e.Anchor-start,
		Base: start, UTF16Base: utf16Base, Multiline: true}, true
}

func (e *CodeEditor) TextInputComposing() bool { return e.composing }
func (e *CodeEditor) ResetCaretBlink() { e.caretVisible = true; e.invalidatePaint() }
func (e *CodeEditor) BlinkCaret() { e.caretVisible = !e.caretVisible; e.invalidatePaint() }

func (e *CodeEditor) goStateAt(line int) goLexState {
	if len(e.lexStates) == 0 {
		e.lexStates = []goLexState{goNormal}
	}
	for len(e.lexStates) <= line {
		previous := len(e.lexStates)-1
		start, end, _ := e.Document.LineRange(previous)
		text, _ := e.Document.Slice(start, end)
		_, state := lexGoLine(text, e.lexStates[previous])
		e.lexStates = append(e.lexStates, state)
	}
	return e.lexStates[line]
}

func (e *CodeEditor) applyStyleRanges(dst []CodeStyle, layout codeLineLayout, lineStart int, ranges []StyledRange) {
	if len(dst) == 0 {
		return
	}
	lineEnd := lineStart+layout.offsets[len(layout.offsets)-1]
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
	styles := make([]CodeStyle, len(layout.offsets)-1)
	local, _ := lexGoLine(text, e.goStateAt(line))
	e.applyStyleRanges(styles, layout, 0, local)
	e.applyStyleRanges(styles, layout, start, e.styles)
	e.applyStyleRanges(styles, layout, start, e.semantic)
	return styles
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
		text, _ := e.Document.Slice(start, end)
		layout := e.layoutLine(text)
		y := view.Y+float32(line)*h-e.scrollY
		e.paintCodeLine(c, layout, text, line, start, end, view.X-e.scrollX, y, h)
	}
	if e.Focused() && e.caretVisible {
		caret := e.CaretRect()
		c.Line(Vec2{X: caret.X, Y: caret.Y+2}, Vec2{X: caret.X, Y: caret.Y+h-2}, 1, c.theme.AccentHover)
	}
	c.PopClip()
	for line := first; line < last; line++ {
		y := view.Y+float32(line)*h-e.scrollY
		if e.breakpoints[line] {
			c.RoundedRect(Rect{X: e.rect.X+7, Y: y+(h-10)/2, W: 10, H: 10}, 5, c.theme.Danger)
		}
		c.TextStyled(strconv.Itoa(line+1), Rect{X: e.rect.X+19, Y: y, W: gutter-25, H: h},
			e.fontSpec(), TextRight, c.theme.MutedText)
	}
}

func (e *CodeEditor) paintCodeLine(c *Canvas, layout codeLineLayout, text string, line, start, end int, x, y, h float32) {
	a, b := min(e.Caret, e.Anchor), max(e.Caret, e.Anchor)
	from, to := max(start, a), min(end, b)
	if from < to || b > end && a <= end {
		left := x+layout.xAt(max(0, from-start))
		right := x+layout.xAt(max(0, to-start))
		if b > end {
			right += 6
		}
		c.Rect(Rect{X: left, Y: y, W: maxFloat32(0, right-left), H: h}, c.theme.Selection)
	}
	styles := e.rowStyles(text, layout, line, start)
	for from := 0; from < len(styles); {
		to := from+1
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
		fragment := string(layout.display[layout.columns[from]:layout.columns[to]])
		c.TextStyled(fragment, Rect{X: x+left, Y: y, W: maxFloat32(1, right-left+2), H: h},
			e.fontSpec(), TextLeft, color)
		from = to
	}
	preeditAt := min(e.Caret, e.Anchor)
	if e.composing && e.preedit != "" && preeditAt >= start && preeditAt <= end {
		left := x+layout.xAt(preeditAt-start)
		width := e.textWidth(e.preedit, e.fontSpec())
		c.Rect(Rect{X: left, Y: y, W: width+2, H: h}, c.theme.Panel)
		c.TextStyled(e.preedit, Rect{X: left, Y: y, W: width+2, H: h}, e.fontSpec(), TextLeft, c.theme.Text)
		c.Line(Vec2{X: left, Y: y+h-2}, Vec2{X: left+width, Y: y+h-2}, 1, c.theme.Accent)
	}
}
