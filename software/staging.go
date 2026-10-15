package software

import "image"

// UpdateBGRA copies a premultiplied RGBA frame into an OS BGRA staging buffer.
// A persistent destination can skip unchanged pixels. The returned rectangle
// conservatively covers all changed 64x64 tiles. Set valid to false for a new
// or recycled buffer whose contents are unknown.
func UpdateBGRA(dst []byte, src *image.RGBA, valid bool) image.Rectangle {
	if src == nil || src.Bounds().Min != (image.Point{}) || len(dst) != src.Bounds().Dx()*src.Bounds().Dy()*4 {
		return image.Rectangle{}
	}
	w, h := src.Bounds().Dx(), src.Bounds().Dy()
	const tile = 64
	damage := image.Rectangle{}
	for ty := 0; ty < h; ty += tile {
		for tx := 0; tx < w; tx += tile {
			x1, y1 := min(w, tx+tile), min(h, ty+tile)
			changed := false
			for y := ty; y < y1; y++ {
				si := src.PixOffset(tx, y)
				di := (y*w + tx) * 4
				for x := tx; x < x1; x++ {
					if !valid || dst[di] != src.Pix[si+2] || dst[di+1] != src.Pix[si+1] ||
						dst[di+2] != src.Pix[si] || dst[di+3] != src.Pix[si+3] {
						changed = true
						dst[di], dst[di+1], dst[di+2], dst[di+3] =
							src.Pix[si+2], src.Pix[si+1], src.Pix[si], src.Pix[si+3]
					}
					si += 4
					di += 4
				}
			}
			if changed {
				r := image.Rect(tx, ty, x1, y1)
				if damage.Empty() {
					damage = r
				} else {
					damage = damage.Union(r)
				}
			}
		}
	}
	return damage
}
