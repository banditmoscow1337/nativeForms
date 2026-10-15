// Package desktop selects the native window adapter for the current OS.
// Applications can keep one main package for Windows, macOS and Linux.
package desktop

import (
	"fmt"

	ui "github.com/banditmoscow1337/nativeForms"
	"github.com/banditmoscow1337/nativeForms/platform"
	"github.com/banditmoscow1337/nativeForms/render"
)

// Window is the shared desktop API. Use a platform-specific adapter directly
// when an application needs methods outside this interface.
type Window interface {
	platform.FileDialogs
	platform.TextClipboard
	Run() error
	Close()
	Show()
	Hide()
	SetSize(width, height int) error
	Capabilities() platform.Capabilities
}

type Options struct {
	Title          string
	Width, Height  int // Logical client dimensions; zero uses 800x600.
	OnReady        func(Window)
	OnFilesDropped func([]string)
	// OnCloseRequest runs on the UI goroutine when the user asks to close the
	// window. A programmatic Close bypasses this policy.
	OnCloseRequest func(Window) platform.CloseAction
	Backend        string      // Linux: "", "x11" or "wayland"; empty selects the session.
	Graphics       render.Kind // "auto", "software", or "hardware" (when installed).
	X11Scale       float32     // Linux X11 only; zero uses the Xft.dpi setting.
}

// App owns the desktop UI loop. Register windows before Run or from a callback
// posted to the running UI goroutine. An App remains alive with no visible
// windows until Quit; this lets a later tray integration own the last window.
type App struct{ driver appDriver }

type appDriver interface {
	Run() error
	Post(func())
	Quit()
	Add(Window) error
}

func NewApp() *App { return &App{driver: newAppDriver()} }

func (a *App) NewWindow(manager *ui.Manager, options Options) (Window, error) {
	if a == nil || a.driver == nil {
		return nil, fmt.Errorf("desktop: nil app")
	}
	w, err := New(manager, options)
	if err != nil {
		return nil, err
	}
	if err := a.driver.Add(w); err != nil {
		return nil, err
	}
	return w, nil
}

// Run must be called from the main UI goroutine (the main OS thread on macOS).
func (a *App) Run() error {
	if a == nil || a.driver == nil {
		return fmt.Errorf("desktop: nil app")
	}
	return a.driver.Run()
}

// Post schedules a callback on the UI goroutine and wakes the native loop.
func (a *App) Post(fn func()) {
	if a != nil && a.driver != nil {
		a.driver.Post(fn)
	}
}

// Quit ends Run and destroys all registered native windows.
func (a *App) Quit() {
	if a != nil && a.driver != nil {
		a.driver.Quit()
	}
}

// New creates a window for the current platform. Run it on the UI goroutine;
// after Run starts, use Manager.Post to call SetSize from another goroutine.
func New(manager *ui.Manager, options Options) (Window, error) {
	return newWindow(manager, options)
}
