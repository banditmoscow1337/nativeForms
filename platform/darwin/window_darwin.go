//go:build darwin && (amd64 || arm64)

package darwin

import (
	"fmt"
	"image"
	"sync/atomic"
	"time"
	"unsafe"

	ui "github.com/banditmoscow1337/nativeForms"
	"github.com/banditmoscow1337/nativeForms/platform"
	"github.com/banditmoscow1337/nativeForms/render"
	"github.com/banditmoscow1337/nativeForms/software"
	"github.com/ebitengine/purego/objc"
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
	manager                           *ui.Manager
	owner                             *App
	options                           Options
	app, window, view, representation objc.ID
	framebuffer                       *image.RGBA
	renderer                          render.Backend
	fallbackReason                    string
	width, height                     int
	pixelWidth, pixelHeight           int
	geomValid                         bool
	cachedBoundsW, cachedBoundsH      int
	cachedPixelW, cachedPixelH        int
	cachedScale                       float32
	lastFrame                         time.Time
	closing                           atomic.Bool
	marked                            string
	markedSelection                   nsRange
	inputIdentity                     *ui.State
	lastInputCaret                    ui.Rect
	lastInputCursor                   int
	lastInputAnchor                   int
	lastInputBase                     int
	currentKey                        ui.Event
	inputHandled                      bool
	wasKey                            bool
}

var _ platform.FileDialogs = (*Window)(nil)

func New(manager *ui.Manager, options Options) *Window {
	return &Window{manager: manager, options: options}
}

func (w *Window) styleMask() uint64 {
	if w.options.Frameless {
		return uint64(8) // NSWindowStyleMaskBorderless | NSWindowStyleMaskResizable
	}
	return uint64(1 | 2 | 4 | 8)
}

func (w *Window) applyWindowControls() {
	if w.window == 0 {
		return
	}
	hidden := w.options.HideWindowControls || w.options.Frameless
	for btnIndex := uint64(0); btnIndex <= 2; btnIndex++ {
		btn := w.window.Send(selector("standardWindowButton:"), btnIndex)
		if btn != 0 {
			btn.Send(selector("setHidden:"), hidden)
		}
	}
}

func (w *Window) runOnMain(fn func()) {
	if w.owner != nil && w.owner.running.Load() && onMainThread() == 0 {
		w.owner.Post(fn)
		return
	}
	fn()
}

func (w *Window) SetFrameless(frameless bool) {
	if w == nil {
		return
	}
	w.options.Frameless = frameless
	w.runOnMain(func() {
		if w.window == 0 || w.closing.Load() {
			return
		}
		w.geomValid = false
		w.window.Send(selector("setStyleMask:"), w.styleMask())
		w.applyWindowControls()
		if w.view != 0 {
			w.window.Send(selector("makeFirstResponder:"), w.view)
		}
		w.manager.InvalidateLayout()
	})
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
	w.runOnMain(func() {
		if w.window == 0 || w.closing.Load() {
			return
		}
		w.applyWindowControls()
		w.manager.InvalidatePaint()
	})
}

func (w *Window) WindowControlsVisible() bool {
	if w == nil {
		return false
	}
	return !w.options.HideWindowControls && !w.options.Frameless
}

func (w *Window) Minimize() {
	if w == nil || w.owner == nil {
		return
	}
	w.owner.Post(func() {
		if w.window != 0 && !w.closing.Load() {
			w.geomValid = false
			w.window.Send(selector("miniaturize:"), objc.ID(0))
		}
	})
}

func (w *Window) Maximize() {
	if w == nil || w.owner == nil {
		return
	}
	w.owner.Post(func() {
		if w.window != 0 && !w.closing.Load() {
			w.geomValid = false
			w.window.Send(selector("zoom:"), objc.ID(0))
		}
	})
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
	w.geomValid = false
	if w.window != 0 {
		w.window.Send(selector("setContentSize:"), size{W: float64(width), H: float64(height)})
		w.manager.InvalidateLayout()
	}
	w.options.Width, w.options.Height = width, height
	return nil
}

func Capabilities() platform.Capabilities {
	return platform.Capabilities{
		SoftwareFrame: true, Pointer: true, Keyboard: true, TextInput: true,
		Clipboard: true, FileDialog: true, IME: true, MultipleWindows: true,
		DragDrop: true, Tray: true, Frameless: true,
	}
}

func (w *Window) Capabilities() platform.Capabilities { return Capabilities() }

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
	if w.owner != nil && w.owner != app.owner {
		return fmt.Errorf("darwin: window belongs to another app")
	}
	if w.owner == nil {
		w.owner = app.owner
		app.owner.windows = append(app.owner.windows, w)
	}
	w.closing.Store(false)
	w.geomValid = false
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
	winCls := objc.ID(nativeWindowClass)
	if winCls == 0 {
		winCls = class("NSWindow")
	}
	w.window = winCls.Send(selector("alloc")).Send(selector("initWithContentRect:styleMask:backing:defer:"), makeRect(float64(width), float64(height)), w.styleMask(), 2, false)
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
	w.window.Send(selector("setDelegate:"), w.view)
	w.applyWindowControls()
	w.window.Send(selector("center"))
	w.window.Send(selector("makeFirstResponder:"), w.view)
	if w.options.OnFilesDropped != nil {
		types := class("NSMutableArray").Send(selector("array"))
		types.Send(selector("addObject:"), nsString("public.file-url"))
		types.Send(selector("addObject:"), nsString("NSFilenamesPboardType"))
		w.view.Send(selector("registerForDraggedTypes:"), types)
	}
	selection, err := software.Open(w.options.Graphics, func() (render.Backend, error) { return newMetalBackend(w.view) })
	if err != nil {
		w.destroy()
		return err
	}
	w.renderer = selection.Backend
	w.fallbackReason = selection.FallbackReason()
	w.window.Send(selector("makeKeyAndOrderFront:"), objc.ID(0))
	w.lastFrame = time.Now()
	if w.options.Scale > 0 && w.options.Scale <= 8 {
		w.manager.SetDisplayScale(w.options.Scale)
	}
	w.manager.SetInteractive(true)
	w.manager.InvalidateLayout()
	w.manager.SetWakeHandler(w.owner.wake)
	w.manager.SetClipboardHandlers(w.ClipboardText, w.SetClipboardText)
	if w.options.OnReady != nil {
		w.options.OnReady(w)
	}
	return nil
}

func (w *Window) destroy() {
	if w.window == 0 {
		return
	}
	if w.renderer != nil {
		_ = w.renderer.Close()
		w.renderer = nil
	}
	w.cancelMarked()
	w.manager.SetWakeHandler(nil)
	w.manager.SetClipboardHandlers(nil, nil)
	w.manager.ClearInteraction()
	w.manager.SetInteractive(false)
	w.window.Send(selector("setDelegate:"), objc.ID(0))
	if activeApplication != nil {
		delete(activeApplication.windows, w.window)
	}
	if w.owner != nil {
		w.owner.remove(w)
	}
	delete(viewWindows, w.view)
	if msgSendBool0(w.window, selIsVisible) {
		w.window.Send(selector("close"))
	}
	if w.representation != 0 {
		w.representation.Send(selRelease)
		w.representation = 0
	}
	w.window.Send(selRelease)
	w.window = 0
	if w.view != 0 {
		w.view.Send(selRelease)
		w.view = 0
	}
	w.framebuffer = nil
	w.inputIdentity = nil
	w.geomValid = false
}

func (w *Window) Close() {
	if w != nil {
		w.closing.Store(true)
		if w.owner != nil {
			w.owner.Post(func() {})
		}
	}
}

func (w *Window) Show() { w.setVisible(true) }
func (w *Window) Hide() { w.setVisible(false) }
func (w *Window) setVisible(visible bool) {
	if w == nil || w.owner == nil {
		return
	}
	w.owner.Post(func() {
		if w.window == 0 || w.closing.Load() {
			return
		}
		w.geomValid = false
		if visible {
			w.window.Send(selector("makeKeyAndOrderFront:"), objc.ID(0))
			w.manager.InvalidatePaint()
		} else {
			w.cancelMarked()
			w.manager.CancelPointerCapture()
			w.window.Send(selector("orderOut:"), objc.ID(0))
		}
	})
}

func (w *Window) uiScale() float32 {
	if w != nil && w.manager != nil {
		if s := w.manager.DisplayScale(); s > 0 && s <= 8 {
			return s
		}
	}
	return 1
}

func (w *Window) closeAction() platform.CloseAction {
	if w.options.OnCloseRequest != nil {
		return w.options.OnCloseRequest(w)
	}
	return platform.CloseWindow
}

func (w *Window) paint(width, height, pixelWidth, pixelHeight int, scale float32) error {
	if pixelWidth > 16384 || pixelHeight > 16384 || int64(pixelWidth)*int64(pixelHeight) > 64*1024*1024 {
		return fmt.Errorf("darwin: framebuffer exceeds 64 megapixels")
	}
	w.pixelWidth, w.pixelHeight = pixelWidth, pixelHeight
	if width != w.width || height != w.height {
		w.width, w.height = width, height
		w.manager.InvalidateLayout()
	}
	now := time.Now()
	w.manager.BeginFrame(width, height, now.Sub(w.lastFrame).Seconds())
	w.lastFrame = now
	trace := w.manager.FrameTrace()
	frame := w.manager.Frame()
	if gpu, ok := w.renderer.(*metalBackend); ok {
		gpu.SetDrawableSize(pixelWidth, pixelHeight)
		err := gpu.RenderAtScale(frame, nil, scale)
		trace.RecordRenderStats(gpu.RenderStats())
		if err == nil {
			trace.RecordDamage(pixelWidth, pixelHeight)
			trace.Finish("hardware/metal", w.fallbackReason)
			return nil
		}
		// Present the same frame in software after any Metal failure, including
		// a completed command buffer reporting device loss on slot reuse.
		w.fallbackReason = err.Error()
		fallback, fallbackErr := render.Failover(gpu, err, func() (render.Backend, error) { return software.New(), nil })
		if fallbackErr != nil {
			return fmt.Errorf("darwin: Metal fallback: %w", fallbackErr)
		}
		w.renderer = fallback
	}
	if w.framebuffer == nil || w.framebuffer.Bounds().Dx() != pixelWidth || w.framebuffer.Bounds().Dy() != pixelHeight {
		w.framebuffer = image.NewRGBA(image.Rect(0, 0, pixelWidth, pixelHeight))
		if w.representation != 0 {
			w.representation.Send(selRelease)
			w.representation = 0
		}
		w.representation = class("NSBitmapImageRep").Send(selector("alloc"))
		w.representation = w.representation.Send(selector("initWithBitmapDataPlanes:pixelsWide:pixelsHigh:bitsPerSample:samplesPerPixel:hasAlpha:isPlanar:colorSpaceName:bytesPerRow:bitsPerPixel:"),
			uintptr(0), pixelWidth, pixelHeight, 8, 4, true, false, nsString("NSDeviceRGBColorSpace"), pixelWidth*4, 32)
		if w.representation == 0 {
			return fmt.Errorf("darwin: bitmap representation failed")
		}
	}
	w.representation.Send(selector("setSize:"), msgSendRect0(w.view, selBounds).Size)
	rasterStart := trace.Start()
	err := w.renderer.RenderAtScale(frame, w.framebuffer, scale)
	trace.Record(ui.PhaseRaster, rasterStart)
	if err != nil {
		return err
	}
	uploadStart := trace.Start()
	pointer := objc.Send[uintptr](w.representation, selector("bitmapData"))
	if pointer == 0 {
		return fmt.Errorf("darwin: bitmapData is nil")
	}
	dst := unsafe.Slice((*byte)(unsafe.Pointer(pointer)), len(w.framebuffer.Pix))
	copy(dst, w.framebuffer.Pix)
	trace.Record(ui.PhaseUpload, uploadStart)
	presentStart := trace.Start()
	w.view.Send(selector("setNeedsDisplay:"), true)
	trace.Record(ui.PhasePresent, presentStart)
	trace.RecordDamage(pixelWidth, pixelHeight)
	trace.Finish("software/appkit", w.fallbackReason)
	return nil
}
