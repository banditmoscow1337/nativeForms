package software

import (
	"image"
	"testing"
)

func TestUpdateBGRAReusesUnchangedTiles(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 130, 130))
	dst := make([]byte, 130*130*4)
	if got := UpdateBGRA(dst, src, false); got != src.Bounds() {
		t.Fatalf("initial damage %v", got)
	}
	if got := UpdateBGRA(dst, src, true); !got.Empty() {
		t.Fatalf("unchanged damage %v", got)
	}
	i := src.PixOffset(70, 80)
	src.Pix[i], src.Pix[i+1], src.Pix[i+2], src.Pix[i+3] = 10, 20, 30, 255
	if got := UpdateBGRA(dst, src, true); got != image.Rect(64, 64, 128, 128) {
		t.Fatalf("tile damage %v", got)
	}
	j := (80*130 + 70) * 4
	if dst[j] != 30 || dst[j+1] != 20 || dst[j+2] != 10 || dst[j+3] != 255 {
		t.Fatalf("BGRA pixel: %v", dst[j:j+4])
	}
}
