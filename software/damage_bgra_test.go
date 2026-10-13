package software

import (
	"image"
	"testing"

	ui "github.com/banditmoscow1337/nativeForms"
	"github.com/banditmoscow1337/nativeForms/internal/renderparity"
)

func TestBGRAParityForEveryGoldenScene(t *testing.T) {
	for _, scene := range renderparity.Scenes() {
		t.Run(scene.Name, func(t *testing.T) {
			width, height := scene.Frame.Width, scene.Frame.Height
			pixels := make([]byte, width*height*4)
			if err := New().RenderBGRAAtScale(scene.Frame, pixels, width, height, width*4, 1); err != nil {
				t.Fatal(err)
			}
			for i, want := range scene.Expected {
				j := i * 4
				if pixels[j] != want.B || pixels[j+1] != want.G || pixels[j+2] != want.R || pixels[j+3] != want.A {
					t.Fatalf("pixel %d BGRA=%v, want RGBA=%v", i, pixels[j:j+4], want)
				}
			}
		})
	}
}

func TestPartialSoftwareDamagePreservesOtherPixels(t *testing.T) {
	whiteUV := ui.Rect{X: 0.5 / 2048, Y: 0.5 / 2048}
	full := ui.Rect{W: 4, H: 3}
	frame := ui.Frame{Width: 4, Height: 3, Commands: []ui.DrawCommand{{
		Kind: ui.CommandQuad, Rect: full, Clip: full, UV: whiteUV, Tint: ui.RGBA(255, 30, 50, 255),
	}}}
	renderer := New()
	partial := image.NewRGBA(image.Rect(0, 0, 4, 3))
	if err := renderer.Render(frame, partial); err != nil {
		t.Fatal(err)
	}
	frame.Commands = append(frame.Commands, ui.DrawCommand{Kind: ui.CommandQuad,
		Rect: ui.Rect{X: 1, Y: 1, W: 2, H: 1}, Clip: full, UV: whiteUV, Tint: ui.RGBA(0, 200, 240, 255)})
	damage := image.Rect(1, 1, 3, 2)
	if err := renderer.RenderDamage(frame, partial, damage); err != nil {
		t.Fatal(err)
	}
	expected := image.NewRGBA(image.Rect(0, 0, 4, 3))
	if err := New().Render(frame, expected); err != nil {
		t.Fatal(err)
	}
	if string(partial.Pix) != string(expected.Pix) {
		t.Fatalf("partial output %v differs from full output %v", partial.Pix, expected.Pix)
	}
	bgra := make([]byte, 4*3*4)
	if err := renderer.RenderBGRAAtScale(frame, bgra, 4, 3, 16, 1); err != nil {
		t.Fatal(err)
	}
	if err := renderer.RenderBGRAAtScaleDamage(frame, bgra, 4, 3, 16, 1, damage); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < len(expected.Pix); i += 4 {
		if bgra[i] != expected.Pix[i+2] || bgra[i+1] != expected.Pix[i+1] ||
			bgra[i+2] != expected.Pix[i] || bgra[i+3] != expected.Pix[i+3] {
			t.Fatalf("BGRA damage differs at pixel %d", i/4)
		}
	}
}

func TestScaledDamageMatchesFullFrame(t *testing.T) {
	frame := ui.Frame{Width: 4, Height: 4, Commands: []ui.DrawCommand{{
		Kind: ui.CommandQuad, Rect: ui.Rect{W: 4, H: 4}, Clip: ui.Rect{W: 4, H: 4},
		UV: ui.Rect{X: 0.5 / 2048, Y: 0.5 / 2048}, Tint: ui.RGBA(120, 20, 200, 255),
	}}}
	partial := image.NewRGBA(image.Rect(0, 0, 8, 8))
	renderer := New()
	if err := renderer.RenderAtScale(frame, partial, 2); err != nil {
		t.Fatal(err)
	}
	frame.Commands = append(frame.Commands, ui.DrawCommand{
		Kind: ui.CommandRoundedRect, Rect: ui.Rect{X: 1, Y: 1, W: 2, H: 2},
		Clip: ui.Rect{W: 4, H: 4}, Radius: 0.5, Tint: ui.RGBA(240, 160, 20, 200),
	})
	damage := image.Rect(1, 1, 7, 7)
	if err := renderer.RenderAtScaleDamage(frame, partial, 2, damage); err != nil {
		t.Fatal(err)
	}
	expected := image.NewRGBA(image.Rect(0, 0, 8, 8))
	if err := New().RenderAtScale(frame, expected, 2); err != nil {
		t.Fatal(err)
	}
	if string(partial.Pix) != string(expected.Pix) {
		t.Fatal("scaled partial damage differs from a full render")
	}
}
