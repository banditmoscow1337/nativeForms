package nativeforms

type MenuItem struct {
	ID, Label string
	Shortcut  string
	Icon      *ImageResource
	Disabled  bool
	Separator bool
	Checked   bool
	Action    func()
}

type Menu struct {
	State
	Items    []MenuItem
	OnSelect func(string)
	// MaxVisibleItems bounds the menu height; the active item scrolls into view.
	// Zero shows all items.
	MaxVisibleItems     int
	active              int
	firstVisible        int
	ensureActiveVisible bool
	popup               PopupID
}

func NewMenu(items []MenuItem, onSelect func(string)) *Menu {
	m := &Menu{Items: append([]MenuItem(nil), items...), OnSelect: onSelect, active: -1, ensureActiveVisible: true}
	m.owner, m.focusable, m.clip = m, true, true
	for i, item := range m.Items {
		if !item.Disabled && !item.Separator {
			m.active = i
			break
		}
	}
	return m
}

func (m *Menu) hasIconsOrChecks() bool {
	for _, item := range m.Items {
		if item.Icon != nil || item.Checked {
			return true
		}
	}
	return false
}

func (m *Menu) Measure(c Constraints) Vec2 {
	w := float32(140)
	if m.autoSize {
		w = 0
	}
	iconSpace := float32(0)
	if m.hasIconsOrChecks() {
		iconSpace = iconSize + iconGap
	}
	spec := FontSpec{Size: m.Theme().FontSize}
	for _, item := range m.Items {
		if item.Separator {
			continue
		}
		itemW := m.measureText(item.Label, spec).X + 32 + iconSpace
		if item.Shortcut != "" {
			itemW += m.measureText(item.Shortcut, spec).X + 24
		}
		w = maxFloat32(w, itemW)
	}
	rows := len(m.Items)
	if m.MaxVisibleItems > 0 {
		rows = min(rows, m.MaxVisibleItems)
	}
	return measureWithState(&m.State, Vec2{X: w, Y: float32(rows)*m.Theme().ControlHeight + 8}, c)
}

func (m *Menu) visibleRows() int {
	rows := len(m.Items)
	if m.MaxVisibleItems > 0 {
		rows = min(rows, m.MaxVisibleItems)
	}
	if m.rect.H > 0 {
		rows = min(rows, max(1, int((m.rect.H-8)/m.Theme().ControlHeight)))
	}
	return rows
}

func (m *Menu) showActive() {
	rows := m.visibleRows()
	if rows == 0 || m.active < 0 {
		return
	}
	if m.active < m.firstVisible {
		m.firstVisible = m.active
	} else if m.active >= m.firstVisible+rows {
		m.firstVisible = m.active - rows + 1
	}
	m.firstVisible = min(m.firstVisible, max(0, len(m.Items)-rows))
}

func (m *Menu) Paint(c *Canvas) {
	if m.ensureActiveVisible {
		m.showActive()
		m.ensureActiveVisible = false
	}
	radius := m.effectiveCornerRadius(c.theme)
	// Guarantee 100% opaque surface and border for dropdown & context menus
	bg := c.theme.PanelAlt
	bg.A = 1
	border := c.theme.Border
	border.A = 1
	sel := c.theme.Selection
	sel.A = 1
	c.RoundedRect(m.rect, radius, bg)
	c.RoundedBorder(m.rect, radius, 1, border)
	icons := m.hasIconsOrChecks()
	h := c.theme.ControlHeight
	itemRadius := minFloat32(radius, 6)
	for row := 0; row < m.visibleRows(); row++ {
		i := m.firstVisible + row
		if i >= len(m.Items) {
			break
		}
		item := m.Items[i]
		r := Rect{X: m.rect.X + 4, Y: m.rect.Y + 4 + float32(row)*h, W: m.rect.W - 8, H: h}
		if item.Separator {
			midY := r.Y + r.H*0.5
			c.Line(Vec2{X: r.X + 8, Y: midY}, Vec2{X: r.X + r.W - 8, Y: midY}, 1, border)
			continue
		}
		if i == m.active {
			c.RoundedRect(r, itemRadius, sel)
		}
		color := c.theme.Text
		if item.Disabled {
			color = c.theme.Disabled
		}
		inner := r.Inset(Symmetric(10, 0))
		if item.Checked && item.Icon == nil {
			chk := Rect{X: inner.X + 4, Y: inner.Y + (inner.H-8)*0.5, W: 8, H: 8}
			c.RoundedRect(chk, 4, c.theme.Accent)
		}
		labelRect := inner
		if item.Shortcut != "" {
			sw := m.measureText(item.Shortcut, FontSpec{Size: c.theme.FontSize}).X + 12
			labelRect.W = maxFloat32(0, inner.W-sw)
			scRect := Rect{X: inner.X + labelRect.W, Y: inner.Y, W: maxFloat32(0, inner.W-labelRect.W), H: inner.H}
			c.Text(item.Shortcut, scRect, c.theme.FontSize*0.9, TextRight, c.theme.MutedText)
		}
		paintIconLabel(c, item.Icon, item.Label, labelRect, icons, color, enabledOpacity(!item.Disabled))
	}
}

func (m *Menu) indexAt(x, y float32) int {
	if !m.rect.Contains(x, y) {
		return -1
	}
	row := int((y - m.rect.Y - 4) / m.Theme().ControlHeight)
	if y < m.rect.Y+4 || row < 0 || row >= m.visibleRows() || m.firstVisible+row >= len(m.Items) {
		return -1
	}
	return m.firstVisible + row
}

func (m *Menu) activate() bool {
	if m.active < 0 || m.active >= len(m.Items) || m.Items[m.active].Disabled || m.Items[m.active].Separator {
		return false
	}
	item := m.Items[m.active]
	if m.manager != nil && m.popup != 0 {
		m.manager.ClosePopup(m.popup)
	}
	if item.Action != nil {
		item.Action()
	}
	if m.OnSelect != nil {
		m.OnSelect(item.ID)
	}
	return true
}

func (m *Menu) selectEnabledFrom(start, step int, wrap bool) {
	n := len(m.Items)
	for count := 0; count < n; count++ {
		idx := start + count*step
		if wrap {
			idx = (idx%n + n) % n
		} else if idx < 0 || idx >= n {
			return
		}
		if !m.Items[idx].Disabled && !m.Items[idx].Separator {
			m.active = idx
			m.showActive()
			m.InvalidatePaint()
			return
		}
	}
}

func (m *Menu) Handle(e Event) bool {
	switch e.Type {
	case PointerScroll:
		delta := e.Scroll.Y
		if delta == 0 {
			delta = e.Scroll.X
		}
		rows := m.visibleRows()
		if delta != 0 && rows > 0 {
			step := max(1, int(absFloat32(delta)))
			if delta > 0 {
				step = -step
			}
			before := m.firstVisible
			m.firstVisible = min(max(0, m.firstVisible+step), max(0, len(m.Items)-rows))
			m.ensureActiveVisible = false
			if m.firstVisible != before {
				m.InvalidatePaint()
			}
			return true
		}
	case PointerMove:
		i := m.indexAt(e.X, e.Y)
		if i >= 0 && (m.Items[i].Disabled || m.Items[i].Separator) {
			i = -1
		}
		if i != m.active {
			m.active = i
			m.InvalidatePaint()
		}
		return true
	case PointerDown:
		return e.Button == MouseLeft || e.Button == MouseRight
	case PointerUp:
		if (e.Button == MouseLeft || e.Button == MouseRight) && m.rect.Contains(e.X, e.Y) {
			m.active = m.indexAt(e.X, e.Y)
			m.activate()
			return true
		}
	case KeyDown:
		switch e.Key {
		case KeyUpArrow, KeyDownArrow:
			if len(m.Items) == 0 {
				return true
			}
			step := 1
			if e.Key == KeyUpArrow {
				step = -1
			}
			start := m.active
			if start < 0 && step < 0 {
				start = 0
			}
			m.selectEnabledFrom(start+step, step, true)
			return true
		case KeyHome:
			m.selectEnabledFrom(0, 1, false)
			return true
		case KeyEnd:
			m.selectEnabledFrom(len(m.Items)-1, -1, false)
			return true
		case KeyEnter, KeySpace:
			m.activate()
			return true
		}
	}
	return m.State.Handle(e)
}

func (manager *Manager) OpenMenu(items []MenuItem, bounds Rect) PopupID {
	for i := len(manager.popups) - 1; i >= 0; i-- {
		if _, ok := manager.popups[i].content.(*Menu); ok && !manager.popups[i].modal {
			manager.ClosePopup(manager.popups[i].id)
		}
	}
	m := NewMenu(items, nil)
	return manager.OpenPopup(m, bounds, false)
}

func (manager *Manager) OpenContextMenu(items []MenuItem, x, y float32) PopupID {
	return manager.OpenMenu(items, Rect{X: x, Y: y})
}
