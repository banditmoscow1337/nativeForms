package software

import (
	"fmt"
	"image"
	"math"

	ui "github.com/banditmoscow1337/nativeForms"
)

// Renderer rasterizes nativeForms paint commands into a premultiplied RGBA
// image. It does not create or present an OS window.
type Renderer struct {
	atlas *image.NRGBA
}

func New() *Renderer { return &Renderer{atlas: ui.FontAtlas().(*image.NRGBA)} }

// Render clears dst and paints a frame. dst must have the frame's dimensions.
// The same Renderer can be reused, but calls must be serialized.
func (renderer *Renderer) Render(frame ui.Frame, dst *image.RGBA) error {
	if dst == nil || dst.Bounds().Min != (image.Point{}) || frame.Width <= 0 || frame.Height <= 0 ||
		dst.Bounds().Dx() != frame.Width || dst.Bounds().Dy() != frame.Height {
		return fmt.Errorf("software target dimensions do not match the UI frame")
	}
	for y:=0; y<frame.Height; y++ {
		offset := dst.PixOffset(0,y)
		clear(dst.Pix[offset:offset+4*frame.Width])
	}
	for _, command := range frame.Commands {
		switch command.Kind {
		case ui.CommandQuad:
			renderer.quad(dst, command)
		case ui.CommandLine:
			renderer.line(dst, command)
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
	left, top, right, bottom := pixels(dst, visible)
	atlasBounds := renderer.atlas.Bounds()
	for y := top; y < bottom; y++ {
		py := float32(y) + 0.5
		cy := clamp(py, c.Rect.Y, c.Rect.Y+c.Rect.H)
		v := c.UV.Y
		if c.UV.H != 0 { v += (cy-c.Rect.Y)*c.UV.H/c.Rect.H }
		for x := left; x < right; x++ {
			px := float32(x) + 0.5
			coverage := overlap(float32(x), float32(x+1), visible.X, visible.X+visible.W) *
				overlap(float32(y), float32(y+1), visible.Y, visible.Y+visible.H)
			if coverage <= 0 { continue }
			cx := clamp(px, c.Rect.X, c.Rect.X+c.Rect.W)
			u := c.UV.X
			if c.UV.W != 0 { u += (cx-c.Rect.X)*c.UV.W/c.Rect.W }
			tx := atlasBounds.Min.X + int(math.Floor(float64(u*float32(atlasBounds.Dx()))))
			ty := atlasBounds.Min.Y + int(math.Floor(float64(v*float32(atlasBounds.Dy()))))
			if tx < atlasBounds.Min.X || tx >= atlasBounds.Max.X || ty < atlasBounds.Min.Y || ty >= atlasBounds.Max.Y {
				continue
			}
			alpha := renderer.atlas.Pix[renderer.atlas.PixOffset(tx,ty)+3]
			blend(dst, x, y, c.Tint, coverage*float32(alpha)/255)
		}
	}
}

func (renderer *Renderer) line(dst *image.RGBA, c ui.DrawCommand) {
	if c.Width <= 0 || c.Tint.A <= 0 { return }
	half := c.Width*0.5
	minX := min(c.From.X, c.To.X)-half
	minY := min(c.From.Y, c.To.Y)-half
	maxX := max(c.From.X, c.To.X)+half
	maxY := max(c.From.Y, c.To.Y)+half
	visible := intersection(ui.Rect{X:minX,Y:minY,W:maxX-minX,H:maxY-minY}, c.Clip)
	if visible.W <= 0 || visible.H <= 0 { return }
	left, top, right, bottom := pixels(dst, visible)
	dx, dy := c.To.X-c.From.X, c.To.Y-c.From.Y
	length2 := dx*dx+dy*dy
	for y:=top; y<bottom; y++ {
		for x:=left; x<right; x++ {
			coverage := float32(0)
			for _, sy := range []float32{0.25, 0.75} {
				for _, sx := range []float32{0.25, 0.75} {
					px, py := float32(x)+sx, float32(y)+sy
					if px < c.Clip.X || px >= c.Clip.X+c.Clip.W ||
						py < c.Clip.Y || py >= c.Clip.Y+c.Clip.H { continue }
					if length2 < 0.000001 {
						if abs(px-c.From.X) <= half && abs(py-c.From.Y) <= half { coverage += 0.25 }
						continue
					}
					t := ((px-c.From.X)*dx+(py-c.From.Y)*dy)/length2
					if t < 0 || t > 1 { continue }
					distance := abs((px-c.From.X)*dy-(py-c.From.Y)*dx)/
						float32(math.Sqrt(float64(length2)))
					if distance <= half { coverage += 0.25 }
				}
			}
			if coverage > 0 { blend(dst, x, y, c.Tint, coverage) }
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

func intersection(a,b ui.Rect) ui.Rect {
	x0,y0 := max(a.X,b.X),max(a.Y,b.Y)
	x1,y1 := min(a.X+a.W,b.X+b.W),min(a.Y+a.H,b.Y+b.H)
	return ui.Rect{X:x0,Y:y0,W:max(float32(0),x1-x0),H:max(float32(0),y1-y0)}
}

func overlap(a,b,c,d float32) float32 { return max(float32(0),min(b,d)-max(a,c)) }
func clamp(v,a,b float32) float32 { return min(max(v,a),b) }
func abs(v float32) float32 { if v < 0 { return -v }; return v }

// image.RGBA stores premultiplied channels; preserve that representation
// when blending a tinted atlas mask over the current pixel.
func blend(dst *image.RGBA, x,y int, tint ui.Color, mask float32) {
	alpha := clamp(tint.A,0,1)*clamp(mask,0,1)
	if alpha <= 0 { return }
	i := dst.PixOffset(x,y)
	inverse := 1-alpha
	dst.Pix[i+0] = byte255(clamp(tint.R,0,1)*alpha*255 + float32(dst.Pix[i+0])*inverse)
	dst.Pix[i+1] = byte255(clamp(tint.G,0,1)*alpha*255 + float32(dst.Pix[i+1])*inverse)
	dst.Pix[i+2] = byte255(clamp(tint.B,0,1)*alpha*255 + float32(dst.Pix[i+2])*inverse)
	dst.Pix[i+3] = byte255(alpha*255 + float32(dst.Pix[i+3])*inverse)
}
func byte255(v float32) uint8 { return uint8(min(float32(255),v+0.5)) }
