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
	Minimize()
	Maximize()
	SetSize(width, height int) error
	SetFrameless(frameless bool)
	Frameless() bool
	SetWindowControls(visible bool)
	WindowControlsVisible() bool
	Capabilities() platform.Capabilities
}

// TrayOptions configures the desktop system tray / menu bar status item.
type TrayOptions struct {
	Title   string
	Tooltip string
	Icon    *ui.ImageResource
	Menu    []ui.MenuItem
	OnClick func()
}

type Options struct {
	Title              string
	Width, Height      int  // Logical client dimensions; zero uses 800x600.
	Frameless          bool // Disables standard OS window decorations (titlebar and borders).
	HideWindowControls bool // Hides standard window control buttons (close/minimize/zoom).
	OnReady            func(Window)
	OnFilesDropped     func([]string)
	// OnCloseRequest runs on the UI goroutine when the user asks to close the
	// window. A programmatic Close bypasses this policy.
	OnCloseRequest func(Window) platform.CloseAction
	Backend        string      // Linux: "", "x11" or "wayland"; empty selects the session.
	Graphics       render.Kind // "auto", "software", or "hardware" (when installed).
	Scale          float32     // Initial UI/DPI scale factor (1.0 = 100%, 1.5 = 150%, etc.); zero keeps Manager.DisplayScale().
	X11Scale       float32     // Linux X11 only; zero uses the Xft.dpi setting.
}

// App owns the desktop UI loop. Register windows before Run or from a callback
// posted to the running UI goroutine. An App remains alive with no visible
// windows until Quit; this lets a tray integration own the last window.
type App struct{ driver appDriver }

type appDriver interface {
	Run() error
	Post(func())
	Quit()
	Add(Window) error
	SetTray(TrayOptions) error
	RemoveTray()
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

// SetTray installs or updates the application's system tray / status bar item.
func (a *App) SetTray(options TrayOptions) error {
	if a == nil || a.driver == nil {
		return fmt.Errorf("desktop: nil app")
	}
	return a.driver.SetTray(options)
}

// RemoveTray removes the application's system tray / status bar item.
func (a *App) RemoveTray() {
	if a != nil && a.driver != nil {
		a.driver.RemoveTray()
	}
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
