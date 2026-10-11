//go:build linux

package desktop

import (
	"testing"

	ui "github.com/banditmoscow1337/nativeForms"
	"github.com/banditmoscow1337/nativeForms/platform/linux"
)

func TestExplicitLinuxBackendNeedsNoSessionProbe(t *testing.T) {
	for _, backend := range []string{"x11", "wayland"} {
		window, err := New(ui.New(), Options{Backend: backend})
		if err != nil {
			t.Fatal(err)
		}
		switch backend {
		case "x11":
			if _, ok := window.(*linux.X11Window); !ok {
				t.Fatalf("got %T for x11", window)
			}
		case "wayland":
			if _, ok := window.(*linux.WaylandWindow); !ok {
				t.Fatalf("got %T for wayland", window)
			}
		}
		if err := window.SetSize(1024, 768); err != nil {
			t.Fatalf("%s pre-run SetSize: %v", backend, err)
		}
		if err := window.SetSize(-1, 768); err == nil {
			t.Fatalf("%s accepted negative width", backend)
		}
	}
	if _, err := New(ui.New(), Options{Backend: "missing"}); err == nil {
		t.Fatal("unknown backend was accepted")
	}
}
