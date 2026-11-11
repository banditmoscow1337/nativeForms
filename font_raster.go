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

func sortScanCrossings(crossings []scanCrossing) {
	for i := 1; i < len(crossings); i++ {
		cur := crossings[i]
		j := i - 1
		for j >= 0 && crossings[j].x > cur.x {
			crossings[j+1] = crossings[j]
			j--
		}
		crossings[j+1] = cur
	}
}

// rasterSegments computes smooth 8-bit anti-aliased coverage using 8 vertical
// sub-scanlines per pixel row and exact analytical horizontal span overlap.
func rasterSegments(segments []segment, left, top, right, bottom int, emit func(x, y int, alpha uint8)) {
	width := right - left
	if width <= 0 || bottom <= top {
		return
	}
	var crossingBuf [64]scanCrossing
	var rowStack [256]float32
	var rowCov []float32
	if width <= len(rowStack) {
		rowCov = rowStack[:width]
	} else {
		rowCov = make([]float32, width)
	}
	subOffsets := [...]float32{0.0625, 0.1875, 0.3125, 0.4375, 0.5625, 0.6875, 0.8125, 0.9375}
	for y := top; y < bottom; y++ {
		clear(rowCov)
		hasInk := false
		fy := float32(y)
		for _, sy := range subOffsets {
			c := collectScanCrossings(segments, fy+sy, crossingBuf[:0])
			if len(c) < 2 {
				continue
			}
			sortScanCrossings(c)
			winding := 0
			var spanStart float32
			for _, cr := range c {
				prev := winding
				winding += cr.dir
				if prev == 0 && winding != 0 {
					spanStart = cr.x
				} else if prev != 0 && winding == 0 {
					spanEnd := cr.x
					if spanEnd <= spanStart {
						continue
					}
					x0 := max(left, int(math.Floor(float64(spanStart))))
					x1 := min(right, int(math.Ceil(float64(spanEnd))))
					for x := x0; x < x1; x++ {
						fx0 := maxFloat32(spanStart, float32(x))
						fx1 := minFloat32(spanEnd, float32(x+1))
						if fx1 > fx0 {
							rowCov[x-left] += fx1 - fx0
							hasInk = true
						}
					}
				}
			}
		}
		if !hasInk {
			continue
		}
		for i, cov := range rowCov {
			if cov > 0 {
				norm := cov * (1.0 / 8.0)
				if norm > 1 {
					norm = 1
				}
				// Stem-darkening contrast curve keeps thin unhinted strokes (such as
				// monospace code fonts) crisp and legible at UI sizes.
				norm = norm*0.4 + float32(math.Sqrt(float64(norm)))*0.6
				a := int(norm*255.0 + 0.5)
				if a > 255 {
					a = 255
				}
				if a > 0 {
					emit(left+i, y, uint8(a))
				}
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
	rasterSegments(segments, left, top, right, bottom, func(x, y int, alpha uint8) {
		mask.Pix[(y-top)*mask.Stride+(x-left)] = alpha
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
				if dst[x] < alpha {
					dst[x] = alpha
				}
				for shift := 1; shift <= bold; shift++ {
					// Smooth fractional dilation on trailing edge so synthetic bold stays anti-aliased.
					dilated := alpha
					if shift == bold && bold > 1 {
						dilated = uint8((uint32(alpha) * 180) / 255)
					}
					if dst[x+shift] < dilated {
						dst[x+shift] = dilated
					}
				}
			}
		}
		mask = thick
	}
	return mask, Vec2{X: float32(left), Y: float32(top)}
}
