package linux

import (
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	ui "github.com/banditmoscow1337/nativeForms"
	"github.com/banditmoscow1337/nativeForms/platform"
)

type appTestWindow struct {
	app     *App
	manager *ui.Manager
	pipe    [2]int
	closed  atomic.Bool
	thread  int
	starts  int
	stops   int
	events  int
	seen    chan struct{}
	stopped chan struct{}
}

func TestNativeCloseRequestPolicy(t *testing.T) {
	for _, action := range []platform.CloseAction{platform.CloseWindow, platform.HideWindow, platform.QuitApp, platform.CancelClose} {
		xApp := NewApp()
		x := NewX11(ui.New(), X11Options{OnCloseRequest: func(*X11Window) platform.CloseAction { return action }})
		x.app = xApp
		x.closeAction()
		if x.closeRequested.Load() != (action == platform.CloseWindow) || x.hidden.Load() != (action == platform.HideWindow) || xApp.queue.Quitting() != (action == platform.QuitApp) {
			t.Errorf("X11 close action %d was not applied", action)
		}
		wlApp := NewApp()
		wl := NewWayland(ui.New(), WaylandOptions{OnCloseRequest: func(*WaylandWindow) platform.CloseAction { return action }})
		wl.app = wlApp
		wl.closeAction()
		if wl.closing.Load() != (action == platform.CloseWindow) || wl.hidden.Load() != (action == platform.HideWindow) || wlApp.queue.Quitting() != (action == platform.QuitApp) {
			t.Errorf("Wayland close action %d was not applied", action)
		}
	}
}

func (w *appTestWindow) appSetApp(a *App)        { w.app = a }
func (w *appTestWindow) appManager() *ui.Manager { return w.manager }
func (w *appTestWindow) appOwner() *App          { return w.app }
func (w *appTestWindow) appClosed() bool         { return w.closed.Load() }
func (w *appTestWindow) appFD() (int, error)     { return w.pipe[0], nil }
func (w *appTestWindow) appStart() error         { w.thread = syscall.Gettid(); w.starts++; return nil }
func (w *appTestWindow) appStop() {
	w.stops++
	_ = syscall.Close(w.pipe[0])
	_ = syscall.Close(w.pipe[1])
	close(w.stopped)
}
func (w *appTestWindow) appStep(read bool) (time.Time, error) {
	if syscall.Gettid() != w.thread {
		panic("appStep ran on another OS thread")
	}
	if read {
		var b [1]byte
		if _, err := syscall.Read(w.pipe[0], b[:]); err != nil {
			return time.Time{}, err
		}
		w.events++
		if w.events == 1 {
			close(w.seen)
		}
	}
	return time.Now().Add(time.Second), nil
}

func newAppTestWindow(t *testing.T) *appTestWindow {
	t.Helper()
	w := &appTestWindow{manager: ui.New(), seen: make(chan struct{}), stopped: make(chan struct{})}
	if err := syscall.Pipe2(w.pipe[:], syscall.O_CLOEXEC|syscall.O_NONBLOCK); err != nil {
		t.Fatal(err)
	}
	return w
}

func TestAppPostMultipleWindowsAndEmptyLoop(t *testing.T) {
	a := NewApp()
	first, second := newAppTestWindow(t), newAppTestWindow(t)
	done := make(chan error, 1)
	go func() { done <- a.Run() }() // Run begins with zero windows.
	ready := make(chan struct{})
	a.Post(func() {
		if err := a.Add(first); err != nil {
			panic(err)
		}
		if err := a.Add(second); err != nil {
			panic(err)
		}
		close(ready)
	})
	select {
	case <-ready:
	case <-time.After(2 * time.Second):
		t.Fatal("App.Post did not wake empty loop")
	}
	if _, err := syscall.Write(first.pipe[1], []byte{1}); err != nil {
		t.Fatal(err)
	}
	if _, err := syscall.Write(second.pipe[1], []byte{1}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-second.seen:
	case <-time.After(2 * time.Second):
		t.Fatal("selector did not dispatch the second window")
	}
	a.Post(func() {
		if syscall.Gettid() != first.thread || first.thread != second.thread {
			panic("callbacks and windows do not share a UI thread")
		}
		first.closed.Store(true)
	})
	select {
	case <-first.stopped:
	case <-time.After(2 * time.Second):
		t.Fatal("closing the first window stopped the selector")
	}
	// Exit with one native window still registered.
	a.Quit()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Quit did not wake the selector")
	}
	if first.starts != 1 || second.starts != 1 || first.stops != 1 || second.stops != 1 || second.events == 0 {
		t.Fatalf("unexpected lifecycle: first=%+v second=%+v", first, second)
	}
}
