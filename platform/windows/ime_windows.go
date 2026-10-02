//go:build windows && (amd64 || arm64)

package windows

import (
	"unicode/utf16"
	"unsafe"

	ui "github.com/banditmoscow1337/nativeForms"
)

func (w *Window) composition(hwnd,flags uintptr) {
	context:=call(win.immGetContext,hwnd)
	if context==0 { return }
	defer call(win.immReleaseContext,hwnd,context)
	read:=func(which uintptr) string {
		length:=int32(call(win.immGetComposition,context,which,0,0))
		if length<=0 || length>1024*1024 { return "" }
		units:=make([]uint16,(int(length)+1)/2)
		copied:=int32(call(win.immGetComposition,context,which,pointer(&units[0]),uintptr(len(units)*2)))
		keep(units)
		if copied<=0 { return "" }
		return string(utf16.Decode(units[:int(copied)/2]))
	}
	if flags&gcsResultStr!=0 {
		w.manager.HandleEvent(ui.Event{Type:ui.CompositionEnd,Text:read(gcsResultStr)})
	}
	if flags&gcsCompStr!=0 {
		w.manager.HandleEvent(ui.Event{Type:ui.CompositionUpdate,Text:read(gcsCompStr)})
	}
	w.placeCandidateWithContext(context)
}

func (w *Window) placeCandidate(hwnd uintptr) {
	context:=call(win.immGetContext,hwnd)
	if context==0 { return }
	w.placeCandidateWithContext(context)
	call(win.immReleaseContext,hwnd,context)
}

func (w *Window) placeCandidateWithContext(context uintptr) {
	caret,ok:=w.manager.FocusedCaretRect()
	if !ok { return }
	scale:=float32(w.dpi)/96
	x,y:=int32(caret.X*scale),int32(caret.Y*scale)
	form:=candidateForm{Style:cfsExclude,
		Position:point{X:x,Y:y+int32(caret.H*scale)},
		Area:rect{Left:x,Top:y,Right:x+2,Bottom:y+int32(caret.H*scale)}}
	call(win.immSetCandidate,context,uintptr(unsafe.Pointer(&form)))
	keep(&form)
}
