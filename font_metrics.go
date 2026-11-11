package nativeforms

import (
	"unicode"
	"unicode/utf8"
)

func (ctx *FontContext) maybeComposeCluster(cluster []rune, order []int, buf *[1]rune) []rune {
	if len(cluster) == 2 {
		if composed := composeRussian(cluster[0], cluster[1]); composed != 0 {
			if _, _, ok := ctx.faceForSingle(composed, order); ok {
				buf[0] = composed
				return buf[:1]
			}
		}
	}
	return cluster
}

func fallbackClusterAdvance(r rune, size float32) float32 {
	if invisibleRune(r) || unicode.Is(unicode.Mn, r) {
		return 0
	}
	if unicode.IsSpace(r) {
		return size * 0.33
	}
	return size * 0.6
}

func stringToRunesInto(text string, buf []rune) []rune {
	for _, r := range text {
		buf = append(buf, r)
	}
	return buf
}

func isSpaceFast(r rune) bool {
	if r < 0x80 {
		return r == ' ' || (r >= '\t' && r <= '\r')
	}
	return unicode.IsSpace(r)
}

func (ctx *FontContext) lineMetricsLocked(spec FontSpec, order []int) (ascent, descent, lineGap float32) {
	if len(order) > 0 {
		f := ctx.faces[order[0]].face
		hhea := f.table("hhea", f.hhea)
		units := float32(f.units)
		ascent = maxFloat32(ascent, float32(i16(hhea, 4))/units*spec.Size)
		descent = maxFloat32(descent, -float32(i16(hhea, 6))/units*spec.Size)
		lineGap = maxFloat32(lineGap, float32(i16(hhea, 8))/units*spec.Size)
	}
	if ascent <= 0 {
		ascent = spec.Size * 0.85
	}
	if descent <= 0 {
		descent = spec.Size * 0.25
	}
	if lineGap < 0 {
		lineGap = 0
	}
	if len(order) == 0 {
		lineGap = spec.Size * 0.1
	}
	return ascent, descent, lineGap
}

func (ctx *FontContext) stepAdvanceLocked(text string, spec FontSpec, order []int, carets []float32) float32 {
	var runeStack [128]rune
	r := stringToRunesInto(text, runeStack[:0])
	advance := float32(0)
	priorFace := -1
	var priorID uint16
	var priorAdvance float32
	var singleSlice [1]rune
	for at := 0; at < len(r); {
		end := nextGrapheme(r, at)
		cluster := ctx.maybeComposeCluster(r[at:end], order, &singleSlice)
		index, id, ok := -1, uint16(0), false
		if len(cluster) == 1 {
			index, id, ok = ctx.faceForSingle(cluster[0], order)
		} else if selected, ids, found := ctx.faceFor(cluster, order); found {
			index, id, ok = selected, ids[0], true
		} else {
			index, id, ok = ctx.faceForSingle(cluster[0], order)
		}
		if !ok {
			if carets != nil {
				for i := at; i < end && i < len(carets); i++ {
					carets[i] = advance
				}
			}
			advance += fallbackClusterAdvance(r[at], spec.Size)
			priorFace = -1
		} else {
			if index == priorFace {
				kern := ctx.faces[index].face.atlasKerning(priorID, id) * spec.Size
				advance += maxFloat32(-priorAdvance, kern)
			}
			if carets != nil {
				for i := at; i < end && i < len(carets); i++ {
					carets[i] = advance
				}
			}
			ch0 := cluster[0]
			step := ctx.faces[index].face.advance(id) * spec.Size
			if id == 0 && isSpaceFast(ch0) {
				step = spec.Size * 0.33
			}
			if ch0 == '\t' {
				step = spec.Size * 0.33 * 4
			}
			if invisibleRune(ch0) {
				step = 0
			}
			advance += maxFloat32(0, step)
			priorFace, priorID, priorAdvance = index, id, step
		}
		at = end
	}
	if carets != nil && len(r) < len(carets) {
		carets[len(r)] = advance
	}
	return advance
}

// Advance measures without rasterizing offscreen glyph masks or creating
// caret/glyph runs. It uses the same face selection and kerning as Layout.
func (ctx *FontContext) Advance(text string, request FontSpec) float32 {
	if ctx == nil || text == "" {
		return 0
	}
	spec := normalFontSpec(request)
	ctx.mu.Lock()
	defer ctx.mu.Unlock()
	key := layoutKey{spec: spec, text: text}
	if entry := ctx.layouts[key]; entry != nil {
		ctx.layoutHits++
		ctx.touchLayout(entry)
		return entry.line.Advance
	}
	order := ctx.orderedFaces(spec)
	return ctx.stepAdvanceLocked(text, spec, order, nil)
}

// CaretPositionsInto writes per-rune caret offsets into dst without rasterizing
// glyph masks or allocating a cached TextLayout when the line is not already cached.
func (ctx *FontContext) CaretPositionsInto(text string, request FontSpec, dst []float32) []float32 {
	spec := normalFontSpec(request)
	n := utf8.RuneCountInString(text)
	if ctx == nil {
		dst = ensureCaretSlice(dst, n+1)
		clear(dst)
		return dst
	}
	ctx.mu.Lock()
	defer ctx.mu.Unlock()
	key := layoutKey{spec: spec, text: text}
	if entry := ctx.layouts[key]; entry != nil && len(entry.line.Carets) == n+1 {
		ctx.layoutHits++
		ctx.touchLayout(entry)
		return entry.line.Carets
	}
	dst = ensureCaretSlice(dst, n+1)
	order := ctx.orderedFaces(spec)
	ctx.stepAdvanceLocked(text, spec, order, dst)
	return dst
}
