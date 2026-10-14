//go:build darwin && (amd64 || arm64)

package desktop

import (
	"fmt"
	"runtime"

	ui "github.com/banditmoscow1337/nativeForms"
	"github.com/banditmoscow1337/nativeForms/platform"
	"github.com/banditmoscow1337/nativeForms/platform/darwin"
)

type darwinDriver struct{ *darwin.App }

func newAppDriver() appDriver { return darwinDriver{darwin.NewApp()} }
func (a darwinDriver) Add(w Window) error {
	native, ok := w.(*darwin.Window)
	if !ok {
		return fmt.Errorf("desktop: expected a macOS window, got %T", w)
	}
	return a.App.Add(native)
}

// AppKit needs main to remain on its startup OS thread. Package initialization
// runs on that thread before main, so applications need no platform-specific init.
func init() { runtime.LockOSThread() }

func newWindow(manager *ui.Manager, options Options) (Window, error) {
	if manager == nil {
		return nil, fmt.Errorf("desktop: nil manager")
	}
	if options.Backend != "" {
		return nil, fmt.Errorf("desktop: backend %q is only available on Linux", options.Backend)
	}
	return darwin.New(manager, darwin.Options{
		Title: options.Title, Width: options.Width, Height: options.Height,
		OnReady: func(w *darwin.Window) {
			if options.OnReady != nil {
				options.OnReady(w)
			}
		}, OnFilesDropped: options.OnFilesDropped,
		OnCloseRequest: func(w *darwin.Window) platform.CloseAction {
			if options.OnCloseRequest != nil {
				return options.OnCloseRequest(w)
			}
			return platform.CloseWindow
		},
	}), nil
}
