package render

import "image"

// GlyphUpload is a newly placed immutable mask. Each in-flight GPU texture
// applies missing uploads only after its previous command buffer completes.
type GlyphUpload struct {
	Rect image.Rectangle
	Mask *image.Alpha
}

// GlyphAtlas packs cached glyph masks; keeping references prevents pointer
// reuse. A full atlas falls back to geometry without invalidating GPU frames.
type GlyphAtlas struct {
	size, x, y, row int
	entries         map[*image.Alpha]image.Rectangle
	updates         []GlyphUpload
	pixels          []byte
	dirty           image.Rectangle
	overflows       int
}

func NewGlyphAtlas(size int) *GlyphAtlas {
	return &GlyphAtlas{size: size, entries: make(map[*image.Alpha]image.Rectangle, 256)}
}

func (a *GlyphAtlas) Size() int      { return a.size }
func (a *GlyphAtlas) Overflows() int { return a.overflows }

func (a *GlyphAtlas) UpdatesSince(index int) []GlyphUpload { return a.updates[index:] }
func (a *GlyphAtlas) UpdateCount() int                     { return len(a.updates) }

// TakeDirtySheet returns the coalesced dirty region and backing pixel sheet
// (with stride == a.Size()) accumulated since the previous TakeDirtySheet call.
func (a *GlyphAtlas) TakeDirtySheet() (image.Rectangle, []byte, int) {
	if a == nil || a.dirty.Empty() || len(a.pixels) == 0 {
		return image.Rectangle{}, nil, 0
	}
	r := a.dirty
	a.dirty = image.Rectangle{}
	return r, a.pixels, a.size
}

func (a *GlyphAtlas) initSolidCorner() {
	if len(a.pixels) > 0 && a.size >= 2 {
		a.pixels[0] = 255
		a.pixels[1] = 255
		a.pixels[a.size] = 255
		a.pixels[a.size+1] = 255
	} else if len(a.pixels) > 0 {
		a.pixels[0] = 255
	}
}

// Reset clears packed entries and pending uploads so a backend can reclaim
// atlas space after waiting for in-flight GPU frames.
func (a *GlyphAtlas) Reset() {
	if a == nil {
		return
	}
	a.x, a.y, a.row = 0, 0, 0
	a.overflows = 0
	a.dirty = image.Rectangle{}
	clear(a.entries)
	clear(a.updates)
	a.updates = a.updates[:0]
	if len(a.pixels) > 0 {
		clear(a.pixels)
		a.initSolidCorner()
	}
}

func (a *GlyphAtlas) Place(mask *image.Alpha) (image.Rectangle, bool) {
	if a == nil || mask == nil || mask.Bounds().Empty() {
		return image.Rectangle{}, false
	}
	if r, ok := a.entries[mask]; ok {
		return r, true
	}
	w, h := mask.Bounds().Dx(), mask.Bounds().Dy()
	if w+2 > a.size || h+2 > a.size {
		a.overflows++
		return image.Rectangle{}, false
	}
	if a.x+w+2 > a.size {
		a.x, a.y, a.row = 0, a.y+a.row, 0
	}
	if a.y+h+2 > a.size {
		a.overflows++
		return image.Rectangle{}, false
	}
	r := image.Rect(a.x+1, a.y+1, a.x+1+w, a.y+1+h)
	a.x += w + 2
	a.row = max(a.row, h+2)
	a.entries[mask] = r
	a.updates = append(a.updates, GlyphUpload{Rect: r, Mask: mask})

	if len(a.pixels) == 0 && a.size > 0 {
		a.pixels = make([]byte, a.size*a.size)
		a.initSolidCorner()
	}
	if len(a.pixels) == a.size*a.size {
		srcMinX, srcMinY := mask.Rect.Min.X, mask.Rect.Min.Y
		for row := 0; row < h; row++ {
			so := mask.PixOffset(srcMinX, srcMinY+row)
			do := (r.Min.Y+row)*a.size + r.Min.X
			copy(a.pixels[do:do+w], mask.Pix[so:so+w])
		}
		if a.dirty.Empty() {
			a.dirty = r
		} else {
			a.dirty = a.dirty.Union(r)
		}
	}
	return r, true
}
