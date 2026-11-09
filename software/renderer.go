package software

import (
	"fmt"
	"image"
	"math"

	ui "github.com/banditmoscow1337/nativeForms"
	"github.com/banditmoscow1337/nativeForms/render"
)

type Renderer struct {
	atlas  *image.NRGBA
	scaled []ui.DrawCommand
}

// The atlas is only needed when a frame contains a bitmap font quad. Native
// text shapers emit masks, so most desktop windows never need its 16 MiB image.
func New() *Renderer { return &Renderer{} }

func (renderer *Renderer) Kind() render.Kind { return render.Software }
func (renderer *Renderer) Close() error {
	renderer.atlas, renderer.scaled = nil, nil
	return nil
}

// Open selects a renderer for a desktop surface. Native adapters supply their
// hardware constructor when that surface supports one.
func Open(preference render.Kind, hardware render.Constructor) (render.Selection, error) {
	return (render.Factory{Software: func() (render.Backend, error) { return New(), nil },
		Hardware: hardware}).Open(preference)
}

func (renderer *Renderer) RenderScaled(frame ui.Frame, dst *image.RGBA) error {
	if dst == nil || frame.Width <= 0 || frame.Height <= 0 {
		return fmt.Errorf("invalid scaled UI target")
	}
	if dst.Bounds().Dx() == frame.Width && dst.Bounds().Dy() == frame.Height {
		return renderer.Render(frame, dst)
	}
	sx := float32(dst.Bounds().Dx()) / float32(frame.Width)
	sy := float32(dst.Bounds().Dy()) / float32(frame.Height)
	return renderer.renderScaled(frame, dst, sx, sy)
}

// RenderAtScale rasterizes a logical UI frame at the display's pixel scale.
// The target may be one pixel narrower than the scaled logical viewport when
// its physical dimensions are not divisible by the display scale.
func (renderer *Renderer) RenderAtScale(frame ui.Frame, dst *image.RGBA, scale float32) error {
	if dst == nil || frame.Width <= 0 || frame.Height <= 0 || !(scale > 0 && scale <= 8) {
		return fmt.Errorf("invalid scaled UI target")
	}
	if scale == 1 && dst.Bounds().Dx() == frame.Width && dst.Bounds().Dy() == frame.Height {
		return renderer.Render(frame, dst)
	}
	return renderer.renderScaled(frame, dst, scale, scale)
}

func (renderer *Renderer) renderScaled(frame ui.Frame, dst *image.RGBA, sx, sy float32) error {
	scaleRect := func(r ui.Rect) ui.Rect {
		return ui.Rect{X: r.X * sx, Y: r.Y * sy, W: r.W * sx, H: r.H * sy}
	}
	if cap(renderer.scaled) < len(frame.Commands) {
		renderer.scaled = make([]ui.DrawCommand, len(frame.Commands))
	} else {
		renderer.scaled = renderer.scaled[:len(frame.Commands)]
	}
	sMin := min(sx, sy)
	for i, c := range frame.Commands {
		c.Rect, c.Clip = scaleRect(c.Rect), scaleRect(c.Clip)
		c.From = ui.Vec2{X: c.From.X * sx, Y: c.From.Y * sy}
		c.To = ui.Vec2{X: c.To.X * sx, Y: c.To.Y * sy}
		c.Width *= sMin
		c.Radius *= sMin
		renderer.scaled[i] = c
	}
	err := renderer.Render(ui.Frame{Width: dst.Bounds().Dx(), Height: dst.Bounds().Dy(),
		Commands: renderer.scaled}, dst)
	clear(renderer.scaled)
	return err
}

func (renderer *Renderer) Render(frame ui.Frame, dst *image.RGBA) error {
	if dst == nil || dst.Bounds().Min != (image.Point{}) || frame.Width <= 0 || frame.Height <= 0 ||
		dst.Bounds().Dx() != frame.Width || dst.Bounds().Dy() != frame.Height {
		return fmt.Errorf("software target dimensions do not match the UI frame")
	}
	if dst.Stride == 4*frame.Width {
		clear(dst.Pix[:frame.Height*dst.Stride])
	} else {
		for y := 0; y < frame.Height; y++ {
			offset := dst.PixOffset(0, y)
			clear(dst.Pix[offset : offset+4*frame.Width])
		}
	}
	for _, command := range frame.Commands {
		switch command.Kind {
		case ui.CommandMask:
			renderer.mask(dst, command)
		case ui.CommandQuad:
			renderer.quad(dst, command)
		case ui.CommandLine:
			renderer.line(dst, command)
		case ui.CommandImage:
			if command.Filter > ui.FilterBilinear {
				return fmt.Errorf("unsupported image filter: %d", command.Filter)
			}
			renderer.image(dst, command)
		case ui.CommandRoundedRect:
			renderer.roundedRect(dst, command)
		default:
			return fmt.Errorf("unsupported paint command: %d", command.Kind)
		}
	}
	return nil
}

func (renderer *Renderer) quad(dst *image.RGBA, c ui.DrawCommand) {
	visible := intersection(c.Rect, c.Clip)
	if visible.W <= 0 || visible.H <= 0 || c.Tint.A <= 0 {
		return
	}
	// Canvas.Fill samples the white center of the atlas's first texel. It is
	// exactly opaque, so software rendering can fill directly without an atlas.
	const whiteUV = float32(0.5 / 2048) // fontAtlasSize in the UI package
	if c.UV == (ui.Rect{X: whiteUV, Y: whiteUV}) {
		renderer.solid(dst, visible, c.Tint, 1)
		return
	}
	if renderer.atlas == nil {
		renderer.atlas = ui.FontAtlas().(*image.NRGBA)
	}
	left, top, right, bottom := pixels(dst, visible)
	atlasBounds := renderer.atlas.Bounds()
	atlasW, atlasH := float32(atlasBounds.Dx()), float32(atlasBounds.Dy())
	if c.UV.W == 0 && c.UV.H == 0 {
		tx := atlasBounds.Min.X + int(math.Floor(float64(c.UV.X*atlasW)))
		ty := atlasBounds.Min.Y + int(math.Floor(float64(c.UV.Y*atlasH)))
		if tx >= atlasBounds.Min.X && tx < atlasBounds.Max.X && ty >= atlasBounds.Min.Y && ty < atlasBounds.Max.Y {
			renderer.solid(dst, visible, c.Tint, float32(renderer.atlas.Pix[renderer.atlas.PixOffset(tx, ty)+3])/255)
		}
		return
	}
	for y := top; y < bottom; y++ {
		covY := overlap(float32(y), float32(y+1), visible.Y, visible.Y+visible.H)
		if covY <= 0 {
			continue
		}
		py := float32(y) + 0.5
		cy := clamp(py, c.Rect.Y, c.Rect.Y+c.Rect.H)
		v := c.UV.Y
		if c.UV.H != 0 {
			v += (cy - c.Rect.Y) * c.UV.H / c.Rect.H
		}
		ty := atlasBounds.Min.Y + int(math.Floor(float64(v*atlasH)))
		if ty < atlasBounds.Min.Y || ty >= atlasBounds.Max.Y {
			continue
		}
		for x := left; x < right; x++ {
			coverage := overlap(float32(x), float32(x+1), visible.X, visible.X+visible.W) * covY
			if coverage <= 0 {
				continue
			}
			px := float32(x) + 0.5
			cx := clamp(px, c.Rect.X, c.Rect.X+c.Rect.W)
			u := c.UV.X
			if c.UV.W != 0 {
				u += (cx - c.Rect.X) * c.UV.W / c.Rect.W
			}
			tx := atlasBounds.Min.X + int(math.Floor(float64(u*atlasW)))
			if tx < atlasBounds.Min.X || tx >= atlasBounds.Max.X {
				continue
			}
			alpha := renderer.atlas.Pix[renderer.atlas.PixOffset(tx, ty)+3]
			if alpha != 0 {
				blend(dst, x, y, c.Tint, coverage*float32(alpha)/255)
			}
		}
	}
}

func (renderer *Renderer) solid(dst *image.RGBA, visible ui.Rect, tint ui.Color, mask float32) {
	left, top, right, bottom := pixels(dst, visible)
	if tint.A >= 1 && mask >= 1 && float32(left) >= visible.X && float32(top) >= visible.Y &&
		float32(right) <= visible.X+visible.W && float32(bottom) <= visible.Y+visible.H {
		r, g, b := byte255(clamp(tint.R, 0, 1)*255), byte255(clamp(tint.G, 0, 1)*255), byte255(clamp(tint.B, 0, 1)*255)
		for y := top; y < bottom; y++ {
			for x := left; x < right; x++ {
				i := dst.PixOffset(x, y)
				dst.Pix[i], dst.Pix[i+1], dst.Pix[i+2], dst.Pix[i+3] = r, g, b, 255
			}
		}
		return
	}
	for y := top; y < bottom; y++ {
		covY := mask * overlap(float32(y), float32(y+1), visible.Y, visible.Y+visible.H)
		if covY <= 0 {
			continue
		}
		for x := left; x < right; x++ {
			coverage := overlap(float32(x), float32(x+1), visible.X, visible.X+visible.W) * covY
			blend(dst, x, y, tint, coverage)
		}
	}
}

func (renderer *Renderer) roundedRect(dst *image.RGBA, c ui.DrawCommand) {
	visible := intersection(c.Rect, c.Clip)
	if visible.W <= 0 || visible.H <= 0 || c.Tint.A <= 0 {
		return
	}
	radius := min(max(c.Radius, float32(0)), min(c.Rect.W, c.Rect.H)*0.5)
	if radius <= 0 {
		renderer.solid(dst, visible, c.Tint, 1)
		return
	}
	left, top, right, bottom := pixels(dst, visible)
	r2 := radius * radius
	innerLeft, innerRight := c.Rect.X+radius, c.Rect.X+c.Rect.W-radius
	innerTop, innerBottom := c.Rect.Y+radius, c.Rect.Y+c.Rect.H-radius
	visRight, visBottom := visible.X+visible.W, visible.Y+visible.H
	opaque := c.Tint.A >= 1
	tr, tg, tb := byte255(clamp(c.Tint.R, 0, 1)*255), byte255(clamp(c.Tint.G, 0, 1)*255), byte255(clamp(c.Tint.B, 0, 1)*255)

	for y := top; y < bottom; y++ {
		fy0, fy1 := float32(y), float32(y+1)
		inVertBand := fy0 >= innerTop && fy1 <= innerBottom
		inVisY := fy0 >= visible.Y && fy1 <= visBottom
		for x := left; x < right; x++ {
			fx0, fx1 := float32(x), float32(x+1)
			if inVisY && fx0 >= visible.X && fx1 <= visRight &&
				(inVertBand || (fx0 >= innerLeft && fx1 <= innerRight)) {
				if opaque {
					i := dst.PixOffset(x, y)
					dst.Pix[i], dst.Pix[i+1], dst.Pix[i+2], dst.Pix[i+3] = tr, tg, tb, 255
				} else {
					blend(dst, x, y, c.Tint, 1)
				}
				continue
			}
			coverage := float32(0)
			for subY := 0; subY < 4; subY++ {
				py := fy0 + (float32(subY)+0.5)*0.25
				if py < visible.Y || py >= visBottom {
					continue
				}
				cy := clamp(py, innerTop, innerBottom)
				dy2 := (py - cy) * (py - cy)
				for subX := 0; subX < 4; subX++ {
					px := fx0 + (float32(subX)+0.5)*0.25
					if px < visible.X || px >= visRight {
						continue
					}
					cx := clamp(px, innerLeft, innerRight)
					dx := px - cx
					if dx*dx+dy2 <= r2 {
						coverage += 1.0 / 16
					}
				}
			}
			if coverage > 0 {
				blend(dst, x, y, c.Tint, coverage)
			}
		}
	}
}

type imageSample struct {
	r, g, b, a float32
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
	left, top, right, bottom := pixels(dst, visible)
	b := c.Mask.Bounds()
	scaleX, scaleY := float32(b.Dx())/c.Rect.W, float32(b.Dy())/c.Rect.H
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
			blend(dst, x, y, c.Tint, coverage*float32(alpha)/255)
		}
	}
}
