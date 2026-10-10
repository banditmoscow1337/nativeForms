package software

import (
	"image"
	"testing"

	ui "github.com/banditmoscow1337/nativeForms"
)

func TestRenderAtScaleClipsFractionalViewport(t *testing.T) {
	dst := image.NewRGBA(image.Rect(0, 0, 5, 5))
	mask := image.NewAlpha(image.Rect(0, 0, 1, 1))
	mask.Pix[0] = 255
	frame := ui.Frame{Width: 4, Height: 4, Commands: []ui.DrawCommand{{
		Kind: ui.CommandMask, Rect: ui.Rect{X: 3, Y: 0, W: 1, H: 1},
		Clip: ui.Rect{W: 4, H: 4}, Mask: mask, Tint: ui.Color{R: 1, A: 1},
	}}}
	if err := New().RenderAtScale(frame, dst, 1.5); err != nil {
		t.Fatal(err)
	}
	if a := dst.RGBAAt(4, 0).A; a == 0 || a == 255 {
		t.Fatalf("last pixel coverage = %d, want partial coverage", a)
	}
	if a := dst.RGBAAt(3, 0).A; a != 0 {
		t.Fatalf("pixel before scaled shape = %d, want 0", a)
	}
}
