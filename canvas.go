package nativeforms

import (
	"image"
	"math"
	"strings"
	"unicode/utf8"
)

type CommandKind uint8

const (
	CommandQuad CommandKind = iota
	CommandLine
	CommandMask
	CommandImage
	CommandRoundedRect
)

type ImageFilter uint8

const (
	FilterNearest ImageFilter = iota
	FilterBilinear
)

type DrawCommand struct {
	Kind   CommandKind
	Rect   Rect
	UV     Rect
	Clip   Rect
	Tint   Color
	From   Vec2
	To     Vec2
	Width  float32
	Mask   *image.Alpha
	Image  *ImageResource
	Source Rect
	Filter ImageFilter
	Radius float32
}

type Canvas struct {
	commands     *[]DrawCommand
	clips        []Rect
	theme        *Theme
	fonts        *FontContext
	displayScale float32
	trace        *FrameTrace
}

func (canvas *Canvas) Theme() *Theme { return canvas.theme }

func (canvas *Canvas) MeasureText(text string, spec FontSpec) Vec2 {
	if canvas.fonts != nil {
		if spec.Scale <= 0 && canvas.displayScale > 0 {
			spec.Scale = canvas.displayScale
		}
		return canvas.fonts.Measure(text, spec)
	}
	return MeasureText(text, spec.Size)
}

func (canvas *Canvas) TextStyled(text string, bounds Rect, spec FontSpec, align TextAlign, tint Color) {
	if canvas.fonts == nil {
		canvas.Text(text, bounds, spec.Size, align, tint)
		return
	}
	if bounds.Empty() || tint.A <= 0 {
		return
	}
	explicitScale := spec.Scale > 0
	spec = normalFontSpec(spec)
	if !explicitScale && canvas.displayScale > 0 {
		spec.Scale = min(canvas.displayScale, 8)
	}
	text = normalizeTextBreaks(text)
	if canvas.trace != nil {
		canvas.trace.sample.TextRunes += utf8.RuneCountInString(text)
	}
	canvas.PushClip(bounds)

	if strings.IndexByte(text, '\n') < 0 {
		start := canvas.trace.Start()
		line := canvas.fonts.layoutShared(text, spec)
		canvas.trace.Record(PhaseTextLayout, start)
		y := bounds.Y + (bounds.H-line.Height())*0.5
		if line.Height() > bounds.H {
			y = bounds.Y
		}
		canvas.paintTextLine(line, bounds, align, tint, y)
		canvas.PopClip()
		return
	}
	var stackLines [16]TextLayout
	lines := stackLines[:0]
	height := float32(0)
	textStart := canvas.trace.Start()
	for start := 0; ; {
		end := strings.IndexByte(text[start:], '\n')
		var part string
		if end < 0 {
			part = text[start:]
		} else {
			part = text[start : start+end]
		}
		line := canvas.fonts.layoutShared(part, spec)
		lines = append(lines, line)
		height += line.Height()
		if end < 0 {
			break
		}
		start += end + 1
	}
	canvas.trace.Record(PhaseTextLayout, textStart)
	y := bounds.Y + (bounds.H-height)*0.5
	if height > bounds.H {
		y = bounds.Y
	}
	for _, line := range lines {
		canvas.paintTextLine(line, bounds, align, tint, y)
		y += line.Height()
	}
	canvas.PopClip()
}

func alignLineX(bounds Rect, lineWidth float32, align TextAlign) float32 {
	switch align {
	case TextCenter:
		if lineWidth <= bounds.W {
			return bounds.X + (bounds.W-lineWidth)*0.5
		}
	case TextRight:
		return bounds.X + bounds.W - lineWidth
	}
	return bounds.X
}

func (canvas *Canvas) paintTextLine(line TextLayout, bounds Rect, align TextAlign, tint Color, y float32) {
	if canvas.trace != nil {
		canvas.trace.sample.Glyphs += len(line.Glyphs)
	}
	if len(line.Glyphs) == 0 || len(canvas.clips) == 0 {
		return
	}
	clip := canvas.clips[len(canvas.clips)-1]
	if clip.Empty() {
		return
	}
	x := alignLineX(bounds, line.Advance, align)
	scale := canvas.displayScale
	if !(scale > 0 && scale <= 8) {
		scale = 1
	}
	x = float32(math.Round(float64(x*scale))) / scale
	y = float32(math.Round(float64(y*scale))) / scale
	cx1, cy1 := clip.X+clip.W, clip.Y+clip.H
	cmds := *canvas.commands
	for i := range line.Glyphs {
		glyph := line.Glyphs[i]
		mask := glyph.Mask
		if mask == nil {
			continue
		}
		gw, gh := glyph.W, glyph.H
		if gw <= 0 || gh <= 0 {
			b := mask.Bounds()
			if b.Empty() {
				continue
			}
			gw = float32(b.Dx()) / glyph.Scale
			gh = float32(b.Dy()) / glyph.Scale
		}
		gx, gy := x+glyph.X, y+glyph.Y
		if gx+gw <= clip.X || gy+gh <= clip.Y || gx >= cx1 || gy >= cy1 {
			continue
		}
		cmds = append(cmds, DrawCommand{
			Kind: CommandMask, Mask: mask,
			Rect: Rect{X: gx, Y: gy, W: gw, H: gh},
			Clip: clip, Tint: tint,
		})
	}
	*canvas.commands = cmds
}

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

func (canvas *Canvas) RoundedRect(rect Rect, radius float32, tint Color) {
	if radius <= 0 {
		canvas.Rect(rect, tint)
		return
	}
	if rect.Empty() || tint.A <= 0 || len(canvas.clips) == 0 {
		return
	}
	clip := canvas.clips[len(canvas.clips)-1]
	if intersect(rect, clip).Empty() {
		return
	}
	radius = minFloat32(radius, minFloat32(rect.W, rect.H)*0.5)
	*canvas.commands = append(*canvas.commands, DrawCommand{
		Kind: CommandRoundedRect, Rect: rect, Clip: clip, Radius: radius, Tint: tint,
	})
}

func (canvas *Canvas) RoundedBorder(rect Rect, radius, width float32, tint Color) {
	if width <= 0 || rect.Empty() || tint.A <= 0 {
		return
	}
	if radius <= 0 {
		canvas.Border(rect, width, tint)
		return
	}
	if len(canvas.clips) == 0 {
		return
	}
	clip := canvas.clips[len(canvas.clips)-1]
	if intersect(rect, clip).Empty() {
		return
	}
	halfMin := minFloat32(rect.W, rect.H) * 0.5
	radius = minFloat32(radius, halfMin)
	width = minFloat32(width, halfMin)
	*canvas.commands = append(*canvas.commands, DrawCommand{
		Kind: CommandRoundedRect, Rect: rect, Clip: clip, Radius: radius, Width: width, Tint: tint,
	})
}

func (canvas *Canvas) Image(resource *ImageResource, dest, source Rect, tint Color, filter ImageFilter) {
	if resource == nil || dest.Empty() || tint.A <= 0 || len(canvas.clips) == 0 {
		return
	}
	if source == (Rect{}) {
		source = Rect{W: float32(resource.width), H: float32(resource.height)}
	}
	if source.Empty() {
		return
	}
	visibleSource := intersect(source, Rect{W: float32(resource.width), H: float32(resource.height)})
	if visibleSource.Empty() {
		return
	}

	fullDest := dest
	dest = Rect{
		X: fullDest.X + (visibleSource.X-source.X)*fullDest.W/source.W,
		Y: fullDest.Y + (visibleSource.Y-source.Y)*fullDest.H/source.H,
		W: visibleSource.W * fullDest.W / source.W,
		H: visibleSource.H * fullDest.H / source.H,
	}
	clip := canvas.clips[len(canvas.clips)-1]
	if intersect(dest, clip).Empty() {
		return
	}
	*canvas.commands = append(*canvas.commands, DrawCommand{
		Kind: CommandImage, Image: resource, Rect: dest, Source: visibleSource,
		Clip: clip, Tint: tint, Filter: filter,
	})
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

// DashedRect draws a rectangular marquee border with alternating dash and gap segments.
func (canvas *Canvas) DashedRect(rect Rect, dashLen, gapLen, width float32, tint Color) {
	if rect.Empty() || width <= 0 || tint.A <= 0 {
		return
	}
	if dashLen <= 0 {
		dashLen = 5
	}
	if gapLen <= 0 {
		gapLen = 4
	}
	step := dashLen + gapLen
	for x := rect.X; x < rect.X+rect.W; x += step {
		w := minFloat32(dashLen, rect.X+rect.W-x)
		canvas.Rect(Rect{X: x, Y: rect.Y, W: w, H: width}, tint)
		canvas.Rect(Rect{X: x, Y: rect.Y + rect.H - width, W: w, H: width}, tint)
	}
	for y := rect.Y; y < rect.Y+rect.H; y += step {
		h := minFloat32(dashLen, rect.Y+rect.H-y)
		canvas.Rect(Rect{X: rect.X, Y: y, W: width, H: h}, tint)
		canvas.Rect(Rect{X: rect.X + rect.W - width, Y: y, W: width, H: h}, tint)
	}
}

// Checkerboard fills rect with an alternating two-color grid pattern.
func (canvas *Canvas) Checkerboard(rect Rect, cellSize float32, c1, c2 Color) {
	if rect.Empty() {
		return
	}
	if cellSize < 2 {
		cellSize = 8
	}
	canvas.Rect(rect, c1)
	if c2.A <= 0 {
		return
	}
	canvas.PushClip(rect)
	defer canvas.PopClip()
	cols := int(math.Ceil(float64(rect.W / cellSize)))
	rows := int(math.Ceil(float64(rect.H / cellSize)))
	if cols*rows > 4096 {
		if chk := SharedCheckerboardResource(); chk != nil {
			canvas.Image(chk, rect, Rect{}, RGBA(255, 255, 255, 255), FilterNearest)
		}
		return
	}
	for ry := 0; ry < rows; ry++ {
		y := rect.Y + float32(ry)*cellSize
		h := minFloat32(cellSize, rect.Y+rect.H-y)
		for rx := ry & 1; rx < cols; rx += 2 {
			x := rect.X + float32(rx)*cellSize
			w := minFloat32(cellSize, rect.X+rect.W-x)
			canvas.Rect(Rect{X: x, Y: y, W: w, H: h}, c2)
		}
	}
}

func (canvas *Canvas) Line(from, to Vec2, width float32, tint Color) {
	if width <= 0 || tint.A <= 0 || len(canvas.clips) == 0 {
		return
	}
	clip := canvas.clips[len(canvas.clips)-1]
	if clip.Empty() {
		return
	}

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
