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
	if w <= 0 || h <= 0 {
		return image.Rectangle{}
	}
	if !valid {
		rowBytes := w * 4
		for y := 0; y < h; y++ {
			si := src.PixOffset(0, y)
			di := y * rowBytes
			sRow := src.Pix[si : si+rowBytes]
			dRow := dst[di : di+rowBytes]
			for i := 0; i < rowBytes; i += 4 {
				dRow[i], dRow[i+1], dRow[i+2], dRow[i+3] = sRow[i+2], sRow[i+1], sRow[i], sRow[i+3]
			}
		}
		return image.Rect(0, 0, w, h)
	}
	const tile = 64
	damage := image.Rectangle{}
	for ty := 0; ty < h; ty += tile {
		y1 := min(h, ty+tile)
		for tx := 0; tx < w; tx += tile {
			x1 := min(w, tx+tile)
			spanBytes := (x1 - tx) * 4
			changed := false
			for y := ty; y < y1; y++ {
				si := src.PixOffset(tx, y)
				di := (y*w + tx) * 4
				sRow := src.Pix[si : si+spanBytes]
				dRow := dst[di : di+spanBytes]
				for i := 0; i < spanBytes; i += 4 {
					r, g, b, a := sRow[i], sRow[i+1], sRow[i+2], sRow[i+3]
					if dRow[i] != b || dRow[i+1] != g || dRow[i+2] != r || dRow[i+3] != a {
						changed = true
						dRow[i], dRow[i+1], dRow[i+2], dRow[i+3] = b, g, r, a
					}
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
