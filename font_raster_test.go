package nativeforms

import "testing"

func TestOutlineSegmentsAndRasterization(t *testing.T) {
	square := outline{
		points: []fontPoint{
			{x: 0, y: 0, on: true},
			{x: 8, y: 0, on: true},
			{x: 8, y: 8, on: true},
			{x: 0, y: 8, on: true},
		},
		ends: []int{3},
	}
	segments, ok := outlineSegments(square, func(p fontPoint) floatPoint {
		return floatPoint{x: p.x, y: p.y}
	})
	if !ok || len(segments) != 5 {
		t.Fatalf("square contour: ok=%v, segments=%d; want 5", ok, len(segments))
	}
	mask, origin := rasterOutline(square, 1, false, 0)
	if mask == nil || mask.Bounds().Dx() != 8 || mask.Bounds().Dy() != 8 ||
		origin != (Vec2{X: 0, Y: -8}) || mask.AlphaAt(4, 4).A == 0 {
		t.Fatalf("square rasterization: mask=%v, origin=%v", mask, origin)
	}

	square.ends = []int{4}
	if _, ok := outlineSegments(square, func(p fontPoint) floatPoint {
		return floatPoint{x: p.x, y: p.y}
	}); ok {
		t.Fatal("invalid contour end was accepted")
	}
	if mask, _ := rasterOutline(square, 1, false, 0); mask != nil {
		t.Fatal("invalid contour was rasterized")
	}
}
