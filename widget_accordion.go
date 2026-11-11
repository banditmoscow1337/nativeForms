package nativeforms

// Collapsible is an expandable/collapsible container with a clickable header
// and chevron indicator.
type Collapsible struct {
	State
	Title        string
	Icon         *ImageResource
	Content      Component
	Expanded     bool
	HeaderHeight float32
	OnChange     func(bool)
}

func NewCollapsible(title string, content Component, expanded bool, onChange func(bool)) *Collapsible {
	c := &Collapsible{
		Title:        title,
		Content:      content,
		Expanded:     expanded,
		HeaderHeight: 36,
		OnChange:     onChange,
	}
	c.owner = c
	c.focusable = true
	if content != nil {
		content.UIState().SetVisible(expanded)
		c.Add(content)
	}
	return c
}

func (c *Collapsible) headerH() float32 {
	if c.HeaderHeight > 0 {
		return c.HeaderHeight
	}
	return maxFloat32(34, c.Theme().ControlHeight*0.9)
}

func (c *Collapsible) SetTitle(title string) {
	if c.Title == title {
		return
	}
	c.Title = title
	c.invalidateLayout()
}

func (c *Collapsible) SetContent(content Component) {
	if sameComponent(c.Content, content) {
		return
	}
	if c.Content != nil {
		c.Remove(c.Content)
	}
	c.Content = content
	if content != nil {
		content.UIState().SetVisible(c.Expanded)
		c.Add(content)
	}
	c.invalidateLayout()
}

func (c *Collapsible) SetExpanded(expanded bool) {
	if c.Expanded == expanded {
		return
	}
	c.Expanded = expanded
	if c.Content != nil {
		c.Content.UIState().SetVisible(expanded)
	}
	c.invalidateLayout()
	if c.OnChange != nil {
		c.OnChange(expanded)
	}
}

func (c *Collapsible) Toggle() { c.SetExpanded(!c.Expanded) }

func (c *Collapsible) measureTextSize() Vec2 {
	theme := c.State.Theme()
	hdrH := c.headerH()
	hdrW := c.measureText(c.Title, FontSpec{Size: theme.FontSize, Weight: 600}).X + 44 + iconWidth(c.Icon)
	size := Vec2{X: maxFloat32(140, hdrW), Y: hdrH}
	if c.Expanded && c.Content != nil && c.Content.UIState().Visible() {
		child := c.Content.Measure(Constraints{})
		size.X = maxFloat32(size.X, child.X+16)
		size.Y += child.Y + 12
	}
	return size
}

func (c *Collapsible) Measure(constraints Constraints) Vec2 {
	theme := c.State.Theme()
	hdrH := c.headerH()
	hdrW := c.measureText(c.Title, FontSpec{Size: theme.FontSize, Weight: 600}).X + 44 + iconWidth(c.Icon)
	size := Vec2{X: maxFloat32(140, hdrW), Y: hdrH}
	if c.Expanded && c.Content != nil && c.Content.UIState().Visible() {
		innerMax := Vec2{X: maxFloat32(0, constraints.Max.X-16), Y: maxFloat32(0, constraints.Max.Y-hdrH-12)}
		child := c.Content.Measure(Constraints{Max: innerMax, BoundedX: constraints.HasMaxX(), BoundedY: constraints.HasMaxY()})
		size.X = maxFloat32(size.X, child.X+16)
		size.Y += child.Y + 12
	}
	return measureWithState(&c.State, size, constraints)
}

func (c *Collapsible) Arrange(r Rect) {
	c.rect = r
	hdrH := c.headerH()
	if c.Expanded && c.Content != nil && c.Content.UIState().Visible() {
		body := Rect{
			X: r.X + 8,
			Y: r.Y + hdrH + 6,
			W: maxFloat32(0, r.W-16),
			H: maxFloat32(0, r.H-hdrH-12),
		}
		c.Content.Arrange(body)
	}
}

func (c *Collapsible) Paint(canvas *Canvas) {
	if c.rect.Empty() {
		return
	}
	radius := c.effectiveCornerRadius(canvas.theme)
	hdrH := minFloat32(c.rect.H, c.headerH())
	hdrRect := Rect{X: c.rect.X, Y: c.rect.Y, W: c.rect.W, H: hdrH}
	if !c.Frameless() {
		if c.Expanded {
			canvas.RoundedRect(c.rect, radius, canvas.theme.PanelAlt)
		}
		hdrBg := canvas.theme.Header
		if c.Hovered() && c.Enabled() {
			hdrBg = canvas.theme.ControlHover
		}
		canvas.RoundedRect(hdrRect, radius, hdrBg)
		border := canvas.theme.Border
		if c.Focused() {
			border = canvas.theme.Accent
		}
		canvas.RoundedBorder(c.rect, radius, 1, border)
	}
	chevronRect := Rect{X: hdrRect.X + 8, Y: hdrRect.Y, W: 18, H: hdrRect.H}
	paintChevron(canvas, chevronRect, c.Expanded, canvas.theme.Text)
	labelRect := Rect{X: hdrRect.X + 30, Y: hdrRect.Y, W: maxFloat32(0, hdrRect.W-38), H: hdrRect.H}
	paintIconLabel(canvas, c.Icon, c.Title, labelRect, c.Icon != nil, canvas.theme.Text, enabledOpacity(c.Enabled()))
}

func (c *Collapsible) Handle(event Event) bool {
	if !c.Enabled() {
		return false
	}
	inHeader := c.rect.Contains(event.X, event.Y) && event.Y < c.rect.Y+c.headerH()
	switch event.Type {
	case PointerDown:
		if event.Button == MouseLeft && inHeader {
			return true
		}
	case PointerUp:
		if event.Button == MouseLeft && c.Pressed() && inHeader {
			c.Toggle()
			return true
		}
	case KeyDown:
		if !event.Repeat {
			switch event.Key {
			case KeyEnter, KeySpace:
				c.Toggle()
				return true
			case KeyRightArrow:
				c.SetExpanded(true)
				return true
			case KeyLeftArrow:
				c.SetExpanded(false)
				return true
			}
		}
	}
	return c.State.Handle(event)
}

// Accordion coordinates a stack of Collapsible sections. When SingleOpen is
// true, expanding one section automatically collapses all other sections.
type Accordion struct {
	State
	Sections   []*Collapsible
	SingleOpen bool
	Gap        float32
	OnChange   func(index int, expanded bool)
	updating   bool
}

func NewAccordion(singleOpen bool, sections ...*Collapsible) *Accordion {
	acc := &Accordion{
		SingleOpen: singleOpen,
		Gap:        6,
	}
	acc.owner = acc
	for _, sec := range sections {
		acc.AddSection(sec)
	}
	return acc
}

func (acc *Accordion) AddSection(sec *Collapsible) {
	if sec == nil {
		return
	}
	idx := len(acc.Sections)
	if acc.SingleOpen && sec.Expanded {
		for _, existing := range acc.Sections {
			if existing.Expanded {
				sec.SetExpanded(false)
				break
			}
		}
	}
	origOnChange := sec.OnChange
	sec.OnChange = func(expanded bool) {
		if origOnChange != nil {
			origOnChange(expanded)
		}
		if acc.updating {
			return
		}
		if acc.SingleOpen && expanded {
			acc.updating = true
			for i, other := range acc.Sections {
				if i != idx && other.Expanded {
					other.SetExpanded(false)
				}
			}
			acc.updating = false
		}
		if acc.OnChange != nil {
			acc.OnChange(idx, expanded)
		}
	}
	acc.Sections = append(acc.Sections, sec)
	acc.Add(sec)
}

func (acc *Accordion) ExpandSection(index int, expanded bool) {
	if index < 0 || index >= len(acc.Sections) {
		return
	}
	acc.Sections[index].SetExpanded(expanded)
}

func (acc *Accordion) Measure(constraints Constraints) Vec2 {
	total := Vec2{}
	vis := 0
	for _, sec := range acc.Sections {
		if sec == nil || !sec.UIState().Visible() {
			continue
		}
		sz := sec.Measure(Constraints{Max: Vec2{X: constraints.Max.X}, BoundedX: constraints.HasMaxX()})
		total.X = maxFloat32(total.X, sz.X)
		total.Y += sz.Y
		vis++
	}
	if vis > 1 {
		total.Y += acc.Gap * float32(vis-1)
	}
	return measureWithState(&acc.State, total, constraints)
}

func (acc *Accordion) Arrange(r Rect) {
	acc.rect = r
	y := r.Y
	for _, sec := range acc.Sections {
		if sec == nil || !sec.UIState().Visible() {
			continue
		}
		sz := sec.Measure(Constraints{Max: Vec2{X: r.W}, BoundedX: true})
		sec.Arrange(Rect{X: r.X, Y: y, W: r.W, H: sz.Y})
		y += sz.Y + acc.Gap
	}
}

func (acc *Accordion) Paint(*Canvas) {}
