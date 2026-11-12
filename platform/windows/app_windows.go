//go:build windows && (amd64 || arm64)

package windows

import (
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"unsafe"

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

// App owns the Win32 message queue, independent of its windows.
type App struct {
	queue       platform.AppQueue
	windows     []*Window // UI goroutine only after Run begins
	running     atomic.Bool
	started     atomic.Bool
	wakePending atomic.Bool
	threadID    atomic.Uint32
	autoQuit    bool // legacy Window.Run
	runErr      error
	trayMu      sync.Mutex
	tray        *TrayOptions
	trayAdded   bool
	trayHWND    uintptr
	trayIcon    uintptr

	// Heap-allocated Win32 message & tray structs so stack moves never
	// invalidate pointers passed to Win32 syscalls.
	msg        message
	nid        notifyIconData
	trayCursor point
	iconInfo   iconInfo
	iconBMI    bitmapInfo
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
		a.syncTray()
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
	a.trayMu.Lock()
	hasTray := a.tray != nil
	a.trayMu.Unlock()
	if hasTray && a.trayAdded && len(a.windows) > 0 {
		// Rebind the tray icon to a remaining window if the previous owner closed.
		a.syncTray()
	}
	if a.autoQuit && len(a.windows) == 0 && !hasTray {
		a.Quit()
	}
}

func (a *App) Post(fn func()) { a.queue.Post(fn) }
func (a *App) Quit()          { a.queue.Quit() }

func (a *App) runOnUI(fn func()) {
	if a == nil || fn == nil || !a.running.Load() {
		return
	}
	if uint32(call(win.getCurrentThreadId)) == a.threadID.Load() {
		busy := false
		for _, w := range a.windows {
			if w.painting || w.inEvent {
				busy = true
				break
			}
		}
		if !busy {
			fn()
			return
		}
	}
	a.Post(fn)
}

func (a *App) SetTray(options TrayOptions) error {
	if a == nil {
		return fmt.Errorf("windows: nil app")
	}
	copied := options
	copied.Menu = append([]ui.MenuItem(nil), options.Menu...)
	a.trayMu.Lock()
	a.tray = &copied
	a.trayMu.Unlock()
	a.runOnUI(a.syncTray)
	return nil
}

func (a *App) RemoveTray() {
	if a == nil {
		return
	}
	a.trayMu.Lock()
	a.tray = nil
	a.trayMu.Unlock()
	a.runOnUI(a.syncTray)
}

func (a *App) invokeTrayClick() {
	a.trayMu.Lock()
	var fn func()
	if a.tray != nil {
		if a.tray.OnClick != nil {
			fn = a.tray.OnClick
		} else if len(a.tray.Menu) > 0 && a.tray.Menu[0].Action != nil {
			fn = a.tray.Menu[0].Action
		}
	}
	a.trayMu.Unlock()
	if fn != nil {
		fn()
	}
}

func (a *App) showTrayMenu(hwnd uintptr) {
	a.trayMu.Lock()
	var items []ui.MenuItem
	if a.tray != nil && len(a.tray.Menu) > 0 {
		items = append([]ui.MenuItem(nil), a.tray.Menu...)
	}
	a.trayMu.Unlock()
	if len(items) == 0 {
		a.invokeTrayClick()
		return
	}
	if win.createPopupMenu == 0 || win.appendMenu == 0 || win.trackPopupMenu == 0 || win.destroyMenu == 0 {
		a.invokeTrayClick()
		return
	}
	menu := call(win.createPopupMenu)
	if menu == 0 {
		return
	}
	defer call(win.destroyMenu, menu)

	for i, item := range items {
		if item.Separator || item.Label == "-" {
			call(win.appendMenu, menu, mfSeparator, 0, 0)
			continue
		}
		label := item.Label
		if item.Shortcut != "" {
			label = label + "\t" + item.Shortcut
		}
		wLabel, err := wide(label)
		if err != nil {
			continue
		}
		flags := uintptr(mfString)
		if item.Disabled {
			flags |= mfDisabled | mfGrayed
		}
		if item.Checked {
			flags |= mfChecked
		}
		call(win.appendMenu, menu, flags, uintptr(i+1), ptr16(wLabel))
		keep(wLabel)
	}
	if win.getCursorPos != 0 {
		call(win.getCursorPos, pointer(&a.trayCursor))
	}
	if win.setForegroundWindow != 0 {
		call(win.setForegroundWindow, hwnd)
	}
	cmd := int(call(win.trackPopupMenu, menu, tpmRightButton|tpmBottomAlign|tpmNonotify|tpmReturnCmd,
		uintptr(a.trayCursor.X), uintptr(a.trayCursor.Y), 0, hwnd, 0))
	keep(a)
	if cmd > 0 && cmd <= len(items) {
		selected := items[cmd-1]
		if !selected.Disabled && selected.Action != nil {
			selected.Action()
		}
	}
}

func (a *App) makeTrayIcon(res *ui.ImageResource) uintptr {
	if res == nil || win.createDIBSection == 0 || win.createBitmap == 0 || win.createIconIndirect == 0 || win.deleteObject == 0 {
		return 0
	}
	b := res.Bounds()
	w, h := b.Dx(), b.Dy()
	if w <= 0 || h <= 0 || w > 256 || h > 256 {
		return 0
	}
	a.iconBMI = bitmapInfo{
		Size:        bitmapInfoHeaderSize,
		Width:       int32(w),
		Height:      -int32(h),
		Planes:      1,
		BitCount:    32,
		Compression: biRGB,
	}
	var bits uintptr
	colorBmp := call(win.createDIBSection, 0, pointer(&a.iconBMI), dibRGBColors, pointer(&bits), 0, 0)
	if colorBmp == 0 || bits == 0 {
		if colorBmp != 0 {
			call(win.deleteObject, colorBmp)
		}
		return 0
	}
	defer call(win.deleteObject, colorBmp)

	dst := unsafe.Slice((*byte)(unsafe.Add(nil, bits)), w*h*4)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			c := res.RGBAAt(b.Min.X+x, b.Min.Y+y)
			i := (y*w + x) * 4
			dst[i], dst[i+1], dst[i+2], dst[i+3] = c.B, c.G, c.R, c.A
		}
	}
	maskBmp := call(win.createBitmap, uintptr(w), uintptr(h), 1, 1, 0)
	if maskBmp == 0 {
		return 0
	}
	defer call(win.deleteObject, maskBmp)

	a.iconInfo = iconInfo{
		Icon:       1,
		MaskBitmap: maskBmp,
		ColorBmp:   colorBmp,
	}
	hicon := call(win.createIconIndirect, pointer(&a.iconInfo))
	keep(a)
	return hicon
}

func (a *App) destroyCustomTrayIcon() {
	if a.trayIcon != 0 && win.destroyIcon != 0 {
		call(win.destroyIcon, a.trayIcon)
		a.trayIcon = 0
	}
}

func (a *App) syncTray() {
	if win.shellNotifyIcon == 0 {
		return
	}
	a.trayMu.Lock()
	opts := a.tray
	a.trayMu.Unlock()
	if opts == nil {
		if a.trayAdded && a.trayHWND != 0 {
			a.nid = notifyIconData{
				Size: uint32(unsafe.Sizeof(notifyIconData{})),
				HWND: a.trayHWND,
				ID:   1,
			}
			call(win.shellNotifyIcon, nimDelete, pointer(&a.nid))
			keep(a)
			a.trayAdded = false
			a.trayHWND = 0
		}
		a.destroyCustomTrayIcon()
		return
	}
	var hwnd uintptr
	for _, w := range a.windows {
		if h := w.hwnd.Load(); h != 0 {
			hwnd = h
			break
		}
	}
	if hwnd == 0 {
		return
	}
	if a.trayAdded && a.trayHWND != 0 && a.trayHWND != hwnd {
		a.nid = notifyIconData{
			Size: uint32(unsafe.Sizeof(notifyIconData{})),
			HWND: a.trayHWND,
			ID:   1,
		}
		call(win.shellNotifyIcon, nimDelete, pointer(&a.nid))
		a.trayAdded = false
		a.trayHWND = 0
	}
	tip := opts.Tooltip
	if tip == "" {
		tip = opts.Title
	}
	if tip == "" {
		tip = "nativeForms"
	}
	a.destroyCustomTrayIcon()
	icon := a.makeTrayIcon(opts.Icon)
	if icon != 0 {
		a.trayIcon = icon
	} else {
		icon = call(win.loadIcon, 0, idiApplication)
	}
	a.nid = notifyIconData{
		Size:            uint32(unsafe.Sizeof(notifyIconData{})),
		HWND:            hwnd,
		ID:              1,
		Flags:           nifMessage | nifIcon | nifTip,
		CallbackMessage: wmTrayCallback,
		Icon:            icon,
	}
	if wTip, err := wide(tip); err == nil {
		copy(a.nid.Tip[:], wTip)
	}
	action := uintptr(nimAdd)
	if a.trayAdded && a.trayHWND == hwnd {
		action = nimModify
	}
	call(win.shellNotifyIcon, action, pointer(&a.nid))
	keep(a)
	a.trayAdded = true
	a.trayHWND = hwnd
}

func (a *App) wake() {
	if !a.wakePending.CompareAndSwap(false, true) {
		return
	}
	for _, w := range a.windows {
		if h := w.hwnd.Load(); h != 0 {
			if call(win.postMessage, h, wmAppWake, 0, 0) != 0 {
				return
			}
		}
	}
	if tid := a.threadID.Load(); tid != 0 {
		call(win.postThreadMessage, uintptr(tid), wmAppWake, 0, 0)
	}
}

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
	call(win.peekMessage, pointer(&a.msg), 0, 0, 0, 0)
	a.threadID.Store(uint32(call(win.getCurrentThreadId)))
	a.running.Store(true)
	a.queue.SetWake(a.wake)
	defer func() {
		a.RemoveTray()
		a.syncTray()
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
		a.wakePending.Store(false)
	}()
	for _, w := range append([]*Window(nil), a.windows...) {
		if err := w.open(); err != nil {
			return err
		}
	}
	a.syncTray()
	for !a.queue.Drain() {
		for _, w := range a.windows {
			w.manager.ProcessPending()
			if w.manager.NeedsFrame() {
				w.invalidate()
			}
		}
		if a.queue.Drain() {
			break
		}
		result := call(win.getMessage, pointer(&a.msg), 0, 0, 0)
		keep(a)
		if int32(result) == -1 {
			return fmt.Errorf("windows: GetMessageW failed")
		}
		if result == 0 {
			break
		}
		if a.msg.Message == wmAppWake {
			a.wakePending.Store(false)
		}
		if a.msg.HWND != 0 {
			call(win.translateMessage, pointer(&a.msg))
			call(win.dispatchMessage, pointer(&a.msg))
			keep(a)
		}
		// Coalesce queued input/wake bursts before the next frame paint.
		for i := 0; i < 64 && !a.queue.Quitting(); i++ {
			if call(win.peekMessage, pointer(&a.msg), 0, 0, 0, pmRemove) == 0 {
				break
			}
			keep(a)
			if a.msg.Message == 0x0012 { // WM_QUIT
				a.Quit()
				break
			}
			if a.msg.Message == wmAppWake {
				a.wakePending.Store(false)
			}
			if a.msg.HWND != 0 {
				call(win.translateMessage, pointer(&a.msg))
				call(win.dispatchMessage, pointer(&a.msg))
				keep(a)
			}
		}
	}
	return a.runErr
}
