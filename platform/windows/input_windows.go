//go:build windows && (amd64 || arm64)

package windows

import (
	"fmt"
	"unicode"
	"unicode/utf16"
	"unsafe"

	ui "github.com/banditmoscow1337/nativeForms"
	"github.com/banditmoscow1337/nativeForms/platform"
)

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
		w.manager.SetBackingScale(w.dpiScale())
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
	case wmTrayCallback:
		if (uint32(lp) == wmLeftUp || uint32(lp) == wmRightUp) && w.app != nil {
			w.app.invokeTrayClick()
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


func (w *Window) mouse(hwnd uintptr, msg uint32, wp, lp uintptr) {
	if msg == wmMouseMove {
		tracking := trackMouseEvent{Size: uint32(unsafe.Sizeof(trackMouseEvent{})), Flags: 2, HWND: hwnd}
		call(win.trackMouse, pointer(&tracking))
	}
	p := point{X: int32(int16(lp & 0xffff)), Y: int32(int16((lp >> 16) & 0xffff))}
	if msg == wmMouseWheel || msg == wmMouseHWheel {
		call(win.screenToClient, hwnd, pointer(&p))
	}
	scale := 1 / w.scale()
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
	if key >= 0x70 && key <= 0x7B {
		return ui.FunctionKey(int(key-0x70) + 1)
	}
	if key >= '0' && key <= '9' {
		return ui.KeyDigit0 + ui.Key(key-'0')
	}
	if key >= 'A' && key <= 'Z' {
		return ui.LetterKey(rune(key))
	}
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
