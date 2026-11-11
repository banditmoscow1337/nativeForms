package nativeforms

import "math"

type ScrollPanel struct {
	State
	Content               Component
	Offset, OffsetX       float32
	Horizontal, Vertical  bool
	width, height         float32
	dragAxis              Direction
	dragStart, dragOffset float32
	targetX, targetY      float32
	smoothScrolling       bool

	cachedLayoutGen   uint64
	cachedViewW       float32
	cachedViewH       float32
	cachedHoriz       bool
	cachedVert        bool
	cachedContentSize Vec2
}

func NewScrollPanel(content Component) *ScrollPanel {
	p := &ScrollPanel{Content: content, Vertical: true}
	p.owner = p
	p.clip = true
	if content != nil {
		p.Add(content)
	}
	return p
}

func (p *ScrollPanel) contentSize(view Rect) Vec2 {
	if p.Content == nil {
		return Vec2{}
	}
	var gen uint64
	if p.manager != nil {
		gen = p.manager.layoutGen
		if gen != 0 && p.cachedLayoutGen == gen &&
			p.cachedViewW == view.W && p.cachedViewH == view.H &&
			p.cachedHoriz == p.Horizontal && p.cachedVert == p.Vertical {
			return p.cachedContentSize
		}
	}
	limits := Constraints{Max: Vec2{X: view.W, Y: view.H}}
	if p.Horizontal {
		limits.Max.X = 0
	}
	if p.Vertical {
		limits.Max.Y = 0
	}
	if !p.Horizontal && view.W > 0 {
		limits.BoundedX = true
		limits.Min.X = view.W
	}
	if !p.Vertical && view.H > 0 {
		limits.BoundedY = true
		limits.Min.Y = view.H
	}
	size := p.Content.Measure(limits)
	res := Vec2{X: maxFloat32(view.W, size.X), Y: maxFloat32(view.H, size.Y)}
	if gen != 0 {
		p.cachedLayoutGen = gen
		p.cachedViewW = view.W
		p.cachedViewH = view.H
		p.cachedHoriz = p.Horizontal
		p.cachedVert = p.Vertical
		p.cachedContentSize = res
	}
	return res
}

func (p *ScrollPanel) Measure(c Constraints) Vec2 {
	if p.Content == nil {
		return p.State.Measure(c)
	}
	view := Rect{W: c.Max.X, H: c.Max.Y}
	size := p.contentSize(view)
	return measureWithState(&p.State, size, c)
}

func (p *ScrollPanel) Arrange(r Rect) {
	p.rect = r
	if p.Content == nil {
		return
	}
	size := p.contentSize(r)
	p.width, p.height = size.X, size.Y
	maxX := maxFloat32(0, size.X-r.W)
	maxY := maxFloat32(0, size.Y-r.H)
	p.OffsetX = clampFloat32(p.OffsetX, 0, maxX)
	p.Offset = clampFloat32(p.Offset, 0, maxY)
	if !p.smoothScrolling {
		p.targetX, p.targetY = p.OffsetX, p.Offset
	} else {
		p.targetX = clampFloat32(p.targetX, 0, maxX)
		p.targetY = clampFloat32(p.targetY, 0, maxY)
	}
	p.Content.Arrange(Rect{X: r.X - p.OffsetX, Y: r.Y - p.Offset, W: size.X, H: size.Y})
}

func shiftComponentRects(c Component, dx, dy float32) {
	if c == nil || (dx == 0 && dy == 0) {
		return
	}
	state := c.UIState()
	state.rect.X += dx
	state.rect.Y += dy
	for _, child := range state.children {
		shiftComponentRects(child, dx, dy)
	}
}

func (p *ScrollPanel) applyScrollStep(dx, dy float32) Vec2 {
	beforeX, beforeY := p.OffsetX, p.Offset
	p.OffsetX = clampFloat32(p.OffsetX+dx, 0, maxFloat32(0, p.width-p.rect.W))
	p.Offset = clampFloat32(p.Offset+dy, 0, maxFloat32(0, p.height-p.rect.H))
	if beforeX != p.OffsetX || beforeY != p.Offset {
		if p.Content != nil && p.width > 0 && p.height > 0 {
			if _, isVirtual := p.Content.(*VirtualStack); isVirtual {
				p.Content.Arrange(Rect{X: p.rect.X - p.OffsetX, Y: p.rect.Y - p.Offset, W: p.width, H: p.height})
			} else {
				shiftComponentRects(p.Content, beforeX-p.OffsetX, beforeY-p.Offset)
			}
		} else {
			p.Arrange(p.rect)
		}
		p.InvalidatePaint()
	}
	return Vec2{X: dx - (p.OffsetX - beforeX), Y: dy - (p.Offset - beforeY)}
}

func (p *ScrollPanel) ScrollBy(dx, dy float32) Vec2 {
	p.smoothScrolling = false
	rem := p.applyScrollStep(dx, dy)
	p.targetX, p.targetY = p.OffsetX, p.Offset
	return rem
}

func (p *ScrollPanel) stepSmoothScroll(dt float64) bool {
	if !p.smoothScrolling {
		return false
	}
	maxX := maxFloat32(0, p.width-p.rect.W)
	maxY := maxFloat32(0, p.height-p.rect.H)
	p.targetX = clampFloat32(p.targetX, 0, maxX)
	p.targetY = clampFloat32(p.targetY, 0, maxY)
	dx := p.targetX - p.OffsetX
	dy := p.targetY - p.Offset
	if absFloat32(dx) <= 0.35 && absFloat32(dy) <= 0.35 {
		p.smoothScrolling = false
		p.applyScrollStep(dx, dy)
		return false
	}
	if dt <= 0 || dt > 0.1 {
		dt = 1.0 / 120.0
	}
	alpha := clampFloat32(float32(1.0-math.Exp(-dt*28.0)), 0.18, 0.85)
	p.applyScrollStep(dx*alpha, dy*alpha)
	return true
}

func (p *ScrollPanel) Scroll(pixels float32) { p.ScrollBy(0, pixels) }

func (p *ScrollPanel) ScrollIntoView(target Component) bool {
	if target == nil || sameComponent(p, target) || !containsComponent(p, target) {
		return false
	}
	r := target.UIState().rect
	dx, dy := float32(0), float32(0)
	if r.X < p.rect.X {
		dx = r.X - p.rect.X
	} else if r.X+r.W > p.rect.X+p.rect.W {
		dx = r.X + r.W - p.rect.X - p.rect.W
	}
	if r.Y < p.rect.Y {
		dy = r.Y - p.rect.Y
	} else if r.Y+r.H > p.rect.Y+p.rect.H {
		dy = r.Y + r.H - p.rect.Y - p.rect.H
	}
	beforeX, beforeY := p.OffsetX, p.Offset
	p.ScrollBy(dx, dy)
	return beforeX != p.OffsetX || beforeY != p.Offset
}

func (p *ScrollPanel) PaintOverlay(c *Canvas) {
	const thickness float32 = 8
	if p.height > p.rect.H && p.rect.H > 0 {
		fraction := p.rect.H / p.height
		h := minFloat32(p.rect.H, maxFloat32(20, p.rect.H*fraction))
		y := p.rect.Y + (p.rect.H-h)*p.Offset/(p.height-p.rect.H)
		c.RoundedRect(Rect{X: p.rect.X + p.rect.W - thickness, Y: y, W: thickness, H: h}, 4, c.theme.Border)
	}
	if p.width > p.rect.W && p.rect.W > 0 {
		fraction := p.rect.W / p.width
		width := minFloat32(p.rect.W, maxFloat32(20, p.rect.W*fraction))
		x := p.rect.X + (p.rect.W-width)*p.OffsetX/(p.width-p.rect.W)
		c.RoundedRect(Rect{X: x, Y: p.rect.Y + p.rect.H - thickness, W: width, H: thickness}, 4, c.theme.Border)
	}
}

func (p *ScrollPanel) Handle(event Event) bool {
	switch event.Type {
	case PointerScroll:
		dx, dy := -event.Scroll.X*48, -event.Scroll.Y*48
		if p.manager == nil {
			remaining := p.ScrollBy(dx, dy)
			return remaining != (Vec2{X: dx, Y: dy})
		}
		maxX := maxFloat32(0, p.width-p.rect.W)
		maxY := maxFloat32(0, p.height-p.rect.H)
		if !p.smoothScrolling {
			p.targetX, p.targetY = p.OffsetX, p.Offset
		}
		nextX := clampFloat32(p.targetX+dx, 0, maxX)
		nextY := clampFloat32(p.targetY+dy, 0, maxY)
		if nextX == p.OffsetX && nextY == p.Offset {
			return false
		}
		p.targetX, p.targetY = nextX, nextY
		p.applyScrollStep((p.targetX-p.OffsetX)*0.42, (p.targetY-p.Offset)*0.42)
		p.smoothScrolling = absFloat32(p.targetX-p.OffsetX) > 0.35 || absFloat32(p.targetY-p.Offset) > 0.35
		return true
	case PointerDown:
		if event.Button != MouseLeft {
			break
		}
		if p.height > p.rect.H && event.X >= p.rect.X+p.rect.W-10 {
			p.dragAxis = Vertical
			p.dragStart = event.Y
			p.dragOffset = p.Offset
			return true
		}
		if p.width > p.rect.W && event.Y >= p.rect.Y+p.rect.H-10 {
			p.dragAxis = Horizontal
			p.dragStart = event.X
			p.dragOffset = p.OffsetX
			return true
		}
	case PointerMove:
		if p.dragAxis == Vertical && p.rect.H > 0 {
			p.ScrollBy(0, p.dragOffset+(event.Y-p.dragStart)*p.height/p.rect.H-p.Offset)
			return true
		}
		if p.dragAxis == Horizontal && p.rect.W > 0 {
			p.ScrollBy(p.dragOffset+(event.X-p.dragStart)*p.width/p.rect.W-p.OffsetX, 0)
			return true
		}
	case PointerUp:
		if p.dragAxis != Overlay && event.Button == MouseLeft {
			p.dragAxis = Overlay
			return true
		}
	}
	return p.State.Handle(event)
}
