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
	overflows       int
}

func NewGlyphAtlas(size int) *GlyphAtlas {
	return &GlyphAtlas{size: size, entries: make(map[*image.Alpha]image.Rectangle)}
}

func (a *GlyphAtlas) Size() int      { return a.size }
func (a *GlyphAtlas) Overflows() int { return a.overflows }

func (a *GlyphAtlas) UpdatesSince(index int) []GlyphUpload { return a.updates[index:] }
func (a *GlyphAtlas) UpdateCount() int                     { return len(a.updates) }

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
	return r, true
}
