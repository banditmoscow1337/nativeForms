package nativeforms

import (
	"image"
	"math"
)

func rasterOutline(o outline, scale float32, italic bool, bold int) (*image.Alpha, Vec2) {
	if len(o.points) == 0 || scale <= 0 {
		return nil, Vec2{}
	}
	transform := func(p fontPoint) floatPoint {
		x := p.x * scale
		if italic {
			x -= p.y * scale * 0.2
		}
		return floatPoint{x: x, y: -p.y * scale}
	}
	var segments []segment
	start := 0
	addCurve := func(a, b, c floatPoint) {
		last := a
		for i := 1; i <= 12; i++ {
			t := float32(i) / 12
			next := floatPoint{
				x: (1-t)*(1-t)*a.x + 2*(1-t)*t*b.x + t*t*c.x,
				y: (1-t)*(1-t)*a.y + 2*(1-t)*t*b.y + t*t*c.y,
			}
			segments = append(segments, segment{last, next})
			last = next
		}
	}
	for _, end := range o.ends {
		if end < start || end >= len(o.points) {
			return nil, Vec2{}
		}
		contour := o.points[start : end+1]
		start = end + 1
		if len(contour) == 0 {
			continue
		}
		last, first := contour[len(contour)-1], contour[0]
		begin := first
		if !first.on {
			if last.on {
				begin = last
			} else {
				begin = fontPoint{x: (first.x + last.x) * 0.5, y: (first.y + last.y) * 0.5, on: true}
			}
		}
		current := transform(begin)
		for i := 0; i < len(contour); {
			next := contour[i]
			if next.on {
				target := transform(next)
				segments = append(segments, segment{current, target})
				current = target
				i++
			} else {
				after := contour[(i+1)%len(contour)]
				if !after.on {
					after = fontPoint{x: (next.x + after.x) * 0.5, y: (next.y + after.y) * 0.5, on: true}
					i++
				} else {
					i += 2
				}
				target := transform(after)
				addCurve(current, transform(next), target)
				current = target
			}
		}
		segments = append(segments, segment{current, transform(begin)})
	}
	if len(segments) == 0 {
		return nil, Vec2{}
	}
	minX, maxX := segments[0].a.x, segments[0].a.x
	minY, maxY := segments[0].a.y, segments[0].a.y
	for _, edge := range segments {
		for _, p := range [...]floatPoint{edge.a, edge.b} {
			minX, maxX = min(minX, p.x), max(maxX, p.x)
			minY, maxY = min(minY, p.y), max(maxY, p.y)
		}
	}
	left, top := int(math.Floor(float64(minX))), int(math.Floor(float64(minY)))
	right, bottom := int(math.Ceil(float64(maxX))), int(math.Ceil(float64(maxY)))
	width, height := right-left, bottom-top
	if width <= 0 || height <= 0 || width > 2048 || height > 2048 || int64(width)*int64(height) > 4<<20 {
		return nil, Vec2{}
	}
	mask := image.NewAlpha(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			hits := 0
			for _, subY := range [...]float32{0.25, 0.75} {
				for _, subX := range [...]float32{0.25, 0.75} {
					px, py := float32(left+x)+subX, float32(top+y)+subY
					winding := 0
					for _, edge := range segments {
						if (edge.a.y <= py && edge.b.y > py) || (edge.b.y <= py && edge.a.y > py) {
							intersection := edge.a.x + (py-edge.a.y)*(edge.b.x-edge.a.x)/(edge.b.y-edge.a.y)
							if intersection > px {
								if edge.b.y > edge.a.y {
									winding++
								} else {
									winding--
								}
							}
						}
					}
					if winding != 0 {
						hits++
					}
				}
			}
			mask.Pix[y*mask.Stride+x] = uint8(hits * 255 / 4)
		}
	}
	if bold > 0 {
		thick := image.NewAlpha(image.Rect(0, 0, width+bold, height))
		for y := 0; y < height; y++ {
			for x := 0; x < width; x++ {
				alpha := mask.Pix[y*mask.Stride+x]
				for shift := 0; shift <= bold; shift++ {
					at := y*thick.Stride + x + shift
					if thick.Pix[at] < alpha {
						thick.Pix[at] = alpha
					}
				}
			}
		}
		mask = thick
	}
	return mask, Vec2{X: float32(left), Y: float32(top)}
}
