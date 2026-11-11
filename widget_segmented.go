package nativeforms

// SegmentedControl renders a horizontal pill/button group for selecting one of
// several mutually exclusive options.
type SegmentedControl struct {
	State
	Options    []RadioOption
	SelectedID string
	OnChange   func(string)
	hoveredIdx int
}

func NewSegmentedControl(options []RadioOption, selected string, onChange func(string)) *SegmentedControl {
	sc := &SegmentedControl{
		Options:    append([]RadioOption(nil), options...),
		SelectedID: selected,
		OnChange:   onChange,
		hoveredIdx: -1,
	}
	sc.owner = sc
	sc.focusable = true
	sc.clip = true
	if sc.SelectedID == "" && len(sc.Options) > 0 {
		sc.SelectedID = sc.Options[0].ID
	}
	return sc
}

func (sc *SegmentedControl) SetOptions(options []RadioOption) {
	sc.Options = append(sc.Options[:0], options...)
	sc.hoveredIdx = -1
	sc.invalidateLayout()
}

func (sc *SegmentedControl) SetSelected(id string) {
	if selectRadioOption(sc.Options, &sc.SelectedID, id) {
		sc.InvalidatePaint()
		if sc.OnChange != nil {
			sc.OnChange(id)
		}
	}
}

func (sc *SegmentedControl) SelectedIndex() int {
	for i, opt := range sc.Options {
		if opt.ID == sc.SelectedID {
			return i
		}
	}
	return -1
}

func (sc *SegmentedControl) measureTextSize() Vec2 {
	theme := sc.State.Theme()
	spec := FontSpec{Size: theme.FontSize}
	totalW := float32(8)
	maxSegW := float32(48)
	for _, opt := range sc.Options {
		w := sc.measureText(opt.Label, spec).X + 24 + iconWidth(opt.Icon)
		maxSegW = maxFloat32(maxSegW, w)
	}
	if len(sc.Options) > 0 {
		totalW += maxSegW * float32(len(sc.Options))
	}
	return Vec2{X: maxFloat32(96, totalW), Y: theme.ControlHeight}
}

func (sc *SegmentedControl) Measure(constraints Constraints) Vec2 {
	return measureWithState(&sc.State, sc.measureTextSize(), constraints)
}

func (sc *SegmentedControl) Paint(canvas *Canvas) {
	if sc.rect.Empty() {
		return
	}
	radius := sc.effectiveCornerRadius(canvas.theme)
	if !sc.Frameless() {
		bg := canvas.theme.PanelAlt
		if !sc.Enabled() {
			bg = canvas.theme.Disabled
		}
		canvas.RoundedRect(sc.rect, radius, bg)
	}
	n := len(sc.Options)
	if n == 0 {
		return
	}
	inner := sc.rect.Inset(All(3))
	segW := inner.W / float32(n)
	segR := maxFloat32(0, radius-2)
	for i, opt := range sc.Options {
		segRect := Rect{X: inner.X + float32(i)*segW, Y: inner.Y, W: segW, H: inner.H}
		selected := opt.ID == sc.SelectedID
		if selected {
			canvas.RoundedRect(segRect, segR, canvas.theme.Accent)
		} else if i == sc.hoveredIdx && sc.Hovered() && sc.Enabled() {
			canvas.RoundedRect(segRect, segR, canvas.theme.ControlHover)
		}
		if i > 0 && !selected && sc.Options[i-1].ID != sc.SelectedID {
			canvas.Line(Vec2{X: segRect.X, Y: segRect.Y + 5}, Vec2{X: segRect.X, Y: segRect.Y + segRect.H - 5}, 1, canvas.theme.Border)
		}
		color := canvas.theme.Text
		if !sc.Enabled() {
			color = canvas.theme.MutedText
		} else if selected && !canvas.theme.IsLight() {
			color = RGBA(15, 18, 24, 255)
		} else if selected && canvas.theme.IsLight() {
			color = RGBA(255, 255, 255, 255)
		}
		if opt.Icon == nil {
			canvas.Text(opt.Label, segRect.Inset(Symmetric(6, 0)), canvas.theme.FontSize, TextCenter, color)
		} else {
			canvas.PushClip(segRect)
			tw := sc.textWidth(opt.Label, FontSpec{Size: canvas.theme.FontSize})
			gap := float32(0)
			if opt.Label != "" {
				gap = iconGap
			}
			groupW := iconSize + gap + tw
			start := segRect.X + maxFloat32(4, (segRect.W-groupW)*0.5)
			paintIcon(canvas, opt.Icon, Rect{X: start, Y: segRect.Y + (segRect.H-iconSize)*0.5, W: iconSize, H: iconSize}, enabledOpacity(sc.Enabled()))
			if opt.Label != "" {
				canvas.Text(opt.Label, Rect{X: start + iconSize + gap, Y: segRect.Y, W: maxFloat32(0, segRect.X+segRect.W-start-iconSize-gap), H: segRect.H}, canvas.theme.FontSize, TextLeft, color)
			}
			canvas.PopClip()
		}
	}
	if !sc.Frameless() {
		border := canvas.theme.Border
		if sc.Focused() {
			border = canvas.theme.Accent
		}
		canvas.RoundedBorder(sc.rect, radius, 1, border)
	}
}

func (sc *SegmentedControl) indexAt(x, y float32) int {
	if !sc.rect.Contains(x, y) || len(sc.Options) == 0 {
		return -1
	}
	segW := maxFloat32(1, sc.rect.W/float32(len(sc.Options)))
	idx := int((x - sc.rect.X) / segW)
	return min(max(0, idx), len(sc.Options)-1)
}

func (sc *SegmentedControl) Handle(event Event) bool {
	if !sc.Enabled() || len(sc.Options) == 0 {
		return false
	}
	switch event.Type {
	case PointerMove:
		idx := sc.indexAt(event.X, event.Y)
		if idx != sc.hoveredIdx {
			sc.hoveredIdx = idx
			sc.InvalidatePaint()
		}
		return true
	case PointerDown:
		return event.Button == MouseLeft && sc.rect.Contains(event.X, event.Y)
	case PointerUp:
		if event.Button == MouseLeft && sc.Pressed() && sc.rect.Contains(event.X, event.Y) {
			if idx := sc.indexAt(event.X, event.Y); idx >= 0 {
				sc.SetSelected(sc.Options[idx].ID)
				return true
			}
		}
	case KeyDown:
		cur := max(0, sc.SelectedIndex())
		switch event.Key {
		case KeyRightArrow, KeyDownArrow:
			sc.SetSelected(sc.Options[(cur+1)%len(sc.Options)].ID)
			return true
		case KeyLeftArrow, KeyUpArrow:
			sc.SetSelected(sc.Options[(cur-1+len(sc.Options))%len(sc.Options)].ID)
			return true
		case KeyHome:
			sc.SetSelected(sc.Options[0].ID)
			return true
		case KeyEnd:
			sc.SetSelected(sc.Options[len(sc.Options)-1].ID)
			return true
		}
	}
	return sc.State.Handle(event)
}

// Separator draws a horizontal or vertical dividing line with an optional
// section label.
type Separator struct {
	State
	Direction Direction
	Text      string
	Color     Color
	Thickness float32
}

func NewSeparator(direction ...Direction) *Separator {
	dir := Horizontal
	if len(direction) > 0 && direction[0] == Vertical {
		dir = Vertical
	}
	sep := &Separator{Direction: dir, Thickness: 1}
	sep.owner = sep
	return sep
}

func NewLabeledSeparator(text string) *Separator {
	sep := NewSeparator(Horizontal)
	sep.Text = text
	return sep
}

func (sep *Separator) SetText(text string) {
	if sep.Text == text {
		return
	}
	sep.Text = text
	sep.invalidateLayout()
}

func (sep *Separator) measureTextSize() Vec2 {
	thick := maxFloat32(1, sep.Thickness)
	if sep.Direction == Vertical {
		return Vec2{X: maxFloat32(8, thick+6), Y: 24}
	}
	if sep.Text == "" {
		return Vec2{X: 32, Y: maxFloat32(10, thick+8)}
	}
	theme := sep.State.Theme()
	sz := sep.measureText(sep.Text, FontSpec{Size: theme.FontSize * 0.85, Weight: 600})
	return Vec2{X: sz.X + 48, Y: maxFloat32(20, sz.Y+6)}
}

func (sep *Separator) Measure(constraints Constraints) Vec2 {
	return measureWithState(&sep.State, sep.measureTextSize(), constraints)
}

func (sep *Separator) Paint(canvas *Canvas) {
	if sep.rect.Empty() {
		return
	}
	lineColor := sep.Color
	if lineColor.A <= 0 {
		lineColor = canvas.theme.Border
	}
	thick := maxFloat32(1, sep.Thickness)
	if sep.Direction == Vertical {
		midX := sep.rect.X + sep.rect.W*0.5
		canvas.Line(Vec2{X: midX, Y: sep.rect.Y + 2}, Vec2{X: midX, Y: sep.rect.Y + sep.rect.H - 2}, thick, lineColor)
		return
	}
	midY := sep.rect.Y + sep.rect.H*0.5
	if sep.Text == "" {
		canvas.Line(Vec2{X: sep.rect.X, Y: midY}, Vec2{X: sep.rect.X + sep.rect.W, Y: midY}, thick, lineColor)
		return
	}
	spec := FontSpec{Size: canvas.theme.FontSize * 0.85, Weight: 600}
	tw := sep.textWidth(sep.Text, spec) + 16
	leftW := maxFloat32(8, (sep.rect.W-tw)*0.5)
	if leftW > 2 {
		canvas.Line(Vec2{X: sep.rect.X, Y: midY}, Vec2{X: sep.rect.X + leftW - 4, Y: midY}, thick, lineColor)
	}
	rightX := sep.rect.X + leftW + tw
	if rightX+4 < sep.rect.X+sep.rect.W {
		canvas.Line(Vec2{X: rightX + 4, Y: midY}, Vec2{X: sep.rect.X + sep.rect.W, Y: midY}, thick, lineColor)
	}
	textRect := Rect{X: sep.rect.X + leftW, Y: sep.rect.Y, W: minFloat32(tw, sep.rect.W), H: sep.rect.H}
	canvas.TextStyled(sep.Text, textRect, spec, TextCenter, canvas.theme.MutedText)
}
