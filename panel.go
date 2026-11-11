package nativeforms

type Panel struct {
	State
	Direction      Direction
	Align          Align
	Gap            float32
	Padding        Insets
	Background     Color
	BackgroundRole ThemeColorRole
	Border         Color
	BorderRole     ThemeColorRole
	BorderWidth    float32
	CornerRadius   float32

	layoutChildren []Component
	layoutSizes    []Vec2
	layoutWeights  []float32
	layoutActive   []bool

	measuredGen    uint64
	measuredInnerW float32
	measuredInnerH float32
	measuredBoundX bool
	measuredBoundY bool
	measuredCount  int
}

func NewPanel(direction ...Direction) *Panel {
	panel := &Panel{Align: AlignStretch}
	if len(direction) > 0 {
		panel.Direction = direction[0]
	}
	panel.State.owner = panel
	return panel
}

func NewStack(direction Direction, gap float32) *Panel {
	panel := NewPanel(direction)
	panel.Gap = gap
	return panel
}

func (panel *Panel) SetDirection(direction Direction) {
	if panel.Direction == direction {
		return
	}
	panel.Direction = direction
	panel.invalidateLayout()
}

func (panel *Panel) SetAlign(align Align) {
	if panel.Align == align {
		return
	}
	panel.Align = align
	panel.invalidateLayout()
}

func (panel *Panel) SetGap(gap float32) {
	if panel.Gap == gap {
		return
	}
	panel.Gap = gap
	panel.invalidateLayout()
}

func (panel *Panel) SetPadding(padding Insets) {
	if panel.Padding == padding {
		return
	}
	panel.Padding = padding
	panel.invalidateLayout()
}

func (panel *Panel) SetBackground(color Color) {
	if panel.Background == color && panel.BackgroundRole == ThemeRoleNone {
		return
	}
	panel.Background = color
	panel.BackgroundRole = ThemeRoleNone
	panel.InvalidatePaint()
}

func (panel *Panel) SetBackgroundRole(role ThemeColorRole) {
	panel.BackgroundRole = role
	if c, ok := panel.Theme().ColorForRole(role); ok {
		panel.Background = c
	}
	panel.InvalidatePaint()
}

func (panel *Panel) SetBorder(color Color, width float32) {
	if panel.Border == color && panel.BorderWidth == width && panel.BorderRole == ThemeRoleNone {
		return
	}
	panel.Border = color
	panel.BorderWidth = width
	panel.BorderRole = ThemeRoleNone
	panel.InvalidatePaint()
}

func (panel *Panel) SetBorderRole(role ThemeColorRole, width float32) {
	panel.BorderRole = role
	panel.BorderWidth = width
	if c, ok := panel.Theme().ColorForRole(role); ok {
		panel.Border = c
	}
	panel.InvalidatePaint()
}

func (panel *Panel) ensureLayoutCapacity(n int) {
	if n <= 0 {
		return
	}
	if cap(panel.layoutChildren) < n {
		panel.layoutChildren = make([]Component, 0, n)
	}
	if cap(panel.layoutSizes) < n {
		panel.layoutSizes = make([]Vec2, 0, n)
	}
	if cap(panel.layoutWeights) < n {
		panel.layoutWeights = make([]float32, n)
	}
	if cap(panel.layoutActive) < n {
		panel.layoutActive = make([]bool, n)
	}
}

func (panel *Panel) Measure(constraints Constraints) Vec2 {
	panel.ensureLayoutCapacity(len(panel.children))
	innerMax := Vec2{
		X: maxFloat32(0, constraints.Max.X-panel.Padding.Left-panel.Padding.Right),
		Y: maxFloat32(0, constraints.Max.Y-panel.Padding.Top-panel.Padding.Bottom),
	}
	childConstraints := Constraints{Max: innerMax, BoundedX: constraints.HasMaxX(), BoundedY: constraints.HasMaxY()}
	visibleCount := 0
	content := Vec2{}
	children := panel.layoutChildren[:0]
	sizes := panel.layoutSizes[:0]

	for _, child := range panel.children {
		if child == nil || !child.UIState().Visible() {
			continue
		}
		visibleCount++
		size := child.Measure(childConstraints)
		children = append(children, child)
		sizes = append(sizes, size)
		margin := child.UIState().margin
		outerW := size.X + margin.Left + margin.Right
		outerH := size.Y + margin.Top + margin.Bottom
		switch panel.Direction {
		case Horizontal:
			content.X += outerW
			content.Y = maxFloat32(content.Y, outerH)
		case Vertical:
			content.X = maxFloat32(content.X, outerW)
			content.Y += outerH
		default:
			content.X = maxFloat32(content.X, outerW)
			content.Y = maxFloat32(content.Y, outerH)
		}
	}
	panel.layoutChildren, panel.layoutSizes = children, sizes
	if panel.manager != nil {
		panel.measuredGen = panel.manager.layoutGen
		panel.measuredInnerW = innerMax.X
		panel.measuredInnerH = innerMax.Y
		panel.measuredBoundX = childConstraints.BoundedX
		panel.measuredBoundY = childConstraints.BoundedY
		panel.measuredCount = visibleCount
	} else {
		panel.measuredGen = 0
	}
	if visibleCount > 1 && panel.Direction != Overlay {
		gap := panel.Gap * float32(visibleCount-1)
		if panel.Direction == Horizontal {
			content.X += gap
		} else {
			content.Y += gap
		}
	}

	content.X += panel.Padding.Left + panel.Padding.Right
	content.Y += panel.Padding.Top + panel.Padding.Bottom
	return measureWithState(&panel.State, content, constraints)
}

func (panel *Panel) Arrange(rect Rect) {
	panel.rect = rect
	inner := rect.Inset(panel.Padding)
	if panel.Direction == Overlay {
		for _, child := range panel.children {
			if child != nil && child.UIState().Visible() {
				arrangeOverlayChild(child, inner)
			}
		}
		return
	}
	panel.arrangeStack(inner)
}

func (panel *Panel) arrangeStack(inner Rect) {
	panel.ensureLayoutCapacity(len(panel.children))
	isVert := panel.Direction == Vertical
	mainAvailable := inner.W
	if isVert {
		mainAvailable = inner.H
	}
	reuseMeasured := panel.manager != nil && panel.measuredGen != 0 &&
		panel.measuredGen == panel.manager.layoutGen &&
		len(panel.layoutChildren) == panel.measuredCount &&
		len(panel.layoutSizes) == panel.measuredCount &&
		((isVert && panel.measuredBoundX && panel.measuredInnerW == inner.W) ||
			(!isVert && panel.measuredBoundY && panel.measuredInnerH == inner.H))

	var children []Component
	var sizes []Vec2
	total := float32(0)
	if reuseMeasured {
		children = panel.layoutChildren
		sizes = panel.layoutSizes
		for i, child := range children {
			size := sizes[i]
			needRemeasure := false
			if isVert {
				if panel.measuredBoundY && size.Y >= panel.measuredInnerH && panel.measuredInnerH > 0 {
					needRemeasure = true
				}
			} else {
				if panel.measuredBoundX && size.X >= panel.measuredInnerW && panel.measuredInnerW > 0 {
					needRemeasure = true
				}
			}
			if needRemeasure {
				limits := Constraints{Max: Vec2{X: inner.W, Y: inner.H}}
				if isVert {
					limits.BoundedX = true
					limits.Max.Y = 0
				} else {
					limits.BoundedY = true
					limits.Max.X = 0
				}
				size = child.Measure(limits)
			}
			state := child.UIState()
			if state.basis > 0 {
				if isVert {
					size.Y = state.basis
				} else {
					size.X = state.basis
				}
			}
			sizes[i] = size
			if isVert {
				total += size.Y + state.margin.Top + state.margin.Bottom
			} else {
				total += size.X + state.margin.Left + state.margin.Right
			}
		}
	} else {
		children = panel.layoutChildren[:0]
		sizes = panel.layoutSizes[:0]
		for _, child := range panel.children {
			if child == nil || !child.UIState().Visible() {
				continue
			}
			limits := Constraints{Max: Vec2{X: inner.W, Y: inner.H}}
			if isVert {
				limits.BoundedX = true
				limits.Max.Y = 0
			} else {
				limits.BoundedY = true
				limits.Max.X = 0
			}
			size := child.Measure(limits)
			state := child.UIState()
			if state.basis > 0 {
				if isVert {
					size.Y = state.basis
				} else {
					size.X = state.basis
				}
			}
			children = append(children, child)
			sizes = append(sizes, size)
			if isVert {
				total += size.Y + state.margin.Top + state.margin.Bottom
			} else {
				total += size.X + state.margin.Left + state.margin.Right
			}
		}
		panel.layoutChildren, panel.layoutSizes = children, sizes
	}
	if len(children) > 1 {
		total += panel.Gap * float32(len(children)-1)
	}
	delta := mainAvailable - total
	weights := panel.layoutWeights[:len(children)]
	active := panel.layoutActive[:len(children)]
	weight := float32(0)
	hasFlexShrinkable := false
	if delta < 0 {
		for _, child := range children {
			st := child.UIState()
			if st.flex > 0 && (!st.shrinkSet || st.shrink > 0) {
				hasFlexShrinkable = true
				break
			}
		}
	}
	for i, child := range children {
		state := child.UIState()
		if delta >= 0 {
			weights[i] = state.flex
		} else {
			shrink := float32(1)
			if state.shrinkSet {
				shrink = state.shrink
			} else if hasFlexShrinkable && state.flex == 0 {
				shrink = 0
			}
			base := sizes[i].X
			if isVert {
				base = sizes[i].Y
			}
			weights[i] = shrink * base
		}
		active[i] = weights[i] > 0
		weight += weights[i]
	}
	remaining := delta
	for weight > 0 {
		frozen := false
		for i, child := range children {
			if !active[i] {
				continue
			}
			state := child.UIState()
			base, low, high := sizes[i].X, state.minimum.X, state.maximum.X
			if isVert {
				base, low, high = sizes[i].Y, state.minimum.Y, state.maximum.Y
			}
			if state.autoSize {
				low = 0
			}
			proposed := base + remaining*weights[i]/weight
			bounded := maxFloat32(low, proposed)
			if high > 0 {
				bounded = minFloat32(bounded, high)
			}
			if bounded != proposed {
				if isVert {
					sizes[i].Y = bounded
				} else {
					sizes[i].X = bounded
				}
				remaining -= bounded - base
				weight -= weights[i]
				active[i] = false
				frozen = true
			}
		}
		if frozen {
			continue
		}
		invWeight := remaining / weight
		for i := range children {
			if !active[i] {
				continue
			}
			if isVert {
				sizes[i].Y += weights[i] * invWeight
			} else {
				sizes[i].X += weights[i] * invWeight
			}
		}
		break
	}
	baseline := float32(0)
	if panel.Align == AlignBaseline && !isVert {
		for i, child := range children {
			height := child.Measure(Constraints{Max: Vec2{X: sizes[i].X, Y: inner.H}, BoundedX: true, BoundedY: true}).Y
			sizes[i].Y = height
			value := childBaseline(child, height)
			baseline = maxFloat32(baseline, value+child.UIState().margin.Top)
		}
	}
	cursor := inner.X
	if isVert {
		cursor = inner.Y
	}
	for i, child := range children {
		state := child.UIState()
		margin := state.margin
		size := sizes[i]
		crossAlign := panel.Align
		if state.autoSize && crossAlign == AlignStretch && state.flex == 0 {
			crossAlign = AlignStart
		}
		if !isVert {
			cursor += margin.Left
			cross := maxFloat32(0, inner.H-margin.Top-margin.Bottom)
			y, height := alignCross(inner.Y+margin.Top, cross, size.Y, crossAlign)
			if panel.Align == AlignBaseline {
				y = inner.Y + baseline - childBaseline(child, size.Y)
				height = size.Y
			}
			child.Arrange(Rect{X: cursor, Y: y, W: maxFloat32(0, size.X), H: height})
			cursor += size.X + margin.Right + panel.Gap
		} else {
			cursor += margin.Top
			cross := maxFloat32(0, inner.W-margin.Left-margin.Right)
			x, width := alignCross(inner.X+margin.Left, cross, size.X, crossAlign)
			child.Arrange(Rect{X: x, Y: cursor, W: width, H: maxFloat32(0, size.Y)})
			cursor += size.Y + margin.Bottom + panel.Gap
		}
	}
}

func childBaseline(child Component, fallbackHeight float32) float32 {
	if b, ok := child.(interface{ Baseline() float32 }); ok {
		return b.Baseline()
	}
	return fallbackHeight * 0.8
}

func (panel *Panel) effectiveRadius(theme *Theme) float32 {
	if panel.CornerRadius > 0 {
		return panel.CornerRadius
	}
	return panel.effectiveCornerRadius(theme)
}

func (panel *Panel) Paint(canvas *Canvas) {
	if panel.Frameless() {
		return
	}
	radius := panel.effectiveRadius(canvas.theme)
	bg := panel.Background
	if bg.A <= 0 && panel.BackgroundRole != ThemeRoleNone && canvas.theme != nil {
		if c, ok := canvas.theme.ColorForRole(panel.BackgroundRole); ok {
			bg = c
		}
	}
	if bg.A > 0 {
		canvas.RoundedRect(panel.rect, radius, bg)
	}
	bd := panel.Border
	if bd.A <= 0 && panel.BorderRole != ThemeRoleNone && canvas.theme != nil {
		if c, ok := canvas.theme.ColorForRole(panel.BorderRole); ok {
			bd = c
		}
	}
	if panel.BorderWidth > 0 && bd.A > 0 {
		canvas.RoundedBorder(panel.rect, radius, panel.BorderWidth, bd)
	}
}

func arrangeOverlayChild(child Component, inner Rect) {
	state := child.UIState()
	available := inner.Inset(state.margin)
	if state.anchor == AnchorFill {
		child.Arrange(Rect{
			X: available.X + state.offset.X,
			Y: available.Y + state.offset.Y,
			W: maxFloat32(0, available.W-state.offset.X),
			H: maxFloat32(0, available.H-state.offset.Y),
		})
		return
	}
	size := child.Measure(Constraints{Max: Vec2{X: available.W, Y: available.H}})
	x, y := available.X, available.Y
	switch state.anchor {
	case AnchorTop, AnchorCenter, AnchorBottom:
		x += (available.W - size.X) * 0.5
	case AnchorTopRight, AnchorRight, AnchorBottomRight:
		x += available.W - size.X
	}
	switch state.anchor {
	case AnchorLeft, AnchorCenter, AnchorRight:
		y += (available.H - size.Y) * 0.5
	case AnchorBottomLeft, AnchorBottom, AnchorBottomRight:
		y += available.H - size.Y
	}
	child.Arrange(Rect{X: x + state.offset.X, Y: y + state.offset.Y, W: size.X, H: size.Y})
}

func alignCross(start, available, requested float32, align Align) (position, size float32) {
	size = minFloat32(requested, available)
	switch align {
	case AlignCenter:
		position = start + (available-size)*0.5
	case AlignEnd:
		position = start + available - size
	case AlignStretch:
		position = start
		size = available
	default:
		position = start
	}
	return
}

type Spacer struct{ State }

func NewSpacer(width, height float32) *Spacer {
	spacer := &Spacer{}
	spacer.owner = spacer
	spacer.preferred = Vec2{X: width, Y: height}
	return spacer
}
