package nativeforms

import "math"

type TableModel interface {
	Len() int
	ID(row int) string
	Columns() int
	Header(column int) string
	Cell(row, column int) string
}

// TableIconModel optionally supplies an icon for each cell. Every cell keeps
// the same icon column when this interface is implemented.
type TableIconModel interface {
	CellIcon(row, column int) *ImageResource
}

type VirtualTable struct {
	State
	Model                                TableModel
	RowHeight, HeaderHeight, ColumnWidth float32
	Offset, OffsetX                      float32
	SelectedID                           string
	OnSelect                             func(string)
	targetY, targetX                     float32
	smoothScrolling                      bool
}

func NewVirtualTable(model TableModel) *VirtualTable {
	v := &VirtualTable{Model: model, RowHeight: 32, HeaderHeight: 36, ColumnWidth: 150}
	v.owner, v.focusable, v.clip = v, true, true
	return v
}

func (v *VirtualTable) dimensions() (int, int, float32, float32, float32) {
	rows, cols := 0, 0
	if v.Model != nil {
		rows = max(0, v.Model.Len())
		cols = max(0, v.Model.Columns())
	}
	h := defaultRowHeight(v.RowHeight)
	header, width := v.HeaderHeight, v.ColumnWidth
	if header <= 0 {
		header = 36
	}
	if width <= 0 {
		width = 150
	}
	return rows, cols, h, header, width
}

func (v *VirtualTable) Measure(c Constraints) Vec2 {
	rows, cols, h, header, w := v.dimensions()
	return measureWithState(&v.State, Vec2{X: minFloat32(640, float32(cols)*w), Y: minFloat32(480, float32(rows)*h+header)}, c)
}

func (v *VirtualTable) Arrange(r Rect) {
	v.rect = r
	rows, cols, h, header, w := v.dimensions()
	maxY := maxFloat32(0, float32(rows)*h-maxFloat32(0, r.H-header))
	maxX := maxFloat32(0, float32(cols)*w-r.W)
	v.Offset = clampFloat32(v.Offset, 0, maxY)
	v.OffsetX = clampFloat32(v.OffsetX, 0, maxX)
	if !v.smoothScrolling {
		v.targetY, v.targetX = v.Offset, v.OffsetX
	} else {
		v.targetY = clampFloat32(v.targetY, 0, maxY)
		v.targetX = clampFloat32(v.targetX, 0, maxX)
	}
}

func (v *VirtualTable) stepSmoothScroll(dt float64) bool {
	if !v.smoothScrolling {
		return false
	}
	rows, cols, h, header, w := v.dimensions()
	maxY := maxFloat32(0, float32(rows)*h-maxFloat32(0, v.rect.H-header))
	maxX := maxFloat32(0, float32(cols)*w-v.rect.W)
	v.targetY = clampFloat32(v.targetY, 0, maxY)
	v.targetX = clampFloat32(v.targetX, 0, maxX)
	dx := v.targetX - v.OffsetX
	dy := v.targetY - v.Offset
	if absFloat32(dx) <= 0.35 && absFloat32(dy) <= 0.35 {
		changed := v.OffsetX != v.targetX || v.Offset != v.targetY
		v.OffsetX, v.Offset = v.targetX, v.targetY
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
	v.OffsetX = clampFloat32(v.OffsetX+dx*alpha, 0, maxX)
	v.Offset = clampFloat32(v.Offset+dy*alpha, 0, maxY)
	v.InvalidatePaint()
	return true
}

// ModelChanged refreshes the table after rows, columns, or cell values change.
func (v *VirtualTable) ModelChanged() {
	v.Arrange(v.rect)
	invalidateVirtualModel(&v.State)
}

func (v *VirtualTable) Paint(c *Canvas) {
	rows, cols, h, header, w := v.dimensions()
	radius := v.effectiveCornerRadius(c.theme)
	c.RoundedRect(v.rect, radius, c.theme.Panel)
	icons, hasIcons := v.Model.(TableIconModel)
	firstCol := max(0, int(v.OffsetX/w))
	lastCol := min(cols, int((v.OffsetX+v.rect.W)/w)+2)
	start := max(0, int(v.Offset/h))
	end := visibleRowEnd(rows, v.Offset+maxFloat32(0, v.rect.H-header), h)
	bodyRect := Rect{X: v.rect.X, Y: v.rect.Y + header, W: v.rect.W, H: maxFloat32(0, v.rect.H-header)}
	c.PushClip(bodyRect)
	rowR := minFloat32(radius, 6)
	for i := start; i < end; i++ {
		r := Rect{X: v.rect.X, Y: v.rect.Y + header + float32(i)*h - v.Offset, W: v.rect.W, H: h}
		if v.SelectedID != "" && v.Model.ID(i) == v.SelectedID {
			c.RoundedRect(r, rowR, c.theme.Selection)
		}
		for col := firstCol; col < lastCol; col++ {
			cell := Rect{X: v.rect.X + float32(col)*w - v.OffsetX, Y: r.Y, W: w, H: h}
			var icon *ImageResource
			if hasIcons {
				icon = icons.CellIcon(i, col)
			}
			paintIconLabel(c, icon, v.Model.Cell(i, col), cell.Inset(Symmetric(8, 0)), hasIcons, c.theme.Text, 1)
		}
	}
	c.PopClip()
	c.RoundedRect(Rect{X: v.rect.X, Y: v.rect.Y, W: v.rect.W, H: header}, radius, c.theme.Header)
	for col := firstCol; col < lastCol; col++ {
		r := Rect{X: v.rect.X + float32(col)*w - v.OffsetX, Y: v.rect.Y, W: w, H: header}
		c.Text(v.Model.Header(col), r.Inset(Symmetric(8, 0)), c.theme.FontSize, TextLeft, c.theme.Text)
		c.Border(Rect{X: r.X + r.W - 1, Y: r.Y, W: 1, H: r.H}, 1, c.theme.Border)
	}
	if v.Focused() {
		c.RoundedBorder(v.rect, radius, 1, c.theme.Accent)
	}
}

func (v *VirtualTable) SetSelected(id string) {
	setVirtualSelectedID(&v.State, &v.SelectedID, id, v.OnSelect)
}

func (v *VirtualTable) Handle(e Event) bool {
	rows, cols, h, header, w := v.dimensions()
	switch e.Type {
	case PointerDown:
		if e.Button == MouseRight {
			if v.rect.Contains(e.X, e.Y) && e.Y >= v.rect.Y+header {
				i := int((e.Y - v.rect.Y - header + v.Offset) / h)
				if i >= 0 && i < rows {
					v.SetSelected(v.Model.ID(i))
				}
			}
			break
		}
		return e.Button == MouseLeft
	case PointerUp:
		if e.Button == MouseLeft && v.Pressed() && v.rect.Contains(e.X, e.Y) && e.Y >= v.rect.Y+header {
			i := int((e.Y - v.rect.Y - header + v.Offset) / h)
			if i >= 0 && i < rows {
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
		beforeX, beforeY := v.OffsetX, v.Offset
		maxY := maxFloat32(0, float32(rows)*h-maxFloat32(0, v.rect.H-header))
		maxX := maxFloat32(0, float32(cols)*w-v.rect.W)
		if !v.smoothScrolling {
			v.targetY, v.targetX = v.Offset, v.OffsetX
		}
		v.targetY = clampFloat32(v.targetY-e.Scroll.Y*48, 0, maxY)
		v.targetX = clampFloat32(v.targetX-e.Scroll.X*48, 0, maxX)
		if v.manager == nil {
			v.Offset, v.OffsetX = v.targetY, v.targetX
			v.smoothScrolling = false
		} else {
			v.Offset = clampFloat32(v.Offset+(v.targetY-v.Offset)*0.42, 0, maxY)
			v.OffsetX = clampFloat32(v.OffsetX+(v.targetX-v.OffsetX)*0.42, 0, maxX)
			v.smoothScrolling = absFloat32(v.targetY-v.Offset) > 0.35 || absFloat32(v.targetX-v.OffsetX) > 0.35
		}
		if beforeX != v.OffsetX || beforeY != v.Offset || v.smoothScrolling {
			v.InvalidatePaint()
			return true
		}
	case KeyDown:
		if e.Key == KeyUpArrow || e.Key == KeyDownArrow {
			if rows == 0 {
				return false
			}
			step := 1
			if e.Key == KeyUpArrow {
				step = -1
			}
			i := stepSelectedIndex(rows, step, func(row int) bool {
				return v.Model.ID(row) == v.SelectedID
			})
			v.SetSelected(v.Model.ID(i))
			v.smoothScrolling = false
			v.Offset = clampFloat32(v.Offset, 0, maxFloat32(0, float32(rows)*h-maxFloat32(0, v.rect.H-header)))
			v.Offset = scrollRowIntoView(v.Offset, float32(i)*h, h, v.rect.H-header)
			v.Arrange(v.rect)
			v.InvalidatePaint()
			return true
		}
	}
	return v.State.Handle(e)
}
