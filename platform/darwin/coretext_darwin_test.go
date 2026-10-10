//go:build darwin && (amd64 || arm64)

package darwin

import "testing"

func TestCoreTextMaskOrientation(t *testing.T) {
	mask := coreTextMask([]byte{
		1, 2, 3,
		4, 5, 6,
	}, 3, 2)
	want := []byte{
		4, 5, 6,
		1, 2, 3,
	}
	for y := 0; y < 2; y++ {
		for x := 0; x < 3; x++ {
			if got, expected := mask.AlphaAt(x, y).A, want[y*3+x]; got != expected {
				t.Fatalf("pixel (%d, %d) = %d, want %d", x, y, got, expected)
			}
		}
	}
}