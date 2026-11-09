package nativeforms

type TrackMode uint8

const (
	TrackAuto TrackMode = iota
	TrackFixed
	TrackStar
)

type Track struct {
	Mode  TrackMode
	Value float32
}
type gridItem struct {
	child                      Component
	row, col, rowSpan, colSpan int
}

type Grid struct {
	State
	Columns, Rows     []Track
	ColumnGap, RowGap float32
	items             []gridItem
}

func NewGrid(columns ...Track) *Grid {
	g := &Grid{Columns: append([]Track(nil), columns...)}
	g.owner = g
	return g
}

func (g *Grid) AddAt(child Component, row, col, rowSpan, colSpan int) {
	if child == nil || row < 0 || col < 0 {
		return
	}
	if sameComponent(child.UIState().parent, g) {
		g.Remove(child)
	}
	if rowSpan < 1 {
		rowSpan = 1
	}
	if colSpan < 1 {
		colSpan = 1
	}
	for len(g.Rows) < row+rowSpan {
		g.Rows = append(g.Rows, Track{Mode: TrackAuto})
	}
	for len(g.Columns) < col+colSpan {
		g.Columns = append(g.Columns, Track{Mode: TrackAuto})
	}
	g.items = append(g.items, gridItem{child, row, col, rowSpan, colSpan})
	g.State.Add(child)
}

func (g *Grid) Remove(child Component) bool {
	if !g.State.Remove(child) {
		return false
	}
	for i := 0; i < len(g.items); i++ {
		if sameComponent(g.items[i].child, child) {
			g.items = append(g.items[:i], g.items[i+1:]...)
			i--
		}
	}
	return true
}

func (g *Grid) AddRow(label, control Component) {
	row := len(g.Rows)
	g.AddAt(label, row, 0, 1, 1)
	g.AddAt(control, row, 1, 1, 1)
}

func NewForm(labelWidth float32) *Grid {
	first := Track{Mode: TrackAuto}
	if labelWidth > 0 {
		first = Track{Mode: TrackFixed, Value: labelWidth}
	}
	return NewGrid(first, Track{Mode: TrackStar, Value: 1})
}

func trackSizes(tracks []Track) []float32 {
	sizes := make([]float32, len(tracks))
	for i, t := range tracks {
		if t.Mode == TrackFixed {
			sizes[i] = maxFloat32(0, t.Value)
		}
	}
	return sizes
}
func distributeTracks(tracks []Track, sizes []float32, space float32) {
	base := float32(0)
	weight := float32(0)
	for i, t := range tracks {
		base += sizes[i]
		if t.Mode == TrackStar {
			weight += maxFloat32(0, t.Value)
		}
	}
	if weight > 0 && space > base {
		for i, t := range tracks {
			if t.Mode == TrackStar {
				sizes[i] += (space - base) * maxFloat32(0, t.Value) / weight
			}
		}
	}
	if space < base {
		deficit := base - space
		for _, mode := range [...]TrackMode{TrackStar, TrackAuto} {
			capacity := float32(0)
			for i, t := range tracks {
				if t.Mode == mode {
					capacity += sizes[i]
				}
			}
			if capacity <= 0 {
				continue
			}
			reduce := minFloat32(deficit, capacity)
			for i, t := range tracks {
				if t.Mode == mode {
					sizes[i] -= reduce * sizes[i] / capacity
				}
			}
			deficit -= reduce
			if deficit <= 0 {
				break
			}
		}
	}
}
func sumTracks(sizes []float32, gap float32) float32 {
	total := float32(0)
	for _, size := range sizes {
		total += size
	}
	if len(sizes) > 1 {
		total += float32(len(sizes)-1) * gap
	}
	return total
}

func expandTrackSpan(tracks []Track, sizes []float32, start, end int, required float32) {
	if start >= end {
		return
	}
	current := float32(0)
	for i := start; i < end; i++ {
		current += sizes[i]
	}
	if current < required {
		share := (required - current) / float32(end-start)
		for i := start; i < end; i++ {
			if tracks[i].Mode != TrackFixed {
				sizes[i] += share
			}
		}
	}
}

func (g *Grid) intrinsic() ([]float32, []float32) {
	cols, rows := trackSizes(g.Columns), trackSizes(g.Rows)
	for _, item := range g.items {
		if !item.child.UIState().Visible() || !sameComponent(item.child.UIState().parent, g) {
			continue
		}
		size := item.child.Measure(Constraints{})
		margin := item.child.UIState().margin
		expandTrackSpan(g.Columns, cols, item.col, min(item.col+item.colSpan, len(cols)), size.X+margin.Left+margin.Right)
		expandTrackSpan(g.Rows, rows, item.row, min(item.row+item.rowSpan, len(rows)), size.Y+margin.Top+margin.Bottom)
	}
	return cols, rows
}

func (g *Grid) measureRowHeights(cols, rows []float32, includeColGap bool) {
	for _, item := range g.items {
		if !item.child.UIState().Visible() || !sameComponent(item.child.UIState().parent, g) {
			continue
		}
		width := float32(0)
		for i := item.col; i < min(item.col+item.colSpan, len(cols)); i++ {
			width += cols[i]
		}
		if includeColGap {
			width += g.ColumnGap * float32(max(0, item.colSpan-1))
		}
		margin := item.child.UIState().margin
		size := item.child.Measure(Constraints{Max: Vec2{X: maxFloat32(0, width-margin.Left-margin.Right)}, BoundedX: true})
		height := size.Y + margin.Top + margin.Bottom
		rowEnd := min(item.row+item.rowSpan, len(rows))
		current := float32(0)
		for i := item.row; i < rowEnd; i++ {
			current += rows[i]
		}
		if height > current {
			share := (height - current) / float32(item.rowSpan)
			for i := item.row; i < rowEnd; i++ {
				if g.Rows[i].Mode != TrackFixed {
					rows[i] += share
				}
			}
		}
	}
}

func (g *Grid) Measure(c Constraints) Vec2 {
	cols, rows := g.intrinsic()
	if c.HasMaxX() {
		distributeTracks(g.Columns, cols, maxFloat32(0, c.Max.X-g.ColumnGap*float32(max(0, len(cols)-1))))
	}
	g.measureRowHeights(cols, rows, true)
	return measureWithState(&g.State, Vec2{X: sumTracks(cols, g.ColumnGap), Y: sumTracks(rows, g.RowGap)}, c)
}

func (g *Grid) Arrange(bounds Rect) {
	g.rect = bounds
	cols, rows := g.intrinsic()
	distributeTracks(g.Columns, cols, maxFloat32(0, bounds.W-g.ColumnGap*float32(max(0, len(cols)-1))))
	g.measureRowHeights(cols, rows, false)
	distributeTracks(g.Rows, rows, maxFloat32(0, bounds.H-g.RowGap*float32(max(0, len(rows)-1))))

	colOffsets := make([]float32, len(cols)+1)
	colOffsets[0] = bounds.X
	for i, w := range cols {
		colOffsets[i+1] = colOffsets[i] + w + g.ColumnGap
	}
	rowOffsets := make([]float32, len(rows)+1)
	rowOffsets[0] = bounds.Y
	for i, h := range rows {
		rowOffsets[i+1] = rowOffsets[i] + h + g.RowGap
	}

	for _, item := range g.items {
		if !item.child.UIState().Visible() || !sameComponent(item.child.UIState().parent, g) {
			continue
		}
		colStart, colEnd := min(item.col, len(cols)), min(item.col+item.colSpan, len(cols))
		rowStart, rowEnd := min(item.row, len(rows)), min(item.row+item.rowSpan, len(rows))
		x, y := colOffsets[colStart], rowOffsets[rowStart]
		width := maxFloat32(0, colOffsets[colEnd]-x-g.ColumnGap)
		height := maxFloat32(0, rowOffsets[rowEnd]-y-g.RowGap)
		item.child.Arrange((Rect{X: x, Y: y, W: width, H: height}).Inset(item.child.UIState().margin))
	}
}

type SplitPane struct {
	State
	First, Second       Component
	Direction           Direction
	Ratio               float32
	Divider             float32
	MinFirst, MinSecond float32
	dragging            bool
}

func NewSplitPane(direction Direction, first, second Component) *SplitPane {
	p := &SplitPane{Direction: direction, First: first, Second: second, Ratio: 0.5, Divider: 6}
	p.owner = p
	p.Add(first, second)
	return p
}
func (p *SplitPane) Measure(c Constraints) Vec2 {
	a, b := Vec2{}, Vec2{}
	if p.First != nil {
		a = p.First.Measure(c)
	}
	if p.Second != nil {
		b = p.Second.Measure(c)
	}
	if p.Direction == Vertical {
		return measureWithState(&p.State, Vec2{X: maxFloat32(a.X, b.X), Y: a.Y + b.Y + p.Divider}, c)
	}
	return measureWithState(&p.State, Vec2{X: a.X + b.X + p.Divider, Y: maxFloat32(a.Y, b.Y)}, c)
}
func (p *SplitPane) Arrange(r Rect) {
	p.rect = r
	space := maxFloat32(0, r.W-p.Divider)
	if p.Direction == Vertical {
		space = maxFloat32(0, r.H-p.Divider)
	}
	minFirst := minFloat32(p.MinFirst, space)
	maxFirst := maxFloat32(minFirst, space-p.MinSecond)
	first := clampFloat32(space*p.Ratio, minFirst, maxFirst)
	if space > 0 {
		p.Ratio = first / space
	}
	if p.Direction == Vertical {
		if p.First != nil {
			p.First.Arrange(Rect{X: r.X, Y: r.Y, W: r.W, H: first})
		}
		if p.Second != nil {
			p.Second.Arrange(Rect{X: r.X, Y: r.Y + first + p.Divider, W: r.W, H: maxFloat32(0, space-first)})
		}
	} else {
		if p.First != nil {
			p.First.Arrange(Rect{X: r.X, Y: r.Y, W: first, H: r.H})
		}
		if p.Second != nil {
			p.Second.Arrange(Rect{X: r.X + first + p.Divider, Y: r.Y, W: maxFloat32(0, space-first), H: r.H})
		}
	}
}
func (p *SplitPane) Paint(c *Canvas) {
	space := p.rect.W - p.Divider
	if p.Direction == Vertical {
		space = p.rect.H - p.Divider
	}
	start := maxFloat32(0, space) * p.Ratio
	if p.Direction == Vertical {
		c.Rect(Rect{X: p.rect.X, Y: p.rect.Y + start, W: p.rect.W, H: p.Divider}, c.theme.Border)
	} else {
		c.Rect(Rect{X: p.rect.X + start, Y: p.rect.Y, W: p.Divider, H: p.rect.H}, c.theme.Border)
	}
}
func (p *SplitPane) Handle(e Event) bool {
	coord := e.X - p.rect.X
	space := p.rect.W - p.Divider
	if p.Direction == Vertical {
		coord = e.Y - p.rect.Y
		space = p.rect.H - p.Divider
	}
	start := space * p.Ratio
	switch e.Type {
	case PointerDown:
		if e.Button == MouseLeft && coord >= start-2 && coord <= start+p.Divider+2 {
			p.dragging = true
			return true
		}
	case PointerMove:
		if p.dragging && space > 0 {
			p.Ratio = clampFloat32(coord-p.Divider/2, p.MinFirst, maxFloat32(p.MinFirst, space-p.MinSecond)) / space
			p.Arrange(p.rect)
			p.invalidatePaint()
			return true
		}
	case PointerUp:
		if p.dragging && e.Button == MouseLeft {
			p.dragging = false
			return true
		}
	}
	return p.State.Handle(e)
}
