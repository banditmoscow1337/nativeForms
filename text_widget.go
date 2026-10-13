package nativeforms

import (
	"strings"
	"time"
	"unicode/utf8"
)

type TextField struct {
	State
	Text, Placeholder             string
	Font                          FontSpec
	Cursor                        int
	OnChange                      func(string)
	OnSubmit                      func(string)
	Multiline, ReadOnly, Password bool
	Validate                      func(string) bool
	editor                        *TextEditor
	scrollX, scrollY              float32
	lastClick                     time.Time
	lastPoint                     Vec2
	clicks                        int
	caretVisible                  bool
	composing                     bool
	layout                        textFieldLayout
}

func NewTextField(text string, onChange func(string)) *TextField {
	f := &TextField{Text: text, Cursor: len([]rune(text)), OnChange: onChange, caretVisible: true}
	f.editor = NewTextEditor(text)
	f.owner = f
	f.focusable = true
	f.minimum = Vec2{X: 180}
	return f
}

func NewTextArea(text string, onChange func(string)) *TextField {
	f := NewTextField(text, onChange)
	f.Multiline = true
	f.minimum.Y = 140
	return f
}

func (f *TextField) Editor() *TextEditor { f.syncExternal(); return f.editor }

func (f *TextField) CommitEditor() {
	if f.editor != nil && f.editor.Text != f.Text {
		f.changed()
	}
}
func (f *TextField) SetText(text string) {
	if text == f.Text {
		return
	}
	if f.editor == nil {
		f.editor = NewTextEditor(f.Text)
	}
	f.Text = text
	f.editor.SetText(text)
	f.composing = false
	f.Cursor = f.editor.Caret
	f.invalidateLayout()
	if f.OnChange != nil {
		f.OnChange(text)
	}
}
func (f *TextField) syncExternal() {
	if f.editor == nil {
		f.editor = NewTextEditor(f.Text)
	}
	if f.editor.Text != f.Text {
		f.editor.SetText(f.Text)
		f.composing = false
	}
	if f.Cursor != f.editor.Caret {
		f.editor.Move(f.Cursor, false)
	}
}

func (f *TextField) TextInputSnapshot(maxBytes int) (TextInputState, bool) {
	if f.ReadOnly {
		return TextInputState{}, false
	}
	e := f.Editor()
	state := TextInputState{Password: f.Password, Multiline: f.Multiline}
	if f.Password {
		return state, true
	}
	cursor := textByteOffset(e.Text, e.Caret)
	anchor := textByteOffset(e.Text, e.Anchor)
	state.Text, state.Base = surroundingWindow(e.Text, cursor, anchor, maxBytes)
	state.Cursor = cursor - state.Base
	state.Anchor = anchor - state.Base
	state.UTF16Base = utf16Units(e.Text[:state.Base])
	return state, true
}

func (f *TextField) ReplaceTextRange(start, end int, value string) bool {
	if f.ReadOnly || !utf8.ValidString(value) {
		return false
	}
	e := f.Editor()
	if start < 0 || end < start || end > len(e.Text) || !utf8.ValidString(e.Text[:start]) || !utf8.ValidString(e.Text[:end]) {
		return false
	}
	if !f.Multiline {
		value = strings.ReplaceAll(strings.ReplaceAll(value, "\r", ""), "\n", "")
	}
	next := e.Text[:start] + value + e.Text[end:]
	if f.Validate != nil && !f.Validate(next) {
		return false
	}
	e.Anchor = utf8.RuneCountInString(e.Text[:start])
	e.Caret = utf8.RuneCountInString(e.Text[:end])
	e.Preedit = ""
	if e.Replace(value) {
		f.changed()
		return true
	}
	f.invalidatePaint()
	return false
}

func (f *TextField) SelectTextRange(start, end int) bool {
	if f.ReadOnly {
		return false
	}
	e := f.Editor()
	if start < 0 || end < start || end > len(e.Text) || !utf8.ValidString(e.Text[:start]) || !utf8.ValidString(e.Text[:end]) {
		return false
	}
	e.Anchor = utf8.RuneCountInString(e.Text[:start])
	e.Caret = utf8.RuneCountInString(e.Text[:end])
	f.moved()
	return true
}

func (f *TextField) TextInputComposing() bool { return f.composing }

func (f *TextField) ResetCaretBlink() {
	f.caretVisible = true
	f.invalidatePaint()
}

func (f *TextField) changed() {
	f.Text = f.editor.Text
	f.Cursor = f.editor.Caret
	f.caretVisible = true
	if f.manager != nil && f.Focused() {
		f.manager.caretDue = f.manager.now().Add(500 * time.Millisecond)
	}
	if f.Multiline {
		f.invalidateLayout()
	} else {
		f.invalidatePaint()
	}
	if f.OnChange != nil {
		f.OnChange(f.Text)
	}
}
func (f *TextField) moved() {
	f.Cursor = f.editor.Caret
	f.caretVisible = true
	f.invalidatePaint()
	if f.manager != nil && f.Focused() {
		f.manager.caretDue = f.manager.now().Add(500 * time.Millisecond)
	}
}
func (f *TextField) insert(s string) bool {
	if f.ReadOnly {
		return false
	}
	if !f.Multiline {
		s = strings.ReplaceAll(strings.ReplaceAll(s, "\r", ""), "\n", "")
	}
	a, b := f.editor.selection()
	r := []rune(f.editor.Text)
	next := string(r[:a]) + s + string(r[b:])
	if f.Validate != nil && !f.Validate(next) {
		return false
	}
	if f.editor.Replace(s) {
		f.changed()
		return true
	}
	return false
}
func (f *TextField) delete(backward, word bool) bool {
	if f.ReadOnly {
		return false
	}
	a, b := f.editor.selection()
	r := []rune(f.editor.Text)
	if a == b {
		if word {
			if backward {
				a = wordBoundary(r, a, -1)
			} else {
				b = wordBoundary(r, b, 1)
			}
		} else if backward {
			a = previousGrapheme(r, a)
		} else {
			b = nextGrapheme(r, b)
		}
	}
	if f.Validate != nil && !f.Validate(string(r[:a])+string(r[b:])) {
		return false
	}
	if f.editor.Delete(backward, word) {
		f.changed()
		return true
	}
	return false
}

type textLine struct {
	start, end int
	width      float32
}

type textParagraph struct {
	start, end          int
	firstLine, lastLine int
}

// textFieldLayout owns the rune/line index for the last text and wrap width.
// It is reused by paint, hit-testing, scrolling and measure. FontContext's
// generation invalidates it if faces change without replacing the context.
type textFieldLayout struct {
	text       string
	runes      []rune
	lines      []textLine
	paragraphs []textParagraph
	width      float32
	spec       FontSpec
	fonts      *FontContext
	revision   uint64
	password   bool
	valid      bool
}

func (f *TextField) displayRunes() []rune {
	r := []rune(f.editor.Text)
	if f.Password {
		maskPasswordRunes(r, f.fontContext() != nil)
	}
	return r
}

func maskPasswordRunes(r []rune, preserveClusters bool) {
	if !preserveClusters {
		for i := range r {
			if hardTextBreak(r, i) == 0 {
				r[i] = '*'
			}
		}
		return
	}
	for i := 0; i < len(r); {
		if count := hardTextBreak(r, i); count != 0 {
			i += count
			continue
		}
		end := nextGrapheme(r, i)
		r[i] = '*'
		for j := i + 1; j < end; j++ {
			r[j] = '\u034f'
		}
		i = end
	}
}

func (f *TextField) fontSpec() FontSpec {
	spec := f.Font
	if spec.Size <= 0 {
		spec.Size = f.Theme().FontSize
	}
	return spec
}

func (f *TextField) lineHeight(spec FontSpec) float32 {
	if fonts := f.fontContext(); fonts != nil {
		return fonts.layoutShared("", spec).Height()
	}
	return spec.Size * 1.2
}

func (f *TextField) lines(r []rune, width float32, spec FontSpec) []textLine {
	var result []textLine
	for _, paragraph := range f.scanParagraphs(r) {
		result = f.appendParagraph(result, r, paragraph, width, spec)
	}
	return result
}

func (f *TextField) scanParagraphs(r []rune) []textParagraph {
	var paragraphs []textParagraph
	for start := 0; ; {
		end := start
		for end < len(r) && !(f.Multiline && hardTextBreak(r, end) != 0) {
			end++
		}
		paragraphs = append(paragraphs, textParagraph{start: start, end: end})
		if end == len(r) {
			break
		}
		start = end + hardTextBreak(r, end)
	}
	return paragraphs
}

func (f *TextField) appendParagraph(result []textLine, r []rune, paragraph textParagraph, width float32, spec FontSpec) []textLine {
	measure := func(s string, _ float32) float32 { return f.textWidth(s, spec) }
	for at := paragraph.start; at < paragraph.end; {
		limit := paragraph.end
		if f.Multiline {
			limit = wrappedLineEndWith(r, at, paragraph.end, width, spec.Size, measure)
		}
		result = append(result, textLine{at, limit, f.textWidth(string(r[at:limit]), spec)})
		at = limit
	}
	if paragraph.start == paragraph.end {
		result = append(result, textLine{paragraph.start, paragraph.end, 0})
	}
	return result
}

func (f *TextField) cachedLines(width float32, spec FontSpec) ([]rune, []textLine) {
	fonts := f.fontContext()
	revision := fonts.Revision()
	cache := &f.layout
	text := f.editor.Text
	if cache.valid && cache.text == text && cache.width == width && cache.spec == spec &&
		cache.fonts == fonts && cache.revision == revision && cache.password == f.Password {
		return cache.runes, cache.lines
	}
	r := []rune(text)
	if f.Password {
		maskPasswordRunes(r, fonts != nil)
	}
	paragraphs := f.scanParagraphs(r)
	canReuse := cache.valid && cache.width == width && cache.spec == spec && cache.fonts == fonts &&
		cache.revision == revision && cache.password == f.Password && !f.Password
	cache.lines = f.rewrapParagraphs(cache, r, paragraphs, width, spec, canReuse)
	cache.paragraphs = paragraphs
	cache.text, cache.runes, cache.width, cache.spec = text, r, width, spec
	cache.fonts, cache.revision, cache.password, cache.valid = fonts, revision, f.Password, true
	return r, cache.lines
}

func sameParagraph(a []rune, old textParagraph, b []rune, current textParagraph) bool {
	if old.end-old.start != current.end-current.start {
		return false
	}
	for i := 0; i < old.end-old.start; i++ {
		if a[old.start+i] != b[current.start+i] {
			return false
		}
	}
	return true
}

func (f *TextField) rewrapParagraphs(cache *textFieldLayout, r []rune, paragraphs []textParagraph, width float32, spec FontSpec, reuse bool) []textLine {
	prefix, suffix := 0, 0
	old := cache.paragraphs
	if reuse {
		for prefix < len(old) && prefix < len(paragraphs) &&
			sameParagraph(cache.runes, old[prefix], r, paragraphs[prefix]) {
			prefix++
		}
		for suffix < len(old)-prefix && suffix < len(paragraphs)-prefix &&
			sameParagraph(cache.runes, old[len(old)-1-suffix], r, paragraphs[len(paragraphs)-1-suffix]) {
			suffix++
		}
	}
	result := make([]textLine, 0, max(len(cache.lines), len(paragraphs)))
	for i := range paragraphs {
		paragraph := &paragraphs[i]
		paragraph.firstLine = len(result)
		oldIndex := -1
		if i < prefix {
			oldIndex = i
		} else if i >= len(paragraphs)-suffix {
			oldIndex = len(old) - (len(paragraphs) - i)
		}
		if oldIndex >= 0 {
			prior := old[oldIndex]
			shift := paragraph.start - prior.start
			for _, line := range cache.lines[prior.firstLine:prior.lastLine] {
				line.start += shift
				line.end += shift
				result = append(result, line)
			}
		} else {
			result = f.appendParagraph(result, r, *paragraph, width, spec)
		}
		paragraph.lastLine = len(result)
	}
	return result
}

func (f *TextField) Measure(c Constraints) Vec2 {
	f.syncExternal()
	spec := f.fontSpec()
	width := maxFloat32(1, c.Max.X-20)
	if c.Max.X <= 0 {
		width = 240
	}
	_, lines := f.cachedLines(width, spec)
	height := f.Theme().ControlHeight
	if f.Multiline {
		height = maxFloat32(height, float32(len(lines))*f.lineHeight(spec)+12)
	}
	return measureWithState(&f.State, Vec2{X: 240, Y: height}, c)
}

func (f *TextField) textGeometry() (Rect, []rune, []textLine, float32) {
	f.syncExternal()
	inner := f.rect.Inset(Symmetric(10, 4))
	spec := f.fontSpec()
	lineHeight := f.lineHeight(spec)
	if !f.Multiline {
		inner.Y = f.rect.Y + (f.rect.H-lineHeight)*0.5
		inner.H = lineHeight
	}
	width := float32(0)
	if f.Multiline {
		width = inner.W
	}
	r, lines := f.cachedLines(width, spec)
	return inner, r, lines, lineHeight
}

func (f *TextField) caretPoint(inner Rect, r []rune, lines []textLine, lineHeight float32) Vec2 {
	if len(lines) == 0 {
		return Vec2{X: inner.X, Y: inner.Y}
	}
	caret := f.editor.Caret
	if f.editor.Preedit != "" {
		a, _ := f.editor.selection()
		caret = a + max(0, f.editor.PreeditCaret)
	}
	lo, hi := 0, len(lines)-1
	for lo < hi {
		mid := lo + (hi-lo)/2
		if caret <= lines[mid].end {
			hi = mid
		} else {
			lo = mid + 1
		}
	}
	line := lines[lo]
	end := min(max(caret, line.start), line.end)
	return Vec2{X: inner.X + f.caretX(string(r[line.start:line.end]), f.fontSpec(), end-line.start) - f.scrollX,
		Y: inner.Y + float32(lo)*lineHeight - f.scrollY}
}

func (f *TextField) CaretRect() Rect {
	inner, r, lines, h := f.compositionGeometry()
	p := f.caretPoint(inner, r, lines, h)
	return Rect{X: p.X, Y: p.Y, W: 1, H: h}
}
func (f *TextField) Baseline() float32 {
	if fonts := f.fontContext(); fonts != nil {
		return fonts.layoutShared("", f.fontSpec()).Ascent + 4
	}
	return f.fontSpec().Size*0.85 + 4
}

func (f *TextField) revealCaret(inner Rect, p Vec2, lineHeight float32) {
	if p.X < inner.X+2 {
		f.scrollX = maxFloat32(0, f.scrollX-(inner.X+2-p.X))
	}
	if p.X > inner.X+inner.W-2 {
		f.scrollX += p.X - (inner.X + inner.W - 2)
	}
	if f.Multiline {
		if p.Y < inner.Y {
			f.scrollY = maxFloat32(0, f.scrollY-(inner.Y-p.Y))
		}
		if p.Y+lineHeight > inner.Y+inner.H {
			f.scrollY += p.Y + lineHeight - inner.Y - inner.H
		}
	}
}

func (f *TextField) Paint(c *Canvas) {
	inner, r, lines, lineHeight := f.compositionGeometry()
	background, border := c.theme.Control, c.theme.Border
	if f.Focused() {
		background, border = c.theme.ControlHover, c.theme.Accent
	}
	c.Rect(f.rect, background)
	c.Border(f.rect, 1, border)
	c.PushClip(f.rect.Inset(Symmetric(10, 4)))
	defer c.PopClip()
	point := f.caretPoint(inner, r, lines, lineHeight)
	if f.Focused() {
		f.revealCaret(inner, point, lineHeight)
		point = f.caretPoint(inner, r, lines, lineHeight)
	}
	if len(r) == 0 && f.Placeholder != "" {
		c.TextStyled(f.Placeholder, inner, f.fontSpec(), TextLeft, c.theme.MutedText)
	}
	a, b := f.editor.selection()
	composing := f.editor.Preedit != ""
	preeditStart := a
	preeditEnd := a + len([]rune(f.editor.Preedit))
	if composing {
		a = preeditStart + max(0, min(f.editor.PreeditCaret, f.editor.PreeditAnchor))
		b = preeditStart + max(0, max(f.editor.PreeditCaret, f.editor.PreeditAnchor))
	}
	startRow, endRow := 0, len(lines)
	if f.Multiline && lineHeight > 0 {
		startRow = max(0, int(f.scrollY/lineHeight)-1)
		endRow = min(len(lines), int((f.scrollY+inner.H)/lineHeight)+2)
	}
	for i := startRow; i < endRow; i++ {
		line := lines[i]
		y := inner.Y + float32(i)*lineHeight - f.scrollY
		if y+lineHeight < inner.Y || y > inner.Y+inner.H {
			continue
		}
		x := inner.X - f.scrollX
		from, to := max(a, line.start), min(b, line.end)
		if from < to {
			left := x + f.caretX(string(r[line.start:line.end]), f.fontSpec(), from-line.start)
			right := x + f.caretX(string(r[line.start:line.end]), f.fontSpec(), to-line.start)
			c.Rect(Rect{X: min(left, right), Y: y, W: absFloat32(right - left), H: lineHeight}, c.theme.Accent.WithAlpha(0.5))
		}
		if line.start < line.end {
			c.TextStyled(string(r[line.start:line.end]), Rect{X: x, Y: y, W: maxFloat32(inner.W+f.scrollX, line.width+2), H: lineHeight},
				f.fontSpec(), TextLeft, c.theme.Text)
		}
	}
	if f.Focused() && composing {
		for row := startRow; row < endRow; row++ {
			line := lines[row]
			from, to := max(preeditStart, line.start), min(preeditEnd, line.end)
			if from >= to {
				continue
			}
			text := string(r[line.start:line.end])
			left := inner.X - f.scrollX + f.caretX(text, f.fontSpec(), from-line.start)
			right := inner.X - f.scrollX + f.caretX(text, f.fontSpec(), to-line.start)
			y := inner.Y + float32(row)*lineHeight - f.scrollY + lineHeight - 2
			c.Line(Vec2{X: left, Y: y}, Vec2{X: right, Y: y}, 1, c.theme.Accent)
		}
	}

	if f.Focused() && f.caretVisible && (!composing || f.editor.PreeditCaret >= 0) {
		c.Line(Vec2{X: point.X, Y: point.Y + 2}, Vec2{X: point.X, Y: point.Y + lineHeight - 2}, 1, c.theme.AccentHover)
	}
}

func (f *TextField) positionAt(x, y float32) int {
	inner, r, lines, h := f.textGeometry()
	row := min(max(int((y-inner.Y+f.scrollY)/h), 0), len(lines)-1)
	line := lines[row]
	local := x - inner.X + f.scrollX
	text := string(r[line.start:line.end])
	positions := f.caretPositions(text, f.fontSpec())
	best, distance := line.start, float32(1e30)
	for at := line.start; ; at = nextGrapheme(r, at) {
		x := positions[at-line.start]
		if d := absFloat32(local - x); d < distance {
			best, distance = at, d
		}
		if at >= line.end {
			break
		}
	}
	return best
}

func (f *TextField) Handle(event Event) bool {
	f.syncExternal()
	e := f.editor
	switch event.Type {
	case FocusLost:
		e.Preedit = ""
		f.composing = false
		f.caretVisible = true
		if f.OnSubmit != nil {
			f.OnSubmit(f.Text)
		}
		return true
	case PointerDown:
		if event.Button != MouseLeft {
			return false
		}
		point := Vec2{X: event.X, Y: event.Y}
		if event.Clicks > 0 {
			f.clicks = event.Clicks
		} else if time.Since(f.lastClick) < 500*time.Millisecond && absFloat32(point.X-f.lastPoint.X) < 5 && absFloat32(point.Y-f.lastPoint.Y) < 5 {
			f.clicks = f.clicks%3 + 1
		} else {
			f.clicks = 1
		}
		f.lastClick = time.Now()
		f.lastPoint = point
		pos := f.positionAt(event.X, event.Y)
		r := []rune(e.Text)
		switch f.clicks {
		case 2:
			start, end := selectedWord(r, pos)
			e.Move(start, false)
			e.Move(end, true)
		case 3:
			start, end := lineBoundaries(r, pos)
			e.Move(start, false)
			e.Move(end, true)
		default:
			e.Move(pos, event.Mods&ModShift != 0)
		}
		f.moved()
		return true
	case PointerMove:
		if f.Pressed() {
			inner, _, lines, h := f.textGeometry()
			if f.Multiline {
				if event.Y < inner.Y {
					f.scrollY = maxFloat32(0, f.scrollY-(inner.Y-event.Y))
				}
				if event.Y > inner.Y+inner.H {
					f.scrollY = minFloat32(maxFloat32(0, float32(len(lines))*h-inner.H), f.scrollY+event.Y-inner.Y-inner.H)
				}
			}
			if event.X < inner.X {
				f.scrollX = maxFloat32(0, f.scrollX-(inner.X-event.X))
			}
			if event.X > inner.X+inner.W {
				f.scrollX += event.X - inner.X - inner.W
			}
			f.editor.Move(f.positionAt(event.X, event.Y), true)
			f.moved()
			return true
		}
	case PointerScroll:
		if f.Multiline && event.Scroll.Y != 0 {
			inner, _, lines, h := f.textGeometry()
			before := f.scrollY
			f.scrollY = clampFloat32(f.scrollY-event.Scroll.Y*40, 0, maxFloat32(0, float32(len(lines))*h-inner.H))
			if before != f.scrollY {
				f.invalidatePaint()
				return true
			}
		}
	case CompositionStart:
		e.Preedit = ""
		f.composing = !f.ReadOnly
		f.invalidatePaint()
		return true
	case CompositionUpdate:
		if f.ReadOnly {
			return true
		}
		f.composing = true
		e.Preedit = event.Text
		e.PreeditCaret = len([]rune(event.Text))
		e.PreeditAnchor = e.PreeditCaret
		if event.CompositionSelection {
			e.PreeditCaret = min(e.PreeditCaret, max(-1, event.CompositionCursor))
			e.PreeditAnchor = min(len([]rune(event.Text)), max(-1, event.CompositionAnchor))
		}
		f.invalidatePaint()
		return true
	case CompositionEnd:
		f.composing = false
		e.Preedit = ""
		f.invalidatePaint()
		if event.Text != "" {
			return f.insert(event.Text)
		}
		return true
	case TextInput:
		if event.Rune >= 32 && event.Rune != 127 {
			return f.insert(string(event.Rune))
		}
	case KeyDown:
		if event.Key == KeyEscape && f.composing {
			f.composing = false
			e.Preedit = ""
			f.invalidatePaint()
			return true
		}
		mod := event.Mods&(ModControl|ModSuper) != 0
		switch {
		case mod && event.Key == KeyA:
			e.SelectAll()
			f.moved()
			return true
		case mod && event.Key == KeyC:
			if !f.Password && f.manager != nil && f.manager.clipboardWrite != nil {
				_ = f.manager.clipboardWrite(e.Selection())
			}
			return true
		case mod && event.Key == KeyX:
			if !f.Password && !f.ReadOnly && e.Selection() != "" && f.manager != nil && f.manager.clipboardWrite != nil {
				if f.manager.clipboardWrite(e.Selection()) == nil {
					f.insert("")
				}
			}
			return true
		case mod && event.Key == KeyV:
			if !f.ReadOnly && f.manager != nil && f.manager.clipboardRead != nil {
				if value, err := f.manager.clipboardRead(); err == nil {
					f.insert(value)
				}
			}
			return true
		case mod && event.Key == KeyZ:
			if !f.ReadOnly {
				changed := false
				if event.Mods&ModShift != 0 {
					changed = e.Redo()
				} else {
					changed = e.Undo()
				}
				if changed {
					f.changed()
				}
			}
			return true
		case mod && event.Key == KeyY:
			if !f.ReadOnly && e.Redo() {
				f.changed()
			}
			return true
		}
		switch event.Key {
		case KeyLeftArrow:
			e.Step(-1, mod, event.Mods&ModShift != 0)
			f.moved()
			return true
		case KeyRightArrow:
			e.Step(1, mod, event.Mods&ModShift != 0)
			f.moved()
			return true
		case KeyHome, KeyEnd:
			r := []rune(e.Text)
			start, end := lineBoundaries(r, e.Caret)
			if mod {
				start = 0
				end = len(r)
			}
			if event.Key == KeyHome {
				e.Move(start, event.Mods&ModShift != 0)
			} else {
				e.Move(end, event.Mods&ModShift != 0)
			}
			f.moved()
			return true
		case KeyUpArrow, KeyDownArrow:
			if f.Multiline {
				caret := f.CaretRect()
				delta := f.Theme().FontSize * 1.2
				if event.Key == KeyUpArrow {
					delta = -delta
				}
				e.Move(f.positionAt(caret.X, caret.Y+delta), event.Mods&ModShift != 0)
				f.moved()
				return true
			}
		case KeyBackspace, KeyDelete:
			f.delete(event.Key == KeyBackspace, mod)
			return true
		case KeyEnter:
			if f.Multiline && !mod {
				f.insert("\n")
			} else if f.OnSubmit != nil {
				f.OnSubmit(f.Text)
			}
			return true
		case KeyW, KeyA, KeyS, KeyD, KeyQ, KeyE, KeyR, KeyF, KeyZ, KeyY, KeyC, KeyV, KeyX, KeySpace:
			if !mod {
				return true
			}
		}
	}
	return f.State.Handle(event)
}

func lineBoundaries(r []rune, pos int) (int, int) {
	pos = min(max(pos, 0), len(r))
	start := 0
	for at := 0; at < len(r); {
		if count := hardTextBreak(r, at); count > 0 {
			if pos < at+count {
				return start, at
			}
			at += count
			start = at
		} else {
			at++
		}
	}
	return start, len(r)
}
func selectedWord(r []rune, pos int) (int, int) {
	if len(r) == 0 {
		return 0, 0
	}
	pos = min(max(pos, 0), len(r))
	if pos == len(r) || pos > 0 && !wordRune(r[pos]) && wordRune(r[pos-1]) {
		pos--
	}
	kind := wordRune(r[pos])
	start, end := pos, pos+1
	for start > 0 && wordRune(r[start-1]) == kind && hardTextBreak(r, start-1) == 0 {
		start--
	}
	for end < len(r) && wordRune(r[end]) == kind && hardTextBreak(r, end) == 0 {
		end++
	}
	return nearestGrapheme(r, start), nextGrapheme(r, end-1)
}
func absFloat32(x float32) float32 {
	if x < 0 {
		return -x
	}
	return x
}

func (f *TextField) BlinkCaret() { f.caretVisible = !f.caretVisible; f.invalidatePaint() }

func (f *TextField) compositionGeometry() (Rect, []rune, []textLine, float32) {
	inner, r, lines, h := f.textGeometry()
	if f.editor.Preedit == "" {
		return inner, r, lines, h
	}
	a, b := f.editor.selection()
	a = min(max(a, 0), len(r))
	b = min(max(b, a), len(r))
	preedit := []rune(f.editor.Preedit)
	if f.Password {
		maskPasswordRunes(preedit, f.fontContext() != nil)
	}
	displayed := make([]rune, 0, len(r)-(b-a)+len(preedit))
	displayed = append(displayed, r[:a]...)
	displayed = append(displayed, preedit...)
	displayed = append(displayed, r[b:]...)
	width := float32(0)
	if f.Multiline {
		width = inner.W
	}
	return inner, displayed, f.lines(displayed, width, f.fontSpec()), h
}
