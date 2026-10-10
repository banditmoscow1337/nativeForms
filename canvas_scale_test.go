package nativeforms

import "testing"

func TestCanvasRasterizesFontAtDisplayScale(t *testing.T) {
	fonts := NewFontContext()
	draw := func(scale float32) *DrawCommand {
		commands := make([]DrawCommand, 0, 8)
		canvas := Canvas{commands: &commands, clips: []Rect{{W: 100, H: 40}}, fonts: fonts, displayScale: scale}
		canvas.TextStyled("A", Rect{W: 100, H: 40}, FontSpec{Size: 14}, TextLeft, Color{R: 1, G: 1, B: 1, A: 1})
		for _, c := range commands {
			if c.Kind == CommandMask {
				return &c
			}
		}
		return nil
	}
	regular, high := draw(1), draw(2)
	if regular == nil || high == nil {
		t.Fatal("expected glyph mask commands")
	}
	if high.Mask.Bounds().Dx() <= regular.Mask.Bounds().Dx() {
		t.Fatalf("HiDPI glyph mask was not rasterized at higher resolution: %d vs %d", high.Mask.Bounds().Dx(), regular.Mask.Bounds().Dx())
	}
	if high.Rect.W != regular.Rect.W {
		t.Fatalf("logical text width changed: %g vs %g", high.Rect.W, regular.Rect.W)
	}
}
