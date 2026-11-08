package nativeforms

import (
	"container/list"
	"fmt"
	"image"
	"math"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"
)

type FontSpec struct {
	Family   string
	Weight   int
	Italic   bool
	Size     float32
	Scale    float32
	Language string
}

type PositionedGlyph struct {
	Mask  *image.Alpha
	X, Y  float32
	Scale float32
	Face  int
	ID    uint16
}

type TextLayout struct {
	Advance, Ascent, Descent, LineGap float32
	Carets                            []float32
	Glyphs                            []PositionedGlyph
}

func (line TextLayout) Height() float32 { return line.Ascent + line.Descent + line.LineGap }

type registeredFace struct {
	family string
	weight int
	italic bool
	face   *TrueTypeFace
}

type glyphKey struct {
	face     int
	id       uint16
	physical int
	italic   bool
	bold     int
}

type cachedGlyph struct {
	key    glyphKey
	mask   *image.Alpha
	offset Vec2
	bytes  int
}

type layoutKey struct {
	text string
	spec FontSpec
}

type faceOrderKey struct {
	family string
	weight int
	italic bool
}

type cachedLayout struct {
	key   layoutKey
	line  TextLayout
	bytes int
}

const defaultGlyphCacheBytes = 8 << 20
const defaultLayoutCacheBytes = 8 << 20

type FontContext struct {
	mu           sync.Mutex
	faces        []registeredFace
	cache        map[glyphKey]*list.Element
	lru          list.List
	bytes, limit int
	layouts      map[layoutKey]*list.Element
	layoutLRU    list.List
	layoutBytes  int
	layoutLimit  int
	layoutHits   uint64
	layoutMisses uint64
	generation   uint64
	faceOrders   map[faceOrderKey][]int
}

func NewFontContext() *FontContext {
	return &FontContext{cache: make(map[glyphKey]*list.Element), limit: defaultGlyphCacheBytes,
		layouts: make(map[layoutKey]*list.Element), layoutLimit: defaultLayoutCacheBytes}
}

func NewSystemFontContext() *FontContext {
	ctx := NewFontContext()
	ctx.AddSystemFallbacks()
	return ctx
}

func (ctx *FontContext) AddSystemFallbacks() {
	if ctx == nil {
		return
	}
	ctx.mu.Lock()
	for _, face := range defaultSystemFaces() {
		ctx.faces = append(ctx.faces, registeredFace{weight: 400, face: face})
	}
	ctx.clearLayouts()
	clear(ctx.faceOrders)
	ctx.generation++
	ctx.mu.Unlock()
}

func (ctx *FontContext) AddFont(family string, weight int, italic bool, data []byte) error {
	if ctx == nil {
		return fmt.Errorf("nil font context")
	}
	face, err := ParseTrueType(append([]byte(nil), data...))
	if err != nil {
		return err
	}
	if weight <= 0 {
		weight = 400
	}
	ctx.mu.Lock()
	ctx.faces = append(ctx.faces, registeredFace{family: strings.ToLower(strings.TrimSpace(family)), weight: weight, italic: italic, face: face})
	ctx.clearLayouts()
	clear(ctx.faceOrders)
	ctx.generation++
	ctx.mu.Unlock()
	return nil
}

func (ctx *FontContext) SetCacheLimit(bytes int) {
	if ctx == nil {
		return
	}
	ctx.mu.Lock()
	ctx.limit = max(0, bytes)
	ctx.evict()
	// A cached run can otherwise keep an evicted mask alive indefinitely.
	ctx.clearLayouts()
	ctx.mu.Unlock()
}

func (ctx *FontContext) CacheUsage() (bytes, entries int) {
	if ctx == nil {
		return 0, 0
	}
	ctx.mu.Lock()
	defer ctx.mu.Unlock()
	return ctx.bytes, len(ctx.cache)
}

// SetLayoutCacheLimit bounds retained shaped runs independently of glyph masks.
func (ctx *FontContext) SetLayoutCacheLimit(bytes int) {
	if ctx == nil {
		return
	}
	ctx.mu.Lock()
	ctx.layoutLimit = max(0, bytes)
	ctx.evictLayouts()
	ctx.mu.Unlock()
}

func (ctx *FontContext) LayoutCacheStats() (bytes, entries int, hits, misses uint64) {
	if ctx == nil {
		return
	}
	ctx.mu.Lock()
	defer ctx.mu.Unlock()
	return ctx.layoutBytes, len(ctx.layouts), ctx.layoutHits, ctx.layoutMisses
}

func (ctx *FontContext) layoutGeneration() uint64 {
	if ctx == nil {
		return 0
	}
	ctx.mu.Lock()
	defer ctx.mu.Unlock()
	return ctx.generation
}

func (ctx *FontContext) clearLayouts() {
	clear(ctx.layouts)
	ctx.layoutLRU.Init()
	ctx.layoutBytes = 0
}

func (ctx *FontContext) evictLayouts() {
	for ctx.layoutBytes > ctx.layoutLimit && ctx.layoutLRU.Len() > 0 {
		e := ctx.layoutLRU.Back()
		entry := e.Value.(cachedLayout)
		ctx.layoutBytes -= entry.bytes
		delete(ctx.layouts, entry.key)
		ctx.layoutLRU.Remove(e)
	}
}

func (ctx *FontContext) evict() {
	for ctx.bytes > ctx.limit && ctx.lru.Len() > 0 {
		entry := ctx.lru.Back()
		g := entry.Value.(cachedGlyph)
		ctx.bytes -= g.bytes
		delete(ctx.cache, g.key)
		ctx.lru.Remove(entry)
	}
}

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
	spec.Family = strings.ToLower(strings.TrimSpace(spec.Family))
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

// Advance measures without rasterizing offscreen glyph masks or creating
// caret/glyph runs. It uses the same face selection and kerning as Layout.
func (ctx *FontContext) Advance(text string, request FontSpec) float32 {
	if ctx == nil {
		return 0
	}
	spec := normalFontSpec(request)
	ctx.mu.Lock()
	defer ctx.mu.Unlock()
	key := layoutKey{spec: spec, text: text}
	if entry := ctx.layouts[key]; entry != nil {
		ctx.layoutHits++
		ctx.layoutLRU.MoveToFront(entry)
		return entry.Value.(cachedLayout).line.Advance
	}
	order := ctx.orderedFaces(spec)
	r := []rune(text)
	advance := float32(0)
	priorFace := -1
	var priorID uint16
	var priorAdvance float32
	for at := 0; at < len(r); {
		end := nextGrapheme(r, at)
		cluster := r[at:end]
		if len(cluster) == 2 {
			if composed := composeRussian(cluster[0], cluster[1]); composed != 0 {
				if _, _, ok := ctx.faceFor([]rune{composed}, order); ok {
					cluster = []rune{composed}
				}
			}
		}
		index, id, ok := -1, uint16(0), false
		if len(cluster) == 1 {
			for _, candidate := range order {
				id = ctx.faces[candidate].face.glyphIndex(cluster[0])
				if id != 0 || invisibleRune(cluster[0]) || unicode.IsSpace(cluster[0]) {
					index, ok = candidate, true
					break
				}
			}
		} else {
			if selected, ids, found := ctx.faceFor(cluster, order); found {
				index, id, ok = selected, ids[0], true
			} else if selected, ids, found := ctx.faceFor(cluster[:1], order); found {
				index, id, ok = selected, ids[0], true
			}
		}
		if !ok {
			step := spec.Size * 0.6
			if invisibleRune(r[at]) || unicode.Is(unicode.Mn, r[at]) {
				step = 0
			}
			if unicode.IsSpace(r[at]) {
				step = spec.Size * 0.33
			}
			advance += step
			priorFace = -1
		} else {
			if index == priorFace {
				kern := ctx.faces[index].face.atlasKerning(priorID, id) * spec.Size
				advance += maxFloat32(-priorAdvance, kern)
			}
			step := ctx.faces[index].face.advance(id) * spec.Size
			if unicode.IsSpace(cluster[0]) && id == 0 {
				step = spec.Size * 0.33
			}
			if cluster[0] == '\t' {
				step = spec.Size * 0.33 * 4
			}
			if invisibleRune(cluster[0]) {
				step = 0
			}
			advance += maxFloat32(0, step)
			priorFace, priorID, priorAdvance = index, id, step
		}
		at = end
	}
	return advance
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

func invisibleRune(r rune) bool {
	switch r {
	case '\r', '\n', '\u0085', '\u2028', '\u2029', '\u00ad', '\u200b', '\u200c', '\u200d', '\u2060', '\ufeff':
		return true
	}
	return false
}

func (ctx *FontContext) maskFor(index int, id uint16, spec FontSpec) (*image.Alpha, Vec2) {
	face := ctx.faces[index]

	physical := max(1, int(math.Round(float64(spec.Size*spec.Scale*4))))
	bolder := 0
	if spec.Weight >= 600 && face.weight < 600 {
		bolder = max(1, int(float32(physical)/4/18))
	}
	key := glyphKey{face: index, id: id, physical: physical, italic: spec.Italic && !face.italic, bold: bolder}
	if entry := ctx.cache[key]; entry != nil {
		ctx.lru.MoveToFront(entry)
		g := entry.Value.(cachedGlyph)
		return g.mask, g.offset
	}
	out, err := face.face.glyph(id, 0)
	if err != nil {
		return nil, Vec2{}
	}
	mask, offset := rasterOutline(out, float32(physical)/4/float32(face.face.units), key.italic, key.bold)
	if mask != nil {
		cost := len(mask.Pix)
		if cost <= ctx.limit {
			entry := ctx.lru.PushFront(cachedGlyph{key: key, mask: mask, offset: offset, bytes: cost})
			ctx.cache[key] = entry
			ctx.bytes += cost
			ctx.evict()
		}
	}
	return mask, offset
}

func (ctx *FontContext) fallbackMaskFor(character rune, spec FontSpec) (*image.Alpha, Vec2) {
	physical := max(1, int(math.Round(float64(spec.Size*spec.Scale*4))))
	key := glyphKey{face: -1, id: uint16(character), physical: physical}
	if entry := ctx.cache[key]; entry != nil {
		ctx.lru.MoveToFront(entry)
		g := entry.Value.(cachedGlyph)
		return g.mask, g.offset
	}
	mask, offset := asciiMask(character, spec.Size*spec.Scale)
	if mask != nil && len(mask.Pix) <= ctx.limit {
		entry := ctx.lru.PushFront(cachedGlyph{key: key, mask: mask, offset: offset, bytes: len(mask.Pix)})
		ctx.cache[key] = entry
		ctx.bytes += len(mask.Pix)
		ctx.evict()
	}
	return mask, offset
}

// Layout gives callers independent slices; package painting uses layoutShared
// while building its synchronous command list.
func (ctx *FontContext) Layout(text string, request FontSpec) TextLayout {
	line := ctx.layoutShared(text, request)
	line.Carets = append([]float32(nil), line.Carets...)
	line.Glyphs = append([]PositionedGlyph(nil), line.Glyphs...)
	return line
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
		if id != 0 || unicode.IsSpace(r) {
			return index, id, true
		}
	}
	return -1, 0, false
}

func (ctx *FontContext) layoutShared(text string, request FontSpec) TextLayout {
	spec := normalFontSpec(request)
	if ctx == nil {
		line := TextLayout{Carets: make([]float32, utf8.RuneCountInString(text)+1)}
		line.Ascent, line.Descent, line.LineGap = spec.Size*0.85, spec.Size*0.25, spec.Size*0.1
		return line
	}
	ctx.mu.Lock()
	defer ctx.mu.Unlock()
	key := layoutKey{spec: spec, text: text}
	if entry := ctx.layouts[key]; entry != nil {
		ctx.layoutHits++
		ctx.layoutLRU.MoveToFront(entry)
		return entry.Value.(cachedLayout).line
	}
	ctx.layoutMisses++
	r := []rune(text)
	line := TextLayout{Carets: make([]float32, len(r)+1)}
	order := ctx.orderedFaces(spec)
	for _, index := range order {
		hhea := ctx.faces[index].face.hhea
		if len(hhea) == 0 {
			hhea = ctx.faces[index].face.tables["hhea"]
		}
		units := float32(ctx.faces[index].face.units)
		if ascent := float32(i16(hhea, 4)) / units * spec.Size; ascent > line.Ascent {
			line.Ascent = ascent
		}
		if descent := -float32(i16(hhea, 6)) / units * spec.Size; descent > line.Descent {
			line.Descent = descent
		}
		if gap := float32(i16(hhea, 8)) / units * spec.Size; gap > line.LineGap {
			line.LineGap = gap
		}
	}
	if line.Ascent <= 0 {
		line.Ascent = spec.Size * 0.85
	}
	if line.Descent <= 0 {
		line.Descent = spec.Size * 0.25
	}
	if line.LineGap < 0 {
		line.LineGap = 0
	}
	if len(order) == 0 {
		line.LineGap = spec.Size * 0.1
	}

	var priorFace = -1
	var priorID uint16
	var priorAdvance float32
	var singleSlice [1]rune
	var idsStack [8]uint16
	var markStack [8]int
	for at := 0; at < len(r); {
		end := nextGrapheme(r, at)
		cluster := r[at:end]
		if len(cluster) == 2 {
			if composed := composeRussian(cluster[0], cluster[1]); composed != 0 {
				if _, _, ok := ctx.faceForSingle(composed, order); ok {
					singleSlice[0] = composed
					cluster = singleSlice[:1]
				}
			}
		}
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
		for i := at; i < end; i++ {
			line.Carets[i] = line.Advance
		}
		if !ok {
			character := r[at]
			if character > 126 || character < 32 {
				character = '?'
			}
			advance := spec.Size * 0.6
			if invisibleRune(r[at]) || unicode.Is(unicode.Mn, r[at]) {
				advance = 0
			}
			if unicode.IsSpace(r[at]) {
				advance = spec.Size * 0.33
			}
			if advance > 0 && !unicode.IsSpace(r[at]) {
				if mask, offset := ctx.fallbackMaskFor(character, spec); mask != nil {
					line.Glyphs = append(line.Glyphs, PositionedGlyph{Mask: mask, X: line.Advance + offset.X/spec.Scale, Y: offset.Y / spec.Scale, Scale: spec.Scale, Face: -1})
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
			if i == 0 {
				baseAdvance = face.advance(id) * spec.Size
				if unicode.IsSpace(character) && id == 0 {
					baseAdvance = spec.Size * 0.33
				}
				if character == '\t' {
					baseAdvance = spec.Size * 0.33 * 4
				}
			}
			if unicode.IsSpace(character) {
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
			x := line.Advance + offset.X/spec.Scale
			y := offset.Y / spec.Scale
			if i > 0 {
				x += (baseAdvance-float32(mask.Bounds().Dx())/spec.Scale)*0.5 - offset.X/spec.Scale
				y = maxFloat32(-line.Ascent, minFloat32(y, -spec.Size*0.75-float32(mask.Bounds().Dy())/spec.Scale))
			}
			line.Glyphs = append(line.Glyphs, PositionedGlyph{Mask: mask, X: x, Y: y, Scale: spec.Scale, Face: source, ID: id})
		}
		line.Advance += maxFloat32(0, baseAdvance)
		priorFace, priorID, priorAdvance = index, ids[0], baseAdvance
		at = end
	}
	line.Carets[len(r)] = line.Advance
	for i := range line.Glyphs {
		line.Glyphs[i].Y += line.Ascent
	}
	cost := len(text) + len(line.Carets)*4 + len(line.Glyphs)*40 + 128
	if cost <= ctx.layoutLimit {
		e := ctx.layoutLRU.PushFront(cachedLayout{key: key, line: line, bytes: cost})
		ctx.layouts[key] = e
		ctx.layoutBytes += cost
		ctx.evictLayouts()
	}
	return line
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
	mask := image.NewAlpha(image.Rect(0, 0, scale*5, scale*7))
	for y, row := range rows {
		for x := 0; x < 5; x++ {
			if row&(1<<uint(4-x)) == 0 {
				continue
			}
			for dy := 0; dy < scale; dy++ {
				for dx := 0; dx < scale; dx++ {
					mask.Pix[(y*scale+dy)*mask.Stride+x*scale+dx] = 255
				}
			}
		}
	}
	return mask, Vec2{Y: -float32(scale * 7)}
}

func accentMask(mark rune, physical float32) (*image.Alpha, Vec2) {
	var pattern [7]byte
	switch mark {
	case '\u0301':
		pattern = [7]byte{0x02, 0x04, 0x08}
	case '\u0308':
		pattern = [7]byte{0x0a, 0x0a}
	case '\u0306':
		pattern = [7]byte{0x11, 0x0e}
	default:
		return nil, Vec2{}
	}
	scale := max(1, int(math.Round(float64(physical/12))))
	mask := image.NewAlpha(image.Rect(0, 0, scale*5, scale*3))
	for y, bits := range pattern {
		for x := 0; x < 5; x++ {
			if bits&(1<<uint(4-x)) == 0 {
				continue
			}
			for dy := 0; dy < scale; dy++ {
				for dx := 0; dx < scale; dx++ {
					mask.Pix[(y*scale+dy)*mask.Stride+x*scale+dx] = 255
				}
			}
		}
	}
	return mask, Vec2{}
}

func (ctx *FontContext) CaretX(text string, spec FontSpec, at int) float32 {
	line := ctx.layoutShared(text, spec)
	return line.Carets[min(max(0, at), len(line.Carets)-1)]
}

func (ctx *FontContext) Measure(text string, spec FontSpec) Vec2 {
	spec = normalFontSpec(spec)
	text = normalizeTextBreaks(text)
	maxWidth, height := float32(0), float32(0)
	for start := 0; ; {
		end := strings.IndexByte(text[start:], '\n')
		var s string
		if end < 0 {
			s = text[start:]
		} else {
			s = text[start : start+end]
		}
		line := ctx.layoutShared(s, spec)
		maxWidth = maxFloat32(maxWidth, line.Advance)
		height += line.Height()
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
	return ellipsizeTextWith(text, width, func(s string) float32 { return ctx.layoutShared(s, spec).Advance }, ellipsis)
}
