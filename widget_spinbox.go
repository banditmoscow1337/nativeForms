package nativeforms

import (
	"math"
	"strconv"
)

// and increment (+) buttons, keyboard arrow/page/home/end navigation, and
// mouse-wheel support.
type SpinBox struct {
	State
	Text        string
	Min         float32
	Max         float32
	Step        float32
	Value       float32
	Precision   int
	Prefix      string
	Suffix      string
	FormatValue func(float32) string
	OnChange    func(float32)

	hoverZone    uint8
	pressZone    uint8
	cacheValid   bool
	cachedValue  float32
	cachedPrec   int
	cachedPrefix string
	cachedSuffix string
	cachedStr    string
}

func NewSpinBox(text string, minimum, maximum, value, step float32, onChange func(float32)) *SpinBox {
	if maximum < minimum {
		minimum, maximum = maximum, minimum
	}
	prec := 0
	if step > 0 && math.Abs(float64(step)-math.Round(float64(step))) > 1e-5 {
		prec = 2
	}
	sb := &SpinBox{
		Text:      text,
		Min:       minimum,
		Max:       maximum,
		Step:      step,
		Value:     minimum,
		Precision: prec,
		OnChange:  onChange,
	}
	sb.owner = sb
	sb.focusable = true
	sb.minimum = Vec2{X: 220}
	sb.setValue(value, false)
	return sb
}

func (sb *SpinBox) SetText(text string) {
	if sb.Text == text {
		return
	}
	sb.Text = text
	sb.invalidateLayout()
}

func (sb *SpinBox) SetRange(minimum, maximum float32) {
	if maximum < minimum {
		minimum, maximum = maximum, minimum
	}
	sb.Min, sb.Max = minimum, maximum
	sb.setValue(sb.Value, true)
	sb.InvalidatePaint()
}

func (sb *SpinBox) SetStep(step float32) {
	sb.Step = step
	sb.setValue(sb.Value, true)
}

func (sb *SpinBox) SetPrecision(digits int) {
	digits = min(max(0, digits), 6)
	if sb.Precision == digits {
		return
	}
	sb.Precision = digits
	sb.cacheValid = false
	sb.InvalidatePaint()
}

func (sb *SpinBox) bounds() (minimum, maximum float32) {
	minimum, maximum = sb.Min, sb.Max
	if maximum < minimum {
		minimum, maximum = maximum, minimum
	}
	return minimum, maximum
}

func (sb *SpinBox) stepSize() float32 {
	if sb.Step > 0 && !math.IsInf(float64(sb.Step), 0) {
		return sb.Step
	}
	minimum, maximum := sb.bounds()
	if maximum > minimum {
		return (maximum - minimum) / 20
	}
	return 1
}

func (sb *SpinBox) SetValue(value float32) { sb.setValue(value, true) }

func (sb *SpinBox) Increment() { sb.SetValue(sb.Value + sb.stepSize()) }

func (sb *SpinBox) Decrement() { sb.SetValue(sb.Value - sb.stepSize()) }

func (sb *SpinBox) setValue(value float32, notify bool) {
	if math.IsNaN(float64(value)) || math.IsInf(float64(value), 0) {
		return
	}
	minimum, maximum := sb.bounds()
	value = clampFloat32(value, minimum, maximum)
	if sb.Step > 0 && !math.IsInf(float64(sb.Step), 0) && value != maximum {
		steps := math.Round(float64(value-minimum) / float64(sb.Step))
		snapped := float64(minimum) + steps*float64(sb.Step)
		if float64(maximum-value) <= math.Abs(float64(value)-snapped) {
			value = maximum
		} else {
			value = clampFloat32(float32(snapped), minimum, maximum)
		}
	}
	if value == sb.Value && sb.cacheValid {
		return
	}
	changed := value != sb.Value
	sb.Value = value
	sb.cacheValid = false
	if sb.autoSize {
		sb.invalidateLayout()
	} else {
		sb.InvalidatePaint()
	}
	if notify && changed && sb.OnChange != nil {
		sb.OnChange(value)
	}
}

func (sb *SpinBox) FormattedValue() string {
	if sb.FormatValue != nil {
		return sb.FormatValue(sb.Value)
	}
	prec := min(max(0, sb.Precision), 6)
	if sb.cacheValid && sb.cachedValue == sb.Value && sb.cachedPrec == prec &&
		sb.cachedPrefix == sb.Prefix && sb.cachedSuffix == sb.Suffix {
		return sb.cachedStr
	}
	var buf [64]byte
	b := append(buf[:0], sb.Prefix...)
	b = strconv.AppendFloat(b, float64(sb.Value), 'f', prec, 32)
	b = append(b, sb.Suffix...)
	sb.cachedValue = sb.Value
	sb.cachedPrec = prec
	sb.cachedPrefix = sb.Prefix
	sb.cachedSuffix = sb.Suffix
	sb.cachedStr = string(b)
	sb.cacheValid = true
	return sb.cachedStr
}

func (sb *SpinBox) buttonRects() (minusRect, plusRect Rect) {
	btnW := minFloat32(32, maxFloat32(22, sb.rect.H*0.8))
	plusRect = Rect{X: sb.rect.X + sb.rect.W - btnW, Y: sb.rect.Y, W: btnW, H: sb.rect.H}
	minusRect = Rect{X: plusRect.X - btnW, Y: sb.rect.Y, W: btnW, H: sb.rect.H}
	return minusRect, plusRect
}

func (sb *SpinBox) measureTextSize() Vec2 {
	theme := sb.State.Theme()
	spec := FontSpec{Size: theme.FontSize}
	titleW := float32(0)
	if sb.Text != "" {
		titleW = sb.textWidth(sb.Text, spec) + 16
	}
	valW := sb.textWidth(sb.FormattedValue(), spec)
	height := maxFloat32(theme.ControlHeight, theme.FontSize*1.2+14)
	btnW := minFloat32(32, maxFloat32(22, height*0.8))
	return Vec2{X: maxFloat32(120, titleW+valW+btnW*2+28), Y: height}
}

func (sb *SpinBox) Measure(constraints Constraints) Vec2 {
	desired := sb.measureTextSize()
	if !sb.autoSize && desired.X < 220 {
		desired.X = 220
	}
	return measureWithState(&sb.State, desired, constraints)
}

func (sb *SpinBox) Paint(canvas *Canvas) {
	if sb.rect.Empty() {
		return
	}
	paintControlSurface(canvas, &sb.State)
	radius := sb.effectiveCornerRadius(canvas.theme)
	minusRect, plusRect := sb.buttonRects()

	canvas.PushClip(sb.rect)
	defer canvas.PopClip()

	if !sb.Frameless() && sb.Enabled() {
		btnR := maxFloat32(0, radius-1)
		if sb.pressZone == 1 {
			canvas.RoundedRect(minusRect.Inset(All(2)), btnR, canvas.theme.ControlPressed)
		} else if sb.hoverZone == 1 && sb.Hovered() {
			canvas.RoundedRect(minusRect.Inset(All(2)), btnR, canvas.theme.Selection)
		}
		if sb.pressZone == 2 {
			canvas.RoundedRect(plusRect.Inset(All(2)), btnR, canvas.theme.ControlPressed)
		} else if sb.hoverZone == 2 && sb.Hovered() {
			canvas.RoundedRect(plusRect.Inset(All(2)), btnR, canvas.theme.Selection)
		}
		canvas.Line(Vec2{X: minusRect.X, Y: sb.rect.Y + 4}, Vec2{X: minusRect.X, Y: sb.rect.Y + sb.rect.H - 4}, 1, canvas.theme.Border)
		canvas.Line(Vec2{X: plusRect.X, Y: sb.rect.Y + 4}, Vec2{X: plusRect.X, Y: sb.rect.Y + sb.rect.H - 4}, 1, canvas.theme.Border)
	}

	textColor := canvas.theme.Text
	valColor := canvas.theme.AccentHover
	if !sb.Enabled() {
		textColor = canvas.theme.MutedText
		valColor = canvas.theme.MutedText
	}
	canvas.Text("−", minusRect, canvas.theme.FontSize, TextCenter, textColor)
	canvas.Text("+", plusRect, canvas.theme.FontSize, TextCenter, textColor)

	contentW := maxFloat32(0, minusRect.X-sb.rect.X-20)
	if contentW <= 0 {
		return
	}
	contentRect := Rect{X: sb.rect.X + 10, Y: sb.rect.Y, W: contentW, H: sb.rect.H}
	valStr := sb.FormattedValue()
	if sb.Text == "" {
		canvas.Text(valStr, contentRect, canvas.theme.FontSize, TextRight, valColor)
		return
	}
	valW := sb.textWidth(valStr, FontSpec{Size: canvas.theme.FontSize}) + 8
	titleW := maxFloat32(0, contentRect.W-valW)
	if titleW > 0 {
		canvas.Text(sb.Text, Rect{X: contentRect.X, Y: contentRect.Y, W: titleW, H: contentRect.H}, canvas.theme.FontSize, TextLeft, textColor)
	}
	canvas.Text(valStr, Rect{X: contentRect.X + titleW, Y: contentRect.Y, W: maxFloat32(0, contentRect.W-titleW), H: contentRect.H}, canvas.theme.FontSize, TextRight, valColor)
}

func (sb *SpinBox) zoneAt(x, y float32) uint8 {
	if !sb.rect.Contains(x, y) {
		return 0
	}
	minusRect, plusRect := sb.buttonRects()
	if minusRect.Contains(x, y) {
		return 1
	}
	if plusRect.Contains(x, y) {
		return 2
	}
	return 0
}

func (sb *SpinBox) Handle(event Event) bool {
	if !sb.Enabled() {
		return false
	}
	switch event.Type {
	case PointerMove:
		z := sb.zoneAt(event.X, event.Y)
		if z != sb.hoverZone {
			sb.hoverZone = z
			sb.InvalidatePaint()
		}
		return true
	case PointerDown:
		if event.Button == MouseLeft && sb.rect.Contains(event.X, event.Y) {
			sb.pressZone = sb.zoneAt(event.X, event.Y)
			sb.InvalidatePaint()
			return true
		}
	case PointerUp:
		if event.Button == MouseLeft && sb.Pressed() {
			z := sb.zoneAt(event.X, event.Y)
			pressed := sb.pressZone
			sb.pressZone = 0
			sb.InvalidatePaint()
			if z != 0 && z == pressed {
				switch z {
				case 1:
					sb.Decrement()
				case 2:
					sb.Increment()
				}
				return true
			}
			return true
		}
	case PointerScroll:
		delta := event.Scroll.Y
		if delta == 0 {
			delta = event.Scroll.X
		}
		if delta > 0 {
			sb.Increment()
			return true
		} else if delta < 0 {
			sb.Decrement()
			return true
		}
	case KeyDown:
		minimum, maximum := sb.bounds()
		switch event.Key {
		case KeyUpArrow, KeyRightArrow, KeyEqual:
			sb.Increment()
			return true
		case KeyDownArrow, KeyLeftArrow, KeyMinus:
			sb.Decrement()
			return true
		case KeyPageUp:
			sb.SetValue(sb.Value + sb.stepSize()*10)
			return true
		case KeyPageDown:
			sb.SetValue(sb.Value - sb.stepSize()*10)
			return true
		case KeyHome:
			sb.SetValue(minimum)
			return true
		case KeyEnd:
			sb.SetValue(maximum)
			return true
		}
	}
	return sb.State.Handle(event)
}
