package nativeforms

type Checkbox struct {
	State
	Text     string
	Icon     *ImageResource
	Checked  bool
	OnChange func(bool)
}

func NewCheckbox(text string, checked bool, onChange func(bool)) *Checkbox {
	c := &Checkbox{Text: text, Checked: checked, OnChange: onChange}
	c.owner, c.focusable = c, true
	return c
}

func (c *Checkbox) SetChecked(value bool) {
	if c.Checked == value {
		return
	}
	c.Checked = value
	c.invalidatePaint()
	if c.OnChange != nil {
		c.OnChange(value)
	}
}

func (c *Checkbox) SetIcon(icon *ImageResource) {
	if c.Icon != icon {
		c.Icon = icon
		c.invalidateLayout()
	}
}

func (c *Checkbox) Measure(limits Constraints) Vec2 {
	size := c.measureText(c.Text, FontSpec{Size: c.Theme().FontSize})
	return measureWithState(&c.State, Vec2{X: size.X + 34 + iconWidth(c.Icon), Y: maxFloat32(size.Y+12, c.Theme().ControlHeight)}, limits)
}

func (c *Checkbox) Paint(canvas *Canvas) {
	r := Rect{X: c.rect.X + 6, Y: c.rect.Y + (c.rect.H-18)/2, W: 18, H: 18}
	canvas.Rect(r, canvas.theme.Control)
	canvas.Border(r, 1, canvas.theme.Border)
	if c.Checked {
		canvas.Rect(r.Inset(All(4)), canvas.theme.Accent)
	}
	if c.Focused() {
		canvas.Border(c.rect, 1, canvas.theme.Accent)
	}
	opacity := float32(1)
	if !c.Enabled() {
		opacity = 0.5
	}
	paintIconLabel(canvas, c.Icon, c.Text, Rect{X: r.X + 26, Y: c.rect.Y, W: maxFloat32(0, c.rect.W-34), H: c.rect.H}, c.Icon != nil, canvas.theme.Text, opacity)
}

func (c *Checkbox) Handle(e Event) bool {
	if !c.Enabled() {
		return false
	}
	if e.Type == PointerDown && e.Button == MouseLeft {
		return true
	}
	if (e.Type == PointerUp && e.Button == MouseLeft && c.Pressed() && c.rect.Contains(e.X, e.Y)) ||
		(e.Type == KeyDown && !e.Repeat && (e.Key == KeySpace || e.Key == KeyEnter)) {
		c.SetChecked(!c.Checked)
		return true
	}
	return c.State.Handle(e)
}

type RadioOption struct {
	ID, Label string
	Icon      *ImageResource
}

type RadioGroup struct {
	State
	Options    []RadioOption
	SelectedID string
	OnChange   func(string)
}

func NewRadioGroup(options []RadioOption, selected string, onChange func(string)) *RadioGroup {
	r := &RadioGroup{Options: append([]RadioOption(nil), options...), SelectedID: selected, OnChange: onChange}
	r.owner, r.focusable = r, true
	return r
}

func (r *RadioGroup) SetSelected(id string) {
	for _, option := range r.Options {
		if option.ID == id {
			if id != r.SelectedID {
				r.SelectedID = id
				r.invalidatePaint()
				if r.OnChange != nil {
					r.OnChange(id)
				}
			}
			return
		}
	}
}

func (r *RadioGroup) Measure(c Constraints) Vec2 {
	w := float32(90)
	icons := false
	for _, option := range r.Options {
		icons = icons || option.Icon != nil
	}
	iconSpace := float32(0)
	if icons {
		iconSpace = iconSize + iconGap
	}
	for _, option := range r.Options {
		w = maxFloat32(w, r.measureText(option.Label, FontSpec{Size: r.Theme().FontSize}).X+38+iconSpace)
	}
	return measureWithState(&r.State, Vec2{X: w, Y: float32(len(r.Options)) * r.Theme().ControlHeight}, c)
}

func (r *RadioGroup) Paint(c *Canvas) {
	h := r.Theme().ControlHeight
	icons := false
	for _, option := range r.Options {
		icons = icons || option.Icon != nil
	}
	for i, option := range r.Options {
		y := r.rect.Y + float32(i)*h
		if y >= r.rect.Y+r.rect.H {
			break
		}
		box := Rect{X: r.rect.X + 8, Y: y + (h-18)/2, W: 18, H: 18}
		c.Border(box, 1, c.theme.Border)
		if option.ID == r.SelectedID {
			c.Rect(box.Inset(All(5)), c.theme.Accent)
		}
		paintIconLabel(c, option.Icon, option.Label, Rect{X: box.X + 27, Y: y, W: maxFloat32(0, r.rect.W-37), H: h}, icons, c.theme.Text, 1)
	}
	if r.Focused() {
		c.Border(r.rect, 1, c.theme.Accent)
	}
}

func (r *RadioGroup) Handle(e Event) bool {
	if !r.Enabled() || len(r.Options) == 0 {
		return false
	}
	if e.Type == PointerDown && e.Button == MouseLeft {
		return true
	}
	if e.Type == PointerUp && e.Button == MouseLeft && r.Pressed() && r.rect.Contains(e.X, e.Y) {
		i := int((e.Y - r.rect.Y) / r.Theme().ControlHeight)
		if i >= 0 && i < len(r.Options) {
			r.SetSelected(r.Options[i].ID)
			return true
		}
	}
	if e.Type == KeyDown && (e.Key == KeyDownArrow || e.Key == KeyUpArrow) {
		i := 0
		for j, o := range r.Options {
			if o.ID == r.SelectedID {
				i = j
				break
			}
		}
		if e.Key == KeyDownArrow {
			i = (i + 1) % len(r.Options)
		} else {
			i = (i - 1 + len(r.Options)) % len(r.Options)
		}
		r.SetSelected(r.Options[i].ID)
		return true
	}
	return r.State.Handle(e)
}

type MenuItem struct {
	ID, Label string
	Icon     *ImageResource
	Disabled  bool
	Action    func()
}

type Menu struct {
	State
	Items    []MenuItem
	OnSelect func(string)
	// MaxVisibleItems bounds the menu height; the active item scrolls into view.
	// Zero shows all items.
	MaxVisibleItems int
	active   int
	firstVisible int
	popup    PopupID
}

func NewMenu(items []MenuItem, onSelect func(string)) *Menu {
	m := &Menu{Items: append([]MenuItem(nil), items...), OnSelect: onSelect, active: -1}
	m.owner, m.focusable, m.clip = m, true, true
	for i, item := range m.Items {
		if !item.Disabled {
			m.active = i
			break
		}
	}
	return m
}

func (m *Menu) Measure(c Constraints) Vec2 {
	w := float32(140)
	iconSpace := float32(0)
	for _, item := range m.Items {
		if item.Icon != nil {
			iconSpace = iconSize + iconGap
			break
		}
	}
	for _, item := range m.Items {
		w = maxFloat32(w, m.measureText(item.Label, FontSpec{Size: m.Theme().FontSize}).X+32+iconSpace)
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
		m.firstVisible = m.active-rows+1
	}
	m.firstVisible = min(m.firstVisible, max(0, len(m.Items)-rows))
}

func (m *Menu) Paint(c *Canvas) {
	m.showActive()
	c.Rect(m.rect, c.theme.PanelAlt)
	c.Border(m.rect, 1, c.theme.Border)
	icons := false
	for _, item := range m.Items {
		icons = icons || item.Icon != nil
	}
	h := c.theme.ControlHeight
	for row := 0; row < m.visibleRows(); row++ {
		i := m.firstVisible+row
		if i >= len(m.Items) {
			break
		}
		item := m.Items[i]
		r := Rect{X: m.rect.X + 4, Y: m.rect.Y + 4 + float32(row)*h, W: m.rect.W - 8, H: h}
		if i == m.active {
			c.Rect(r, c.theme.Selection)
		}
		color := c.theme.Text
		if item.Disabled {
			color = c.theme.Disabled
		}
		opacity := float32(1)
		if item.Disabled {
			opacity = 0.5
		}
		paintIconLabel(c, item.Icon, item.Label, r.Inset(Symmetric(10, 0)), icons, color, opacity)
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
	return m.firstVisible+row
}

func (m *Menu) activate() bool {
	if m.active < 0 || m.active >= len(m.Items) || m.Items[m.active].Disabled {
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

func (m *Menu) Handle(e Event) bool {
	switch e.Type {
	case PointerMove:
		i := m.indexAt(e.X, e.Y)
		if i >= 0 && m.Items[i].Disabled {
			i = -1
		}
		if i != m.active {
			m.active = i
			m.invalidatePaint()
		}
		return true
	case PointerDown:
		return e.Button == MouseLeft
	case PointerUp:
		if e.Button == MouseLeft && m.rect.Contains(e.X, e.Y) {
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
			index := m.active
			if index < 0 && step < 0 {
				index = 0
			}
			for n := 0; n < len(m.Items); n++ {
				index = (index + step + len(m.Items)) % len(m.Items)
				if !m.Items[index].Disabled {
					m.active = index
					m.showActive()
					m.invalidatePaint()
					break
				}
			}
			return true
		case KeyHome, KeyEnd:
			start, step := 0, 1
			if e.Key == KeyEnd {
				start, step = len(m.Items)-1, -1
			}
			for i := start; i >= 0 && i < len(m.Items); i += step {
				if !m.Items[i].Disabled {
					m.active = i
					m.showActive()
					m.invalidatePaint()
					break
				}
			}
			return true
		case KeyEnter, KeySpace:
			m.activate()
			return true
		}
	}
	return m.State.Handle(e)
}

func (manager *Manager) OpenMenu(items []MenuItem, bounds Rect) PopupID {
	m := NewMenu(items, nil)
	return manager.OpenPopup(m, bounds, false)
}

func (manager *Manager) OpenContextMenu(items []MenuItem, x, y float32) PopupID {
	return manager.OpenMenu(items, Rect{X: x, Y: y})
}

type ComboBox struct {
	State
	Options    []RadioOption
	SelectedID string
	OnChange   func(string)
}

func NewComboBox(options []RadioOption, selected string, onChange func(string)) *ComboBox {
	c := &ComboBox{Options: append([]RadioOption(nil), options...), SelectedID: selected, OnChange: onChange}
	c.owner, c.focusable = c, true
	return c
}

func (c *ComboBox) SetSelected(id string) {
	for _, option := range c.Options {
		if option.ID == id {
			if c.SelectedID != id {
				c.SelectedID = id
				c.invalidatePaint()
				if c.OnChange != nil {
					c.OnChange(id)
				}
			}
			return
		}
	}
}

func (c *ComboBox) Measure(limits Constraints) Vec2 {
	w := float32(140)
	iconSpace := float32(0)
	for _, option := range c.Options {
		if option.Icon != nil {
			iconSpace = iconSize + iconGap
			break
		}
	}
	for _, option := range c.Options {
		w = maxFloat32(w, c.measureText(option.Label, FontSpec{Size: c.Theme().FontSize}).X+40+iconSpace)
	}
	return measureWithState(&c.State, Vec2{X: w, Y: c.Theme().ControlHeight}, limits)
}

func (c *ComboBox) Paint(canvas *Canvas) {
	canvas.Rect(c.rect, canvas.theme.Control)
	border := canvas.theme.Border
	if c.Focused() {
		border = canvas.theme.Accent
	}
	canvas.Border(c.rect, 1, border)
	label := ""
	var icon *ImageResource
	icons := false
	for _, option := range c.Options {
		icons = icons || option.Icon != nil
		if option.ID == c.SelectedID {
			label = option.Label
			icon = option.Icon
		}
	}
	paintIconLabel(canvas, icon, label, Rect{X: c.rect.X + 10, Y: c.rect.Y, W: maxFloat32(0, c.rect.W-38), H: c.rect.H}, icons, canvas.theme.Text, 1)
	paintChevron(canvas, Rect{X: c.rect.X + c.rect.W - 26, Y: c.rect.Y, W: 18, H: c.rect.H}, true, canvas.theme.Text)
}

func (c *ComboBox) open() bool {
	if c.manager == nil || len(c.Options) == 0 {
		return false
	}
	items := make([]MenuItem, 0, len(c.Options))
	for _, option := range c.Options {
		items = append(items, MenuItem{ID: option.ID, Label: option.Label, Icon: option.Icon})
	}
	m := NewMenu(items, c.SetSelected)
	for i, option := range c.Options {
		if option.ID == c.SelectedID {
			m.active = i
			break
		}
	}
	c.manager.OpenPopup(m, Rect{X: c.rect.X, Y: c.rect.Y + c.rect.H, W: c.rect.W}, false)
	return true
}

func (c *ComboBox) Handle(e Event) bool {
	if !c.Enabled() {
		return false
	}
	if e.Type == PointerDown && e.Button == MouseLeft {
		return true
	}
	if (e.Type == PointerUp && e.Button == MouseLeft && c.Pressed() && c.rect.Contains(e.X, e.Y)) ||
		(e.Type == KeyDown && !e.Repeat && (e.Key == KeySpace || e.Key == KeyEnter || e.Key == KeyDownArrow)) {
		return c.open()
	}
	return c.State.Handle(e)
}

type Tab struct {
	ID, Title string
	Icon      *ImageResource
	Content   Component
}

type Tabs struct {
	State
	Tabs         []Tab
	SelectedID   string
	OnChange     func(string)
	headerHeight float32
}

func NewTabs(tabs []Tab, selected string, onChange func(string)) *Tabs {
	t := &Tabs{Tabs: append([]Tab(nil), tabs...), SelectedID: selected, OnChange: onChange, headerHeight: 38}
	t.owner, t.focusable = t, true
	for _, tab := range t.Tabs {
		if tab.Content != nil {
			t.Add(tab.Content)
		}
	}
	t.selectTab(selected, false)
	return t
}

func (t *Tabs) selectTab(id string, notify bool) {
	found := false
	for _, tab := range t.Tabs {
		if tab.ID == id {
			found = true
			break
		}
	}
	if !found && len(t.Tabs) > 0 {
		id = t.Tabs[0].ID
	}
	if id == t.SelectedID && notify {
		return
	}
	t.SelectedID = id
	for _, tab := range t.Tabs {
		if tab.Content != nil {
			tab.Content.UIState().SetVisible(tab.ID == id)
		}
	}
	t.invalidateLayout()
	if notify && t.OnChange != nil {
		t.OnChange(id)
	}
}

func (t *Tabs) Select(id string) { t.selectTab(id, true) }

func (t *Tabs) tabWidth(tab Tab) float32 {
	return maxFloat32(80, t.measureText(tab.Title, FontSpec{Size: t.Theme().FontSize}).X+24+iconWidth(tab.Icon))
}

func (t *Tabs) Measure(c Constraints) Vec2 {
	size := Vec2{X: 120, Y: t.headerHeight}
	stripWidth := float32(0)
	for _, tab := range t.Tabs {
		stripWidth += t.tabWidth(tab)
		if tab.ID == t.SelectedID && tab.Content != nil {
			child := tab.Content.Measure(c)
			size.X = maxFloat32(size.X, child.X)
			size.Y += child.Y
		}
	}
	size.X = maxFloat32(size.X, stripWidth)
	return measureWithState(&t.State, size, c)
}

func (t *Tabs) Arrange(r Rect) {
	t.rect = r
	for _, tab := range t.Tabs {
		if tab.ID == t.SelectedID && tab.Content != nil {
			tab.Content.Arrange(Rect{X: r.X, Y: r.Y + t.headerHeight, W: r.W, H: maxFloat32(0, r.H-t.headerHeight)})
		}
	}
}

func (t *Tabs) Paint(c *Canvas) {
	c.Rect(Rect{X: t.rect.X, Y: t.rect.Y, W: t.rect.W, H: t.headerHeight}, c.theme.Header)
	x := t.rect.X
	for _, tab := range t.Tabs {
		width := t.tabWidth(tab)
		r := Rect{X: x, Y: t.rect.Y, W: width, H: t.headerHeight}
		if tab.ID == t.SelectedID {
			c.Rect(r, c.theme.ControlHover)
			c.Rect(Rect{X: x, Y: r.Y + r.H - 3, W: width, H: 3}, c.theme.Accent)
		}
		if tab.Icon == nil {
			c.Text(tab.Title, r, c.theme.FontSize, TextCenter, c.theme.Text)
		} else {
			gap := float32(0)
			if tab.Title != "" {
				gap = iconGap
			}
			labelWidth := t.measureText(tab.Title, FontSpec{Size: c.theme.FontSize}).X
			left := r.X + maxFloat32(0, (r.W-iconSize-gap-labelWidth)*0.5)
			paintIcon(c, tab.Icon, Rect{X: left, Y: r.Y + (r.H-iconSize)*0.5, W: iconSize, H: iconSize}, 1)
			if tab.Title != "" {
				c.Text(tab.Title, Rect{X: left + iconSize + gap, Y: r.Y, W: maxFloat32(0, r.X+r.W-left-iconSize-gap), H: r.H}, c.theme.FontSize, TextLeft, c.theme.Text)
			}
		}
		x += width
	}
}

func (t *Tabs) Handle(e Event) bool {
	if e.Type == PointerDown && e.Button == MouseLeft && e.Y < t.rect.Y+t.headerHeight {
		return true
	}
	if e.Type == PointerUp && e.Button == MouseLeft && e.Y < t.rect.Y+t.headerHeight {
		x := t.rect.X
		for _, tab := range t.Tabs {
			w := t.tabWidth(tab)
			if e.X >= x && e.X < x+w {
				t.Select(tab.ID)
				return true
			}
			x += w
		}
	}
	return t.State.Handle(e)
}

func NewDialog(title string, body Component, actions ...*Button) *Panel {
	p := NewStack(Vertical, 12)
	p.SetAccessibility(AccessibilityInfo{Role: RoleDialog, Name: title})
	p.Background = DefaultTheme().PanelAlt
	p.Border, p.BorderWidth = DefaultTheme().Border, 1
	p.Padding = All(16)
	p.Add(NewLabel(title))
	if body != nil {
		body.UIState().SetFlex(1)
		p.Add(body)
	}
	footer := NewStack(Horizontal, 8)
	for _, action := range actions {
		if action != nil {
			footer.Add(action)
		}
	}
	p.Add(footer)
	return p
}
