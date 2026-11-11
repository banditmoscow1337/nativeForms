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
	readAtlasAlpha := func(tx, ty int) float32 {
		if tx < atlasBounds.Min.X || tx >= atlasBounds.Max.X || ty < atlasBounds.Min.Y || ty >= atlasBounds.Max.Y {
			return 0
		}
		return float32(renderer.atlas.Pix[renderer.atlas.PixOffset(tx, ty)+3])
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
		sy := float32(atlasBounds.Min.Y) + v*atlasH - 0.5
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
			sx := float32(atlasBounds.Min.X) + u*atlasW - 0.5
			alpha := sampleBilinearScalar(sx, sy, readAtlasAlpha)
			if alpha > 0.5 {
				blend(dst, x, y, c.Tint, coverage*alpha/255)
			}
		}
	}
}

func fillOpaqueSpan(pix []byte, startOffset, count int, r, g, b uint8) {
	endOffset := startOffset + count*4
	row := pix[startOffset:endOffset]
	for i := 0; i < len(row); i += 4 {
		row[i], row[i+1], row[i+2], row[i+3] = r, g, b, 255
	}
}

func (renderer *Renderer) solid(dst *image.RGBA, visible ui.Rect, tint ui.Color, mask float32) {
	left, top, right, bottom := pixels(dst, visible)
	if left >= right || top >= bottom {
		return
	}
	if tint.A >= 1 && mask >= 1 && float32(left) >= visible.X && float32(top) >= visible.Y &&
		float32(right) <= visible.X+visible.W && float32(bottom) <= visible.Y+visible.H {
		r, g, b := byte255(clamp(tint.R, 0, 1)*255), byte255(clamp(tint.G, 0, 1)*255), byte255(clamp(tint.B, 0, 1)*255)
		span := right - left
		firstStart := dst.PixOffset(left, top)
		fillOpaqueSpan(dst.Pix, firstStart, span, r, g, b)
		firstRow := dst.Pix[firstStart : firstStart+span*4]
		for y := top + 1; y < bottom; y++ {
			rowStart := dst.PixOffset(left, y)
			copy(dst.Pix[rowStart:rowStart+span*4], firstRow)
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
	halfMin := min(c.Rect.W, c.Rect.H) * 0.5
	radius := min(max(c.Radius, float32(0)), halfMin)
	if c.Width > 0 {
		renderer.roundedBorder(dst, c, visible, radius, min(c.Width, halfMin))
		return
	}
	if radius <= 0 {
		renderer.solid(dst, visible, c.Tint, 1)
		return
	}
	left, top, right, bottom := pixels(dst, visible)
	if left >= right || top >= bottom {
		return
	}
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

func (renderer *Renderer) roundedBorder(dst *image.RGBA, c ui.DrawCommand, visible ui.Rect, radius, stroke float32) {
	left, top, right, bottom := pixels(dst, visible)
	if left >= right || top >= bottom {
		return
	}
	r2 := radius * radius
	innerLeft, innerRight := c.Rect.X+radius, c.Rect.X+c.Rect.W-radius
	innerTop, innerBottom := c.Rect.Y+radius, c.Rect.Y+c.Rect.H-radius

	inRectX0, inRectX1 := c.Rect.X+stroke, c.Rect.X+c.Rect.W-stroke
	inRectY0, inRectY1 := c.Rect.Y+stroke, c.Rect.Y+c.Rect.H-stroke
	inRadius := max(float32(0), radius-stroke)
	inR2 := inRadius * inRadius
	inArcLeft, inArcRight := inRectX0+inRadius, inRectX1-inRadius
	inArcTop, inArcBottom := inRectY0+inRadius, inRectY1-inRadius

	visRight, visBottom := visible.X+visible.W, visible.Y+visible.H
	for y := top; y < bottom; y++ {
		fy0, fy1 := float32(y), float32(y+1)
		inHollowBandY := fy0 >= inArcTop && fy1 <= inArcBottom
		for x := left; x < right; x++ {
			fx0, fx1 := float32(x), float32(x+1)
			// Fast skip for hollow interior of the rounded border.
			if inHollowBandY && fx0 >= inRectX0 && fx1 <= inRectX1 {
				continue
			}
			if fy0 >= inRectY0 && fy1 <= inRectY1 && fx0 >= inArcLeft && fx1 <= inArcRight {
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
					if dx*dx+dy2 > r2 {
						continue
					}
					// Check if outside inner rounded rect.
					if px >= inRectX0 && px < inRectX1 && py >= inRectY0 && py < inRectY1 {
						if inRadius <= 0 {
							continue
						}
						icx := clamp(px, inArcLeft, inArcRight)
						icy := clamp(py, inArcTop, inArcBottom)
						idx, idy := px-icx, py-icy
						if idx*idx+idy*idy <= inR2 {
							continue
						}
					}
					coverage += 1.0 / 16
				}
			}
			if coverage > 0 {
				blend(dst, x, y, c.Tint, coverage)
			}
		}
	}
}
