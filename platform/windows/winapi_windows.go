//go:build windows && (amd64 || arm64)

package windows

import (
	"fmt"
	"runtime"
	"syscall"
	"unsafe"
)

type point struct{ X, Y int32 }
type rect struct{ Left, Top, Right, Bottom int32 }
type message struct {
	HWND           uintptr
	Message        uint32
	WParam, LParam uintptr
	Time           uint32
	Pt             point
	Private        uint32
}
type windowClass struct {
	Size, Style                                                        uint32
	Proc                                                               uintptr
	ClassExtra, WindowExtra                                            int32
	Instance, Icon, Cursor, Background, MenuName, ClassName, SmallIcon uintptr
}
type paintStruct struct {
	DC                 uintptr
	Erase              int32
	Paint              rect
	Restore, IncUpdate int32
	Reserved           [32]byte
}
type bitmapInfo struct {
	Size                         uint32
	Width, Height                int32
	Planes, BitCount             uint16
	Compression, ImageSize       uint32
	XPelsPerMeter, YPelsPerMeter int32
	ColorsUsed, ColorsImportant  uint32
	Colors                       [1]uint32
}
type trackMouseEvent struct {
	Size, Flags uint32
	HWND        uintptr
	HoverTime   uint32
}
type candidateForm struct {
	Index, Style uint32
	Position     point
	Area         rect
}
type monitorInfo struct {
	Size          uint32
	Monitor, Work rect
	Flags         uint32
}
type iconInfo struct {
	Icon                 int32
	XHotspot, YHotspot   uint32
	MaskBitmap, ColorBmp uintptr
}
type notifyIconData struct {
	Size             uint32
	HWND             uintptr
	ID, Flags        uint32
	CallbackMessage  uint32
	Icon             uintptr
	Tip              [128]uint16
	State, StateMask uint32
	Info             [256]uint16
	Version          uint32
	InfoTitle        [64]uint16
	InfoFlags        uint32
}

type functions struct {
	user, kernel, gdi, imm, shell                                                                                     uintptr
	createWindow, registerClass, defProc, getMessage, peekMessage, translateMessage, dispatchMessage, showWindow      uintptr
	destroyWindow, postQuit, postMessage, postThreadMessage, getCurrentThreadId, beginPaint, endPaint, invalidateRect uintptr
	getClientRect, screenToClient, clientToScreen, getDpi, setDpi, setWindowPos, setWindowLong, getWindowLong         uintptr
	adjustWindowRect, adjustWindowRectForDpi, isZoomed, isIconic, updateWindow, redrawWindow, getSystemMetrics        uintptr
	monitorFromWindow, getMonitorInfo                                                                                 uintptr
	setCapture, releaseCapture, loadCursor, setCursor, loadIcon, getKeyState, trackMouse                              uintptr
	setTimer, killTimer, openClipboard, closeClipboard, emptyClipboard                                                uintptr
	setClipboardData, getClipboardData, globalAlloc, globalLock, globalUnlock, globalFree, globalSize                 uintptr
	stretchDIBits, createDIBSection, createCompatibleDC, deleteDC, selectObject, deleteObject, bitBlt, gdiFlush       uintptr
	createBitmap, createIconIndirect, destroyIcon                                                                     uintptr
	createPopupMenu, destroyMenu, appendMenu, trackPopupMenu, setForegroundWindow, getCursorPos                       uintptr
	getModuleHandle                                                                                                   uintptr
	immGetContext, immReleaseContext, immGetComposition, immSetCandidate                                              uintptr
	dragAcceptFiles, dragQueryFile, dragFinish, shellNotifyIcon                                                       uintptr
}

var win functions

func load() error {
	var err error
	open := func(name string) (uintptr, error) {
		handle, loadErr := syscall.LoadLibrary(name)
		if loadErr != nil {
			return 0, fmt.Errorf("load %s: %w", name, loadErr)
		}
		return uintptr(handle), nil
	}
	win.user, err = open("user32.dll")
	if err != nil {
		return err
	}
	win.gdi, err = open("gdi32.dll")
	if err != nil {
		return err
	}
	win.kernel, err = open("kernel32.dll")
	if err != nil {
		return err
	}
	win.imm, err = open("imm32.dll")
	if err != nil {
		return err
	}
	win.shell, err = open("shell32.dll")
	if err != nil {
		return err
	}
	get := func(lib uintptr, name string, dst *uintptr) error {
		*dst, err = syscall.GetProcAddress(syscall.Handle(lib), name)
		if err != nil {
			return fmt.Errorf("resolve %s: %w", name, err)
		}
		return nil
	}
	symbols := []struct {
		lib  uintptr
		name string
		dst  *uintptr
	}{
		{win.user, "CreateWindowExW", &win.createWindow},
		{win.user, "RegisterClassExW", &win.registerClass},
		{win.user, "DefWindowProcW", &win.defProc},
		{win.user, "GetMessageW", &win.getMessage},
		{win.user, "PeekMessageW", &win.peekMessage},
		{win.user, "TranslateMessage", &win.translateMessage},
		{win.user, "DispatchMessageW", &win.dispatchMessage},
		{win.user, "ShowWindow", &win.showWindow},
		{win.user, "DestroyWindow", &win.destroyWindow},
		{win.user, "PostQuitMessage", &win.postQuit},
		{win.user, "PostMessageW", &win.postMessage},
		{win.user, "PostThreadMessageW", &win.postThreadMessage},
		{win.user, "BeginPaint", &win.beginPaint},
		{win.user, "EndPaint", &win.endPaint},
		{win.user, "InvalidateRect", &win.invalidateRect},
		{win.user, "GetClientRect", &win.getClientRect},
		{win.user, "ScreenToClient", &win.screenToClient},
		{win.user, "ClientToScreen", &win.clientToScreen},
		{win.user, "SetWindowPos", &win.setWindowPos},
		{win.user, "SetWindowLongPtrW", &win.setWindowLong},
		{win.user, "GetWindowLongPtrW", &win.getWindowLong},
		{win.user, "AdjustWindowRectEx", &win.adjustWindowRect},
		{win.user, "IsZoomed", &win.isZoomed},
		{win.user, "IsIconic", &win.isIconic},
		{win.user, "UpdateWindow", &win.updateWindow},
		{win.user, "RedrawWindow", &win.redrawWindow},
		{win.user, "GetSystemMetrics", &win.getSystemMetrics},
		{win.user, "MonitorFromWindow", &win.monitorFromWindow},
		{win.user, "GetMonitorInfoW", &win.getMonitorInfo},
		{win.user, "SetCapture", &win.setCapture},
		{win.user, "ReleaseCapture", &win.releaseCapture},
		{win.user, "LoadCursorW", &win.loadCursor},
		{win.user, "SetCursor", &win.setCursor},
		{win.user, "LoadIconW", &win.loadIcon},
		{win.user, "CreateIconIndirect", &win.createIconIndirect},
		{win.user, "DestroyIcon", &win.destroyIcon},
		{win.user, "CreatePopupMenu", &win.createPopupMenu},
		{win.user, "DestroyMenu", &win.destroyMenu},
		{win.user, "AppendMenuW", &win.appendMenu},
		{win.user, "TrackPopupMenu", &win.trackPopupMenu},
		{win.user, "SetForegroundWindow", &win.setForegroundWindow},
		{win.user, "GetCursorPos", &win.getCursorPos},
		{win.user, "GetKeyState", &win.getKeyState},
		{win.user, "TrackMouseEvent", &win.trackMouse},
		{win.user, "SetTimer", &win.setTimer},
		{win.user, "KillTimer", &win.killTimer},
		{win.user, "OpenClipboard", &win.openClipboard},
		{win.user, "CloseClipboard", &win.closeClipboard},
		{win.user, "EmptyClipboard", &win.emptyClipboard},
		{win.user, "SetClipboardData", &win.setClipboardData},
		{win.user, "GetClipboardData", &win.getClipboardData},
		{win.kernel, "GlobalAlloc", &win.globalAlloc},
		{win.kernel, "GlobalLock", &win.globalLock},
		{win.kernel, "GlobalUnlock", &win.globalUnlock},
		{win.kernel, "GlobalFree", &win.globalFree},
		{win.kernel, "GlobalSize", &win.globalSize},
		{win.gdi, "StretchDIBits", &win.stretchDIBits},
		{win.gdi, "CreateDIBSection", &win.createDIBSection},
		{win.gdi, "CreateCompatibleDC", &win.createCompatibleDC},
		{win.gdi, "DeleteDC", &win.deleteDC},
		{win.gdi, "SelectObject", &win.selectObject},
		{win.gdi, "DeleteObject", &win.deleteObject},
		{win.gdi, "BitBlt", &win.bitBlt},
		{win.gdi, "CreateBitmap", &win.createBitmap},
		{win.gdi, "GdiFlush", &win.gdiFlush},
		{win.kernel, "GetModuleHandleW", &win.getModuleHandle},
		{win.kernel, "GetCurrentThreadId", &win.getCurrentThreadId},
		{win.imm, "ImmGetContext", &win.immGetContext},
		{win.imm, "ImmReleaseContext", &win.immReleaseContext},
		{win.imm, "ImmGetCompositionStringW", &win.immGetComposition},
		{win.imm, "ImmSetCandidateWindow", &win.immSetCandidate},
		{win.shell, "DragAcceptFiles", &win.dragAcceptFiles},
		{win.shell, "DragQueryFileW", &win.dragQueryFile},
		{win.shell, "DragFinish", &win.dragFinish},
		{win.shell, "Shell_NotifyIconW", &win.shellNotifyIcon},
	}
	for _, s := range symbols {
		if err := get(s.lib, s.name, s.dst); err != nil {
			return err
		}
	}
	// A host may have set its process DPI policy already. That is fine.
	win.setDpi, _ = syscall.GetProcAddress(syscall.Handle(win.user), "SetProcessDpiAwarenessContext")
	win.getDpi, _ = syscall.GetProcAddress(syscall.Handle(win.user), "GetDpiForWindow")
	win.adjustWindowRectForDpi, _ = syscall.GetProcAddress(syscall.Handle(win.user), "AdjustWindowRectExForDpi")
	return nil
}

func call(fn uintptr, args ...uintptr) uintptr {
	result, _, _ := syscall.SyscallN(fn, args...)
	return result
}
func pointer[T any](v *T) uintptr     { return uintptr(unsafe.Pointer(v)) }
func wide(s string) ([]uint16, error) { return syscall.UTF16FromString(s) }
func ptr16(s []uint16) uintptr        { return uintptr(unsafe.Pointer(&s[0])) }
func keep(values ...any) {
	for _, v := range values {
		runtime.KeepAlive(v)
	}
}

const (
	wmCreate                = 0x0001
	wmDestroy               = 0x0002
	wmMove                  = 0x0003
	wmSize                  = 0x0005
	wmActivate              = 0x0006
	wmKillFocus             = 0x0008
	wmPaint                 = 0x000F
	wmClose                 = 0x0010
	wmEraseBackground       = 0x0014
	wmShowWindow            = 0x0018
	wmSetCursor             = 0x0020
	wmGetMinMaxInfo         = 0x0024
	wmWindowPosChanged      = 0x0047
	wmDisplayChange         = 0x007E
	wmNcCreate              = 0x0081
	wmNcCalcSize            = 0x0083
	wmNcHitTest             = 0x0084
	wmKeyDown               = 0x0100
	wmKeyUp                 = 0x0101
	wmChar                  = 0x0102
	wmSysKeyDown            = 0x0104
	wmSysKeyUp              = 0x0105
	wmSysChar               = 0x0106
	wmIMEStart              = 0x010D
	wmIMEEnd                = 0x010E
	wmIMEComposition        = 0x010F
	wmSysCommand            = 0x0112
	wmTimer                 = 0x0113
	wmMouseMove             = 0x0200
	wmLeftDown              = 0x0201
	wmLeftUp                = 0x0202
	wmRightDown             = 0x0204
	wmRightUp               = 0x0205
	wmMiddleDown            = 0x0207
	wmMiddleUp              = 0x0208
	wmMouseWheel            = 0x020A
	wmMouseHWheel           = 0x020E
	wmCaptureChanged        = 0x0215
	wmEnterSizeMove         = 0x0231
	wmExitSizeMove          = 0x0232
	wmDropFiles             = 0x0233
	wmMouseLeave            = 0x02A3
	wmDpiChanged            = 0x02E0
	wmWake                  = 0x8001
	wmAppWake               = 0x8002
	wmAppDestroy            = 0x8003
	wmTrayCallback          = 0x8004
	csVRedraw               = 0x0001
	csHRedraw               = 0x0002
	csDblClks               = 0x0008
	wsOverlapped            = 0x00000000
	wsMaximizeBox           = 0x00010000
	wsMinimizeBox           = 0x00020000
	wsThickFrame            = 0x00040000
	wsSysMenu               = 0x00080000
	wsCaption               = 0x00C00000
	wsOverlappedWindow      = 0x00CF0000
	wsMaximize              = 0x01000000
	wsClipChildren          = 0x02000000
	wsClipSiblings          = 0x04000000
	wsDisabled              = 0x08000000
	wsVisible               = 0x10000000
	wsMinimize              = 0x20000000
	wsPopup                 = 0x80000000
	cwUseDefault            = 0x80000000
	swHide                  = 0
	swNormal                = 1
	swMaximize              = 3
	swShow                  = 5
	swMinimize              = 6
	swRestore               = 9
	swpNoSize               = 0x0001
	swpNoMove               = 0x0002
	swpNoZOrder             = 0x0004
	swpNoActivate           = 0x0010
	swpFrameChanged         = 0x0020
	swpShowWindow           = 0x0040
	swpNoOwnerZOrder        = 0x0200
	rdwInvalidate           = 0x0001
	rdwErase                = 0x0004
	rdwAllChildren          = 0x0080
	rdwUpdatenow            = 0x0100
	rdwFrame                = 0x0400
	sizeRestored            = 0
	sizeMinimized           = 1
	sizeMaximized           = 2
	pmRemove                = 0x0001
	smCxScreen              = 0
	smCyScreen              = 1
	monitorDefaultToNearest = 2
	gwlStyle                = ^uintptr(15) // -16
	biRGB                   = 0
	dibRGBColors            = 0
	srccopy                 = 0x00CC0020
	cfUnicodeText           = 13
	gmemMoveable            = 0x0002
	gcsCompStr              = 0x0008
	gcsResultStr            = 0x0800
	cfsExclude              = 0x0080
	nimAdd                  = 0x00000000
	nimModify               = 0x00000001
	nimDelete               = 0x00000002
	nifMessage              = 0x00000001
	nifIcon                 = 0x00000002
	nifTip                  = 0x00000004
	idiApplication          = 32512
	idcArrow                = 32512
	idcIBeam                = 32513
	mfString                = 0x0000
	mfGrayed                = 0x0001
	mfDisabled              = 0x0002
	mfChecked               = 0x0008
	mfSeparator             = 0x0800
	tpmRightButton          = 0x0002
	tpmBottomAlign          = 0x0020
	tpmNonotify             = 0x0080
	tpmReturnCmd            = 0x0100
	bitmapInfoHeaderSize    = 40
)
