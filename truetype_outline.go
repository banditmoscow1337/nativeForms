package nativeforms

import (
	"fmt"
	"image"
	"image/color"
	"math"
)

type fontPoint struct {
	x, y float32
	on   bool
}

type outline struct {
	points []fontPoint
	ends   []int
}

func (f *TrueTypeFace) canDrawGlyph(id uint16) bool {
	o, err := f.glyph(id, 0)
	if err != nil {
		return false
	}
	if len(o.points) == 0 {
		return true
	}
	for start, i := 0, 0; i < len(o.ends); i++ {
		end := o.ends[i]
		if end < start || end >= len(o.points) {
			return false
		}
		start = end + 1
	}
	return true
}

func (f *TrueTypeFace) glyph(id uint16, depth int) (outline, error) {
	if id >= f.glyphCount || depth > 8 {
		return outline{}, fmt.Errorf("invalid composite glyph")
	}
	loca, glyf := f.table("loca", f.loca), f.table("glyf", f.glyf)
	var start, end uint32
	if f.locaFormat == 0 {
		start = uint32(u16(loca, int(id)*2)) * 2
		end = uint32(u16(loca, (int(id)+1)*2)) * 2
	} else {
		start = u32(loca, int(id)*4)
		end = u32(loca, (int(id)+1)*4)
	}
	if start == end {
		return outline{}, nil
	}
	if end < start || uint64(end) > uint64(len(glyf)) || end-start < 10 {
		return outline{}, fmt.Errorf("invalid glyph range")
	}
	b := glyf[start:end]
	count := int(i16(b, 0))
	if count >= 0 {
		return parseSimpleGlyph(b, count)
	}
	result := outline{}
	p := 10
	for component := 0; component < 64; component++ {
		if p+4 > len(b) {
			return outline{}, fmt.Errorf("truncated composite")
		}
		flags, id2 := u16(b, p), u16(b, p+2)
		p += 4
		if flags&0x0002 == 0 {
			return outline{}, fmt.Errorf("point-aligned composites are unsupported")
		}
		var x, y float32
		if flags&0x0001 != 0 {
			if p+4 > len(b) {
				return outline{}, fmt.Errorf("truncated component offset")
			}
			x, y = float32(i16(b, p)), float32(i16(b, p+2))
			p += 4
		} else {
			if p+2 > len(b) {
				return outline{}, fmt.Errorf("truncated component offset")
			}
			x, y = float32(int8(b[p])), float32(int8(b[p+1]))
			p += 2
		}
		a, d, bx, cy := float32(1), float32(1), float32(0), float32(0)
		readScale := func() (float32, error) {
			if p+2 > len(b) {
				return 0, fmt.Errorf("truncated component transform")
			}
			v := float32(i16(b, p)) / 16384
			p += 2
			return v, nil
		}
		if flags&0x0008 != 0 {
			v, e := readScale()
			if e != nil {
				return outline{}, e
			}
			a, d = v, v
		} else if flags&0x0040 != 0 {
			var e error
			if a, e = readScale(); e != nil {
				return outline{}, e
			}
			if d, e = readScale(); e != nil {
				return outline{}, e
			}
		} else if flags&0x0080 != 0 {
			for _, dst := range [...]*float32{&a, &bx, &cy, &d} {
				v, e := readScale()
				if e != nil {
					return outline{}, e
				}
				*dst = v
			}
		}
		child, e := f.glyph(id2, depth+1)
		if e != nil {
			return outline{}, e
		}
		base := len(result.points)
		for _, q := range child.points {
			result.points = append(result.points, fontPoint{x: a*q.x + bx*q.y + x, y: cy*q.x + d*q.y + y, on: q.on})
		}
		for _, v := range child.ends {
			result.ends = append(result.ends, base+v)
		}
		if flags&0x0020 == 0 {
			return result, nil
		}
	}
	return outline{}, fmt.Errorf("too many glyph components")
}

func parseSimpleGlyph(b []byte, count int) (outline, error) {
	if count > 1024 || 10+count*2+2 > len(b) {
		return outline{}, fmt.Errorf("invalid contour count")
	}
	if count == 0 {
		return outline{}, nil
	}
	o := outline{ends: make([]int, count)}
	p := 10
	for i := range o.ends {
		o.ends[i] = int(u16(b, p))
		p += 2
		if i > 0 && o.ends[i] <= o.ends[i-1] {
			return outline{}, fmt.Errorf("invalid contour endpoints")
		}
	}
	n := o.ends[count-1] + 1
	if n > 32768 || p+2 > len(b) {
		return outline{}, fmt.Errorf("invalid point count")
	}
	instructions := int(u16(b, p))
	p += 2 + instructions
	if p > len(b) {
		return outline{}, fmt.Errorf("truncated glyph instructions")
	}
	flags := make([]byte, 0, n)
	for len(flags) < n {
		if p >= len(b) {
			return outline{}, fmt.Errorf("truncated glyph flags")
		}
		flag := b[p]
		p++
		repeat := 1
		if flag&8 != 0 {
			if p >= len(b) {
				return outline{}, fmt.Errorf("truncated repeat flag")
			}
			repeat += int(b[p])
			p++
		}
		if repeat > n-len(flags) {
			return outline{}, fmt.Errorf("invalid glyph flags")
		}
		for j := 0; j < repeat; j++ {
			flags = append(flags, flag)
		}
	}
	o.points = make([]fontPoint, n)
	readCoordinates := func(xAxis bool) error {
		value := int32(0)
		short, positive := byte(2), byte(16)
		if !xAxis {
			short, positive = 4, 32
		}
		for i, flag := range flags {
			delta := int32(0)
			if flag&short != 0 {
				if p >= len(b) {
					return fmt.Errorf("truncated glyph coordinates")
				}
				delta = int32(b[p])
				p++
				if flag&positive == 0 {
					delta = -delta
				}
			} else if flag&positive == 0 {
				if p+2 > len(b) {
					return fmt.Errorf("truncated glyph coordinates")
				}
				delta = int32(i16(b, p))
				p += 2
			}
			value += delta
			if xAxis {
				o.points[i].x = float32(value)
				o.points[i].on = flag&1 != 0
			} else {
				o.points[i].y = float32(value)
			}
		}
		return nil
	}
	if err := readCoordinates(true); err != nil {
		return outline{}, err
	}
	if err := readCoordinates(false); err != nil {
		return outline{}, err
	}
	return o, nil
}

type floatPoint struct{ x, y float32 }
type segment struct{ a, b floatPoint }

// outlineSegments flattens TrueType contours after applying the caller's transform.
func outlineSegments(o outline, transform func(fontPoint) floatPoint) ([]segment, bool) {
	var segments []segment
	start := 0
	addCurve := func(a, b, c floatPoint) {
		last := a
		for i := 1; i <= 12; i++ {
			t := float32(i) / 12
			u := 1 - t
			next := floatPoint{
				x: u*u*a.x + 2*u*t*b.x + t*t*c.x,
				y: u*u*a.y + 2*u*t*b.y + t*t*c.y,
			}
			segments = append(segments, segment{last, next})
			last = next
		}
	}
	for _, end := range o.ends {
		if end < start || end >= len(o.points) {
			return nil, false
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
	return segments, true
}

func (f *TrueTypeFace) drawGlyph(atlas *image.NRGBA, id uint16, cellX, cellY int) error {
	o, err := f.glyph(id, 0)
	if err != nil {
		return err
	}
	if len(o.points) == 0 {
		return nil
	}
	scale := float32(fontReferenceSize) / float32(f.units)
	transform := func(p fontPoint) floatPoint {
		return floatPoint{x: float32(cellX) + 5 + p.x*scale, y: float32(cellY) + 52 - p.y*scale}
	}
	segments, ok := outlineSegments(o, transform)
	if !ok {
		return fmt.Errorf("invalid glyph outline")
	}
	if len(segments) == 0 {
		return nil
	}
	minX, maxX, minY, maxY := segmentBounds(segments)
	left := max(cellX, int(math.Floor(float64(minX))))
	top := max(cellY, int(math.Floor(float64(minY))))
	right := min(cellX+fontCellSize, int(math.Ceil(float64(maxX))))
	bottom := min(cellY+fontCellSize, int(math.Ceil(float64(maxY))))
	rasterSegments(segments, left, top, right, bottom, func(x, y int, alpha uint8) {
		atlas.SetNRGBA(x, y, color.NRGBA{R: 255, G: 255, B: 255, A: alpha})
	})
	return nil
}
