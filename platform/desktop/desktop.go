// Package desktop selects the native window adapter for the current OS.
// Applications can keep one main package for Windows, macOS and Linux.
package desktop

import (
	ui "github.com/banditmoscow1337/nativeForms"
	"github.com/banditmoscow1337/nativeForms/platform"
)

// Window is the shared desktop API. Use a platform-specific adapter directly
// when an application needs methods outside this interface.
type Window interface {
	platform.FileDialogs
	platform.TextClipboard
	Run() error
	Close()
	SetSize(width, height int) error
	Capabilities() platform.Capabilities
}

type Options struct {
	Title          string
	Width, Height  int // Logical client dimensions; zero uses 800x600.
	OnReady        func(Window)
	OnFilesDropped func([]string)
	Backend        string  // Linux: "", "x11" or "wayland"; empty selects the session.
	X11Scale       float32 // Linux X11 only; zero uses the Xft.dpi setting.
}

// New creates a window for the current platform. Run it on the UI goroutine;
// after Run starts, use Manager.Post to call SetSize from another goroutine.
func New(manager *ui.Manager, options Options) (Window, error) {
	return newWindow(manager, options)
}
