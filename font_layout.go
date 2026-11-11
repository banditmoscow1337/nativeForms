package nativeforms

import (
	"image"
	"math"
	"strings"
	"unicode/utf8"
)

// Layout gives callers independent slices; package painting uses layoutShared
// while building its synchronous command list.
func (ctx *FontContext) Layout(text string, request FontSpec) TextLayout {
	line := ctx.layoutSharedCarets(text, request)
	line.Carets = append([]float32(nil), line.Carets...)
	line.Glyphs = append([]PositionedGlyph(nil), line.Glyphs...)
	return line
}

func (ctx *FontContext) layoutShared(text string, request FontSpec) TextLayout {
	return ctx.layoutInternal(text, request, false)
}

func (ctx *FontContext) layoutSharedCarets(text string, request FontSpec) TextLayout {
	return ctx.layoutInternal(text, request, true)
}

func (ctx *FontContext) layoutInternal(text string, request FontSpec, needCarets bool) TextLayout {
	spec := normalFontSpec(request)
	if ctx == nil {
		var carets []float32
		if needCarets {
			carets = make([]float32, utf8.RuneCountInString(text)+1)
		}
		line := TextLayout{Carets: carets}
		line.Ascent, line.Descent, line.LineGap = spec.Size*0.85, spec.Size*0.25, spec.Size*0.1
		return line
	}
	ctx.mu.Lock()
	defer ctx.mu.Unlock()
	key := layoutKey{spec: spec, text: text}
	if entry := ctx.layouts[key]; entry != nil {
		ctx.layoutHits++
		ctx.touchLayout(entry)
		if needCarets && len(entry.line.Carets) == 0 {
			n := utf8.RuneCountInString(text)
			var carets []float32
			if cap(entry.line.Carets) >= n+1 {
				carets = entry.line.Carets[:n+1]
			} else {
				carets = make([]float32, n+1)
			}
			order := ctx.orderedFaces(spec)
			ctx.stepAdvanceLocked(text, spec, order, carets)
			entry.line.Carets = carets
			extra := cap(carets) * 4
			entry.bytes += extra
			ctx.layoutBytes += extra
			ctx.evictLayouts()
		}
		return entry.line
	}
	ctx.layoutMisses++
	var runeStack [128]rune
	r := stringToRunesInto(text, runeStack[:0])
	visibleEstimate := 0
	for _, ch := range r {
		if !isSpaceFast(ch) && !invisibleRune(ch) {
			visibleEstimate++
		}
	}
	// Evict before allocating so we can immediately reuse a recycled cachedLayout
	// and its backing []PositionedGlyph slice without touching the Go heap.
	estCost := len(text) + visibleEstimate*48 + 96
	if needCarets {
		estCost += (len(r) + 1) * 4
	}
	if ctx.layoutBytes+estCost > ctx.layoutLimit && len(ctx.freeLayouts) == 0 && ctx.layoutTail != nil {
		e := ctx.layoutTail
		ctx.unlinkLayout(e)
		ctx.layoutBytes -= e.bytes
		delete(ctx.layouts, e.key)
		ctx.recycleCachedLayout(e)
	}
	pooled := ctx.allocCachedLayout()
	var line TextLayout
	if needCarets {
		if cap(pooled.line.Carets) >= len(r)+1 {
			line.Carets = pooled.line.Carets[:len(r)+1]
		} else {
			line.Carets = make([]float32, len(r)+1)
		}
	}
	if visibleEstimate > 0 {
		if cap(pooled.line.Glyphs) >= visibleEstimate {
			line.Glyphs = pooled.line.Glyphs[:0]
		} else {
			line.Glyphs = make([]PositionedGlyph, 0, max(visibleEstimate, 32))
		}
	}
	order := ctx.orderedFaces(spec)
	line.Ascent, line.Descent, line.LineGap = ctx.lineMetricsLocked(spec, order)
	invScale := 1.0 / spec.Scale

	var priorFace = -1
	var priorID uint16
	var priorAdvance float32
	var singleSlice [1]rune
	var idsStack [8]uint16
	var markStack [8]int
	for at := 0; at < len(r); {
		end := nextGrapheme(r, at)
		cluster := ctx.maybeComposeCluster(r[at:end], order, &singleSlice)
		var index int
		var ids []uint16
		var markFaces []int
		var ok bool
		if len(cluster) == 1 {
			var singleID uint16
			index, singleID, ok = ctx.faceForSingle(cluster[0], order)
			if ok {
				idsStack[0] = singleID
				markStack[0] = index
				ids = idsStack[:1]
				markFaces = markStack[:1]
			}
		} else {
			index, ids, ok = ctx.faceFor(cluster, order)
			if len(cluster) <= len(markStack) {
				markFaces = markStack[:len(cluster)]
			} else {
				markFaces = make([]int, len(cluster))
			}
			if !ok {
				baseIdx, baseID, baseOK := ctx.faceForSingle(cluster[0], order)
				if baseOK {
					index, ok = baseIdx, true
					markFaces[0] = index
					if len(cluster) <= len(idsStack) {
						ids = idsStack[:len(cluster)]
					} else {
						ids = make([]uint16, len(cluster))
					}
					ids[0] = baseID
					for j := 1; j < len(cluster); j++ {
						markFaces[j], ids[j] = -1, 0
						if source, mark, found := ctx.faceForSingle(cluster[j], order); found {
							markFaces[j], ids[j] = source, mark
						}
					}
				}
			} else {
				for j := range markFaces {
					markFaces[j] = index
				}
			}
		}
		if ok && index == priorFace && len(ids) > 0 {
			kern := ctx.faces[index].face.atlasKerning(priorID, ids[0]) * spec.Size
			line.Advance += maxFloat32(-priorAdvance, kern)
		}
		if needCarets {
			for i := at; i < end; i++ {
				line.Carets[i] = line.Advance
			}
		}
		if !ok {
			character := r[at]
			if character > 126 || character < 32 {
				character = '?'
			}
			advance := fallbackClusterAdvance(r[at], spec.Size)
			if advance > 0 && !isSpaceFast(r[at]) {
				if mask, offset := ctx.fallbackMaskFor(character, spec); mask != nil {
					b := mask.Bounds()
					line.Glyphs = append(line.Glyphs, PositionedGlyph{
						Mask: mask, X: line.Advance + offset.X*invScale, Y: offset.Y * invScale,
						W: float32(b.Dx()) * invScale, H: float32(b.Dy()) * invScale,
						Scale: spec.Scale, Face: -1,
					})
				}
			}
			line.Advance += advance
			priorFace = -1
			at = end
			continue
		}
		face := ctx.faces[index].face
		baseAdvance := float32(0)
		for i, character := range cluster {
			if invisibleRune(character) {
				continue
			}
			id := ids[i]
			space := isSpaceFast(character)
			if i == 0 {
				baseAdvance = face.advance(id) * spec.Size
				if space && id == 0 {
					baseAdvance = spec.Size * 0.33
				}
				if character == '\t' {
					baseAdvance = spec.Size * 0.33 * 4
				}
			}
			if space {
				continue
			}
			source := markFaces[i]
			var mask *image.Alpha
			var offset Vec2
			if source >= 0 && id != 0 {
				mask, offset = ctx.maskFor(source, id, spec)
			}
			if i > 0 && mask == nil {
				mask, offset = accentMask(character, spec.Size*spec.Scale)
			}
			if mask == nil {
				continue
			}
			b := mask.Bounds()
			gw := float32(b.Dx()) * invScale
			gh := float32(b.Dy()) * invScale
			snappedAdv := float32(math.Round(float64(line.Advance*spec.Scale))) * invScale
			x := snappedAdv + offset.X*invScale
			y := offset.Y * invScale
			if i > 0 {
				x += (baseAdvance-gw)*0.5 - offset.X*invScale
				y = maxFloat32(-line.Ascent, minFloat32(y, -spec.Size*0.75-gh))
			}
			line.Glyphs = append(line.Glyphs, PositionedGlyph{
				Mask: mask, X: x, Y: y, W: gw, H: gh,
				Scale: spec.Scale, Face: source, ID: id,
			})
		}
		line.Advance += maxFloat32(0, baseAdvance)
		priorFace, priorID, priorAdvance = index, ids[0], baseAdvance
		at = end
	}
	if needCarets {
		line.Carets[len(r)] = line.Advance
	}
	ascentSnap := float32(math.Round(float64(line.Ascent*spec.Scale))) * invScale
	for i := range line.Glyphs {
		line.Glyphs[i].Y += ascentSnap
	}
	cost := len(text) + cap(line.Carets)*4 + cap(line.Glyphs)*48 + 96
	if cost <= ctx.layoutLimit {
		pooled.key = key
		pooled.line = line
		pooled.bytes = cost
		ctx.pushLayoutFront(pooled)
		ctx.layouts[key] = pooled
		ctx.layoutBytes += cost
		ctx.evictLayouts()
	} else {
		pooled.line = TextLayout{}
		ctx.recycleCachedLayout(pooled)
	}
	return line
}

func (ctx *FontContext) CaretX(text string, spec FontSpec, at int) float32 {
	line := ctx.layoutSharedCarets(text, spec)
	return line.Carets[min(max(0, at), len(line.Carets)-1)]
}

func (ctx *FontContext) Measure(text string, spec FontSpec) Vec2 {
	spec = normalFontSpec(spec)
	if ctx == nil {
		return Vec2{Y: spec.Size * 1.2}
	}
	text = normalizeTextBreaks(text)
	ctx.mu.Lock()
	defer ctx.mu.Unlock()
	order := ctx.orderedFaces(spec)
	ascent, descent, lineGap := ctx.lineMetricsLocked(spec, order)
	lineH := ascent + descent + lineGap
	if !strings.Contains(text, "\n") {
		if text == "" {
			return Vec2{X: 0, Y: lineH}
		}
		key := layoutKey{spec: spec, text: text}
		if entry := ctx.layouts[key]; entry != nil {
			ctx.layoutHits++
			ctx.touchLayout(entry)
			return Vec2{X: entry.line.Advance, Y: entry.line.Height()}
		}
		return Vec2{X: ctx.stepAdvanceLocked(text, spec, order, nil), Y: lineH}
	}
	maxWidth, height := float32(0), float32(0)
	for start := 0; ; {
		end := strings.IndexByte(text[start:], '\n')
		var s string
		if end < 0 {
			s = text[start:]
		} else {
			s = text[start : start+end]
		}
		var adv float32
		if s != "" {
			key := layoutKey{spec: spec, text: s}
			if entry := ctx.layouts[key]; entry != nil {
				ctx.layoutHits++
				ctx.touchLayout(entry)
				adv = entry.line.Advance
			} else {
				adv = ctx.stepAdvanceLocked(s, spec, order, nil)
			}
		}
		maxWidth = maxFloat32(maxWidth, adv)
		height += lineH
		if end < 0 {
			break
		}
		start += end + 1
	}
	return Vec2{X: maxWidth, Y: height}
}

func (ctx *FontContext) Ellipsize(text string, width float32, spec FontSpec) string {
	ellipsis := "…"
	if glyphs := ctx.layoutShared(ellipsis, spec).Glyphs; len(glyphs) == 0 || glyphs[0].Face < 0 {
		ellipsis = "..."
	}
	return ellipsizeTextWith(text, width, func(s string) float32 { return ctx.Advance(s, spec) }, ellipsis)
}
