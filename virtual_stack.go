package nativeforms

import (
	"math"
	"sort"
)

// VirtualStackModel supplies stable row IDs, estimated heights and components.
// Build is called only for rows near the viewport. IDs must be unique and
// stable across ModelChanged calls; the model should store durable row data.
type VirtualStackModel interface {
	Len() int
	ID(index int) string
	Height(index int, width float32) float32
	Build(index int) Component
}

// VirtualStackUpdater refreshes an already realized component after
// ModelChanged without destroying its focus/selection state.
type VirtualStackUpdater interface {
	Update(index int, component Component)
}

// VirtualStack is a scrollable component that realizes only visible rows.
// Height may vary by row and available width. A model change rebuilds the
// prefix index; normal scroll and repaint use binary search on that index.
type VirtualStack struct {
	State
	Model    VirtualStackModel
	Offset   float32
	Overscan float32

	width       float32
	prefix      []float32
	dirty       bool
	realized    map[string]Component
	spare       map[string]Component
	updateRows  bool
	anchorID    string
	anchorDelta float32
}

func NewVirtualStack(model VirtualStackModel) *VirtualStack {
	v := &VirtualStack{Model: model, Overscan: 64, dirty: true}
	v.owner, v.clip = v, true
	return v
}

func (v *VirtualStack) rebuild(width float32) {
	if !v.dirty && v.width == width {
		return
	}
	v.width, v.dirty = width, false
	count := 0
	if v.Model != nil {
		count = max(0, v.Model.Len())
	}
	v.prefix = make([]float32, count+1)
	for i := 0; i < count; i++ {
		height := v.Model.Height(i, width)
		if !(height >= 1) || math.IsInf(float64(height), 0) {
			height = 1
		}
		v.prefix[i+1] = v.prefix[i] + height
	}
}

func (v *VirtualStack) contentHeight() float32 {
	if len(v.prefix) == 0 {
		return 0
	}
	return v.prefix[len(v.prefix)-1]
}

func (v *VirtualStack) Measure(c Constraints) Vec2 {
	width := c.Max.X
	if width <= 0 {
		width = 240
	}
	v.rebuild(width)
	return measureWithState(&v.State, Vec2{X: width, Y: minFloat32(400, v.contentHeight())}, c)
}

func (v *VirtualStack) Arrange(r Rect) {
	v.rect = r
	v.rebuild(r.W)
	v.Offset = clampFloat32(v.Offset, 0, maxFloat32(0, v.contentHeight()-r.H))
	v.realize()
}

func (v *VirtualStack) rangeAt(top, bottom float32) (start, end int) {
	count := len(v.prefix) - 1
	if count <= 0 || bottom <= 0 || top >= v.contentHeight() {
		return 0, 0
	}
	start = sort.Search(count, func(i int) bool { return v.prefix[i+1] > top })
	end = sort.Search(count, func(i int) bool { return v.prefix[i] >= bottom })
	return start, max(start, end)
}

// VisibleRange returns the model indices intersecting the viewport.
func (v *VirtualStack) VisibleRange() (start, end int) {
	return v.rangeAt(v.Offset, v.Offset+v.rect.H)
}

func (v *VirtualStack) realize() {
	if v.Model == nil {
		v.detachAll()
		return
	}
	start, end := v.rangeAt(maxFloat32(0, v.Offset-v.Overscan), v.Offset+v.rect.H+v.Overscan)
	previous := v.realized
	current := v.spare
	if current == nil {
		current = make(map[string]Component, end-start)
	} else {
		clear(current)
	}
	children := v.children[:0]
	updater, canUpdate := v.Model.(VirtualStackUpdater)
	for i := start; i < end; i++ {
		id := v.Model.ID(i)
		if id == "" || current[id] != nil {
			continue // Invalid/duplicate IDs cannot be safely recycled.
		}
		child := previous[id]
		if child == nil {
			child = v.Model.Build(i)
			if child == nil {
				continue
			}
			bindTree(child, v, v.manager)
			if v.themed {
				applyTheme(child, v.theme)
			}
		} else if v.updateRows && canUpdate {
			updater.Update(i, child)
		}
		current[id] = child
		children = append(children, child)
		child.Arrange(Rect{X: v.rect.X, Y: v.rect.Y + v.prefix[i] - v.Offset,
			W: v.rect.W, H: v.prefix[i+1] - v.prefix[i]})
	}
	for id, child := range previous {
		if current[id] == nil {
			if v.manager != nil {
				v.manager.clearSubtreeInteraction(child.UIState())
			}
			bindTree(child, nil, nil)
		}
	}
	v.spare, v.realized = previous, current
	v.children = children
	v.updateRows = false
	first, visibleEnd := v.VisibleRange()
	if first < visibleEnd {
		v.anchorID = v.Model.ID(first)
		v.anchorDelta = v.Offset - v.prefix[first]
	} else {
		v.anchorID, v.anchorDelta = "", 0
	}
}

func (v *VirtualStack) detachAll() {
	for _, child := range v.realized {
		if v.manager != nil {
			v.manager.clearSubtreeInteraction(child.UIState())
		}
		bindTree(child, nil, nil)
	}
	v.realized = nil
	v.spare = nil
	v.children = nil
}

// ModelChanged must be called after inserting, deleting or resizing rows.
func (v *VirtualStack) ModelChanged() {
	v.dirty = true
	v.updateRows = true
	v.rebuild(v.rect.W)
	if v.anchorID != "" && v.Model != nil {
		for i := 0; i < v.Model.Len(); i++ {
			if v.Model.ID(i) == v.anchorID {
				v.Offset = v.prefix[i] + v.anchorDelta
				break
			}
		}
	}
	v.Arrange(v.rect)
	v.invalidateLayout()
}

func (v *VirtualStack) ScrollBy(pixels float32) float32 {
	before := v.Offset
	v.Offset = clampFloat32(before+pixels, 0, maxFloat32(0, v.contentHeight()-v.rect.H))
	if v.Offset != before {
		v.realize()
		v.invalidatePaint()
	}
	return pixels - (v.Offset - before)
}

func (v *VirtualStack) ScrollTo(index int) {
	if index < 0 || index+1 >= len(v.prefix) {
		return
	}
	if v.prefix[index] < v.Offset {
		v.ScrollBy(v.prefix[index] - v.Offset)
	} else if v.prefix[index+1] > v.Offset+v.rect.H {
		v.ScrollBy(v.prefix[index+1] - v.Offset - v.rect.H)
	}
}

func (v *VirtualStack) Handle(event Event) bool {
	if event.Type == PointerScroll && event.Scroll.Y != 0 {
		before := v.Offset
		v.ScrollBy(-event.Scroll.Y * 40)
		if v.Offset != before {
			return true
		}
	}
	return v.State.Handle(event)
}
