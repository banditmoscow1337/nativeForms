package nativeforms

import (
	"sort"
	"unicode/utf8"
)

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
	cache := f.layoutFor(width, spec)
	return inner, cache.runes, cache.lines, lineHeight
}

func (f *TextField) lineStringAt(i int, r []rune, line textLine) string {
	if line.start >= line.end {
		return ""
	}
	if !f.Multiline && f.singleLineText != "" && f.editor.Preedit == "" && line.start == 0 && line.end == len(r) {
		return f.singleLineText
	}
	if f.layoutCache != nil {
		return f.layoutCache.lineText(i, r, line)
	}
	return string(r[line.start:line.end])
}

func (f *TextField) caretPoint(inner Rect, r []rune, lines []textLine, lineHeight float32) Vec2 {
	caret := f.editor.Caret
	if f.editor.Preedit != "" {
		a, _ := f.editor.selection()
		caret = a + max(0, f.editor.PreeditCaret)
	}
	if len(lines) > 0 {
		i := sort.Search(len(lines), func(i int) bool { return lines[i].end >= caret })
		i = min(i, len(lines)-1)
		line := lines[i]
		end := min(max(caret, line.start), line.end)
		spec := f.fontSpec()
		var cx float32
		if f.fontContext() == nil && !hasTextShaper() {
			cx = walkAtlasRunesCaretX(r[line.start:line.end], spec.Size, end-line.start)
		} else {
			cx = f.caretX(f.lineStringAt(i, r, line), spec, end-line.start)
		}
		return Vec2{X: inner.X + cx - f.scrollX,
			Y: inner.Y + float32(i)*lineHeight - f.scrollY}
	}
	return Vec2{X: inner.X, Y: inner.Y}
}

func (f *TextField) CaretRect() Rect {
	inner, r, lines, h := f.compositionGeometry()
	p := f.caretPoint(inner, r, lines, h)
	return Rect{X: p.X, Y: p.Y, W: 2, H: h}
}

func (f *TextField) Baseline() float32 {
	if fonts := f.fontContext(); fonts != nil {
		return fonts.layoutShared("", f.effectiveFontSpec(f.fontSpec())).Ascent + 4
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
	if !f.Frameless() {
		background, border := c.theme.Control, c.theme.Border
		if f.Focused() {
			background, border = c.theme.ControlHover, c.theme.Accent
		}
		radius := f.effectiveCornerRadius(c.theme)
		c.RoundedRect(f.rect, radius, background)
		c.RoundedBorder(f.rect, radius, 1, border)
	}
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
	preeditEnd := a + utf8.RuneCountInString(f.editor.Preedit)
	if composing {
		a = preeditStart + max(0, min(f.editor.PreeditCaret, f.editor.PreeditAnchor))
		b = preeditStart + max(0, max(f.editor.PreeditCaret, f.editor.PreeditAnchor))
	}
	spec := f.fontSpec()
	atlasOnly := c.fonts == nil && !hasTextShaper()
	first := max(0, int(f.scrollY/lineHeight)-1)
	last := min(len(lines), int((f.scrollY+inner.H)/lineHeight)+2)
	for i := first; i < last; i++ {
		line := lines[i]
		y := inner.Y + float32(i)*lineHeight - f.scrollY
		if y+lineHeight < inner.Y || y > inner.Y+inner.H {
			continue
		}
		x := inner.X - f.scrollX
		from, to := max(a, line.start), min(b, line.end)
		if atlasOnly {
			if from < to {
				left := x + walkAtlasRunesCaretX(r[line.start:line.end], spec.Size, from-line.start)
				right := x + walkAtlasRunesCaretX(r[line.start:line.end], spec.Size, to-line.start)
				c.Rect(Rect{X: min(left, right), Y: y, W: absFloat32(right - left), H: lineHeight}, c.theme.Accent.WithAlpha(0.5))
			}
			if line.start < line.end {
				c.textSingleLineRunes(r[line.start:line.end], Rect{X: x, Y: y, W: maxFloat32(inner.W+f.scrollX, line.width+2), H: lineHeight},
					spec.Size, c.theme.Text)
			}
		} else {
			text := f.lineStringAt(i, r, line)
			if from < to {
				positions := f.caretPositionsInto(text, spec, f.caretScratch[:0])
				f.caretScratch = positions
				left := x + positions[from-line.start]
				right := x + positions[to-line.start]
				c.Rect(Rect{X: min(left, right), Y: y, W: absFloat32(right - left), H: lineHeight}, c.theme.Accent.WithAlpha(0.5))
			}
			if line.start < line.end {
				c.TextStyled(text, Rect{X: x, Y: y, W: maxFloat32(inner.W+f.scrollX, line.width+2), H: lineHeight},
					spec, TextLeft, c.theme.Text)
			}
		}
	}
	if f.Focused() && composing {
		for row := first; row < last; row++ {
			line := lines[row]
			from, to := max(preeditStart, line.start), min(preeditEnd, line.end)
			if from >= to {
				continue
			}
			text := string(r[line.start:line.end])
			positions := f.caretPositions(text, f.fontSpec())
			left := inner.X - f.scrollX + positions[from-line.start]
			right := inner.X - f.scrollX + positions[to-line.start]
			y := inner.Y + float32(row)*lineHeight - f.scrollY + lineHeight - 2
			c.Line(Vec2{X: left, Y: y}, Vec2{X: right, Y: y}, 1, c.theme.Accent)
		}
	}
	if f.Focused() && f.caretVisible && (!composing || f.editor.PreeditCaret >= 0) {
		caretCol := RGBA(248, 250, 255, 255)
		if c.theme.IsLight() {
			caretCol = RGBA(12, 16, 26, 255)
		}
		c.Rect(Rect{X: point.X, Y: point.Y + 1, W: 2.2, H: maxFloat32(2, lineHeight-2)}, caretCol)
	}
}

func (f *TextField) compositionGeometry() (Rect, []rune, []textLine, float32) {
	inner, r, lines, h := f.textGeometry()
	if f.editor.Preedit == "" {
		return inner, r, lines, h
	}
	a, b := f.editor.selection()
	a = min(max(a, 0), len(r))
	b = min(max(b, a), len(r))
	if f.compositionCache != nil && f.compositionBase == f.layoutCache &&
		f.compositionText == f.editor.Preedit && f.compositionStart == a && f.compositionEnd == b {
		return inner, f.compositionCache.runes, f.compositionCache.lines, h
	}
	preedit := []rune(f.editor.Preedit)
	if f.Password {
		maskPasswordRunes(preedit, f.fontContext() != nil)
	}
	copyOfBase := *f.layoutCache
	copyOfBase.runes = append([]rune(nil), r...)
	copyOfBase.lines = append([]textLine(nil), lines...)
	copyOfBase.shared = false
	copyOfBase.replaceRange(f, a, b, preedit)
	f.compositionCache = &copyOfBase
	f.compositionBase = f.layoutCache
	f.compositionText = f.editor.Preedit
	f.compositionStart, f.compositionEnd = a, b
	return inner, copyOfBase.runes, copyOfBase.lines, h
}
