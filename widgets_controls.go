package nativeforms

import (
	"math"
	"strconv"
)

type Button struct {
	State
	Text     string
	Icon     *ImageResource
	FontSize float32
	Font     FontSpec
	OnClick  func()
}

func NewButton(text string, onClick func()) *Button {
	button := &Button{Text: text, OnClick: onClick}
	button.owner = button
	button.focusable = true
	button.minimum = Vec2{X: 240}
	return button
}

// NewCompactButton constructs a Button whose minimum width adapts to its text
// and icon without requiring manual SetMinimumSize/SetPreferredSize calls.
func NewCompactButton(text string, onClick func()) *Button {
	button := &Button{Text: text, OnClick: onClick}
	button.owner = button
	button.focusable = true
	return button
}

func (button *Button) SetText(text string) {
	if button.Text == text {
		return
	}
	button.Text = text
	button.invalidateLayout()
}

func (button *Button) SetIcon(icon *ImageResource) {
	if button.Icon == icon {
		return
	}
	button.Icon = icon
	button.invalidateLayout()
}

func (button *Button) SetFont(font FontSpec) {
	if button.Font == font {
		return
	}
	button.Font = font
	button.invalidateLayout()
}

func (button *Button) SetFontSize(size float32) {
	if button.FontSize == size {
		return
	}
	button.FontSize = size
	button.invalidateLayout()
}

func (button *Button) SetOnClick(onClick func()) {
	button.OnClick = onClick
}

func (button *Button) measureTextSize() Vec2 {
	theme := button.State.Theme()
	spec := button.fontSpec()
	text := button.measureText(button.Text, spec)
	iconSpace := iconWidth(button.Icon)
	if button.Text == "" && button.Icon != nil {
		iconSpace -= iconGap
	}
	desired := Vec2{X: text.X + 36 + iconSpace, Y: maxFloat32(text.Y+16, theme.ControlHeight)}
	if button.Icon != nil {
		desired.Y = maxFloat32(desired.Y, iconSize+12)
	}
	return desired
}

func (button *Button) Measure(constraints Constraints) Vec2 {
	return measureWithState(&button.State, button.measureTextSize(), constraints)
}

func (button *Button) fontSpec() FontSpec {
	return resolveFontSpec(button.Font, button.FontSize, button.Theme().FontSize)
}

func (button *Button) Paint(canvas *Canvas) {
	paintControlSurface(canvas, &button.State)
	spec := button.fontSpec()
	color := canvas.theme.Text
	if !button.Enabled() {
		color = canvas.theme.MutedText
	}
	inner := button.rect.Inset(Symmetric(12, 4))
	if inner.Empty() {
		return
	}
	if button.Icon == nil {
		canvas.TextStyled(button.Text, inner, spec, TextCenter, color)
		return
	}
	canvas.PushClip(inner)
	textWidth := button.textWidth(button.Text, spec)
	gap := float32(0)
	if button.Text != "" {
		gap = iconGap
	}
	groupWidth := iconSize + gap + textWidth
	start := inner.X + maxFloat32(0, (inner.W-groupWidth)*0.5)
	paintIcon(canvas, button.Icon, Rect{X: start, Y: inner.Y + (inner.H-iconSize)*0.5, W: iconSize, H: iconSize}, enabledOpacity(button.Enabled()))
	if button.Text != "" {
		bounds := Rect{X: start + iconSize + gap, Y: inner.Y, W: maxFloat32(0, inner.X+inner.W-start-iconSize-gap), H: inner.H}
		canvas.TextStyled(button.Text, bounds, spec, TextLeft, color)
	}
	canvas.PopClip()
}

func (button *Button) Handle(event Event) bool {
	return handleControlActivation(&button.State, event, button.OnClick)
}

type Toggle struct {
	State
	Text     string
	Icon     *ImageResource
	Value    bool
	OnChange func(bool)
}

func NewToggle(text string, value bool, onChange func(bool)) *Toggle {
	toggle := &Toggle{Text: text, Value: value, OnChange: onChange}
	toggle.owner = toggle
	toggle.focusable = true
	toggle.minimum = Vec2{X: 300}
	return toggle
}

func (toggle *Toggle) SetText(text string) {
	if toggle.Text == text {
		return
	}
	toggle.Text = text
	toggle.invalidateLayout()
}

func (toggle *Toggle) SetValue(value bool) {
	if toggle.Value == value {
		return
	}
	toggle.Value = value
	toggle.InvalidatePaint()
	if toggle.OnChange != nil {
		toggle.OnChange(value)
	}
}

func (toggle *Toggle) SetIcon(icon *ImageResource) {
	if toggle.Icon != icon {
		toggle.Icon = icon
		toggle.invalidateLayout()
	}
}

func (toggle *Toggle) measureTextSize() Vec2 {
	theme := toggle.State.Theme()
	text := toggle.measureText(toggle.Text, FontSpec{Size: theme.FontSize})
	height := maxFloat32(text.Y+16, theme.ControlHeight)
	return Vec2{X: text.X + 86 + iconWidth(toggle.Icon), Y: height}
}

func (toggle *Toggle) Measure(constraints Constraints) Vec2 {
	theme := toggle.State.Theme()
	text := toggle.measureText(toggle.Text, FontSpec{Size: theme.FontSize})
	height := maxFloat32(text.Y+16, theme.ControlHeight)
	pad := float32(110)
	if toggle.autoSize {
		pad = 86
	}
	return measureWithState(&toggle.State, Vec2{X: text.X + pad + iconWidth(toggle.Icon), Y: height}, constraints)
}

func (toggle *Toggle) Paint(canvas *Canvas) {
	paintControlSurface(canvas, &toggle.State)
	inner := toggle.rect.Inset(Symmetric(14, 4))
	if inner.Empty() {
		return
	}
	label := inner
	label.W = maxFloat32(0, label.W-55)
	paintIconLabel(canvas, toggle.Icon, toggle.Text, label, toggle.Icon != nil, canvas.theme.Text, enabledOpacity(toggle.Enabled()))
	value := "OFF"
	valueColor := canvas.theme.MutedText
	if toggle.Value {
		value = "ON"
		valueColor = canvas.theme.AccentHover
	}
	valRect := Rect{X: inner.X + label.W, Y: inner.Y, W: maxFloat32(0, inner.W-label.W), H: inner.H}
	canvas.Text(value, valRect, canvas.theme.FontSize, TextRight, valueColor)
}

func (toggle *Toggle) Handle(event Event) bool {
	return handleControlActivation(&toggle.State, event, func() {
		toggle.SetValue(!toggle.Value)
	})
}

type ProgressBar struct {
	State
	Value         float32
	Fill          Color
	Back          Color
	ShowValue     bool
	Indeterminate bool
	Phase         float32
	Text          string
	FormatValue   func(float32) string

	cachedPct   int
	cachedLabel string
}

func NewProgressBar(value float32) *ProgressBar {
	bar := &ProgressBar{Value: value, cachedPct: -1}
	bar.owner = bar
	bar.preferred = Vec2{X: 240, Y: 24}
	return bar
}

func (bar *ProgressBar) SetValue(value float32) {
	if bar.Value == value && !bar.Indeterminate {
		return
	}
	bar.Value = value
	bar.Indeterminate = false
	bar.InvalidatePaint()
}

func (bar *ProgressBar) SetIndeterminate(indeterminate bool) {
	if bar.Indeterminate == indeterminate {
		return
	}
	bar.Indeterminate = indeterminate
	bar.InvalidatePaint()
}

func (bar *ProgressBar) StepPhase(delta float32) {
	if math.IsNaN(float64(delta)) || math.IsInf(float64(delta), 0) {
		return
	}
	next := float32(math.Mod(float64(bar.Phase+delta), 1.0))
	if next < 0 {
		next += 1
	}
	if bar.Phase != next {
		bar.Phase = next
		if bar.Indeterminate {
			bar.InvalidatePaint()
		}
	}
}

func (bar *ProgressBar) SetText(text string) {
	if bar.Text == text {
		return
	}
	bar.Text = text
	if bar.autoSize {
		bar.invalidateLayout()
	} else {
		bar.InvalidatePaint()
	}
}

func (bar *ProgressBar) percentString() string {
	if bar.Text != "" {
		return bar.Text
	}
	clamped := clampFloat32(bar.Value, 0, 1)
	if bar.FormatValue != nil {
		return bar.FormatValue(clamped)
	}
	if bar.Indeterminate {
		return "..."
	}
	pct := int(clamped*100 + 0.5)
	if bar.cachedPct == pct && bar.cachedLabel != "" {
		return bar.cachedLabel
	}
	bar.cachedPct = pct
	bar.cachedLabel = strconv.Itoa(pct) + "%"
	return bar.cachedLabel
}

func (bar *ProgressBar) measureTextSize() Vec2 {
	theme := bar.State.Theme()
	label := bar.percentString()
	text := bar.measureText(label, FontSpec{Size: theme.FontSize * 0.8})
	return Vec2{X: maxFloat32(80, text.X+32), Y: maxFloat32(24, text.Y+8)}
}

func (bar *ProgressBar) Paint(canvas *Canvas) {
	if bar.rect.Empty() {
		return
	}
	background := bar.Back
	if background.A <= 0 {
		background = canvas.theme.Panel
	}
	fill := bar.Fill
	if fill.A <= 0 {
		fill = canvas.theme.Accent
	}
	radius := bar.effectiveCornerRadius(canvas.theme)
	if !bar.Frameless() {
		canvas.RoundedRect(bar.rect, radius, background)
	}
	var fillRect Rect
	if bar.Indeterminate {
		segW := maxFloat32(16, bar.rect.W*0.32)
		travel := maxFloat32(0, bar.rect.W-segW)
		pos := float32(0.5 - 0.5*math.Cos(float64(bar.Phase)*2*math.Pi))
		fillRect = Rect{X: bar.rect.X + travel*pos, Y: bar.rect.Y, W: segW, H: bar.rect.H}
	} else {
		clamped := clampFloat32(bar.Value, 0, 1)
		fillRect = Rect{X: bar.rect.X, Y: bar.rect.Y, W: bar.rect.W * clamped, H: bar.rect.H}
	}
	if radius > 0 {
		canvas.PushClip(bar.rect)
		canvas.RoundedRect(fillRect, radius, fill)
		canvas.PopClip()
	} else {
		canvas.Rect(fillRect, fill)
	}
	if !bar.Frameless() {
		canvas.RoundedBorder(bar.rect, radius, 1, canvas.theme.Border)
	}
	if bar.ShowValue || bar.Text != "" {
		canvas.Text(bar.percentString(), bar.rect, canvas.theme.FontSize*0.8, TextCenter, canvas.theme.Text)
	}
}
