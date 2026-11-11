//go:build windows && (amd64 || arm64)

package windows

import (
	"fmt"
	"image"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	ui "github.com/banditmoscow1337/nativeForms"
	"github.com/banditmoscow1337/nativeForms/platform"
	"github.com/banditmoscow1337/nativeForms/render"
	"github.com/banditmoscow1337/nativeForms/software"
	"github.com/ebitengine/purego"
)

type Options struct {
	Title              string
	Width, Height      int
	Scale              float32
	Frameless          bool
	HideWindowControls bool
	OnReady            func(*Window)
	OnFilesDropped     func([]string)
	OnCloseRequest     func(*Window) platform.CloseAction
	Graphics           render.Kind
}

type Window struct {
	manager         *ui.Manager
	app             *App
	options         Options
	hwnd            atomic.Uintptr
	dpi             uint32
	renderer        render.Backend
	fallbackReason  string
	framebuffer     *image.RGBA
	dib             []byte
	dibValid        bool
	lastRenderScale float32
	lastFrame       time.Time
	highSurrogate   uint16
	suppressSysChar bool
	captureButton   ui.MouseButton
	hasCapture      bool
	err             error
}

func New(manager *ui.Manager, options Options) *Window {
	return &Window{manager: manager, options: options}
}

func (w *Window) windowStyle() uintptr {
	if w.options.Frameless {
		return wsPopup | wsThickFrame
	}
	if w.options.HideWindowControls {
		return wsCaption | wsThickFrame
	}
	return wsOverlappedWindow
}

func (w *Window) applyStyle() {
	if hwnd := w.hwnd.Load(); hwnd != 0 && win.setWindowLong != 0 {
		call(win.setWindowLong, hwnd, gwlStyle, w.windowStyle())
		call(win.setWindowPos, hwnd, 0, 0, 0, 0, 0, swpNoMove|swpNoSize|swpNoZOrder|swpNoActivate|swpFrameChanged)
		w.manager.InvalidateLayout()
		w.invalidate()
	}
}

func (w *Window) SetFrameless(frameless bool) {
	if w == nil {
		return
	}
	w.options.Frameless = frameless
	if w.app != nil && w.app.running.Load() {
		w.app.Post(w.applyStyle)
	}
}

func (w *Window) Frameless() bool {
	if w == nil {
		return false
	}
	return w.options.Frameless
}

func (w *Window) SetWindowControls(visible bool) {
	if w == nil {
		return
	}
	w.options.HideWindowControls = !visible
	if w.app != nil && w.app.running.Load() {
		w.app.Post(w.applyStyle)
	}
}

func (w *Window) WindowControlsVisible() bool {
	if w == nil {
		return false
	}
	return !w.options.HideWindowControls && !w.options.Frameless
}

func (w *Window) Minimize() {
	if w == nil || w.app == nil {
		return
	}
	w.app.Post(func() {
		if h := w.hwnd.Load(); h != 0 {
			call(win.showWindow, h, swMinimize)
		}
	})
}

func (w *Window) Maximize() {
	if w == nil || w.app == nil {
		return
	}
	w.app.Post(func() {
		if h := w.hwnd.Load(); h != 0 {
			call(win.showWindow, h, swMaximize)
		}
	})
}

// SetSize changes the logical client size. Call it on the window's UI goroutine
// after Run starts (for example from OnReady or Manager.Post).
func (w *Window) SetSize(width, height int) error {
	if w == nil || w.manager == nil {
		return fmt.Errorf("windows: nil window or manager")
	}
	if err := platform.ValidateSize(width, height, w.scale()); err != nil {
		return err
	}
	if hwnd := w.hwnd.Load(); hwnd != 0 {
		if err := w.sizeClient(hwnd, width, height); err != nil {
			return err
		}
	}
	w.options.Width, w.options.Height = width, height
	return nil
}

var creationMu sync.Mutex
var windowsMu sync.RWMutex
var windowsByHandle = make(map[uintptr]*Window)
var creatingWindow atomic.Pointer[Window] // synchronous CreateWindowExW callbacks
var apiOnce sync.Once
var apiError error
var classOnce sync.Once
var classError error
var className []uint16
var windowCallback uintptr

func (w *Window) Run() error {
	app := NewApp()
	app.autoQuit = true
	if err := app.Add(w); err != nil {
		return err
	}
	return app.Run()
}

func (w *Window) open() error {
	if w == nil || w.manager == nil {
		return fmt.Errorf("windows: nil window or manager")
	}
	if win.setDpi != 0 {
		call(win.setDpi, ^uintptr(3))
	} // per-monitor v2, when available
	width, height := w.options.Width, w.options.Height
	if width <= 0 {
		width = 800
	}
	if height <= 0 {
		height = 600
	}
	title := w.options.Title
	if title == "" {
		title = "nativeForms"
	}
	name, err := wide(title)
	if err != nil {
		return err
	}
	selection, err := software.Open(w.options.Graphics, nil)
	if err != nil {
		return err
	}
	w.renderer = selection.Backend
	w.fallbackReason = selection.FallbackReason()
	w.lastFrame = time.Now()
	w.dpi = 96
	w.err = nil
	style := w.windowStyle()
	creationMu.Lock()
	creatingWindow.Store(w)
	hwnd := call(win.createWindow, 0, ptr16(className), ptr16(name), style,
		cwUseDefault, cwUseDefault, uintptr(width), uintptr(height), 0, 0,
		call(win.getModuleHandle, 0), 0)
	creatingWindow.Store(nil)
	if hwnd != 0 {
		windowsMu.Lock()
		windowsByHandle[hwnd] = w
		windowsMu.Unlock()
	}
	creationMu.Unlock()
	keep(name, className)
	if hwnd == 0 {
		return fmt.Errorf("windows: CreateWindowExW failed")
	}
	w.hwnd.Store(hwnd)
	call(win.dragAcceptFiles, hwnd, 1)
	if win.getDpi != 0 {
		if dpi := uint32(call(win.getDpi, hwnd)); dpi != 0 {
			w.dpi = dpi
		}
	}
	w.manager.SetBackingScale(w.dpiScale())
	if w.options.Scale > 0 && w.options.Scale <= 8 {
		w.manager.SetDisplayScale(w.options.Scale)
	}
	if err := w.sizeClient(hwnd, width, height); err != nil {
		call(win.destroyWindow, hwnd)
		return err
	}
	w.manager.SetWakeHandler(func() {
		if h := w.hwnd.Load(); h != 0 {
			call(win.postMessage, h, wmWake, 0, 0)
		}
	})
	w.manager.SetClipboardHandlers(w.ClipboardText, w.SetClipboardText)
	w.manager.SetInteractive(true)
	call(win.showWindow, hwnd, swShow)
	if w.options.OnReady != nil {
		w.options.OnReady(w)
	}
	w.invalidate()
	return nil
}

func (w *Window) cleanup() {
	w.manager.SetWakeHandler(nil)
	w.manager.SetClipboardHandlers(nil, nil)
	w.manager.SetInteractive(false)
	if w.renderer != nil {
		_ = w.renderer.Close()
	}
	w.framebuffer, w.renderer, w.dib = nil, nil, nil
}

func registerWindowClass() {
	var err error
	className, err = wide("nativeFormsSoftwareWindow")
	if err != nil {
		classError = err
		return
	}
	windowCallback = purego.NewCallback(windowProc)
	class := windowClass{
		Size: uint32(unsafe.Sizeof(windowClass{})), Proc: windowCallback,
		Instance: call(win.getModuleHandle, 0), ClassName: ptr16(className),
	}
	if call(win.registerClass, pointer(&class)) == 0 {
		classError = fmt.Errorf("windows: RegisterClassExW failed")
	}
	keep(className, &class)
}


func (w *Window) paint(hwnd, dc uintptr) error {
	var client rect
	if call(win.getClientRect, hwnd, pointer(&client)) == 0 {
		return fmt.Errorf("windows: GetClientRect failed")
	}
	width, height := int(client.Right-client.Left), int(client.Bottom-client.Top)
	if width <= 0 || height <= 0 {
		return nil
	}
	if width > 16384 || height > 16384 || int64(width)*int64(height) > 64*1024*1024 {
		return fmt.Errorf("windows: framebuffer exceeds 64 megapixels")
	}
	renderScale := w.scale()
	resized := w.framebuffer == nil || w.framebuffer.Bounds().Dx() != width || w.framebuffer.Bounds().Dy() != height
	if resized {
		w.framebuffer = image.NewRGBA(image.Rect(0, 0, width, height))
		w.dib = make([]byte, width*height*4)
		w.dibValid = false
	}
	if w.lastRenderScale != renderScale {
		w.lastRenderScale = renderScale
		w.dibValid = false
	}
	rendered := resized || w.manager.NeedsFrame()
	var trace *ui.FrameTrace
	if rendered {
		logicalWidth := platform.LogicalSize(width, w.scale())
		logicalHeight := platform.LogicalSize(height, w.scale())
		now := time.Now()
		delta := now.Sub(w.lastFrame).Seconds()
		w.lastFrame = now
		w.manager.BeginFrame(logicalWidth, logicalHeight, delta)
		trace = w.manager.FrameTrace()
		rasterStart := trace.Start()
		err := w.renderer.RenderAtScale(w.manager.Frame(), w.framebuffer, w.scale())
		trace.Record(ui.PhaseRaster, rasterStart)
		if err != nil {
			return err
		}
		if w.manager.TextInputFocused() {
			w.placeCandidate(hwnd)
		}
		uploadStart := trace.Start()
		dirty := software.UpdateBGRA(w.dib, w.framebuffer, w.dibValid)
		trace.RecordDamage(dirty.Dx(), dirty.Dy())
		w.dibValid = true
		trace.Record(ui.PhaseUpload, uploadStart)
	}
	info := bitmapInfo{Size: uint32(unsafe.Sizeof(bitmapInfo{})), Width: int32(width), Height: -int32(height),
		Planes: 1, BitCount: 32, Compression: biRGB}
	presentStart := trace.Start()
	result := call(win.stretchDIBits, dc, 0, 0, uintptr(width), uintptr(height), 0, 0,
		uintptr(width), uintptr(height), pointer(&w.dib[0]), pointer(&info), dibRGBColors, srccopy)
	trace.Record(ui.PhasePresent, presentStart)
	runtime.KeepAlive(w.dib)
	runtime.KeepAlive(&info)
	if int32(result) == -1 {
		return fmt.Errorf("windows: StretchDIBits failed")
	}
	if rendered {
		trace.Finish("software/gdi", w.fallbackReason)
		w.armFrameTimer(hwnd)
	}
	return nil
}

func (w *Window) dpiScale() float32 {
	scale := float32(w.dpi) / 96
	if !(scale > 0 && scale <= 8) {
		return 1
	}
	return scale
}

func (w *Window) uiScale() float32 {
	if w != nil && w.manager != nil {
		if s := w.manager.DisplayScale(); s > 0 && s <= 8 {
			return s
		}
	}
	return 1
}

func (w *Window) scale() float32 {
	return min(w.dpiScale()*w.uiScale(), 8)
}

func (w *Window) sizeClient(hwnd uintptr, width, height int) error {
	scale := w.scale()
	pw, ph := platform.PixelSize(width, scale), platform.PixelSize(height, scale)
	if pw < 1 || ph < 1 || pw > 16384 || ph > 16384 || int64(pw)*int64(ph) > 64*1024*1024 {
		return fmt.Errorf("windows: requested client size exceeds framebuffer limit")
	}
	r := rect{Right: int32(pw), Bottom: int32(ph)}
	style := w.windowStyle()
	var ok uintptr
	if win.adjustWindowRectForDpi != 0 {
		ok = call(win.adjustWindowRectForDpi, pointer(&r), style, 0, 0, uintptr(w.dpi))
	} else {
		ok = call(win.adjustWindowRect, pointer(&r), style, 0, 0)
	}
	if ok == 0 || call(win.setWindowPos, hwnd, 0, 0, 0, uintptr(r.Right-r.Left), uintptr(r.Bottom-r.Top), swpNoMove|swpNoZOrder|swpNoActivate) == 0 {
		return fmt.Errorf("windows: could not size client area")
	}
	return nil
}

func (w *Window) armFrameTimer(hwnd uintptr) {
	call(win.killTimer, hwnd, 1)
	if next, scheduled := w.manager.NextFrameDeadline(); scheduled {
		delay := max(time.Duration(0), time.Until(next))
		ms := uint64(delay / time.Millisecond)
		if delay%time.Millisecond != 0 {
			ms++
		}
		if ms > 0xffffffff {
			ms = 0xffffffff
		}
		call(win.setTimer, hwnd, 1, uintptr(max(uint64(1), ms)), 0)
	}
}

func (w *Window) invalidate() {
	if h := w.hwnd.Load(); h != 0 {
		call(win.invalidateRect, h, 0, 0)
	}
}

func (w *Window) Close() {
	if w != nil {
		if h := w.hwnd.Load(); h != 0 {
			call(win.postMessage, h, wmAppDestroy, 0, 0)
		}
	}
}

func (w *Window) closeAction() platform.CloseAction {
	if w.options.OnCloseRequest != nil {
		return w.options.OnCloseRequest(w)
	}
	return platform.CloseWindow
}

// Show and Hide may be called from any goroutine while the App is running.
func (w *Window) Show() { w.setVisible(true) }
func (w *Window) Hide() { w.setVisible(false) }
func (w *Window) setVisible(visible bool) {
	if w == nil || w.app == nil {
		return
	}
	w.app.Post(func() {
		if h := w.hwnd.Load(); h != 0 {
			mode := uintptr(swHide)
			if visible {
				mode = swShow
			}
			call(win.showWindow, h, mode)
			if visible {
				w.invalidate()
			}
		}
	})
}

