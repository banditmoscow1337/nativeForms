package nativeforms

import (
	"fmt"
	"math"
)

// Slider is a focusable horizontal control for a value in [Min, Max].
// Step <= 0 allows continuous pointer input; keyboard and accessibility
// actions then change the value by one twentieth of the range.
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
	if maximum < minimum {
		minimum, maximum = maximum, minimum
	}
	slider := &Slider{Text: text, Min: minimum, Max: maximum, Step: step, Value: minimum, OnChange: onChange}
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

// SetValue clamps the value to the range, snaps it to Step if positive,
// and calls OnChange only when the resulting value changes.
func (slider *Slider) SetValue(value float32) { slider.setValue(value, true) }

func (slider *Slider) bounds() (minimum, maximum float32) {
	minimum, maximum = slider.Min, slider.Max
	if maximum < minimum {
		minimum, maximum = maximum, minimum
	}
	return minimum, maximum
}

func (slider *Slider) stepSize() float32 {
	if slider.Step > 0 && !math.IsInf(float64(slider.Step), 0) {
		return slider.Step
	}
	minimum, maximum := slider.bounds()
	return (maximum - minimum) / 20
}

func (slider *Slider) setValue(value float32, notify bool) {
	if math.IsNaN(float64(value)) || math.IsInf(float64(value), 0) {
		return
	}
	minimum, maximum := slider.bounds()
	value = clampFloat32(value, minimum, maximum)
	if slider.Step > 0 && !math.IsInf(float64(slider.Step), 0) && value != maximum {
		// Steps start at Min. Keep the exact maximum reachable even when
		// (Max-Min) is not divisible by Step.
		steps := math.Round(float64(value-minimum) / float64(slider.Step))
		snapped := float64(minimum) + steps*float64(slider.Step)
		if float64(maximum-value) <= math.Abs(float64(value)-snapped) {
			value = maximum
		} else {
			value = clampFloat32(float32(snapped), minimum, maximum)
		}
	}
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
	textColor := canvas.theme.Text
	valueColor := canvas.theme.AccentHover
	fillColor := canvas.theme.Accent
	if !slider.Enabled() {
		background = canvas.theme.Disabled
		textColor = canvas.theme.MutedText
		valueColor = canvas.theme.MutedText
		fillColor = canvas.theme.Disabled
	} else if slider.Hovered() || slider.Focused() {
		background = canvas.theme.ControlHover
		border = canvas.theme.Accent
	}
	canvas.Rect(slider.rect, background)
	canvas.Border(slider.rect, 1, border)
	labelBounds := Rect{
		X: slider.rect.X + 12,
		Y: slider.rect.Y + 3,
		W: maxFloat32(0, slider.rect.W-24),
		H: canvas.theme.FontSize*1.2 + 6,
	}
	canvas.Text(slider.Text, labelBounds, canvas.theme.FontSize, TextLeft, textColor)
	canvas.Text(slider.formattedValue(), labelBounds, canvas.theme.FontSize, TextRight, valueColor)
	track := slider.trackRect()
	canvas.Rect(track, canvas.theme.Panel)
	minimum, maximum := slider.bounds()
	fraction := float32(0)
	if maximum > minimum {
		fraction = clampFloat32((slider.Value-minimum)/(maximum-minimum), 0, 1)
	}
	canvas.Rect(Rect{X: track.X, Y: track.Y, W: track.W * fraction, H: track.H}, fillColor)
	thumbX := track.X + track.W*fraction
	canvas.Rect(Rect{X: thumbX - 3, Y: track.Y - 4, W: 6, H: track.H + 8}, valueColor)
}

func (slider *Slider) updateFromPointer(x float32) {
	track := slider.trackRect()
	fraction := clampFloat32((x-track.X)/maxFloat32(track.W, 1), 0, 1)
	minimum, maximum := slider.bounds()
	slider.SetValue(minimum + (maximum-minimum)*fraction)
}

func (slider *Slider) Handle(event Event) bool {
	if !slider.Enabled() {
		return false
	}
	switch event.Type {
	case PointerDown:
		if event.Button == MouseLeft && slider.rect.Contains(event.X, event.Y) {
			slider.updateFromPointer(event.X)
			return true
		}
	case PointerMove:
		if slider.Pressed() {
			slider.updateFromPointer(event.X)
			return true
		}
	case PointerUp:
		if event.Button == MouseLeft && slider.Pressed() {
			return true
		}
	case KeyDown:
		minimum, maximum := slider.bounds()
		switch event.Key {
		case KeyLeftArrow, KeyDownArrow, KeyA:
			slider.SetValue(slider.Value - slider.stepSize())
			return true
		case KeyRightArrow, KeyUpArrow, KeyD:
			slider.SetValue(slider.Value + slider.stepSize())
			return true
		case KeyHome:
			slider.SetValue(minimum)
			return true
		case KeyEnd:
			slider.SetValue(maximum)
			return true
		}
	}
	return slider.State.Handle(event)
}

func (slider *Slider) trackRect() Rect {
	return Rect{X: slider.rect.X + 14, Y: slider.rect.Y + slider.rect.H - 17, W: maxFloat32(0, slider.rect.W-28), H: 5}
