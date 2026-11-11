package nativeforms

import "math"

type Vertex struct {
	X, Y       float32
	U, V       float32
	R, G, B, A float32
}

// Precomputed quarter-circle unit vectors for segment counts [8..32] so
// tessellateRoundedRect performs zero runtime math.Sin / math.Cos calls.
var roundedQuarterUnit [33][33]Vec2

func init() {
	for segments := 8; segments <= 32; segments++ {
		for i := 0; i <= segments; i++ {
			angle := float64(i) * (math.Pi / 2) / float64(segments)
			roundedQuarterUnit[segments][i] = Vec2{
				X: float32(math.Cos(angle)),
				Y: float32(math.Sin(angle)),
			}
		}
	}
}

func Tessellate(commands []DrawCommand, dst []Vertex) []Vertex {
	dst = dst[:0]
	for i := range commands {
		dst = AppendTessellatedCommand(commands[i], dst)
	}
	return dst
}

// AppendTessellatedCommand appends the tessellated triangles for a single
// DrawCommand directly into dst without an intermediate scratch slice copy.
func AppendTessellatedCommand(c DrawCommand, dst []Vertex) []Vertex {
	switch c.Kind {
	case CommandMask:
		return tessellateMask(c, dst)
	case CommandRoundedRect:
		return tessellateRoundedRect(c, dst)
	case CommandQuad:
		r := intersect(c.Rect, c.Clip)
		if r.Empty() {
			return dst
		}
		u0, v0 := c.UV.X, c.UV.Y
		u1, v1 := c.UV.X+c.UV.W, c.UV.Y+c.UV.H
		if c.UV.W != 0 {
			u0 += c.UV.W * (r.X - c.Rect.X) / c.Rect.W
			u1 -= c.UV.W * (c.Rect.X + c.Rect.W - r.X - r.W) / c.Rect.W
		}
		if c.UV.H != 0 {
			v0 += c.UV.H * (r.Y - c.Rect.Y) / c.Rect.H
			v1 -= c.UV.H * (c.Rect.Y + c.Rect.H - r.Y - r.H) / c.Rect.H
		}
		tr, tg, tb, ta := c.Tint.R, c.Tint.G, c.Tint.B, c.Tint.A
		x0, y0, x1, y1 := r.X, r.Y, r.X+r.W, r.Y+r.H
		return append(dst,
			Vertex{X: x0, Y: y0, U: u0, V: v0, R: tr, G: tg, B: tb, A: ta},
			Vertex{X: x1, Y: y0, U: u1, V: v0, R: tr, G: tg, B: tb, A: ta},
			Vertex{X: x0, Y: y1, U: u0, V: v1, R: tr, G: tg, B: tb, A: ta},
			Vertex{X: x0, Y: y1, U: u0, V: v1, R: tr, G: tg, B: tb, A: ta},
			Vertex{X: x1, Y: y0, U: u1, V: v0, R: tr, G: tg, B: tb, A: ta},
			Vertex{X: x1, Y: y1, U: u1, V: v1, R: tr, G: tg, B: tb, A: ta},
		)
	case CommandLine:
		dx, dy := c.To.X-c.From.X, c.To.Y-c.From.Y
		length := float32(math.Sqrt(float64(dx*dx + dy*dy)))
		const uv = float32(0.5 / fontAtlasSize)
		tr, tg, tb, ta := c.Tint.R, c.Tint.G, c.Tint.B, c.Tint.A
		if length < 0.001 {
			r := intersect(Rect{X: c.From.X - c.Width*0.5, Y: c.From.Y - c.Width*0.5, W: c.Width, H: c.Width}, c.Clip)
			if !r.Empty() {
				x0, y0, x1, y1 := r.X, r.Y, r.X+r.W, r.Y+r.H
				dst = append(dst,
					Vertex{X: x0, Y: y0, U: uv, V: uv, R: tr, G: tg, B: tb, A: ta},
					Vertex{X: x1, Y: y0, U: uv, V: uv, R: tr, G: tg, B: tb, A: ta},
					Vertex{X: x0, Y: y1, U: uv, V: uv, R: tr, G: tg, B: tb, A: ta},
					Vertex{X: x0, Y: y1, U: uv, V: uv, R: tr, G: tg, B: tb, A: ta},
					Vertex{X: x1, Y: y0, U: uv, V: uv, R: tr, G: tg, B: tb, A: ta},
					Vertex{X: x1, Y: y1, U: uv, V: uv, R: tr, G: tg, B: tb, A: ta},
				)
			}
			return dst
		}
		inv := (c.Width * 0.5) / length
		nx, ny := -dy*inv, dx*inv
		p0 := Vec2{X: c.From.X + nx, Y: c.From.Y + ny}
		p1 := Vec2{X: c.To.X + nx, Y: c.To.Y + ny}
		p2 := Vec2{X: c.To.X - nx, Y: c.To.Y - ny}
		p3 := Vec2{X: c.From.X - nx, Y: c.From.Y - ny}
		return emitClippedN(4, [4]Vec2{p0, p1, p2, p3}, c.Clip, uv, c.Tint, dst)
	}
	return dst
}

func appendClippedAxisRect(dst []Vertex, x, y, w, h float32, clip Rect, unclipped bool, uv, tr, tg, tb, ta float32) []Vertex {
	if w <= 0 || h <= 0 {
		return dst
	}
	x0, y0, x1, y1 := x, y, x+w, y+h
	if !unclipped {
		if x0 < clip.X {
			x0 = clip.X
		}
		if y0 < clip.Y {
			y0 = clip.Y
		}
		if cx1 := clip.X + clip.W; x1 > cx1 {
			x1 = cx1
		}
		if cy1 := clip.Y + clip.H; y1 > cy1 {
			y1 = cy1
		}
		if x1 <= x0 || y1 <= y0 {
			return dst
		}
	}
	return append(dst,
		Vertex{X: x0, Y: y0, U: uv, V: uv, R: tr, G: tg, B: tb, A: ta},
		Vertex{X: x1, Y: y0, U: uv, V: uv, R: tr, G: tg, B: tb, A: ta},
		Vertex{X: x1, Y: y1, U: uv, V: uv, R: tr, G: tg, B: tb, A: ta},
		Vertex{X: x0, Y: y0, U: uv, V: uv, R: tr, G: tg, B: tb, A: ta},
		Vertex{X: x1, Y: y1, U: uv, V: uv, R: tr, G: tg, B: tb, A: ta},
		Vertex{X: x0, Y: y1, U: uv, V: uv, R: tr, G: tg, B: tb, A: ta},
	)
}

func quarterUnitPoint(cx, cy, rad float32, u Vec2, quad int) Vec2 {
	switch quad {
	case 0: // top-left: pi .. 3pi/2
		return Vec2{X: cx - rad*u.X, Y: cy - rad*u.Y}
	case 1: // top-right: 3pi/2 .. 2pi
		return Vec2{X: cx + rad*u.Y, Y: cy - rad*u.X}
	case 2: // bottom-right: 0 .. pi/2
		return Vec2{X: cx + rad*u.X, Y: cy + rad*u.Y}
	default: // bottom-left: pi/2 .. pi
		return Vec2{X: cx - rad*u.Y, Y: cy + rad*u.X}
	}
}

func tessellateRoundedRect(c DrawCommand, dst []Vertex) []Vertex {
	if c.Rect.Empty() || c.Clip.Empty() || c.Tint.A <= 0 {
		return dst
	}
	r := c.Rect
	if intersect(r, c.Clip).Empty() {
		return dst
	}
	halfMin := minFloat32(r.W, r.H) * 0.5
	radius := minFloat32(maxFloat32(0, c.Radius), halfMin)
	const uv = float32(0.5 / fontAtlasSize)
	tr, tg, tb, ta := c.Tint.R, c.Tint.G, c.Tint.B, c.Tint.A
	unclipped := r.X >= c.Clip.X && r.Y >= c.Clip.Y && r.X+r.W <= c.Clip.X+c.Clip.W && r.Y+r.H <= c.Clip.Y+c.Clip.H

	if c.Width > 0 {
		stroke := minFloat32(c.Width, halfMin)
		if radius == 0 {
			dst = appendClippedAxisRect(dst, r.X, r.Y, r.W, stroke, c.Clip, unclipped, uv, tr, tg, tb, ta)
			dst = appendClippedAxisRect(dst, r.X, r.Y+r.H-stroke, r.W, stroke, c.Clip, unclipped, uv, tr, tg, tb, ta)
			dst = appendClippedAxisRect(dst, r.X, r.Y+stroke, stroke, r.H-2*stroke, c.Clip, unclipped, uv, tr, tg, tb, ta)
			dst = appendClippedAxisRect(dst, r.X+r.W-stroke, r.Y+stroke, stroke, r.H-2*stroke, c.Clip, unclipped, uv, tr, tg, tb, ta)
			return dst
		}
		segments := max(8, min(32, int(math.Ceil(float64(radius)*math.Pi/2))))
		if need := 24 + 4*segments*6; cap(dst)-len(dst) < need {
			grown := make([]Vertex, len(dst), max(cap(dst)*2, len(dst)+need))
			copy(grown, dst)
			dst = grown
		}
		dst = appendClippedAxisRect(dst, r.X+radius, r.Y, r.W-2*radius, stroke, c.Clip, unclipped, uv, tr, tg, tb, ta)
		dst = appendClippedAxisRect(dst, r.X+radius, r.Y+r.H-stroke, r.W-2*radius, stroke, c.Clip, unclipped, uv, tr, tg, tb, ta)
		dst = appendClippedAxisRect(dst, r.X, r.Y+radius, stroke, r.H-2*radius, c.Clip, unclipped, uv, tr, tg, tb, ta)
		dst = appendClippedAxisRect(dst, r.X+r.W-stroke, r.Y+radius, stroke, r.H-2*radius, c.Clip, unclipped, uv, tr, tg, tb, ta)

		innerR := maxFloat32(0, radius-stroke)
		unitTab := &roundedQuarterUnit[segments]
		centers := [4]Vec2{
			{X: r.X + radius, Y: r.Y + radius},
			{X: r.X + r.W - radius, Y: r.Y + radius},
			{X: r.X + r.W - radius, Y: r.Y + r.H - radius},
			{X: r.X + radius, Y: r.Y + r.H - radius},
		}
		for quad := 0; quad < 4; quad++ {
			cx, cy := centers[quad].X, centers[quad].Y
			u0 := unitTab[0]
			prevOut := quarterUnitPoint(cx, cy, radius, u0, quad)
			if innerR <= 0 {
				center := Vec2{X: cx, Y: cy}
				vCenter := Vertex{X: cx, Y: cy, U: uv, V: uv, R: tr, G: tg, B: tb, A: ta}
				for i := 1; i <= segments; i++ {
					nextOut := quarterUnitPoint(cx, cy, radius, unitTab[i], quad)
					if unclipped {
						dst = append(dst,
							vCenter,
							Vertex{X: prevOut.X, Y: prevOut.Y, U: uv, V: uv, R: tr, G: tg, B: tb, A: ta},
							Vertex{X: nextOut.X, Y: nextOut.Y, U: uv, V: uv, R: tr, G: tg, B: tb, A: ta},
						)
					} else {
						dst = emitClippedN(3, [4]Vec2{center, prevOut, nextOut}, c.Clip, uv, c.Tint, dst)
					}
					prevOut = nextOut
				}
			} else {
				prevIn := quarterUnitPoint(cx, cy, innerR, u0, quad)
				for i := 1; i <= segments; i++ {
					ui := unitTab[i]
					nextOut := quarterUnitPoint(cx, cy, radius, ui, quad)
					nextIn := quarterUnitPoint(cx, cy, innerR, ui, quad)
					if unclipped {
						vPrevIn := Vertex{X: prevIn.X, Y: prevIn.Y, U: uv, V: uv, R: tr, G: tg, B: tb, A: ta}
						vNextOut := Vertex{X: nextOut.X, Y: nextOut.Y, U: uv, V: uv, R: tr, G: tg, B: tb, A: ta}
						dst = append(dst,
							vPrevIn,
							Vertex{X: prevOut.X, Y: prevOut.Y, U: uv, V: uv, R: tr, G: tg, B: tb, A: ta},
							vNextOut,
							vPrevIn,
							vNextOut,
							Vertex{X: nextIn.X, Y: nextIn.Y, U: uv, V: uv, R: tr, G: tg, B: tb, A: ta},
						)
					} else {
						dst = emitClippedN(4, [4]Vec2{prevIn, prevOut, nextOut, nextIn}, c.Clip, uv, c.Tint, dst)
					}
					prevOut, prevIn = nextOut, nextIn
				}
			}
		}
		return dst
	}

	if radius == 0 {
		return appendClippedAxisRect(dst, r.X, r.Y, r.W, r.H, c.Clip, unclipped, uv, tr, tg, tb, ta)
	}
	segments := max(8, min(32, int(math.Ceil(float64(radius)*math.Pi/2))))
	if need := 18 + 4*segments*3; cap(dst)-len(dst) < need {
		grown := make([]Vertex, len(dst), max(cap(dst)*2, len(dst)+need))
		copy(grown, dst)
		dst = grown
	}
	dst = appendClippedAxisRect(dst, r.X+radius, r.Y, r.W-2*radius, r.H, c.Clip, unclipped, uv, tr, tg, tb, ta)
	dst = appendClippedAxisRect(dst, r.X, r.Y+radius, radius, r.H-2*radius, c.Clip, unclipped, uv, tr, tg, tb, ta)
	dst = appendClippedAxisRect(dst, r.X+r.W-radius, r.Y+radius, radius, r.H-2*radius, c.Clip, unclipped, uv, tr, tg, tb, ta)
	unitTab := &roundedQuarterUnit[segments]
	centers := [4]Vec2{
		{X: r.X + radius, Y: r.Y + radius},
		{X: r.X + r.W - radius, Y: r.Y + radius},
		{X: r.X + r.W - radius, Y: r.Y + r.H - radius},
		{X: r.X + radius, Y: r.Y + r.H - radius},
	}
	for quad := 0; quad < 4; quad++ {
		cx, cy := centers[quad].X, centers[quad].Y
		center := Vec2{X: cx, Y: cy}
		vCenter := Vertex{X: cx, Y: cy, U: uv, V: uv, R: tr, G: tg, B: tb, A: ta}
		prev := quarterUnitPoint(cx, cy, radius, unitTab[0], quad)
		for i := 1; i <= segments; i++ {
			next := quarterUnitPoint(cx, cy, radius, unitTab[i], quad)
			if unclipped {
				dst = append(dst,
					vCenter,
					Vertex{X: prev.X, Y: prev.Y, U: uv, V: uv, R: tr, G: tg, B: tb, A: ta},
					Vertex{X: next.X, Y: next.Y, U: uv, V: uv, R: tr, G: tg, B: tb, A: ta},
				)
			} else {
				dst = emitClippedN(3, [4]Vec2{center, prev, next}, c.Clip, uv, c.Tint, dst)
			}
			prev = next
		}
	}
	return dst
}

func clipPolygonAxis(src []Vec2, dst []Vec2, xAxis, minSide bool, bound float32) []Vec2 {
	if len(src) < 3 {
		return dst[:0]
	}
	prev := src[len(src)-1]
	var prevVal float32
	if xAxis {
		prevVal = prev.X
	} else {
		prevVal = prev.Y
	}
	wasIn := prevVal <= bound
	if minSide {
		wasIn = prevVal >= bound
	}
	for _, cur := range src {
		var curVal float32
		if xAxis {
			curVal = cur.X
		} else {
			curVal = cur.Y
		}
		isIn := curVal <= bound
		if minSide {
			isIn = curVal >= bound
		}
		if isIn != wasIn {
			var hit Vec2
			if xAxis {
				t := (bound - prev.X) / (cur.X - prev.X)
				hit = Vec2{X: bound, Y: prev.Y + t*(cur.Y-prev.Y)}
			} else {
				t := (bound - prev.Y) / (cur.Y - prev.Y)
				hit = Vec2{X: prev.X + t*(cur.X-prev.X), Y: bound}
			}
			dst = append(dst, hit)
		}
		if isIn {
			dst = append(dst, cur)
		}
		prev, wasIn = cur, isIn
	}
	return dst
}

func emitClippedN(n int, poly [4]Vec2, r Rect, uv float32, tint Color, dst []Vertex) []Vertex {
	if r.Empty() || n < 3 {
		return dst
	}
	var bufA, bufB [16]Vec2
	a := clipPolygonAxis(poly[:n], bufA[:0], true, true, r.X)
	b := clipPolygonAxis(a, bufB[:0], true, false, r.X+r.W)
	a = clipPolygonAxis(b, bufA[:0], false, true, r.Y)
	b = clipPolygonAxis(a, bufB[:0], false, false, r.Y+r.H)
	if len(b) < 3 {
		return dst
	}
	tr, tg, tb, ta := tint.R, tint.G, tint.B, tint.A
	v0 := Vertex{X: b[0].X, Y: b[0].Y, U: uv, V: uv, R: tr, G: tg, B: tb, A: ta}
	for i := 1; i+1 < len(b); i++ {
		v1 := Vertex{X: b[i].X, Y: b[i].Y, U: uv, V: uv, R: tr, G: tg, B: tb, A: ta}
		v2 := Vertex{X: b[i+1].X, Y: b[i+1].Y, U: uv, V: uv, R: tr, G: tg, B: tb, A: ta}
		dst = append(dst, v0, v1, v2)
	}
	return dst
}
