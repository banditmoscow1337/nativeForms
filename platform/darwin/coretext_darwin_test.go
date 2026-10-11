//go:build darwin && (amd64 || arm64)

package darwin

import (
	"strings"
	"testing"
)

func TestCoreTextMaskOrientation(t *testing.T) {
	mask := coreTextMask([]byte{
		1, 2, 3,
		4, 5, 6,
	}, 3, 2)
	want := []byte{
		1, 2, 3,
		4, 5, 6,
	}
	for y := 0; y < 2; y++ {
		for x := 0; x < 3; x++ {
			if got, expected := mask.AlphaAt(x, y).A, want[y*3+x]; got != expected {
				t.Fatalf("pixel (%d, %d) = %d, want %d", x, y, got, expected)
			}
		}
	}
}

// L has a horizontal stroke at its bottom. Check the real CoreText bitmap,
// not just a synthetic row copy, so a future Y-axis change cannot mirror it.
func TestCoreTextGlyphUpright(t *testing.T) {
	if err := EnableCoreText(); err != nil {
		t.Fatal(err)
	}
	for _, scale := range []float32{1, 2} {
		shaped, ok := systemShaper.ShapeAtScale("L", 48, scale)
		if !ok || shaped.Mask == nil {
			t.Fatalf("could not rasterize L at scale %g", scale)
		}
		mask := shaped.Mask
		width, height := mask.Bounds().Dx(), mask.Bounds().Dy()
		if width < 6 || height < 6 {
			t.Fatalf("unexpected glyph size at scale %g: %dx%d", scale, width, height)
		}
		var top, bottom int
		for y := 0; y < height/3; y++ {
			for x := width / 2; x < width; x++ {
				top += int(mask.AlphaAt(x, y).A)
				bottom += int(mask.AlphaAt(x, height-1-y).A)
			}
		}
		if bottom <= top {
			t.Fatalf("CoreText L is upside down at scale %g: upper-right=%d lower-right=%d", scale, top, bottom)
		}
	}
}

// Exercise the long native shaping path that can run during IME selection.
func TestCoreTextLongLineAutoreleasePool(t *testing.T) {
	if err := EnableCoreText(); err != nil {
		t.Fatal(err)
	}
	shaped, ok := systemShaper.ShapeAtScale(strings.Repeat("🙂", 9000)+"xy", 18, 1)
	if ok && len(shaped.Carets) != 9003 {
		t.Fatalf("long line has %d carets, want 9003", len(shaped.Carets))
	}
}
