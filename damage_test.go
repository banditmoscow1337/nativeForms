package nativeforms

import "testing"

func TestManagerDamageUnionAndFullInvalidation(t *testing.T) {
	m := New()
	m.SetRoot(NewPanel())
	m.BeginFrame(100, 80, 0)
	if got := m.Frame().Damage; got != (Rect{W: 100, H: 80}) {
		t.Fatalf("first frame damage = %+v", got)
	}
	m.InvalidateRect(Rect{X: 10, Y: 20, W: 5, H: 5})
	m.InvalidateRect(Rect{X: 20, Y: 10, W: 5, H: 10})
	m.BeginFrame(100, 80, 0)
	if got := m.Frame().Damage; got != (Rect{X: 10, Y: 10, W: 15, H: 15}) {
		t.Fatalf("union damage = %+v", got)
	}
	m.InvalidateLayout()
	m.BeginFrame(100, 80, 0)
	if got := m.Frame().Damage; got != (Rect{W: 100, H: 80}) {
		t.Fatalf("layout damage = %+v", got)
	}
	m.InvalidateRect(Rect{X: -20, Y: -20, W: 25, H: 25})
	m.BeginFrame(100, 80, 0)
	if got := m.Frame().Damage; got != (Rect{W: 5, H: 5}) {
		t.Fatalf("clipped damage = %+v", got)
	}
}
