package software_test

import (
	"image"
	"testing"

	ui "github.com/banditmoscow1337/nativeForms"
	"github.com/banditmoscow1337/nativeForms/software"
)

func TestManagedFontMaskThroughSoftware(t *testing.T) {
	manager := ui.New()
	manager.SetFontContext(ui.NewFontContext())
	label := ui.NewLabel("A")
	label.Font = ui.FontSpec{Size: 16, Scale: 2}
	label.Color = ui.RGBA(255, 255, 255, 255)
	manager.SetRoot(label)
	manager.BeginFrame(32, 32, 0)
	masks := 0
	for _, command := range manager.Frame().Commands {
		if command.Kind == ui.CommandMask {
			masks++
		}
	}
	if masks != 1 {
		t.Fatalf("managed glyph masks: %d", masks)
	}
	dst := image.NewRGBA(image.Rect(0, 0, 32, 32))
	if err := software.New().Render(manager.Frame(), dst); err != nil {
		t.Fatal(err)
	}
	ink := 0
	for y := 0; y < 32; y++ {
		for x := 0; x < 32; x++ {
			if dst.RGBAAt(x, y).A != 0 {
				ink++
			}
		}
	}
	if ink == 0 {
		t.Fatal("software renderer dropped the managed glyph mask")
	}
}
