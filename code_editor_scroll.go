package nativeforms

import "math"

func (e *CodeEditor) fontSpec() FontSpec {
	spec := e.Font
	if spec.Size <= 0 {
		spec.Size = 16
	}
	return spec
}

func (e *CodeEditor) rowHeight() float32 {
	if fonts := e.fontContext(); fonts != nil {
		h := fonts.Measure("", e.effectiveFontSpec(e.fontSpec())).Y
		return maxFloat32(1, float32(math.Ceil(float64(h))))
	}
	return float32(math.Ceil(float64(e.fontSpec().Size * 1.25)))
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
		W: maxFloat32(0, e.rect.W-gutter-8-codeEditorScrollbarWidth), H: maxFloat32(0, e.rect.H-8)}
}

func (e *CodeEditor) scrollbarTrackRect() Rect {
	if e.rect.W <= codeEditorScrollbarWidth || e.rect.H <= 0 {
		return Rect{}
	}
	return Rect{
		X: e.rect.X + e.rect.W - codeEditorScrollbarWidth,
		Y: e.rect.Y,
		W: codeEditorScrollbarWidth,
		H: e.rect.H,
	}
}

func (e *CodeEditor) scrollbarThumbRect() (track, thumb Rect) {
	track = e.scrollbarTrackRect()
	if track.Empty() {
		return Rect{}, Rect{}
	}
	lineCount := max(1, e.Document.LineCount())
	h := e.rowHeight()
	totalH := maxFloat32(track.H, float32(lineCount)*h)
	viewH := maxFloat32(1, e.rect.H-8)
	availH := maxFloat32(1, track.H-4)
	thumbH := clampFloat32(availH*(viewH/totalH), minFloat32(24, availH), availH)
	maxY := e.maxScrollY()
	ratio := float32(0)
	if maxY > 0 {
		ratio = clampFloat32(e.scrollY/maxY, 0, 1)
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

func (e *CodeEditor) maxScrollY() float32 {
	if e.rect.H <= 8 {
		return maxFloat32(0, float32(e.Document.LineCount())*e.rowHeight())
	}
	return maxFloat32(0, float32(e.Document.LineCount())*e.rowHeight()-(e.rect.H-8))
}

// maxScrollX returns the maximum horizontal scroll offset for the currently visible lines.
// When all visible lines fit within the editor viewport width, maxScrollX returns 0 so the
// editor never scrolls horizontally into empty space.
func (e *CodeEditor) maxScrollX() float32 {
	view := e.contentRect()
	if view.W <= 0 || e.Document == nil {
		return 0
	}
	maxW := e.visibleMaxLineW
	if maxW <= 0 {
		first, last := e.VisibleLines()
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
	}
	if maxW <= view.W-8 {
		return 0
	}
	return maxFloat32(0, maxW+16-view.W)
}

func (e *CodeEditor) Measure(c Constraints) Vec2 { return e.State.Measure(c) }

func (e *CodeEditor) Arrange(r Rect) {
	e.rect = r
	e.syncDocument()
	if e.pendingCenterLine >= 0 && r.H > 8 {
		line := min(max(0, e.pendingCenterLine), e.Document.LineCount()-1)
		e.pendingCenterLine = -1
		h := e.rowHeight()
		viewH := maxFloat32(h, r.H-8)
		targetY := float32(line)*h - (viewH-h)*0.35
		e.scrollY = clampFloat32(targetY, 0, e.maxScrollY())
		e.scrollTargetY = e.scrollY
		e.smoothScrolling = false
		e.revealCaret()
	}
	maxY := e.maxScrollY()
	e.scrollY = clampFloat32(e.scrollY, 0, maxY)
	maxX := e.maxScrollX()
	e.scrollX = clampFloat32(e.scrollX, 0, maxX)
	if !e.smoothScrolling {
		e.scrollTargetY = e.scrollY
		e.scrollTargetX = e.scrollX
	} else {
		e.scrollTargetY = clampFloat32(e.scrollTargetY, 0, maxY)
		e.scrollTargetX = clampFloat32(e.scrollTargetX, 0, maxX)
	}
}

// VisibleLines reports the half-open range rendered for the current viewport.
func (e *CodeEditor) VisibleLines() (first, last int) {
	e.syncDocument()
	if e.rect.H <= 8 || e.rect.W <= 0 {
		return 0, 0
	}
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
	nextY := clampFloat32(float32(line)*e.rowHeight(), 0, e.maxScrollY())
	e.scrollTargetY = nextY
	e.smoothScrolling = false
	if e.scrollY != nextY {
		e.scrollY = nextY
		e.visibleMaxLineW = 0
		e.scrollX = clampFloat32(e.scrollX, 0, e.maxScrollX())
		e.scrollTargetX = e.scrollX
		e.caretRectValid = false
		e.InvalidatePaint()
	}
}

// CenterOnLine scrolls the editor so that line (0-based) is positioned with
// comfortable context lines above it in the viewport. If the editor has not been
// arranged yet, it defers centering until Arrange runs.
func (e *CodeEditor) CenterOnLine(line int) {
	e.syncDocument()
	line = min(max(0, line), e.Document.LineCount()-1)
	if e.rect.H <= 8 {
		e.pendingCenterLine = line
		e.ScrollToLine(line)
		return
	}
	e.pendingCenterLine = -1
	h := e.rowHeight()
	viewH := maxFloat32(h, e.rect.H-8)
	targetY := float32(line)*h - (viewH-h)*0.35
	nextY := clampFloat32(targetY, 0, e.maxScrollY())
	e.scrollTargetY = nextY
	e.smoothScrolling = false
	if e.scrollY != nextY {
		e.scrollY = nextY
		e.visibleMaxLineW = 0
		e.scrollX = clampFloat32(e.scrollX, 0, e.maxScrollX())
		e.scrollTargetX = e.scrollX
		e.caretRectValid = false
		e.InvalidatePaint()
	}
}

func (e *CodeEditor) stepSmoothScroll(dt float64) bool {
	if !e.smoothScrolling {
		return false
	}
	maxY := e.maxScrollY()
	maxX := e.maxScrollX()
	e.scrollTargetY = clampFloat32(e.scrollTargetY, 0, maxY)
	e.scrollTargetX = clampFloat32(e.scrollTargetX, 0, maxX)
	dx := e.scrollTargetX - e.scrollX
	dy := e.scrollTargetY - e.scrollY
	if absFloat32(dx) <= 0.35 && absFloat32(dy) <= 0.35 {
		changed := e.scrollX != e.scrollTargetX || e.scrollY != e.scrollTargetY
		e.scrollX = e.scrollTargetX
		e.scrollY = e.scrollTargetY
		e.smoothScrolling = false
		if changed {
			e.caretRectValid = false
			e.InvalidatePaint()
		}
		return false
	}
	if dt <= 0 || dt > 0.1 {
		dt = 1.0 / 120.0
	}
	alpha := float32(1.0 - math.Exp(-dt*28.0))
	alpha = clampFloat32(alpha, 0.18, 0.85)
	e.scrollX = clampFloat32(e.scrollX+dx*alpha, 0, maxX)
	e.scrollY = clampFloat32(e.scrollY+dy*alpha, 0, maxY)
	e.caretRectValid = false
	e.InvalidatePaint()
	return true
}
