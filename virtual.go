package nativeforms

// ListModel is indexed for viewport access. ID must remain stable across
// inserts, removals and sorting, and should be unique within this model.
type ListModel interface {
	Len() int
	ID(index int) string
	Label(index int) string
}

type VirtualList struct {
	State
	Model      ListModel
	RowHeight  float32
	Offset     float32
	SelectedID string
	OnSelect   func(string)
}

func NewVirtualList(model ListModel) *VirtualList {
	v := &VirtualList{Model: model, RowHeight: 32}
	v.owner, v.focusable, v.clip = v, true, true
	return v
}

func (v *VirtualList) rowHeight() float32 {
	if v.RowHeight > 0 {
		return v.RowHeight
	}
	return 32
}

func (v *VirtualList) count() int {
	if v.Model == nil {
		return 0
	}
	return max(0, v.Model.Len())
}

func (v *VirtualList) Measure(c Constraints) Vec2 {
	return measureWithState(&v.State, Vec2{X: 160, Y: minFloat32(400, float32(v.count())*v.rowHeight())}, c)
}

func (v *VirtualList) Arrange(r Rect) {
	v.rect = r
	v.Offset = clampFloat32(v.Offset, 0, maxFloat32(0, float32(v.count())*v.rowHeight()-r.H))
}

func (v *VirtualList) Paint(c *Canvas) {
	c.Rect(v.rect, c.theme.Panel)
	h := v.rowHeight()
	start := max(0, int(v.Offset/h))
	end := min(v.count(), int((v.Offset+v.rect.H)/h)+2)
	for i := start; i < end; i++ {
		r := Rect{X: v.rect.X, Y: v.rect.Y + float32(i)*h - v.Offset, W: v.rect.W, H: h}
		if v.Model.ID(i) == v.SelectedID {
			c.Rect(r, c.theme.Selection)
		}
		c.Text(v.Model.Label(i), r.Inset(Symmetric(9, 0)), c.theme.FontSize, TextLeft, c.theme.Text)
	}
	if v.Focused() {
		c.Border(v.rect, 1, c.theme.Accent)
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
	if v.SelectedID == id {
		return
	}
	v.SelectedID = id
	v.invalidatePaint()
	if v.OnSelect != nil {
		v.OnSelect(id)
	}
}

func (v *VirtualList) ScrollTo(index int) {
	if index < 0 || index >= v.count() {
		return
	}
	y := float32(index) * v.rowHeight()
	if y < v.Offset {
		v.Offset = y
	} else if y+v.rowHeight() > v.Offset+v.rect.H {
		v.Offset = y + v.rowHeight() - v.rect.H
	}
	v.Arrange(v.rect)
	v.invalidatePaint()
}

func (v *VirtualList) move(step int) bool {
	if v.count() == 0 {
		return false
	}
	i := -1
	for j := 0; j < v.count(); j++ {
		if v.Model.ID(j) == v.SelectedID {
			i = j
			break
		}
	}
	i = max(0, min(v.count()-1, i+step))
	v.SetSelected(v.Model.ID(i))
	v.ScrollTo(i)
	return true
}

func (v *VirtualList) Handle(e Event) bool {
	switch e.Type {
	case PointerDown:
		return e.Button == MouseLeft
	case PointerUp:
		if e.Button == MouseLeft && v.Pressed() {
			if i := v.indexAt(e.Y); i >= 0 {
				v.SetSelected(v.Model.ID(i))
				return true
			}
		}
	case PointerScroll:
		previous := v.Offset
		v.Offset = clampFloat32(v.Offset-e.Scroll.Y*40, 0, maxFloat32(0, float32(v.count())*v.rowHeight()-v.rect.H))
		if previous != v.Offset {
			v.invalidatePaint()
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

// TableModel owns its data. Implementations may load cells lazily.
type TableModel interface {
	Len() int
	ID(row int) string
	Columns() int
	Header(column int) string
	Cell(row, column int) string
}

type VirtualTable struct {
	State
	Model                                TableModel
	RowHeight, HeaderHeight, ColumnWidth float32
	Offset, OffsetX                      float32
	SelectedID                           string
	OnSelect                             func(string)
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
	h, header, width := v.RowHeight, v.HeaderHeight, v.ColumnWidth
	if h <= 0 {
		h = 32
	}
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
	v.Offset = clampFloat32(v.Offset, 0, maxFloat32(0, float32(rows)*h-maxFloat32(0, r.H-header)))
	v.OffsetX = clampFloat32(v.OffsetX, 0, maxFloat32(0, float32(cols)*w-r.W))
}

func (v *VirtualTable) Paint(c *Canvas) {
	rows, cols, h, header, w := v.dimensions()
	c.Rect(v.rect, c.theme.Panel)
	firstCol := max(0, int(v.OffsetX/w))
	lastCol := min(cols, int((v.OffsetX+v.rect.W)/w)+2)
	start := max(0, int(v.Offset/h))
	end := min(rows, int((v.Offset+maxFloat32(0, v.rect.H-header))/h)+2)
	for i := start; i < end; i++ {
		r := Rect{X: v.rect.X, Y: v.rect.Y + header + float32(i)*h - v.Offset, W: v.rect.W, H: h}
		if v.Model.ID(i) == v.SelectedID {
			c.Rect(r, c.theme.Selection)
		}
		for col := firstCol; col < lastCol; col++ {
			cell := Rect{X: v.rect.X + float32(col)*w - v.OffsetX, Y: r.Y, W: w, H: h}
			c.Text(v.Model.Cell(i, col), cell.Inset(Symmetric(8, 0)), c.theme.FontSize, TextLeft, c.theme.Text)
		}
	}
	c.Rect(Rect{X: v.rect.X, Y: v.rect.Y, W: v.rect.W, H: header}, c.theme.Header)
	for col := firstCol; col < lastCol; col++ {
		r := Rect{X: v.rect.X + float32(col)*w - v.OffsetX, Y: v.rect.Y, W: w, H: header}
		c.Text(v.Model.Header(col), r.Inset(Symmetric(8, 0)), c.theme.FontSize, TextLeft, c.theme.Text)
		c.Border(Rect{X: r.X + r.W - 1, Y: r.Y, W: 1, H: r.H}, 1, c.theme.Border)
	}
	if v.Focused() {
		c.Border(v.rect, 1, c.theme.Accent)
	}
}

func (v *VirtualTable) SetSelected(id string) {
	if v.SelectedID == id {
		return
	}
	v.SelectedID = id
	v.invalidatePaint()
	if v.OnSelect != nil {
		v.OnSelect(id)
	}
}

func (v *VirtualTable) Handle(e Event) bool {
	rows, cols, h, header, w := v.dimensions()
	switch e.Type {
	case PointerDown:
		return e.Button == MouseLeft
	case PointerUp:
		if e.Button == MouseLeft && v.Pressed() && v.rect.Contains(e.X, e.Y) && e.Y >= v.rect.Y+header {
			i := int((e.Y - v.rect.Y - header + v.Offset) / h)
			if i >= 0 && i < rows {
				v.SetSelected(v.Model.ID(i))
				return true
			}
		}
	case PointerScroll:
		beforeX, beforeY := v.OffsetX, v.Offset
		v.Offset = clampFloat32(v.Offset-e.Scroll.Y*40, 0, maxFloat32(0, float32(rows)*h-maxFloat32(0, v.rect.H-header)))
		v.OffsetX = clampFloat32(v.OffsetX-e.Scroll.X*40, 0, maxFloat32(0, float32(cols)*w-v.rect.W))
		if beforeX != v.OffsetX || beforeY != v.Offset {
			v.invalidatePaint()
			return true
		}
	case KeyDown:
		if e.Key == KeyUpArrow || e.Key == KeyDownArrow {
			i := -1
			for row := 0; row < rows; row++ {
				if v.Model.ID(row) == v.SelectedID {
					i = row
					break
				}
			}
			if e.Key == KeyDownArrow {
				i++
			} else {
				i--
			}
			if rows == 0 {
				return false
			}
			i = max(0, min(rows-1, i))
			v.SetSelected(v.Model.ID(i))
			v.Offset = clampFloat32(v.Offset, 0, maxFloat32(0, float32(rows)*h-maxFloat32(0, v.rect.H-header)))
			y := float32(i) * h
			if y < v.Offset {
				v.Offset = y
			} else if y+h > v.Offset+v.rect.H-header {
				v.Offset = y + h - v.rect.H + header
			}
			v.Arrange(v.rect)
			v.invalidatePaint()
			return true
		}
	}
	return v.State.Handle(e)
}

// TreeModel provides stable node IDs and children by index. The tree flattens
// expanded nodes on layout; painting and hit testing only visit viewport rows.
type TreeModel interface {
	Roots() int
	Root(index int) string
	Label(id string) string
	Children(id string) int
	Child(id string, index int) string
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
}

func NewVirtualTree(model TreeModel) *VirtualTree {
	v := &VirtualTree{Model: model, Expanded: make(map[string]bool), RowHeight: 32}
	v.owner, v.focusable, v.clip = v, true, true
	return v
}

func (v *VirtualTree) rebuild() {
	v.rows = v.rows[:0]
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

func (v *VirtualTree) Measure(c Constraints) Vec2 {
	v.rebuild()
	h := v.RowHeight
	if h <= 0 {
		h = 32
	}
	return measureWithState(&v.State, Vec2{X: 180, Y: minFloat32(480, float32(len(v.rows))*h)}, c)
}

func (v *VirtualTree) Arrange(r Rect) {
	v.rect = r
	v.rebuild()
	h := v.RowHeight
	if h <= 0 {
		h = 32
	}
	v.Offset = clampFloat32(v.Offset, 0, maxFloat32(0, float32(len(v.rows))*h-r.H))
}

func (v *VirtualTree) Paint(c *Canvas) {
	c.Rect(v.rect, c.theme.Panel)
	h := v.RowHeight
	if h <= 0 {
		h = 32
	}
	start := max(0, int(v.Offset/h))
	end := min(len(v.rows), int((v.Offset+v.rect.H)/h)+2)
	for i := start; i < end; i++ {
		row := v.rows[i]
		r := Rect{X: v.rect.X, Y: v.rect.Y + float32(i)*h - v.Offset, W: v.rect.W, H: h}
		if row.id == v.SelectedID {
			c.Rect(r, c.theme.Selection)
		}
		indent := float32(row.depth) * 18
		if row.children {
			icon := "▸"
			if v.Expanded[row.id] {
				icon = "▾"
			}
			c.Text(icon, Rect{X: r.X + indent + 5, Y: r.Y, W: 18, H: r.H}, c.theme.FontSize, TextLeft, c.theme.Text)
		}
		c.Text(v.Model.Label(row.id), Rect{X: r.X + indent + 26, Y: r.Y, W: maxFloat32(0, r.W-indent-30), H: r.H}, c.theme.FontSize, TextLeft, c.theme.Text)
	}
	if v.Focused() {
		c.Border(v.rect, 1, c.theme.Accent)
	}
}

func (v *VirtualTree) SetExpanded(id string, expanded bool) {
	if v.Expanded == nil {
		v.Expanded = make(map[string]bool)
	}
	v.Expanded[id] = expanded
	v.invalidateLayout()
}

func (v *VirtualTree) SetSelected(id string) {
	if v.SelectedID == id {
		return
	}
	v.SelectedID = id
	v.invalidatePaint()
	if v.OnSelect != nil {
		v.OnSelect(id)
	}
}

func (v *VirtualTree) Handle(e Event) bool {
	h := v.RowHeight
	if h <= 0 {
		h = 32
	}
	switch e.Type {
	case PointerDown:
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
			} else {
				v.SetSelected(row.id)
			}
			return true
		}
	case PointerScroll:
		before := v.Offset
		v.Offset = clampFloat32(v.Offset-e.Scroll.Y*40, 0, maxFloat32(0, float32(len(v.rows))*h-v.rect.H))
		if before != v.Offset {
			v.invalidatePaint()
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
		y := float32(index) * h
		if y < v.Offset {
			v.Offset = y
		} else if y+h > v.Offset+v.rect.H {
			v.Offset = y + h - v.rect.H
		}
		v.invalidatePaint()
		return true
	}
	return v.State.Handle(e)
}
