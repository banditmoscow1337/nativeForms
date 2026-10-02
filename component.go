package nativeforms

type Component interface {
	UIState() *State
	Measure(Constraints) Vec2
	Arrange(Rect)
	Paint(*Canvas)
	Handle(Event) bool
}

type State struct {
	owner    Component
	parent   Component
	manager  *Manager
	children []Component
	rect     Rect
	theme    Theme
	themed   bool

	preferred Vec2
	minimum   Vec2
	maximum   Vec2
	margin    Insets
	offset    Vec2
	anchor    Anchor
	flex      float32

	hidden    bool
	disabled  bool
	focusable bool
	clip      bool
	hovered   bool
	pressed   bool
	focused   bool

	onEvent func(Event) bool
}

func (state *State) UIState() *State { return state }

func (state *State) Measure(constraints Constraints) Vec2 {
	size := state.preferred
	size.X = maxFloat32(size.X, state.minimum.X)
	size.Y = maxFloat32(size.Y, state.minimum.Y)
	if state.maximum.X > 0 {
		size.X = minFloat32(size.X, state.maximum.X)
	}
	if state.maximum.Y > 0 {
		size.Y = minFloat32(size.Y, state.maximum.Y)
	}
	return constraints.Constrain(size)
}

func (state *State) Arrange(rect Rect)    { state.rect = rect }
func (state *State) Paint(canvas *Canvas) {}

func (state *State) Handle(event Event) bool {
	if state.onEvent != nil {
		return state.onEvent(event)
	}
	return false
}

func (state *State) Add(children ...Component) {
	for _, child := range children {
		if child == nil || child.UIState() == state || componentContainsState(child, state) {
			continue
		}
		childState := child.UIState()
		if childState.parent != nil {
			childState.parent.UIState().Remove(child)
		}
		childState.parent = state.owner
		state.children = append(state.children, child)
		bindTree(child, state.owner, state.manager)
		if state.themed {
			applyTheme(child, state.theme)
		}
	}
	state.invalidateLayout()
}

func componentContainsState(component Component, target *State) bool {
	if component == nil {
		return false
	}
	state := component.UIState()
	if state == target {
		return true
	}
	for _, child := range state.children {
		if componentContainsState(child, target) {
			return true
		}
	}
	return false
}

func (state *State) Remove(child Component) bool {
	for index, candidate := range state.children {
		if child == nil || candidate.UIState() != child.UIState() {
			continue
		}
		copy(state.children[index:], state.children[index+1:])
		state.children[len(state.children)-1] = nil
		state.children = state.children[:len(state.children)-1]
		bindTree(child, nil, nil)
		if state.manager != nil { state.manager.clearSubtreeInteraction(child.UIState()) }
		state.invalidateLayout()
		return true
	}
	return false
}

func (state *State) ClearChildren() {
	children := state.children
	state.children = nil
	for _, child := range children {
		if child != nil {
			bindTree(child, nil, nil)
			if state.manager != nil { state.manager.clearSubtreeInteraction(child.UIState()) }
		}
	}
	state.invalidateLayout()
}

func (state *State) Children() []Component {
	return append([]Component(nil), state.children...)
}

func (state *State) Parent() Component   { return state.parent }
func (state *State) Bounds() Rect        { return state.rect }
func (state *State) Visible() bool       { return !state.hidden }
func (state *State) Enabled() bool       { return !state.disabled }
func (state *State) Focusable() bool     { return state.focusable }
func (state *State) Hovered() bool       { return state.hovered }
func (state *State) Pressed() bool       { return state.pressed }
func (state *State) Focused() bool       { return state.focused }
func (state *State) Clipped() bool       { return state.clip }
func (state *State) Flex() float32       { return state.flex }
func (state *State) Margin() Insets      { return state.margin }
func (state *State) PreferredSize() Vec2 { return state.preferred }
func (state *State) Theme() Theme {
	if state.themed {
		return state.theme
	}
	return DefaultTheme()
}

func (state *State) SetVisible(visible bool) {
	if state.hidden == !visible { return }
	if !visible && state.manager != nil { state.manager.clearSubtreeInteraction(state) }
	state.hidden = !visible
	state.invalidateLayout()
}
func (state *State) SetEnabled(enabled bool) {
	if state.disabled == !enabled { return }
	if !enabled && state.manager != nil { state.manager.clearSubtreeInteraction(state) }
	state.disabled = !enabled
	state.invalidatePaint()
}
func (state *State) SetFocusable(focusable bool) {
	state.focusable = focusable
	if !focusable && state.manager != nil && sameComponent(state.owner, state.manager.focused) {
		state.manager.setFocus(nil)
	}
	state.invalidatePaint()
}
func (state *State) SetClip(clip bool) { state.clip = clip; state.invalidatePaint() }
func (state *State) SetFlex(flex float32) { state.flex = maxFloat32(0, flex); state.invalidateLayout() }
func (state *State) SetMargin(margin Insets) { state.margin = margin; state.invalidateLayout() }
func (state *State) SetOffset(offset Vec2) { state.offset = offset; state.invalidateLayout() }
func (state *State) SetAnchor(anchor Anchor) { state.anchor = anchor; state.invalidateLayout() }
func (state *State) SetPreferredSize(size Vec2) { state.preferred = size; state.invalidateLayout() }
func (state *State) SetMinimumSize(size Vec2) { state.minimum = size; state.invalidateLayout() }
func (state *State) SetMaximumSize(size Vec2) { state.maximum = size; state.invalidateLayout() }
func (state *State) SetEventHandler(fn func(Event) bool) { state.onEvent = fn }

func (state *State) invalidateLayout() {
	if state.manager != nil { state.manager.InvalidateLayout() }
}
func (state *State) invalidatePaint() {
	if state.manager != nil { state.manager.InvalidatePaint() }
}

func bindTree(component Component, parent Component, manager *Manager) {
	if component == nil {
		return
	}
	state := component.UIState()
	state.owner = component
	state.parent = parent
	state.manager = manager
	for _, child := range state.children {
		bindTree(child, component, manager)
	}
}

func applyTheme(component Component, theme Theme) {
	if component == nil {
		return
	}
	state := component.UIState()
	state.theme = theme
	state.themed = true
	for _, child := range state.children {
		applyTheme(child, theme)
	}
}

type Panel struct {
	State
	Direction   Direction
	Align       Align
	Gap         float32
	Padding     Insets
	Background  Color
	Border      Color
	BorderWidth float32

	layoutChildren []Component
	layoutSizes    []Vec2
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

func (panel *Panel) Measure(constraints Constraints) Vec2 {
	innerMax := Vec2{
		X: maxFloat32(0, constraints.Max.X-panel.Padding.Left-panel.Padding.Right),
		Y: maxFloat32(0, constraints.Max.Y-panel.Padding.Top-panel.Padding.Bottom),
	}
	childConstraints := Constraints{Max: innerMax}
	visibleCount := 0
	content := Vec2{}

	for _, child := range panel.children {
		if child == nil || !child.UIState().Visible() {
			continue
		}
		visibleCount++
		size := child.Measure(childConstraints)
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
	content.X = maxFloat32(content.X, panel.preferred.X)
	content.Y = maxFloat32(content.Y, panel.preferred.Y)
	content.X = maxFloat32(content.X, panel.minimum.X)
	content.Y = maxFloat32(content.Y, panel.minimum.Y)
	if panel.maximum.X > 0 {
		content.X = minFloat32(content.X, panel.maximum.X)
	}
	if panel.maximum.Y > 0 {
		content.Y = minFloat32(content.Y, panel.maximum.Y)
	}
	return constraints.Constrain(content)
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
	clear(panel.layoutChildren)
	children := panel.layoutChildren[:0]
	measured := panel.layoutSizes[:0]
	mainFixed := float32(0)
	flexTotal := float32(0)
	for _, child := range panel.children {
		if child == nil || !child.UIState().Visible() {
			continue
		}
		size := child.Measure(Constraints{Max: Vec2{X: inner.W, Y: inner.H}})
		children = append(children, child)
		measured = append(measured, size)
		state := child.UIState()
		margin := state.margin
		if panel.Direction == Horizontal {
			mainFixed += margin.Left + margin.Right
			if state.flex <= 0 {
				mainFixed += size.X
			}
		} else {
			mainFixed += margin.Top + margin.Bottom
			if state.flex <= 0 {
				mainFixed += size.Y
			}
		}
		flexTotal += state.flex
	}
	panel.layoutChildren = children
	panel.layoutSizes = measured
	if len(children) > 1 {
		mainFixed += panel.Gap * float32(len(children)-1)
	}
	mainAvailable := inner.W
	if panel.Direction == Vertical {
		mainAvailable = inner.H
	}
	flexSpace := maxFloat32(0, mainAvailable-mainFixed)
	// Freeze flex items that hit min/max, then redistribute the remainder.
	active := make([]bool, len(children))
	for i, child := range children { active[i] = child.UIState().flex > 0 }
	remaining, weight := flexSpace, flexTotal
	for weight > 0 {
		clamped := false
		for i, child := range children {
			if !active[i] { continue }
			state := child.UIState()
			share := maxFloat32(0, remaining) * state.flex / weight
			minimum, maximum := state.minimum.X, state.maximum.X
			if panel.Direction == Vertical { minimum, maximum = state.minimum.Y, state.maximum.Y }
			value := share
			if value < minimum { value = minimum }
			if maximum > 0 && value > maximum { value = maximum }
			if value != share {
				if panel.Direction == Horizontal { measured[i].X = value } else { measured[i].Y = value }
				active[i] = false
				remaining -= value
				weight -= state.flex
				clamped = true
			}
		}
		if clamped { continue }
		for i, child := range children {
			if !active[i] { continue }
			value := maxFloat32(0, remaining) * child.UIState().flex / weight
			if panel.Direction == Horizontal { measured[i].X = value } else { measured[i].Y = value }
		}
		break
	}
	cursor := inner.X
	if panel.Direction == Vertical {
		cursor = inner.Y
	}

	for index, child := range children {
		state := child.UIState()
		margin := state.margin
		size := measured[index]
		if panel.Direction == Horizontal {
			cursor += margin.Left
			availableCross := maxFloat32(0, inner.H-margin.Top-margin.Bottom)
			y, height := alignCross(inner.Y+margin.Top, availableCross, size.Y, panel.Align)
			child.Arrange(Rect{X: cursor, Y: y, W: size.X, H: height})
			cursor += size.X + margin.Right + panel.Gap
		} else {
			cursor += margin.Top
			availableCross := maxFloat32(0, inner.W-margin.Left-margin.Right)
			x, width := alignCross(inner.X+margin.Left, availableCross, size.X, panel.Align)
			child.Arrange(Rect{X: x, Y: cursor, W: width, H: size.Y})
			cursor += size.Y + margin.Bottom + panel.Gap
		}
	}
}

func (panel *Panel) Paint(canvas *Canvas) {
	if panel.Background.A > 0 {
		canvas.Rect(panel.rect, panel.Background)
	}
	if panel.BorderWidth > 0 && panel.Border.A > 0 {
		canvas.Border(panel.rect, panel.BorderWidth, panel.Border)
	}
}

func arrangeOverlayChild(child Component, inner Rect) {
	state := child.UIState()
	margin := state.margin
	available := inner.Inset(margin)
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
	x := available.X
	y := available.Y
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
