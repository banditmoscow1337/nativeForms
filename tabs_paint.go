package nativeforms

func (t *Tabs) tabWidth(tab Tab) float32 {
	width := float32(50)
	if tab.NonClosable {
		width = 26
	}
	return maxFloat32(100, t.measureText(tab.Title, FontSpec{Size: t.Theme().FontSize}).X+width+iconWidth(tab.Icon))
}

func (t *Tabs) maxOffset() float32 {
	width := float32(0)
	for _, tab := range t.Tabs {
		width += t.tabWidth(tab)
	}
	return maxFloat32(0, width-t.rect.W)
}

// ScrollTo makes a tab's header visible in the current viewport.
func (t *Tabs) ScrollTo(id string) {
	x := float32(0)
	for _, tab := range t.Tabs {
		w := t.tabWidth(tab)
		if tab.ID == id {
			t.Offset = clampFloat32(scrollRowIntoView(t.Offset, x, w, t.rect.W), 0, t.maxOffset())
			t.InvalidatePaint()
			return
		}
		x += w
	}
}

func (t *Tabs) Measure(c Constraints) Vec2 {
	width := float32(0)
	for _, tab := range t.Tabs {
		width += t.tabWidth(tab)
	}
	size := Vec2{X: maxFloat32(120, width), Y: t.headerHeight}
	childC := c
	if childC.HasMaxY() {
		childC.Max.Y = maxFloat32(0, childC.Max.Y-t.headerHeight)
		childC.Min.Y = minFloat32(childC.Min.Y, childC.Max.Y)
	}
	for _, tab := range t.Tabs {
		if tab.ID == t.SelectedID && tab.Content != nil {
			child := tab.Content.Measure(childC)
			size.X = maxFloat32(size.X, child.X)
			size.Y += child.Y
		}
	}
	return measureWithState(&t.State, size, c)
}

func (t *Tabs) Arrange(r Rect) {
	t.rect = r
	t.Offset = clampFloat32(t.Offset, 0, t.maxOffset())
	for _, tab := range t.Tabs {
		if tab.ID == t.SelectedID && tab.Content != nil {
			tab.Content.Arrange(Rect{X: r.X, Y: r.Y + t.headerHeight, W: r.W, H: maxFloat32(0, r.H-t.headerHeight)})
		}
	}
}

func (t *Tabs) tabAt(x float32) (int, Rect) {
	left := t.rect.X - t.Offset
	for i, tab := range t.Tabs {
		w := t.tabWidth(tab)
		rect := Rect{X: left, Y: t.rect.Y, W: w, H: t.headerHeight}
		if x >= left && x < left+w && x >= t.rect.X && x < t.rect.X+t.rect.W {
			return i, rect
		}
		left += w
	}
	return -1, Rect{}
}

func (t *Tabs) Paint(c *Canvas) {
	header := Rect{X: t.rect.X, Y: t.rect.Y, W: t.rect.W, H: t.headerHeight}
	c.Rect(header, c.theme.Header)
	c.PushClip(header)
	x := t.rect.X - t.Offset
	for _, tab := range t.Tabs {
		width := t.tabWidth(tab)
		r := Rect{X: x, Y: t.rect.Y, W: width, H: t.headerHeight}
		if r.X+r.W <= t.rect.X || r.X >= t.rect.X+t.rect.W {
			x += width
			continue
		}
		if tab.ID == t.SelectedID {
			c.Rect(r, c.theme.ControlHover)
			indicatorColor := c.theme.Accent
			if tab.HasError {
				indicatorColor = c.theme.Danger
			}
			c.Rect(Rect{X: x, Y: r.Y + r.H - 3, W: width, H: 3}, indicatorColor)
		}
		left := r.X + 10
		if tab.Icon != nil {
			paintIcon(c, tab.Icon, Rect{X: left, Y: r.Y + (r.H-iconSize)*0.5, W: iconSize, H: iconSize}, 1)
			left += iconSize + iconGap
		}
		right := r.X + r.W - 12
		if !tab.NonClosable {
			right = r.X + r.W - 26
		}
		if tab.HasError {
			c.RoundedRect(Rect{X: right - 10, Y: r.Y + (r.H-7)*0.5, W: 7, H: 7}, 3.5, c.theme.Danger)
			right -= 13
		} else if tab.Dirty {
			c.RoundedRect(Rect{X: right - 10, Y: r.Y + (r.H-7)*0.5, W: 7, H: 7}, 3.5, c.theme.Accent)
			right -= 13
		}
		titleColor := c.theme.Text
		if tab.HasError {
			titleColor = c.theme.Danger
		}
		c.Text(tab.Title, Rect{X: left, Y: r.Y, W: maxFloat32(0, right-left), H: r.H},
			c.theme.FontSize, TextLeft, titleColor)
		if !tab.NonClosable {
			c.Text("×", Rect{X: r.X + r.W - 24, Y: r.Y, W: 20, H: r.H},
				c.theme.FontSize, TextCenter, c.theme.Text)
		}
		x += width
	}
	c.PopClip()
}

func (t *Tabs) Handle(e Event) bool {
	inHeader := t.rect.Contains(e.X, e.Y) && e.Y < t.rect.Y+t.headerHeight
	switch e.Type {
	case PointerDown:
		if t.OnFocus != nil && t.rect.Contains(e.X, e.Y) {
			t.OnFocus()
		}
		if e.Button == MouseRight && inHeader {
			tabID := t.SelectedID
			if i, _ := t.tabAt(e.X); i >= 0 {
				tabID = t.Tabs[i].ID
				t.Select(tabID)
			}
			if t.OnTabContextMenu != nil && t.manager != nil {
				if items := t.OnTabContextMenu(tabID, e.X, e.Y); len(items) > 0 {
					t.manager.OpenContextMenu(items, e.X, e.Y)
					return true
				}
			}
		}
		if e.Button == MouseLeft && inHeader {
			t.downID, t.downClose, t.dragged, t.downX = "", false, false, e.X
			if i, r := t.tabAt(e.X); i >= 0 {
				t.downID = t.Tabs[i].ID
				t.downClose = !t.Tabs[i].NonClosable && e.X >= r.X+r.W-26
			}
			return true
		}
		if e.Button == MouseMiddle && inHeader {
			if i, _ := t.tabAt(e.X); i >= 0 && !t.Tabs[i].NonClosable {
				t.Close(t.Tabs[i].ID)
				return true
			}
		}
	case PointerMove:
		if t.downID != "" && t.Pressed() && !t.downClose {
			if e.X < t.downX-5 || e.X > t.downX+5 {
				t.dragged = true
			}
			if t.dragged && inHeader {
				if i, _ := t.tabAt(e.X); i >= 0 {
					t.Move(t.downID, i)
				}
			}
			return true
		}
	case PointerUp:
		if t.downID != "" && e.Button == MouseLeft {
			id, closing := t.downID, t.downClose
			t.downID = ""
			if inHeader {
				if i, r := t.tabAt(e.X); i >= 0 && t.Tabs[i].ID == id {
					if closing && e.X >= r.X+r.W-26 {
						t.Close(id)
					} else if !closing {
						t.Select(id)
					}
				}
			}
			return true
		}
		// Keep direct PointerUp selection for callers using the original Tabs API.
		if e.Button == MouseLeft && inHeader {
			if i, _ := t.tabAt(e.X); i >= 0 {
				t.Select(t.Tabs[i].ID)
				return true
			}
		}
	case PointerScroll:
		if inHeader {
			delta := e.Scroll.X
			if delta == 0 {
				delta = e.Scroll.Y
			}
			before := t.Offset
			t.Offset = clampFloat32(t.Offset-delta*40, 0, t.maxOffset())
			if before != t.Offset {
				t.InvalidatePaint()
				return true
			}
		}
	}
	return t.State.Handle(e)
}

func (t *Tabs) Count() int { return len(t.Tabs) }
