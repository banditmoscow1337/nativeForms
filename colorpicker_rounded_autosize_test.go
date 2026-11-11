package nativeforms

import (
	"image"
	"math"
	"testing"
)

func TestColorHexAndHSVConversion(t *testing.T) {
	c, ok := ParseHexColor("#E2AE3A")
	if !ok {
		t.Fatal("failed to parse #E2AE3A")
	}
	if got := c.Hex(); got != "#E2AE3A" {
		t.Fatalf("c.Hex() = %q, want #E2AE3A", got)
	}
	cAlpha, ok := ParseHexColor("3480E080")
	if !ok || cAlpha.HexAlpha() != "#3480E080" {
		t.Fatalf("cAlpha.HexAlpha() = %q, want #3480E080", cAlpha.HexAlpha())
	}
	shortC, ok := ParseHexColor("#F0A")
	if !ok || shortC.Hex() != "#FF00AA" {
		t.Fatalf("shortC.Hex() = %q, want #FF00AA", shortC.Hex())
	}
	h, s, v := ColorToHSV(c)
	rebuilt := HSV(h, s, v)
	r1, g1, b1, _ := c.RGBA8()
	r2, g2, b2, _ := rebuilt.RGBA8()
	if math.Abs(float64(int(r1)-int(r2))) > 1 || math.Abs(float64(int(g1)-int(g2))) > 1 || math.Abs(float64(int(b1)-int(b2))) > 1 {
		t.Fatalf("HSV roundtrip mismatch: got (%d,%d,%d), want (%d,%d,%d)", r2, g2, b2, r1, g1, b1)
	}
}

func TestColorPickerInteractionAndAccessibility(t *testing.T) {
	mgr := New()
	mgr.SetInteractive(true)
	var changed Color
	calls := 0
	cp := NewColorPicker("Accent", RGBA(200, 50, 50, 255), func(c Color) {
		changed = c
		calls++
	})
	cp.SetCornerRadius(8)
	mgr.SetRoot(cp)
	mgr.BeginFrame(300, 220, 0.016)

	if !cp.SetHex("#226CD6") || calls == 0 || changed.Hex() != "#226CD6" {
		t.Fatalf("SetHex failed: calls=%d hex=%s", calls, cp.Hex())
	}

	// Pointer drag in SV area
	_, sv, _, _, _ := cp.layoutRegions()
	mgr.HandleEvent(Event{Type: PointerDown, Button: MouseLeft, X: sv.X + sv.W*0.8, Y: sv.Y + sv.H*0.2})
	mgr.HandleEvent(Event{Type: PointerMove, X: sv.X + sv.W*0.5, Y: sv.Y + sv.H*0.5})
	mgr.HandleEvent(Event{Type: PointerUp, Button: MouseLeft, X: sv.X + sv.W*0.5, Y: sv.Y + sv.H*0.5})
	_, s, v := cp.HSV()
	if math.Abs(float64(s-0.5)) > 0.05 || math.Abs(float64(v-0.5)) > 0.05 {
		t.Fatalf("SV pointer drag did not update s/v: s=%v v=%v", s, v)
	}

	// Accessibility SetValue & Increment
	tree := mgr.AccessibilitySnapshot()
	node, ok := tree.Node(cp.UIState().AccessibilityID())
	if !ok || node.Role != RoleColorPicker {
		t.Fatalf("expected RoleColorPicker node, got %+v", node)
	}
	if !mgr.PerformAccessibilityAction(cp.UIState().AccessibilityID(), AccessibilitySetValue, "#48C476") || cp.Hex() != "#48C476" {
		t.Fatalf("AccessibilitySetValue failed: hex=%s", cp.Hex())
	}

	// Compact mode opens popup
	compact := NewCompactColorPicker("Compact", RGBA(100, 150, 200, 255), nil)
	mgr.SetRoot(compact)
	mgr.BeginFrame(300, 220, 0.016)
	b := compact.Bounds()
	mgr.HandleEvent(Event{Type: PointerDown, Button: MouseLeft, X: b.X + 10, Y: b.Y + 10})
	mgr.HandleEvent(Event{Type: PointerUp, Button: MouseLeft, X: b.X + 10, Y: b.Y + 10})
	if len(mgr.popups) != 1 {
		t.Fatalf("expected compact ColorPicker to open 1 popup, got %d", len(mgr.popups))
	}
}

func TestCornerRadiusAndRoundedBorder(t *testing.T) {
	mgr := New()
	panel := NewStack(Vertical, 8)
	panel.Background = DefaultTheme().Panel
	panel.Border = DefaultTheme().Border
	panel.BorderWidth = 2
	panel.SetCornerRadius(10)

	btn := NewButton("Rounded", nil)
	btn.SetCornerRadius(8)
	panel.Add(btn)
	mgr.SetRoot(panel)
	mgr.BeginFrame(280, 120, 0.016)

	roundedFills := 0
	roundedBorders := 0
	for _, cmd := range mgr.Frame().Commands {
		if cmd.Kind == CommandRoundedRect {
			if cmd.Width > 0 {
				roundedBorders++
			} else {
				roundedFills++
			}
		}
	}
	if roundedFills < 2 || roundedBorders < 2 {
		t.Fatalf("expected rounded fills and borders for panel and button, got fills=%d borders=%d", roundedFills, roundedBorders)
	}

	verts := Tessellate(mgr.Frame().Commands, nil)
	if len(verts) == 0 || len(verts)%3 != 0 {
		t.Fatalf("invalid tessellated vertex count: %d", len(verts))
	}
	_ = image.NewRGBA(image.Rect(0, 0, 280, 120))
}

func TestAutoSizeAndSizeToText(t *testing.T) {
	mgr := New()
	stack := NewStack(Vertical, 6)

	btn := NewButton("OK", nil)
	mgr.SetRoot(stack)
	stack.Add(btn)
	mgr.BeginFrame(400, 200, 0.016)
	defaultW := btn.Bounds().W
	if defaultW < 240 {
		t.Fatalf("expected default stretched/min button width >= 240, got %v", defaultW)
	}

	// Enable AutoSize: button shrinks to fit "OK" and is not stretched by Panel
	btn.SetAutoSize(true)
	mgr.BeginFrame(400, 200, 0.016)
	shortW := btn.Bounds().W
	if shortW >= 120 {
		t.Fatalf("expected AutoSize button with 'OK' to shrink (<120px), got %v", shortW)
	}

	// Changing text automatically grows the button
	btn.SetText("Очень длинный текст на кнопке с авторазмером")
	mgr.BeginFrame(400, 200, 0.016)
	longW := btn.Bounds().W
	if longW <= shortW*2 {
		t.Fatalf("expected AutoSize button to grow for longer text: short=%v long=%v", shortW, longW)
	}

	// On-demand SizeToText() on a fixed-size Label and TextField
	lbl := NewLabel("Short")
	lbl.SetPreferredSize(Vec2{X: 300, Y: 40})
	fitted := lbl.SizeToText()
	if fitted.X >= 150 || lbl.PreferredSize().X != fitted.X {
		t.Fatalf("SizeToText did not shrink label preferred size: %+v", fitted)
	}

	field := NewTextField("Hi", nil)
	field.SetAutoSize(true)
	stack.Add(field)
	mgr.BeginFrame(400, 200, 0.016)
	w1 := field.Bounds().W
	field.SetText("Hello World AutoResizing TextField!")
	mgr.BeginFrame(400, 200, 0.016)
	w2 := field.Bounds().W
	if w2 <= w1 {
		t.Fatalf("expected AutoSize TextField to expand when text grows: w1=%v w2=%v", w1, w2)
	}
}
