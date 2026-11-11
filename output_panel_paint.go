package nativeforms

import (
	"math"
	"strings"
	"unicode/utf8"
)

func (p *OutputPanel) contentRect() Rect {
	return Rect{
		X: p.rect.X + 8,
		Y: p.rect.Y + 4,
		W: maxFloat32(0, p.rect.W-8-outputPanelScrollbarWidth-4),
		H: maxFloat32(0, p.rect.H-8),
	}
}

func (p *OutputPanel) scrollbarTrackRect() Rect {
	if p.rect.W <= outputPanelScrollbarWidth || p.rect.H <= 0 {
		return Rect{}
	}
	return Rect{
		X: p.rect.X + p.rect.W - outputPanelScrollbarWidth,
		Y: p.rect.Y,
		W: outputPanelScrollbarWidth,
		H: p.rect.H,
	}
}

func (p *OutputPanel) maxScrollY() float32 {
	view := p.contentRect()
	totalH := float32(p.displayLineCount()) * p.visualLineHeight()
	return maxFloat32(0, totalH-view.H)
}

func (p *OutputPanel) visibleRange() (first, last int) {
	total := p.displayLineCount()
	if total == 0 || p.rect.Empty() {
		return 0, 0
	}
	view := p.contentRect()
	h := p.visualLineHeight()
	first = min(total-1, max(0, int(p.scrollY/h)))
	last = min(total, int((p.scrollY+view.H)/h)+2)
	return first, last
}

func (p *OutputPanel) maxScrollX() float32 {
	view := p.contentRect()
	if view.W <= 0 {
		return 0
	}
	first, last := p.visibleRange()
	spec := p.fontSpec()
	maxW := float32(0)
	for i := first; i < last; i++ {
		w := p.textWidth(p.displayLine(i), spec)
		if w > maxW {
			maxW = w
		}
	}
	if maxW <= view.W-4 {
		return 0
	}
	return maxFloat32(0, maxW+16-view.W)
}

func (p *OutputPanel) scrollbarThumbRect() (track, thumb Rect) {
	track = p.scrollbarTrackRect()
	if track.Empty() {
		return Rect{}, Rect{}
	}
	total := max(1, p.displayLineCount())
	h := p.visualLineHeight()
	totalH := maxFloat32(track.H, float32(total)*h)
	viewH := maxFloat32(1, p.rect.H-8)
	availH := maxFloat32(1, track.H-4)
	thumbH := clampFloat32(availH*(viewH/totalH), minFloat32(20, availH), availH)
	maxY := p.maxScrollY()
	ratio := float32(0)
	if maxY > 0 {
		ratio = clampFloat32(p.scrollY/maxY, 0, 1)
	}
	thumbY := track.Y + 2 + ratio*maxFloat32(0, availH-thumbH)
	thumb = Rect{
		X: track.X + 2,
		Y: thumbY,
		W: maxFloat32(4, track.W-4),
		H: thumbH,
	}
	return track, thumb
}

func (p *OutputPanel) Measure(c Constraints) Vec2 {
	return measureWithState(&p.State, p.List.Measure(c), c)
}

func (p *OutputPanel) Arrange(r Rect) {
	p.rect = r
	p.List.Arrange(r)
	p.scrollY = clampFloat32(p.scrollY, 0, p.maxScrollY())
	p.scrollX = clampFloat32(p.scrollX, 0, p.maxScrollX())
	if !p.smoothScrolling {
		p.targetScrollY = p.scrollY
	} else {
		p.targetScrollY = clampFloat32(p.targetScrollY, 0, p.maxScrollY())
	}
}

func (p *OutputPanel) stepSmoothScroll(dt float64) bool {
	if !p.smoothScrolling {
		return false
	}
	maxY := p.maxScrollY()
	p.targetScrollY = clampFloat32(p.targetScrollY, 0, maxY)
	dy := p.targetScrollY - p.scrollY
	if absFloat32(dy) <= 0.35 {
		p.scrollY = p.targetScrollY
		p.smoothScrolling = false
		p.scrollX = clampFloat32(p.scrollX, 0, p.maxScrollX())
		p.InvalidatePaint()
		return false
	}
	if dt <= 0 || dt > 0.1 {
		dt = 1.0 / 120.0
	}
	alpha := clampFloat32(float32(1.0-math.Exp(-dt*28.0)), 0.18, 0.85)
	p.scrollY = clampFloat32(p.scrollY+dy*alpha, 0, maxY)
	p.scrollX = clampFloat32(p.scrollX, 0, p.maxScrollX())
	p.InvalidatePaint()
	return true
}

func (p *OutputPanel) colX(lineText string, col int, spec FontSpec) float32 {
	if col <= 0 || lineText == "" {
		return 0
	}
	r := []rune(lineText)
	if col >= len(r) {
		return p.textWidth(lineText, spec)
	}
	return p.textWidth(string(r[:col]), spec)
}

func (p *OutputPanel) colAtX(lineText string, localX float32, spec FontSpec) int {
	if localX <= 0 || lineText == "" {
		return 0
	}
	positions := p.caretPositions(lineText, spec)
	if len(positions) == 0 {
		return 0
	}
	bestCol := 0
	bestDist := float32(1e30)
	for i, pos := range positions {
		d := absFloat32(localX - pos)
		if d < bestDist {
			bestDist = d
			bestCol = i
		}
	}
	return bestCol
}

func (p *OutputPanel) posAt(x, y float32) OutputTextPos {
	total := p.displayLineCount()
	if total == 0 {
		return OutputTextPos{}
	}
	view := p.contentRect()
	h := p.visualLineHeight()
	line := int((y - view.Y + p.scrollY) / h)
	line = min(max(0, line), total-1)
	lineText := p.displayLine(line)
	col := p.colAtX(lineText, x-view.X+p.scrollX, p.fontSpec())
	return OutputTextPos{Line: line, Col: col}
}

// Paint renders the continuous sequential text stream, multi-line character selection,
// optional terminal prompt cursor, and scrollbar.
func (p *OutputPanel) Paint(c *Canvas) {
	if p.rect.Empty() {
		return
	}
	radius := p.effectiveCornerRadius(c.theme)
	c.RoundedRect(p.rect, radius, c.theme.Panel)

	view := p.contentRect()
	if view.Empty() {
		return
	}
	spec := p.fontSpec()
	h := p.visualLineHeight()
	first, last := p.visibleRange()
	hasSel := p.HasSelection()
	selStart, selEnd := p.normalizedSelection()
	promptIdx := p.promptLineIndex()

	c.PushClip(view)
	for i := first; i < last; i++ {
		lineText := p.displayLine(i)
		y := view.Y + float32(i)*h - p.scrollY
		x := view.X - p.scrollX

		if hasSel && i >= selStart.Line && i <= selEnd.Line {
			runeCount := utf8.RuneCountInString(lineText)
			fromCol := 0
			toCol := runeCount
			if i == selStart.Line {
				fromCol = min(max(0, selStart.Col), runeCount)
			}
			if i == selEnd.Line {
				toCol = min(max(fromCol, selEnd.Col), runeCount)
			}
			x0 := x + p.colX(lineText, fromCol, spec)
			x1 := x + p.colX(lineText, toCol, spec)
			if i < selEnd.Line {
				x1 += 6
			}
			if x1 > x0 {
				c.Rect(Rect{X: x0, Y: y, W: x1 - x0, H: h}, c.theme.Selection)
			}
		}

		if lineText != "" {
			lineColor := c.theme.Text
			if p.TerminalMode && i == promptIdx {
				prefix := p.promptPrefixForLastLine()
				prefixW := p.textWidth(prefix, spec)
				if prefix != "" {
					c.TextStyled(prefix, Rect{X: x, Y: y, W: prefixW + 4, H: h}, spec, TextLeft, c.theme.AccentHover)
				}
				cmdStr := p.InputText + p.preedit
				if cmdStr != "" {
					cmdW := p.textWidth(cmdStr, spec)
					c.TextStyled(cmdStr, Rect{X: x + prefixW, Y: y, W: cmdW + 8, H: h}, spec, TextLeft, c.theme.Text)
				}
			} else {
				w := maxFloat32(view.W+p.scrollX, p.textWidth(lineText, spec)+8)
				c.TextStyled(lineText, Rect{X: x, Y: y, W: w, H: h}, spec, TextLeft, lineColor)
			}
		}

		if p.TerminalMode && i == promptIdx && p.Focused() && p.caretVisible {
			prefix := p.promptPrefixForLastLine()
			prefixRunes := utf8.RuneCountInString(prefix)
			caretCol := prefixRunes + min(max(0, p.InputCaret), utf8.RuneCountInString(p.InputText)) + utf8.RuneCountInString(p.preedit)
			cx := x + p.colX(lineText, caretCol, spec)
			c.Rect(Rect{X: cx, Y: y + 2, W: 2, H: maxFloat32(2, h-4)}, c.theme.AccentHover)
		}
	}
	c.PopClip()

	// Draw vertical scrollbar
	track, thumb := p.scrollbarThumbRect()
	if !track.Empty() {
		c.Rect(track, c.theme.PanelAlt)
		c.Line(Vec2{X: track.X, Y: track.Y}, Vec2{X: track.X, Y: track.Y + track.H}, 1, c.theme.Border)
		thumbColor := c.theme.Border
		if p.scrollDrag {
			thumbColor = c.theme.Accent
		} else if p.scrollHover {
			thumbColor = c.theme.ControlHover
		}
		c.RoundedRect(thumb, 3, thumbColor)
	}

	if p.Focused() {
		c.RoundedBorder(p.rect, radius, 1, c.theme.Accent)
	}
}

// SetMaxLines limits retained lines; values below one use the default limit.
func (p *OutputPanel) SetMaxLines(limit int) {
	if limit < 1 {
		limit = 5000
	}
	p.maxLines = limit
	p.trim()
	p.List.ModelChanged()
	p.InvalidatePaint()
}

func (p *OutputPanel) trim() {
	if excess := len(p.lines) - p.maxLines; excess > 0 {
		copy(p.lines, p.lines[excess:])
		p.lines = p.lines[:len(p.lines)-excess]
		p.firstID += uint64(excess)
		if p.selAnchor.Line >= excess {
			p.selAnchor.Line -= excess
		} else {
			p.selAnchor = OutputTextPos{}
		}
		if p.selCaret.Line >= excess {
			p.selCaret.Line -= excess
		} else {
			p.selCaret = OutputTextPos{}
		}
	}
}

// AppendText updates the last unfinished line when streaming newline-delimited chunks,
// and appends standalone lines cleanly. A trailing newline does not create an empty extra row.
func (p *OutputPanel) AppendText(chunk string) {
	if chunk == "" {
		return
	}
	follow := p.atBottom()
	if p.partial && len(p.lines) > 0 && (strings.ContainsAny(chunk, "\r\n") || strings.HasSuffix(p.lines[len(p.lines)-1], " ")) {
		chunk = p.lines[len(p.lines)-1] + chunk
		p.lines = p.lines[:len(p.lines)-1]
	}
	parts := strings.Split(chunk, "\n")
	for _, part := range parts[:len(parts)-1] {
		p.lines = append(p.lines, strings.TrimSuffix(part, "\r"))
	}
	p.partial = !strings.HasSuffix(chunk, "\n")
	if p.partial {
		p.lines = append(p.lines, strings.TrimSuffix(parts[len(parts)-1], "\r"))
	}
	p.trim()
	p.List.ModelChanged()
	if follow {
		p.ScrollToEnd()
	} else {
		p.InvalidatePaint()
	}
}

func (p *OutputPanel) atBottom() bool {
	height := p.List.rowHeight()
	end := maxFloat32(0, float32(len(p.lines))*height-p.List.rect.H)
	if p.List.Offset < end-1 {
		return false
	}
	maxY := p.maxScrollY()
	return p.scrollY >= maxY-float32(p.visualLineHeight()*1.5)
}

// ScrollToEnd follows the most recent line of output.
func (p *OutputPanel) ScrollToEnd() {
	if n := len(p.lines); n != 0 {
		p.List.ScrollTo(n - 1)
	}
	p.smoothScrolling = false
	p.scrollY = p.maxScrollY()
	p.targetScrollY = p.scrollY
	p.InvalidatePaint()
}

func (p *OutputPanel) Clear() {
	p.firstID += uint64(len(p.lines))
	p.lines = nil
	p.partial = false
	p.selAnchor = OutputTextPos{}
	p.selCaret = OutputTextPos{}
	p.selecting = false
	p.scrollX = 0
	p.scrollY = 0
	p.targetScrollY = 0
	p.smoothScrolling = false
	p.List.SelectedID = ""
	p.List.Offset = 0
	p.List.ModelChanged()
	p.InvalidatePaint()
}
