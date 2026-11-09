package nativeforms

import (
	"image"
	"math"
)

type scanCrossing struct {
	x   float32
	dir int
}

func segmentBounds(segments []segment) (minX, maxX, minY, maxY float32) {
	minX, maxX = segments[0].a.x, segments[0].a.x
	minY, maxY = segments[0].a.y, segments[0].a.y
	for _, edge := range segments {
		minX, maxX = min(minX, min(edge.a.x, edge.b.x)), max(maxX, max(edge.a.x, edge.b.x))
		minY, maxY = min(minY, min(edge.a.y, edge.b.y)), max(maxY, max(edge.a.y, edge.b.y))
	}
	return minX, maxX, minY, maxY
}

func collectScanCrossings(segments []segment, py float32, dst []scanCrossing) []scanCrossing {
	dst = dst[:0]
	for _, s := range segments {
		if (s.a.y <= py && s.b.y > py) || (s.b.y <= py && s.a.y > py) {
			x := s.a.x + (py-s.a.y)*(s.b.x-s.a.x)/(s.b.y-s.a.y)
			dir := -1
			if s.b.y > s.a.y {
				dir = 1
			}
			dst = append(dst, scanCrossing{x: x, dir: dir})
		}
	}
	return dst
}

func windingHitsAtX(crossings []scanCrossing, x float32) int {
	hits := 0
	for _, subX := range [...]float32{0.25, 0.75} {
		px := x + subX
		winding := 0
		for _, c := range crossings {
			if c.x > px {
				winding += c.dir
			}
		}
		if winding != 0 {
			hits++
		}
	}
	return hits
}

func rasterSegments(segments []segment, left, top, right, bottom int, emit func(x, y, hits int)) {
	var buf0, buf1 [64]scanCrossing
	c0, c1 := buf0[:0], buf1[:0]
	for y := top; y < bottom; y++ {
		c0 = collectScanCrossings(segments, float32(y)+0.25, c0)
		c1 = collectScanCrossings(segments, float32(y)+0.75, c1)
		if len(c0) == 0 && len(c1) == 0 {
			continue
		}
		for x := left; x < right; x++ {
			fx := float32(x)
			hits := windingHitsAtX(c0, fx) + windingHitsAtX(c1, fx)
			if hits > 0 {
				emit(x, y, hits)
			}
		}
	}
}

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
	segments, ok := outlineSegments(o, transform)
	if !ok || len(segments) == 0 {
		return nil, Vec2{}
	}
	minX, maxX, minY, maxY := segmentBounds(segments)
	left, top := int(math.Floor(float64(minX))), int(math.Floor(float64(minY)))
	right, bottom := int(math.Ceil(float64(maxX))), int(math.Ceil(float64(maxY)))
	width, height := right-left, bottom-top
	if width <= 0 || height <= 0 || width > 2048 || height > 2048 || int64(width)*int64(height) > 4<<20 {
		return nil, Vec2{}
	}
	mask := image.NewAlpha(image.Rect(0, 0, width, height))
	rasterSegments(segments, left, top, right, bottom, func(x, y, hits int) {
		mask.Pix[(y-top)*mask.Stride+(x-left)] = uint8(hits * 255 / 4)
	})
	if bold > 0 {
		thick := image.NewAlpha(image.Rect(0, 0, width+bold, height))
		for y := 0; y < height; y++ {
			row := mask.Pix[y*mask.Stride : y*mask.Stride+width]
			dst := thick.Pix[y*thick.Stride : y*thick.Stride+width+bold]
			for x, alpha := range row {
				if alpha == 0 {
					continue
				}
				for shift := 0; shift <= bold; shift++ {
					if dst[x+shift] < alpha {
						dst[x+shift] = alpha
					}
				}
			}
		}
		mask = thick
	}
	return mask, Vec2{X: float32(left), Y: float32(top)}
}
