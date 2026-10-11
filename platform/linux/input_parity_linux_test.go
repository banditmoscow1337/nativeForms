//go:build linux

package linux

import (
	"testing"

	ui "github.com/banditmoscow1337/nativeForms"
)

func captureEvents() (*ui.Manager, *[]ui.Event) {
	m := ui.New()
	events := new([]ui.Event)
	m.SetUnhandledEventHandler(func(event ui.Event) bool {
		*events = append(*events, event)
		return true
	})
	return m, events
}

func TestX11RepeatAndPointerModifiers(t *testing.T) {
	m, events := captureEvents()
	w := NewX11(m, X11Options{})
	w.scale = 1
	w.keysyms = map[byte][]uint32{38: {'a'}}
	key := func(kind byte, timestamp uint32) {
		packet := make([]byte, 32)
		packet[0], packet[1] = kind, 38
		le.PutUint32(packet[4:], timestamp)
		w.handle(packet)
	}
	key(2, 100)
	key(3, 101)
	key(2, 101) // X11 auto-repeat pair.
	key(3, 102)
	key(2, 103) // A real press at a different timestamp.
	var downs []ui.Event
	for _, event := range *events {
		if event.Type == ui.KeyDown {
			downs = append(downs, event)
		}
	}
	if len(downs) != 3 || downs[0].Repeat || !downs[1].Repeat || downs[2].Repeat {
		t.Fatalf("X11 repeat flags: %+v", downs)
	}

	for _, kind := range []byte{6, 4} {
		packet := make([]byte, 32)
		packet[0], packet[1] = kind, 4 // Motion or wheel up.
		le.PutUint16(packet[28:], 1|4|8|64)
		w.handle(packet)
		got := (*events)[len(*events)-1]
		if got.Mods != ui.ModShift|ui.ModControl|ui.ModAlt|ui.ModSuper {
			t.Fatalf("X11 event %d lost modifiers: %+v", kind, got)
		}
	}
}

func TestWaylandPointerModifiers(t *testing.T) {
	m, events := captureEvents()
	w := NewWayland(m, WaylandOptions{})
	w.keyMods = 4 | 8 | 64
	w.pressed = map[uint32]bool{42: true}
	for _, input := range []struct {
		opcode uint16
		data   []byte
		kind   ui.EventType
	}{
		{2, make([]byte, 12), ui.PointerMove},
		{3, make([]byte, 16), ui.PointerDown},
		{4, make([]byte, 12), ui.PointerScroll},
	} {
		if input.opcode == 3 {
			le.PutUint32(input.data[8:], 0x110)
			le.PutUint32(input.data[12:], 1)
		}
		if err := w.pointerEvent(input.opcode, input.data); err != nil {
			t.Fatal(err)
		}
		got := (*events)[len(*events)-1]
		if got.Type != input.kind || got.Mods != ui.ModShift|ui.ModControl|ui.ModAlt|ui.ModSuper {
			t.Fatalf("Wayland event %d lost modifiers: %+v", input.opcode, got)
		}
	}
}
