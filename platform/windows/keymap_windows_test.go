//go:build windows && (amd64 || arm64)

package windows

import (
	"testing"

	ui "github.com/banditmoscow1337/nativeForms"
)

func TestEditorKeyTranslation(t *testing.T) {
	for _, tc := range []struct {
		vk   uint32
		want ui.Key
	}{
		{0x09, ui.KeyTab}, {0x21, ui.KeyPageUp}, {0x22, ui.KeyPageDown},
		{0x2D, ui.KeyInsert}, {0x70, ui.KeyF1}, {0x76, ui.KeyF7},
		{0x78, ui.KeyF9}, {0x7B, ui.KeyF12}, {'0', ui.KeyDigit0},
		{'9', ui.KeyDigit9}, {0x60, ui.KeyDigit0}, {0x69, ui.KeyDigit9},
		{'B', ui.KeyB}, {'I', ui.KeyI}, {0x11, ui.KeyControl},
		{0xA2, ui.KeyControl}, {0xA3, ui.KeyControl},
		{0xBD, ui.KeyMinus}, {0x6D, ui.KeyMinus},
		{0xBB, ui.KeyEqual}, {0x6B, ui.KeyEqual},
		{0xDB, ui.KeyLeftBracket}, {0xDC, ui.KeyBackslash},
		{0xDE, ui.KeyQuote}, {0xBF, ui.KeySlash}, {0x6F, ui.KeySlash},
		{0xBE, ui.KeyPeriod}, {0x6E, ui.KeyPeriod},
	} {
		if got := translateKey(tc.vk); got != tc.want {
			t.Errorf("VK %#x: got %v, want %v", tc.vk, got, tc.want)
		}
	}
}

func TestWindowFramelessAndWindowControlsStyles(t *testing.T) {
	mgr := ui.New()
	defer mgr.Close()
	w := New(mgr, Options{Width: 800, Height: 600})

	if w.Frameless() || !w.WindowControlsVisible() {
		t.Fatalf("unexpected default window state: frameless=%v controls=%v", w.Frameless(), w.WindowControlsVisible())
	}
	normalStyle := w.windowStyle()
	if normalStyle&wsCaption == 0 || normalStyle&wsSysMenu == 0 || normalStyle&wsThickFrame == 0 {
		t.Fatalf("normal windowStyle=%#x missing caption, sysmenu or thickframe", normalStyle)
	}

	w.SetWindowControls(false)
	if w.Frameless() || w.WindowControlsVisible() {
		t.Fatalf("SetWindowControls(false) failed: frameless=%v controls=%v", w.Frameless(), w.WindowControlsVisible())
	}
	noControlsStyle := w.windowStyle()
	if noControlsStyle&wsCaption == 0 || noControlsStyle&wsThickFrame == 0 || noControlsStyle&wsSysMenu != 0 {
		t.Fatalf("noControlsStyle=%#x should keep caption & thickframe and clear wsSysMenu", noControlsStyle)
	}

	w.SetFrameless(true)
	if !w.Frameless() || w.WindowControlsVisible() {
		t.Fatalf("SetFrameless(true) failed: frameless=%v controls=%v", w.Frameless(), w.WindowControlsVisible())
	}
	framelessStyle := w.windowStyle()
	if framelessStyle&wsPopup == 0 || framelessStyle&wsCaption != 0 || framelessStyle&wsThickFrame != 0 {
		t.Fatalf("framelessStyle=%#x should use wsPopup without wsCaption or wsThickFrame", framelessStyle)
	}

	w.SetFrameless(false)
	w.SetWindowControls(true)
	if w.Frameless() || !w.WindowControlsVisible() || w.windowStyle() != normalStyle {
		t.Fatalf("restored windowStyle=%#x, want %#x", w.windowStyle(), normalStyle)
	}
}
