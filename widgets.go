package nativeforms

import "strings"

func resolveFontSpec(spec FontSpec, fontSize, themeSize float32) FontSpec {
	if fontSize > 0 {
		spec.Size = fontSize
	}
	if spec.Size <= 0 {
		spec.Size = themeSize
	}
	return spec
}

func controlColors(state *State, theme *Theme) (background, border Color) {
	if state.Frameless() {
		if !state.Enabled() {
			return Transparent, Transparent
		}
		if state.Pressed() {
			return theme.ControlPressed, Transparent
		}
		if state.Hovered() || state.Focused() {
			return theme.ControlHover, Transparent
		}
		return Transparent, Transparent
	}
	background = theme.Control
	border = theme.Border
	if !state.Enabled() {
		background = theme.Disabled
	} else if state.Pressed() {
		background = theme.ControlPressed
	} else if state.Hovered() || state.Focused() {
		background = theme.ControlHover
		border = theme.Accent
	}
	return background, border
}

func paintControlSurface(canvas *Canvas, state *State) {
	background, border := controlColors(state, canvas.theme)
	radius := state.effectiveCornerRadius(canvas.theme)
	if background.A > 0 {
		canvas.RoundedRect(state.rect, radius, background)
	}
	if border.A > 0 {
		canvas.RoundedBorder(state.rect, radius, 1, border)
	}
}

func enabledOpacity(enabled bool) float32 {
	if enabled {
		return 1
	}
	return 0.5
}

func handleControlActivation(state *State, event Event, onActivate func()) bool {
	if !state.Enabled() {
		return false
	}
	switch event.Type {
	case PointerDown:
		if event.Button == MouseLeft {
			return true
		}
	case PointerUp:
		if event.Button == MouseLeft && state.Pressed() && state.rect.Contains(event.X, event.Y) {
			if onActivate != nil {
				onActivate()
			}
			return true
		}
	case KeyDown:
		if !event.Repeat && (event.Key == KeyEnter || event.Key == KeySpace) {
			if onActivate != nil {
				onActivate()
			}
			return true
		}
	}
	return state.Handle(event)
}

type Label struct {
	State
	Text           string
	TextFunc       func() string
	Color          Color
	ColorRole      ThemeColorRole
	Background     Color
	BackgroundRole ThemeColorRole
	Border         Color
	BorderRole     ThemeColorRole
	Padding        Insets
	FontSize       float32
	Font           FontSpec
	Align          TextAlign
	Wrap           bool
	Ellipsis       bool

	cachedIn     string
	cachedOut    string
	cachedSize   Vec2
	cachedAvailW float32
	cachedSpec   FontSpec
	cachedGen    uint64
	cachedMode   uint8
}

func NewLabel(text string) *Label {
	label := &Label{Text: text}
	label.owner = label
	return label
}

func (label *Label) SetText(text string) {
	if label.Text == text {
		return
	}
	oldText := label.Text
	label.Text = text
	if !label.autoSize && !label.Wrap && !label.rect.Empty() &&
		!strings.Contains(oldText, "\n") && !strings.Contains(text, "\n") {
		if label.flex > 0 || (label.preferred.X > 0 && label.preferred.Y > 0) {
			label.InvalidatePaint()
			return
		}
		if panel, ok := label.parent.(*Panel); ok && panel.Direction == Vertical && panel.Align == AlignStretch {
			label.InvalidatePaint()
			return
		}
	}
	label.invalidateLayout()
}

func (label *Label) SetWrap(wrap bool) {
	if label.Wrap == wrap {
		return
	}
	label.Wrap = wrap
	label.cachedMode = 0
	label.invalidateLayout()
}

func (label *Label) SetEllipsis(ellipsis bool) {
	if label.Ellipsis == ellipsis {
		return
	}
	label.Ellipsis = ellipsis
	label.cachedMode = 0
	label.invalidateLayout()
}

func (label *Label) SetAlign(align TextAlign) {
	if label.Align == align {
		return
	}
	label.Align = align
	label.InvalidatePaint()
}

func (label *Label) SetFont(font FontSpec) {
	if label.Font == font {
		return
	}
	label.Font = font
	label.cachedMode = 0
	label.invalidateLayout()
}

func (label *Label) SetFontSize(size float32) {
	if label.FontSize == size {
		return
	}
	label.FontSize = size
	label.cachedMode = 0
	label.invalidateLayout()
}

func (label *Label) SetColor(color Color) {
	if label.Color == color && label.ColorRole == ThemeRoleNone {
		return
	}
	label.Color = color
	label.ColorRole = ThemeRoleNone
	label.InvalidatePaint()
}

func (label *Label) SetColorRole(role ThemeColorRole) {
	label.ColorRole = role
	if c, ok := label.Theme().ColorForRole(role); ok {
		label.Color = c
	}
	label.InvalidatePaint()
}

func (label *Label) SetBackground(color Color) {
	if label.Background == color && label.BackgroundRole == ThemeRoleNone {
		return
	}
	label.Background = color
	label.BackgroundRole = ThemeRoleNone
	label.InvalidatePaint()
}

func (label *Label) SetBackgroundRole(role ThemeColorRole) {
	label.BackgroundRole = role
	if c, ok := label.Theme().ColorForRole(role); ok {
		label.Background = c
	}
	label.InvalidatePaint()
}

func (label *Label) SetBorder(color Color) {
	if label.Border == color && label.BorderRole == ThemeRoleNone {
		return
	}
	label.Border = color
	label.BorderRole = ThemeRoleNone
	label.InvalidatePaint()
}

func (label *Label) SetBorderRole(role ThemeColorRole) {
	label.BorderRole = role
	if c, ok := label.Theme().ColorForRole(role); ok {
		label.Border = c
	}
	label.InvalidatePaint()
}

func (label *Label) SetPadding(padding Insets) {
	if label.Padding == padding {
		return
	}
	label.Padding = padding
	label.invalidateLayout()
}

func (label *Label) currentText() string {
	if label.TextFunc != nil {
		return label.TextFunc()
	}
	return label.Text
}

func (label *Label) shapeTextCached(text string, availW float32, spec FontSpec, mode uint8, needSize bool) (string, Vec2) {
	eff := label.effectiveFontSpec(spec)
	gen := label.fontContext().layoutGeneration()
	if label.cachedMode == mode && label.cachedIn == text && label.cachedAvailW == availW &&
		label.cachedSpec == eff && label.cachedGen == gen {
		if needSize && label.cachedSize == (Vec2{}) && label.cachedOut != "" {
			label.cachedSize = label.measureText(label.cachedOut, spec)
		}
		return label.cachedOut, label.cachedSize
	}
	out := text
	switch mode {
	case 2: // wrap
		out = label.wrapText(text, availW, spec)
	case 3: // ellipsis
		out = label.ellipsizeText(text, availW, spec)
	}
	var sz Vec2
	if needSize {
		sz = label.measureText(out, spec)
	}
	label.cachedMode = mode
	label.cachedIn = text
	label.cachedOut = out
	label.cachedAvailW = availW
	label.cachedSpec = eff
	label.cachedGen = gen
	label.cachedSize = sz
	return out, sz
}

func (label *Label) measureTextSize() Vec2 {
	spec := label.fontSpec()
	text := label.currentText()
	mode := uint8(1)
	availW := float32(0)
	if label.Wrap && label.maximum.X > 0 {
		availW = maxFloat32(0, label.maximum.X-label.Padding.Left-label.Padding.Right)
		mode = 2
	}
	_, size := label.shapeTextCached(text, availW, spec, mode, true)
	size.X += label.Padding.Left + label.Padding.Right
	size.Y += label.Padding.Top + label.Padding.Bottom
	return size
}

func (label *Label) Measure(constraints Constraints) Vec2 {
	spec := label.fontSpec()
	text := label.currentText()
	availW := maxFloat32(0, constraints.Max.X-label.Padding.Left-label.Padding.Right)
	mode := uint8(1)
	if label.Wrap && constraints.HasMaxX() {
		mode = 2
	} else if label.Ellipsis && !label.Wrap && constraints.HasMaxX() && !label.autoSize {
		mode = 3
	} else {
		availW = 0
	}
	_, size := label.shapeTextCached(text, availW, spec, mode, true)
	size.X += label.Padding.Left + label.Padding.Right
	size.Y += label.Padding.Top + label.Padding.Bottom
	return measureWithState(&label.State, size, constraints)
}

func (label *Label) fontSpec() FontSpec {
	return resolveFontSpec(label.Font, label.FontSize, label.Theme().FontSize)
}

func (label *Label) Baseline() float32 {
	if fonts := label.fontContext(); fonts != nil {
		return fonts.layoutShared("", label.effectiveFontSpec(label.fontSpec())).Ascent + label.Padding.Top
	}
	return label.fontSpec().Size*0.85 + label.Padding.Top
}

func (label *Label) Paint(canvas *Canvas) {
	if label.rect.Empty() {
		return
	}
	if !label.Frameless() {
		radius := label.effectiveCornerRadius(canvas.theme)
		bg := label.Background
		if bg.A <= 0 && label.BackgroundRole != ThemeRoleNone && canvas.theme != nil {
			if c, ok := canvas.theme.ColorForRole(label.BackgroundRole); ok {
				bg = c
			}
		}
		if bg.A > 0 {
			canvas.RoundedRect(label.rect, radius, bg)
		}
		bd := label.Border
		if bd.A <= 0 && label.BorderRole != ThemeRoleNone && canvas.theme != nil {
			if c, ok := canvas.theme.ColorForRole(label.BorderRole); ok {
				bd = c
			}
		}
		if bd.A > 0 {
			canvas.RoundedBorder(label.rect, radius, 1, bd)
		}
	}
	inner := label.rect.Inset(label.Padding)
	if inner.Empty() {
		return
	}
	spec := label.fontSpec()
	color := label.Color
	if color.A <= 0 && label.ColorRole != ThemeRoleNone && canvas.theme != nil {
		if c, ok := canvas.theme.ColorForRole(label.ColorRole); ok {
			color = c
		}
	}
	if color.A <= 0 {
		color = canvas.theme.Text
	}
	if !label.Enabled() {
		color = canvas.theme.Disabled
	}
	text := label.currentText()
	if label.Wrap {
		text, _ = label.shapeTextCached(text, inner.W, spec, 2, false)
	} else if label.Ellipsis {
		text, _ = label.shapeTextCached(text, inner.W, spec, 3, false)
	}
	canvas.TextStyled(text, inner, spec, label.Align, color)
}
