package nativeforms

import "testing"

func TestTessellateRoundedRectClipsCorners(t *testing.T) {
	command := DrawCommand{
		Kind: CommandRoundedRect, Rect: Rect{X: 2, Y: 3, W: 12, H: 12},
		Clip: Rect{X: 5, Y: 3, W: 7, H: 12}, Radius: 6,
		Tint: RGBA(80, 160, 240, 255),
	}
	vertices := Tessellate([]DrawCommand{command}, nil)
	if len(vertices) == 0 || len(vertices)%3 != 0 {
		t.Fatalf("rounded rectangle produced %d vertices", len(vertices))
	}
	for _, v := range vertices {
		if v.X < 5-1e-4 || v.X > 12+1e-4 || v.Y < 3-1e-4 || v.Y > 15+1e-4 {
			t.Fatalf("vertex outside clip: %+v", v)
		}
		if v.U != 0.5/fontAtlasSize || v.V != 0.5/fontAtlasSize || v.A != 1 {
			t.Fatalf("rounded rectangle did not use the solid atlas texel and tint: %+v", v)
		}
	}
	contains := func(x, y float32) bool {
		point := Vec2{X: x, Y: y}
		for i := 0; i < len(vertices); i += 3 {
			a := Vec2{X: vertices[i].X, Y: vertices[i].Y}
			b := Vec2{X: vertices[i+1].X, Y: vertices[i+1].Y}
			c := Vec2{X: vertices[i+2].X, Y: vertices[i+2].Y}
			cross := func(p, q Vec2) float32 {
				return (q.X-p.X)*(point.Y-p.Y) - (q.Y-p.Y)*(point.X-p.X)
			}
			ab, bc, ca := cross(a, b), cross(b, c), cross(c, a)
			if ab >= -1e-4 && bc >= -1e-4 && ca >= -1e-4 ||
				ab <= 1e-4 && bc <= 1e-4 && ca <= 1e-4 {
				return true
			}
		}
		return false
	}
	if !contains(8, 9) || contains(5.1, 3.1) || contains(4, 9) {
		t.Fatal("rounded rectangle geometry does not cover the center and exclude clipped corners")
	}
}
