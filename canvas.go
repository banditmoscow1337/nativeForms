package nativeforms

import (
	"math"
	"strings"
)

type Vertex struct {
	X, Y       float32
	U, V       float32
	R, G, B, A float32
}

type Canvas struct {
	vertices *[]Vertex
	limit    int
	clips    []Rect
	theme    *Theme
}

func (canvas *Canvas) Theme() *Theme { return canvas.theme }

func (canvas *Canvas) PushClip(rect Rect) {
	if len(canvas.clips) > 0 {
		rect = intersect(canvas.clips[len(canvas.clips)-1], rect)
	}
	canvas.clips = append(canvas.clips, rect)
}

func (canvas *Canvas) PopClip() {
	if len(canvas.clips) > 1 {
		canvas.clips = canvas.clips[:len(canvas.clips)-1]
	}
}

func (canvas *Canvas) Rect(rect Rect, tint Color) {
	const uv = 0.5 / fontAtlasSize
	canvas.quad(rect, Rect{X: uv, Y: uv, W: 0, H: 0}, tint)
}

func (canvas *Canvas) Border(rect Rect, width float32, tint Color) {
	if width <= 0 || rect.Empty() {
		return
	}
	width = minFloat32(width, minFloat32(rect.W, rect.H)*0.5)
	canvas.Rect(Rect{X: rect.X, Y: rect.Y, W: rect.W, H: width}, tint)
	canvas.Rect(Rect{X: rect.X, Y: rect.Y + rect.H - width, W: rect.W, H: width}, tint)
	canvas.Rect(Rect{X: rect.X, Y: rect.Y + width, W: width, H: rect.H - width*2}, tint)
	canvas.Rect(Rect{X: rect.X + rect.W - width, Y: rect.Y + width, W: width, H: rect.H - width*2}, tint)
}

func (canvas *Canvas) Line(from, to Vec2, width float32, tint Color) {
	if width <= 0 || tint.A <= 0 || len(canvas.clips) == 0 {
		return
	}
	var visible bool
	from, to, visible = clipLine(from, to, canvas.clips[len(canvas.clips)-1])
	if !visible {
		return
	}
	dx, dy := to.X-from.X, to.Y-from.Y
	length := float32(math.Sqrt(float64(dx*dx + dy*dy)))
	if length < 0.001 {
		canvas.Rect(Rect{X: from.X - width*0.5, Y: from.Y - width*0.5, W: width, H: width}, tint)
		return
	}
	if len(*canvas.vertices)+6 > canvas.limit {
		return
	}
	nx, ny := -dy/length*width*0.5, dx/length*width*0.5
	const uv = 0.5 / fontAtlasSize
	vertex := func(x, y float32) Vertex {
		return Vertex{X: x, Y: y, U: uv, V: uv, R: tint.R, G: tint.G, B: tint.B, A: tint.A}
	}
	a := vertex(from.X+nx, from.Y+ny)
	b := vertex(to.X+nx, to.Y+ny)
	c := vertex(from.X-nx, from.Y-ny)
	d := vertex(to.X-nx, to.Y-ny)
	*canvas.vertices = append(*canvas.vertices, a, b, c, c, b, d)
}

func clipLine(from, to Vec2, bounds Rect) (Vec2, Vec2, bool) {
	dx, dy := to.X-from.X, to.Y-from.Y
	t0, t1 := float32(0), float32(1)
	clip := func(p, q float32) bool {
		if p == 0 {
			return q >= 0
		}
		ratio := q / p
		if p < 0 {
			if ratio > t1 {
				return false
			}
			if ratio > t0 {
				t0 = ratio
			}
		} else {
			if ratio < t0 {
				return false
			}
			if ratio < t1 {
				t1 = ratio
			}
		}
		return true
	}
	if bounds.Empty() ||
		!clip(-dx, from.X-bounds.X) ||
		!clip(dx, bounds.X+bounds.W-from.X) ||
		!clip(-dy, from.Y-bounds.Y) ||
		!clip(dy, bounds.Y+bounds.H-from.Y) {
		return Vec2{}, Vec2{}, false
	}
	clippedFrom := Vec2{X: from.X + dx*t0, Y: from.Y + dy*t0}
	clippedTo := Vec2{X: from.X + dx*t1, Y: from.Y + dy*t1}
	return clippedFrom, clippedTo, true
}

func (canvas *Canvas) Text(text string, bounds Rect, size float32, align TextAlign, tint Color) {
	if size <= 0 || bounds.Empty() || tint.A <= 0 {
		return
	}
	lineCount := strings.Count(text, "\n") + 1
	lineHeight := size * 1.2
	y := bounds.Y + (bounds.H-lineHeight*float32(lineCount))*0.5
	for lineStart := 0; ; {
		lineEnd := len(text)
		lastLine := true
		if offset := strings.IndexByte(text[lineStart:], '\n'); offset >= 0 {
			lineEnd = lineStart + offset
			lastLine = false
		}
		line := text[lineStart:lineEnd]
		lineWidth := measureTextLine(line, size)
		x := bounds.X
		switch align {
		case TextCenter:
			x += (bounds.W - lineWidth) * 0.5
		case TextRight:
			x += bounds.W - lineWidth
		}
		for _, character := range line {
			glyph := glyphForRune(character)
			if character != ' ' {
				cellX := float32(glyph.cellX*fontCellSize) / fontAtlasSize
				cellY := float32(glyph.cellY*fontCellSize) / fontAtlasSize
				cell := float32(fontCellSize) / fontAtlasSize
				canvas.quad(
					Rect{X: x, Y: y - size*0.12, W: size * float32(fontCellSize) / fontReferenceSize, H: size * float32(fontCellSize) / fontReferenceSize},
					Rect{X: cellX, Y: cellY, W: cell, H: cell},
					tint,
				)
			}
			x += glyph.advance * size
		}
		if lastLine {
			break
		}
		lineStart = lineEnd + 1
		y += lineHeight
	}
}

func (canvas *Canvas) quad(rect, uv Rect, tint Color) {
	if rect.Empty() || tint.A <= 0 || len(canvas.clips) == 0 {
		return
	}
	clipped := intersect(rect, canvas.clips[len(canvas.clips)-1])
	if clipped.Empty() {
		return
	}
	vertices := *canvas.vertices
	if len(vertices)+6 > canvas.limit {
		return
	}

	u0, v0 := uv.X, uv.Y
	u1, v1 := uv.X+uv.W, uv.Y+uv.H
	if uv.W != 0 {
		u0 += uv.W * (clipped.X - rect.X) / rect.W
		u1 -= uv.W * ((rect.X + rect.W) - (clipped.X + clipped.W)) / rect.W
	}
	if uv.H != 0 {
		v0 += uv.H * (clipped.Y - rect.Y) / rect.H
		v1 -= uv.H * ((rect.Y + rect.H) - (clipped.Y + clipped.H)) / rect.H
	}

	vertex := func(x, y, u, v float32) Vertex {
		return Vertex{X: x, Y: y, U: u, V: v, R: tint.R, G: tint.G, B: tint.B, A: tint.A}
	}
	x0, y0 := clipped.X, clipped.Y
	x1, y1 := clipped.X+clipped.W, clipped.Y+clipped.H
	vertices = append(vertices,
		vertex(x0, y0, u0, v0), vertex(x1, y0, u1, v0), vertex(x0, y1, u0, v1),
		vertex(x0, y1, u0, v1), vertex(x1, y0, u1, v0), vertex(x1, y1, u1, v1),
	)
	*canvas.vertices = vertices
}
