package platform

import (
	"image"
	"math"

	ui "github.com/banditmoscow1337/nativeForms"
)

// LogicalSize covers all physical pixels while keeping layout in display units.
// The last logical unit may be clipped by the physical framebuffer.
func LogicalSize(pixels int, scale float32) int {
	if pixels <= 0 {
		return 0
	}
	if !(scale > 0 && scale <= 8) {
		scale = 1
	}
	return int(math.Ceil(float64(pixels) / float64(scale)))
}

func PixelSize(logical int, scale float32) int {
	if logical <= 0 {
		return 0
	}
	if !(scale > 0 && scale <= 8) {
		scale = 1
	}
	return int(math.Round(float64(logical) * float64(scale)))
}

// PixelDamage rounds out a logical damage rect and includes one pixel of
// antialiasing fringe on every edge. It is clipped to the target framebuffer.
func PixelDamage(rect ui.Rect, scale float32, width, height int) image.Rectangle {
	if rect.Empty() || width <= 0 || height <= 0 {
		return image.Rectangle{}
	}
	if !(scale > 0 && scale <= 8) {
		scale = 1
	}
	x0 := int(math.Floor(float64(rect.X*scale))) - 1
	y0 := int(math.Floor(float64(rect.Y*scale))) - 1
	x1 := int(math.Ceil(float64((rect.X+rect.W)*scale))) + 1
	y1 := int(math.Ceil(float64((rect.Y+rect.H)*scale))) + 1
	return image.Rect(x0, y0, x1, y1).Intersect(image.Rect(0, 0, width, height))
}
