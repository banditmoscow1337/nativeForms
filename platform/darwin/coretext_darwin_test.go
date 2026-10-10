//go:build darwin && (amd64 || arm64)

package darwin

import "testing"

func TestCoreTextMaskOrientation(t *testing.T) {
	// Every pixel differs so reversing either axis alone cannot pass.
	mask := coreTextMask([]byte{
		1, 2, 3,
		4, 5, 6,
	}, 3, 2)
	want := []byte{
		6, 5, 4,
		3, 2, 1,
	}
	for y := 0; y < 2; y++ {
		for x := 0; x < 3; x++ {
			if got, expected := mask.AlphaAt(x, y).A, want[y*3+x]; got != expected {
				t.Fatalf("pixel (%d, %d) = %d, want %d", x, y, got, expected)
			}
		}
	}
}
