//go:build darwin && (amd64 || arm64)

// Package darwin presents software frames through AppKit using purego/objc.
package darwin

import (
	"fmt"
	"image"
	"runtime"
	"structs"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"
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
	Title         string
	Width, Height int
	OnReady       func(*Window)
}

type Window struct {
	manager                                   *ui.Manager
	options                                   Options
	app, window, view, bitmap, representation objc.ID
	framebuffer                               *image.RGBA
	renderer                                  *software.Renderer
	width, height                             int
	lastFrame                                 time.Time
	closing                                   atomic.Bool
}

func New(manager *ui.Manager, options Options) *Window {
	return &Window{manager: manager, options: options}
}

func Capabilities() platform.Capabilities {
	return platform.Capabilities{SoftwareFrame: true, Pointer: true, Keyboard: true, TextInput: true, Clipboard: true}
}

func (w *Window) Capabilities() platform.Capabilities { return Capabilities() }

var appMu sync.Mutex

func selector(name string) objc.SEL { return objc.RegisterName(name) }
func class(name string) objc.ID     { return objc.ID(objc.GetClass(name)) }
func nsString(value string) objc.ID {
	return class("NSString").Send(selector("stringWithUTF8String:"), value)
}

func (w *Window) Run() error {
	if w == nil || w.manager == nil {
		return fmt.Errorf("darwin: nil window or manager")
	}
	if !appMu.TryLock() {
		return platform.Unsupported("darwin", platform.FeatureMultipleWindows)
	}
	defer appMu.Unlock()
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if _, err := purego.Dlopen("/System/Library/Frameworks/Cocoa.framework/Cocoa", purego.RTLD_GLOBAL|purego.RTLD_LAZY); err != nil {
		return err
	}
	pool := class("NSAutoreleasePool").Send(selector("new"))
	defer pool.Send(selector("drain"))
	w.app = class("NSApplication").Send(selector("sharedApplication"))
	w.app.Send(selector("setActivationPolicy:"), 0)
	width, height := w.options.Width, w.options.Height
	if width <= 0 {
		width = 800
	}
	if height <= 0 {
		height = 600
	}
	w.window = class("NSWindow").Send(selector("alloc"))
	w.window = w.window.Send(selector("initWithContentRect:styleMask:backing:defer:"), makeRect(float64(width), float64(height)), uint64(1|2|4|8), 2, false)
	if w.window == 0 {
		return fmt.Errorf("darwin: NSWindow initialization failed")
	}
	defer w.window.Send(selector("release"))
	title := w.options.Title
	if title == "" {
		title = "nativeForms"
	}
	w.window.Send(selector("setTitle:"), nsString(title))
	w.view = class("NSImageView").Send(selector("alloc")).Send(selector("initWithFrame:"), makeRect(float64(width), float64(height)))
	if w.view == 0 {
		return fmt.Errorf("darwin: NSImageView initialization failed")
	}
	defer w.view.Send(selector("release"))
	w.view.Send(selector("setAutoresizingMask:"), uint64(2|16))
	w.window.Send(selector("setContentView:"), w.view)
	w.window.Send(selector("center"))
	w.app.Send(selector("finishLaunching"))
	w.window.Send(selector("makeKeyAndOrderFront:"), objc.ID(0))
	w.app.Send(selector("activateIgnoringOtherApps:"), true)
	w.renderer = software.New()
	w.lastFrame = time.Now()
	w.manager.SetInteractive(true)
	w.manager.SetClipboardHandlers(w.ClipboardText, w.SetClipboardText)
	defer func() {
		w.manager.SetClipboardHandlers(nil, nil)
		w.manager.SetInteractive(false)
		if w.representation != 0 {
			w.representation.Send(selector("release"))
		}
		if w.bitmap != 0 {
			w.bitmap.Send(selector("release"))
		}
	}()
	if w.options.OnReady != nil {
		w.options.OnReady(w)
	}
	mode := nsString("kCFRunLoopDefaultMode")
	for !w.closing.Load() && objc.Send[bool](w.window, selector("isVisible")) {
		iterationPool := class("NSAutoreleasePool").Send(selector("new"))
		bounds := objc.Send[rect](w.view, selector("bounds"))
		newWidth, newHeight := int(bounds.Size.W), int(bounds.Size.H)
		if newWidth > 0 && newHeight > 0 && (w.manager.NeedsFrame() || newWidth != w.width || newHeight != w.height) {
			if err := w.paint(newWidth, newHeight); err != nil {
				iterationPool.Send(selector("drain"))
				return err
			}
		}
		seconds := 0.05
		if delay := w.manager.NextFrameAfter(); delay > 0 && delay.Seconds() < seconds {
			seconds = delay.Seconds()
		}
		date := class("NSDate").Send(selector("dateWithTimeIntervalSinceNow:"), seconds)
		event := w.app.Send(selector("nextEventMatchingMask:untilDate:inMode:dequeue:"), ^uint64(0), date, mode, true)
		if event != 0 {
			eventType := objc.Send[uint64](event, selector("type"))
			w.event(event, newHeight)
			// Keyboard input belongs to the nativeForms tree. Forwarding it to
			// an NSImageView without a text responder makes AppKit beep.
			if eventType != 10 && eventType != 11 {
				w.app.Send(selector("sendEvent:"), event)
			}
		}
		iterationPool.Send(selector("drain"))
	}
	return nil
}

func (w *Window) Close() {
	if w != nil {
		w.closing.Store(true)
	}
}

func (w *Window) paint(width, height int) error {
	if width > 16384 || height > 16384 || int64(width)*int64(height) > 64*1024*1024 {
		return fmt.Errorf("darwin: framebuffer exceeds 64 megapixels")
	}
	if w.framebuffer == nil || width != w.width || height != w.height {
		w.framebuffer = image.NewRGBA(image.Rect(0, 0, width, height))
		w.width, w.height = width, height
		w.manager.InvalidateLayout()
		if w.representation != 0 {
			w.representation.Send(selector("release"))
			w.representation = 0
		}
		if w.bitmap != 0 {
			w.bitmap.Send(selector("release"))
			w.bitmap = 0
		}
		w.representation = class("NSBitmapImageRep").Send(selector("alloc"))
		w.representation = w.representation.Send(selector("initWithBitmapDataPlanes:pixelsWide:pixelsHigh:bitsPerSample:samplesPerPixel:hasAlpha:isPlanar:colorSpaceName:bytesPerRow:bitsPerPixel:"),
			uintptr(0), width, height, 8, 4, true, false, nsString("NSDeviceRGBColorSpace"), width*4, 32)
		if w.representation == 0 {
			return fmt.Errorf("darwin: bitmap representation failed")
		}
		w.bitmap = class("NSImage").Send(selector("alloc")).Send(selector("initWithSize:"), size{W: float64(width), H: float64(height)})
		if w.bitmap == 0 {
			return fmt.Errorf("darwin: NSImage initialization failed")
		}
		w.bitmap.Send(selector("addRepresentation:"), w.representation)
		w.view.Send(selector("setImage:"), w.bitmap)
	}
	now := time.Now()
	w.manager.BeginFrame(width, height, now.Sub(w.lastFrame).Seconds())
	w.lastFrame = now
	if err := w.renderer.Render(w.manager.Frame(), w.framebuffer); err != nil {
		return err
	}
	pointer := objc.Send[uintptr](w.representation, selector("bitmapData"))
	if pointer == 0 {
		return fmt.Errorf("darwin: bitmapData is nil")
	}
	dst := unsafe.Slice((*byte)(unsafe.Pointer(pointer)), len(w.framebuffer.Pix))
	copy(dst, w.framebuffer.Pix)
	w.view.Send(selector("setNeedsDisplay:"), true)
	return nil
}

func (w *Window) ClipboardText() (string, error) {
	board := class("NSPasteboard").Send(selector("generalPasteboard"))
	value := board.Send(selector("stringForType:"), nsString("public.utf8-plain-text"))
	if value == 0 {
		return "", nil
	}
	return stringValue(value), nil
}

func (w *Window) SetClipboardText(value string) error {
	board := class("NSPasteboard").Send(selector("generalPasteboard"))
	board.Send(selector("clearContents"))
	if !objc.Send[bool](board, selector("setString:forType:"), nsString(value), nsString("public.utf8-plain-text")) {
		return fmt.Errorf("darwin: clipboard write failed")
	}
	return nil
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
		handled := w.manager.HandleEvent(ui.Event{Type: kind, Key: key, Mods: flags, Repeat: objc.Send[bool](event, selector("isARepeat"))})
		if typ == 10 && flags&ui.ModSuper != 0 && key == ui.KeyQ && !handled {
			w.Close()
			break
		}
		if typ == 10 && flags&(ui.ModControl|ui.ModAlt|ui.ModSuper) == 0 {
			value := stringValue(event.Send(selector("characters")))
			for len(value) > 0 {
				r, n := utf8.DecodeRuneInString(value)
				value = value[n:]
				if r >= 32 {
					w.manager.HandleEvent(ui.Event{Type: ui.TextInput, Rune: r})
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
	case 6:
		return ui.KeyZ
	case 7:
		return ui.KeyX
	case 8:
		return ui.KeyC
	case 9:
		return ui.KeyV
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
	case 36:
		return ui.KeyEnter
	case 48:
		return ui.KeyTab
	case 49:
		return ui.KeySpace
	case 51:
		return ui.KeyBackspace
	case 53:
		return ui.KeyEscape
	case 96:
		return ui.KeyF5
	case 97:
		return ui.KeyF6
	case 100:
		return ui.KeyF8
	case 115:
		return ui.KeyHome
	case 117:
		return ui.KeyDelete
	case 119:
		return ui.KeyEnd
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
