//go:build windows && (amd64 || arm64)

package windows

import (
	"fmt"
	"runtime"
	"syscall"
	"unsafe"

	"github.com/ebitengine/purego"
)

type point struct { X,Y int32 }
type rect struct { Left,Top,Right,Bottom int32 }
type message struct {
	HWND uintptr
	Message uint32
	WParam,LParam uintptr
	Time uint32
	Pt point
	Private uint32
}
type windowClass struct {
	Size,Style uint32
	Proc uintptr
	ClassExtra,WindowExtra int32
	Instance,Icon,Cursor,Background,MenuName,ClassName,SmallIcon uintptr
}
type paintStruct struct {
	DC uintptr
	Erase int32
	Paint rect
	Restore,IncUpdate int32
	Reserved [32]byte
}
type bitmapInfo struct {
	Size uint32
	Width,Height int32
	Planes,BitCount uint16
	Compression,ImageSize uint32
	XPelsPerMeter,YPelsPerMeter int32
	ColorsUsed,ColorsImportant uint32
}
type trackMouseEvent struct {
	Size, Flags uint32
	HWND uintptr
	HoverTime uint32
}
type candidateForm struct {
	Index,Style uint32
	Position point
	Area rect
}

type functions struct {
	user,kernel,gdi,imm uintptr
	createWindow,registerClass,defProc,getMessage,translateMessage,dispatchMessage,showWindow uintptr
	destroyWindow,postQuit,postMessage,beginPaint,endPaint,invalidateRect uintptr
	getClientRect,screenToClient,getDpi,setDpi,setWindowPos uintptr
	setCapture,releaseCapture,loadCursor,setCursor,getKeyState,trackMouse uintptr
	setTimer,killTimer,openClipboard,closeClipboard,emptyClipboard uintptr
	setClipboardData,getClipboardData,globalAlloc,globalLock,globalUnlock,globalFree,globalSize uintptr
	stretchDIBits,getModuleHandle uintptr
	immGetContext,immReleaseContext,immGetComposition,immSetCandidate uintptr
}
var win functions

func load() error {
	var err error
	win.user,err=purego.Dlopen("user32.dll",0);if err!=nil{return err}
	win.gdi,err=purego.Dlopen("gdi32.dll",0);if err!=nil{return err}
	win.kernel,err=purego.Dlopen("kernel32.dll",0);if err!=nil{return err}
	win.imm,err=purego.Dlopen("imm32.dll",0);if err!=nil{return err}
	get:=func(lib uintptr,name string,dst *uintptr) error {
		*dst,err=purego.Dlsym(lib,name)
		if err!=nil{return fmt.Errorf("resolve %s: %w",name,err)}
		return nil
	}
	symbols:=[]struct{ lib uintptr; name string; dst *uintptr }{
		{win.user,"CreateWindowExW",&win.createWindow},
		{win.user,"RegisterClassExW",&win.registerClass},
		{win.user,"DefWindowProcW",&win.defProc},
		{win.user,"GetMessageW",&win.getMessage},
		{win.user,"TranslateMessage",&win.translateMessage},
		{win.user,"DispatchMessageW",&win.dispatchMessage},
		{win.user,"ShowWindow",&win.showWindow},
		{win.user,"DestroyWindow",&win.destroyWindow},
		{win.user,"PostQuitMessage",&win.postQuit},
		{win.user,"PostMessageW",&win.postMessage},
		{win.user,"BeginPaint",&win.beginPaint},
		{win.user,"EndPaint",&win.endPaint},
		{win.user,"InvalidateRect",&win.invalidateRect},
		{win.user,"GetClientRect",&win.getClientRect},
		{win.user,"ScreenToClient",&win.screenToClient},
		{win.user,"GetDpiForWindow",&win.getDpi},
		{win.user,"SetWindowPos",&win.setWindowPos},
		{win.user,"SetCapture",&win.setCapture},
		{win.user,"ReleaseCapture",&win.releaseCapture},
		{win.user,"LoadCursorW",&win.loadCursor},
		{win.user,"SetCursor",&win.setCursor},
		{win.user,"GetKeyState",&win.getKeyState},
		{win.user,"TrackMouseEvent",&win.trackMouse},
		{win.user,"SetTimer",&win.setTimer},
		{win.user,"KillTimer",&win.killTimer},
		{win.user,"OpenClipboard",&win.openClipboard},
		{win.user,"CloseClipboard",&win.closeClipboard},
		{win.user,"EmptyClipboard",&win.emptyClipboard},
		{win.user,"SetClipboardData",&win.setClipboardData},
		{win.user,"GetClipboardData",&win.getClipboardData},
		{win.kernel,"GlobalAlloc",&win.globalAlloc},
		{win.kernel,"GlobalLock",&win.globalLock},
		{win.kernel,"GlobalUnlock",&win.globalUnlock},
		{win.kernel,"GlobalFree",&win.globalFree},
		{win.kernel,"GlobalSize",&win.globalSize},
		{win.gdi,"StretchDIBits",&win.stretchDIBits},
		{win.kernel,"GetModuleHandleW",&win.getModuleHandle},
		{win.imm,"ImmGetContext",&win.immGetContext},
		{win.imm,"ImmReleaseContext",&win.immReleaseContext},
		{win.imm,"ImmGetCompositionStringW",&win.immGetComposition},
		{win.imm,"ImmSetCandidateWindow",&win.immSetCandidate},
	}
	for _,s:=range symbols {if err:=get(s.lib,s.name,s.dst);err!=nil{return err}}
	// A host may have set its process DPI policy already. That is fine.
	win.setDpi,_=purego.Dlsym(win.user,"SetProcessDpiAwarenessContext")
	return nil
}

func call(fn uintptr,args ...uintptr) uintptr {
	result,_,_:=purego.SyscallN(fn,args...)
	return result
}
func pointer[T any](v *T) uintptr {return uintptr(unsafe.Pointer(v))}
func wide(s string) ([]uint16,error) {return syscall.UTF16FromString(s)}
func ptr16(s []uint16) uintptr {return uintptr(unsafe.Pointer(&s[0]))}
func keep(values ...any) {for _,v:=range values {runtime.KeepAlive(v)}}

const (
	wmDestroy=0x0002
	wmSize=0x0005
	wmSetCursor=0x0020
	wmKillFocus=0x0008
	wmPaint=0x000F
	wmEraseBackground=0x0014
	wmClose=0x0010
	wmKeyDown=0x0100
	wmKeyUp=0x0101
	wmChar=0x0102
	wmIMEStart=0x010D
	wmIMEEnd=0x010E
	wmIMEComposition=0x010F
	wmTimer=0x0113
	wmMouseMove=0x0200
	wmLeftDown=0x0201
	wmLeftUp=0x0202
	wmRightDown=0x0204
	wmRightUp=0x0205
	wmMiddleDown=0x0207
	wmMiddleUp=0x0208
	wmMouseWheel=0x020A
	wmMouseHWheel=0x020E
	wmMouseLeave=0x02A3
	wmCaptureChanged=0x0215
	wmDpiChanged=0x02E0
	wmWake=0x8001
	wsOverlappedWindow=0x00CF0000
	cwUseDefault=0x80000000
	swShow=5
	swpNoZOrder=0x0004
	swpNoActivate=0x0010
	biRGB=0
	dibRGBColors=0
	srccopy=0x00CC0020
	cfUnicodeText=13
	gmemMoveable=0x0002
	gcsCompStr=0x0008
	gcsResultStr=0x0800
	cfsExclude=0x0080
)
