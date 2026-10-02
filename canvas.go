package nativeforms

import (
	"math"
	"strings"
)

type CommandKind uint8

const (
	CommandQuad CommandKind = iota
	CommandLine
)

// DrawCommand is an ordered paint operation in logical coordinates.
// Quad UVs address the shared font atlas; its white texel draws solid shapes.
type DrawCommand struct {
	Kind  CommandKind
	Rect  Rect
	UV    Rect
	Clip  Rect
	Tint  Color
	From  Vec2
	To    Vec2
	Width float32
}

type Canvas struct {
	commands *[]DrawCommand
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
	canvas.quad(rect, Rect{X: uv, Y: uv}, tint)
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
	clip := canvas.clips[len(canvas.clips)-1]
	if clip.Empty() {
		return
	}
	// Preserve strokes whose centerline lies just outside the clip.
	expanded := Rect{X: clip.X - width*0.5, Y: clip.Y - width*0.5, W: clip.W + width, H: clip.H + width}
	var visible bool
	from, to, visible = clipLine(from, to, expanded)
	if visible {
		*canvas.commands = append(*canvas.commands, DrawCommand{
			Kind: CommandLine, From: from, To: to, Width: width, Clip: clip, Tint: tint,
		})
	}
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
	return Vec2{X: from.X + dx*t0, Y: from.Y + dy*t0},
		Vec2{X: from.X + dx*t1, Y: from.Y + dy*t1}, true
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
					Rect{X: cellX, Y: cellY, W: cell, H: cell}, tint,
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
	clip := canvas.clips[len(canvas.clips)-1]
	if !intersect(rect, clip).Empty() {
		*canvas.commands = append(*canvas.commands, DrawCommand{
			Kind: CommandQuad, Rect: rect, UV: uv, Clip: clip, Tint: tint,
		})
	}
}

// Vertex is the format consumed by the optional Vulkan backend.
type Vertex struct {
	X, Y       float32
	U, V       float32
	R, G, B, A float32
}

// Tessellate returns all vertices in paint order without silently truncating.
func Tessellate(commands []DrawCommand, dst []Vertex) []Vertex {
	dst = dst[:0]
	for _, c := range commands {
		if c.Kind == CommandQuad {
			r := intersect(c.Rect, c.Clip)
			if r.Empty() {
				continue
			}
			u0, v0 := c.UV.X, c.UV.Y
			u1, v1 := c.UV.X+c.UV.W, c.UV.Y+c.UV.H
			if c.UV.W != 0 {
				u0 += c.UV.W * (r.X-c.Rect.X) / c.Rect.W
				u1 -= c.UV.W * (c.Rect.X+c.Rect.W-r.X-r.W) / c.Rect.W
			}
			if c.UV.H != 0 {
				v0 += c.UV.H * (r.Y-c.Rect.Y) / c.Rect.H
				v1 -= c.UV.H * (c.Rect.Y+c.Rect.H-r.Y-r.H) / c.Rect.H
			}
			v := func(x, y, u, vv float32) Vertex {
				return Vertex{X: x, Y: y, U: u, V: vv, R: c.Tint.R, G: c.Tint.G, B: c.Tint.B, A: c.Tint.A}
			}
			x0, y0, x1, y1 := r.X, r.Y, r.X+r.W, r.Y+r.H
			dst = append(dst, v(x0, y0, u0, v0), v(x1, y0, u1, v0), v(x0, y1, u0, v1),
				v(x0, y1, u0, v1), v(x1, y0, u1, v0), v(x1, y1, u1, v1))
		} else if c.Kind == CommandLine {
			dx, dy := c.To.X-c.From.X, c.To.Y-c.From.Y
			length := float32(math.Sqrt(float64(dx*dx + dy*dy)))
			uv := float32(0.5 / fontAtlasSize)
			v := func(x, y float32) Vertex {
				return Vertex{X: x, Y: y, U: uv, V: uv, R: c.Tint.R, G: c.Tint.G, B: c.Tint.B, A: c.Tint.A}
			}
			if length < 0.001 {
				r := intersect(Rect{X: c.From.X-c.Width*0.5, Y: c.From.Y-c.Width*0.5, W: c.Width, H: c.Width}, c.Clip)
				if !r.Empty() {
					dst = append(dst, v(r.X, r.Y), v(r.X+r.W, r.Y), v(r.X, r.Y+r.H),
						v(r.X, r.Y+r.H), v(r.X+r.W, r.Y), v(r.X+r.W, r.Y+r.H))
				}
				continue
			}
			nx, ny := -dy/length*c.Width*0.5, dx/length*c.Width*0.5
			polygon := []Vec2{
				{X: c.From.X+nx, Y: c.From.Y+ny},
				{X: c.To.X+nx, Y: c.To.Y+ny},
				{X: c.To.X-nx, Y: c.To.Y-ny},
				{X: c.From.X-nx, Y: c.From.Y-ny},
			}
			polygon = clipPolygon(polygon, c.Clip)
			for i := 1; i+1 < len(polygon); i++ {
				dst = append(dst, v(polygon[0].X, polygon[0].Y),
					v(polygon[i].X, polygon[i].Y),
					v(polygon[i+1].X, polygon[i+1].Y))
			}
		}
	}
	return dst
}

func clipPolygon(points []Vec2, r Rect) []Vec2 {
	if r.Empty() { return nil }
	type edge struct {
		inside func(Vec2) bool
		at     func(Vec2, Vec2) Vec2
	}
	edges := []edge{
		{func(p Vec2) bool { return p.X >= r.X }, func(a, b Vec2) Vec2 {
			t := (r.X-a.X)/(b.X-a.X); return Vec2{X: r.X, Y: a.Y+t*(b.Y-a.Y)}
		}},
		{func(p Vec2) bool { return p.X <= r.X+r.W }, func(a, b Vec2) Vec2 {
			x := r.X+r.W; t := (x-a.X)/(b.X-a.X); return Vec2{X: x, Y: a.Y+t*(b.Y-a.Y)}
		}},
		{func(p Vec2) bool { return p.Y >= r.Y }, func(a, b Vec2) Vec2 {
			t := (r.Y-a.Y)/(b.Y-a.Y); return Vec2{X: a.X+t*(b.X-a.X), Y: r.Y}
		}},
		{func(p Vec2) bool { return p.Y <= r.Y+r.H }, func(a, b Vec2) Vec2 {
			y := r.Y+r.H; t := (y-a.Y)/(b.Y-a.Y); return Vec2{X: a.X+t*(b.X-a.X), Y: y}
		}},
	}
	for _, e := range edges {
		if len(points) == 0 { break }
		out := make([]Vec2, 0, len(points)+2)
		previous := points[len(points)-1]
		wasInside := e.inside(previous)
		for _, current := range points {
			isInside := e.inside(current)
			if isInside != wasInside { out = append(out, e.at(previous, current)) }
			if isInside { out = append(out, current) }
			previous, wasInside = current, isInside
		}
		points = out
	}
	return points
}
