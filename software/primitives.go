package software

import (
	"image"
	"math"

	ui "github.com/banditmoscow1337/nativeForms"
)

type imageSample struct {
	r, g, b, a float32
}

func sampleBilinearScalar(sx, sy float32, read func(x, y int) float32) float32 {
	x0, y0 := int(math.Floor(float64(sx))), int(math.Floor(float64(sy)))
	fx, fy := sx-float32(x0), sy-float32(y0)
	ux, uy := 1-fx, 1-fy
	return (read(x0, y0)*ux+read(x0+1, y0)*fx)*uy +
		(read(x0, y0+1)*ux+read(x0+1, y0+1)*fx)*fy
}

func (renderer *Renderer) image(dst *image.RGBA, c ui.DrawCommand) {
	if c.Image == nil || c.Rect.Empty() || c.Tint.A <= 0 {
		return
	}
	source := c.Source
	if source == (ui.Rect{}) {
		bounds := c.Image.Bounds()
		source = ui.Rect{W: float32(bounds.Dx()), H: float32(bounds.Dy())}
	}
	if source.Empty() {
		return
	}
	visible := intersection(c.Rect, c.Clip)
	if visible.Empty() {
		return
	}
	left, top, right, bottom := pixels(dst, visible)
	bounds := c.Image.Bounds()
	minX := max(0, int(math.Floor(float64(source.X))))
	minY := max(0, int(math.Floor(float64(source.Y))))
	maxX := min(bounds.Dx()-1, int(math.Ceil(float64(source.X+source.W)))-1)
	maxY := min(bounds.Dy()-1, int(math.Ceil(float64(source.Y+source.H)))-1)
	if maxX < minX || maxY < minY {
		return
	}
	read := func(x, y int) imageSample {
		pixel := c.Image.RGBAAt(clampInt(x, minX, maxX), clampInt(y, minY, maxY))
		return imageSample{float32(pixel.R), float32(pixel.G), float32(pixel.B), float32(pixel.A)}
	}
	scaleX, scaleY := source.W/c.Rect.W, source.H/c.Rect.H
	for y := top; y < bottom; y++ {
		covY := overlap(float32(y), float32(y+1), visible.Y, visible.Y+visible.H)
		if covY <= 0 {
			continue
		}
		py := float32(y) + 0.5
		sy := source.Y + (py-c.Rect.Y)*scaleY - 0.5
		for x := left; x < right; x++ {
			px := float32(x) + 0.5
			sx := source.X + (px-c.Rect.X)*scaleX - 0.5
			var pixel imageSample
			if c.Filter == ui.FilterBilinear {
				x0, y0 := int(math.Floor(float64(sx))), int(math.Floor(float64(sy)))
				fx, fy := sx-float32(x0), sy-float32(y0)
				ux, uy := 1-fx, 1-fy
				a, b, cc, d := read(x0, y0), read(x0+1, y0), read(x0, y0+1), read(x0+1, y0+1)
				pixel = imageSample{
					r: (a.r*ux+b.r*fx)*uy + (cc.r*ux+d.r*fx)*fy,
					g: (a.g*ux+b.g*fx)*uy + (cc.g*ux+d.g*fx)*fy,
					b: (a.b*ux+b.b*fx)*uy + (cc.b*ux+d.b*fx)*fy,
					a: (a.a*ux+b.a*fx)*uy + (cc.a*ux+d.a*fx)*fy,
				}
			} else {
				pixel = read(int(math.Floor(float64(sx+0.5))), int(math.Floor(float64(sy+0.5))))
			}
			coverage := overlap(float32(x), float32(x+1), visible.X, visible.X+visible.W) * covY
			blendImage(dst, x, y, pixel, c.Tint, coverage)
		}
	}
}

func clampInt(value, low, high int) int { return min(max(value, low), high) }

func blendImage(dst *image.RGBA, x, y int, source imageSample, tint ui.Color, coverage float32) {
	factor := clamp(tint.A, 0, 1) * clamp(coverage, 0, 1)
	alpha := source.a * factor / 255
	if alpha <= 0 {
		return
	}
	inverse := 1 - alpha
	i := dst.PixOffset(x, y)
	dst.Pix[i+0] = byte255(source.r*clamp(tint.R, 0, 1)*factor + float32(dst.Pix[i+0])*inverse)
	dst.Pix[i+1] = byte255(source.g*clamp(tint.G, 0, 1)*factor + float32(dst.Pix[i+1])*inverse)
	dst.Pix[i+2] = byte255(source.b*clamp(tint.B, 0, 1)*factor + float32(dst.Pix[i+2])*inverse)
	dst.Pix[i+3] = byte255(source.a*factor + float32(dst.Pix[i+3])*inverse)
}

func (renderer *Renderer) line(dst *image.RGBA, c ui.DrawCommand) {
	if c.Width <= 0 || c.Tint.A <= 0 {
		return
	}
	half := c.Width * 0.5
	minX := min(c.From.X, c.To.X) - half
	minY := min(c.From.Y, c.To.Y) - half
	maxX := max(c.From.X, c.To.X) + half
	maxY := max(c.From.Y, c.To.Y) + half
	visible := intersection(ui.Rect{X: minX, Y: minY, W: maxX - minX, H: maxY - minY}, c.Clip)
	if visible.W <= 0 || visible.H <= 0 {
		return
	}
	left, top, right, bottom := pixels(dst, visible)
	dx, dy := c.To.X-c.From.X, c.To.Y-c.From.Y
	length2 := dx*dx + dy*dy
	invLen := float32(0)
	if length2 >= 0.000001 {
		invLen = 1 / float32(math.Sqrt(float64(length2)))
	}
	clipRight, clipBottom := c.Clip.X+c.Clip.W, c.Clip.Y+c.Clip.H
	for y := top; y < bottom; y++ {
		rowLeft, rowRight := left, right
		if invLen > 0 && abs(dy) > 0.001 {
			x0 := c.From.X + (float32(y)-c.From.Y)*dx/dy
			x1 := c.From.X + (float32(y+1)-c.From.Y)*dx/dy
			span := half/(abs(dy)*invLen) + 1
			rowLeft = max(left, int(math.Floor(float64(min(x0, x1)-span))))
			rowRight = min(right, int(math.Ceil(float64(max(x0, x1)+span))))
		}
		for x := rowLeft; x < rowRight; x++ {
			coverage := float32(0)
			for _, sy := range [...]float32{0.25, 0.75} {
				py := float32(y) + sy
				if py < c.Clip.Y || py >= clipBottom {
					continue
				}
				for _, sx := range [...]float32{0.25, 0.75} {
					px := float32(x) + sx
					if px < c.Clip.X || px >= clipRight {
						continue
					}
					if invLen == 0 {
						if abs(px-c.From.X) <= half && abs(py-c.From.Y) <= half {
							coverage += 0.25
						}
						continue
					}
					t := ((px-c.From.X)*dx + (py-c.From.Y)*dy) / length2
					if t < 0 || t > 1 {
						continue
					}
					distance := abs((px-c.From.X)*dy-(py-c.From.Y)*dx) * invLen
					if distance <= half {
						coverage += 0.25
					}
				}
			}
			if coverage > 0 {
				blend(dst, x, y, c.Tint, coverage)
			}
		}
	}
}

func pixels(dst *image.RGBA, r ui.Rect) (left, top, right, bottom int) {
	b := dst.Bounds()
	left = max(b.Min.X, int(math.Floor(float64(r.X))))
	top = max(b.Min.Y, int(math.Floor(float64(r.Y))))
	right = min(b.Max.X, int(math.Ceil(float64(r.X+r.W))))
	bottom = min(b.Max.Y, int(math.Ceil(float64(r.Y+r.H))))
	return
}

func intersection(a, b ui.Rect) ui.Rect {
	x0, y0 := max(a.X, b.X), max(a.Y, b.Y)
	x1, y1 := min(a.X+a.W, b.X+b.W), min(a.Y+a.H, b.Y+b.H)
	return ui.Rect{X: x0, Y: y0, W: max(float32(0), x1-x0), H: max(float32(0), y1-y0)}
}

func overlap(a, b, c, d float32) float32 { return max(float32(0), min(b, d)-max(a, c)) }
func clamp(v, a, b float32) float32      { return min(max(v, a), b) }
func abs(v float32) float32 {
	if v < 0 {
		return -v
	}
	return v
}

func blend(dst *image.RGBA, x, y int, tint ui.Color, mask float32) {
	alpha := clamp(tint.A, 0, 1) * clamp(mask, 0, 1)
	if alpha <= 0 {
		return
	}
	i := dst.PixOffset(x, y)
	inverse := 1 - alpha
	dst.Pix[i+0] = byte255(clamp(tint.R, 0, 1)*alpha*255 + float32(dst.Pix[i+0])*inverse)
	dst.Pix[i+1] = byte255(clamp(tint.G, 0, 1)*alpha*255 + float32(dst.Pix[i+1])*inverse)
	dst.Pix[i+2] = byte255(clamp(tint.B, 0, 1)*alpha*255 + float32(dst.Pix[i+2])*inverse)
	dst.Pix[i+3] = byte255(alpha*255 + float32(dst.Pix[i+3])*inverse)
}

func byte255(v float32) uint8 { return uint8(min(float32(255), v+0.5)) }

func (renderer *Renderer) mask(dst *image.RGBA, c ui.DrawCommand) {
	if c.Mask == nil || c.Mask.Bounds().Empty() || c.Rect.W <= 0 || c.Rect.H <= 0 {
		return
	}
	visible := intersection(c.Rect, c.Clip)
	if visible.W <= 0 || visible.H <= 0 {
		return
	}
	tintA := clamp(c.Tint.A, 0, 1)
	if tintA <= 0 {
		return
	}
	tintR255 := clamp(c.Tint.R, 0, 1) * 255
	tintG255 := clamp(c.Tint.G, 0, 1) * 255
	tintB255 := clamp(c.Tint.B, 0, 1) * 255
	opaqueTint := tintA >= 1
	tr, tg, tb := byte255(tintR255), byte255(tintG255), byte255(tintB255)

	left, top, right, bottom := pixels(dst, visible)
	b := c.Mask.Bounds()
	scaleX, scaleY := float32(b.Dx())/c.Rect.W, float32(b.Dy())/c.Rect.H

	// Fast integer path for 1:1 unscaled integer-aligned glyph masks (the common case for shaped text).
	if opaqueTint && abs(scaleX-1) < 1e-4 && abs(scaleY-1) < 1e-4 &&
		float32(left) >= visible.X && float32(top) >= visible.Y &&
		float32(right) <= visible.X+visible.W && float32(bottom) <= visible.Y+visible.H {
		rx, ry := int(math.Round(float64(c.Rect.X))), int(math.Round(float64(c.Rect.Y)))
		if abs(c.Rect.X-float32(rx)) < 1e-3 && abs(c.Rect.Y-float32(ry)) < 1e-3 {
			trU, tgU, tbU := uint32(tr), uint32(tg), uint32(tb)
			for y := top; y < bottom; y++ {
				sy := b.Min.Y + (y - ry)
				if sy < b.Min.Y || sy >= b.Max.Y {
					continue
				}
				maskRow := c.Mask.Pix[(sy-b.Min.Y)*c.Mask.Stride:]
				dstRow := dst.Pix[y*dst.Stride:]
				for x := left; x < right; x++ {
					sx := b.Min.X + (x - rx)
					if sx < b.Min.X || sx >= b.Max.X {
						continue
					}
					alpha := uint32(maskRow[sx-b.Min.X])
					if alpha == 0 {
						continue
					}
					i := x * 4
					if alpha == 255 {
						dstRow[i], dstRow[i+1], dstRow[i+2], dstRow[i+3] = tr, tg, tb, 255
						continue
					}
					inv := 255 - alpha
					dstRow[i+0] = uint8((trU*alpha + uint32(dstRow[i+0])*inv + 127) / 255)
					dstRow[i+1] = uint8((tgU*alpha + uint32(dstRow[i+1])*inv + 127) / 255)
					dstRow[i+2] = uint8((tbU*alpha + uint32(dstRow[i+2])*inv + 127) / 255)
					dstRow[i+3] = uint8((255*alpha + uint32(dstRow[i+3])*inv + 127) / 255)
				}
			}
			return
		}
	}

	if scaleX > 1.01 || scaleY > 1.01 {
		readMask := func(sx, sy int) float32 {
			if sx < b.Min.X || sx >= b.Max.X || sy < b.Min.Y || sy >= b.Max.Y {
				return 0
			}
			return float32(c.Mask.Pix[(sy-b.Min.Y)*c.Mask.Stride+(sx-b.Min.X)])
		}
		for y := top; y < bottom; y++ {
			covY := overlap(float32(y), float32(y+1), visible.Y, visible.Y+visible.H)
			if covY <= 0 {
				continue
			}
			sy := float32(b.Min.Y) + (float32(y)+0.5-c.Rect.Y)*scaleY - 0.5
			for x := left; x < right; x++ {
				sx := float32(b.Min.X) + (float32(x)+0.5-c.Rect.X)*scaleX - 0.5
				alpha := sampleBilinearScalar(sx, sy, readMask)
				if alpha <= 0.5 {
					continue
				}
				coverage := overlap(float32(x), float32(x+1), visible.X, visible.X+visible.W) * covY
				a := tintA * clamp(coverage*alpha/255, 0, 1)
				if a <= 0 {
					continue
				}
				i := dst.PixOffset(x, y)
				inv := 1 - a
				dst.Pix[i+0] = byte255(tintR255*a + float32(dst.Pix[i+0])*inv)
				dst.Pix[i+1] = byte255(tintG255*a + float32(dst.Pix[i+1])*inv)
				dst.Pix[i+2] = byte255(tintB255*a + float32(dst.Pix[i+2])*inv)
				dst.Pix[i+3] = byte255(a*255 + float32(dst.Pix[i+3])*inv)
			}
		}
		return
	}
	for y := top; y < bottom; y++ {
		covY := overlap(float32(y), float32(y+1), visible.Y, visible.Y+visible.H)
		if covY <= 0 {
			continue
		}
		sy := min(b.Max.Y-1, max(b.Min.Y, b.Min.Y+int((float32(y)+0.5-c.Rect.Y)*scaleY)))
		rowOffset := (sy - b.Min.Y) * c.Mask.Stride
		for x := left; x < right; x++ {
			sx := min(b.Max.X-1, max(b.Min.X, b.Min.X+int((float32(x)+0.5-c.Rect.X)*scaleX)))
			alpha := c.Mask.Pix[rowOffset+(sx-b.Min.X)]
			if alpha == 0 {
				continue
			}
			coverage := overlap(float32(x), float32(x+1), visible.X, visible.X+visible.W) * covY
			if alpha == 255 && opaqueTint && coverage >= 1 {
				i := dst.PixOffset(x, y)
				dst.Pix[i+0], dst.Pix[i+1], dst.Pix[i+2], dst.Pix[i+3] = tr, tg, tb, 255
				continue
			}
			a := tintA * clamp(coverage*float32(alpha)/255, 0, 1)
			if a <= 0 {
				continue
			}
			i := dst.PixOffset(x, y)
			inv := 1 - a
			dst.Pix[i+0] = byte255(tintR255*a + float32(dst.Pix[i+0])*inv)
			dst.Pix[i+1] = byte255(tintG255*a + float32(dst.Pix[i+1])*inv)
			dst.Pix[i+2] = byte255(tintB255*a + float32(dst.Pix[i+2])*inv)
			dst.Pix[i+3] = byte255(a*255 + float32(dst.Pix[i+3])*inv)
		}
	}
}
