//go:build linux

package linux

import (
	"fmt"
	"net"
	"runtime"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	ui "github.com/banditmoscow1337/nativeForms"
	"github.com/banditmoscow1337/nativeForms/platform"
)

type TrayOptions struct {
	Title   string
	Tooltip string
	Icon    *ui.ImageResource
	Menu    []ui.MenuItem
	OnClick func()
}

type appWindow interface {
	appStart() error
	appStep(bool) (time.Time, error)
	appStop()
	appClosed() bool
	appFD() (int, error)
	appManager() *ui.Manager
	appOwner() *App
	appSetApp(*App)
}

// App multiplexes all X11 and Wayland connections on one locked UI thread.
// A pipe wakes the selector for App.Post, Manager.Post, Close and Quit.
type App struct {
	queue    platform.AppQueue
	windows  []appWindow
	running  atomic.Bool
	started  atomic.Bool
	threadID atomic.Int32
	wakeMu   sync.Mutex
	wakeFD   int
	autoQuit bool // legacy Window.Run
	runErr   error
	trayMu   sync.Mutex
	tray     *TrayOptions
}

var appMu sync.Mutex

func NewApp() *App { return &App{wakeFD: -1} }

func (a *App) SetTray(options TrayOptions) error {
	if a == nil {
		return fmt.Errorf("linux: nil app")
	}
	copied := options
	copied.Menu = append([]ui.MenuItem(nil), options.Menu...)
	a.trayMu.Lock()
	a.tray = &copied
	a.trayMu.Unlock()
	a.wake()
	return nil
}

func (a *App) RemoveTray() {
	if a == nil {
		return
	}
	a.trayMu.Lock()
	a.tray = nil
	a.trayMu.Unlock()
	a.wake()
}

func (a *App) Tray() (TrayOptions, bool) {
	if a == nil {
		return TrayOptions{}, false
	}
	a.trayMu.Lock()
	defer a.trayMu.Unlock()
	if a.tray == nil {
		return TrayOptions{}, false
	}
	return *a.tray, true
}

// Add accepts an X11Window or WaylandWindow. Call it before Run or on the UI
// thread inside App.Post; distinct windows must have distinct managers.
func (a *App) Add(window any) error {
	if a == nil {
		return fmt.Errorf("linux: nil app")
	}
	w, ok := window.(appWindow)
	if !ok || window == (*X11Window)(nil) || window == (*WaylandWindow)(nil) || w.appManager() == nil {
		return fmt.Errorf("linux: expected a non-nil X11Window or WaylandWindow")
	}
	if w.appOwner() != nil || w.appClosed() {
		return fmt.Errorf("linux: window is already registered or closed")
	}
	if a.running.Load() && int32(syscall.Gettid()) != a.threadID.Load() {
		return fmt.Errorf("linux: Add must run on the App UI thread (use App.Post)")
	}
	if a.queue.Quitting() {
		return fmt.Errorf("linux: application is quitting")
	}
	for _, other := range a.windows {
		if other == w || other.appManager() == w.appManager() {
			return fmt.Errorf("linux: window already registered or manager shared")
		}
	}
	w.appSetApp(a)
	a.windows = append(a.windows, w)
	if a.running.Load() {
		if err := w.appStart(); err != nil {
			w.appStop()
			a.remove(w)
			w.appSetApp(nil)
			return err
		}
	}
	a.wake()
	return nil
}

func (a *App) remove(w appWindow) {
	for i, other := range a.windows {
		if other == w {
			a.windows = append(a.windows[:i], a.windows[i+1:]...)
			return
		}
	}
}

func (a *App) wake() {
	a.wakeMu.Lock()
	if a.wakeFD >= 0 {
		_, _ = syscall.Write(a.wakeFD, []byte{1}) // EAGAIN means a wake is pending.
	}
	a.wakeMu.Unlock()
}

func (a *App) Post(fn func()) { a.queue.Post(fn) }
func (a *App) Quit()          { a.queue.Quit() }
func (a *App) fail(err error) {
	if a.runErr == nil {
		a.runErr = err
	}
	a.Quit()
}

func (a *App) Run() error {
	if a == nil {
		return fmt.Errorf("linux: nil app")
	}
	if !appMu.TryLock() {
		return fmt.Errorf("linux: an application is already running")
	}
	defer appMu.Unlock()
	if !a.started.CompareAndSwap(false, true) {
		return fmt.Errorf("linux: App.Run may only be called once")
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	var pipe [2]int
	if err := syscall.Pipe2(pipe[:], syscall.O_NONBLOCK|syscall.O_CLOEXEC); err != nil {
		return err
	}
	a.wakeMu.Lock()
	a.wakeFD = pipe[1]
	a.wakeMu.Unlock()
	a.threadID.Store(int32(syscall.Gettid()))
	a.running.Store(true)
	a.queue.SetWake(a.wake)
	defer func() {
		a.queue.SetWake(nil)
		a.queue.Quit()
		for _, w := range a.windows {
			w.appStop()
		}
		a.running.Store(false)
		a.threadID.Store(0)
		a.wakeMu.Lock()
		a.wakeFD = -1
		_ = syscall.Close(pipe[1])
		a.wakeMu.Unlock()
		_ = syscall.Close(pipe[0])
	}()
	for _, w := range append([]appWindow(nil), a.windows...) {
		if err := w.appStart(); err != nil {
			return err
		}
	}
	ready := make(map[appWindow]bool)
	for !a.queue.Drain() {
		deadline := time.Now().Add(50 * time.Millisecond)
		for _, w := range append([]appWindow(nil), a.windows...) {
			if w.appClosed() {
				w.appStop()
				a.remove(w)
				continue
			}
			next, err := w.appStep(ready[w])
			if err != nil {
				return err
			}
			if w.appClosed() {
				w.appStop()
				a.remove(w)
			} else if !next.IsZero() && next.Before(deadline) {
				deadline = next
			}
		}
		if a.runErr != nil {
			return a.runErr
		}
		_, hasTray := a.Tray()
		if a.autoQuit && len(a.windows) == 0 && !hasTray {
			break
		}
		if a.queue.Drain() {
			break
		}
		ready = make(map[appWindow]bool)
		var fds syscall.FdSet
		if err := addFD(&fds, pipe[0]); err != nil {
			return err
		}
		maxFD := pipe[0]
		windowFDs := make(map[appWindow]int, len(a.windows))
		for _, w := range a.windows {
			fd, err := w.appFD()
			if err != nil {
				return err
			}
			if err := addFD(&fds, fd); err != nil {
				return err
			}
			windowFDs[w] = fd
			maxFD = max(maxFD, fd)
		}
		wait := max(time.Duration(0), time.Until(deadline))
		tv := syscall.NsecToTimeval(wait.Nanoseconds())
		_, err := syscall.Select(maxFD+1, &fds, nil, nil, &tv)
		if err == syscall.EINTR {
			continue
		}
		if err != nil {
			return err
		}
		if hasFD(&fds, pipe[0]) {
			var buf [256]byte
			for {
				if _, err := syscall.Read(pipe[0], buf[:]); err != nil {
					break // nonblocking EAGAIN
				}
			}
		}
		for w, fd := range windowFDs {
			ready[w] = hasFD(&fds, fd)
		}
	}
	return a.runErr
}

func addFD(set *syscall.FdSet, fd int) error {
	if fd < 0 || fd >= len(set.Bits)*64 {
		return fmt.Errorf("linux: file descriptor %d exceeds select capacity", fd)
	}
	set.Bits[fd/64] |= 1 << uint(fd%64)
	return nil
}

func hasFD(set *syscall.FdSet, fd int) bool {
	return set.Bits[fd/64]&(1<<uint(fd%64)) != 0
}

func unixFD(conn *net.UnixConn) (int, error) {
	raw, err := conn.SyscallConn()
	if err != nil {
		return 0, err
	}
	var fd int
	err = raw.Control(func(value uintptr) { fd = int(value) })
	return fd, err
}
