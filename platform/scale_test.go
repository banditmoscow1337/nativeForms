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

func TestValidateSize(t *testing.T) {
	for _, size := range [][3]float32{{0, 100, 1}, {-1, 100, 1}, {9000, 9000, 1}, {9000, 100, 2}} {
		if err := ValidateSize(int(size[0]), int(size[1]), size[2]); err == nil {
			t.Errorf("ValidateSize(%g, %g, %g) accepted invalid size", size[0], size[1], size[2])
		}
	}
	if err := ValidateSize(1100, 700, 1.5); err != nil {
		t.Fatal(err)
	}
}
