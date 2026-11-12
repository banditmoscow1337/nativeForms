package nativeforms

import (
	"image/color"
	"testing"
)

func TestBlankImageResourceInPlaceUpdateAndWidgets(t *testing.T) {
	res, err := NewBlankImageResource(4, 4)
	if err != nil || res.Width() != 4 || res.Height() != 4 {
		t.Fatalf("NewBlankImageResource failed: res=%v err=%v", res, err)
	}
	v1 := res.Version()
	nrgba := make([]byte, 4*4*4)
	nrgba[0], nrgba[1], nrgba[2], nrgba[3] = 200, 100, 50, 128
	if err := res.UpdateNRGBA(4, 4, nrgba); err != nil {
		t.Fatalf("UpdateNRGBA failed: %v", err)
	}
	if res.Version() <= v1 || res.RGBAAt(0, 0) != (color.RGBA{R: 100, G: 50, B: 25, A: 128}) {
		t.Fatalf("unexpected premultiplied pixel after UpdateNRGBA: v=%d px=%v", res.Version(), res.RGBAAt(0, 0))
	}

	mgr := New()
	mgr.SetInteractive(true)

	painted := false
	ic := NewInteractiveCanvas(func(c *Canvas, r Rect) {
		painted = true
		c.Checkerboard(r, 8, RGBA(27, 36, 48, 255), RGBA(37, 48, 61, 255))
		c.DashedRect(r.Inset(All(4)), 5, 4, 1.5, RGBA(180, 242, 135, 255))
	})
	ic.SetOverlayBadge("Z = 0")
	ic.SetSecondaryBadge("All Z")
	ic.SetBottomHints("Wheel — zoom", "100%")

	selectedSwatch := -1
	sg := NewSwatchGrid([]Color{RGBA(255, 0, 0, 255), RGBA(0, 255, 0, 255)}, RGBA(255, 0, 0, 255), func(idx int, _ Color) {
		selectedSwatch = idx
	})

	selectedLayer := -1
	ll := NewLayerList(func(idx int) { selectedLayer = idx })
	ll.SetItems([]LayerItem{
		{ID: "l1", Name: "Слой 1", Meta: "Z 01 · 100%", Visible: true, Opacity: 1, Thumbnail: res},
		{ID: "l0", Name: "Слой 0", Meta: "Z 00 · 100%", Visible: true, Opacity: 1, Thumbnail: res},
	}, 0)

	selectedFrame := -1
	fs := NewFrameStrip(func(idx int) { selectedFrame = idx })
	fs.SetItems([]FrameStripItem{
		{ID: "f1", Header: "01 · Z 2", Title: "Покой", Thumbnail: res},
		{ID: "f2", Header: "02 · Z 2", Title: "Прыжок", Thumbnail: res},
	}, 0, 1)

	root := NewStack(Vertical, 6)
	root.Add(ic, sg, ll, fs)
	mgr.SetRoot(root)
	mgr.BeginFrame(500, 600, 0.016)

	if !painted {
		t.Fatal("InteractiveCanvas did not invoke OnPaint")
	}
	if !mgr.PerformAccessibilityAction(sg.UIState().AccessibilityID(), AccessibilityIncrement, "") || selectedSwatch != 1 {
		t.Fatalf("SwatchGrid AccessibilityIncrement failed: selectedSwatch=%d", selectedSwatch)
	}
	if !mgr.PerformAccessibilityAction(ll.UIState().AccessibilityID(), AccessibilityIncrement, "") || selectedLayer != 1 {
		t.Fatalf("LayerList AccessibilityIncrement failed: selectedLayer=%d", selectedLayer)
	}
	if !mgr.PerformAccessibilityAction(fs.UIState().AccessibilityID(), AccessibilityIncrement, "") || selectedFrame != 1 {
		t.Fatalf("FrameStrip AccessibilityIncrement failed: selectedFrame=%d", selectedFrame)
	}
}
