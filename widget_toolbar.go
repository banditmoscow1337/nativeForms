package nativeforms

// ToolbarItem describes an action button, toggle button, separator, or flexible
// spacer inside a Toolbar.
type ToolbarItem struct {
	ID        string
	Label     string
	Tooltip   string
	Icon      *ImageResource
	Separator bool
	Spacer    bool
	Toggle    bool
	Active    bool
	Disabled  bool
	Action    func()
	OnToggle  func(bool)
}

// Toolbar renders a horizontal action bar with compact icon/text buttons,
// toggle buttons, separators, and spacers.
type Toolbar struct {
	State
	Items      []ToolbarItem
	Height     float32
	hoveredIdx int
	pressedIdx int
}

func NewToolbar(items ...ToolbarItem) *Toolbar {
	tb := &Toolbar{
		Items:      append([]ToolbarItem(nil), items...),
		Height:     38,
		hoveredIdx: -1,
		pressedIdx: -1,
	}
	tb.owner = tb
	tb.focusable = true
	tb.clip = true
	tb.minimum = Vec2{Y: 38}
	tb.SetFlexShrink(0)
	return tb
}

func (tb *Toolbar) barHeight() float32 {
	if tb.Height > 0 {
		return tb.Height
	}
	return 38
}

func (tb *Toolbar) SetItems(items []ToolbarItem) {
	tb.Items = append(tb.Items[:0], items...)
	tb.hoveredIdx, tb.pressedIdx = -1, -1
	tb.invalidateLayout()
}

func (tb *Toolbar) AddItem(item ToolbarItem) {
	tb.Items = append(tb.Items, item)
	tb.invalidateLayout()
}

func (tb *Toolbar) SetItemActive(id string, active bool) bool {
	for i := range tb.Items {
		if tb.Items[i].ID == id {
			if tb.Items[i].Active == active {
				return false
			}
			tb.Items[i].Active = active
			tb.InvalidatePaint()
			return true
		}
	}
	return false
}

func (tb *Toolbar) SetItemDisabled(id string, disabled bool) bool {
	for i := range tb.Items {
		if tb.Items[i].ID == id {
			if tb.Items[i].Disabled == disabled {
				return false
			}
			tb.Items[i].Disabled = disabled
			tb.InvalidatePaint()
			return true
		}
	}
	return false
}

func (tb *Toolbar) itemWidth(item ToolbarItem) float32 {
	if item.Separator {
		return 10
	}
	if item.Spacer {
		return 16
	}
	fontSize := tb.Theme().FontSize * 0.9
	tw := float32(0)
	if item.Label != "" {
		tw = tb.textWidth(item.Label, FontSpec{Size: fontSize})
	}
	iw := float32(0)
	if item.Icon != nil {
		iw = iconSize
		if item.Label != "" {
			iw += iconGap
		}
	}
	return maxFloat32(32, tw+iw+18)
}

func (tb *Toolbar) Measure(constraints Constraints) Vec2 {
	h := tb.barHeight()
	if tb.minimum.Y < h {
		tb.minimum.Y = h
	}
	w := float32(12)
	for _, item := range tb.Items {
		w += tb.itemWidth(item) + 4
	}
	return measureWithState(&tb.State, Vec2{X: w, Y: h}, constraints)
}

func (tb *Toolbar) itemRectAt(index int) Rect {
	if index < 0 || index >= len(tb.Items) {
		return Rect{}
	}
	spacers := 0
	fixedW := float32(12)
	for _, item := range tb.Items {
		if item.Spacer {
			spacers++
		} else {
			fixedW += tb.itemWidth(item) + 4
		}
	}
	spacerW := float32(16)
	if spacers > 0 && tb.rect.W > fixedW {
		spacerW = (tb.rect.W - fixedW) / float32(spacers)
	}
	curX := tb.rect.X + 6
	for i, item := range tb.Items {
		w := tb.itemWidth(item)
		if item.Spacer {
			w = spacerW
		}
		if i == index {
			return Rect{X: curX, Y: tb.rect.Y + 4, W: w, H: maxFloat32(0, tb.rect.H-8)}
		}
		curX += w + 4
	}
	return Rect{}
}

func (tb *Toolbar) indexAt(x, y float32) int {
	if !tb.rect.Contains(x, y) {
		return -1
	}
	for i, item := range tb.Items {
		if item.Separator || item.Spacer {
			continue
		}
		if tb.itemRectAt(i).Contains(x, y) {
			return i
		}
	}
	return -1
}

func (tb *Toolbar) activateIndex(index int) bool {
	if index < 0 || index >= len(tb.Items) {
		return false
	}
	item := &tb.Items[index]
	if item.Disabled || item.Separator || item.Spacer {
		return false
	}
	if item.Toggle {
		item.Active = !item.Active
		tb.InvalidatePaint()
		if item.OnToggle != nil {
			item.OnToggle(item.Active)
		}
	}
	if item.Action != nil {
		item.Action()
	}
	return true
}

func (tb *Toolbar) Paint(canvas *Canvas) {
	if tb.rect.Empty() {
		return
	}
	radius := tb.effectiveCornerRadius(canvas.theme)
	if !tb.Frameless() {
		canvas.RoundedRect(tb.rect, radius, canvas.theme.Header)
		canvas.RoundedBorder(tb.rect, radius, 1, canvas.theme.Border)
	}
	fontSize := canvas.theme.FontSize * 0.9
	btnRadius := maxFloat32(4, radius-2)
	for i, item := range tb.Items {
		r := tb.itemRectAt(i)
		if item.Spacer {
			continue
		}
		if item.Separator {
			midX := r.X + r.W*0.5
			canvas.Line(Vec2{X: midX, Y: r.Y + 3}, Vec2{X: midX, Y: r.Y + r.H - 3}, 1, canvas.theme.Border)
			continue
		}
		if !item.Disabled {
			if i == tb.pressedIdx && tb.Pressed() {
				canvas.RoundedRect(r, btnRadius, canvas.theme.ControlPressed)
			} else if item.Active {
				canvas.RoundedRect(r, btnRadius, canvas.theme.Selection)
				canvas.RoundedBorder(r, btnRadius, 1, canvas.theme.Accent)
			} else if i == tb.hoveredIdx && tb.Hovered() {
				canvas.RoundedRect(r, btnRadius, canvas.theme.ControlHover)
			}
		}
		color := canvas.theme.Text
		if item.Disabled {
			color = canvas.theme.Disabled
		} else if item.Active {
			color = canvas.theme.AccentHover
		}
		inner := r.Inset(Symmetric(8, 0))
		if item.Icon != nil && item.Label == "" {
			paintIcon(canvas, item.Icon, Rect{X: r.X + (r.W-iconSize)*0.5, Y: r.Y + (r.H-iconSize)*0.5, W: iconSize, H: iconSize}, enabledOpacity(!item.Disabled))
		} else if item.Icon != nil {
			paintIconLabel(canvas, item.Icon, item.Label, inner, true, color, enabledOpacity(!item.Disabled))
		} else {
			canvas.TextStyled(item.Label, inner, FontSpec{Size: fontSize, Weight: 500}, TextCenter, color)
		}
	}
}

func (tb *Toolbar) Handle(event Event) bool {
	if !tb.Enabled() {
		return false
	}
	switch event.Type {
	case PointerMove:
		idx := tb.indexAt(event.X, event.Y)
		if idx != tb.hoveredIdx {
			tb.hoveredIdx = idx
			if idx >= 0 && tb.Items[idx].Tooltip != "" {
				tb.SetTooltip(tb.Items[idx].Tooltip)
			}
			tb.InvalidatePaint()
		}
		return idx >= 0
	case PointerDown:
		if event.Button == MouseLeft {
			idx := tb.indexAt(event.X, event.Y)
			if idx >= 0 && !tb.Items[idx].Disabled {
				tb.pressedIdx = idx
				tb.InvalidatePaint()
				return true
			}
		}
	case PointerUp:
		if event.Button == MouseLeft && tb.pressedIdx >= 0 {
			pressed := tb.pressedIdx
			tb.pressedIdx = -1
			tb.InvalidatePaint()
			if tb.indexAt(event.X, event.Y) == pressed {
				return tb.activateIndex(pressed)
			}
			return true
		}
	case KeyDown:
		if !event.Repeat && (event.Key == KeyEnter || event.Key == KeySpace) {
			target := tb.hoveredIdx
			if target < 0 {
				for i, item := range tb.Items {
					if !item.Separator && !item.Spacer && !item.Disabled {
						target = i
						break
					}
				}
			}
			return tb.activateIndex(target)
		}
	}
	return tb.State.Handle(event)
}
