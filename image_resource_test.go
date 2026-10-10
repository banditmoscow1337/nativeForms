package nativeforms

import (
	"image"
	"image/color"
	"testing"
)

func TestImageResourceSnapshotsAndVersion(t *testing.T) {
	source := image.NewNRGBA(image.Rect(5, 7, 7, 8))
	source.SetNRGBA(5, 7, color.NRGBA{R: 255, A: 128})
	first, err := NewImageResource(source)
	if err != nil {
		t.Fatal(err)
	}
	if first.ID() == 0 || first.Version() != 1 || first.Bounds() != image.Rect(0, 0, 2, 1) ||
		first.RGBAAt(0, 0) != (color.RGBA{R: 128, A: 128}) {
		t.Fatalf("unexpected image snapshot: id=%d version=%d bounds=%v pixel=%v",
			first.ID(), first.Version(), first.Bounds(), first.RGBAAt(0, 0))
	}
	source.SetNRGBA(5, 7, color.NRGBA{G: 255, A: 255})
	second, err := first.Replace(source)
	if err != nil {
		t.Fatal(err)
	}
	if second.ID() != first.ID() || second.Version() != 2 ||
		first.RGBAAt(0, 0) != (color.RGBA{R: 128, A: 128}) ||
		second.RGBAAt(0, 0) != (color.RGBA{G: 255, A: 255}) {
		t.Fatal("replacing an image changed a frame's old resource or lost its ID/version")
	}
	if _, err := NewImageResource(nil); err == nil {
		t.Fatal("nil image was accepted")
	}
	if _, err := NewImageResource(image.NewRGBA(image.Rectangle{})); err == nil {
		t.Fatal("empty image was accepted")
	}
	if _, err := first.Replace(nil); err == nil {
		t.Fatal("nil replacement was accepted")
	}
}

func TestImageWidgetFitsAndSourceClipping(t *testing.T) {
	resource, err := NewImageResource(image.NewRGBA(image.Rect(0, 0, 4, 2)))
	if err != nil {
		t.Fatal(err)
	}
	widget := NewImage(resource)
	if widget.Measure(Constraints{}) != (Vec2{X: 4, Y: 2}) {
		t.Fatal("image did not measure its natural dimensions")
	}
	widget.Arrange(Rect{W: 8, H: 8})
	commands := make([]DrawCommand, 0, 3)
	theme := DefaultTheme()
	canvas := Canvas{commands: &commands, clips: []Rect{{W: 8, H: 8}}, theme: &theme}
	widget.Paint(&canvas)
	if len(commands) != 1 || commands[0].Rect != (Rect{X: 0, Y: 2, W: 8, H: 4}) {
		t.Fatalf("contain mapping: %v", commands)
	}
	commands = commands[:0]
	widget.Fit = ImageCover
	widget.Paint(&canvas)
	if len(commands) != 1 || commands[0].Source != (Rect{X: 1, W: 2, H: 2}) ||
		commands[0].Rect != (Rect{W: 8, H: 8}) {
		t.Fatalf("cover mapping: %v", commands)
	}
	commands = commands[:0]
	widget.Fit = ImageStretch
	widget.Paint(&canvas)
	if len(commands) != 1 || commands[0].Source != (Rect{W: 4, H: 2}) ||
		commands[0].Rect != (Rect{W: 8, H: 8}) {
		t.Fatalf("stretch mapping: %v", commands)
	}
	commands = commands[:0]
	canvas.Image(resource, Rect{W: 4, H: 2}, Rect{X: -1, W: 4, H: 2}, RGBA(255, 255, 255, 255), FilterNearest)
	if len(commands) != 1 || commands[0].Source != (Rect{W: 3, H: 2}) ||
		commands[0].Rect != (Rect{X: 1, W: 3, H: 2}) {
		t.Fatalf("outside source was stretched instead of clipped: %v", commands)
	}
	commands = commands[:0]
	canvas.PushClip(Rect{X: 2, Y: 2, W: 2, H: 2})
	canvas.RoundedRect(Rect{W: 8, H: 8}, 20, RGBA(255, 255, 255, 255))
	if len(commands) != 1 || commands[0].Kind != CommandRoundedRect ||
		commands[0].Radius != 4 || commands[0].Clip != (Rect{X: 2, Y: 2, W: 2, H: 2}) {
		t.Fatalf("rounded corner radius or nested clip: %v", commands)
	}
	canvas.PopClip()
}

func TestFrameKeepsImageSnapshotAndPopupPaintOrder(t *testing.T) {
	source := image.NewRGBA(image.Rect(0, 0, 1, 1))
	source.SetRGBA(0, 0, color.RGBA{R: 255, A: 255})
	resource, err := NewImageResource(source)
	if err != nil {
		t.Fatal(err)
	}
	root := NewPanel(Overlay)
	root.Background = RGBA(0, 0, 0, 255)
	widget := NewImage(resource)
	widget.Fit = ImageStretch
	widget.SetAnchor(AnchorFill)
	root.Add(widget)
	manager := New()
	manager.SetRoot(root)
	manager.OpenPopup(NewImage(resource), Rect{X: 1, Y: 1, W: 1, H: 1}, false)
	manager.BeginFrame(2, 2, 0)
	commands := manager.Frame().Commands
	if len(commands) != 3 || commands[0].Kind != CommandQuad ||
		commands[1].Kind != CommandImage || commands[2].Kind != CommandImage ||
		commands[2].Rect != (Rect{X: 1, Y: 1, W: 1, H: 1}) {
		t.Fatalf("root/image/popup paint order: %v", commands)
	}
	old := commands[1].Image
	source.SetRGBA(0, 0, color.RGBA{B: 255, A: 255})
	newResource, err := resource.Replace(source)
	if err != nil {
		t.Fatal(err)
	}
	widget.SetImage(newResource)
	if old != resource || old.RGBAAt(0, 0) != (color.RGBA{R: 255, A: 255}) ||
		!manager.NeedsFrame() {
		t.Fatal("image replacement mutated an existing command or failed to invalidate layout")
	}
	manager.SetRoot(NewPanel())
	manager.BeginFrame(2, 2, 0)
	for _, command := range commands {
		if command.Image != nil {
			t.Fatal("reused frame storage still pins a discarded image resource")
		}
	}
}
