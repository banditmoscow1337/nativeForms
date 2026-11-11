//go:build linux

package linux

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"time"
	"unicode"

	ui "github.com/banditmoscow1337/nativeForms"
)

func (w *WaylandWindow) handle(object uint32, opcode uint16, payload []byte) error {
	u32 := func(offset int) uint32 {
		if offset+4 > len(payload) {
			return 0
		}
		return binary.LittleEndian.Uint32(payload[offset:])
	}
	switch object {
	case 1:
		if opcode == 0 {
			return fmt.Errorf("wayland: compositor protocol error (%d): %s", u32(4), string(payload[min(12, len(payload)):]))
		}
	case 2:
		if opcode == 1 {
			for id, output := range w.outputs {
				if output.global == u32(0) {
					delete(w.outputs, id)
					if err := w.updateScale(); err != nil {
						return err
					}
				}
			}
			switch w.globals[u32(0)] {
			case "wl_compositor", "wl_shm", "xdg_wm_base":
				return fmt.Errorf("wayland: required global was removed")
			}
		}
	case w.wm:
		if opcode == 0 {
			return w.wire.send(w.wm, 3, wlWord(u32(0)))
		}
	case w.surface:
		if (opcode == 0 || opcode == 1) && len(payload) >= 4 {
			if output := w.outputs[u32(0)]; output != nil {
				output.entered = opcode == 0
				return w.updateScale()
			}
		}
	case w.shm:
		// wl_shm.format is informational; ARGB8888 is mandatory.
	case w.fractional:
		if opcode == 0 && len(payload) >= 4 {
			preferred := float32(u32(0)) / 120
			if preferred >= 0.5 && preferred <= 8 {
				w.preferredScale = preferred
				return w.updateScale()
			}
		}
	case w.toplevel:
		if opcode == 1 {
			w.closeAction()
			return nil
		}
		if opcode == 0 && len(payload) >= 8 {
			width, height := int(int32(u32(0))), int(int32(u32(4)))
			if width > 0 && height > 0 && (width != w.width || height != w.height) {
				w.width, w.height = width, height
				w.Manager.InvalidateLayout()
			}
		}
	case w.xdg:
		if opcode == 0 && len(payload) >= 4 {
			if err := w.wire.send(w.xdg, 4, wlWord(u32(0))); err != nil {
				return err
			}
			w.ready = !w.hidden.Load()
			w.Manager.InvalidatePaint()
		}
	case w.seat:
		if opcode == 0 && len(payload) >= 4 {
			capabilities := u32(0)
			if capabilities&2 == 0 {
				w.repeatCode = 0
				w.pressed = make(map[uint32]bool)
				w.compose.reset(w.Manager)
				w.Manager.ClearInteraction()
			}
			if capabilities&1 != 0 && w.pointer == 0 {
				w.pointer = w.wire.id()
				if err := w.wire.send(w.seat, 0, wlWord(w.pointer)); err != nil {
					return err
				}
			}
			if capabilities&2 != 0 && w.keyboard == 0 {
				w.keyboard = w.wire.id()
				if err := w.wire.send(w.seat, 1, wlWord(w.keyboard)); err != nil {
					return err
				}
			}
		}
	case w.pointer:
		if opcode == 3 && len(payload) >= 4 {
			w.serial = u32(0)
		}
		if !w.modal {
			return w.pointerEvent(opcode, payload)
		}
		return nil
	case w.keyboard:
		if (opcode == 1 || opcode == 3) && len(payload) >= 4 {
			w.serial = u32(0)
		}
		return w.keyboardEvent(opcode, payload)
	case w.textInput:
		return w.textInputEvent(opcode, payload)
	case w.dataDevice:
		return w.dataDeviceEvent(opcode, payload)
	default:
		if output := w.outputs[object]; output != nil {
			if opcode == 3 && len(payload) >= 4 {
				scale := int(int32(u32(0)))
				if scale >= 1 && scale <= 8 {
					output.scale = scale
					return w.updateScale()
				}
			}
			return nil
		}
		if _, ok := w.dataSources[object]; ok {
			return w.dataSourceEvent(object, opcode, payload)
		}
		if _, ok := w.dataOffers[object]; ok {
			return w.dataOfferEvent(object, opcode, payload)
		}
		if buffer := w.buffers[object]; buffer != nil && opcode == 0 {
			buffer.busy = false
			if buffer.retired {
				w.destroyBuffer(buffer)
			}
		}
	}
	return nil
}


func (w *WaylandWindow) pointerEvent(opcode uint16, p []byte) error {
	u := func(offset int) uint32 {
		if offset+4 > len(p) {
			return 0
		}
		return binary.LittleEndian.Uint32(p[offset:])
	}
	switch opcode {
	case 0, 2:
		offset := 4
		if opcode == 0 {
			offset = 8
		}
		if len(p) < offset+8 {
			return fmt.Errorf("wayland: short pointer event")
		}
		uiScale := w.uiScale()
		w.pointerPos = ui.Vec2{X: float32(int32(u(offset))) / 256 / uiScale, Y: float32(int32(u(offset+4))) / 256 / uiScale}
		w.Manager.HandleEvent(ui.Event{Type: ui.PointerMove, X: w.pointerPos.X, Y: w.pointerPos.Y, Mods: w.modifiers()})
	case 1:
		w.Manager.HandleEvent(ui.Event{Type: ui.PointerMove, X: -1e6, Y: -1e6, Mods: w.modifiers()})
	case 3:
		if len(p) < 16 {
			return fmt.Errorf("wayland: short pointer button")
		}
		button := ui.MouseLeft
		switch u(8) {
		case 0x110:
			button = ui.MouseLeft
		case 0x111:
			button = ui.MouseRight
		case 0x112:
			button = ui.MouseMiddle
		default:
			return nil
		}
		kind := ui.PointerDown
		if u(12) == 0 {
			kind = ui.PointerUp
		}
		w.Manager.HandleEvent(ui.Event{Type: kind, Button: button, X: w.pointerPos.X, Y: w.pointerPos.Y, Mods: w.modifiers()})
	case 4:
		if len(p) < 12 {
			return fmt.Errorf("wayland: short pointer axis")
		}
		value := -float32(int32(u(8))) / 256 / 40
		scroll := ui.Vec2{}
		if u(4) == 0 {
			scroll.Y = value
		} else {
			scroll.X = value
		}
		w.Manager.HandleEvent(ui.Event{Type: ui.PointerScroll, X: w.pointerPos.X, Y: w.pointerPos.Y, Scroll: scroll, Mods: w.modifiers()})
	}
	return nil
}

func (w *WaylandWindow) keyboardEvent(opcode uint16, p []byte) error {
	u := func(offset int) uint32 {
		if offset+4 > len(p) {
			return 0
		}
		return binary.LittleEndian.Uint32(p[offset:])
	}
	switch opcode {
	case 0:
		fd, err := w.wire.takeFD()
		if err != nil {
			return err
		}
		file := os.NewFile(uintptr(fd), "wayland-keymap")
		defer file.Close()
		w.keymap = nil
		if len(p) < 8 || u(0) != 1 || u(4) == 0 || u(4) > 4<<20 {
			return nil
		}
		data := make([]byte, u(4))
		if _, err := io.ReadFull(file, data); err != nil {
			return fmt.Errorf("wayland: read keymap: %w", err)
		}
		w.keymap = parseWaylandKeymap(data)
	case 1: // enter: keys already held do not generate text or repeat.
		w.pressed = make(map[uint32]bool)
		w.repeatCode = 0
		if len(p) >= 12 {
			n := int(u(8))
			if n <= len(p)-12 {
				for i := 12; i+4 <= 12+n; i += 4 {
					w.pressed[le.Uint32(p[i:])] = true
				}
			}
		}
	case 2:
		w.pressed = make(map[uint32]bool)
		w.repeatCode = 0
		w.compose.reset(w.Manager)
		w.Manager.ClearInteraction()
	case 3:
		if len(p) < 16 {
			return fmt.Errorf("wayland: short keyboard key")
		}
		code, down := u(8), u(12) != 0
		w.pressed[code] = down
		if !w.modal {
			w.dispatchKey(code, down, false)
		}
		if !down && code == w.repeatCode {
			w.repeatCode = 0
		}
		if down && !w.modal && w.repeatRate > 0 {
			repeat := code != 42 && code != 54 && code != 29 && code != 97 && code != 56 && code != 100 && code != 125 && code != 126 && code != 58 && code != 69
			if w.keymap != nil {
				repeat = w.keymap[code+8].repeat
			}
			if repeat {
				w.repeatCode = code
				w.repeatAt = time.Now().Add(time.Duration(w.repeatDelay) * time.Millisecond)
			}
		}
	case 4:
		if len(p) >= 20 {
			w.keyMods = u(4) | u(8) | u(12)
			w.keyGroup = u(16)
		}
	case 5:
		if len(p) >= 8 {
			w.repeatRate = max(0, min(1000, int(int32(u(0)))))
			w.repeatDelay = max(0, int(int32(u(4))))
			if w.repeatRate == 0 {
				w.repeatCode = 0
			}
		}

	}
	return nil
}


func evdevUS(code uint32, shift bool) (ui.Key, rune) {
	if code >= 59 && code <= 68 {
		return ui.FunctionKey(int(code-59) + 1), 0
	}
	switch code {
	case 1:
		return ui.KeyEscape, 0
	case 14:
		return ui.KeyBackspace, 0
	case 15:
		return ui.KeyTab, 0
	case 28:
		return ui.KeyEnter, 0
	case 57:
		return ui.KeySpace, ' '
	case 87:
		return ui.KeyF11, 0
	case 88:
		return ui.KeyF12, 0
	case 102:
		return ui.KeyHome, 0
	case 103:
		return ui.KeyUpArrow, 0
	case 104:
		return ui.KeyPageUp, 0
	case 105:
		return ui.KeyLeftArrow, 0
	case 106:
		return ui.KeyRightArrow, 0
	case 107:
		return ui.KeyEnd, 0
	case 108:
		return ui.KeyDownArrow, 0
	case 109:
		return ui.KeyPageDown, 0
	case 110:
		return ui.KeyInsert, 0
	case 111:
		return ui.KeyDelete, 0
	}
	if letter := waylandUSLetters[code]; letter != 0 {
		key := letterKey(letter)
		if shift {
			letter = unicode.ToUpper(letter)
		}
		return key, letter
	}
	if pair, ok := waylandUSPunctuation[code]; ok {
		key := waylandUSPunctuationKeys[code]
		if shift {
			return key, pair[1]
		}
		return key, pair[0]
	}
	return ui.KeyUnknown, 0
}

var waylandUSLetters = map[uint32]rune{16: 'q', 17: 'w', 18: 'e', 19: 'r', 20: 't', 21: 'y', 22: 'u', 23: 'i', 24: 'o', 25: 'p', 30: 'a', 31: 's', 32: 'd', 33: 'f', 34: 'g', 35: 'h', 36: 'j', 37: 'k', 38: 'l', 44: 'z', 45: 'x', 46: 'c', 47: 'v', 48: 'b', 49: 'n', 50: 'm'}

var waylandUSPunctuation = map[uint32][2]rune{
	2: {'1', '!'}, 3: {'2', '@'}, 4: {'3', '#'}, 5: {'4', '$'}, 6: {'5', '%'},
	7: {'6', '^'}, 8: {'7', '&'}, 9: {'8', '*'}, 10: {'9', '('}, 11: {'0', ')'},
	12: {'-', '_'}, 13: {'=', '+'}, 26: {'[', '{'}, 27: {']', '}'}, 39: {';', ':'},
	40: {'\'', '"'}, 41: {'`', '~'}, 43: {'\\', '|'}, 51: {',', '<'}, 52: {'.', '>'}, 53: {'/', '?'},
}

var waylandUSPunctuationKeys = map[uint32]ui.Key{
	2: ui.KeyDigit1, 3: ui.KeyDigit2, 4: ui.KeyDigit3, 5: ui.KeyDigit4, 6: ui.KeyDigit5,
	7: ui.KeyDigit6, 8: ui.KeyDigit7, 9: ui.KeyDigit8, 10: ui.KeyDigit9, 11: ui.KeyDigit0,
	12: ui.KeyMinus, 13: ui.KeyEqual, 26: ui.KeyLeftBracket, 27: ui.KeyRightBracket,
	39: ui.KeySemicolon, 40: ui.KeyQuote, 41: ui.KeyBackquote, 43: ui.KeyBackslash,
	51: ui.KeyComma, 52: ui.KeyPeriod, 53: ui.KeySlash,
}

func (w *WaylandWindow) modifiers() int {
	mods := x11Mods(uint16(w.keyMods))
	if w.pressed[42] || w.pressed[54] {
		mods |= ui.ModShift
	}
	return mods
}

func (w *WaylandWindow) dispatchKey(code uint32, down, repeat bool) {
	mods := w.modifiers()
	key, r := evdevUS(code, mods&ui.ModShift != 0)
	sym := uint32(r) | 0x1000000
	if w.keymap != nil {
		sym = w.keymap.symbol(code, w.keyGroup, w.keyMods)
	}
	kind := ui.KeyDown
	if !down {
		kind = ui.KeyUp
	}
	if down && w.compose.node != nil && (sym == 0xff08 || sym == 0xff1b) {
		w.compose.input(w.Manager, sym)
		return
	}
	if mods&(ui.ModControl|ui.ModAlt|ui.ModSuper) != 0 {
		w.compose.reset(w.Manager)
	}
	w.Manager.HandleEvent(ui.Event{Type: kind, Key: key, Mods: mods, Repeat: repeat})

	if down && mods&(ui.ModControl|ui.ModAlt|ui.ModSuper) == 0 {
		if w.keymap != nil {
			for _, s := range w.keymap.symbols(code, w.keyGroup, w.keyMods) {
				w.compose.input(w.Manager, s)
			}
		} else {
			w.compose.input(w.Manager, sym)
		}
	}
}
