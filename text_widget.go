package nativeforms

import (
	"strings"
	"time"
)

// TextField and NewTextArea share the same editor, selection and undo model.
// Text and Cursor remain exported for source compatibility; prefer SetText and
// Editor methods for mutations. ReadOnly permits selecting and copying.
type TextField struct {
	State
	Text, Placeholder             string
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

// CommitEditor publishes changes made through Editor to the widget and layout.
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
	}
	if f.Cursor != f.editor.Caret {
		f.editor.Move(f.Cursor, false)
	}
}
func (f *TextField) changed() {
	f.Text = f.editor.Text
	f.Cursor = f.editor.Caret
	f.caretVisible = true
	if f.manager != nil && f.Focused() {
		f.manager.caretDue = time.Now().Add(500 * time.Millisecond)
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
		f.manager.caretDue = time.Now().Add(500 * time.Millisecond)
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

func (f *TextField) displayRunes() []rune {
	r := []rune(f.editor.Text)
	if f.Password {
		for i := range r {
			if r[i] != '\n' {
				r[i] = '*'
			}
		}
	}
	return r
}

func (f *TextField) lines(r []rune, width, fontSize float32) []textLine {
	var lines []textLine
	start, at := 0, 0
	var x float32
	for at < len(r) {
		if r[at] == '\n' && f.Multiline {
			lines = append(lines, textLine{start, at, x})
			at++
			start = at
			x = 0
			continue
		}
		end := nextGrapheme(r, at)
		advance := measureTextLine(string(r[at:end]), fontSize)
		if f.Multiline && width > 0 && x+advance > width && at > start {
			lines = append(lines, textLine{start, at, x})
			start = at
			x = 0
		}
		x += advance
		at = end
	}
	lines = append(lines, textLine{start, len(r), x})
	return lines
}

func (f *TextField) Measure(c Constraints) Vec2 {
	f.syncExternal()
	fontSize := f.Theme().FontSize
	width := maxFloat32(1, c.Max.X-20)
	if c.Max.X <= 0 {
		width = 240
	}
	lines := f.lines(f.displayRunes(), width, fontSize)
	height := f.Theme().ControlHeight
	if f.Multiline {
		height = maxFloat32(height, float32(len(lines))*fontSize*1.2+12)
	}
	return measureWithState(&f.State, Vec2{X: 240, Y: height}, c)
}

func (f *TextField) textGeometry() (Rect, []rune, []textLine, float32) {
	f.syncExternal()
	inner := f.rect.Inset(Symmetric(10, 4))
	fontSize := f.Theme().FontSize
	lineHeight := fontSize * 1.2
	if !f.Multiline {
		inner.Y = f.rect.Y + (f.rect.H-lineHeight)*0.5
		inner.H = lineHeight
	}
	r := f.displayRunes()
	width := float32(0)
	if f.Multiline {
		width = inner.W
	}
	return inner, r, f.lines(r, width, fontSize), lineHeight
}

func (f *TextField) caretPoint(inner Rect, r []rune, lines []textLine, lineHeight float32) Vec2 {
	caret := f.editor.Caret
	for i, line := range lines {
		if caret <= line.end || i == len(lines)-1 {
			end := min(max(caret, line.start), line.end)
			return Vec2{X: inner.X + measureTextLine(string(r[line.start:end]), f.Theme().FontSize) - f.scrollX,
				Y: inner.Y + float32(i)*lineHeight - f.scrollY}
		}
	}
	return Vec2{X: inner.X, Y: inner.Y}
}

// CaretRect is in logical window coordinates for IME candidate placement.
func (f *TextField) CaretRect() Rect {
	inner, r, lines, h := f.textGeometry()
	p := f.caretPoint(inner, r, lines, h)
	return Rect{X: p.X, Y: p.Y, W: 1, H: h}
}
func (f *TextField) Baseline() float32 { return f.Theme().FontSize*0.85 + 4 }

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
	inner, r, lines, lineHeight := f.textGeometry()
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
		c.Text(f.Placeholder, inner, c.theme.FontSize, TextLeft, c.theme.MutedText)
	}
	a, b := f.editor.selection()
	for i, line := range lines {
		y := inner.Y + float32(i)*lineHeight - f.scrollY
		if y+lineHeight < inner.Y || y > inner.Y+inner.H {
			continue
		}
		x := inner.X - f.scrollX
		from, to := max(a, line.start), min(b, line.end)
		if from < to {
			left := x + measureTextLine(string(r[line.start:from]), c.theme.FontSize)
			right := x + measureTextLine(string(r[line.start:to]), c.theme.FontSize)
			c.Rect(Rect{X: left, Y: y, W: right - left, H: lineHeight}, c.theme.Accent.WithAlpha(0.5))
		}
		if line.start < line.end {
			c.Text(string(r[line.start:line.end]), Rect{X: x, Y: y, W: maxFloat32(inner.W+f.scrollX, line.width+2), H: lineHeight},
				c.theme.FontSize, TextLeft, c.theme.Text)
		}
	}
	if f.Focused() && f.editor.Preedit != "" {
		c.Text(f.editor.Preedit, Rect{X: point.X, Y: point.Y, W: maxFloat32(inner.W, measureTextLine(f.editor.Preedit, c.theme.FontSize)), H: lineHeight},
			c.theme.FontSize, TextLeft, c.theme.AccentHover)
		c.Line(Vec2{X: point.X, Y: point.Y + lineHeight - 2}, Vec2{X: point.X + measureTextLine(f.editor.Preedit, c.theme.FontSize), Y: point.Y + lineHeight - 2}, 1, c.theme.Accent)
	}
	if f.Focused() && f.caretVisible {
		c.Line(Vec2{X: point.X, Y: point.Y + 2}, Vec2{X: point.X, Y: point.Y + lineHeight - 2}, 1, c.theme.AccentHover)
	}
}

func (f *TextField) positionAt(x, y float32) int {
	inner, r, lines, h := f.textGeometry()
	row := min(max(int((y-inner.Y+f.scrollY)/h), 0), len(lines)-1)
	line := lines[row]
	local := x - inner.X + f.scrollX
	advance := float32(0)
	for i := line.start; i < line.end; {
		end := nextGrapheme(r, i)
		part := measureTextLine(string(r[i:end]), f.Theme().FontSize)
		if local < advance+part*0.5 {
			return i
		}
		advance += part
		i = end
	}
	return line.end
}

func (f *TextField) Handle(event Event) bool {
	f.syncExternal()
	e := f.editor
	switch event.Type {
	case FocusLost:
		e.Preedit = ""
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
		f.invalidatePaint()
		return true
	case CompositionUpdate:
		e.Preedit = event.Text
		f.invalidatePaint()
		return true
	case CompositionEnd:
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
	start, end := pos, pos
	for start > 0 && r[start-1] != '\n' {
		start--
	}
	for end < len(r) && r[end] != '\n' {
		end++
	}
	return start, end
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
	for start > 0 && wordRune(r[start-1]) == kind && r[start-1] != '\n' {
		start--
	}
	for end < len(r) && wordRune(r[end]) == kind && r[end] != '\n' {
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

// BlinkCaret is driven by the host's next-frame timer.
func (f *TextField) BlinkCaret() { f.caretVisible = !f.caretVisible; f.invalidatePaint() }
