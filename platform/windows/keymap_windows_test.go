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
		{'9', ui.KeyDigit9}, {'B', ui.KeyB}, {'I', ui.KeyI},
		{0xBD, ui.KeyMinus}, {0xDB, ui.KeyLeftBracket},
		{0xDC, ui.KeyBackslash}, {0xDE, ui.KeyQuote}, {0xBF, ui.KeySlash},
	} {
		if got := translateKey(tc.vk); got != tc.want {
			t.Errorf("VK %#x: got %v, want %v", tc.vk, got, tc.want)
		}
	}
}
