package software

import (
	"image"
	"image/color"
	"testing"

	ui "github.com/banditmoscow1337/nativeForms"
)

func imageResource(t *testing.T, colors ...color.NRGBA) *ui.ImageResource {
	t.Helper()
	source := image.NewNRGBA(image.Rect(0, 0, len(colors), 1))
	for x, pixel := range colors {
		source.SetNRGBA(x, 0, pixel)
	}
	resource, err := ui.NewImageResource(source)
	if err != nil {
		t.Fatal(err)
	}
	return resource
}

func assertPixels(t *testing.T, dst *image.RGBA, expected ...color.RGBA) {
	t.Helper()
	if len(expected) != dst.Bounds().Dx()*dst.Bounds().Dy() {
		t.Fatal("invalid golden pixel count")
	}
	for y := 0; y < dst.Bounds().Dy(); y++ {
		for x := 0; x < dst.Bounds().Dx(); x++ {
			got, want := dst.RGBAAt(x, y), expected[y*dst.Bounds().Dx()+x]
			if got != want {
				t.Fatalf("pixel (%d,%d): got %v, want %v", x, y, got, want)
			}
		}
	}
}

func TestGoldenImageAlphaClipAndPopupOrder(t *testing.T) {
	background := imageResource(t, color.NRGBA{A: 255})
	imagePixels := imageResource(t, color.NRGBA{R: 255, A: 128}, color.NRGBA{B: 255, A: 255})
	popup := imageResource(t, color.NRGBA{G: 255, A: 128})
	white := ui.RGBA(255, 255, 255, 255)
	frame := ui.Frame{Width: 3, Height: 1, Commands: []ui.DrawCommand{
		{Kind: ui.CommandImage, Image: background, Rect: ui.Rect{W: 3, H: 1}, Clip: ui.Rect{W: 3, H: 1}, Tint: white},
		{Kind: ui.CommandImage, Image: imagePixels, Rect: ui.Rect{W: 2, H: 1}, Clip: ui.Rect{W: 1, H: 1}, Tint: white},
		{Kind: ui.CommandImage, Image: popup, Rect: ui.Rect{W: 1, H: 1}, Clip: ui.Rect{W: 3, H: 1}, Tint: white},
	}}
	dst := image.NewRGBA(image.Rect(0, 0, 3, 1))
	if err := New().Render(frame, dst); err != nil {
		t.Fatal(err)
	}
	assertPixels(t, dst, color.RGBA{R: 64, G: 128, A: 255},
		color.RGBA{A: 255}, color.RGBA{A: 255})
}

func TestGoldenBilinearPremultipliedNoTransparentFringe(t *testing.T) {
	resource := imageResource(t, color.NRGBA{R: 255}, color.NRGBA{B: 255, A: 255})
	frame := ui.Frame{Width: 1, Height: 1, Commands: []ui.DrawCommand{{
		Kind: ui.CommandImage, Image: resource, Rect: ui.Rect{W: 1, H: 1},
		Clip: ui.Rect{W: 1, H: 1}, Tint: ui.RGBA(255, 255, 255, 255), Filter: ui.FilterBilinear,
	}}}
	dst := image.NewRGBA(image.Rect(0, 0, 1, 1))
	if err := New().Render(frame, dst); err != nil {
		t.Fatal(err)
	}
	assertPixels(t, dst, color.RGBA{B: 128, A: 128})
}

func TestGoldenSourceCropTintAndOpacity(t *testing.T) {
	blue := imageResource(t, color.NRGBA{B: 255, A: 255})
	content := imageResource(t, color.NRGBA{G: 255, A: 255}, color.NRGBA{R: 255, A: 128})
	frame := ui.Frame{Width: 1, Height: 1, Commands: []ui.DrawCommand{
		{Kind: ui.CommandImage, Image: blue, Rect: ui.Rect{W: 1, H: 1},
			Clip: ui.Rect{W: 1, H: 1}, Tint: ui.RGBA(255, 255, 255, 255)},
		{Kind: ui.CommandImage, Image: content, Rect: ui.Rect{W: 1, H: 1},
			Source: ui.Rect{X: 1, W: 1, H: 1}, Clip: ui.Rect{W: 1, H: 1},
			Tint: ui.RGBA(255, 255, 255, 128)},
	}}
	dst := image.NewRGBA(image.Rect(0, 0, 1, 1))
	if err := New().Render(frame, dst); err != nil {
		t.Fatal(err)
	}
	assertPixels(t, dst, color.RGBA{R: 64, B: 191, A: 255})
}

func TestGoldenFractionalPixelsAndScaledScratchReuse(t *testing.T) {
	resource := imageResource(t, color.NRGBA{R: 255, G: 255, B: 255, A: 255})
	frame := ui.Frame{Width: 2, Height: 1, Commands: []ui.DrawCommand{{
		Kind: ui.CommandImage, Image: resource, Rect: ui.Rect{X: 0.5, W: 1, H: 1},
		Clip: ui.Rect{W: 2, H: 1}, Tint: ui.RGBA(255, 255, 255, 255),
	}}}
	renderer := New()
	dst := image.NewRGBA(image.Rect(0, 0, 2, 1))
	if err := renderer.Render(frame, dst); err != nil {
		t.Fatal(err)
	}
	assertPixels(t, dst, color.RGBA{R: 128, G: 128, B: 128, A: 128},
		color.RGBA{R: 128, G: 128, B: 128, A: 128})
	scaled := image.NewRGBA(image.Rect(0, 0, 4, 2))
	if err := renderer.RenderScaled(frame, scaled); err != nil {
		t.Fatal(err)
	}
	first := &renderer.scaled[0]
	if err := renderer.RenderScaled(frame, scaled); err != nil {
		t.Fatal(err)
	}
	if first != &renderer.scaled[0] || frame.Commands[0].Rect.X != 0.5 || renderer.scaled[0].Image != nil {
		t.Fatal("scaled renderer reallocated its scratch, mutated input or pinned an old image")
	}
	white := color.RGBA{R: 255, G: 255, B: 255, A: 255}
	assertPixels(t, scaled, color.RGBA{}, white, white, color.RGBA{},
		color.RGBA{}, white, white, color.RGBA{})
}

func TestGoldenRoundedRectAndShapedGlyphMask(t *testing.T) {
	frame := ui.Frame{Width: 3, Height: 3, Commands: []ui.DrawCommand{{
		Kind: ui.CommandRoundedRect, Rect: ui.Rect{W: 3, H: 3},
		Clip: ui.Rect{W: 3, H: 3}, Radius: 1, Tint: ui.RGBA(255, 255, 255, 255),
	}}}
	dst := image.NewRGBA(image.Rect(0, 0, 3, 3))
	if err := New().Render(frame, dst); err != nil {
		t.Fatal(err)
	}
	corner := color.RGBA{R: 207, G: 207, B: 207, A: 207}
	full := color.RGBA{R: 255, G: 255, B: 255, A: 255}
	assertPixels(t, dst, corner, full, corner, full, full, full, corner, full, corner)
	mask := image.NewAlpha(image.Rect(0, 0, 1, 1))
	mask.SetAlpha(0, 0, color.Alpha{A: 128})
	frame = ui.Frame{Width: 1, Height: 1, Commands: []ui.DrawCommand{{
		Kind: ui.CommandMask, Mask: mask, Rect: ui.Rect{W: 1, H: 1},
		Clip: ui.Rect{W: 1, H: 1}, Tint: ui.RGBA(255, 255, 255, 255),
	}}}
	glyph := image.NewRGBA(image.Rect(0, 0, 1, 1))
	if err := New().Render(frame, glyph); err != nil {
		t.Fatal(err)
	}
	assertPixels(t, glyph, color.RGBA{R: 128, G: 128, B: 128, A: 128})
}

func TestGoldenOpaqueRectFastPathAndFractionalClip(t *testing.T) {
	whiteUV := ui.Rect{X: 0.5 / 2048, Y: 0.5 / 2048}
	frame := ui.Frame{Width: 2, Height: 1, Commands: []ui.DrawCommand{{
		Kind: ui.CommandQuad, Rect: ui.Rect{W: 2, H: 1}, UV: whiteUV,
		Clip: ui.Rect{W: 2, H: 1}, Tint: ui.RGBA(200, 40, 20, 255),
	}}}
	dst := image.NewRGBA(image.Rect(0, 0, 2, 1))
	renderer := New()
	if err := renderer.Render(frame, dst); err != nil {
		t.Fatal(err)
	}
	if renderer.atlas != nil {
		t.Fatal("solid rect eagerly allocated the font atlas")
	}
	full := color.RGBA{R: 200, G: 40, B: 20, A: 255}
	assertPixels(t, dst, full, full)
	frame.Commands[0].Clip = ui.Rect{X: 0.5, W: 1, H: 1}
	if err := renderer.Render(frame, dst); err != nil {
		t.Fatal(err)
	}
	assertPixels(t, dst, color.RGBA{R: 100, G: 20, B: 10, A: 128},
		color.RGBA{R: 100, G: 20, B: 10, A: 128})
	if renderer.atlas != nil {
		t.Fatal("clipped solid rect allocated the font atlas")
	}
	frame.Commands[0].UV.W = 1.0 / 2048
	if err := renderer.Render(frame, dst); err != nil {
		t.Fatal(err)
	}
	if renderer.atlas == nil {
		t.Fatal("textured quad did not load the font atlas")
	}
}

func TestEmptyAndInvalidImageCommands(t *testing.T) {
	resource := imageResource(t, color.NRGBA{R: 255, A: 255})
	renderer := New()
	dst := image.NewRGBA(image.Rect(0, 0, 1, 1))
	frame := ui.Frame{Width: 1, Height: 1, Commands: []ui.DrawCommand{{
		Kind: ui.CommandImage, Image: resource, Rect: ui.Rect{W: 0, H: 1},
		Clip: ui.Rect{W: 1, H: 1}, Tint: ui.RGBA(255, 255, 255, 255),
	}}}
	if err := renderer.Render(frame, dst); err != nil {
		t.Fatal(err)
	}
	assertPixels(t, dst, color.RGBA{})
	frame.Commands[0].Filter = ui.ImageFilter(77)
	if err := renderer.Render(frame, dst); err == nil {
		t.Fatal("unknown image filter was silently ignored")
	}
	if err := renderer.Render(ui.Frame{Width: 0, Height: 1}, dst); err == nil {
		t.Fatal("zero-size frame was accepted")
	}
}
