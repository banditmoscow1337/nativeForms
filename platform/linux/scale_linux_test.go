//go:build linux

package linux

import "testing"

func TestX11ResourceScale(t *testing.T) {
	for _, tc := range []struct {
		resources string
		want      float32
	}{
		{"Xft.dpi:\t144\nXcursor.size: 32", 1.5},
		{"*Xft.dpi: 192\n", 2},
		{"Xft.dpi: nope", 1},
		{"Xft.dpi: 100000", 1},
	} {
		if got := x11ResourceScale(tc.resources); got != tc.want {
			t.Errorf("x11ResourceScale(%q) = %g, want %g", tc.resources, got, tc.want)
		}
	}
}

func TestWaylandOutputScale(t *testing.T) {
	w := &WaylandWindow{compositorVersion: 4, outputs: map[uint32]*wlOutput{
		1: {scale: 2, entered: true}, 2: {scale: 3},
	}}
	if got := w.desiredScale(); got != 2 {
		t.Fatalf("first output: got %g, want 2", got)
	}
	w.outputs[2].entered = true
	if got := w.desiredScale(); got != 3 {
		t.Fatalf("mixed outputs: got %g, want 3", got)
	}
	w.outputs[1].entered, w.outputs[2].entered = false, false
	if got := w.desiredScale(); got != 1 {
		t.Fatalf("no outputs: got %g, want 1", got)
	}
	w.compositorVersion = 1
	w.outputs[1].entered = true
	if got := w.desiredScale(); got != 1 {
		t.Fatalf("legacy compositor: got %g, want 1", got)
	}
	w.viewport = 9
	w.preferredScale = 1.25
	if got := w.desiredScale(); got != 1.25 {
		t.Fatalf("fractional preference: got %g, want 1.25", got)
	}
}
