package nativeforms

import (
	"image"
	"sync"
	"unicode/utf8"
)

type ShapedText struct {
	Advance float32
	Mask    *image.Alpha
	Offset  Vec2
	Carets  []float32
}
type TextShaper interface {
	Shape(text string, size float32) (ShapedText, bool)
}

// ScaledTextShaper optionally rasterizes glyph masks at a display's pixel
// scale. Advance, caret positions and offsets remain in logical coordinates.
type ScaledTextShaper interface {
	TextShaper
	ShapeAtScale(text string, size, scale float32) (ShapedText, bool)
}

var textShaping struct {
	sync.RWMutex
	engine TextShaper
}

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

func shapeTextRaster(text string, size, scale float32) (ShapedText, float32, bool) {
	textShaping.RLock()
	engine := textShaping.engine
	textShaping.RUnlock()
	if engine == nil {
		return ShapedText{}, 1, false
	}
	if scaled, ok := engine.(ScaledTextShaper); ok && scale > 0 {
		result, success := scaled.ShapeAtScale(text, size, scale)
		return result, scale, success
	}
	result, success := engine.Shape(text, size)
	return result, 1, success
}

func hasTextShaper() bool {
	textShaping.RLock()
	ok := textShaping.engine != nil
	textShaping.RUnlock()
	return ok
}

func TextCaretX(text string, size float32, runeIndex int) float32 {
	if runeIndex <= 0 {
		return 0
	}
	if line, ok := shapeText(text, size); ok {
		count := utf8.RuneCountInString(text)
		runeIndex = min(runeIndex, count)
		if len(line.Carets) == count+1 {
			return line.Carets[runeIndex]
		}
		return measureTextLine(text[:textByteOffset(text, runeIndex)], size)
	}
	return walkAtlasCaretX(text, size, runeIndex)
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
	count := utf8.RuneCountInString(text)
	if shaped, ok := shapeText(text, size); ok {
		if len(shaped.Carets) == count+1 {
			return shaped.Carets
		}
		positions := make([]float32, count+1)
		idx := 0
		for offset := range text {
			if idx > 0 {
				positions[idx] = measureTextLine(text[:offset], size)
			}
			idx++
		}
		if count > 0 {
			positions[count] = measureTextLine(text, size)
		}
		return positions
	}
	positions := make([]float32, count+1)
	walkAtlasText(text, size, positions, nil)
	return positions
}

func textCaretPositionsInto(text string, size float32, dst []float32) []float32 {
	count := utf8.RuneCountInString(text)
	if shaped, ok := shapeText(text, size); ok {
		if len(shaped.Carets) == count+1 {
			return shaped.Carets
		}
		if cap(dst) < count+1 {
			dst = make([]float32, count+1)
		} else {
			dst = dst[:count+1]
		}
		dst[0] = 0
		idx := 0
		for offset := range text {
			if idx > 0 {
				dst[idx] = measureTextLine(text[:offset], size)
			}
			idx++
		}
		if count > 0 {
			dst[count] = measureTextLine(text, size)
		}
		return dst
	}
	if cap(dst) < count+1 {
		dst = make([]float32, count+1)
	} else {
		dst = dst[:count+1]
	}
	walkAtlasText(text, size, dst, nil)
	return dst
}

func textCaretPositionsRunesInto(runes []rune, size float32, dst []float32) []float32 {
	count := len(runes)
	if cap(dst) < count+1 {
		dst = make([]float32, count+1)
	} else {
		dst = dst[:count+1]
	}
	walkAtlasRunes(runes, size, dst, nil)
	return dst
}
