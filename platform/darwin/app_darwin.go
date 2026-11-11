//go:build darwin && (amd64 || arm64)

package darwin

import (
	"fmt"
	"math"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/banditmoscow1337/nativeForms/platform"
	"github.com/ebitengine/purego/objc"
)

func (w *Window) Run() error { return Run(w) }

// App owns the AppKit event loop and survives hidden or closed windows.
type App struct {
	queue       platform.AppQueue
	windows     []*Window
	running     atomic.Bool
	started     atomic.Bool
	wakePending atomic.Bool
	sleeping    atomic.Bool
	wakeFn      func()
	autoQuit    bool // legacy Run(windows...)
	trayMu      sync.Mutex
	pendingTray *TrayOptions
	statusItem  objc.ID
	trayMenu    objc.ID
	trayActions map[int]func()
}

func NewApp() *App { return &App{} }

func (a *App) Add(w *Window) error {
	if a == nil || w == nil || w.manager == nil {
		return fmt.Errorf("darwin: nil app, window or manager")
	}
	if w.owner != nil {
		return fmt.Errorf("darwin: window is already registered")
	}
	if a.running.Load() && onMainThread() == 0 {
		return fmt.Errorf("darwin: Add must run on the AppKit UI thread (use App.Post)")
	}
	if a.queue.Quitting() {
		return fmt.Errorf("darwin: application is quitting")
	}
	for _, other := range a.windows {
		if other.manager == w.manager {
			return fmt.Errorf("darwin: windows must have distinct managers")
		}
	}
	w.owner = a
	a.windows = append(a.windows, w)
	if a.running.Load() {
		if err := w.Open(); err != nil {
			a.remove(w) // Open may already have destroyed and unregistered it.
			w.owner = nil
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

func (a *App) wake() {
	if a != nil && a.wakeFn != nil {
		a.wakeFn()
	}
}

func (a *App) Post(fn func()) { a.queue.Post(fn) }
func (a *App) Quit()          { a.queue.Quit() }

func (a *App) runOnMain(fn func()) {
	if !a.running.Load() {
		return
	}
	if onMainThread() != 0 {
		fn()
	} else {
		a.Post(fn)
	}
}

type application struct {
	app     objc.ID
	windows map[objc.ID]*Window
	owner   *App
}

var activeApplication *application
var viewWindows = make(map[objc.ID]*Window)

func Run(windows ...*Window) error {
	a := NewApp()
	a.autoQuit = true
	for _, w := range windows {
		if err := a.Add(w); err != nil {
			return err
		}
	}
	return a.Run()
}

func (a *App) needsCoreText() bool {
	for _, w := range a.windows {
		if w != nil && w.manager != nil && w.manager.FontContext() == nil {
			return true
		}
	}
	return len(a.windows) == 0
}

func (a *App) Run() error {
	if a == nil {
		return fmt.Errorf("darwin: nil app")
	}
	if !appMu.TryLock() {
		return fmt.Errorf("darwin: application is already running; use Window.Open on its UI goroutine")
	}
	defer appMu.Unlock()
	if !a.started.CompareAndSwap(false, true) {
		return fmt.Errorf("darwin: App.Run may only be called once")
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if err := loadAppKit(); err != nil {
		return err
	}
	if onMainThread() == 0 {
		return fmt.Errorf("darwin: Run must execute on the main OS thread; call runtime.LockOSThread in main's init")
	}
	if a.needsCoreText() {
		if err := EnableCoreText(); err != nil {
			return err
		}
	}
	pool := msgSend0(clsNSAutoreleasePool, selNew)
	defer msgSend0(pool, selDrain)
	app := &application{app: class("NSApplication").Send(selector("sharedApplication")), windows: make(map[objc.ID]*Window), owner: a}
	app.app.Send(selector("setActivationPolicy:"), 0)
	activeApplication = app
	a.running.Store(true)
	// Pre-allocate and retain a single immutable wake NSEvent so background
	// wakeups never allocate Objective-C objects or LockOSThread per call.
	wakeEvent := class("NSEvent").Send(selector("otherEventWithType:location:modifierFlags:timestamp:windowNumber:context:subtype:data1:data2:"),
		uint64(15), point{}, uint64(0), float64(0), uint64(0), objc.ID(0), int16(0), int64(0), int64(0))
	if wakeEvent != 0 {
		msgSend0(wakeEvent, selRetain)
		defer msgSend0(wakeEvent, selRelease)
	}
	a.wakeFn = func() {
		a.wakePending.Store(true)
		if a.sleeping.CompareAndSwap(true, false) {
			msgSend2(app.app, selPostEventAtStart, uintptr(wakeEvent), 1)
		}
	}
	a.queue.SetWake(a.wakeFn)
	defer func() {
		a.queue.SetWake(nil)
		a.wakeFn = nil
		a.queue.Quit()
		a.removeTrayOnMain()
		a.running.Store(false)
		for _, w := range app.windows {
			w.destroy()
		}
		activeApplication = nil
	}()
	a.trayMu.Lock()
	initialTray := a.pendingTray
	a.trayMu.Unlock()
	if initialTray != nil {
		a.applyTrayOnMain(*initialTray)
	}
	for _, w := range append([]*Window(nil), a.windows...) {
		if err := w.Open(); err != nil {
			return err
		}
	}
	app.app.Send(selector("finishLaunching"))
	app.app.Send(selector("activateIgnoringOtherApps:"), true)
	mode := nsString("kCFRunLoopDefaultMode")
	for !a.queue.Drain() {
		if a.autoQuit && len(app.windows) == 0 && a.statusItem == 0 {
			break
		}
		iteration := msgSend0(clsNSAutoreleasePool, selNew)
		err := app.step(mode)
		msgSend0(iteration, selDrain)
		if err != nil {
			return err
		}
	}
	return nil
}

func (a *application) step(mode objc.ID) error {
	a.owner.wakePending.Store(false)
	seconds := 1.0
	for _, w := range a.windows {
		if w.closing.Load() {
			w.destroy()
			continue
		}
		w.manager.ProcessPending()
		if !w.geomValid || w.cachedBoundsW <= 0 || w.cachedBoundsH <= 0 {
			if !msgSendBool0(w.window, selIsVisible) {
				continue
			}
			key := msgSendBool0(w.window, selIsKeyWindow)
			if w.wasKey && !key {
				w.cancelMarked()
				w.manager.CancelPointerCapture()
			}
			w.wasKey = key
			bounds := msgSendRect0(w.view, selBounds)
			backing := msgSendRect1(w.view, selConvertToBacking, bounds)
			scale := float32(msgSendFloat0(w.window, selBackingScale))
			if !(scale > 0 && scale <= 8) {
				scale = 1
			}
			w.cachedBoundsW = int(math.Ceil(bounds.Size.W))
			w.cachedBoundsH = int(math.Ceil(bounds.Size.H))
			w.cachedPixelW = int(math.Round(backing.Size.W))
			w.cachedPixelH = int(math.Round(backing.Size.H))
			w.cachedScale = scale
			w.geomValid = w.cachedBoundsW > 0 && w.cachedBoundsH > 0 && w.cachedPixelW > 0 && w.cachedPixelH > 0
		}
		w.syncInputFocus()
		scale := w.cachedScale
		w.manager.SetBackingScale(scale)
		uiScale := w.uiScale()
		width := platform.LogicalSize(w.cachedBoundsW, uiScale)
		height := platform.LogicalSize(w.cachedBoundsH, uiScale)
		pixelWidth, pixelHeight := w.cachedPixelW, w.cachedPixelH
		renderScale := min(scale*uiScale, 8)
		if width > 0 && height > 0 && pixelWidth > 0 && pixelHeight > 0 &&
			(w.manager.NeedsFrame() || width != w.width || height != w.height || pixelWidth != w.pixelWidth || pixelHeight != w.pixelHeight) {
			if err := w.paint(width, height, pixelWidth, pixelHeight, renderScale); err != nil {
				return err
			}
		}
		if next, scheduled := w.manager.NextFrameDeadline(); width > 0 && height > 0 && scheduled {
			if delay := time.Until(next).Seconds(); delay < seconds {
				seconds = max(0.001, delay)
			}
		}
	}
	for _, w := range a.windows {
		w.manager.ProcessPending()
		if w.width > 0 && w.height > 0 && w.manager.NeedsFrame() {
			seconds = 0
			break
		}
	}
	var event objc.ID
	if seconds > 0 {
		a.owner.sleeping.Store(true)
		if a.owner.wakePending.Swap(false) || a.owner.queue.HasPending() {
			a.owner.sleeping.Store(false)
			seconds = 0
		}
	}
	if seconds > 0 {
		date := msgSendFloat1(clsNSDate, selDateWithInterval, seconds)
		event = msgSend4(a.app, selNextEvent, ^uintptr(0), uintptr(date), uintptr(mode), 1)
		a.owner.sleeping.Store(false)
		a.owner.wakePending.Store(false)
	} else {
		event = msgSend4(a.app, selNextEvent, ^uintptr(0), 0, uintptr(mode), 1)
	}
	if event == 0 {
		return nil
	}
	for event != 0 {
		typ := msgSendUint0(event, selEventType)
		if typ != 15 { // 15 is App.Post's application-defined wake event.
			id := msgSend0(event, selEventWindow)
			w := a.windows[id]
			handled := false
			if w != nil {
				boundsH := w.cachedBoundsH
				if boundsH <= 0 {
					boundsH = w.height
				}
				if typ == 10 || typ == 11 || typ == 12 {
					w.event(event, boundsH)
					handled = true
				} else {
					switch typ {
					case 1, 2, 3, 4, 5, 6, 7, 9, 22, 25, 26, 27:
						p := msgSendPoint0(event, selLocationInWindow)
						if p.Y >= 0 && p.Y <= float64(boundsH) || typ == 2 || typ == 4 || typ == 6 || typ == 7 || typ == 9 || typ == 26 || typ == 27 {
							w.event(event, boundsH)
							if typ == 22 || typ == 5 || typ == 6 || typ == 7 {
								handled = true
							}
						}
					}
				}
			}
			if !handled {
				msgSend1(a.app, selSendEvent, uintptr(event))
			}
		}
		// Drain any other queued events before rendering the next frame.
		event = msgSend4(a.app, selNextEvent, ^uintptr(0), 0, uintptr(mode), 1)
	}
	return nil
}
