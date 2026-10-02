//go:build windows && (amd64 || arm64)

// Package windows presents a nativeForms software frame in one Win32 window.
// Run owns the UI goroutine and must be called from the application's main goroutine.
package windows

import (
	"fmt"
	"image"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
	"unicode"
	"unicode/utf16"
	"unsafe"

	ui "github.com/banditmoscow1337/nativeForms"
	"github.com/banditmoscow1337/nativeForms/software"
	"github.com/ebitengine/purego"
)

// Options specifies the initial window size in physical pixels. OnReady runs
// on the UI thread after the window is shown and may update the UI tree.
type Options struct {
	Title string
	Width, Height int
	OnReady func(*Window)
}

type Window struct {
	manager *ui.Manager
	options Options
	hwnd atomic.Uintptr
	dpi uint32
	renderer *software.Renderer
	framebuffer *image.RGBA
	dib []byte
	lastFrame time.Time
	highSurrogate uint16
	captureButton ui.MouseButton
	hasCapture bool
	err error
}

func New(manager *ui.Manager, options Options) *Window {
	return &Window{manager: manager, options: options}
}

var windowMu sync.Mutex
var activeWindow *Window // accessed only on the locked Win32 UI thread
var apiOnce sync.Once
var apiError error
var classOnce sync.Once
var classError error
var className []uint16
var windowCallback uintptr

func (w *Window) Run() error {
	if w == nil || w.manager == nil { return fmt.Errorf("windows: nil window or manager") }
	if !windowMu.TryLock() { return fmt.Errorf("windows: only one window can run at a time") }
	defer windowMu.Unlock()
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	apiOnce.Do(func() { apiError = load() })
	if apiError != nil { return apiError }
	classOnce.Do(registerWindowClass)
	if classError != nil { return classError }
	if win.setDpi != 0 { call(win.setDpi, ^uintptr(3)) } // per-monitor v2, when available
	width,height := w.options.Width,w.options.Height
	if width <= 0 { width = 800 }
	if height <= 0 { height = 600 }
	title := w.options.Title
	if title == "" { title = "nativeForms" }
	name,err := wide(title)
	if err != nil { return err }
	w.renderer = software.New()
	w.lastFrame = time.Now()
	w.dpi=96
	w.err=nil
	activeWindow = w
	defer func() { activeWindow = nil; w.manager.SetWakeHandler(nil) }()
	hwnd := call(win.createWindow, 0, ptr16(className), ptr16(name), wsOverlappedWindow,
		cwUseDefault, cwUseDefault, uintptr(width), uintptr(height), 0, 0,
		call(win.getModuleHandle, 0), 0)
	keep(name,className)
	if hwnd == 0 { return fmt.Errorf("windows: CreateWindowExW failed") }
	w.hwnd.Store(hwnd)
	w.dpi = uint32(call(win.getDpi, hwnd))
	if w.dpi == 0 { w.dpi = 96 }
	w.manager.SetWakeHandler(func() {
		if h := w.hwnd.Load(); h != 0 { call(win.postMessage,h,wmWake,0,0) }
	})
	call(win.showWindow,hwnd,swShow)
	if w.options.OnReady != nil { w.options.OnReady(w) }
	w.invalidate()
	var msg message
	for {
		result := call(win.getMessage,pointer(&msg),0,0,0)
		if int32(result) == -1 {
			if w.hwnd.Load()!=0 { call(win.destroyWindow,w.hwnd.Load()) }
			return fmt.Errorf("windows: GetMessageW failed")
		}
		if result == 0 { break }
		call(win.translateMessage,pointer(&msg))
		call(win.dispatchMessage,pointer(&msg))
	}
	return w.err
}

func registerWindowClass() {
	var err error
	className,err=wide("nativeFormsSoftwareWindow")
	if err!=nil { classError=err;return }
	windowCallback=purego.NewCallback(windowProc)
	class:=windowClass{
		Size:uint32(unsafe.Sizeof(windowClass{})), Proc:windowCallback,
		Instance:call(win.getModuleHandle,0),ClassName:ptr16(className),
	}
	if call(win.registerClass,pointer(&class))==0 {
		classError=fmt.Errorf("windows: RegisterClassExW failed")
	}
	keep(className,&class)
}

func windowProc(hwnd uintptr, msg uint32, wp,lp uintptr) (result uintptr) {
	w:=activeWindow
	if w==nil || (w.hwnd.Load()!=0 && w.hwnd.Load()!=hwnd) {
		return call(win.defProc,hwnd,uintptr(msg),wp,lp)
	}
	defer func() {
		if value:=recover();value!=nil {
			w.err=fmt.Errorf("windows: UI callback: %v",value)
			call(win.postMessage,hwnd,wmClose,0,0)
			result=0
		}
	}()
	return w.handle(hwnd,msg,wp,lp)
}

func (w *Window) handle(hwnd uintptr, msg uint32, wp,lp uintptr) uintptr {
	switch msg {
	case wmPaint:
		var ps paintStruct
		dc:=call(win.beginPaint,hwnd,pointer(&ps))
		if dc!=0 {
			if err:=w.paint(hwnd,dc);err!=nil {
				w.err=err
				call(win.postMessage,hwnd,wmClose,0,0)
			}
			call(win.endPaint,hwnd,pointer(&ps))
		}
		return 0
	case wmEraseBackground:
		return 1 // the complete DIB is presented by WM_PAINT
	case wmSize:
		w.manager.InvalidateLayout()
		w.invalidate()
		return 0
	case wmDpiChanged:
		w.dpi=uint32(wp&0xffff)
		if w.dpi==0 { w.dpi=96 }
		r:=(*rect)(unsafe.Pointer(lp))
		if r!=nil {
			call(win.setWindowPos,hwnd,0,uintptr(r.Left),uintptr(r.Top),
				uintptr(r.Right-r.Left),uintptr(r.Bottom-r.Top),swpNoZOrder|swpNoActivate)
		}
		w.manager.InvalidateLayout()
		w.invalidate()
		return 0
	case wmWake:
		w.manager.ProcessPending()
		if w.manager.NeedsFrame() { w.invalidate() }
		return 0
	case wmTimer:
		call(win.killTimer,hwnd,1)
		w.invalidate()
		return 0
	case wmMouseMove,wmLeftDown,wmLeftUp,wmRightDown,wmRightUp,wmMiddleDown,wmMiddleUp,wmMouseWheel:
		w.mouse(hwnd,msg,wp,lp)
		return 0
	case wmMouseLeave:
		w.manager.HandleEvent(ui.Event{Type:ui.PointerMove,X:-1e6,Y:-1e6})
		return 0
	case wmCaptureChanged:
		w.hasCapture=false
		w.manager.CancelPointerCapture()
		return 0
	case wmKeyDown,wmKeyUp:
		key:=translateKey(uint32(wp))
		w.manager.HandleEvent(ui.Event{Type:map[bool]ui.EventType{true:ui.KeyDown,false:ui.KeyUp}[msg==wmKeyDown],
			Key:key,Repeat:msg==wmKeyDown && (lp&(1<<30))!=0,Mods:w.mods()})
		return 0
	case wmChar:
		w.character(uint16(wp))
		return 0
	case wmKillFocus:
		w.highSurrogate=0
		w.manager.ClearInteraction()
		return 0
	case wmSetCursor:
		if lp&0xffff==1 { // HTCLIENT
			id:=uintptr(32512) // IDC_ARROW
			if w.manager.HoveredTextInput() { id=32513 } // IDC_IBEAM
			call(win.setCursor,call(win.loadCursor,0,id))
			return 1
		}
	case wmClose:
		call(win.destroyWindow,hwnd)
		return 0
	case wmDestroy:
		w.hwnd.Store(0)
		call(win.killTimer,hwnd,1)
		call(win.postQuit,0)
		return 0
	}
	return call(win.defProc,hwnd,uintptr(msg),wp,lp)
}

func (w *Window) paint(hwnd,dc uintptr) error {
	var client rect
	if call(win.getClientRect,hwnd,pointer(&client))==0 { return fmt.Errorf("windows: GetClientRect failed") }
	width,height:=int(client.Right-client.Left),int(client.Bottom-client.Top)
	if width<=0 || height<=0 { return nil }
	if width>16384 || height>16384 || int64(width)*int64(height)>64*1024*1024 {
		return fmt.Errorf("windows: framebuffer exceeds 64 megapixels")
	}
	if w.framebuffer==nil || w.framebuffer.Bounds().Dx()!=width || w.framebuffer.Bounds().Dy()!=height {
		w.framebuffer=image.NewRGBA(image.Rect(0,0,width,height))
		w.dib=make([]byte,width*height*4)
	}
	logicalWidth:=max(1,int((int64(width)*96+int64(w.dpi)/2)/int64(w.dpi)))
	logicalHeight:=max(1,int((int64(height)*96+int64(w.dpi)/2)/int64(w.dpi)))
	now:=time.Now()
	delta:=now.Sub(w.lastFrame).Seconds()
	w.lastFrame=now
	w.manager.BeginFrame(logicalWidth,logicalHeight,delta)
	if err:=w.renderer.RenderScaled(w.manager.Frame(),w.framebuffer);err!=nil { return err }
	for y:=0;y<height;y++ {
		for x:=0;x<width;x++ {
			i:=w.framebuffer.PixOffset(x,y)
			j:=(y*width+x)*4
			w.dib[j],w.dib[j+1],w.dib[j+2],w.dib[j+3]=
				w.framebuffer.Pix[i+2],w.framebuffer.Pix[i+1],w.framebuffer.Pix[i],w.framebuffer.Pix[i+3]
		}
	}
	info:=bitmapInfo{Size:uint32(unsafe.Sizeof(bitmapInfo{})),Width:int32(width),Height:-int32(height),
		Planes:1,BitCount:32,Compression:biRGB}
	result:=call(win.stretchDIBits,dc,0,0,uintptr(width),uintptr(height),0,0,
		uintptr(width),uintptr(height),pointer(&w.dib[0]),pointer(&info),dibRGBColors,srccopy)
	keep(w.dib,&info)
	if int32(result)==-1 { return fmt.Errorf("windows: StretchDIBits failed") }
	call(win.killTimer,hwnd,1)
	if delay:=w.manager.NextFrameAfter();delay>0 {
		ms:=uint64((delay+time.Millisecond-1)/time.Millisecond)
		if ms>0xffffffff { ms=0xffffffff }
		call(win.setTimer,hwnd,1,uintptr(max(uint64(1),ms)),0)
	}
	return nil
}

func (w *Window) invalidate() {
	if h:=w.hwnd.Load();h!=0 { call(win.invalidateRect,h,0,0) }
}

// Close requests shutdown and is safe to call from a different goroutine.
func (w *Window) Close() {
	if w!=nil {
		if h:=w.hwnd.Load();h!=0 { call(win.postMessage,h,wmClose,0,0) }
	}
}

func (w *Window) mouse(hwnd uintptr,msg uint32,wp,lp uintptr) {
	if msg==wmMouseMove {
		tracking:=trackMouseEvent{Size:uint32(unsafe.Sizeof(trackMouseEvent{})),Flags:2,HWND:hwnd}
		call(win.trackMouse,pointer(&tracking))
	}
	p:=point{X:int32(int16(lp&0xffff)),Y:int32(int16((lp>>16)&0xffff))}
	if msg==wmMouseWheel {
		call(win.screenToClient,hwnd,pointer(&p))
	}
	scale:=float32(96)/float32(w.dpi)
	event:=ui.Event{X:float32(p.X)*scale,Y:float32(p.Y)*scale,Mods:w.mods()}
	switch msg {
	case wmMouseMove: event.Type=ui.PointerMove
	case wmMouseWheel:
		event.Type=ui.PointerScroll
		event.Scroll.Y=float32(int16((wp>>16)&0xffff))/120
	case wmLeftDown,wmRightDown,wmMiddleDown:
		event.Type=ui.PointerDown
		event.Button=button(msg)
		if !w.hasCapture {
			call(win.setCapture,hwnd)
			w.captureButton=event.Button
			w.hasCapture=true
		}
	case wmLeftUp,wmRightUp,wmMiddleUp:
		event.Type=ui.PointerUp
		event.Button=button(msg)
	}
	w.manager.HandleEvent(event)
	if event.Type==ui.PointerUp && w.hasCapture && event.Button==w.captureButton {
		w.hasCapture=false
		call(win.releaseCapture)
	}
}

func button(msg uint32) ui.MouseButton {
	switch msg {
	case wmRightDown,wmRightUp: return ui.MouseRight
	case wmMiddleDown,wmMiddleUp: return ui.MouseMiddle
	default: return ui.MouseLeft
	}
}

func (w *Window) mods() int {
	mods:=0
	if call(win.getKeyState,0x10)&0x8000!=0 { mods|=ui.ModShift }
	if call(win.getKeyState,0x11)&0x8000!=0 { mods|=ui.ModControl }
	if call(win.getKeyState,0x12)&0x8000!=0 { mods|=ui.ModAlt }
	if call(win.getKeyState,0x5B)&0x8000!=0 || call(win.getKeyState,0x5C)&0x8000!=0 { mods|=ui.ModSuper }
	return mods
}

func translateKey(key uint32) ui.Key {
	switch key {
	case 0x20: return ui.KeySpace
	case 0x0D: return ui.KeyEnter
	case 0x1B: return ui.KeyEscape
	case 0x09: return ui.KeyTab
	case 0x26: return ui.KeyUpArrow
	case 0x28: return ui.KeyDownArrow
	case 0x25: return ui.KeyLeftArrow
	case 0x27: return ui.KeyRightArrow
	case 0x11: return ui.KeyControl
	case 0x08: return ui.KeyBackspace
	case 0x2E: return ui.KeyDelete
	case 0x24: return ui.KeyHome
	case 0x23: return ui.KeyEnd
	case 0x74: return ui.KeyF5
	case 0x75: return ui.KeyF6
	case 0x77: return ui.KeyF8
	case 'W': return ui.KeyW
	case 'A': return ui.KeyA
	case 'S': return ui.KeyS
	case 'D': return ui.KeyD
	case 'Q': return ui.KeyQ
	case 'E': return ui.KeyE
	case 'R': return ui.KeyR
	case 'F': return ui.KeyF
	case 'Z': return ui.KeyZ
	case 'Y': return ui.KeyY
	default: return ui.KeyUnknown
	}
}

func (w *Window) character(code uint16) {
	if code>=0xd800 && code<=0xdbff { w.highSurrogate=code;return }
	var r rune
	if code>=0xdc00 && code<=0xdfff && w.highSurrogate!=0 {
		r=utf16.DecodeRune(rune(w.highSurrogate),rune(code))
	} else {
		r=rune(code)
	}
	w.highSurrogate=0
	if r>=32 && r!=0x7f && r!=unicode.ReplacementChar {
		w.manager.HandleEvent(ui.Event{Type:ui.TextInput,Rune:r,Mods:w.mods()})
	}
}
