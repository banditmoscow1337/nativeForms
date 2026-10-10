package paintdemo

import (
	"image"
	"image/color"

	ui "github.com/banditmoscow1337/nativeForms"
)

type badge struct{ ui.State }

func (b *badge) Paint(canvas *ui.Canvas) {
	canvas.RoundedRect(b.Bounds(), 8, ui.RGBA(8, 13, 24, 208))
}

func New() ui.Component {
	pixels := image.NewNRGBA(image.Rect(0, 0, 4, 4))
	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			if (x+y)%2 == 0 {
				pixels.SetNRGBA(x, y, color.NRGBA{R: 255, G: 160, B: 40, A: 192})
			} else {
				pixels.SetNRGBA(x, y, color.NRGBA{R: 40, G: 120, B: 230, A: 128})
			}
		}
	}
	resource, err := ui.NewImageResource(pixels)
	if err != nil {
		panic(err)
	}
	preview := ui.NewImage(resource)
	preview.Fit = ui.ImageCover
	preview.Filter = ui.FilterBilinear
	preview.SetAnchor(ui.AnchorFill)
	background := &badge{}
	background.SetPreferredSize(ui.Vec2{X: 140, Y: 32})
	background.SetAnchor(ui.AnchorBottomLeft)
	caption := ui.NewLabel("RGBA + clip")
	caption.SetPreferredSize(ui.Vec2{X: 140, Y: 32})
	caption.SetAnchor(ui.AnchorBottomLeft)
	panel := ui.NewPanel(ui.Overlay)
	panel.SetClip(true)
	panel.SetPreferredSize(ui.Vec2{X: 220, Y: 120})
	panel.Background = ui.RGBA(20, 24, 31, 255)
	panel.Add(preview, background, caption)
	return panel
}
