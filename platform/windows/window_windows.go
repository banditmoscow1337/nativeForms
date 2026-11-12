//go:build windows && (amd64 || arm64)

package windows

import (
	"fmt"
	"image"
	"runtime"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"

	ui "github.com/banditmoscow1337/nativeForms"
	"github.com/banditmoscow1337/nativeForms/platform"
	"github.com/banditmoscow1337/nativeForms/render"
	"github.com/banditmoscow1337/nativeForms/software"
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
	hidden          atomic.Bool
	wakePending     atomic.Bool
	dpi             uint32
	renderer        render.Backend
	fallbackReason  string
	framebuffer     *image.RGBA
	fbPix           []byte
	fbWidth         int
	fbHeight        int
	dib             []byte
	dibFallback     []byte
	dibValid        bool
	memDC           uintptr
	dibBitmap       uintptr
	oldBitmap       uintptr
	dibBitsPtr      uintptr
	lastRenderScale float32
	lastFrame       time.Time
	lastCaretRect   ui.Rect
	lastCaretScale  float32
	highSurrogate   uint16
	suppressSysChar bool
	captureButton   ui.MouseButton
	hasCapture      bool
	painting        bool
	inEvent         bool
	err             error

	// Heap-allocated Win32 scratch structs so goroutine stack growth never
	// invalidates pointers passed across syscalls.
	ps           paintStruct
	clientRect   rect
	scratchRect  rect
	scratchPoint point
	monInfo      monitorInfo
	bmi          bitmapInfo
	trackEvent   trackMouseEvent
	candForm     candidateForm
}

func New(manager *ui.Manager, options Options) *Window {
	return &Window{manager: manager, options: options}
}

func (w *Window) windowStyle() uintptr {
	if w.options.Frameless {
		return wsPopup | wsSysMenu | wsMinimizeBox | wsMaximizeBox | wsClipChildren | wsClipSiblings
	}
	if w.options.HideWindowControls {
		return wsOverlapped | wsCaption | wsThickFrame | wsClipChildren | wsClipSiblings
	}
	return wsOverlappedWindow | wsClipChildren | wsClipSiblings
}

func (w *Window) adjustRect(r *rect, style uintptr) bool {
	if r == nil {
		return false
	}
	if style&wsPopup != 0 {
		return true
	}
	if win.adjustWindowRectForDpi != 0 {
		if call(win.adjustWindowRectForDpi, pointer(r), style, 0, 0, uintptr(w.dpi)) != 0 {
			keep(w)
			return true
		}
	}
	ok := call(win.adjustWindowRect, pointer(r), style, 0, 0) != 0
	keep(w)
	return ok
}

func (w *Window) monitorWorkRect(hwnd uintptr) (rect, bool) {
	if win.monitorFromWindow == 0 || win.getMonitorInfo == 0 {
		return rect{}, false
	}
	mon := call(win.monitorFromWindow, hwnd, monitorDefaultToNearest)
	if mon == 0 {
		return rect{}, false
	}
	w.monInfo = monitorInfo{Size: uint32(unsafe.Sizeof(monitorInfo{}))}
	if call(win.getMonitorInfo, mon, pointer(&w.monInfo)) == 0 {
		keep(w)
		return rect{}, false
	}
	keep(w)
	return w.monInfo.Work, true
}

func (w *Window) runOnUI(fn func()) {
	if w == nil || fn == nil || w.app == nil || !w.app.running.Load() {
		return
	}
	if uint32(call(win.getCurrentThreadId)) == w.app.threadID.Load() && !w.painting && !w.inEvent {
		fn()
		return
	}
	w.app.Post(fn)
}

func (w *Window) applyStyle() {
	hwnd := w.hwnd.Load()
	if hwnd == 0 || win.setWindowLong == 0 {
		return
	}
	zoomed := win.isZoomed != 0 && call(win.isZoomed, hwnd) != 0
	iconic := win.isIconic != 0 && call(win.isIconic, hwnd) != 0

	var haveClient bool
	var clientW, clientH int32
	var screenX, screenY int32
	if !zoomed && !iconic && call(win.getClientRect, hwnd, pointer(&w.scratchRect)) != 0 {
		clientW = w.scratchRect.Right - w.scratchRect.Left
		clientH = w.scratchRect.Bottom - w.scratchRect.Top
		w.scratchPoint = point{X: 0, Y: 0}
		if clientW > 0 && clientH > 0 && win.clientToScreen != 0 && call(win.clientToScreen, hwnd, pointer(&w.scratchPoint)) != 0 {
			screenX, screenY = w.scratchPoint.X, w.scratchPoint.Y
			haveClient = true
		}
	}

	var preserved uintptr
	if win.getWindowLong != 0 {
		cur := call(win.getWindowLong, hwnd, gwlStyle)
		preserved = cur & (wsVisible | wsMinimize | wsMaximize | wsDisabled)
	}
	if !w.hidden.Load() {
		preserved |= wsVisible
	}
	baseStyle := w.windowStyle()
	newStyle := baseStyle | preserved
	call(win.setWindowLong, hwnd, gwlStyle, newStyle)

	posFlags := uintptr(swpNoZOrder | swpNoOwnerZOrder | swpNoActivate | swpFrameChanged)
	if !w.hidden.Load() {
		posFlags |= swpShowWindow
	}
	if haveClient {
		w.scratchRect = rect{
			Left:   screenX,
			Top:    screenY,
			Right:  screenX + clientW,
			Bottom: screenY + clientH,
		}
		if !w.options.Frameless {
			w.adjustRect(&w.scratchRect, baseStyle)
		}
		call(win.setWindowPos, hwnd, 0,
			uintptr(w.scratchRect.Left), uintptr(w.scratchRect.Top),
			uintptr(w.scratchRect.Right-w.scratchRect.Left), uintptr(w.scratchRect.Bottom-w.scratchRect.Top),
			posFlags)
	} else {
		call(win.setWindowPos, hwnd, 0, 0, 0, 0, 0, posFlags|swpNoMove|swpNoSize)
		if zoomed && !w.hidden.Load() {
			if w.options.Frameless {
				if work, ok := w.monitorWorkRect(hwnd); ok {
					call(win.setWindowPos, hwnd, 0,
						uintptr(work.Left), uintptr(work.Top),
						uintptr(work.Right-work.Left), uintptr(work.Bottom-work.Top),
						posFlags)
				}
			} else {
				call(win.showWindow, hwnd, swMaximize)
			}
		}
	}
	if win.redrawWindow != 0 {
		call(win.redrawWindow, hwnd, 0, 0, rdwFrame|rdwInvalidate|rdwErase|rdwAllChildren)
	}
	w.dibValid = false
	w.manager.InvalidateLayout()
	w.invalidate()
	keep(w)
}

func (w *Window) SetFrameless(frameless bool) {
	if w == nil {
		return
	}
	w.options.Frameless = frameless
	w.runOnUI(w.applyStyle)
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
	w.runOnUI(w.applyStyle)
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
	w.runOnUI(func() {
		if h := w.hwnd.Load(); h != 0 {
			call(win.showWindow, h, swMinimize)
		}
	})
}

func (w *Window) Maximize() {
	if w == nil || w.app == nil {
		return
	}
	w.runOnUI(func() {
		if h := w.hwnd.Load(); h != 0 {
			mode := uintptr(swMaximize)
			if win.isZoomed != 0 && call(win.isZoomed, h) != 0 {
				mode = swRestore
			}
			call(win.showWindow, h, mode)
			w.dibValid = false
			w.manager.InvalidateLayout()
			w.invalidate()
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
var wndClass windowClass

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
	initX, initY := uintptr(cwUseDefault), uintptr(cwUseDefault)
	if w.options.Frameless {
		initX, initY = 100, 100
		if win.getSystemMetrics != 0 {
			sw := int(call(win.getSystemMetrics, smCxScreen))
			sh := int(call(win.getSystemMetrics, smCyScreen))
			if sw > width {
				initX = uintptr((sw - width) / 2)
			}
			if sh > height {
				initY = uintptr((sh - height) / 2)
			}
		}
	}
	creationMu.Lock()
	creatingWindow.Store(w)
	hwnd := call(win.createWindow, 0, ptr16(className), ptr16(name), style,
		initX, initY, uintptr(width), uintptr(height), 0, 0,
		call(win.getModuleHandle, 0), 0)
	creatingWindow.Store(nil)
	if hwnd != 0 {
		w.hwnd.Store(hwnd)
		windowsMu.Lock()
		windowsByHandle[hwnd] = w
		windowsMu.Unlock()
	}
	creationMu.Unlock()
	keep(name, className, w)
	if hwnd == 0 {
		return fmt.Errorf("windows: CreateWindowExW failed")
	}
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
		if h := w.hwnd.Load(); h != 0 && w.wakePending.CompareAndSwap(false, true) {
			call(win.postMessage, h, wmWake, 0, 0)
		}
	})
	w.manager.SetClipboardHandlers(w.ClipboardText, w.SetClipboardText)
	w.manager.SetInteractive(true)
	if !w.hidden.Load() {
		call(win.showWindow, hwnd, swShow)
	}
	if w.options.OnReady != nil {
		w.options.OnReady(w)
	}
	w.invalidate()
	return nil
}

func (w *Window) releaseDIB() {
	w.dib = nil
	if w.memDC != 0 && w.oldBitmap != 0 {
		call(win.selectObject, w.memDC, w.oldBitmap)
		w.oldBitmap = 0
	}
	if w.dibBitmap != 0 {
		call(win.deleteObject, w.dibBitmap)
		w.dibBitmap = 0
	}
	if w.memDC != 0 {
		call(win.deleteDC, w.memDC)
		w.memDC = 0
	}
	w.dibBitsPtr = 0
}

func (w *Window) ensureBuffers(dc uintptr, width, height int) bool {
	if w.fbWidth == width && w.fbHeight == height && w.framebuffer != nil && len(w.dib) == width*height*4 {
		return false
	}
	w.releaseDIB()
	w.framebuffer = nil
	needed := width * height * 4
	if cap(w.fbPix) < needed {
		w.fbPix = nil
		w.fbPix = make([]byte, needed)
	} else {
		w.fbPix = w.fbPix[:needed]
	}
	w.framebuffer = &image.RGBA{
		Pix:    w.fbPix,
		Stride: width * 4,
		Rect:   image.Rect(0, 0, width, height),
	}
	w.bmi = bitmapInfo{
		Size:        bitmapInfoHeaderSize,
		Width:       int32(width),
		Height:      -int32(height),
		Planes:      1,
		BitCount:    32,
		Compression: biRGB,
	}
	if win.createCompatibleDC != 0 && win.createDIBSection != 0 && win.selectObject != 0 && win.bitBlt != 0 {
		memDC := call(win.createCompatibleDC, dc)
		if memDC != 0 {
			w.dibBitsPtr = 0
			hbmp := call(win.createDIBSection, memDC, pointer(&w.bmi), dibRGBColors, pointer(&w.dibBitsPtr), 0, 0)
			if hbmp != 0 && w.dibBitsPtr != 0 {
				w.memDC = memDC
				w.dibBitmap = hbmp
				w.oldBitmap = call(win.selectObject, memDC, hbmp)
				w.dib = unsafe.Slice((*byte)(unsafe.Add(nil, w.dibBitsPtr)), needed)
			} else {
				if hbmp != 0 {
					call(win.deleteObject, hbmp)
				}
				call(win.deleteDC, memDC)
			}
		}
	}
	if w.dib == nil {
		if cap(w.dibFallback) < needed {
			w.dibFallback = nil
			w.dibFallback = make([]byte, needed)
		} else {
			w.dibFallback = w.dibFallback[:needed]
		}
		w.dib = w.dibFallback
	}
	w.fbWidth, w.fbHeight = width, height
	w.dibValid = false
	keep(w)
	return true
}

func (w *Window) cleanup() {
	w.manager.SetWakeHandler(nil)
	w.manager.SetClipboardHandlers(nil, nil)
	w.manager.SetInteractive(false)
	if w.renderer != nil {
		_ = w.renderer.Close()
	}
	w.releaseDIB()
	w.framebuffer, w.fbPix, w.dibFallback, w.renderer = nil, nil, nil, nil
	w.fbWidth, w.fbHeight = 0, 0
}

func registerWindowClass() {
	var err error
	className, err = wide("nativeFormsSoftwareWindow")
	if err != nil {
		classError = err
		return
	}
	windowCallback = syscall.NewCallback(windowProc)
	wndClass = windowClass{
		Size:      uint32(unsafe.Sizeof(windowClass{})),
		Style:     csHRedraw | csVRedraw | csDblClks,
		Proc:      windowCallback,
		Instance:  call(win.getModuleHandle, 0),
		Cursor:    call(win.loadCursor, 0, idcArrow),
		ClassName: ptr16(className),
	}
	if call(win.registerClass, pointer(&wndClass)) == 0 {
		classError = fmt.Errorf("windows: RegisterClassExW failed")
	}
	keep(className, &wndClass)
}

func (w *Window) paint(hwnd, dc uintptr) error {
	if w.painting {
		return nil
	}
	w.painting = true
	defer func() { w.painting = false }()

	if call(win.getClientRect, hwnd, pointer(&w.clientRect)) == 0 {
		keep(w)
		return nil
	}
	width, height := int(w.clientRect.Right-w.clientRect.Left), int(w.clientRect.Bottom-w.clientRect.Top)
	if width <= 0 || height <= 0 {
		return nil
	}
	if width > 16384 || height > 16384 || int64(width)*int64(height) > 64*1024*1024 {
		return fmt.Errorf("windows: framebuffer exceeds 64 megapixels")
	}
	w.manager.ProcessPending()
	renderScale := w.scale()
	resized := w.ensureBuffers(dc, width, height)
	if w.lastRenderScale != renderScale {
		w.lastRenderScale = renderScale
		w.dibValid = false
	}
	rendered := resized || !w.dibValid || w.manager.NeedsFrame()
	var trace *ui.FrameTrace
	if rendered {
		logicalWidth := platform.LogicalSize(width, renderScale)
		logicalHeight := platform.LogicalSize(height, renderScale)
		now := time.Now()
		delta := now.Sub(w.lastFrame).Seconds()
		w.lastFrame = now
		w.manager.BeginFrame(logicalWidth, logicalHeight, delta)
		trace = w.manager.FrameTrace()
		rasterStart := trace.Start()
		err := w.renderer.RenderAtScale(w.manager.Frame(), w.framebuffer, renderScale)
		trace.Record(ui.PhaseRaster, rasterStart)
		if err != nil {
			return err
		}
		uploadStart := trace.Start()
		dirty := software.UpdateBGRA(w.dib, w.framebuffer, w.dibValid)
		trace.RecordDamage(dirty.Dx(), dirty.Dy())
		w.dibValid = true
		trace.Record(ui.PhaseUpload, uploadStart)
	}
	presentStart := trace.Start()
	if w.memDC != 0 && w.dibBitmap != 0 {
		if win.gdiFlush != 0 {
			call(win.gdiFlush)
		}
		call(win.bitBlt, dc, 0, 0, uintptr(width), uintptr(height), w.memDC, 0, 0, srccopy)
	} else if len(w.dib) >= width*height*4 {
		w.bmi = bitmapInfo{
			Size:        bitmapInfoHeaderSize,
			Width:       int32(width),
			Height:      -int32(height),
			Planes:      1,
			BitCount:    32,
			Compression: biRGB,
		}
		call(win.stretchDIBits, dc, 0, 0, uintptr(width), uintptr(height), 0, 0,
			uintptr(width), uintptr(height), pointer(&w.dib[0]), pointer(&w.bmi), dibRGBColors, srccopy)
	}
	trace.Record(ui.PhasePresent, presentStart)
	keep(w)
	if rendered {
		trace.Finish("software/gdi", w.fallbackReason)
		w.armFrameTimer(hwnd)
	}
	return nil
}

func (w *Window) syncCandidateIfNeeded(hwnd uintptr) {
	if !w.manager.TextInputFocused() {
		return
	}
	caret, ok := w.manager.FocusedCaretRect()
	if !ok {
		return
	}
	scale := w.scale()
	if caret != w.lastCaretRect || scale != w.lastCaretScale {
		w.lastCaretRect = caret
		w.lastCaretScale = scale
		w.placeCandidate(hwnd)
	}
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
	w.scratchRect = rect{Right: int32(pw), Bottom: int32(ph)}
	style := w.windowStyle()
	if !w.options.Frameless && !w.adjustRect(&w.scratchRect, style) {
		return fmt.Errorf("windows: could not size client area")
	}
	wDiff := uintptr(w.scratchRect.Right - w.scratchRect.Left)
	hDiff := uintptr(w.scratchRect.Bottom - w.scratchRect.Top)
	if call(win.setWindowPos, hwnd, 0, 0, 0, wDiff, hDiff, swpNoMove|swpNoZOrder|swpNoOwnerZOrder|swpNoActivate) == 0 {
		keep(w)
		return fmt.Errorf("windows: could not size client area")
	}
	keep(w)
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
	if h := w.hwnd.Load(); h != 0 && !w.hidden.Load() {
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
	if w == nil {
		return
	}
	w.hidden.Store(!visible)
	w.runOnUI(func() {
		if h := w.hwnd.Load(); h != 0 {
			if visible {
				call(win.showWindow, h, swShow)
				w.dibValid = false
				w.manager.InvalidatePaint()
				w.invalidate()
			} else {
				w.manager.CancelPointerCapture()
				w.manager.ClearInteraction()
				call(win.showWindow, h, swHide)
			}
		}
	})
}
