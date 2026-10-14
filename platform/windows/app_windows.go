//go:build windows && (amd64 || arm64)

package windows

import (
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"

	"github.com/banditmoscow1337/nativeForms/platform"
)

// App owns the Win32 message queue, independent of its windows.
type App struct {
	queue    platform.AppQueue
	windows  []*Window // UI goroutine only after Run begins
	running  atomic.Bool
	started  atomic.Bool
	threadID atomic.Uint32
	autoQuit bool // legacy Window.Run
	runErr   error
}

var appMu sync.Mutex

func NewApp() *App { return &App{} }

// Add registers a window. While Run is active call Add only on its UI thread.
func (a *App) Add(w *Window) error {
	if a == nil || w == nil || w.manager == nil {
		return fmt.Errorf("windows: nil app, window or manager")
	}
	if a.running.Load() && uint32(call(win.getCurrentThreadId)) != a.threadID.Load() {
		return fmt.Errorf("windows: Add must run on the App UI thread (use App.Post)")
	}
	if a.queue.Quitting() {
		return fmt.Errorf("windows: application is quitting")
	}
	if w.app != nil {
		return fmt.Errorf("windows: window is already registered")
	}
	for _, other := range a.windows {
		if other.manager == w.manager {
			return fmt.Errorf("windows: windows must have distinct managers")
		}
	}
	w.app = a
	a.windows = append(a.windows, w)
	if a.running.Load() {
		if err := w.open(); err != nil {
			a.remove(w)
			w.app = nil
			return err
		}
	}
	return nil
}

func (a *App) remove(w *Window) {
	for i, existing := range a.windows {
		if existing == w {
			a.windows = append(a.windows[:i], a.windows[i+1:]...)
			return
		}
	}
}

func (a *App) closed(w *Window) {
	a.remove(w)
	if w.err != nil && a.runErr == nil {
		a.runErr = w.err
		a.Quit()
	}
	if a.autoQuit && len(a.windows) == 0 {
		a.Quit()
	}
}

func (a *App) Post(fn func()) { a.queue.Post(fn) }
func (a *App) Quit()          { a.queue.Quit() }

func (a *App) Run() error {
	if a == nil {
		return fmt.Errorf("windows: nil app")
	}
	if !appMu.TryLock() {
		return fmt.Errorf("windows: an application is already running")
	}
	defer appMu.Unlock()
	if !a.started.CompareAndSwap(false, true) {
		return fmt.Errorf("windows: App.Run may only be called once")
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	apiOnce.Do(func() { apiError = load() })
	if apiError != nil {
		return apiError
	}
	classOnce.Do(registerWindowClass)
	if classError != nil {
		return classError
	}
	// PeekMessage creates a queue even if the app starts without a window.
	var msg message
	call(win.peekMessage, pointer(&msg), 0, 0, 0, 0)
	a.threadID.Store(uint32(call(win.getCurrentThreadId)))
	a.running.Store(true)
	a.queue.SetWake(func() { call(win.postThreadMessage, uintptr(a.threadID.Load()), wmAppWake, 0, 0) })
	defer func() {
		a.queue.SetWake(nil)
		a.queue.Quit()
		for len(a.windows) > 0 {
			w := a.windows[0]
			if h := w.hwnd.Load(); h != 0 {
				call(win.destroyWindow, h)
			} else {
				a.remove(w)
			}
		}
		a.running.Store(false)
		a.threadID.Store(0)
	}()
	for _, w := range append([]*Window(nil), a.windows...) {
		if err := w.open(); err != nil {
			return err
		}
	}
	for !a.queue.Drain() {
		result := call(win.getMessage, pointer(&msg), 0, 0, 0)
		if int32(result) == -1 {
			return fmt.Errorf("windows: GetMessageW failed")
		}
		if result == 0 {
			break
		}
		if msg.HWND != 0 {
			call(win.translateMessage, pointer(&msg))
			call(win.dispatchMessage, pointer(&msg))
		}
	}
	return a.runErr
}
