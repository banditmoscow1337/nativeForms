package nativeforms

import (
	"fmt"
)

type Label struct {
	State
	Text     string
	TextFunc func() string
	Color    Color
	FontSize float32
	Align    TextAlign
	Wrap     bool
}

func NewLabel(text string) *Label {
	label := &Label{Text: text}
	label.owner = label
	return label
}

func (label *Label) SetText(text string) {
	if label.Text == text { return }
	label.Text = text
	label.invalidateLayout()
}

func (label *Label) currentText() string {
	if label.TextFunc != nil {
		return label.TextFunc()
	}
	return label.Text
}

func (label *Label) Measure(constraints Constraints) Vec2 {
	fontSize := label.FontSize
	if fontSize <= 0 {
		fontSize = label.State.Theme().FontSize
	}
	text:=label.currentText()
	if label.Wrap && constraints.HasMaxX() { text=WrapText(text,maxFloat32(0,constraints.Max.X),fontSize) }
	return measureWithState(&label.State, MeasureText(text, fontSize), constraints)
}

func (label *Label) Baseline() float32 {
	size:=label.FontSize;if size<=0 { size=label.Theme().FontSize }
	return size*0.85
}

func (label *Label) Paint(canvas *Canvas) {
	fontSize := label.FontSize
	if fontSize <= 0 {
		fontSize = canvas.theme.FontSize
	}
	color := label.Color
	if color.A <= 0 {
		color = canvas.theme.Text
	}
	if !label.Enabled() {
		color = canvas.theme.Disabled
	}
	text:=label.currentText()
	if label.Wrap { text=WrapText(text,label.rect.W,fontSize) }
	canvas.Text(text, label.rect, fontSize, label.Align, color)
}

type Button struct {
	State
	Text     string
	FontSize float32
	OnClick  func()
}

func NewButton(text string, onClick func()) *Button {
	button := &Button{Text: text, OnClick: onClick}
	button.owner = button
	button.focusable = true
	button.minimum = Vec2{X: 240}
	return button
}

func (button *Button) SetText(text string) {
	if button.Text == text { return }
	button.Text = text
	button.invalidateLayout()
}

func (button *Button) Measure(constraints Constraints) Vec2 {
	theme := button.State.Theme()
	fontSize := button.FontSize
	if fontSize <= 0 {
		fontSize = theme.FontSize
	}
	text := MeasureText(button.Text, fontSize)
	desired := Vec2{X: text.X + 36, Y: maxFloat32(text.Y+16, theme.ControlHeight)}
	return measureWithState(&button.State, desired, constraints)
}

func (button *Button) Paint(canvas *Canvas) {
	background := canvas.theme.Control
	border := canvas.theme.Border
	if !button.Enabled() {
		background = canvas.theme.Disabled
	} else if button.Pressed() {
		background = canvas.theme.ControlPressed
	} else if button.Hovered() || button.Focused() {
		background = canvas.theme.ControlHover
		border = canvas.theme.Accent
	}
	canvas.Rect(button.rect, background)
	canvas.Border(button.rect, 1, border)
	fontSize := button.FontSize
	if fontSize <= 0 {
		fontSize = canvas.theme.FontSize
	}
	color := canvas.theme.Text
	if !button.Enabled() {
		color = canvas.theme.MutedText
	}
	canvas.Text(button.Text, button.rect.Inset(Symmetric(12, 4)), fontSize, TextCenter, color)
}

func (button *Button) Handle(event Event) bool {
	if !button.Enabled() {
		return false
	}
	switch event.Type {
	case PointerDown:
		return event.Button == MouseLeft
	case PointerUp:
		if event.Button == MouseLeft && button.Pressed() && button.rect.Contains(event.X, event.Y) {
			if button.OnClick != nil {
				button.OnClick()
			}
			return true
		}
	case KeyDown:
		if !event.Repeat && (event.Key == KeyEnter || event.Key == KeySpace) {
			if button.OnClick != nil {
				button.OnClick()
			}
			return true
		}
	}
	return button.State.Handle(event)
}

type Toggle struct {
	State
	Text     string
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

func (toggle *Toggle) SetValue(value bool) {
	if toggle.Value == value { return }
	toggle.Value = value
	toggle.invalidatePaint()
	if toggle.OnChange != nil { toggle.OnChange(value) }
}

func (toggle *Toggle) Measure(constraints Constraints) Vec2 {
	theme := toggle.State.Theme()
	text := MeasureText(toggle.Text, theme.FontSize)
	height := maxFloat32(text.Y+16, theme.ControlHeight)
	return measureWithState(&toggle.State, Vec2{X: text.X + 110, Y: height}, constraints)
}

func (toggle *Toggle) Paint(canvas *Canvas) {
	background := canvas.theme.Control
	border := canvas.theme.Border
	if !toggle.Enabled() {
		background = canvas.theme.Disabled
	} else if toggle.Pressed() {
		background = canvas.theme.ControlPressed
	} else if toggle.Hovered() || toggle.Focused() {
		background = canvas.theme.ControlHover
		border = canvas.theme.Accent
	}
	canvas.Rect(toggle.rect, background)
	canvas.Border(toggle.rect, 1, border)
	inner := toggle.rect.Inset(Symmetric(14, 4))
	canvas.Text(toggle.Text, inner, canvas.theme.FontSize, TextLeft, canvas.theme.Text)
	value := "OFF"
	valueColor := canvas.theme.MutedText
	if toggle.Value {
		value = "ON"
		valueColor = canvas.theme.AccentHover
	}
	canvas.Text(value, inner, canvas.theme.FontSize, TextRight, valueColor)
}

func (toggle *Toggle) Handle(event Event) bool {
	if !toggle.Enabled() {
		return false
	}
	activate := event.Type == KeyDown && !event.Repeat && (event.Key == KeyEnter || event.Key == KeySpace)
	activate = activate || (event.Type == PointerUp && event.Button == MouseLeft && toggle.Pressed() && toggle.rect.Contains(event.X, event.Y))
	if event.Type == PointerDown && event.Button == MouseLeft {
		return true
	}
	if activate {
		toggle.Value = !toggle.Value
		if toggle.OnChange != nil {
			toggle.OnChange(toggle.Value)
		}
		return true
	}
	return toggle.State.Handle(event)
}

type Slider struct {
	State
	Text        string
	Min         float32
	Max         float32
	Step        float32
	Value       float32
	FormatValue func(float32) string
	OnChange    func(float32)
}

func NewSlider(text string, minimum, maximum, value, step float32, onChange func(float32)) *Slider {
	slider := &Slider{Text: text, Min: minimum, Max: maximum, Step: step, OnChange: onChange}
	slider.owner = slider
	slider.focusable = true
	slider.minimum = Vec2{X: 360}
	slider.setValue(value, false)
	return slider
}

func (slider *Slider) Measure(constraints Constraints) Vec2 {
	theme := slider.State.Theme()
	height := maxFloat32(58, maxFloat32(theme.ControlHeight+14, theme.FontSize*1.2+30))
	return measureWithState(&slider.State, Vec2{X: 360, Y: height}, constraints)
}

func (slider *Slider) SetValue(value float32) { slider.setValue(value, true) }

func (slider *Slider) setValue(value float32, notify bool) {
	if slider.Max < slider.Min {
		slider.Min, slider.Max = slider.Max, slider.Min
	}
	value = roundToStep(clampFloat32(value, slider.Min, slider.Max), slider.Step)
	value = clampFloat32(value, slider.Min, slider.Max)
	if value == slider.Value {
		return
	}
	slider.Value = value
	slider.invalidatePaint()
	if notify && slider.OnChange != nil {
		slider.OnChange(value)
	}
}

func (slider *Slider) formattedValue() string {
	if slider.FormatValue != nil {
		return slider.FormatValue(slider.Value)
	}
	return fmt.Sprintf("%.2f", slider.Value)
}

func (slider *Slider) Paint(canvas *Canvas) {
	background := canvas.theme.Control
	border := canvas.theme.Border
	if !slider.Enabled() {
		background = canvas.theme.Disabled
	} else if slider.Hovered() || slider.Focused() {
		background = canvas.theme.ControlHover
		border = canvas.theme.Accent
	}
	canvas.Rect(slider.rect, background)
	canvas.Border(slider.rect, 1, border)
	labelBounds := Rect{
		X: slider.rect.X + 12,
		Y: slider.rect.Y + 3,
		W: slider.rect.W - 24,
		H: canvas.theme.FontSize*1.2 + 6,
	}
	canvas.Text(slider.Text, labelBounds, canvas.theme.FontSize, TextLeft, canvas.theme.Text)
	canvas.Text(slider.formattedValue(), labelBounds, canvas.theme.FontSize, TextRight, canvas.theme.AccentHover)
	track := slider.trackRect()
	canvas.Rect(track, canvas.theme.Panel)
	fraction := float32(0)
	if slider.Max > slider.Min {
		fraction = (slider.Value - slider.Min) / (slider.Max - slider.Min)
	}
	canvas.Rect(Rect{X: track.X, Y: track.Y, W: track.W * fraction, H: track.H}, canvas.theme.Accent)
	thumbX := track.X + track.W*fraction
	canvas.Rect(Rect{X: thumbX - 3, Y: track.Y - 4, W: 6, H: track.H + 8}, canvas.theme.AccentHover)
}

func (slider *Slider) Handle(event Event) bool {
	if !slider.Enabled() {
		return false
	}
	switch event.Type {
	case PointerDown, PointerMove:
		if event.Type == PointerDown && event.Button != MouseLeft {
			return false
		}
		if event.Type == PointerDown || slider.Pressed() {
			track := slider.trackRect()
			fraction := clampFloat32((event.X-track.X)/maxFloat32(track.W, 1), 0, 1)
			slider.setValue(slider.Min+(slider.Max-slider.Min)*fraction, true)
			return true
		}
	case PointerUp:
		return event.Button == MouseLeft
	case KeyDown:
		delta := slider.Step
		if delta <= 0 {
			delta = (slider.Max - slider.Min) / 20
		}
		if event.Key == KeyLeftArrow || event.Key == KeyA {
			slider.setValue(slider.Value-delta, true)
			return true
		}
		if event.Key == KeyRightArrow || event.Key == KeyD {
			slider.setValue(slider.Value+delta, true)
			return true
		}
	}
	return slider.State.Handle(event)
}

func (slider *Slider) trackRect() Rect {
	return Rect{X: slider.rect.X + 14, Y: slider.rect.Y + slider.rect.H - 17, W: maxFloat32(0, slider.rect.W-28), H: 5}
}

type ProgressBar struct {
	State
	Value     float32
	Fill      Color
	Back      Color
	ShowValue bool
}

func NewProgressBar(value float32) *ProgressBar {
	bar := &ProgressBar{Value: value}
	bar.owner = bar
	bar.preferred = Vec2{X: 240, Y: 24}
	return bar
}

func (bar *ProgressBar) SetValue(value float32) {
	if bar.Value == value { return }
	bar.Value = value
	bar.invalidatePaint()
}

func (bar *ProgressBar) Paint(canvas *Canvas) {
	background := bar.Back
	if background.A <= 0 {
		background = canvas.theme.Panel
	}
	fill := bar.Fill
	if fill.A <= 0 {
		fill = canvas.theme.Accent
	}
	canvas.Rect(bar.rect, background)
	canvas.Rect(Rect{X: bar.rect.X, Y: bar.rect.Y, W: bar.rect.W * clampFloat32(bar.Value, 0, 1), H: bar.rect.H}, fill)
	canvas.Border(bar.rect, 1, canvas.theme.Border)
	if bar.ShowValue {
		canvas.Text(fmt.Sprintf("%d%%", int(clampFloat32(bar.Value, 0, 1)*100+0.5)), bar.rect, canvas.theme.FontSize*0.8, TextCenter, canvas.theme.Text)
	}
}

func measureWithState(state *State, desired Vec2, constraints Constraints) Vec2 {
	if state.preferred.X > 0 {
		desired.X = state.preferred.X
	}
	if state.preferred.Y > 0 {
		desired.Y = state.preferred.Y
	}
	desired.X = maxFloat32(desired.X, state.minimum.X)
	desired.Y = maxFloat32(desired.Y, state.minimum.Y)
	if state.maximum.X > 0 {
		desired.X = minFloat32(desired.X, state.maximum.X)
	}
	if state.maximum.Y > 0 {
		desired.Y = minFloat32(desired.Y, state.maximum.Y)
	}
	return constraints.Constrain(desired)
}
