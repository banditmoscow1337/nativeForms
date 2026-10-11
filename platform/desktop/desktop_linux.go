//go:build linux

package desktop

import (
	"fmt"

	ui "github.com/banditmoscow1337/nativeForms"
	"github.com/banditmoscow1337/nativeForms/platform/linux"
)

func newWindow(manager *ui.Manager, options Options) (Window, error) {
	if manager == nil {
		return nil, fmt.Errorf("desktop: nil manager")
	}
	backend := options.Backend
	if backend == "" {
		var err error
		backend, err = linux.BackendName()
		if err != nil {
			return nil, err
		}
	}
	switch backend {
	case "wayland":
		return linux.NewWayland(manager, linux.WaylandOptions{
			Title: options.Title, Width: options.Width, Height: options.Height,
			OnReady: func(w *linux.WaylandWindow) {
				if options.OnReady != nil {
					options.OnReady(w)
				}
			}, OnFilesDropped: options.OnFilesDropped,
		}), nil
	case "x11":
		return linux.NewX11(manager, linux.X11Options{
			Title: options.Title, Width: options.Width, Height: options.Height, Scale: options.X11Scale,
			OnReady: func(w *linux.X11Window) {
				if options.OnReady != nil {
					options.OnReady(w)
				}
			}, OnFilesDropped: options.OnFilesDropped,
		}), nil
	default:
		return nil, fmt.Errorf("desktop: unknown Linux backend %q", backend)
	}
}
