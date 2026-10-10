//go:build windows && (amd64 || arm64)

package windows

import (
	"fmt"
	"runtime"
	"unicode/utf16"
	"unsafe"
)

// SetClipboardText copies UTF-8 text into the system clipboard. Call this
// from the Run goroutine (for example a widget callback or Options.OnReady).
func (w *Window) SetClipboardText(value string) error {
	hwnd:=w.hwnd.Load()
	if hwnd==0 { return fmt.Errorf("windows: window is not running") }
	if call(win.openClipboard,hwnd)==0 { return fmt.Errorf("windows: OpenClipboard failed") }
	defer call(win.closeClipboard)
	units:=utf16.Encode([]rune(value))
	units=append(units,0)
	size:=uintptr(len(units)*2)
	handle:=call(win.globalAlloc,gmemMoveable,size)
	if handle==0 { return fmt.Errorf("windows: GlobalAlloc failed") }
	owned:=true
	defer func() { if owned { call(win.globalFree,handle) } }()
	address:=call(win.globalLock,handle)
	if address==0 { return fmt.Errorf("windows: GlobalLock failed") }
	copy(unsafe.Slice((*uint16)(unsafe.Pointer(address)),len(units)),units)
	call(win.globalUnlock,handle)
	runtime.KeepAlive(units)
	if call(win.emptyClipboard)==0 { return fmt.Errorf("windows: EmptyClipboard failed") }
	if call(win.setClipboardData,cfUnicodeText,handle)==0 {
		return fmt.Errorf("windows: SetClipboardData failed")
	}
	owned=false // ownership transferred to Windows
	return nil
}

// ClipboardText reads Unicode text from the system clipboard on the Run goroutine.
func (w *Window) ClipboardText() (string,error) {
	hwnd:=w.hwnd.Load()
	if hwnd==0 { return "",fmt.Errorf("windows: window is not running") }
	if call(win.openClipboard,hwnd)==0 { return "",fmt.Errorf("windows: OpenClipboard failed") }
	defer call(win.closeClipboard)
	handle:=call(win.getClipboardData,cfUnicodeText)
	if handle==0 { return "",nil }
	size:=call(win.globalSize,handle)
	if size>64*1024*1024 { return "",fmt.Errorf("windows: clipboard text is too large") }
	address:=call(win.globalLock,handle)
	if address==0 { return "",fmt.Errorf("windows: GlobalLock failed") }
	defer call(win.globalUnlock,handle)
	raw:=unsafe.Slice((*uint16)(unsafe.Pointer(address)),int(size/2))
	end:=0
	for end<len(raw) && raw[end]!=0 { end++ }
	return string(utf16.Decode(raw[:end])),nil
}
