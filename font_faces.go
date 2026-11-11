package nativeforms

import (
	"image"
	"math"
	"strings"
	"unicode"
)

func normalFontSpec(spec FontSpec) FontSpec {
	if spec.Size <= 0 || math.IsNaN(float64(spec.Size)) {
		spec.Size = 14
	}
	if spec.Scale <= 0 || math.IsNaN(float64(spec.Scale)) {
		spec.Scale = 1
	}
	if math.IsInf(float64(spec.Size), 0) || spec.Size > 512 {
		spec.Size = 512
	}
	if math.IsInf(float64(spec.Scale), 0) || spec.Scale > 8 {
		spec.Scale = 8
	}
	if spec.Weight <= 0 {
		spec.Weight = 400
	}
	if spec.Family != "" {
		hasUpperOrSpace := false
		for i := 0; i < len(spec.Family); i++ {
			c := spec.Family[i]
			if (c >= 'A' && c <= 'Z') || c == ' ' || c == '\t' || c >= 0x80 {
				hasUpperOrSpace = true
				break
			}
		}
		if hasUpperOrSpace {
			spec.Family = strings.ToLower(strings.TrimSpace(spec.Family))
		}
	}
	return spec
}

func (ctx *FontContext) orderedFaces(spec FontSpec) []int {
	key := faceOrderKey{family: spec.Family, weight: spec.Weight, italic: spec.Italic}
	if order, ok := ctx.faceOrders[key]; ok {
		return order
	}
	order := make([]int, 0, len(ctx.faces))
	if spec.Family == "" && len(ctx.faces) > 0 {
		spec.Family = ctx.faces[0].family
	}
	for i, f := range ctx.faces {
		if f.family == spec.Family {
			order = append(order, i)
		}
	}
	for i := 1; i < len(order); i++ {
		j := i
		for j > 0 && faceDistance(ctx.faces[order[j]], spec) < faceDistance(ctx.faces[order[j-1]], spec) {
			order[j], order[j-1] = order[j-1], order[j]
			j--
		}
	}
	for i, f := range ctx.faces {
		if f.family != spec.Family {
			order = append(order, i)
		}
	}
	if ctx.faceOrders == nil {
		ctx.faceOrders = make(map[faceOrderKey][]int)
	}
	ctx.faceOrders[key] = order
	return order
}

func faceDistance(face registeredFace, spec FontSpec) int {
	distance := max(face.weight-spec.Weight, spec.Weight-face.weight)
	if face.italic != spec.Italic {
		distance += 1000
	}
	return distance
}

func (ctx *FontContext) faceFor(cluster []rune, order []int) (int, []uint16, bool) {
	for _, index := range order {
		ids := make([]uint16, len(cluster))
		valid := true
		for j, r := range cluster {
			if invisibleRune(r) {
				continue
			}
			ids[j] = ctx.faces[index].face.glyphIndex(r)
			if ids[j] == 0 && !unicode.IsSpace(r) {
				valid = false
				break
			}
		}
		if valid {
			return index, ids, true
		}
	}
	return -1, nil, false
}

func (ctx *FontContext) faceForSingle(r rune, order []int) (int, uint16, bool) {
	if invisibleRune(r) {
		if len(order) > 0 {
			return order[0], 0, true
		}
		return -1, 0, false
	}
	for _, index := range order {
		id := ctx.faces[index].face.glyphIndex(r)
		if id != 0 || (r == ' ' || r == '\t' || unicode.IsSpace(r)) {
			return index, id, true
		}
	}
	return -1, 0, false
}

func invisibleRune(r rune) bool {
	if r >= 0x20 && r < 0x7f {
		return false
	}
	switch r {
	case '\r', '\n', '\u0085', '\u2028', '\u2029', '\u00ad', '\u200b', '\u200c', '\u200d', '\u2060', '\ufeff':
		return true
	}
	return false
}

func (ctx *FontContext) putCachedMask(key glyphKey, mask *image.Alpha, offset Vec2) {
	if mask != nil && len(mask.Pix) <= ctx.limit {
		cost := len(mask.Pix)
		entry := &cachedGlyph{key: key, mask: mask, offset: offset, bytes: cost}
		ctx.pushGlyphFront(entry)
		ctx.cache[key] = entry
		ctx.bytes += cost
		ctx.evict()
	}
}

func (ctx *FontContext) maskFor(index int, id uint16, spec FontSpec) (*image.Alpha, Vec2) {
	face := ctx.faces[index]
	physical := max(1, int(math.Round(float64(spec.Size*spec.Scale*4))))
	bolder := 0
	if spec.Weight >= 600 && face.weight < 600 {
		bolder = max(1, int(float32(physical)/4/24))
	}
	key := glyphKey{face: index, id: id, physical: physical, italic: spec.Italic && !face.italic, bold: bolder}
	if entry := ctx.cache[key]; entry != nil {
		ctx.touchGlyph(entry)
		return entry.mask, entry.offset
	}
	out, err := face.face.glyph(id, 0)
	if err != nil {
		return nil, Vec2{}
	}
	mask, offset := rasterOutline(out, float32(physical)/4/float32(face.face.units), key.italic, key.bold)
	ctx.putCachedMask(key, mask, offset)
	return mask, offset
}

func (ctx *FontContext) fallbackMaskFor(character rune, spec FontSpec) (*image.Alpha, Vec2) {
	physical := max(1, int(math.Round(float64(spec.Size*spec.Scale*4))))
	key := glyphKey{face: -1, id: uint16(character), physical: physical}
	if entry := ctx.cache[key]; entry != nil {
		ctx.touchGlyph(entry)
		return entry.mask, entry.offset
	}
	mask, offset := asciiMask(character, spec.Size*spec.Scale)
	ctx.putCachedMask(key, mask, offset)
	return mask, offset
}

func scaleBitmapRows(rows []byte, scale int) *image.Alpha {
	mask := image.NewAlpha(image.Rect(0, 0, scale*5, scale*len(rows)))
	for y, row := range rows {
		if row == 0 {
			continue
		}
		for x := 0; x < 5; x++ {
			if row&(1<<uint(4-x)) == 0 {
				continue
			}
			for dy := 0; dy < scale; dy++ {
				base := (y*scale+dy)*mask.Stride + x*scale
				for dx := 0; dx < scale; dx++ {
					mask.Pix[base+dx] = 255
				}
			}
		}
	}
	return mask
}

func asciiMask(character rune, physical float32) (*image.Alpha, Vec2) {
	rows, ok := glyphRows[byte(character)]
	if !ok {
		rows = glyphRows['?']
	}
	scale := max(1, int(math.Round(float64(physical/8))))
	if scale > 256 {
		return nil, Vec2{}
	}
	return scaleBitmapRows(rows[:], scale), Vec2{Y: -float32(scale * 7)}
}

func accentMask(mark rune, physical float32) (*image.Alpha, Vec2) {
	var pattern [3]byte
	switch mark {
	case '\u0301':
		pattern = [3]byte{0x02, 0x04, 0x08}
	case '\u0308':
		pattern = [3]byte{0x0a, 0x0a}
	case '\u0306':
		pattern = [3]byte{0x11, 0x0e}
	default:
		return nil, Vec2{}
	}
	scale := max(1, int(math.Round(float64(physical/12))))
	return scaleBitmapRows(pattern[:], scale), Vec2{}
}
