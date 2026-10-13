//go:build darwin && (amd64 || arm64)

package darwin

import (
	"fmt"
	"image"
	"math"
	"runtime"
	"structs"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	ui "github.com/banditmoscow1337/nativeForms"
	"github.com/banditmoscow1337/nativeForms/platform"
	"github.com/banditmoscow1337/nativeForms/software"
	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"
)

type point struct {
	_    structs.HostLayout
	X, Y float64
}
type size struct {
	_    structs.HostLayout
	W, H float64
}
type rect struct {
	_      structs.HostLayout
	Origin point
	Size   size
}

func makeRect(w, h float64) rect { return rect{Size: size{W: w, H: h}} }

type Options struct {
	Title          string
	Width, Height  int
	OnReady        func(*Window)
	OnFilesDropped func([]string)
}

type Window struct {
	manager                           *ui.Manager
	options                           Options
	app, window, view, representation objc.ID
	renderer                          *software.Renderer
	width, height                     int
	pixelWidth, pixelHeight           int
	lastFrame                         time.Time
	closing                           atomic.Bool
	marked                            string
	markedSelection                   nsRange
	inputIdentity                     *ui.State
	currentKey                        ui.Event
	inputHandled                      bool
	wasKey                            bool
}

var _ platform.FileDialogs = (*Window)(nil)

func New(manager *ui.Manager, options Options) *Window {
	return &Window{manager: manager, options: options}
}

// SetSize changes the logical client size. Call it on the AppKit UI goroutine
// after Run starts (for example from OnReady or Manager.Post).
func (w *Window) SetSize(width, height int) error {
	if w == nil || w.manager == nil {
		return fmt.Errorf("darwin: nil window or manager")
	}
	if err := platform.ValidateSize(width, height, w.manager.DisplayScale()); err != nil {
		return err
	}
	if w.window != 0 {
		w.window.Send(selector("setContentSize:"), size{W: float64(width), H: float64(height)})
		w.manager.InvalidateLayout()
	}
	w.options.Width, w.options.Height = width, height
	return nil
}

func Capabilities() platform.Capabilities {
	return platform.Capabilities{SoftwareFrame: true, Pointer: true, Keyboard: true, TextInput: true, Clipboard: true, FileDialog: true, IME: true, MultipleWindows: true, DragDrop: true}
}

func (w *Window) Capabilities() platform.Capabilities { return Capabilities() }

var appMu sync.Mutex

func selector(name string) objc.SEL { return objc.RegisterName(name) }
func class(name string) objc.ID     { return objc.ID(objc.GetClass(name)) }
func nsString(value string) objc.ID {
	return class("NSString").Send(selector("stringWithUTF8String:"), value)
}

func (w *Window) Run() error { return Run(w) }

type application struct {
	app     objc.ID
	windows map[objc.ID]*Window
}

var activeApplication *application
var viewWindows = make(map[objc.ID]*Window)
var cocoaOnce sync.Once
var cocoaErr error
var onMainThread func() int32

func loadAppKit() error {
	cocoaOnce.Do(func() {
		var lib uintptr
		lib, cocoaErr = purego.Dlopen("/System/Library/Frameworks/Cocoa.framework/Cocoa", purego.RTLD_GLOBAL|purego.RTLD_LAZY)
		if cocoaErr != nil {
			return
		}
		purego.RegisterLibFunc(&onMainThread, lib, "pthread_main_np")
		cocoaErr = registerView()
	})
	return cocoaErr
}

func Run(windows ...*Window) error {
	if len(windows) == 0 {
		return fmt.Errorf("darwin: no windows")
	}
	if !appMu.TryLock() {
		return fmt.Errorf("darwin: application is already running; use Window.Open on its UI goroutine")
	}
	defer appMu.Unlock()
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if err := loadAppKit(); err != nil {
		return err
	}
	if onMainThread() == 0 {
		return fmt.Errorf("darwin: Run must execute on the main OS thread; call runtime.LockOSThread in main's init")
	}
	if err := EnableCoreText(); err != nil {
		return err
	}
	pool := class("NSAutoreleasePool").Send(selector("new"))
	defer pool.Send(selector("drain"))
	app := &application{app: class("NSApplication").Send(selector("sharedApplication")), windows: make(map[objc.ID]*Window)}
	app.app.Send(selector("setActivationPolicy:"), 0)
	activeApplication = app
	defer func() {
		for _, w := range app.windows {
			w.destroy()
		}
		activeApplication = nil
	}()
	for _, w := range windows {
		if err := w.Open(); err != nil {
			return err
		}
	}
	app.app.Send(selector("finishLaunching"))
	app.app.Send(selector("activateIgnoringOtherApps:"), true)
	mode := nsString("kCFRunLoopDefaultMode")
	for len(app.windows) > 0 {
		iteration := class("NSAutoreleasePool").Send(selector("new"))
		err := app.step(mode)
		iteration.Send(selector("drain"))
		if err != nil {
			return err
		}
	}
	return nil
}

func (w *Window) Open() error {
	if w == nil || w.manager == nil {
		return fmt.Errorf("darwin: nil window or manager")
	}
	app := activeApplication
	if app == nil || onMainThread() == 0 {
		return fmt.Errorf("darwin: Open needs the running AppKit UI goroutine")
	}
	if w.window != 0 {
		return fmt.Errorf("darwin: window is already open")
	}
	for _, other := range app.windows {
		if other.manager == w.manager {
			return fmt.Errorf("darwin: windows must have distinct managers")
		}
	}
	w.closing.Store(false)
	w.app = app.app
	width, height := w.options.Width, w.options.Height
	if width <= 0 {
		width = 800
	}
	if height <= 0 {
		height = 600
	}
	if width > 16384 || height > 16384 {
		return fmt.Errorf("darwin: window dimensions exceed 16384")
	}
	w.window = class("NSWindow").Send(selector("alloc")).Send(selector("initWithContentRect:styleMask:backing:defer:"), makeRect(float64(width), float64(height)), uint64(1|2|4|8), 2, false)
	if w.window == 0 {
		return fmt.Errorf("darwin: NSWindow initialization failed")
	}
	w.window.Send(selector("setReleasedWhenClosed:"), false)
	w.view = objc.ID(nativeViewClass).Send(selector("alloc")).Send(selector("initWithFrame:"), makeRect(float64(width), float64(height)))
	if w.view == 0 {
		w.destroy()
		return fmt.Errorf("darwin: view initialization failed")
	}
	viewWindows[w.view] = w
	app.windows[w.window] = w
	w.view.Send(selector("setUserInterfaceLayoutDirection:"), uint64(0))
	title := w.options.Title
	if title == "" {
		title = "nativeForms"
	}
	w.window.Send(selector("setTitle:"), nsString(title))
	w.window.Send(selector("setAcceptsMouseMovedEvents:"), true)
	w.view.Send(selector("setAutoresizingMask:"), uint64(2|16))
	w.window.Send(selector("setContentView:"), w.view)
	w.window.Send(selector("center"))
	w.window.Send(selector("makeFirstResponder:"), w.view)
	if w.options.OnFilesDropped != nil {
		types := class("NSMutableArray").Send(selector("array"))
		types.Send(selector("addObject:"), nsString("public.file-url"))
		types.Send(selector("addObject:"), nsString("NSFilenamesPboardType"))
		w.view.Send(selector("registerForDraggedTypes:"), types)
	}
	w.window.Send(selector("makeKeyAndOrderFront:"), objc.ID(0))
	w.renderer = software.New()
	w.lastFrame = time.Now()
	w.manager.SetInteractive(true)
	w.manager.InvalidateLayout()
	w.manager.SetClipboardHandlers(w.ClipboardText, w.SetClipboardText)
	if w.options.OnReady != nil {
		w.options.OnReady(w)
	}
	return nil
}
func (a *application) step(mode objc.ID) error {
	seconds := 0.05
	for _, w := range a.windows {
		if w.closing.Load() || !objc.Send[bool](w.window, selector("isVisible")) {
			w.destroy()
			continue
		}
		w.manager.ProcessPending()
		key := objc.Send[bool](w.window, selector("isKeyWindow"))
		if w.wasKey && !key {
			w.cancelMarked()
			w.manager.CancelPointerCapture()
		}
		w.wasKey = key
		w.syncInputFocus()
		bounds := objc.Send[rect](w.view, selector("bounds"))
		backing := objc.Send[rect](w.view, selector("convertRectToBacking:"), bounds)
		width, height := int(math.Ceil(bounds.Size.W)), int(math.Ceil(bounds.Size.H))
		pixelWidth, pixelHeight := int(math.Round(backing.Size.W)), int(math.Round(backing.Size.H))
		scale := float32(objc.Send[float64](w.window, selector("backingScaleFactor")))
		if !(scale > 0 && scale <= 8) {
			scale = 1
		}
		w.manager.SetDisplayScale(scale)
		if width > 0 && height > 0 && pixelWidth > 0 && pixelHeight > 0 &&
			(w.manager.NeedsFrame() || width != w.width || height != w.height || pixelWidth != w.pixelWidth || pixelHeight != w.pixelHeight) {
			if err := w.paint(width, height, pixelWidth, pixelHeight, scale); err != nil {
				return err
			}
		}
		if next, scheduled := w.manager.NextFrameDeadline(); width > 0 && height > 0 && scheduled {
			if delay := time.Until(next).Seconds(); delay < seconds {
				seconds = max(0.001, delay)
			}
		}
	}
	if len(a.windows) == 0 {
		return nil
	}
	for _, w := range a.windows {
		w.manager.ProcessPending()
		bounds := objc.Send[rect](w.view, selector("bounds"))
		if bounds.Size.W > 0 && bounds.Size.H > 0 && w.manager.NeedsFrame() {
			return nil
		}
	}
	date := class("NSDate").Send(selector("dateWithTimeIntervalSinceNow:"), seconds)
	event := a.app.Send(selector("nextEventMatchingMask:untilDate:inMode:dequeue:"), ^uint64(0), date, mode, true)
	if event == 0 {
		return nil
	}
	typ := objc.Send[uint64](event, selector("type"))
	id := event.Send(selector("window"))
	w := a.windows[id]
	if w != nil {
		if typ == 10 || typ == 11 || typ == 12 {
			w.event(event, w.height)
			return nil
		}
		switch typ {
		case 1, 2, 3, 4, 5, 6, 7, 9, 22, 25, 26, 27:
			p := objc.Send[point](event, selector("locationInWindow"))
			if p.Y >= 0 && p.Y <= float64(w.height) || typ == 2 || typ == 4 || typ == 6 || typ == 7 || typ == 9 || typ == 26 || typ == 27 {
				w.event(event, w.height)
			}
		}

	}
	a.app.Send(selector("sendEvent:"), event)
	return nil
}
func (w *Window) destroy() {
	if w.window == 0 {
		return
	}
	w.cancelMarked()
	w.manager.SetClipboardHandlers(nil, nil)
	w.manager.ClearInteraction()
	w.manager.SetInteractive(false)
	if activeApplication != nil {
		delete(activeApplication.windows, w.window)
	}
	delete(viewWindows, w.view)
	if objc.Send[bool](w.window, selector("isVisible")) {
		w.window.Send(selector("close"))
	}
	if w.representation != 0 {
		w.representation.Send(selector("release"))
		w.representation = 0
	}
	w.window.Send(selector("release"))
	w.window = 0
	if w.view != 0 {
		w.view.Send(selector("release"))
		w.view = 0
	}
	w.inputIdentity = nil
}

func (w *Window) Close() {
	if w != nil {
		w.closing.Store(true)
	}
}

func (w *Window) paint(width, height, pixelWidth, pixelHeight int, scale float32) error {
	if pixelWidth > 16384 || pixelHeight > 16384 || int64(pixelWidth)*int64(pixelHeight) > 64*1024*1024 {
		return fmt.Errorf("darwin: framebuffer exceeds 64 megapixels")
	}
	resized := w.representation == 0 || pixelWidth != w.pixelWidth || pixelHeight != w.pixelHeight
	if resized {
		w.pixelWidth, w.pixelHeight = pixelWidth, pixelHeight
		if w.representation != 0 {
			w.representation.Send(selector("release"))
			w.representation = 0
		}
		w.representation = class("NSBitmapImageRep").Send(selector("alloc"))
		w.representation = w.representation.Send(selector("initWithBitmapDataPlanes:pixelsWide:pixelsHigh:bitsPerSample:samplesPerPixel:hasAlpha:isPlanar:colorSpaceName:bytesPerRow:bitsPerPixel:"),
			uintptr(0), pixelWidth, pixelHeight, 8, 4, true, false, nsString("NSDeviceRGBColorSpace"), pixelWidth*4, 32)
		if w.representation == 0 {
			return fmt.Errorf("darwin: bitmap representation failed")
		}
	}
	if width != w.width || height != w.height {
		w.width, w.height = width, height
		w.manager.InvalidateLayout()
	}
	w.representation.Send(selector("setSize:"), objc.Send[rect](w.view, selector("bounds")).Size)
	now := time.Now()
	w.manager.BeginFrame(width, height, now.Sub(w.lastFrame).Seconds())
	w.lastFrame = now
	trace := w.manager.FrameTrace()
	pointer := objc.Send[uintptr](w.representation, selector("bitmapData"))
	if pointer == 0 {
		return fmt.Errorf("darwin: bitmapData is nil")
	}
	pixels := unsafe.Slice((*byte)(unsafe.Pointer(pointer)), pixelWidth*pixelHeight*4)
	target := &image.RGBA{Pix: pixels, Stride: pixelWidth * 4, Rect: image.Rect(0, 0, pixelWidth, pixelHeight)}
	damage := platform.PixelDamage(w.manager.Frame().Damage, scale, pixelWidth, pixelHeight)
	if resized {
		damage = target.Bounds()
	}
	rasterStart := trace.Start()
	err := w.renderer.RenderAtScaleDamage(w.manager.Frame(), target, scale, damage)
	trace.Record(ui.PhaseRaster, rasterStart)
	if err != nil {
		return err
	}
	presentStart := trace.Start()
	w.view.Send(selector("setNeedsDisplay:"), true)
	trace.Record(ui.PhasePresent, presentStart)
	trace.Finish("software/appkit", "")
	return nil
}

func (w *Window) ClipboardText() (string, error) {
	if w == nil || w.window == 0 {
		return "", fmt.Errorf("darwin: clipboard needs a running window")
	}
	board := class("NSPasteboard").Send(selector("generalPasteboard"))
	value := board.Send(selector("stringForType:"), nsString("public.utf8-plain-text"))
	if value == 0 {
		return "", nil
	}
	if objc.Send[uintptr](value, selector("lengthOfBytesUsingEncoding:"), uint64(4)) > platform.ClipboardTextLimit {
		return "", fmt.Errorf("darwin: clipboard text is too large")
	}
	text := stringValue(value)
	if err := platform.ValidateClipboardText(text); err != nil {
		return "", err
	}
	return text, nil
}

func (w *Window) SetClipboardText(value string) error {
	if w == nil || w.window == 0 {
		return fmt.Errorf("darwin: clipboard needs a running window")
	}
	if err := platform.ValidateClipboardText(value); err != nil {
		return err
	}
	board := class("NSPasteboard").Send(selector("generalPasteboard"))
	board.Send(selector("clearContents"))
	if !objc.Send[bool](board, selector("setString:forType:"), nsString(value), nsString("public.utf8-plain-text")) {
		return fmt.Errorf("darwin: clipboard write failed")
	}
	return nil
}

func (w *Window) OpenFile() (string, error) {
	if w == nil || w.window == 0 {
		return "", fmt.Errorf("darwin: file dialog needs a running window")
	}
	panel := class("NSOpenPanel").Send(selector("openPanel"))
	if panel == 0 {
		return "", fmt.Errorf("darwin: NSOpenPanel unavailable")
	}
	panel.Send(selector("setCanChooseDirectories:"), false)
	panel.Send(selector("setCanChooseFiles:"), true)
	panel.Send(selector("setAllowsMultipleSelection:"), false)
	if objc.Send[int64](panel, selector("runModal")) != 1 {
		return "", nil
	}
	return stringValue(panel.Send(selector("URL")).Send(selector("path"))), nil
}

// OpenFolder chooses a project directory with a native macOS panel.
func (w *Window) OpenFolder() (string, error) {
	if w == nil || w.window == 0 {
		return "", fmt.Errorf("darwin: folder dialog needs a running window")
	}
	panel := class("NSOpenPanel").Send(selector("openPanel"))
	if panel == 0 {
		return "", fmt.Errorf("darwin: NSOpenPanel unavailable")
	}
	panel.Send(selector("setCanChooseDirectories:"), true)
	panel.Send(selector("setCanChooseFiles:"), false)
	panel.Send(selector("setAllowsMultipleSelection:"), false)
	if objc.Send[int64](panel, selector("runModal")) != 1 {
		return "", nil
	}
	return stringValue(panel.Send(selector("URL")).Send(selector("path"))), nil
}

func (w *Window) SaveFile(defaultName string) (string, error) {
	if w == nil || w.window == 0 {
		return "", fmt.Errorf("darwin: file dialog needs a running window")
	}
	panel := class("NSSavePanel").Send(selector("savePanel"))
	if panel == 0 {
		return "", fmt.Errorf("darwin: NSSavePanel unavailable")
	}
	if defaultName != "" {
		panel.Send(selector("setNameFieldStringValue:"), nsString(defaultName))
	}
	if objc.Send[int64](panel, selector("runModal")) != 1 {
		return "", nil
	}
	return stringValue(panel.Send(selector("URL")).Send(selector("path"))), nil
}

func stringValue(value objc.ID) string {
	length := objc.Send[uintptr](value, selector("lengthOfBytesUsingEncoding:"), uint64(4))
	pointer := objc.Send[uintptr](value, selector("UTF8String"))
	if pointer == 0 || length == 0 || length > 16<<20 {
		return ""
	}
	return string(unsafe.Slice((*byte)(unsafe.Pointer(pointer)), length))
}

func (w *Window) event(event objc.ID, height int) {
	typ := objc.Send[uint64](event, selector("type"))
	mods := objc.Send[uint64](event, selector("modifierFlags"))
	flags := 0
	if mods&(1<<17) != 0 {
		flags |= ui.ModShift
	}
	if mods&(1<<18) != 0 {
		flags |= ui.ModControl
	}
	if mods&(1<<19) != 0 {
		flags |= ui.ModAlt
	}
	if mods&(1<<20) != 0 {
		flags |= ui.ModSuper
	}
	switch typ {
	case 1, 2, 3, 4, 5, 6, 7, 9, 22, 25, 26, 27:
		if typ == 1 || typ == 3 || typ == 25 {
			w.cancelMarked()
		}
		position := objc.Send[point](event, selector("locationInWindow"))
		x, y := float32(position.X), float32(float64(height)-position.Y)
		kind := ui.PointerMove
		if typ == 1 || typ == 3 || typ == 25 {
			kind = ui.PointerDown
		}
		if typ == 2 || typ == 4 || typ == 26 {
			kind = ui.PointerUp
		}
		if typ == 9 {
			x, y = -1e6, -1e6
		}
		if typ == 22 {
			dx := objc.Send[float64](event, selector("scrollingDeltaX"))
			dy := objc.Send[float64](event, selector("scrollingDeltaY"))
			w.manager.HandleEvent(ui.Event{Type: ui.PointerScroll, X: x, Y: y, Scroll: ui.Vec2{X: float32(dx / 40), Y: float32(dy / 40)}, Mods: flags})
			break
		}
		button := ui.MouseLeft
		if typ == 3 || typ == 4 || typ == 7 {
			button = ui.MouseRight
		}
		if typ == 25 || typ == 26 || typ == 27 {
			button = ui.MouseMiddle
		}
		w.manager.HandleEvent(ui.Event{Type: kind, Button: button, X: x, Y: y, Mods: flags})
	case 10, 11:
		code := objc.Send[uint16](event, selector("keyCode"))
		key := macKey(code)
		kind := ui.KeyDown
		if typ == 11 {
			kind = ui.KeyUp
		}
		e := ui.Event{Type: kind, Key: key, Mods: flags, Repeat: objc.Send[bool](event, selector("isARepeat"))}
		if typ == 11 {
			w.manager.HandleEvent(e)
			break
		}
		w.syncInputFocus()
		if w.manager.TextInputFocused() && flags&ui.ModSuper == 0 {
			w.currentKey = e
			w.inputHandled = false
			events := class("NSArray").Send(selector("arrayWithObject:"), event)
			w.view.Send(selector("interpretKeyEvents:"), events)
		} else {
			if flags&ui.ModSuper != 0 {
				w.cancelMarked()
			}
			handled := w.manager.HandleEvent(e)
			if flags&ui.ModSuper != 0 && key == ui.KeyQ && !handled {
				if activeApplication != nil {
					for _, other := range activeApplication.windows {
						other.Close()
					}
				}
			}
		}

	case 12:
		// Modifier state is carried by the subsequent key and pointer events.
	}
}

func macKey(code uint16) ui.Key {
	switch code {
	case 0:
		return ui.KeyA
	case 1:
		return ui.KeyS
	case 2:
		return ui.KeyD
	case 3:
		return ui.KeyF
	case 4:
		return ui.KeyH
	case 5:
		return ui.KeyG
	case 6:
		return ui.KeyZ
	case 7:
		return ui.KeyX
	case 8:
		return ui.KeyC
	case 9:
		return ui.KeyV
	case 11:
		return ui.KeyB
	case 12:
		return ui.KeyQ
	case 13:
		return ui.KeyW
	case 14:
		return ui.KeyE
	case 15:
		return ui.KeyR
	case 16:
		return ui.KeyY
	case 17:
		return ui.KeyT
	case 18:
		return ui.KeyDigit1
	case 19:
		return ui.KeyDigit2
	case 20:
		return ui.KeyDigit3
	case 21:
		return ui.KeyDigit4
	case 22:
		return ui.KeyDigit6
	case 23:
		return ui.KeyDigit5
	case 24:
		return ui.KeyEqual
	case 25:
		return ui.KeyDigit9
	case 26:
		return ui.KeyDigit7
	case 27:
		return ui.KeyMinus
	case 28:
		return ui.KeyDigit8
	case 29:
		return ui.KeyDigit0
	case 30:
		return ui.KeyRightBracket
	case 31:
		return ui.KeyO
	case 32:
		return ui.KeyU
	case 33:
		return ui.KeyLeftBracket
	case 34:
		return ui.KeyI
	case 35:
		return ui.KeyP
	case 36:
		return ui.KeyEnter
	case 37:
		return ui.KeyL
	case 38:
		return ui.KeyJ
	case 39:
		return ui.KeyQuote
	case 40:
		return ui.KeyK
	case 41:
		return ui.KeySemicolon
	case 42:
		return ui.KeyBackslash
	case 43:
		return ui.KeyComma
	case 44:
		return ui.KeySlash
	case 45:
		return ui.KeyN
	case 46:
		return ui.KeyM
	case 47:
		return ui.KeyPeriod
	case 48:
		return ui.KeyTab
	case 49:
		return ui.KeySpace
	case 50:
		return ui.KeyBackquote
	case 51:
		return ui.KeyBackspace
	case 53:
		return ui.KeyEscape
	case 96:
		return ui.KeyF5
	case 97:
		return ui.KeyF6
	case 98:
		return ui.KeyF7
	case 99:
		return ui.KeyF3
	case 100:
		return ui.KeyF8
	case 101:
		return ui.KeyF9
	case 103:
		return ui.KeyF11
	case 109:
		return ui.KeyF10
	case 111:
		return ui.KeyF12
	case 114:
		return ui.KeyInsert
	case 115:
		return ui.KeyHome
	case 116:
		return ui.KeyPageUp
	case 117:
		return ui.KeyDelete
	case 118:
		return ui.KeyF4
	case 119:
		return ui.KeyEnd
	case 120:
		return ui.KeyF2
	case 121:
		return ui.KeyPageDown
	case 122:
		return ui.KeyF1
	case 123:
		return ui.KeyLeftArrow
	case 124:
		return ui.KeyRightArrow
	case 125:
		return ui.KeyDownArrow
	case 126:
		return ui.KeyUpArrow
	}
	return ui.KeyUnknown
}
