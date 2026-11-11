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

func recycleVirtualRow(child Component) {
	ReleaseComponent(child)
}

// VirtualStack is a vertical container for large forms. Place it inside a
// ScrollPanel; only rows near the clipped viewport are created and arranged.
// Heights are indexed once and rebuilt when ModelChanged is called.
type VirtualStack struct {
	State
	Model          StackModel
	prefix         []float32
	current        map[string]Component
	spare          map[string]Component
	activeStart    int
	activeEnd      int
	activeKeys     []string
	activeChildren []Component
	scratchKeys    []string
	evicted        []Component
	dirty          bool
	reconcileKeys  bool
	keysSynced     bool
}

func NewVirtualStack(model StackModel) *VirtualStack {
	v := &VirtualStack{
		Model:          model,
		current:        make(map[string]Component, 64),
		spare:          make(map[string]Component, 64),
		activeKeys:     make([]string, 0, 64),
		activeChildren: make([]Component, 0, 64),
		scratchKeys:    make([]string, 0, 64),
		evicted:        make([]Component, 0, 16),
		dirty:          true,
		reconcileKeys:  true,
		keysSynced:     true,
	}
	v.children = make([]Component, 0, 64)
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
	v.reconcileKeys = true
}

func (v *VirtualStack) syncActiveKeysToMap() {
	if v.keysSynced {
		return
	}
	clear(v.current)
	for offset, child := range v.activeChildren {
		if child == nil {
			continue
		}
		var key string
		if offset < len(v.activeKeys) && v.activeKeys[offset] != "" {
			key = v.activeKeys[offset]
		} else if v.Model != nil {
			key = v.Model.Key(v.activeStart + offset)
		}
		if key != "" {
			v.current[key] = child
		}
	}
	v.keysSynced = true
}

func (v *VirtualStack) ModelChanged() {
	var beforeTotal float32
	if len(v.prefix) > 0 {
		beforeTotal = v.prefix[len(v.prefix)-1]
	}
	v.syncActiveKeysToMap()
	v.dirty = true
	v.reconcileKeys = true
	v.indexRows()
	v.Arrange(v.rect)
	afterTotal := v.prefix[len(v.prefix)-1]
	if !v.rect.Empty() && (v.flex > 0 || beforeTotal == afterTotal) {
		v.InvalidatePaint()
	} else {
		v.invalidateLayout()
	}
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
		v.current = make(map[string]Component, 64)
	}
	if v.spare == nil {
		v.spare = make(map[string]Component, 64)
	}

	updater, canUpdate := v.Model.(StackModelUpdater)

	// Fast path when scrolling without ModelChanged: rows in the overlapping
	// index window retain their components by index offset without calling
	// Model.Key(i) or mutating the key map.
	if !v.reconcileKeys && len(v.activeChildren) == v.activeEnd-v.activeStart {
		evicted := v.evicted[:0]
		for i := v.activeStart; i < v.activeEnd; i++ {
			if i >= start && i < end {
				continue
			}
			child := v.activeChildren[i-v.activeStart]
			if child != nil {
				if v.manager != nil {
					v.manager.clearSubtreeInteraction(child.UIState())
				}
				if canUpdate {
					evicted = append(evicted, child)
				} else {
					bindTree(child, nil, nil)
					recycleVirtualRow(child)
				}
			}
		}
		v.children = v.children[:0]
		for i := start; i < end; i++ {
			var child Component
			if i >= v.activeStart && i < v.activeEnd {
				child = v.activeChildren[i-v.activeStart]
			} else if canUpdate && len(evicted) > 0 {
				child = evicted[len(evicted)-1]
				evicted = evicted[:len(evicted)-1]
				updater.Update(i, child)
			} else {
				child = v.Model.Build(i)
				if child != nil {
					bindTree(child, v, v.manager)
					if v.themed {
						applyTheme(child, v.theme)
					}
				}
			}
			if child != nil {
				v.children = append(v.children, child)
				child.Arrange(Rect{X: r.X, Y: r.Y + v.prefix[i], W: r.W, H: v.prefix[i+1] - v.prefix[i]})
			}
		}
		for _, child := range evicted {
			bindTree(child, nil, nil)
			recycleVirtualRow(child)
		}
		v.evicted = evicted[:0]
		v.activeStart, v.activeEnd = start, end
		v.activeKeys = v.activeKeys[:0]
		v.activeChildren = append(v.activeChildren[:0], v.children...)
		v.keysSynced = false
		return
	}

	v.syncActiveKeysToMap()
	clear(v.spare)
	v.children = v.children[:0]
	v.activeKeys = v.activeKeys[:0]
	for i := start; i < end; i++ {
		key := v.Model.Key(i)
		child := v.current[key]
		if child != nil {
			if canUpdate {
				updater.Update(i, child)
			}
		} else {
			child = v.Model.Build(i)
			if child == nil {
				v.activeKeys = append(v.activeKeys, key)
				continue
			}
			bindTree(child, v, v.manager)
			if v.themed {
				applyTheme(child, v.theme)
			}
		}
		v.spare[key] = child
		v.activeKeys = append(v.activeKeys, key)
		v.children = append(v.children, child)
		child.Arrange(Rect{X: r.X, Y: r.Y + v.prefix[i], W: r.W, H: v.prefix[i+1] - v.prefix[i]})
	}
	for key, child := range v.current {
		if v.spare[key] == nil {
			if v.manager != nil {
				v.manager.clearSubtreeInteraction(child.UIState())
			}
			bindTree(child, nil, nil)
			recycleVirtualRow(child)
		}
	}
	v.current, v.spare = v.spare, v.current
	v.activeStart, v.activeEnd = start, end
	v.activeChildren = append(v.activeChildren[:0], v.children...)
	v.reconcileKeys = false
	v.keysSynced = true
}

func (v *VirtualStack) Paint(*Canvas) {}
