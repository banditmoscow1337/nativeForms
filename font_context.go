package nativeforms

import (
	"fmt"
	"image"
	"strings"
	"sync"
	"sync/atomic"
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
	W, H  float32
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
	prev, next *cachedGlyph
	key        glyphKey
	mask       *image.Alpha
	offset     Vec2
	bytes      int
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
	prev, next *cachedLayout
	key        layoutKey
	line       TextLayout
	bytes      int
}

const defaultGlyphCacheBytes = 512 << 10
const defaultLayoutCacheBytes = 256 << 10

var (
	systemFacesOnce sync.Once
	systemFacesList []registeredFace
)

func getSharedSystemFaces() []registeredFace {
	systemFacesOnce.Do(func() {
		loaded := make(map[faceOrderKey]bool, 6)
		for _, cand := range systemFontCandidates() {
			key := faceOrderKey{family: cand.family, weight: cand.weight, italic: cand.italic}
			if loaded[key] {
				continue
			}
			data, mapped, err := readSystemFontFile(cand.path)
			if err != nil {
				continue
			}
			face, err := parseTrueType(data, !mapped)
			if err != nil {
				continue
			}
			systemFacesList = append(systemFacesList, registeredFace{
				family: cand.family,
				weight: cand.weight,
				italic: cand.italic,
				face:   face,
			})
			loaded[key] = true
		}
	})
	return systemFacesList
}

type FontContext struct {
	mu                     sync.Mutex
	faces                  []registeredFace
	cache                  map[glyphKey]*cachedGlyph
	glyphHead, glyphTail   *cachedGlyph
	bytes, limit           int
	layouts                map[layoutKey]*cachedLayout
	layoutHead, layoutTail *cachedLayout
	freeLayouts            []*cachedLayout
	layoutSlab             []cachedLayout
	layoutBytes            int
	layoutLimit            int
	layoutHits             uint64
	layoutMisses           uint64
	generation             atomic.Uint64
	faceOrders             map[faceOrderKey][]int
}

func NewFontContext() *FontContext {
	ctx := &FontContext{
		cache:       make(map[glyphKey]*cachedGlyph, 256),
		limit:       defaultGlyphCacheBytes,
		layouts:     make(map[layoutKey]*cachedLayout, 256),
		layoutLimit: defaultLayoutCacheBytes,
		layoutSlab:  make([]cachedLayout, 256),
		freeLayouts: make([]*cachedLayout, 0, 256),
	}
	for i := len(ctx.layoutSlab) - 1; i >= 0; i-- {
		ctx.freeLayouts = append(ctx.freeLayouts, &ctx.layoutSlab[i])
	}
	return ctx
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
	shared := getSharedSystemFaces()
	ctx.mu.Lock()
	ctx.faces = append(ctx.faces, shared...)
	ctx.clearLayouts()
	clear(ctx.faceOrders)
	ctx.generation.Add(1)
	ctx.mu.Unlock()
}

func (ctx *FontContext) AddFont(family string, weight int, italic bool, data []byte) error {
	if ctx == nil {
		return fmt.Errorf("nil font context")
	}
	face, err := ParseTrueType(data)
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
	ctx.generation.Add(1)
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
		return 0, 0, 0, 0
	}
	ctx.mu.Lock()
	defer ctx.mu.Unlock()
	return ctx.layoutBytes, len(ctx.layouts), ctx.layoutHits, ctx.layoutMisses
}

func (ctx *FontContext) layoutGeneration() uint64 {
	if ctx == nil {
		return 0
	}
	return ctx.generation.Load()
}

func (ctx *FontContext) allocCachedLayout() *cachedLayout {
	if n := len(ctx.freeLayouts); n > 0 {
		e := ctx.freeLayouts[n-1]
		ctx.freeLayouts[n-1] = nil
		ctx.freeLayouts = ctx.freeLayouts[:n-1]
		return e
	}
	return &cachedLayout{}
}

func (ctx *FontContext) recycleCachedLayout(e *cachedLayout) {
	if e == nil {
		return
	}
	glyphs := e.line.Glyphs
	carets := e.line.Carets
	clear(glyphs)
	*e = cachedLayout{}
	if cap(glyphs) <= 128 {
		e.line.Glyphs = glyphs[:0]
	}
	if cap(carets) <= 128 {
		e.line.Carets = carets[:0]
	}
	if len(ctx.freeLayouts) < 512 {
		ctx.freeLayouts = append(ctx.freeLayouts, e)
	}
}

func (ctx *FontContext) clearLayouts() {
	for e := ctx.layoutHead; e != nil; {
		next := e.next
		ctx.recycleCachedLayout(e)
		e = next
	}
	clear(ctx.layouts)
	ctx.layoutHead, ctx.layoutTail = nil, nil
	ctx.layoutBytes = 0
}

func (ctx *FontContext) pushLayoutFront(e *cachedLayout) {
	e.prev = nil
	e.next = ctx.layoutHead
	if ctx.layoutHead != nil {
		ctx.layoutHead.prev = e
	} else {
		ctx.layoutTail = e
	}
	ctx.layoutHead = e
}

func (ctx *FontContext) unlinkLayout(e *cachedLayout) {
	if e.prev != nil {
		e.prev.next = e.next
	} else {
		ctx.layoutHead = e.next
	}
	if e.next != nil {
		e.next.prev = e.prev
	} else {
		ctx.layoutTail = e.prev
	}
	e.prev, e.next = nil, nil
}

func (ctx *FontContext) touchLayout(e *cachedLayout) {
	if ctx.layoutHead == e {
		return
	}
	ctx.unlinkLayout(e)
	ctx.pushLayoutFront(e)
}

func (ctx *FontContext) evictLayouts() {
	for ctx.layoutBytes > ctx.layoutLimit && ctx.layoutTail != nil {
		e := ctx.layoutTail
		ctx.unlinkLayout(e)
		ctx.layoutBytes -= e.bytes
		delete(ctx.layouts, e.key)
		ctx.recycleCachedLayout(e)
	}
}

func (ctx *FontContext) pushGlyphFront(g *cachedGlyph) {
	g.prev = nil
	g.next = ctx.glyphHead
	if ctx.glyphHead != nil {
		ctx.glyphHead.prev = g
	} else {
		ctx.glyphTail = g
	}
	ctx.glyphHead = g
}

func (ctx *FontContext) unlinkGlyph(g *cachedGlyph) {
	if g.prev != nil {
		g.prev.next = g.next
	} else {
		ctx.glyphHead = g.next
	}
	if g.next != nil {
		g.next.prev = g.prev
	} else {
		ctx.glyphTail = g.prev
	}
	g.prev, g.next = nil, nil
}

func (ctx *FontContext) touchGlyph(g *cachedGlyph) {
	if ctx.glyphHead == g {
		return
	}
	ctx.unlinkGlyph(g)
	ctx.pushGlyphFront(g)
}

func (ctx *FontContext) evict() {
	for ctx.bytes > ctx.limit && ctx.glyphTail != nil {
		g := ctx.glyphTail
		ctx.unlinkGlyph(g)
		ctx.bytes -= g.bytes
		delete(ctx.cache, g.key)
	}
}
