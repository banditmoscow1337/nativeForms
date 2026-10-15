package nativeforms

import (
	"math"
	"sort"
)

// StackModel describes independently sized rows. Keys must be unique and
// stable across ModelChanged calls for visible components to retain state.
type StackModel interface {
	Len() int
	Key(index int) string
	Height(index int) float32
	Build(index int) Component
}

// StackModelUpdater optionally refreshes a retained row after its index or
// data changes. Build is called only for newly visible keys.
type StackModelUpdater interface {
	Update(index int, row Component)
}

// VirtualStack is a vertical container for large forms. Place it inside a
// ScrollPanel; only rows near the clipped viewport are created and arranged.
// Heights are indexed once and rebuilt when ModelChanged is called.
type VirtualStack struct {
	State
	Model   StackModel
	prefix  []float32
	current map[string]Component
	spare   map[string]Component
	dirty   bool
}

func NewVirtualStack(model StackModel) *VirtualStack {
	v := &VirtualStack{Model: model, dirty: true}
	v.owner = v
	return v
}

func (v *VirtualStack) indexRows() {
	n := 0
	if v.Model != nil {
		n = max(0, v.Model.Len())
	}
	if !v.dirty && len(v.prefix) == n+1 {
		return
	}
	if cap(v.prefix) < n+1 {
		v.prefix = make([]float32, n+1)
	} else {
		v.prefix = v.prefix[:n+1]
		clear(v.prefix)
	}
	for i := 0; i < n; i++ {
		h := v.Model.Height(i)
		if !(h > 0) || math.IsInf(float64(h), 0) {
			h = 1
		}
		v.prefix[i+1] = v.prefix[i] + h
	}
	v.dirty = false
}

func (v *VirtualStack) ModelChanged() {
	v.dirty = true
	v.indexRows()
	v.Arrange(v.rect)
	v.invalidateLayout()
}

func (v *VirtualStack) Measure(c Constraints) Vec2 {
	v.indexRows()
	width := c.Max.X
	if !c.HasMaxX() {
		width = v.preferred.X
	}
	return measureWithState(&v.State, Vec2{X: width, Y: v.prefix[len(v.prefix)-1]}, c)
}

func (v *VirtualStack) Arrange(r Rect) {
	v.rect = r
	v.indexRows()
	view := r
	for parent := v.parent; parent != nil; parent = parent.UIState().parent {
		if parent.UIState().clip {
			view = intersect(view, parent.UIState().rect)
		}
	}
	n := len(v.prefix) - 1
	start, end := 0, 0
	if !view.Empty() && n > 0 {
		top, bottom := view.Y-r.Y, view.Y+view.H-r.Y
		start = sort.Search(n, func(i int) bool { return v.prefix[i+1] > top })
		end = sort.Search(n, func(i int) bool { return v.prefix[i] >= bottom })
		start, end = max(0, start-1), min(n, end+1)
	}
	if v.current == nil {
		v.current = make(map[string]Component)
	}
	if v.spare == nil {
		v.spare = make(map[string]Component)
	}
	clear(v.spare)
	v.children = v.children[:0]
	for i := start; i < end; i++ {
		key := v.Model.Key(i)
		child := v.current[key]
		if child != nil {
			if updater, ok := v.Model.(StackModelUpdater); ok {
				updater.Update(i, child)
			}
		} else {
			child = v.Model.Build(i)
			if child == nil {
				continue
			}
			bindTree(child, v, v.manager)
			if v.themed {
				applyTheme(child, v.theme)
			}
		}
		v.spare[key] = child
		v.children = append(v.children, child)
		child.Arrange(Rect{X: r.X, Y: r.Y + v.prefix[i], W: r.W, H: v.prefix[i+1] - v.prefix[i]})
	}
	for key, child := range v.current {
		if v.spare[key] == nil {
			if v.manager != nil {
				v.manager.clearSubtreeInteraction(child.UIState())
			}
			bindTree(child, nil, nil)
		}
	}
	v.current, v.spare = v.spare, v.current
}

func (v *VirtualStack) Paint(*Canvas) {}
