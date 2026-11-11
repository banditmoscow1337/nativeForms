package nativeforms

// MenuBarItem describes one top-level pull-down menu or command on a MenuBar.
type MenuBarItem struct {
	ID       string
	Label    string
	Icon     *ImageResource
	Disabled bool
	Items    []MenuItem
	Action   func()
}

// MenuBar renders a horizontal application menu bar with pull-down menus,
// hover-switching between menus while open, and keyboard navigation.
type MenuBar struct {
	State
	Menus       []MenuBarItem
	Height      float32
	activeIndex int
	hoveredIdx  int
	popupID     PopupID
}

func NewMenuBar(menus ...MenuBarItem) *MenuBar {
	bar := &MenuBar{
		Menus:       append([]MenuBarItem(nil), menus...),
		Height:      34,
		activeIndex: -1,
		hoveredIdx:  -1,
	}
	bar.owner = bar
	bar.focusable = true
	bar.clip = true
	bar.minimum = Vec2{Y: 34}
	bar.SetFlexShrink(0)
	return bar
}

func (bar *MenuBar) ActiveIndex() int { return bar.activeIndex }

func (bar *MenuBar) SetMenus(menus []MenuBarItem) {
	bar.CloseMenu()
	bar.Menus = append(bar.Menus[:0], menus...)
	bar.invalidateLayout()
}

func (bar *MenuBar) AddMenu(menu MenuBarItem) {
	bar.Menus = append(bar.Menus, menu)
	bar.invalidateLayout()
}

func (bar *MenuBar) barHeight() float32 {
	if bar.Height > 0 {
		return bar.Height
	}
	return 34
}

func (bar *MenuBar) itemWidth(item MenuBarItem) float32 {
	w := bar.measureText(item.Label, FontSpec{Size: bar.Theme().FontSize * 0.95}).X + 24 + iconWidth(item.Icon)
	return maxFloat32(56, w)
}

func (bar *MenuBar) Measure(c Constraints) Vec2 {
	h := bar.barHeight()
	if bar.minimum.Y < h {
		bar.minimum.Y = h
	}
	w := float32(8)
	for _, item := range bar.Menus {
		w += bar.itemWidth(item) + 2
	}
	return measureWithState(&bar.State, Vec2{X: w, Y: h}, c)
}

func (bar *MenuBar) itemAt(x, y float32) (int, Rect) {
	if !bar.rect.Contains(x, y) {
		return -1, Rect{}
	}
	curX := bar.rect.X + 4
	h := bar.rect.H
	for i, item := range bar.Menus {
		w := bar.itemWidth(item)
		r := Rect{X: curX, Y: bar.rect.Y + 2, W: w, H: maxFloat32(0, h-4)}
		if x >= curX && x < curX+w {
			return i, r
		}
		curX += w + 2
	}
	return -1, Rect{}
}

func (bar *MenuBar) itemRect(index int) Rect {
	if index < 0 || index >= len(bar.Menus) {
		return Rect{}
	}
	curX := bar.rect.X + 4
	h := bar.rect.H
	for i, item := range bar.Menus {
		w := bar.itemWidth(item)
		if i == index {
			return Rect{X: curX, Y: bar.rect.Y, W: w, H: h}
		}
		curX += w + 2
	}
	return Rect{}
}

func (bar *MenuBar) CloseMenu() {
	if bar.popupID != 0 && bar.manager != nil {
		id := bar.popupID
		bar.popupID = 0
		bar.activeIndex = -1
		bar.manager.ClosePopup(id)
		bar.InvalidatePaint()
	} else {
		bar.activeIndex = -1
		bar.popupID = 0
	}
}

func (bar *MenuBar) isPopupAlive() bool {
	if bar.popupID == 0 || bar.manager == nil {
		return false
	}
	for _, p := range bar.manager.popups {
		if p.id == bar.popupID {
			return true
		}
	}
	bar.popupID = 0
	bar.activeIndex = -1
	return false
}

func (bar *MenuBar) OpenMenuAt(index int) bool {
	if index < 0 || index >= len(bar.Menus) || bar.Menus[index].Disabled || bar.manager == nil {
		return false
	}
	item := bar.Menus[index]
	if len(item.Items) == 0 {
		bar.CloseMenu()
		if item.Action != nil {
			item.Action()
		}
		return true
	}
	if bar.isPopupAlive() && bar.activeIndex == index {
		return true
	}
	bar.CloseMenu()
	bar.activeIndex = index
	r := bar.itemRect(index)
	m := NewMenu(item.Items, func(_ string) {
		bar.activeIndex = -1
		bar.popupID = 0
		bar.InvalidatePaint()
	})
	m.SetEventHandler(func(ev Event) bool {
		if ev.Type == KeyDown {
			if ev.Key == KeyLeftArrow {
				bar.OpenMenuAt((index - 1 + len(bar.Menus)) % len(bar.Menus))
				return true
			}
			if ev.Key == KeyRightArrow {
				bar.OpenMenuAt((index + 1) % len(bar.Menus))
				return true
			}
		}
		return false
	})
	bar.popupID = bar.manager.OpenPopup(m, Rect{X: r.X, Y: r.Y + r.H}, false)
	bar.InvalidatePaint()
	return true
}

func (bar *MenuBar) Paint(c *Canvas) {
	if bar.rect.Empty() {
		return
	}
	_ = bar.isPopupAlive()
	if !bar.Frameless() {
		c.Rect(bar.rect, c.theme.Header)
		c.Line(Vec2{X: bar.rect.X, Y: bar.rect.Y + bar.rect.H - 0.5}, Vec2{X: bar.rect.X + bar.rect.W, Y: bar.rect.Y + bar.rect.H - 0.5}, 1, c.theme.Border)
	}
	curX := bar.rect.X + 4
	h := bar.rect.H
	fontSize := c.theme.FontSize * 0.95
	for i, item := range bar.Menus {
		w := bar.itemWidth(item)
		r := Rect{X: curX, Y: bar.rect.Y + 2, W: w, H: maxFloat32(0, h-4)}
		if i == bar.activeIndex {
			c.RoundedRect(r, 4, c.theme.Selection)
		} else if i == bar.hoveredIdx && bar.Hovered() && !item.Disabled {
			c.RoundedRect(r, 4, c.theme.ControlHover)
		}
		color := c.theme.Text
		if item.Disabled {
			color = c.theme.Disabled
		}
		inner := r.Inset(Symmetric(10, 0))
		if item.Icon != nil {
			paintIconLabel(c, item.Icon, item.Label, inner, true, color, enabledOpacity(!item.Disabled))
		} else {
			c.TextStyled(item.Label, inner, FontSpec{Size: fontSize, Weight: 500}, TextCenter, color)
		}
		curX += w + 2
	}
}

func (bar *MenuBar) Handle(e Event) bool {
	if !bar.Enabled() {
		return false
	}
	switch e.Type {
	case PointerMove:
		idx, _ := bar.itemAt(e.X, e.Y)
		if idx != bar.hoveredIdx {
			bar.hoveredIdx = idx
			bar.InvalidatePaint()
		}
		if bar.isPopupAlive() && idx >= 0 && idx != bar.activeIndex && !bar.Menus[idx].Disabled {
			bar.OpenMenuAt(idx)
		}
		return idx >= 0
	case PointerDown:
		if e.Button == MouseLeft {
			idx, _ := bar.itemAt(e.X, e.Y)
			if idx >= 0 && !bar.Menus[idx].Disabled {
				if bar.isPopupAlive() && bar.activeIndex == idx {
					bar.CloseMenu()
				} else {
					bar.OpenMenuAt(idx)
				}
				return true
			}
		}
	case KeyDown:
		if len(bar.Menus) == 0 {
			break
		}
		switch e.Key {
		case KeyRightArrow:
			return bar.OpenMenuAt((max(0, bar.activeIndex) + 1) % len(bar.Menus))
		case KeyLeftArrow:
			return bar.OpenMenuAt((max(0, bar.activeIndex) - 1 + len(bar.Menus)) % len(bar.Menus))
		case KeyDownArrow, KeyEnter, KeySpace:
			target := bar.activeIndex
			if target < 0 {
				target = max(0, bar.hoveredIdx)
			}
			return bar.OpenMenuAt(target)
		case KeyEscape:
			if bar.isPopupAlive() {
				bar.CloseMenu()
				return true
			}
		}
	}
	return bar.State.Handle(e)
}
