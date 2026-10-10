package platform

import "math"

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
