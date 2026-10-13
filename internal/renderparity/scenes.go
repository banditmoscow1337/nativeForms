// Package renderparity provides a shared pixel-golden corpus for every
// renderer backend. Frames are constructed without OS fonts or a window.
package renderparity

import (
	"image"
	"image/color"

	ui "github.com/banditmoscow1337/nativeForms"
)

type Scene struct {
	Name     string
	Frame    ui.Frame
	Expected []color.RGBA // Row-major, premultiplied RGBA pixels.
}

func resource(colors ...color.NRGBA) *ui.ImageResource {
	source := image.NewNRGBA(image.Rect(0, 0, len(colors), 1))
	for x, pixel := range colors {
		source.SetNRGBA(x, 0, pixel)
	}
	r, err := ui.NewImageResource(source)
	if err != nil {
		panic(err) // Fixed valid fixture, not user-supplied image data.
	}
	return r
}

// Scenes covers all DrawCommand kinds, clipping, source cropping, image
// filtering, premultiplication, and command order. Future renderers should
// replay these exact frames and compare their readback with Expected.
func Scenes() []Scene {
	white := ui.RGBA(255, 255, 255, 255)
	full := ui.Rect{W: 1, H: 1}
	whitePixel := color.RGBA{R: 255, G: 255, B: 255, A: 255}
	mask := image.NewAlpha(image.Rect(0, 0, 1, 1))
	mask.SetAlpha(0, 0, color.Alpha{A: 128})
	background := resource(color.NRGBA{A: 255})
	foreground := resource(color.NRGBA{R: 255, A: 128}, color.NRGBA{B: 255, A: 255})
	popup := resource(color.NRGBA{G: 255, A: 128})
	bilinear := resource(color.NRGBA{R: 255}, color.NRGBA{B: 255, A: 255})
	cropped := resource(color.NRGBA{G: 255, A: 255}, color.NRGBA{R: 255, A: 128})
	return []Scene{
		{
			Name: "quad-fractional-clip", Frame: ui.Frame{Width: 2, Height: 1, Commands: []ui.DrawCommand{{
				Kind: ui.CommandQuad, Rect: ui.Rect{W: 2, H: 1}, UV: ui.Rect{X: 0.5 / 2048, Y: 0.5 / 2048},
				Clip: ui.Rect{X: 0.5, W: 1, H: 1}, Tint: ui.RGBA(200, 40, 20, 255),
			}}}, Expected: []color.RGBA{{R: 100, G: 20, B: 10, A: 128}, {R: 100, G: 20, B: 10, A: 128}},
		},
		{
			Name: "line", Frame: ui.Frame{Width: 1, Height: 1, Commands: []ui.DrawCommand{{
				Kind: ui.CommandLine, From: ui.Vec2{Y: 0.5}, To: ui.Vec2{X: 1, Y: 0.5},
				Width: 1, Clip: full, Tint: white,
			}}}, Expected: []color.RGBA{whitePixel},
		},
		{
			Name: "glyph-mask", Frame: ui.Frame{Width: 1, Height: 1, Commands: []ui.DrawCommand{{
				Kind: ui.CommandMask, Mask: mask, Rect: full, Clip: full, Tint: white,
			}}}, Expected: []color.RGBA{{R: 128, G: 128, B: 128, A: 128}},
		},
		{
			Name: "rounded-rect", Frame: ui.Frame{Width: 3, Height: 3, Commands: []ui.DrawCommand{{
				Kind: ui.CommandRoundedRect, Rect: ui.Rect{W: 3, H: 3}, Clip: ui.Rect{W: 3, H: 3}, Radius: 1, Tint: white,
			}}}, Expected: []color.RGBA{
				{R: 207, G: 207, B: 207, A: 207}, whitePixel, {R: 207, G: 207, B: 207, A: 207},
				whitePixel, whitePixel, whitePixel,
				{R: 207, G: 207, B: 207, A: 207}, whitePixel, {R: 207, G: 207, B: 207, A: 207},
			},
		},
		{
			Name: "image-bilinear-premul", Frame: ui.Frame{Width: 1, Height: 1, Commands: []ui.DrawCommand{{
				Kind: ui.CommandImage, Image: bilinear, Rect: full, Clip: full, Tint: white, Filter: ui.FilterBilinear,
			}}}, Expected: []color.RGBA{{B: 128, A: 128}},
		},
		{
			Name: "image-crop-tint", Frame: ui.Frame{Width: 1, Height: 1, Commands: []ui.DrawCommand{
				{Kind: ui.CommandImage, Image: resource(color.NRGBA{B: 255, A: 255}), Rect: full, Clip: full, Tint: white},
				{Kind: ui.CommandImage, Image: cropped, Rect: full, Source: ui.Rect{X: 1, W: 1, H: 1}, Clip: full, Tint: ui.RGBA(255, 255, 255, 128)},
			}}, Expected: []color.RGBA{{R: 64, B: 191, A: 255}},
		},
		{
			Name: "image-clip-and-layer-order", Frame: ui.Frame{Width: 3, Height: 1, Commands: []ui.DrawCommand{
				{Kind: ui.CommandImage, Image: background, Rect: ui.Rect{W: 3, H: 1}, Clip: ui.Rect{W: 3, H: 1}, Tint: white},
				{Kind: ui.CommandImage, Image: foreground, Rect: ui.Rect{W: 2, H: 1}, Clip: full, Tint: white},
				{Kind: ui.CommandImage, Image: popup, Rect: full, Clip: ui.Rect{W: 3, H: 1}, Tint: white},
			}}, Expected: []color.RGBA{{R: 64, G: 128, A: 255}, {A: 255}, {A: 255}},
		},
	}
}
