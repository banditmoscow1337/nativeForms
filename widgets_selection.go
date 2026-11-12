package nativeforms

func hasRadioOptionIcons(options []RadioOption) bool {
	for _, option := range options {
		if option.Icon != nil {
			return true
		}
	}
	return false
}

// selectRadioOption updates a shared option selection only for a known, new ID.
func selectRadioOption(options []RadioOption, selected *string, id string) bool {
	for _, option := range options {
		if option.ID == id {
			if *selected == id {
				return false
			}
			*selected = id
			return true
		}
	}
	return false
}

func measureRadioOptionsWidth(state *State, options []RadioOption, minWidth, padding float32) float32 {
	w := minWidth
	if state.autoSize {
		w = 0
	}
	iconSpace := float32(0)
	if hasRadioOptionIcons(options) {
		iconSpace = iconSize + iconGap
	}
	spec := FontSpec{Size: state.Theme().FontSize}
	for _, option := range options {
		w = maxFloat32(w, state.measureText(option.Label, spec).X+padding+iconSpace)
	}
	return w
}

type Checkbox struct {
	State
	Text          string
	Icon          *ImageResource
	Checked       bool
	Indeterminate bool
	OnChange      func(bool)
}

func NewCheckbox(text string, checked bool, onChange func(bool)) *Checkbox {
	c := &Checkbox{Text: text, Checked: checked, OnChange: onChange}
	c.owner, c.focusable = c, true
	return c
}

func (c *Checkbox) SetText(text string) {
	if c.Text == text {
		return
	}
	c.Text = text
	c.invalidateLayout()
}

func (c *Checkbox) SetChecked(value bool) {
	if c.Checked == value && !c.Indeterminate {
		return
	}
	c.Checked = value
	c.Indeterminate = false
	c.InvalidatePaint()
	if c.OnChange != nil {
		c.OnChange(value)
	}
}

func (c *Checkbox) SetIndeterminate(indeterminate bool) {
	if c.Indeterminate == indeterminate {
		return
	}
	c.Indeterminate = indeterminate
	c.InvalidatePaint()
}

func (c *Checkbox) SetIcon(icon *ImageResource) {
	if c.Icon != icon {
		c.Icon = icon
		c.invalidateLayout()
	}
}

func (c *Checkbox) measureTextSize() Vec2 {
	size := c.measureText(c.Text, FontSpec{Size: c.Theme().FontSize})
	return Vec2{X: size.X + 34 + iconWidth(c.Icon), Y: maxFloat32(size.Y+12, c.Theme().ControlHeight)}
}

func (c *Checkbox) Measure(limits Constraints) Vec2 {
	return measureWithState(&c.State, c.measureTextSize(), limits)
}

func (c *Checkbox) Paint(canvas *Canvas) {
	if c.rect.Empty() {
		return
	}
	r := Rect{X: c.rect.X + 6, Y: c.rect.Y + (c.rect.H-18)/2, W: 18, H: 18}
	radius := c.effectiveCornerRadius(canvas.theme)
	boxRadius := minFloat32(radius, 6)
	if !c.Frameless() {
		canvas.RoundedRect(r, boxRadius, canvas.theme.Control)
		canvas.RoundedBorder(r, boxRadius, 1, canvas.theme.Border)
	}
	if c.Indeterminate {
		dash := Rect{X: r.X + 3, Y: r.Y + (r.H-4)*0.5, W: r.W - 6, H: 4}
		canvas.RoundedRect(dash, 2, canvas.theme.Accent)
	} else if c.Checked {
		canvas.RoundedRect(r.Inset(All(4)), maxFloat32(0, boxRadius-2), canvas.theme.Accent)
	}
	if c.Focused() && !c.Frameless() {
		canvas.RoundedBorder(c.rect, radius, 1, canvas.theme.Accent)
	}
	paintIconLabel(canvas, c.Icon, c.Text, Rect{X: r.X + 26, Y: c.rect.Y, W: maxFloat32(0, c.rect.W-34), H: c.rect.H}, c.Icon != nil, canvas.theme.Text, enabledOpacity(c.Enabled()))
}

func (c *Checkbox) Handle(e Event) bool {
	return handleControlActivation(&c.State, e, func() {
		if c.Indeterminate {
			c.SetChecked(true)
		} else {
			c.SetChecked(!c.Checked)
		}
	})
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
	r.owner, r.focusable, r.clip = r, true, true
	return r
}

func (r *RadioGroup) SetOptions(options []RadioOption) {
	r.Options = append(r.Options[:0], options...)
	r.invalidateLayout()
}

func (r *RadioGroup) SetSelected(id string) {
	if selectRadioOption(r.Options, &r.SelectedID, id) {
		r.InvalidatePaint()
		if r.OnChange != nil {
			r.OnChange(id)
		}
	}
}

func (r *RadioGroup) measureTextSize() Vec2 {
	w := measureRadioOptionsWidth(&r.State, r.Options, 0, 38)
	return Vec2{X: w, Y: float32(len(r.Options)) * r.Theme().ControlHeight}
}

func (r *RadioGroup) Measure(c Constraints) Vec2 {
	w := measureRadioOptionsWidth(&r.State, r.Options, 90, 38)
	return measureWithState(&r.State, Vec2{X: w, Y: float32(len(r.Options)) * r.Theme().ControlHeight}, c)
}

func (r *RadioGroup) Paint(c *Canvas) {
	if r.rect.Empty() {
		return
	}
	h := r.Theme().ControlHeight
	icons := hasRadioOptionIcons(r.Options)
	radius := r.effectiveCornerRadius(c.theme)
	boxRadius := minFloat32(radius, 9)
	for i, option := range r.Options {
		y := r.rect.Y + float32(i)*h
		if y >= r.rect.Y+r.rect.H {
			break
		}
		box := Rect{X: r.rect.X + 8, Y: y + (h-18)/2, W: 18, H: 18}
		if !r.Frameless() {
			c.RoundedBorder(box, boxRadius, 1, c.theme.Border)
		}
		if option.ID == r.SelectedID {
			c.RoundedRect(box.Inset(All(5)), maxFloat32(0, boxRadius-3), c.theme.Accent)
		}
		paintIconLabel(c, option.Icon, option.Label, Rect{X: box.X + 27, Y: y, W: maxFloat32(0, r.rect.W-37), H: h}, icons, c.theme.Text, 1)
	}
	if r.Focused() && !r.Frameless() {
		c.RoundedBorder(r.rect, radius, 1, c.theme.Accent)
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

type ComboBox struct {
	State
	Options    []RadioOption
	SelectedID string
	OnChange   func(string)
}

func NewComboBox(options []RadioOption, selected string, onChange func(string)) *ComboBox {
	c := &ComboBox{Options: append([]RadioOption(nil), options...), SelectedID: selected, OnChange: onChange}
	c.owner, c.focusable, c.clip = c, true, true
	return c
}

func (c *ComboBox) SetOptions(options []RadioOption) {
	c.Options = append(c.Options[:0], options...)
	if len(c.Options) > 0 {
		found := false
		for _, opt := range c.Options {
			if opt.ID == c.SelectedID {
				found = true
				break
			}
		}
		if !found {
			c.SelectedID = c.Options[0].ID
		}
	}
	c.invalidateLayout()
}

func (c *ComboBox) SetSelected(id string) {
	if selectRadioOption(c.Options, &c.SelectedID, id) {
		if c.autoSize {
			c.invalidateLayout()
		} else {
			c.InvalidatePaint()
		}
		if c.OnChange != nil {
			c.OnChange(id)
		}
	}
}

func (c *ComboBox) measureTextSize() Vec2 {
	spec := FontSpec{Size: c.Theme().FontSize}
	iconSpace := float32(0)
	if hasRadioOptionIcons(c.Options) {
		iconSpace = iconSize + iconGap
	}
	for _, option := range c.Options {
		if option.ID == c.SelectedID {
			size := c.measureText(option.Label, spec)
			return Vec2{X: size.X + 42 + iconSpace, Y: maxFloat32(size.Y+14, c.Theme().ControlHeight)}
		}
	}
	w := measureRadioOptionsWidth(&c.State, c.Options, 64, 42)
	return Vec2{X: w, Y: c.Theme().ControlHeight}
}

func (c *ComboBox) Measure(limits Constraints) Vec2 {
	if c.autoSize {
		return measureWithState(&c.State, c.measureTextSize(), limits)
	}
	w := measureRadioOptionsWidth(&c.State, c.Options, 140, 40)
	return measureWithState(&c.State, Vec2{X: w, Y: c.Theme().ControlHeight}, limits)
}

func (c *ComboBox) Paint(canvas *Canvas) {
	if c.rect.Empty() {
		return
	}
	paintControlSurface(canvas, &c.State)
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
	m.MaxVisibleItems = 8
	m.SetCornerRadius(c.CornerRadius())
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

func NewDialog(title string, body Component, actions ...*Button) *Panel {
	p := NewStack(Vertical, 12)
	p.SetAccessibility(AccessibilityInfo{Role: RoleDialog, Name: title})
	theme := DefaultTheme()
	p.Background = theme.PanelAlt
	p.Border, p.BorderWidth = theme.Border, 1
	p.CornerRadius = theme.CornerRadius
	p.Padding = All(16)
	titleLbl := NewLabel(title)
	titleLbl.Font = FontSpec{Weight: 700}
	titleLbl.Wrap = true
	p.Add(titleLbl)
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
