package nativeforms

import (
	"math"
	"strings"
	"unicode/utf8"
)

func (canvas *Canvas) Text(text string, bounds Rect, size float32, align TextAlign, tint Color) {
	if size <= 0 || bounds.Empty() || tint.A <= 0 {
		return
	}
	if canvas.fonts != nil {
		canvas.TextStyled(text, bounds, FontSpec{Size: size}, align, tint)
		return
	}
	text = normalizeTextBreaks(text)
	if canvas.trace != nil {
		canvas.trace.sample.TextRunes += utf8.RuneCountInString(text)
	}
	canvas.PushClip(bounds)

	firstNL := strings.IndexByte(text, '\n')
	lineHeight := size * 1.2
	if firstNL < 0 {
		y := bounds.Y + (bounds.H-lineHeight)*0.5
		if lineHeight > bounds.H {
			y = bounds.Y
		}
		x := bounds.X
		if align != TextLeft {
			textStart := canvas.trace.Start()
			lineWidth := measureTextLine(text, size)
			canvas.trace.Record(PhaseTextLayout, textStart)
			x = alignLineX(bounds, lineWidth, align)
		}
		if hasTextShaper() {
			pixelScale := canvas.displayScale
			if !(pixelScale > 0 && pixelScale <= 8) {
				pixelScale = 1
			}
			shapeStart := canvas.trace.Start()
			shaped, maskScale, ok := shapeTextRaster(text, size, pixelScale)
			canvas.trace.Record(PhaseTextLayout, shapeStart)
			if ok {
				if mask := shaped.Mask; mask != nil {
					if canvas.trace != nil {
						canvas.trace.sample.Glyphs++
					}
					gx := float32(math.Round(float64((x+shaped.Offset.X)*pixelScale))) / pixelScale
					gy := float32(math.Round(float64((y+shaped.Offset.Y)*pixelScale))) / pixelScale
					canvas.mask(mask, Rect{X: gx, Y: gy, W: float32(mask.Bounds().Dx()) / maskScale, H: float32(mask.Bounds().Dy()) / maskScale}, tint)
				}
			} else {
				canvas.emitAtlasLine(text, x, y, size, tint)
			}
		} else {
			canvas.emitAtlasLine(text, x, y, size, tint)
		}
		canvas.PopClip()
		return
	}

	lineCount := strings.Count(text[firstNL+1:], "\n") + 2
	totalH := lineHeight * float32(lineCount)
	y := bounds.Y + (bounds.H-totalH)*0.5
	if totalH > bounds.H {
		y = bounds.Y
	}
	for lineStart := 0; ; {
		lineEnd := len(text)
		lastLine := true
		if offset := strings.IndexByte(text[lineStart:], '\n'); offset >= 0 {
			lineEnd = lineStart + offset
			lastLine = false
		}
		line := text[lineStart:lineEnd]
		x := bounds.X
		if align != TextLeft {
			textStart := canvas.trace.Start()
			lineWidth := measureTextLine(line, size)
			canvas.trace.Record(PhaseTextLayout, textStart)
			x = alignLineX(bounds, lineWidth, align)
		}
		if hasTextShaper() {
			pixelScale := canvas.displayScale
			if !(pixelScale > 0 && pixelScale <= 8) {
				pixelScale = 1
			}
			shapeStart := canvas.trace.Start()
			shaped, maskScale, ok := shapeTextRaster(line, size, pixelScale)
			canvas.trace.Record(PhaseTextLayout, shapeStart)
			if ok {
				if mask := shaped.Mask; mask != nil {
					if canvas.trace != nil {
						canvas.trace.sample.Glyphs++
					}
					gx := float32(math.Round(float64((x+shaped.Offset.X)*pixelScale))) / pixelScale
					gy := float32(math.Round(float64((y+shaped.Offset.Y)*pixelScale))) / pixelScale
					canvas.mask(mask, Rect{X: gx, Y: gy, W: float32(mask.Bounds().Dx()) / maskScale, H: float32(mask.Bounds().Dy()) / maskScale}, tint)
				}
			} else {
				canvas.emitAtlasLine(line, x, y, size, tint)
			}
		} else {
			canvas.emitAtlasLine(line, x, y, size, tint)
		}
		if lastLine {
			break
		}
		lineStart = lineEnd + 1
		y += lineHeight
	}
	canvas.PopClip()
}

func (canvas *Canvas) emitAtlasGlyph(glyph glyphInfo, x, quadY, quadSize float32, tint Color) {
	if !glyph.visible {
		return
	}
	if canvas.trace != nil {
		canvas.trace.sample.Glyphs++
	}
	const cell = float32(fontCellSize) / fontAtlasSize
	cellX := float32(glyph.cellX*fontCellSize) / fontAtlasSize
	cellY := float32(glyph.cellY*fontCellSize) / fontAtlasSize
	canvas.quad(
		Rect{X: x, Y: quadY, W: quadSize, H: quadSize},
		Rect{X: cellX, Y: cellY, W: cell, H: cell}, tint,
	)
}

func (canvas *Canvas) emitAtlasLine(line string, x, y, size float32, tint Color) {
	if len(line) == 0 || len(canvas.clips) == 0 {
		return
	}
	clip := canvas.clips[len(canvas.clips)-1]
	if clip.Empty() {
		return
	}
	quadSize := size * float32(fontCellSize) / fontReferenceSize
	quadY := y - size*0.12
	if quadY+quadSize <= clip.Y || quadY >= clip.Y+clip.H {
		return
	}
	var previous glyphInfo
	offset := float32(0)
	const cell = float32(fontCellSize) / fontAtlasSize
	const scale = float32(fontCellSize) / fontAtlasSize
	cx0, cx1 := clip.X, clip.X+clip.W
	cmds := *canvas.commands
	glyphsEmitted := 0
	for i := 0; i < len(line); {
		var character rune
		var width int
		b := line[i]
		if b < 0x80 {
			character, width = rune(b), 1
		} else {
			character, width, _ = decodeComposedTextRune(line, i)
		}
		glyph, gx, nextX := stepAtlasGlyph(previous, character, size, offset)
		if glyph.visible {
			glyphsEmitted++
			px := x + gx
			if px+quadSize > cx0 && px < cx1 {
				cellX := float32(glyph.cellX) * scale
				cellY := float32(glyph.cellY) * scale
				cmds = append(cmds, DrawCommand{
					Kind: CommandQuad,
					Rect: Rect{X: px, Y: quadY, W: quadSize, H: quadSize},
					UV:   Rect{X: cellX, Y: cellY, W: cell, H: cell},
					Clip: clip, Tint: tint,
				})
			}
		}
		previous, offset = glyph, nextX
		i += width
	}
	*canvas.commands = cmds
	if canvas.trace != nil {
		canvas.trace.sample.Glyphs += glyphsEmitted
	}
}

func (canvas *Canvas) textSingleLineRunes(runes []rune, bounds Rect, size float32, tint Color) {
	if size <= 0 || bounds.Empty() || tint.A <= 0 || len(runes) == 0 {
		return
	}
	if canvas.trace != nil {
		canvas.trace.sample.TextRunes += len(runes)
	}
	canvas.PushClip(bounds)

	lineHeight := size * 1.2
	y := bounds.Y + (bounds.H-lineHeight)*0.5
	if lineHeight > bounds.H {
		y = bounds.Y
	}
	x := bounds.X
	var previous glyphInfo
	offset := float32(0)
	quadSize := size * float32(fontCellSize) / fontReferenceSize
	quadY := y - size*0.12
	for i := 0; i < len(runes); {
		character, count := decodeComposedSliceRune(runes, i)
		glyph, gx, nextX := stepAtlasGlyph(previous, character, size, offset)
		canvas.emitAtlasGlyph(glyph, x+gx, quadY, quadSize, tint)
		previous, offset = glyph, nextX
		i += count
	}
	canvas.PopClip()
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
