package nativeforms

import (
	"image"
	"testing"
)

type testScaledShaper struct{}

func (testScaledShaper) Shape(string, float32) (ShapedText, bool) {
	return ShapedText{Advance: 5, Carets: []float32{0, 5}}, true
}
func (testScaledShaper) ShapeAtScale(_ string, _, scale float32) (ShapedText, bool) {
	n := int(scale)
	return ShapedText{Advance: 5, Mask: image.NewAlpha(image.Rect(0, 0, n, n)), Carets: []float32{0, 5}}, true
}

func TestCanvasScaledTextShaperKeepsLogicalBounds(t *testing.T) {
	SetTextShaper(testScaledShaper{})
	defer SetTextShaper(nil)
	commands := make([]DrawCommand, 0, 2)
	canvas := Canvas{commands: &commands, clips: []Rect{{W: 100, H: 40}}, displayScale: 2}
	canvas.Text("X", Rect{W: 100, H: 40}, 14, TextLeft, Color{A: 1})
	if len(commands) != 1 || commands[0].Mask.Bounds().Dx() != 2 || commands[0].Rect.W != 1 {
		t.Fatalf("scaled mask must occupy one logical unit: %+v", commands)
	}
}
