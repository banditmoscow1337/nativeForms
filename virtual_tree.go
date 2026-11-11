package nativeforms

import "math"

type TreeModel interface {
	Roots() int
	Root(index int) string
	Label(id string) string
	Children(id string) int
	Child(id string, index int) string
}

// TreeIconModel optionally supplies an icon for a visible tree node.
type TreeIconModel interface {
	Icon(id string) *ImageResource
}

// TreeDecorationModel optionally supplies custom text color and a right-aligned
// status badge (e.g. error/warning indicators) for tree nodes.
type TreeDecorationModel interface {
	NodeColor(id string, theme Theme) (Color, bool)
	NodeBadge(id string, theme Theme) (text string, fg Color, bg Color, ok bool)
}

type treeRow struct {
	id       string
	depth    int
	children bool
}

type VirtualTree struct {
	State
	Model             TreeModel
	Expanded          map[string]bool
	SelectedID        string
	OnSelect          func(string)
	Offset, RowHeight float32
	rows              []treeRow
	rowsDirty         bool
	targetOffset      float32
	smoothScrolling   bool
}

func NewVirtualTree(model TreeModel) *VirtualTree {
	v := &VirtualTree{Model: model, Expanded: make(map[string]bool), RowHeight: 32, rowsDirty: true}
	v.owner, v.focusable, v.clip = v, true, true
	return v
}

func (v *VirtualTree) rowHeight() float32 { return defaultRowHeight(v.RowHeight) }

func (v *VirtualTree) rebuild() {
	v.rows = v.rows[:0]
	v.rowsDirty = false
	if v.Model == nil {
		return
	}
	seen := make(map[string]bool)
	var visit func(string, int)
	visit = func(id string, depth int) {
		if seen[id] {
			return
		}
		seen[id] = true
		children := max(0, v.Model.Children(id))
		v.rows = append(v.rows, treeRow{id: id, depth: depth, children: children > 0})
		if v.Expanded[id] {
			for i := 0; i < children; i++ {
				visit(v.Model.Child(id, i), depth+1)
			}
		}
	}
	for i := 0; i < max(0, v.Model.Roots()); i++ {
		visit(v.Model.Root(i), 0)
	}
}

func (v *VirtualTree) maxOffset() float32 {
	return maxFloat32(0, float32(len(v.rows))*v.rowHeight()-v.rect.H)
}

func (v *VirtualTree) Measure(c Constraints) Vec2 {
	if v.rowsDirty || len(v.rows) == 0 {
		v.rebuild()
	}
	return measureWithState(&v.State, Vec2{X: 180, Y: minFloat32(480, float32(len(v.rows))*v.rowHeight())}, c)
}

func (v *VirtualTree) Arrange(r Rect) {
	v.rect = r
	if v.rowsDirty || len(v.rows) == 0 {
		v.rebuild()
	}
	maxY := v.maxOffset()
	v.Offset = clampFloat32(v.Offset, 0, maxY)
	if !v.smoothScrolling {
		v.targetOffset = v.Offset
	} else {
		v.targetOffset = clampFloat32(v.targetOffset, 0, maxY)
	}
}

func (v *VirtualTree) stepSmoothScroll(dt float64) bool {
	if !v.smoothScrolling {
		return false
	}
	maxY := v.maxOffset()
	v.targetOffset = clampFloat32(v.targetOffset, 0, maxY)
	dy := v.targetOffset - v.Offset
	if absFloat32(dy) <= 0.35 {
		changed := v.Offset != v.targetOffset
		v.Offset = v.targetOffset
		v.smoothScrolling = false
		if changed {
			v.InvalidatePaint()
		}
		return false
	}
	if dt <= 0 || dt > 0.1 {
		dt = 1.0 / 120.0
	}
	alpha := clampFloat32(float32(1.0-math.Exp(-dt*28.0)), 0.18, 0.85)
	v.Offset = clampFloat32(v.Offset+dy*alpha, 0, maxY)
	v.InvalidatePaint()
	return true
}

// ModelChanged rebuilds visible nodes after the model changes, including child
// counts and labels. Expanded IDs and the selected ID are preserved.
func (v *VirtualTree) ModelChanged() {
	v.rowsDirty = true
	v.Arrange(v.rect)
	invalidateVirtualModel(&v.State)
}

func (v *VirtualTree) Paint(c *Canvas) {
	radius := v.effectiveCornerRadius(c.theme)
	c.RoundedRect(v.rect, radius, c.theme.Panel)
	icons, hasIcons := v.Model.(TreeIconModel)
	decor, hasDecor := v.Model.(TreeDecorationModel)
	h := v.rowHeight()
	start := max(0, int(v.Offset/h))
	end := visibleRowEnd(len(v.rows), v.Offset+v.rect.H, h)
	rowR := minFloat32(radius, 6)
	for i := start; i < end; i++ {
		row := v.rows[i]
		r := Rect{X: v.rect.X, Y: v.rect.Y + float32(i)*h - v.Offset, W: v.rect.W, H: h}
		if row.id == v.SelectedID {
			c.RoundedRect(r, rowR, c.theme.Selection)
		}
		indent := float32(row.depth) * 18
		textColor := c.theme.Text
		if hasDecor {
			if col, ok := decor.NodeColor(row.id, *c.theme); ok {
				textColor = col
			}
		}
		if row.children {
			paintChevron(c, Rect{X: r.X + indent + 5, Y: r.Y, W: 18, H: r.H}, v.Expanded[row.id], textColor)
		}
		var icon *ImageResource
		if hasIcons {
			icon = icons.Icon(row.id)
		}
		rightReserve := float32(30)
		if hasDecor {
			if badgeText, badgeFG, badgeBG, ok := decor.NodeBadge(row.id, *c.theme); ok && badgeText != "" {
				bw := maxFloat32(20, v.measureText(badgeText, FontSpec{Size: c.theme.FontSize * 0.82, Weight: 700}).X+10)
				bh := minFloat32(h-6, 18)
				bx := r.X + r.W - bw - 6
				by := r.Y + (h-bh)*0.5
				c.RoundedRect(Rect{X: bx, Y: by, W: bw, H: bh}, bh*0.5, badgeBG)
				c.TextStyled(badgeText, Rect{X: bx, Y: by, W: bw, H: bh}, FontSpec{Size: c.theme.FontSize * 0.82, Weight: 700}, TextCenter, badgeFG)
				rightReserve = bw + 36
			}
		}
		paintIconLabel(c, icon, v.Model.Label(row.id), Rect{X: r.X + indent + 26, Y: r.Y, W: maxFloat32(0, r.W-indent-rightReserve), H: r.H}, hasIcons, textColor, 1)
	}
	if v.Focused() {
		c.RoundedBorder(v.rect, radius, 1, c.theme.Accent)
	}
}

func (v *VirtualTree) SetExpanded(id string, expanded bool) {
	if v.Expanded == nil {
		v.Expanded = make(map[string]bool)
	}
	if v.Expanded[id] == expanded && !v.rowsDirty {
		return
	}
	v.Expanded[id] = expanded
	v.ModelChanged()
}

func (v *VirtualTree) SetSelected(id string) {
	setVirtualSelectedID(&v.State, &v.SelectedID, id, v.OnSelect)
}

func (v *VirtualTree) Handle(e Event) bool {
	h := v.rowHeight()
	switch e.Type {
	case PointerDown:
		if e.Button == MouseRight {
			if v.rect.Contains(e.X, e.Y) {
				i := int((e.Y - v.rect.Y + v.Offset) / h)
				if i >= 0 && i < len(v.rows) {
					v.SetSelected(v.rows[i].id)
				}
			}
			break
		}
		return e.Button == MouseLeft
	case PointerUp:
		if e.Button == MouseLeft && v.Pressed() && v.rect.Contains(e.X, e.Y) {
			i := int((e.Y - v.rect.Y + v.Offset) / h)
			if i < 0 || i >= len(v.rows) {
				return false
			}
			row := v.rows[i]
			if row.children && e.X < v.rect.X+float32(row.depth)*18+25 {
				v.SetExpanded(row.id, !v.Expanded[row.id])
			} else if v.SelectedID == row.id {
				v.InvalidatePaint()
				if v.OnSelect != nil {
					v.OnSelect(row.id)
				}
			} else {
				v.SetSelected(row.id)
			}
			return true
		}
	case PointerScroll:
		before := v.Offset
		maxY := v.maxOffset()
		if !v.smoothScrolling {
			v.targetOffset = v.Offset
		}
		v.targetOffset = clampFloat32(v.targetOffset-e.Scroll.Y*48, 0, maxY)
		if v.manager == nil {
			v.Offset = v.targetOffset
			v.smoothScrolling = false
		} else {
			v.Offset = clampFloat32(v.Offset+(v.targetOffset-v.Offset)*0.42, 0, maxY)
			v.smoothScrolling = absFloat32(v.targetOffset-v.Offset) > 0.35
		}
		if before != v.Offset || v.smoothScrolling {
			v.InvalidatePaint()
			return true
		}
	case KeyDown:
		index := -1
		for i, row := range v.rows {
			if row.id == v.SelectedID {
				index = i
				break
			}
		}
		if e.Key == KeyRightArrow && index >= 0 && v.rows[index].children {
			v.SetExpanded(v.SelectedID, true)
			return true
		}
		if e.Key == KeyLeftArrow && index >= 0 && v.rows[index].children {
			v.SetExpanded(v.SelectedID, false)
			return true
		}
		if e.Key == KeyDownArrow {
			index++
		} else if e.Key == KeyUpArrow {
			index--
		} else {
			break
		}
		if len(v.rows) == 0 {
			return false
		}
		index = max(0, min(len(v.rows)-1, index))
		v.SetSelected(v.rows[index].id)
		v.smoothScrolling = false
		v.Offset = scrollRowIntoView(v.Offset, float32(index)*h, h, v.rect.H)
		v.targetOffset = v.Offset
		v.InvalidatePaint()
		return true
	}
	return v.State.Handle(e)
}
