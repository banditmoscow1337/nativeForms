package nativeforms

import (
	"math"
	"testing"
)

func TestSliderStepStartsAtMinimumAndIncludesMaximum(t *testing.T) {
	var changes []float32
	slider := NewSlider("Volume", 3, 10, 4.1, 2, func(v float32) {
		changes = append(changes, v)
	})
	if slider.Value != 5 || len(changes) != 0 {
		t.Fatalf("initial value = %v, callbacks = %v; want 5 and none", slider.Value, changes)
	}
	slider.SetValue(4.9)
	slider.SetValue(7.2)
	slider.SetValue(9.4)
	slider.SetValue(9.5)
	slider.SetValue(20)
	slider.SetValue(float32(math.NaN()))
	slider.SetValue(float32(math.Inf(1)))
	if slider.Value != 10 {
		t.Fatalf("maximum is not reachable: got %v", slider.Value)
	}
	want := []float32{7, 9, 10}
	if len(changes) != len(want) {
		t.Fatalf("callbacks = %v; want %v", changes, want)
	}
	for i := range want {
		if changes[i] != want[i] {
			t.Fatalf("callbacks = %v; want %v", changes, want)
		}
	}

	reversed := NewSlider("Gain", 10, 3, 10, 2, nil)
	if reversed.Min != 3 || reversed.Max != 10 || reversed.Value != 10 {
		t.Fatalf("reversed range: min=%v max=%v value=%v", reversed.Min, reversed.Max, reversed.Value)
	}
	reversed.Min, reversed.Max = 10, 3 // Public fields may also be changed after construction.
	reversed.SetValue(5)
	if reversed.Value != 5 || reversed.Min != 10 || reversed.Max != 3 {
		t.Fatalf("setting a value unexpectedly changed the range: %+v", reversed)
	}
}

func TestSliderPointerDragAndKeyboard(t *testing.T) {
	slider := NewSlider("Volume", 0, 100, 50, 1, nil)
	manager := New()
	manager.SetInteractive(true)
	manager.SetRoot(slider)
	manager.BeginFrame(360, 80, 0)
	track := slider.trackRect()

	if !manager.HandleEvent(Event{Type: PointerDown, Button: MouseLeft, X: track.X, Y: track.Y}) || slider.Value != 0 {
		t.Fatalf("pointer down: value = %v; want 0", slider.Value)
	}
	if !manager.HandleEvent(Event{Type: PointerMove, X: track.X + track.W + 60, Y: -10}) || slider.Value != 100 {
		t.Fatalf("captured drag outside: value = %v; want 100", slider.Value)
	}
	if !manager.HandleEvent(Event{Type: PointerUp, Button: MouseLeft, X: track.X + track.W + 60, Y: -10}) || manager.captured != nil {
		t.Fatal("pointer release did not end the drag")
	}
	manager.HandleEvent(Event{Type: PointerMove, X: track.X, Y: track.Y})
	if slider.Value != 100 {
		t.Fatal("slider changed after pointer release")
	}
	manager.HandleEvent(Event{Type: PointerDown, Button: MouseRight, X: track.X, Y: track.Y})
	manager.HandleEvent(Event{Type: PointerUp, Button: MouseRight, X: track.X, Y: track.Y})
	if slider.Value != 100 {
		t.Fatal("right mouse button changed the value")
	}

	manager.setFocus(slider)
	for _, tc := range []struct {
		key  Key
		want float32
	}{
		{KeyLeftArrow, 99}, {KeyDownArrow, 98}, {KeyHome, 0},
		{KeyUpArrow, 1}, {KeyEnd, 100},
	} {
		if !manager.HandleEvent(Event{Type: KeyDown, Key: tc.key}) || slider.Value != tc.want {
			t.Fatalf("key %v: value = %v; want %v", tc.key, slider.Value, tc.want)
		}
	}
	slider.SetEnabled(false)
	if manager.HandleEvent(Event{Type: PointerDown, Button: MouseLeft, X: track.X, Y: track.Y}) || slider.Value != 100 {
		t.Fatal("disabled slider accepted pointer input")
	}

	continuous := NewSlider("Continuous", 0, 100, 50, 0, nil)
	continuous.Handle(Event{Type: KeyDown, Key: KeyRightArrow})
	if continuous.Value != 55 {
		t.Fatalf("continuous keyboard increment = %v; want 55", continuous.Value)
	}
}

func TestSliderAccessibilityUsesSameStep(t *testing.T) {
	slider := NewSlider("Gain", 3, 10, 5, 2, nil)
	manager := New()
	manager.SetRoot(slider)
	manager.SetInteractive(true)
	manager.BeginFrame(360, 80, 0)
	id := slider.AccessibilityID()
	node, ok := manager.AccessibilitySnapshot().Node(id)
	if !ok || node.Role != RoleSlider || node.Range == nil ||
		node.Range.Min != 3 || node.Range.Max != 10 || node.Range.Value != 5 || node.Range.Step != 2 {
		t.Fatalf("unexpected slider accessibility node: %+v, found=%v", node, ok)
	}
	if !manager.PerformAccessibilityAction(id, AccessibilityIncrement, "") || slider.Value != 7 {
		t.Fatalf("accessibility increment = %v; want 7", slider.Value)
	}
	if !manager.PerformAccessibilityAction(id, AccessibilitySetValue, "9.5") || slider.Value != 10 {
		t.Fatalf("accessibility maximum = %v; want 10", slider.Value)
	}
	if manager.PerformAccessibilityAction(id, AccessibilitySetValue, "NaN") || slider.Value != 10 {
		t.Fatal("accessibility accepted a non-finite value")
	}
}

func TestToggleKeyboardRequestsRepaint(t *testing.T) {
	toggle := NewToggle("Mute", false, nil)
	manager := New()
	manager.SetInteractive(true)
	manager.SetRoot(toggle)
	manager.BeginFrame(320, 60, 0)
	manager.setFocus(toggle)
	manager.BeginFrame(320, 60, 0)

	if !manager.HandleEvent(Event{Type: KeyDown, Key: KeySpace}) || !toggle.Value || !manager.NeedsFrame() {
		t.Fatal("keyboard toggle did not update its value and request a frame")
	}
}
