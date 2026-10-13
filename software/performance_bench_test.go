package software

import (
	"image"
	"image/color"
	"testing"

	ui "github.com/banditmoscow1337/nativeForms"
)

func BenchmarkRenderCommands(b *testing.B) {
	for _, count := range []int{1000, 10000} {
		name := "1k"
		if count == 10000 {
			name = "10k"
		}
		b.Run(name, func(b *testing.B) {
			commands := make([]ui.DrawCommand, count)
			for i := range commands {
				commands[i] = ui.DrawCommand{Kind: ui.CommandQuad,
					Rect: ui.Rect{X: float32(i % 64 * 16), Y: float32(i / 64 % 48 * 16), W: 16, H: 16},
					Clip: ui.Rect{W: 1024, H: 768}, UV: ui.Rect{X: 0.5 / 2048, Y: 0.5 / 2048},
					Tint: ui.RGBA(40, 80, 160, 255)}
			}
			frame := ui.Frame{Width: 1024, Height: 768, Commands: commands}
			dst := image.NewRGBA(image.Rect(0, 0, 1024, 768))
			renderer := New()
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if err := renderer.Render(frame, dst); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkRenderTextMasks(b *testing.B) {
	mask := image.NewAlpha(image.Rect(0, 0, 8, 12))
	for i := range mask.Pix {
		mask.Pix[i] = 192
	}
	commands := make([]ui.DrawCommand, 10000)
	for i := range commands {
		commands[i] = ui.DrawCommand{Kind: ui.CommandMask, Mask: mask,
			Rect: ui.Rect{X: float32(i % 100 * 9), Y: float32(i / 100 % 50 * 14), W: 8, H: 12},
			Clip: ui.Rect{W: 1024, H: 768}, Tint: ui.RGBA(240, 240, 240, 255)}
	}
	frame := ui.Frame{Width: 1024, Height: 768, Commands: commands}
	dst := image.NewRGBA(image.Rect(0, 0, 1024, 768))
	renderer := New()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := renderer.Render(frame, dst); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkRenderImageHiDPI(b *testing.B) {
	source := image.NewNRGBA(image.Rect(0, 0, 512, 512))
	for y := 0; y < 512; y++ {
		for x := 0; x < 512; x++ {
			source.SetNRGBA(x, y, color.NRGBA{R: uint8(x), G: uint8(y), B: 180, A: 255})
		}
	}
	resource, err := ui.NewImageResource(source)
	if err != nil {
		b.Fatal(err)
	}
	frame := ui.Frame{Width: 800, Height: 600, Commands: []ui.DrawCommand{{
		Kind: ui.CommandImage, Image: resource, Rect: ui.Rect{W: 800, H: 600},
		Clip: ui.Rect{W: 800, H: 600}, Tint: ui.RGBA(255, 255, 255, 255), Filter: ui.FilterBilinear,
	}}}
	dst := image.NewRGBA(image.Rect(0, 0, 1600, 1200))
	renderer := New()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := renderer.RenderAtScale(frame, dst, 2); err != nil {
			b.Fatal(err)
		}
	}
}
