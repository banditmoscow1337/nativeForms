//go:build linux

// Package linux contains independent X11 wire and Wayland discovery adapters.
package linux

import (
	"fmt"
	"os"

	"github.com/banditmoscow1337/nativeForms/platform"
)

func X11Capabilities() platform.Capabilities {
	return platform.Capabilities{SoftwareFrame: true, Pointer: true, Keyboard: true, TextInput: true}
}

func WaylandCapabilities() platform.Capabilities {
	return platform.Capabilities{Wayland: true}
}

// BackendName selects a display endpoint. An unsupported Wayland session is
// reported explicitly; it is never silently routed through Xwayland.
func BackendName() (string, error) {
	if os.Getenv("WAYLAND_DISPLAY") != "" {
		return "wayland", nil
	}
	if os.Getenv("DISPLAY") != "" {
		return "x11", nil
	}
	return "", fmt.Errorf("linux: neither WAYLAND_DISPLAY nor DISPLAY is set")
}
