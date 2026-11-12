package nativeforms

// InteractiveCanvas is a focusable, custom-painted interactive surface widget
// designed for pixel-art editors, 2D/3D viewports, CAD canvases, and graphs.
// It supports pointer down/move/up/scroll, keyboard input, viewport culling,
// top overlay badges, and bottom status/hint overlays without child allocations.
type InteractiveCanvas struct {
	State
	Background          Color
	Border              Color
	BorderWidth         float32
	OverlayBadge        string
	OverlayBadgeColor   Color
	SecondaryBadge      string
	SecondaryBadgeColor Color
	BottomLeftHint      string
	BottomRightHint     string

	OnPaint       func(canvas *Canvas, bounds Rect)
	OnPointerDown func(event Event, local Vec2) bool
	OnPointerMove func(event Event, local Vec2) bool
	OnPointerUp   func(event Event, local Vec2) bool
	OnPointerExit func()
	OnScroll      func(event Event, local Vec2) bool
	OnKeyDown     func(event Event) bool
	OnKeyUp       func(event Event) bool
}

func NewInteractiveCanvas(onPaint func(canvas *Canvas, bounds Rect)) *InteractiveCanvas {
	ic := &InteractiveCanvas{
		Background:  RGBA(16, 21, 29, 255),
		BorderWidth: 1,
		OnPaint:     onPaint,
	}
	ic.owner = ic
	ic.focusable = true
	ic.clip = true
	ic.minimum = Vec2{X: 160, Y: 130}
	return ic
}

func (ic *InteractiveCanvas) SetOverlayBadge(text string) {
	if ic.OverlayBadge == text {
		return
	}
	ic.OverlayBadge = text
	ic.InvalidatePaint()
}

func (ic *InteractiveCanvas) SetSecondaryBadge(text string) {
	if ic.SecondaryBadge == text {
		return
	}
	ic.SecondaryBadge = text
	ic.InvalidatePaint()
}

func (ic *InteractiveCanvas) SetBottomHints(left, right string) {
	if ic.BottomLeftHint == left && ic.BottomRightHint == right {
		return
	}
	ic.BottomLeftHint = left
	ic.BottomRightHint = right
	ic.InvalidatePaint()
}

func (ic *InteractiveCanvas) Cull(viewport Rect) bool {
	b := ic.rect
	return b.Empty() || b.X+b.W <= viewport.X || b.X >= viewport.X+viewport.W ||
		b.Y+b.H <= viewport.Y || b.Y >= viewport.Y+viewport.H
}

func (ic *InteractiveCanvas) Measure(constraints Constraints) Vec2 {
	desired := ic.preferred
	if desired.X <= 0 {
		desired.X = maxFloat32(240, ic.minimum.X)
	}
	if desired.Y <= 0 {
		desired.Y = maxFloat32(180, ic.minimum.Y)
	}
	return measureWithState(&ic.State, desired, constraints)
}

func (ic *InteractiveCanvas) Paint(canvas *Canvas) {
	if ic.rect.Empty() {
		return
	}
	radius := ic.effectiveCornerRadius(canvas.theme)
	if !ic.Frameless() {
		bg := ic.Background
		if bg.A <= 0 {
			bg = canvas.theme.Panel
		}
		canvas.RoundedRect(ic.rect, radius, bg)
	}
	canvas.PushClip(ic.rect)
	if ic.OnPaint != nil {
		ic.OnPaint(canvas, ic.rect)
	}
	ic.paintOverlays(canvas)
	canvas.PopClip()

	if !ic.Frameless() && ic.BorderWidth > 0 {
		bd := ic.Border
		if bd.A <= 0 {
			bd = canvas.theme.Border
		}
		if ic.Focused() {
			bd = canvas.theme.Accent
		}
		canvas.RoundedBorder(ic.rect, radius, ic.BorderWidth, bd)
	}
}

func (ic *InteractiveCanvas) paintOverlays(canvas *Canvas) {
	fontSize := maxFloat32(11, canvas.theme.FontSize*0.72)
	spec := FontSpec{Size: fontSize, Weight: 500}
	topY := ic.rect.Y + 8
	if ic.OverlayBadge != "" {
		tw := ic.textWidth(ic.OverlayBadge, spec)
		bw := minFloat32(ic.rect.W-20, tw+16)
		bh := maxFloat32(22, fontSize+8)
		bRect := Rect{X: ic.rect.X + 10, Y: topY, W: bw, H: bh}
		fg := ic.OverlayBadgeColor
		if fg.A <= 0 {
			fg = RGBA(159, 175, 192, 255)
		}
		canvas.RoundedRect(bRect, 5, RGBA(17, 24, 34, 215))
		canvas.RoundedBorder(bRect, 5, 1, fg.WithAlpha(0.5))
		canvas.TextStyled(ic.OverlayBadge, bRect.Inset(Symmetric(6, 2)), spec, TextCenter, fg)
		topY += bh + 4
	}
	if ic.SecondaryBadge != "" {
		tw := ic.textWidth(ic.SecondaryBadge, spec)
		bw := minFloat32(ic.rect.W-20, tw+16)
		bh := maxFloat32(22, fontSize+8)
		bRect := Rect{X: maxFloat32(ic.rect.X+10, ic.rect.X+ic.rect.W-bw-10), Y: ic.rect.Y + 8, W: bw, H: bh}
		if ic.OverlayBadge != "" && bRect.X < ic.rect.X+160 {
			bRect.Y = topY
		}
		fg := ic.SecondaryBadgeColor
		if fg.A <= 0 {
			fg = canvas.theme.Accent
		}
		canvas.RoundedRect(bRect, 5, RGBA(17, 24, 34, 215))
		canvas.RoundedBorder(bRect, 5, 1, fg.WithAlpha(0.45))
		canvas.TextStyled(ic.SecondaryBadge, bRect.Inset(Symmetric(6, 2)), spec, TextCenter, fg)
	}
	hintH := maxFloat32(18, fontSize+4)
	hintY := ic.rect.Y + ic.rect.H - hintH - 6
	hintColor := RGBA(118, 134, 152, 235)
	if ic.BottomLeftHint != "" {
		leftRect := Rect{X: ic.rect.X + 10, Y: hintY, W: maxFloat32(0, ic.rect.W-90), H: hintH}
		canvas.TextStyled(ic.BottomLeftHint, leftRect, spec, TextLeft, hintColor)
	}
	if ic.BottomRightHint != "" {
		rightRect := Rect{X: ic.rect.X + 10, Y: hintY, W: maxFloat32(0, ic.rect.W-20), H: hintH}
		canvas.TextStyled(ic.BottomRightHint, rightRect, spec, TextRight, hintColor)
	}
}

func (ic *InteractiveCanvas) Handle(event Event) bool {
	if !ic.Enabled() {
		return false
	}
	local := Vec2{X: event.X - ic.rect.X, Y: event.Y - ic.rect.Y}
	switch event.Type {
	case PointerDown:
		if ic.OnPointerDown != nil && ic.OnPointerDown(event, local) {
			return true
		}
		if ic.rect.Contains(event.X, event.Y) {
			return true
		}
	case PointerMove:
		if ic.OnPointerMove != nil && ic.OnPointerMove(event, local) {
			return true
		}
	case PointerUp:
		if ic.OnPointerUp != nil && ic.OnPointerUp(event, local) {
			return true
		}
	case PointerScroll:
		if ic.OnScroll != nil && ic.OnScroll(event, local) {
			return true
		}
	case KeyDown:
		if ic.OnKeyDown != nil && ic.OnKeyDown(event) {
			return true
		}
	case KeyUp:
		if ic.OnKeyUp != nil && ic.OnKeyUp(event) {
			return true
		}
	case FocusLost:
		if ic.OnPointerExit != nil {
			ic.OnPointerExit()
		}
	}
	return ic.State.Handle(event)
}
