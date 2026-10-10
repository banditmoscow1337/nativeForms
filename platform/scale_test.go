package platform

import "testing"

func TestLogicalAndPixelSizes(t *testing.T) {
	for _, tc := range []struct {
		pixels  int
		scale   float32
		logical int
	}{
		{1000, 1, 1000},
		{1000, 1.25, 800},
		{1001, 1.25, 801},
		{1501, 2, 751},
		{0, 2, 0},
	} {
		if got := LogicalSize(tc.pixels, tc.scale); got != tc.logical {
			t.Errorf("LogicalSize(%d, %g) = %d, want %d", tc.pixels, tc.scale, got, tc.logical)
		}
		if tc.pixels > 0 && PixelSize(tc.logical, tc.scale) < tc.pixels {
			t.Errorf("logical viewport %d does not cover %d pixels at %g", tc.logical, tc.pixels, tc.scale)
		}
	}
}
