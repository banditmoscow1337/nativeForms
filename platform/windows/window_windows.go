//go:build windows && (amd64 || arm64)

package windows

import (
	"fmt"
	"image"
	"sync"
	"sync/atomic"
	"time"
	"unicode"
	"unicode/utf16"
	"unsafe"

	ui "github.com/banditmoscow1337/nativeForms"
	"github.com/banditmoscow1337/nativeForms/platform"
	"github.com/banditmoscow1337/nativeForms/render"
	"github.com/banditmoscow1337/nativeForms/software"
	"github.com/ebitengine/purego"
)

type Options struct {
	Title          string
	Width, Height  int
	OnReady        func(*Window)
	OnFilesDropped func([]string)
	OnCloseRequest func(*Window) platform.CloseAction
	Graphics       render.Kind
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
	creationMu.Lock()
	creatingWindow.Store(w)
	hwnd := call(win.createWindow, 0, ptr16(className), ptr16(name), wsOverlappedWindow,
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
	w.manager.SetDisplayScale(w.scale())
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

func windowProc(hwnd uintptr, msg uint32, wp, lp uintptr) (result uintptr) {
	windowsMu.RLock()
	w := windowsByHandle[hwnd]
	windowsMu.RUnlock()
	if w == nil {
		w = creatingWindow.Load()
	}
	if w == nil || (w.hwnd.Load() != 0 && w.hwnd.Load() != hwnd) {
		return call(win.defProc, hwnd, uintptr(msg), wp, lp)
	}
	defer func() {
		if value := recover(); value != nil {
			w.err = fmt.Errorf("windows: UI callback: %v", value)
			call(win.postMessage, hwnd, wmAppDestroy, 0, 0)
			result = 0
		}
	}()
	return w.handle(hwnd, msg, wp, lp)
}

func (w *Window) handle(hwnd uintptr, msg uint32, wp, lp uintptr) uintptr {
	switch msg {
	case wmPaint:
		var ps paintStruct
		dc := call(win.beginPaint, hwnd, pointer(&ps))
		if dc != 0 {
			if err := w.paint(hwnd, dc); err != nil {
				w.err = err
				call(win.postMessage, hwnd, wmAppDestroy, 0, 0)
			}
			call(win.endPaint, hwnd, pointer(&ps))
		}
		return 0
	case wmEraseBackground:
		return 1 // the complete DIB is presented by WM_PAINT
	case wmSize:
		w.manager.InvalidateLayout()
		w.invalidate()
		return 0
	case wmDpiChanged:
		w.dpi = uint32(wp & 0xffff)
		if w.dpi == 0 {
			w.dpi = 96
		}
		w.manager.SetDisplayScale(w.scale())
		r := (*rect)(unsafe.Pointer(lp))
		if r != nil {
			call(win.setWindowPos, hwnd, 0, uintptr(r.Left), uintptr(r.Top),
				uintptr(r.Right-r.Left), uintptr(r.Bottom-r.Top), swpNoZOrder|swpNoActivate)
		}
		w.manager.InvalidateLayout()
		w.invalidate()
		return 0
	case wmWake:
		w.manager.ProcessPending()
		if w.manager.NeedsFrame() {
			w.invalidate()
		}
		return 0
	case wmTimer:
		call(win.killTimer, hwnd, 1)
		if w.manager.NeedsFrame() {
			w.invalidate()
		} else {
			w.armFrameTimer(hwnd)
		}
		return 0
	case wmMouseMove, wmLeftDown, wmLeftUp, wmRightDown, wmRightUp, wmMiddleDown, wmMiddleUp, wmMouseWheel, wmMouseHWheel:
		w.mouse(hwnd, msg, wp, lp)
		return 0
	case wmMouseLeave:
		w.manager.HandleEvent(ui.Event{Type: ui.PointerMove, X: -1e6, Y: -1e6})
		return 0
	case wmCaptureChanged:
		w.hasCapture = false
		w.manager.CancelPointerCapture()
		return 0
	case wmDropFiles:
		w.filesDropped(wp)
		return 0
	case wmKeyDown, wmKeyUp, wmSysKeyDown, wmSysKeyUp:
		key := translateKey(uint32(wp))
		down := msg == wmKeyDown || msg == wmSysKeyDown
		kind := ui.KeyUp
		if down {
			kind = ui.KeyDown
		}
		handled := w.manager.HandleEvent(ui.Event{Type: kind, Key: key,
			Repeat: down && (lp&(1<<30)) != 0, Mods: w.mods()})
		if msg == wmSysKeyDown {
			w.suppressSysChar = handled
		}
		if (msg == wmSysKeyDown || msg == wmSysKeyUp) && !handled {
			return call(win.defProc, hwnd, uintptr(msg), wp, lp)
		}
		return 0
	case wmSysChar:
		if w.suppressSysChar {
			w.suppressSysChar = false
			return 0
		}
	case wmChar:
		w.character(uint16(wp))
		return 0
	case wmIMEStart:
		if !w.manager.TextInputFocused() {
			break
		}
		w.manager.HandleEvent(ui.Event{Type: ui.CompositionStart})
		w.placeCandidate(hwnd)
		return 0
	case wmIMEComposition:
		if !w.manager.TextInputFocused() {
			break
		}
		w.composition(hwnd, lp)
		return 0
	case wmIMEEnd:
		if !w.manager.TextInputFocused() {
			break
		}
		w.manager.HandleEvent(ui.Event{Type: ui.CompositionEnd})
		return 0
	case wmKillFocus:
		w.highSurrogate = 0
		w.manager.ClearInteraction()
		return 0
	case wmSetCursor:
		if lp&0xffff == 1 { // HTCLIENT
			id := uintptr(32512) // IDC_ARROW
			if w.manager.HoveredTextInput() {
				id = 32513
			} // IDC_IBEAM
			call(win.setCursor, call(win.loadCursor, 0, id))
			return 1
		}
	case wmClose:
		switch w.closeAction() {
		case platform.CancelClose:
			return 0
		case platform.HideWindow:
			w.Hide()
			return 0
		case platform.QuitApp:
			w.app.Quit()
			return 0
		}
		fallthrough
	case wmAppDestroy:
		call(win.destroyWindow, hwnd)
		return 0
	case wmDestroy:
		call(win.dragAcceptFiles, hwnd, 0)
		w.hwnd.Store(0)
		call(win.killTimer, hwnd, 1)
		windowsMu.Lock()
		delete(windowsByHandle, hwnd)
		windowsMu.Unlock()
		w.cleanup()
		if w.app != nil {
			w.app.closed(w)
		}
		return 0
	}
	return call(win.defProc, hwnd, uintptr(msg), wp, lp)
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
	resized := w.framebuffer == nil || w.framebuffer.Bounds().Dx() != width || w.framebuffer.Bounds().Dy() != height
	if resized {
		w.framebuffer = image.NewRGBA(image.Rect(0, 0, width, height))
		w.dib = make([]byte, width*height*4)
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
		w.placeCandidate(hwnd)
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
	keep(w.dib, &info)
	if int32(result) == -1 {
		return fmt.Errorf("windows: StretchDIBits failed")
	}
	if rendered {
		trace.Finish("software/gdi", w.fallbackReason)
		w.armFrameTimer(hwnd)
	}
	return nil
}

func (w *Window) scale() float32 {
	scale := float32(w.dpi) / 96
	if !(scale > 0 && scale <= 8) {
		return 1
	}
	return scale
}

func (w *Window) sizeClient(hwnd uintptr, width, height int) error {
	scale := w.scale()
	pw, ph := platform.PixelSize(width, scale), platform.PixelSize(height, scale)
	if pw < 1 || ph < 1 || pw > 16384 || ph > 16384 || int64(pw)*int64(ph) > 64*1024*1024 {
		return fmt.Errorf("windows: requested client size exceeds framebuffer limit")
	}
	r := rect{Right: int32(pw), Bottom: int32(ph)}
	var ok uintptr
	if win.adjustWindowRectForDpi != 0 {
		ok = call(win.adjustWindowRectForDpi, pointer(&r), wsOverlappedWindow, 0, 0, uintptr(w.dpi))
	} else {
		ok = call(win.adjustWindowRect, pointer(&r), wsOverlappedWindow, 0, 0)
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

func (w *Window) mouse(hwnd uintptr, msg uint32, wp, lp uintptr) {
	if msg == wmMouseMove {
		tracking := trackMouseEvent{Size: uint32(unsafe.Sizeof(trackMouseEvent{})), Flags: 2, HWND: hwnd}
		call(win.trackMouse, pointer(&tracking))
	}
	p := point{X: int32(int16(lp & 0xffff)), Y: int32(int16((lp >> 16) & 0xffff))}
	if msg == wmMouseWheel || msg == wmMouseHWheel {
		call(win.screenToClient, hwnd, pointer(&p))
	}
	scale := float32(96) / float32(w.dpi)
	event := ui.Event{X: float32(p.X) * scale, Y: float32(p.Y) * scale, Mods: w.mods()}
	switch msg {
	case wmMouseMove:
		event.Type = ui.PointerMove
	case wmMouseWheel:
		event.Type = ui.PointerScroll
		event.Scroll.Y = float32(int16((wp>>16)&0xffff)) / 120
		if event.Mods&ui.ModShift != 0 {
			event.Scroll.X = -event.Scroll.Y
			event.Scroll.Y = 0
		}
	case wmMouseHWheel:
		event.Type = ui.PointerScroll
		event.Scroll.X = -float32(int16((wp>>16)&0xffff)) / 120
	case wmLeftDown, wmRightDown, wmMiddleDown:
		event.Type = ui.PointerDown
		event.Button = button(msg)
		if !w.hasCapture {
			call(win.setCapture, hwnd)
			w.captureButton = event.Button
			w.hasCapture = true
		}
	case wmLeftUp, wmRightUp, wmMiddleUp:
		event.Type = ui.PointerUp
		event.Button = button(msg)
	}
	w.manager.HandleEvent(event)
	if event.Type == ui.PointerUp && w.hasCapture && event.Button == w.captureButton {
		w.hasCapture = false
		call(win.releaseCapture)
	}
}

func button(msg uint32) ui.MouseButton {
	switch msg {
	case wmRightDown, wmRightUp:
		return ui.MouseRight
	case wmMiddleDown, wmMiddleUp:
		return ui.MouseMiddle
	default:
		return ui.MouseLeft
	}
}

func (w *Window) mods() int {
	mods := 0
	if call(win.getKeyState, 0x10)&0x8000 != 0 {
		mods |= ui.ModShift
	}
	if call(win.getKeyState, 0x11)&0x8000 != 0 {
		mods |= ui.ModControl
	}
	if call(win.getKeyState, 0x12)&0x8000 != 0 {
		mods |= ui.ModAlt
	}
	if call(win.getKeyState, 0x5B)&0x8000 != 0 || call(win.getKeyState, 0x5C)&0x8000 != 0 {
		mods |= ui.ModSuper
	}
	return mods
}

func translateKey(key uint32) ui.Key {
	switch key {
	case 0x20:
		return ui.KeySpace
	case 0x0D:
		return ui.KeyEnter
	case 0x1B:
		return ui.KeyEscape
	case 0x09:
		return ui.KeyTab
	case 0x26:
		return ui.KeyUpArrow
	case 0x28:
		return ui.KeyDownArrow
	case 0x25:
		return ui.KeyLeftArrow
	case 0x27:
		return ui.KeyRightArrow
	case 0x11:
		return ui.KeyControl
	case 0x08:
		return ui.KeyBackspace
	case 0x2E:
		return ui.KeyDelete
	case 0x24:
		return ui.KeyHome
	case 0x23:
		return ui.KeyEnd
	case 0x21:
		return ui.KeyPageUp
	case 0x22:
		return ui.KeyPageDown
	case 0x2D:
		return ui.KeyInsert
	case 0x70:
		return ui.KeyF1
	case 0x71:
		return ui.KeyF2
	case 0x72:
		return ui.KeyF3
	case 0x73:
		return ui.KeyF4
	case 0x74:
		return ui.KeyF5
	case 0x75:
		return ui.KeyF6
	case 0x76:
		return ui.KeyF7
	case 0x77:
		return ui.KeyF8
	case 0x78:
		return ui.KeyF9
	case 0x79:
		return ui.KeyF10
	case 0x7A:
		return ui.KeyF11
	case 0x7B:
		return ui.KeyF12
	case '0':
		return ui.KeyDigit0
	case '1':
		return ui.KeyDigit1
	case '2':
		return ui.KeyDigit2
	case '3':
		return ui.KeyDigit3
	case '4':
		return ui.KeyDigit4
	case '5':
		return ui.KeyDigit5
	case '6':
		return ui.KeyDigit6
	case '7':
		return ui.KeyDigit7
	case '8':
		return ui.KeyDigit8
	case '9':
		return ui.KeyDigit9
	case 0xBD:
		return ui.KeyMinus
	case 0xBB:
		return ui.KeyEqual
	case 0xDB:
		return ui.KeyLeftBracket
	case 0xDD:
		return ui.KeyRightBracket
	case 0xDC:
		return ui.KeyBackslash
	case 0xBA:
		return ui.KeySemicolon
	case 0xDE:
		return ui.KeyQuote
	case 0xC0:
		return ui.KeyBackquote
	case 0xBC:
		return ui.KeyComma
	case 0xBE:
		return ui.KeyPeriod
	case 0xBF:
		return ui.KeySlash
	case 'W':
		return ui.KeyW
	case 'A':
		return ui.KeyA
	case 'S':
		return ui.KeyS
	case 'D':
		return ui.KeyD
	case 'Q':
		return ui.KeyQ
	case 'E':
		return ui.KeyE
	case 'R':
		return ui.KeyR
	case 'F':
		return ui.KeyF
	case 'Z':
		return ui.KeyZ
	case 'Y':
		return ui.KeyY
	case 'C':
		return ui.KeyC
	case 'V':
		return ui.KeyV
	case 'X':
		return ui.KeyX
	case 'B':
		return ui.KeyB
	case 'G':
		return ui.KeyG
	case 'H':
		return ui.KeyH
	case 'I':
		return ui.KeyI
	case 'J':
		return ui.KeyJ
	case 'K':
		return ui.KeyK
	case 'L':
		return ui.KeyL
	case 'M':
		return ui.KeyM
	case 'N':
		return ui.KeyN
	case 'O':
		return ui.KeyO
	case 'P':
		return ui.KeyP
	case 'T':
		return ui.KeyT
	case 'U':
		return ui.KeyU
	default:
		return ui.KeyUnknown
	}
}

func (w *Window) character(code uint16) {
	if code >= 0xd800 && code <= 0xdbff {
		w.highSurrogate = code
		return
	}
	var r rune
	if code >= 0xdc00 && code <= 0xdfff && w.highSurrogate != 0 {
		r = utf16.DecodeRune(rune(w.highSurrogate), rune(code))
	} else {
		r = rune(code)
	}
	w.highSurrogate = 0
	if r >= 32 && r != 0x7f && r != unicode.ReplacementChar {
		w.manager.HandleEvent(ui.Event{Type: ui.TextInput, Rune: r, Mods: w.mods()})
	}
}
