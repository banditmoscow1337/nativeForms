//go:build darwin && (amd64 || arm64)

package darwin

import (
	"math"

	ui "github.com/banditmoscow1337/nativeForms"
	"github.com/ebitengine/purego/objc"
)

func normalizeDiscreteScrollDelta(d float64) float32 {
	if d == 0 {
		return 0
	}
	if math.Abs(d) < 1.0 {
		if d > 0 {
			return 1
		}
		return -1
	}
	return float32(math.Max(-6, math.Min(6, d)))
}

func (w *Window) event(event objc.ID, height int) {
	typ := msgSendUint0(event, selEventType)
	mods := msgSendUint0(event, selector("modifierFlags"))
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
		position := msgSendPoint0(event, selLocationInWindow)
		uiScale := w.uiScale()
		x, y := float32(position.X)/uiScale, float32(float64(height)-position.Y)/uiScale
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
			dx := msgSendFloat0(event, selector("scrollingDeltaX"))
			dy := msgSendFloat0(event, selector("scrollingDeltaY"))
			precise := msgSendBool0(event, selector("hasPreciseScrollingDeltas"))
			var sx, sy float32
			if precise {
				sx, sy = float32(dx/40.0), float32(dy/40.0)
			} else {
				sx = normalizeDiscreteScrollDelta(dx)
				sy = normalizeDiscreteScrollDelta(dy)
			}
			w.manager.HandleEvent(ui.Event{Type: ui.PointerScroll, X: x, Y: y, Scroll: ui.Vec2{X: sx, Y: sy}, Mods: flags})
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
		e := ui.Event{Type: kind, Key: key, Mods: flags, Repeat: msgSendBool0(event, selector("isARepeat"))}
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
