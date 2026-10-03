package nativeforms

// Checkbox is a compact boolean editor. Its callback runs on the UI owner goroutine.
type Checkbox struct {
	State
	Text     string
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

func (c *Checkbox) Measure(limits Constraints) Vec2 {
	size := MeasureText(c.Text, c.Theme().FontSize)
	return measureWithState(&c.State, Vec2{X: size.X + 34, Y: maxFloat32(size.Y+12, c.Theme().ControlHeight)}, limits)
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
	canvas.Text(c.Text, Rect{X: r.X + 26, Y: c.rect.Y, W: maxFloat32(0, c.rect.W-34), H: c.rect.H}, canvas.theme.FontSize, TextLeft, canvas.theme.Text)
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

// RadioOption has an application-owned stable ID.
type RadioOption struct{ ID, Label string }

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
	for _, option := range r.Options {
		w = maxFloat32(w, MeasureText(option.Label, r.Theme().FontSize).X+38)
	}
	return measureWithState(&r.State, Vec2{X: w, Y: float32(len(r.Options)) * r.Theme().ControlHeight}, c)
}

func (r *RadioGroup) Paint(c *Canvas) {
	h := r.Theme().ControlHeight
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
		c.Text(option.Label, Rect{X: box.X + 27, Y: y, W: r.rect.W - 37, H: h}, c.theme.FontSize, TextLeft, c.theme.Text)
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
	Disabled  bool
	Action    func()
}

// Menu is a popup body with pointer and keyboard selection.
type Menu struct {
	State
	Items    []MenuItem
	OnSelect func(string)
	active   int
	popup    PopupID
}

func NewMenu(items []MenuItem, onSelect func(string)) *Menu {
	m := &Menu{Items: append([]MenuItem(nil), items...), OnSelect: onSelect}
	m.owner, m.focusable = m, true
	return m
}

func (m *Menu) Measure(c Constraints) Vec2 {
	w := float32(140)
	for _, item := range m.Items {
		w = maxFloat32(w, MeasureText(item.Label, m.Theme().FontSize).X+32)
	}
	return measureWithState(&m.State, Vec2{X: w, Y: float32(len(m.Items))*m.Theme().ControlHeight + 8}, c)
}

func (m *Menu) Paint(c *Canvas) {
	c.Rect(m.rect, c.theme.PanelAlt)
	c.Border(m.rect, 1, c.theme.Border)
	h := c.theme.ControlHeight
	for i, item := range m.Items {
		r := Rect{X: m.rect.X + 4, Y: m.rect.Y + 4 + float32(i)*h, W: m.rect.W - 8, H: h}
		if r.Y >= m.rect.Y+m.rect.H {
			break
		}
		if i == m.active {
			c.Rect(r, c.theme.Selection)
		}
		color := c.theme.Text
		if item.Disabled {
			color = c.theme.Disabled
		}
		c.Text(item.Label, r.Inset(Symmetric(10, 0)), c.theme.FontSize, TextLeft, color)
	}
}

func (m *Menu) indexAt(y float32) int {
	i := int((y - m.rect.Y - 4) / m.Theme().ControlHeight)
	if y < m.rect.Y+4 || i < 0 || i >= len(m.Items) {
		return -1
	}
	return i
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
		if i := m.indexAt(e.Y); i >= 0 && i != m.active {
			m.active = i
			m.invalidatePaint()
		}
	case PointerDown:
		return e.Button == MouseLeft
	case PointerUp:
		if e.Button == MouseLeft && m.rect.Contains(e.X, e.Y) {
			m.active = m.indexAt(e.Y)
			return m.activate()
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
			for n := 0; n < len(m.Items); n++ {
				m.active = (m.active + step + len(m.Items)) % len(m.Items)
				if !m.Items[m.active].Disabled {
					break
				}
			}
			m.invalidatePaint()
			return true
		case KeyEnter, KeySpace:
			return m.activate()
		}
	}
	return m.State.Handle(e)
}

func (manager *Manager) OpenMenu(items []MenuItem, bounds Rect) PopupID {
	m := NewMenu(items, nil)
	id := manager.OpenPopup(m, bounds, false)
	m.popup = id
	return id
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
	for _, option := range c.Options {
		w = maxFloat32(w, MeasureText(option.Label, c.Theme().FontSize).X+40)
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
	for _, option := range c.Options {
		if option.ID == c.SelectedID {
			label = option.Label
			break
		}
	}
	canvas.Text(label, c.rect.Inset(Symmetric(10, 0)), canvas.theme.FontSize, TextLeft, canvas.theme.Text)
	canvas.Text("▾", c.rect.Inset(Symmetric(10, 0)), canvas.theme.FontSize, TextRight, canvas.theme.Text)
}

func (c *ComboBox) open() {
	if c.manager == nil {
		return
	}
	items := make([]MenuItem, 0, len(c.Options))
	for _, option := range c.Options {
		items = append(items, MenuItem{ID: option.ID, Label: option.Label})
	}
	m := NewMenu(items, c.SetSelected)
	m.popup = c.manager.OpenPopup(m, Rect{X: c.rect.X, Y: c.rect.Y + c.rect.H, W: c.rect.W}, false)
}

func (c *ComboBox) Handle(e Event) bool {
	if !c.Enabled() {
		return false
	}
	if e.Type == PointerDown && e.Button == MouseLeft {
		return true
	}
	if (e.Type == PointerUp && e.Button == MouseLeft && c.Pressed() && c.rect.Contains(e.X, e.Y)) ||
		(e.Type == KeyDown && (e.Key == KeySpace || e.Key == KeyEnter)) {
		c.open()
		return true
	}
	return c.State.Handle(e)
}

type Tab struct {
	ID, Title string
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

func (t *Tabs) Measure(c Constraints) Vec2 {
	size := Vec2{X: 120, Y: t.headerHeight}
	for _, tab := range t.Tabs {
		if tab.ID == t.SelectedID && tab.Content != nil {
			child := tab.Content.Measure(c)
			size.X = maxFloat32(size.X, child.X)
			size.Y += child.Y
		}
	}
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
		width := maxFloat32(80, MeasureText(tab.Title, c.theme.FontSize).X+24)
		r := Rect{X: x, Y: t.rect.Y, W: width, H: t.headerHeight}
		if tab.ID == t.SelectedID {
			c.Rect(r, c.theme.ControlHover)
			c.Rect(Rect{X: x, Y: r.Y + r.H - 3, W: width, H: 3}, c.theme.Accent)
		}
		c.Text(tab.Title, r, c.theme.FontSize, TextCenter, c.theme.Text)
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
			w := maxFloat32(80, MeasureText(tab.Title, t.Theme().FontSize).X+24)
			if e.X >= x && e.X < x+w {
				t.Select(tab.ID)
				return true
			}
			x += w
		}
	}
	return t.State.Handle(e)
}

// NewDialog returns content that can be passed to Manager.OpenDialog.
func NewDialog(title string, body Component, actions ...*Button) *Panel {
	p := NewStack(Vertical, 12)
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
