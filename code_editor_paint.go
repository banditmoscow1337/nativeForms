package nativeforms

import (
	"strconv"
	"unicode"
	"unicode/utf16"
	"unicode/utf8"
	"unsafe"
)

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

func diagnosticColor(severity int, theme Theme) Color {
	if severity == 2 {
		if theme.IsLight() {
			return RGBA(195, 118, 0, 255)
		}
		return RGBA(238, 178, 42, 255)
	}
	return theme.Danger
}

func diagnosticSpanByteOffsets(lineText string, col, endCol int) (int, int) {
	if len(lineText) == 0 {
		return 0, 0
	}
	// Convert UTF-16 column to byte offset in lineText
	toByte := func(targetCol int) int {
		if targetCol <= 0 {
			return 0
		}
		u16 := 0
		for idx, r := range lineText {
			if u16 >= targetCol {
				return idx
			}
			u16 += utf16.RuneLen(r)
		}
		return len(lineText)
	}

	firstNonWS := 0
	for firstNonWS < len(lineText) && (lineText[firstNonWS] == ' ' || lineText[firstNonWS] == '\t') {
		firstNonWS++
	}
	if firstNonWS >= len(lineText) {
		return 0, len(lineText)
	}

	bStart := toByte(col)
	if bStart < firstNonWS || bStart >= len(lineText) {
		bStart = firstNonWS
	}
	bEnd := toByte(endCol)
	if bEnd <= bStart {
		// Advance to the end of the identifier/token starting at bStart
		cur := bStart
		r, sz := utf8.DecodeRuneInString(lineText[cur:])
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' {
			cur += sz
			for cur < len(lineText) {
				nr, nsz := utf8.DecodeRuneInString(lineText[cur:])
				if !unicode.IsLetter(nr) && !unicode.IsDigit(nr) && nr != '_' {
					break
				}
				cur += nsz
			}
			bEnd = cur
		} else {
			bEnd = len(lineText)
		}
	}
	if bEnd <= bStart {
		bEnd = min(len(lineText), bStart+1)
	}
	return bStart, bEnd
}

func drawSquigglyUnderline(c *Canvas, x1, x2, y float32, color Color) {
	w := maxFloat32(12, x2-x1)
	step := float32(4)
	curX := x1
	up := true
	for curX < x1+w {
		nextX := minFloat32(x1+w, curX+step)
		yFrom := y
		yTo := y - 2.2
		if !up {
			yFrom = y - 2.2
			yTo = y
		}
		c.Line(Vec2{X: curX, Y: yFrom}, Vec2{X: nextX, Y: yTo}, 1.6, color)
		curX = nextX
		up = !up
	}
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
	caretLine, _, _ := e.lineAtCaret()

	// Measure max width across currently visible lines and clamp scrollX so it never
	// scrolls past the visible lines into empty space.
	maxW := float32(0)
	for line := first; line < last; line++ {
		start, end, _ := e.Document.LineRange(line)
		if end <= start {
			continue
		}
		text := e.sliceLine(line, start, end, true)
		layout, _ := e.cachedLayoutAndStyles(text, line, start, end)
		if len(layout.carets) > 0 {
			if lw := layout.carets[len(layout.carets)-1]; lw > maxW {
				maxW = lw
			}
		}
	}
	e.visibleMaxLineW = maxW
	maxX := float32(0)
	if maxW > view.W-8 {
		maxX = maxFloat32(0, maxW+16-view.W)
	}
	if e.scrollX > maxX {
		e.scrollX = maxX
		e.scrollTargetX = maxX
		e.caretRectValid = false
	}

	c.PushClip(view)
	for line := first; line < last; line++ {
		start, end, _ := e.Document.LineRange(line)
		text := e.sliceLine(line, start, end, true)
		layout, styles := e.cachedLayoutAndStyles(text, line, start, end)
		y := view.Y + float32(line)*h - e.scrollY

		// 1. Highlight active caret line so the editing line is immediately clear
		if line == caretLine && e.Caret == e.Anchor {
			lineBg := RGBA(255, 255, 255, 14)
			lineBorder := RGBA(255, 255, 255, 24)
			if c.theme.IsLight() {
				lineBg = RGBA(0, 0, 0, 11)
				lineBorder = RGBA(0, 0, 0, 20)
			}
			c.Rect(Rect{X: view.X, Y: y, W: view.W, H: h}, lineBg)
			c.Line(Vec2{X: view.X, Y: y}, Vec2{X: view.X + view.W, Y: y}, 1, lineBorder)
			c.Line(Vec2{X: view.X, Y: y + h}, Vec2{X: view.X + view.W, Y: y + h}, 1, lineBorder)
		}

		// 2. Highlight active debugger stopped line (debugLine)
		if e.debugLine >= 0 && line == e.debugLine {
			dbgBg := RGBA(238, 188, 38, 48)
			if c.theme.IsLight() {
				dbgBg = RGBA(245, 200, 50, 68)
			}
			c.Rect(Rect{X: view.X, Y: y, W: view.W, H: h}, dbgBg)
			c.Rect(Rect{X: view.X, Y: y, W: 3, H: h}, RGBA(245, 190, 30, 255))
		}

		// 3. Per-line error / warning background tint
		diag, hasDiag := e.diagByLine[line]
		if hasDiag {
			dCol := diagnosticColor(diag.Severity, *c.theme)
			tintAlpha := uint8(34)
			if diag.Severity == 2 {
				tintAlpha = 26
			}
			c.Rect(Rect{X: view.X, Y: y, W: view.W, H: h}, Color{R: dCol.R, G: dCol.G, B: dCol.B, A: float32(tintAlpha) / 255.0})
			c.Rect(Rect{X: view.X, Y: y, W: 3, H: h}, dCol)
		}

		e.paintCodeLineWithStyles(c, layout, styles, text, line, start, end, view.X-e.scrollX, y, h)

		// 4. Per-line squiggly underline and inline Error Lens message
		if hasDiag {
			dCol := diagnosticColor(diag.Severity, *c.theme)
			bStart, bEnd := diagnosticSpanByteOffsets(text, diag.Column, diag.EndColumn)
			uStartX := view.X - e.scrollX + layout.xAt(bStart)
			uEndX := view.X - e.scrollX + layout.xAt(bEnd)
			if uEndX <= uStartX+8 {
				uEndX = uStartX + 16
			}
			drawSquigglyUnderline(c, uStartX, uEndX, y+h-1.5, dCol)

			if diag.Message != "" && c.fonts != nil {
				lineEndX := view.X - e.scrollX
				if len(layout.carets) > 0 {
					lineEndX += layout.carets[len(layout.carets)-1]
				}
				lensX := maxFloat32(view.X+16, lineEndX+20)
				if lensX < view.X+view.W-40 {
					prefix := "⊗ "
					if diag.Severity == 2 {
						prefix = "⚠ "
					}
					msg := prefix + diag.Message
					if len(msg) > 96 {
						msg = msg[:96] + "…"
					}
					availW := maxFloat32(40, view.X+view.W-lensX-6)
					pillSpec := FontSpec{Family: e.fontSpec().Family, Size: maxFloat32(11, e.fontSpec().Size-2.5)}
					c.RoundedRect(Rect{X: lensX - 4, Y: y + 2, W: availW, H: maxFloat32(4, h-4)}, 4, Color{R: dCol.R, G: dCol.G, B: dCol.B, A: 0.14})
					c.TextStyled(msg, Rect{X: lensX + 2, Y: y, W: maxFloat32(1, availW-8), H: h}, pillSpec, TextLeft, dCol)
				}
			}
		}
	}

	// 5. High-visibility cursor (caret) rendering
	caret := e.CaretRect()
	caretPrimary := RGBA(248, 250, 255, 255)
	caretHalo := c.theme.Accent
	if c.theme.IsLight() {
		caretPrimary = RGBA(12, 16, 26, 255)
	}
	if e.Focused() {
		if e.caretVisible {
			// Subtle accent glow/outline + solid 2.5px high-contrast cursor bar + top/bottom I-beam serifs
			c.Rect(Rect{X: caret.X - 0.5, Y: caret.Y + 1, W: 3.5, H: maxFloat32(2, h-2)}, caretHalo.WithAlpha(0.55))
			c.Rect(Rect{X: caret.X, Y: caret.Y + 1, W: 2.5, H: maxFloat32(2, h-2)}, caretPrimary)
			c.Rect(Rect{X: caret.X - 1.5, Y: caret.Y + 1, W: 5.5, H: 1.5}, caretPrimary)
			c.Rect(Rect{X: caret.X - 1.5, Y: caret.Y + h - 2.5, W: 5.5, H: 1.5}, caretPrimary)
		}
	} else {
		// Unfocused editor still shows a clear caret marker so the user can always see the cursor position
		c.Rect(Rect{X: caret.X, Y: caret.Y + 1, W: 2, H: maxFloat32(2, h-2)}, caretPrimary.WithAlpha(0.62))
	}
	c.PopClip()

	// 6. Gutter rendering: breakpoints, debug arrow, error/warning badges, and line numbers
	spec := e.fontSpec()
	atlasOnly := c.fonts == nil && !hasTextShaper()
	for line := first; line < last; line++ {
		y := view.Y + float32(line)*h - e.scrollY
		diag, hasDiag := e.diagByLine[line]
		if e.debugLine >= 0 && line == e.debugLine {
			c.RoundedRect(Rect{X: e.rect.X + 5, Y: y + (h-12)/2, W: 12, H: 12}, 3, RGBA(245, 190, 30, 255))
			if !atlasOnly {
				c.TextStyled("▶", Rect{X: e.rect.X + 4, Y: y, W: 14, H: h}, FontSpec{Size: 10, Weight: 700}, TextCenter, RGBA(20, 20, 20, 255))
			}
		} else if e.breakpoints[line] {
			c.RoundedRect(Rect{X: e.rect.X + 7, Y: y + (h-10)/2, W: 10, H: 10}, 5, c.theme.Danger)
		} else if hasDiag {
			dCol := diagnosticColor(diag.Severity, *c.theme)
			c.RoundedRect(Rect{X: e.rect.X + 6, Y: y + (h-11)/2, W: 11, H: 11}, 5.5, dCol)
			if !atlasOnly {
				c.TextStyled("!", Rect{X: e.rect.X + 6, Y: y, W: 11, H: h}, FontSpec{Size: 10, Weight: 700}, TextCenter, RGBA(255, 255, 255, 255))
			}
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
		numColor := c.theme.MutedText
		if hasDiag {
			numColor = diagnosticColor(diag.Severity, *c.theme)
		} else if line == caretLine {
			numColor = c.theme.Text
		}
		c.TextStyled(label, Rect{X: e.rect.X + 19, Y: y, W: gutter - 25, H: h},
			spec, TextRight, numColor)
	}
	e.paintScrollbar(c, view, maxX)
}

func (e *CodeEditor) paintScrollbar(c *Canvas, view Rect, maxX float32) {
	track, thumb := e.scrollbarThumbRect()
	if track.Empty() {
		return
	}
	// Draw scrollbar track background & left separator line
	c.Rect(track, c.theme.PanelAlt)
	c.Line(Vec2{X: track.X, Y: track.Y}, Vec2{X: track.X, Y: track.Y + track.H}, 1, c.theme.Border)

	// Draw overview markers on the scrollbar track (breakpoints, diagnostics, debug line, and caret line)
	totalLines := max(1, e.Document.LineCount())
	availH := maxFloat32(1, track.H-4)
	for bpLine := range e.breakpoints {
		if bpLine >= 0 && bpLine < totalLines {
			my := track.Y + 2 + (float32(bpLine)/float32(totalLines))*availH
			c.Rect(Rect{X: track.X + 2, Y: my, W: track.W - 4, H: 3}, c.theme.Danger)
		}
	}
	for diagLine, diag := range e.diagByLine {
		if diagLine >= 0 && diagLine < totalLines {
			my := track.Y + 2 + (float32(diagLine)/float32(totalLines))*availH
			c.Rect(Rect{X: track.X + 1, Y: my, W: track.W - 2, H: 3}, diagnosticColor(diag.Severity, *c.theme))
		}
	}
	if e.debugLine >= 0 && e.debugLine < totalLines {
		dy := track.Y + 2 + (float32(e.debugLine)/float32(totalLines))*availH
		c.Rect(Rect{X: track.X + 1, Y: dy, W: track.W - 2, H: 3}, RGBA(245, 190, 30, 255))
	}
	caretLine, _, _ := e.lineAtCaret()
	if caretLine >= 0 && caretLine < totalLines {
		cy := track.Y + 2 + (float32(caretLine)/float32(totalLines))*availH
		c.Rect(Rect{X: track.X + 1, Y: cy, W: track.W - 2, H: 2}, c.theme.AccentHover)
	}

	// Draw proportional vertical thumb
	thumbColor := c.theme.Border
	if e.scrollDrag {
		thumbColor = c.theme.Accent
	} else if e.scrollHover {
		thumbColor = c.theme.ControlHover
	}
	c.RoundedRect(thumb, 4, thumbColor)

	// Draw subtle horizontal scrollbar indicator only when visible lines exceed viewport width
	if maxX > 0 && view.W > 20 {
		totalW := view.W + maxX
		hThumbW := clampFloat32(view.W*(view.W/totalW), 24, view.W)
		hRatio := clampFloat32(e.scrollX/maxX, 0, 1)
		hThumbX := view.X + hRatio*maxFloat32(0, view.W-hThumbW)
		c.RoundedRect(Rect{X: hThumbX, Y: e.rect.Y + e.rect.H - 6, W: hThumbW, H: 4}, 2, c.theme.Border)
	}
}

func (e *CodeEditor) paintCodeLine(c *Canvas, layout codeLineLayout, text string, line, start, end int, x, y, h float32) {
	styles := e.rowStyles(text, layout, line, start)
	e.paintCodeLineWithStyles(c, layout, styles, text, line, start, end, x, y, h)
}

func (e *CodeEditor) paintCodeLineWithStyles(c *Canvas, layout codeLineLayout, styles []CodeStyle, text string, line, start, end int, x, y, h float32) {
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
	spec := e.fontSpec()
	for from := 0; from < len(styles); {
		to := from + 1
		for to < len(styles) && styles[to] == styles[from] {
			to++
		}
		var fragment string
		if !layout.hasTabs {
			fragment = text[layout.offsets[from]:layout.offsets[to]]
		} else {
			fragment = string(layout.display[layout.columns[from]:layout.columns[to]])
		}
		if !isWhitespaceSpan(fragment) {
			color := codeStyleColor(styles[from], *c.theme)
			if e.Palette != nil {
				if custom, ok := e.Palette[styles[from]]; ok {
					color = custom
				}
			}
			left := layout.carets[layout.columns[from]]
			right := layout.carets[layout.columns[to]]
			c.TextStyled(fragment, Rect{X: x + left, Y: y, W: maxFloat32(1, right-left+2), H: h},
				spec, TextLeft, color)
		}
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
