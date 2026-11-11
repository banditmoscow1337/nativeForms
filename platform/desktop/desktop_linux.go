//go:build linux

package desktop

import (
	"fmt"

	ui "github.com/banditmoscow1337/nativeForms"
	"github.com/banditmoscow1337/nativeForms/platform"
	"github.com/banditmoscow1337/nativeForms/platform/linux"
)

type linuxDriver struct{ *linux.App }

func newAppDriver() appDriver            { return linuxDriver{linux.NewApp()} }
func (a linuxDriver) Add(w Window) error { return a.App.Add(w) }
func (a linuxDriver) SetTray(options TrayOptions) error {
	return a.App.SetTray(linux.TrayOptions{
		Title:   options.Title,
		Tooltip: options.Tooltip,
		Icon:    options.Icon,
		Menu:    options.Menu,
		OnClick: options.OnClick,
	})
}

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
			Title: options.Title, Width: options.Width, Height: options.Height, Scale: options.Scale, Graphics: options.Graphics,
			Frameless: options.Frameless, HideWindowControls: options.HideWindowControls,
			OnReady: func(w *linux.WaylandWindow) {
				if options.OnReady != nil {
					options.OnReady(w)
				}
			}, OnFilesDropped: options.OnFilesDropped,
			OnCloseRequest: func(w *linux.WaylandWindow) platform.CloseAction {
				if options.OnCloseRequest != nil {
					return options.OnCloseRequest(w)
				}
				return platform.CloseWindow
			},
		}), nil
	case "x11":
		x11Scale := options.X11Scale
		if x11Scale <= 0 {
			x11Scale = options.Scale
		}
		return linux.NewX11(manager, linux.X11Options{
			Title: options.Title, Width: options.Width, Height: options.Height, Scale: x11Scale, Graphics: options.Graphics,
			Frameless: options.Frameless, HideWindowControls: options.HideWindowControls,
			OnReady: func(w *linux.X11Window) {
				if options.OnReady != nil {
					options.OnReady(w)
				}
			}, OnFilesDropped: options.OnFilesDropped,
			OnCloseRequest: func(w *linux.X11Window) platform.CloseAction {
				if options.OnCloseRequest != nil {
					return options.OnCloseRequest(w)
				}
				return platform.CloseWindow
			},
		}), nil
	default:
		return nil, fmt.Errorf("desktop: unknown Linux backend %q", backend)
	}
}
