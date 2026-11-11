//go:build linux

package linux

import (
	"unicode"

	ui "github.com/banditmoscow1337/nativeForms"
)

func x11Mods(state uint16) int {
	mods := 0
	if state&1 != 0 {
		mods |= ui.ModShift
	}
	if state&4 != 0 {
		mods |= ui.ModControl
	}
	if state&8 != 0 {
		mods |= ui.ModAlt
	}
	if state&64 != 0 {
		mods |= ui.ModSuper
	}
	return mods
}


func letterKey(r rune) ui.Key {
	return ui.LetterKey(unicode.ToLower(r))
}

func x11Key(sym uint32) ui.Key {
	if sym >= 0xffbe && sym <= 0xffc9 {
		return ui.FunctionKey(int(sym-0xffbe) + 1)
	}
	if sym >= '0' && sym <= '9' {
		return ui.KeyDigit0 + ui.Key(sym-'0')
	}
	switch sym {
	case 0xff08:
		return ui.KeyBackspace
	case 0xff09:
		return ui.KeyTab
	case 0xff0d:
		return ui.KeyEnter
	case 0xff1b:
		return ui.KeyEscape
	case 0xffff:
		return ui.KeyDelete
	case 0xff50:
		return ui.KeyHome
	case 0xff57:
		return ui.KeyEnd
	case 0xff55:
		return ui.KeyPageUp
	case 0xff56:
		return ui.KeyPageDown
	case 0xff63:
		return ui.KeyInsert
	case 0xff51:
		return ui.KeyLeftArrow
	case 0xff52:
		return ui.KeyUpArrow
	case 0xff53:
		return ui.KeyRightArrow
	case 0xff54:
		return ui.KeyDownArrow
	case 0x20:
		return ui.KeySpace
	case ')':
		return ui.KeyDigit0
	case '!':
		return ui.KeyDigit1
	case '@':
		return ui.KeyDigit2
	case '#':
		return ui.KeyDigit3
	case '$':
		return ui.KeyDigit4
	case '%':
		return ui.KeyDigit5
	case '^':
		return ui.KeyDigit6
	case '&':
		return ui.KeyDigit7
	case '*':
		return ui.KeyDigit8
	case '(':
		return ui.KeyDigit9
	case '-', '_':
		return ui.KeyMinus
	case '=', '+':
		return ui.KeyEqual
	case '[', '{':
		return ui.KeyLeftBracket
	case ']', '}':
		return ui.KeyRightBracket
	case '\\', '|':
		return ui.KeyBackslash
	case ';', ':':
		return ui.KeySemicolon
	case '\'', '"':
		return ui.KeyQuote
	case '`', '~':
		return ui.KeyBackquote
	case ',', '<':
		return ui.KeyComma
	case '.', '>':
		return ui.KeyPeriod
	case '/', '?':
		return ui.KeySlash
	}
	return letterKey(rune(sym))
}

func (w *X11Window) handle(message []byte) bool {
	typ := message[0] & 0x7f
	// X11 autorepeat sends an adjacent KeyRelease/KeyPress pair with the same
	// keycode and timestamp. Preserve repeat information for controls and
	// shortcuts that should activate only once per held key.
	repeat := typ == 2 && w.hadRelease && message[1] == w.lastReleaseCode &&
		le.Uint32(message[4:]) == w.lastReleaseTime
	w.hadRelease = typ == 3
	if w.hadRelease {
		w.lastReleaseCode = message[1]
		w.lastReleaseTime = le.Uint32(message[4:])
	}
	if w.modal && typ >= 2 && typ <= 8 {
		return false
	}
	switch typ {
	case 2, 3:
		syms := w.keysyms[message[1]]
		if len(syms) == 0 {
			return false
		}
		state := le.Uint16(message[28:])
		group := int((state >> 13) & 3)
		base := group * 2
		if state&128 != 0 && len(syms) > 2 {
			base += 2
		}
		if base >= len(syms) || syms[base] == 0 {
			base = 0
		}
		sym := syms[base]
		upper := uint32(0)
		if base+1 < len(syms) {
			upper = syms[base+1]
		}
		shift := state&1 != 0
		if state&2 != 0 && unicode.IsLetter(keysymRune(sym)) {
			shift = !shift
		}
		if state&16 != 0 && upper >= 0xffb0 && upper <= 0xffb9 {
			shift = !shift
		}
		if shift {
			if upper != 0 {
				sym = upper
			} else if r := keysymRune(sym); unicode.IsLetter(r) {
				sym = uint32(unicode.ToUpper(r)) | 0x1000000
			}
		}
		if w.ime != nil && w.ime.key(sym, uint32(message[1]-8), uint32(state), typ == 2) {
			return false
		}

		kind := ui.KeyDown
		if typ == 3 {
			kind = ui.KeyUp
		}
		mods := x11Mods(state)
		if typ == 2 && w.compose.node != nil && (sym == 0xff08 || sym == 0xff1b) {
			w.compose.input(w.manager, sym)
			return false
		}
		if mods&(ui.ModControl|ui.ModAlt|ui.ModSuper) != 0 {
			w.compose.reset(w.manager)
		}
		key := x11Key(sym)
		if mods&(ui.ModControl|ui.ModAlt|ui.ModSuper) != 0 {
			key = x11Key(syms[0])
			if key == ui.KeyUnknown {
				key, _ = evdevUS(uint32(message[1]-8), false)
			}
		}
		w.manager.HandleEvent(ui.Event{Type: kind, Key: key, Mods: mods, Repeat: repeat})
		if typ == 2 && mods&(ui.ModControl|ui.ModAlt|ui.ModSuper) == 0 {
			w.compose.input(w.manager, sym)
		}

	case 4, 5, 6:
		if typ == 4 {
			w.compose.reset(w.manager)
		}
		w.screenX = int(int16(le.Uint16(message[20:]))) - int(int16(le.Uint16(message[24:])))
		w.screenY = int(int16(le.Uint16(message[22:]))) - int(int16(le.Uint16(message[26:])))
		effScale := w.effectiveScale()
		x, y := float32(int16(le.Uint16(message[24:])))/effScale, float32(int16(le.Uint16(message[26:])))/effScale
		mods := x11Mods(le.Uint16(message[28:]))
		if typ == 6 {
			w.manager.HandleEvent(ui.Event{Type: ui.PointerMove, X: x, Y: y, Mods: mods})
			break
		}
		button := message[1]
		if button == 4 || button == 5 || button == 6 || button == 7 {
			if typ == 4 {
				scroll := ui.Vec2{}
				switch button {
				case 4:
					scroll.Y = 1
				case 5:
					scroll.Y = -1
				case 6:
					scroll.X = 1
				case 7:
					scroll.X = -1
				}
				w.manager.HandleEvent(ui.Event{Type: ui.PointerScroll, X: x, Y: y, Scroll: scroll, Mods: mods})
			}
			break
		}
		mouse := ui.MouseLeft
		if button == 2 {
			mouse = ui.MouseMiddle
		}
		if button == 3 {
			mouse = ui.MouseRight
		}
		kind := ui.PointerDown
		if typ == 5 {
			kind = ui.PointerUp
		}
		w.manager.HandleEvent(ui.Event{Type: kind, Button: mouse, X: x, Y: y, Mods: mods})
	case 10:
		w.compose.reset(w.manager)
		if w.ime != nil {
			w.ime.focusOut()
		}
		w.manager.ClearInteraction()
	case 34:
		if message[4] == 1 {
			if err := w.readKeyboardMapping(); err != nil {
				w.clipboardError = err
			}
		}
	case 28:
		if err := w.selectionProgress(message); err != nil {
			w.clipboardError = err
		}
	case 29: // SelectionClear
		if le.Uint32(message[12:]) == w.clipboard {
			w.clipboardOwned = false
			w.clipboardText = ""
		}
	case 30: // SelectionRequest
		if err := w.selectionRequest(message); err != nil {
			w.clipboardError = err
		}
	case 12:
		w.stagingValid = false // Expose needs a full upload even if UI pixels match
		return true
	case 22:
		w.originDirty = true
		if message[0]&128 != 0 {
			w.screenX = int(int16(le.Uint16(message[16:])))
			w.screenY = int(int16(le.Uint16(message[18:])))
		}
		width, height := int(le.Uint16(message[20:])), int(le.Uint16(message[22:]))
		if width != w.width || height != w.height {
			w.width, w.height = width, height
			w.manager.InvalidateLayout()
			return true
		}
	case 33:
		if message[1] == 32 && le.Uint32(message[8:]) == w.wmProtocols && le.Uint32(message[12:]) == w.wmDelete {
			w.closeAction()
		} else {
			w.handleDND(message)
		}
	}
	return false
}

// IBus expects screen coordinates. WM-reparented X11 windows may report
// ConfigureNotify positions relative to their decoration frame instead.
