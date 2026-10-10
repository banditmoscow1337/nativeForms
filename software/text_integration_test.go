package software_test

import (
	"image"
	"image/color"
	"testing"

	ui "github.com/banditmoscow1337/nativeForms"
	"github.com/banditmoscow1337/nativeForms/software"
)

type fixedGlyph struct{}

func (fixedGlyph) Shape(text string, size float32) (ui.ShapedText, bool) {
	if text != "X" {
		return ui.ShapedText{}, false
	}
	mask := image.NewAlpha(image.Rect(0, 0, 1, 1))
	mask.SetAlpha(0, 0, color.Alpha{A: 128})
	return ui.ShapedText{Advance: 1, Mask: mask, Carets: []float32{0, 1}}, true
}

func TestGoldenLabelShapedTextThroughSoftware(t *testing.T) {
	ui.SetTextShaper(fixedGlyph{})
	t.Cleanup(func() { ui.SetTextShaper(nil) })
	root := ui.NewPanel(ui.Overlay)
	label := ui.NewLabel("X")
	label.FontSize = 1
	label.Color = ui.RGBA(255, 255, 255, 255)
	label.SetPreferredSize(ui.Vec2{X: 1, Y: 1.2})
	root.Add(label)
	manager := ui.New()
	manager.SetRoot(root)
	manager.BeginFrame(1, 2, 0)
	dst := image.NewRGBA(image.Rect(0, 0, 1, 2))
	if err := software.New().Render(manager.Frame(), dst); err != nil {
		t.Fatal(err)
	}
	if got := dst.RGBAAt(0, 0); got != (color.RGBA{R: 128, G: 128, B: 128, A: 128}) {
		t.Fatalf("text mask pixel: %v", got)
	}
	if got := dst.RGBAAt(0, 1); got != (color.RGBA{}) {
		t.Fatalf("text paint escaped its bounds: %v", got)
	}
}
