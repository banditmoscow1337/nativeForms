package nativeforms

func (state *State) Parent() Component     { return state.parent }
func (state *State) Bounds() Rect          { return state.rect }
func (state *State) Visible() bool         { return !state.hidden }
func (state *State) Enabled() bool         { return !state.disabled }
func (state *State) Focusable() bool       { return state.focusable }
func (state *State) Hovered() bool         { return state.hovered }
func (state *State) Pressed() bool         { return state.pressed }
func (state *State) Focused() bool         { return state.focused }
func (state *State) Clipped() bool         { return state.clip }
func (state *State) Frameless() bool       { return state.frameless }
func (state *State) CornerRadius() float32 { return state.cornerRadius }
func (state *State) AutoSize() bool        { return state.autoSize }
func (state *State) Flex() float32         { return state.flex }
func (state *State) Margin() Insets        { return state.margin }
func (state *State) PreferredSize() Vec2   { return state.preferred }
func (state *State) MinimumSize() Vec2     { return state.minimum }
func (state *State) MaximumSize() Vec2     { return state.maximum }
func (state *State) Theme() Theme {
	if state.themed {
		return state.theme
	}
	return DefaultTheme()
}

func (state *State) effectiveCornerRadius(theme *Theme) float32 {
	if state.cornerRadius > 0 {
		return state.cornerRadius
	}
	if theme != nil && theme.CornerRadius > 0 {
		return theme.CornerRadius
	}
	if state.themed && state.theme.CornerRadius > 0 {
		return state.theme.CornerRadius
	}
	return 0
}

func (state *State) SetVisible(visible bool) {
	if state.hidden == !visible {
		return
	}
	if !visible && state.manager != nil {
		state.manager.clearSubtreeInteraction(state)
	}
	state.hidden = !visible
	state.invalidateLayout()
}

func (state *State) SetEnabled(enabled bool) {
	if state.disabled == !enabled {
		return
	}
	if !enabled && state.manager != nil {
		state.manager.clearSubtreeInteraction(state)
	}
	state.disabled = !enabled
	state.InvalidatePaint()
}

func (state *State) SetFocusable(focusable bool) {
	state.focusable = focusable
	if !focusable && state.manager != nil && sameComponent(state.owner, state.manager.focused) {
		state.manager.setFocus(nil)
	}
	state.InvalidatePaint()
}

func (state *State) SetClip(clip bool) { state.clip = clip; state.InvalidatePaint() }

func (state *State) SetFrameless(frameless bool) {
	if state.frameless == frameless {
		return
	}
	state.frameless = frameless
	state.InvalidatePaint()
}

// SetCornerRadius sets the corner rounding radius in logical pixels.
func (state *State) SetCornerRadius(radius float32) {
	radius = maxFloat32(0, radius)
	if state.cornerRadius == radius {
		return
	}
	state.cornerRadius = radius
	state.InvalidatePaint()
}

// SetAutoSize enables or disables automatic element resizing to fit its text
// whenever the text changes or layout is measured.
func (state *State) SetAutoSize(autoSize bool) {
	if state.autoSize == autoSize {
		return
	}
	state.autoSize = autoSize
	state.invalidateLayout()
}

// SizeToText measures the element's natural text-fitted size on demand,
// updates its preferred and current bounds to fit the text, and invalidates layout.
func (state *State) SizeToText() Vec2 {
	var desired Vec2
	if state.owner != nil {
		if m, ok := state.owner.(textSizeMeasurer); ok {
			desired = m.measureTextSize()
		} else {
			prevAuto := state.autoSize
			state.autoSize = true
			desired = state.owner.Measure(Constraints{})
			state.autoSize = prevAuto
		}
	}
	if state.maximum.X > 0 {
		desired.X = minFloat32(desired.X, state.maximum.X)
	}
	if state.maximum.Y > 0 {
		desired.Y = minFloat32(desired.Y, state.maximum.Y)
	}
	state.preferred = desired
	if state.minimum.X > desired.X {
		state.minimum.X = desired.X
	}
	if state.minimum.Y > desired.Y {
		state.minimum.Y = desired.Y
	}
	state.rect.W = desired.X
	state.rect.H = desired.Y
	state.invalidateLayout()
	return desired
}

func (state *State) SetFlex(flex float32) { state.flex = maxFloat32(0, flex); state.invalidateLayout() }

func (state *State) SetFlexBasis(basis float32) {
	state.basis = maxFloat32(0, basis)
	state.invalidateLayout()
}

func (state *State) SetFlexShrink(shrink float32) {
	state.shrink = maxFloat32(0, shrink)
	state.shrinkSet = true
	state.invalidateLayout()
}

func (state *State) SetMargin(margin Insets)             { state.margin = margin; state.invalidateLayout() }
func (state *State) SetOffset(offset Vec2)               { state.offset = offset; state.invalidateLayout() }
func (state *State) SetAnchor(anchor Anchor)             { state.anchor = anchor; state.invalidateLayout() }
func (state *State) SetPreferredSize(size Vec2)          { state.preferred = size; state.invalidateLayout() }
func (state *State) SetMinimumSize(size Vec2)            { state.minimum = size; state.invalidateLayout() }
func (state *State) SetMaximumSize(size Vec2)            { state.maximum = size; state.invalidateLayout() }
func (state *State) SetEventHandler(fn func(Event) bool) { state.onEvent = fn }
func (state *State) SetTooltip(text string)              { state.tooltip = text }
func (state *State) Tooltip() string                     { return state.tooltip }

func (state *State) invalidateLayout() {
	if state.manager != nil {
		state.manager.InvalidateLayout()
	}
}

func (state *State) InvalidatePaint() {
	if state.manager != nil {
		state.manager.InvalidatePaint()
	}
}

func bindTree(component Component, parent Component, manager *Manager) {
	if component == nil {
		return
	}
	state := component.UIState()
	state.owner = component
	state.parent = parent
	state.manager = manager
	if manager != nil {
		if panel, ok := component.(*Panel); ok && len(panel.children) > 0 {
			panel.ensureLayoutCapacity(len(panel.children))
		}
	}
	for _, child := range state.children {
		bindTree(child, component, manager)
	}
}

func sameRGB(a, b Color) bool {
	return a.A > 0 && b.A > 0 && a.R == b.R && a.G == b.G && a.B == b.B
}

func matchesThemeSlot(c, prev, dark, light Color) bool {
	return sameRGB(c, prev) || sameRGB(c, dark) || sameRGB(c, light)
}

func applyTheme(component Component, theme Theme) {
	if component == nil {
		return
	}
	state := component.UIState()
	prev := DefaultTheme()
	if state.themed {
		prev = state.theme
	}
	defDark := DefaultTheme()
	defLight := LightTheme()
	if panel, ok := component.(*Panel); ok {
		bg := panel.Background
		switch {
		case matchesThemeSlot(bg, prev.Panel, defDark.Panel, defLight.Panel):
			panel.Background = theme.Panel
		case matchesThemeSlot(bg, prev.PanelAlt, defDark.PanelAlt, defLight.PanelAlt):
			panel.Background = theme.PanelAlt
		case matchesThemeSlot(bg, prev.Header, defDark.Header, defLight.Header):
			panel.Background = theme.Header
		case matchesThemeSlot(bg, prev.Control, defDark.Control, defLight.Control):
			panel.Background = theme.Control
		case matchesThemeSlot(bg, prev.Tooltip, defDark.Tooltip, defLight.Tooltip):
			panel.Background = theme.Tooltip
		}
		bd := panel.Border
		switch {
		case matchesThemeSlot(bd, prev.Border, defDark.Border, defLight.Border):
			panel.Border = theme.Border
		case matchesThemeSlot(bd, prev.Accent, defDark.Accent, defLight.Accent):
			panel.Border = theme.Accent
		}
	}
	state.theme = theme
	state.themed = true
	for _, child := range state.children {
		applyTheme(child, theme)
	}
}

func measureWithState(state *State, desired Vec2, constraints Constraints) Vec2 {
	if !state.autoSize {
		if state.preferred.X > 0 {
			desired.X = state.preferred.X
		}
		if state.preferred.Y > 0 {
			desired.Y = state.preferred.Y
		}
		desired.X = maxFloat32(desired.X, state.minimum.X)
		desired.Y = maxFloat32(desired.Y, state.minimum.Y)
	}
	if state.maximum.X > 0 {
		desired.X = minFloat32(desired.X, state.maximum.X)
	}
	if state.maximum.Y > 0 {
		desired.Y = minFloat32(desired.Y, state.maximum.Y)
	}
	return constraints.Constrain(desired)
}
