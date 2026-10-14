//go:build windows && (amd64 || arm64)

package desktop

import (
	"fmt"

	ui "github.com/banditmoscow1337/nativeForms"
	"github.com/banditmoscow1337/nativeForms/platform"
	"github.com/banditmoscow1337/nativeForms/platform/windows"
)

type windowsDriver struct{ *windows.App }

func newAppDriver() appDriver { return windowsDriver{windows.NewApp()} }
func (a windowsDriver) Add(w Window) error {
	native, ok := w.(*windows.Window)
	if !ok {
		return fmt.Errorf("desktop: expected a Windows window, got %T", w)
	}
	return a.App.Add(native)
}

func newWindow(manager *ui.Manager, options Options) (Window, error) {
	if manager == nil {
		return nil, fmt.Errorf("desktop: nil manager")
	}
	if options.Backend != "" {
		return nil, fmt.Errorf("desktop: backend %q is only available on Linux", options.Backend)
	}
	return windows.New(manager, windows.Options{
		Title: options.Title, Width: options.Width, Height: options.Height,
		OnReady: func(w *windows.Window) {
			if options.OnReady != nil {
				options.OnReady(w)
			}
		}, OnFilesDropped: options.OnFilesDropped,
		OnCloseRequest: func(w *windows.Window) platform.CloseAction {
			if options.OnCloseRequest != nil {
				return options.OnCloseRequest(w)
			}
			return platform.CloseWindow
		},
	}), nil
}
