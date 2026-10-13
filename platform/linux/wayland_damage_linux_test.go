package linux

import (
	"image"
	"testing"
)

func TestWaylandAlternatingBuffersAccumulateMissedDamage(t *testing.T) {
	full := image.Rect(0, 0, 100, 100)
	a, b := &wlBuffer{dirty: full}, &wlBuffer{dirty: full}
	w := &WaylandWindow{current: [2]*wlBuffer{a, b}}
	w.advanceBufferDamage(a, image.Rect(1, 1, 10, 10))
	if !a.dirty.Empty() || b.dirty != full {
		t.Fatalf("initial buffers: a=%v b=%v", a.dirty, b.dirty)
	}
	w.advanceBufferDamage(b, image.Rect(20, 20, 30, 30))
	if got := a.dirty; got != image.Rect(20, 20, 30, 30) {
		t.Fatalf("first buffer lost changes while busy: %v", got)
	}
	current := image.Rect(40, 40, 50, 50)
	if got := a.dirty.Union(current); got != image.Rect(20, 20, 50, 50) {
		t.Fatalf("reused buffer damage = %v", got)
	}
}
