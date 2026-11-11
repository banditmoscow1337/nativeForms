package nativeforms

import "math"

func visibleRowEnd(count int, bottom, height float32) int {
	return min(count, int(math.Ceil(float64(bottom/height))))
}

func scrollRowIntoView(offset, pos, size, viewport float32) float32 {
	if pos < offset {
		return pos
	}
	if pos+size > offset+viewport {
		return pos + size - viewport
	}
	return offset
}

func defaultRowHeight(h float32) float32 {
	if h > 0 {
		return h
	}
	return 32
}

func invalidateVirtualModel(state *State) {
	if state.flex > 0 && !state.rect.Empty() {
		state.InvalidatePaint()
		return
	}
	state.invalidateLayout()
}

func setVirtualSelectedID(state *State, current *string, id string, onSelect func(string)) {
	if *current == id {
		return
	}
	*current = id
	state.InvalidatePaint()
	if onSelect != nil {
		onSelect(id)
	}
}

func stepSelectedIndex(count, step int, match func(int) bool) int {
	i := -1
	for row := 0; row < count; row++ {
		if match(row) {
			i = row
			break
		}
	}
	return max(0, min(count-1, i+step))
}

type ListModel interface {
	Len() int
	ID(index int) string
	Label(index int) string
}

// ListIconModel is optional. Implement it to provide row icons; nil icons keep
// their space so labels in the same list stay aligned.
type ListIconModel interface {
	Icon(index int) *ImageResource
}

// ListDecorationModel optionally supplies custom text color and a right-aligned
// status badge for list rows.
type ListDecorationModel interface {
	ItemColor(index int, theme Theme) (Color, bool)
	ItemBadge(index int, theme Theme) (text string, fg Color, bg Color, ok bool)
}

type VirtualList struct {
	State
	Model           ListModel
	RowHeight       float32
	Offset          float32
	SelectedID      string
	OnSelect        func(string)
	targetOffset    float32
	smoothScrolling bool
}

func NewVirtualList(model ListModel) *VirtualList {
	v := &VirtualList{Model: model, RowHeight: 32}
	v.owner, v.focusable, v.clip = v, true, true
	return v
}

func (v *VirtualList) rowHeight() float32 { return defaultRowHeight(v.RowHeight) }

func (v *VirtualList) count() int {
	if v.Model == nil {
		return 0
	}
	return max(0, v.Model.Len())
}

func (v *VirtualList) maxOffset() float32 {
	return maxFloat32(0, float32(v.count())*v.rowHeight()-v.rect.H)
}

func (v *VirtualList) Measure(c Constraints) Vec2 {
	return measureWithState(&v.State, Vec2{X: 160, Y: minFloat32(400, float32(v.count())*v.rowHeight())}, c)
}

func (v *VirtualList) Arrange(r Rect) {
	v.rect = r
	maxY := v.maxOffset()
	v.Offset = clampFloat32(v.Offset, 0, maxY)
	if !v.smoothScrolling {
		v.targetOffset = v.Offset
	} else {
		v.targetOffset = clampFloat32(v.targetOffset, 0, maxY)
	}
}

func (v *VirtualList) stepSmoothScroll(dt float64) bool {
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

// ModelChanged invalidates rows after the backing model changes.
// It clamps the scroll position immediately so a shortened model remains usable.
func (v *VirtualList) ModelChanged() {
	v.Arrange(v.rect)
	invalidateVirtualModel(&v.State)
}

func (v *VirtualList) Paint(c *Canvas) {
	radius := v.effectiveCornerRadius(c.theme)
	c.RoundedRect(v.rect, radius, c.theme.Panel)
	icons, hasIcons := v.Model.(ListIconModel)
	decor, hasDecor := v.Model.(ListDecorationModel)
	h := v.rowHeight()
	start := max(0, int(v.Offset/h))
	end := visibleRowEnd(v.count(), v.Offset+v.rect.H, h)
	rowR := minFloat32(radius, 6)
	for i := start; i < end; i++ {
		r := Rect{X: v.rect.X, Y: v.rect.Y + float32(i)*h - v.Offset, W: v.rect.W, H: h}
		if v.SelectedID != "" && v.Model.ID(i) == v.SelectedID {
			c.RoundedRect(r, rowR, c.theme.Selection)
		}
		var icon *ImageResource
		if hasIcons {
			icon = icons.Icon(i)
		}
		textColor := c.theme.Text
		if hasDecor {
			if col, ok := decor.ItemColor(i, *c.theme); ok {
				textColor = col
			}
		}
		labelRect := r.Inset(Symmetric(9, 0))
		if hasDecor {
			if badgeText, badgeFG, badgeBG, ok := decor.ItemBadge(i, *c.theme); ok && badgeText != "" {
				bw := maxFloat32(20, v.measureText(badgeText, FontSpec{Size: c.theme.FontSize * 0.82, Weight: 700}).X+10)
				bh := minFloat32(h-6, 18)
				bx := r.X + r.W - bw - 6
				by := r.Y + (h-bh)*0.5
				c.RoundedRect(Rect{X: bx, Y: by, W: bw, H: bh}, bh*0.5, badgeBG)
				c.TextStyled(badgeText, Rect{X: bx, Y: by, W: bw, H: bh}, FontSpec{Size: c.theme.FontSize * 0.82, Weight: 700}, TextCenter, badgeFG)
				labelRect.W = maxFloat32(0, labelRect.W-bw-8)
			}
		}
		paintIconLabel(c, icon, v.Model.Label(i), labelRect, hasIcons, textColor, 1)
	}
	if v.Focused() {
		c.RoundedBorder(v.rect, radius, 1, c.theme.Accent)
	}
}

func (v *VirtualList) indexAt(y float32) int {
	i := int((y - v.rect.Y + v.Offset) / v.rowHeight())
	if y < v.rect.Y || y >= v.rect.Y+v.rect.H || i < 0 || i >= v.count() {
		return -1
	}
	return i
}

func (v *VirtualList) SetSelected(id string) {
	setVirtualSelectedID(&v.State, &v.SelectedID, id, v.OnSelect)
}

func (v *VirtualList) ScrollTo(index int) {
	if index < 0 || index >= v.count() {
		return
	}
	h := v.rowHeight()
	before := v.Offset
	v.smoothScrolling = false
	v.Offset = scrollRowIntoView(v.Offset, float32(index)*h, h, v.rect.H)
	v.Arrange(v.rect)
	if v.Offset != before {
		v.InvalidatePaint()
	}
}

func (v *VirtualList) move(step int) bool {
	if v.count() == 0 {
		return false
	}
	i := stepSelectedIndex(v.count(), step, func(j int) bool {
		return v.Model.ID(j) == v.SelectedID
	})
	v.SetSelected(v.Model.ID(i))
	v.ScrollTo(i)
	return true
}

func (v *VirtualList) Handle(e Event) bool {
	switch e.Type {
	case PointerDown:
		if e.Button == MouseRight {
			if i := v.indexAt(e.Y); i >= 0 {
				v.SetSelected(v.Model.ID(i))
			}
			break
		}
		return e.Button == MouseLeft
	case PointerUp:
		if e.Button == MouseLeft && v.Pressed() {
			if i := v.indexAt(e.Y); i >= 0 {
				id := v.Model.ID(i)
				if v.SelectedID == id {
					v.InvalidatePaint()
					if v.OnSelect != nil {
						v.OnSelect(id)
					}
				} else {
					v.SetSelected(id)
				}
				return true
			}
		}
	case PointerScroll:
		previous := v.Offset
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
		if previous != v.Offset || v.smoothScrolling {
			v.InvalidatePaint()
			return true
		}
	case KeyDown:
		if e.Key == KeyDownArrow {
			return v.move(1)
		}
		if e.Key == KeyUpArrow {
			return v.move(-1)
		}
	}
	return v.State.Handle(e)
}
