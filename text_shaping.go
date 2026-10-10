package nativeforms

import (
	"image"
	"sync"
)

// ShapedText is a single line at the requested logical font size. Carets are
// indexed by rune boundary. Mask and Carets must remain immutable after return.
type ShapedText struct {
	Advance float32
	Mask    *image.Alpha
	Offset  Vec2 // mask's top-left relative to the line box
	Carets  []float32
}
type TextShaper interface {
	Shape(text string, size float32) (ShapedText, bool)
}

var textShaping struct {
	sync.RWMutex
	engine TextShaper
}

// SetTextShaper installs native shaping before constructing UI trees. Passing
// nil restores the portable TrueType atlas. Existing managers need layout
// invalidation if the shaper is changed while they are running.
func SetTextShaper(engine TextShaper) {
	textShaping.Lock()
	textShaping.engine = engine
	textShaping.Unlock()
}
func shapeText(text string, size float32) (ShapedText, bool) {
	textShaping.RLock()
	engine := textShaping.engine
	textShaping.RUnlock()
	if engine == nil {
		return ShapedText{}, false
	}
	return engine.Shape(text, size)
}

// TextCaretX measures a caret in the context of the whole shaped line, including
// kerning, ligatures and right-to-left runs. runeIndex is a rune boundary.
func TextCaretX(text string, size float32, runeIndex int) float32 {
	r := []rune(text)
	runeIndex = min(max(0, runeIndex), len(r))
	if line, ok := shapeText(text, size); ok {
		if len(line.Carets) == len(r)+1 {
			return line.Carets[runeIndex]
		}
		return measureTextLine(string(r[:runeIndex]), size)
	}
	positions := make([]float32, len(r)+1)
	walkAtlasText(text, size, positions, nil)
	return positions[runeIndex]
}
func (canvas *Canvas) mask(mask *image.Alpha, rect Rect, tint Color) {
	if mask == nil || mask.Bounds().Empty() || rect.Empty() || len(canvas.clips) == 0 {
		return
	}
	clip := canvas.clips[len(canvas.clips)-1]
	if intersect(rect, clip).Empty() {
		return
	}
	*canvas.commands = append(*canvas.commands, DrawCommand{Kind: CommandMask, Mask: mask, Rect: rect, Clip: clip, Tint: tint})
}

// The Vulkan adapter's fixed atlas can also present shaped masks as constant
// alpha spans. Software consumes the mask directly. This fallback trades
// vertex count for keeping the renderer's existing single-texture contract.
func tessellateMask(c DrawCommand, dst []Vertex) []Vertex {
	if c.Mask == nil || c.Mask.Bounds().Empty() || c.Rect.Empty() {
		return dst
	}
	b := c.Mask.Bounds()
	sx, sy := c.Rect.W/float32(b.Dx()), c.Rect.H/float32(b.Dy())
	uv := float32(0.5 / fontAtlasSize)
	for y := b.Min.Y; y < b.Max.Y; y++ {
		py := c.Rect.Y + float32(y-b.Min.Y)*sy
		if py+sy <= c.Clip.Y || py >= c.Clip.Y+c.Clip.H {
			continue
		}
		for x := b.Min.X; x < b.Max.X; {
			alpha := c.Mask.Pix[c.Mask.PixOffset(x, y)]
			end := x + 1
			for end < b.Max.X && c.Mask.Pix[c.Mask.PixOffset(end, y)] == alpha {
				end++
			}
			if alpha != 0 {
				r := intersect(Rect{X: c.Rect.X + float32(x-b.Min.X)*sx, Y: py, W: float32(end-x) * sx, H: sy}, c.Clip)
				if !r.Empty() {
					a := c.Tint.A * float32(alpha) / 255
					v := func(x, y float32) Vertex {
						return Vertex{X: x, Y: y, U: uv, V: uv, R: c.Tint.R, G: c.Tint.G, B: c.Tint.B, A: a}
					}
					dst = append(dst, v(r.X, r.Y), v(r.X+r.W, r.Y), v(r.X, r.Y+r.H), v(r.X, r.Y+r.H), v(r.X+r.W, r.Y), v(r.X+r.W, r.Y+r.H))
				}
			}
			x = end
		}
	}
	return dst
}

func textCaretPositions(text string, size float32) []float32 {
	count := len([]rune(text))
	if shaped, ok := shapeText(text, size); ok {
		if len(shaped.Carets) == count+1 {
			return shaped.Carets
		}
		runes := []rune(text)
		positions := make([]float32, count+1)
		for i := 1; i <= count; i++ {
			positions[i] = measureTextLine(string(runes[:i]), size)
		}
		return positions
	}
	positions := make([]float32, count+1)
	walkAtlasText(text, size, positions, nil)
	return positions
}
