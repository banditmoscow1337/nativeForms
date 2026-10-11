//go:build linux

package linux

import (
	"testing"
	"unicode/utf8"

	ui "github.com/banditmoscow1337/nativeForms"
)

func TestX11EditorKeyTranslation(t *testing.T) {
	for _, tc := range []struct {
		sym  uint32
		want ui.Key
	}{
		{0xff09, ui.KeyTab}, {0xff55, ui.KeyPageUp}, {0xff56, ui.KeyPageDown},
		{0xff63, ui.KeyInsert}, {0xffbe, ui.KeyF1}, {0xffc4, ui.KeyF7},
		{0xffc6, ui.KeyF9}, {0xffc9, ui.KeyF12}, {'0', ui.KeyDigit0},
		{'!', ui.KeyDigit1}, {'9', ui.KeyDigit9}, {'B', ui.KeyB},
		{'i', ui.KeyI}, {'_', ui.KeyMinus}, {'{', ui.KeyLeftBracket},
		{'|', ui.KeyBackslash}, {'"', ui.KeyQuote}, {'?', ui.KeySlash},
	} {
		if got := x11Key(tc.sym); got != tc.want {
			t.Errorf("X11 keysym %#x: got %v, want %v", tc.sym, got, tc.want)
		}
	}
}

func TestSurroundingTextKeepsLocalOffsetsAndUTF8Boundaries(t *testing.T) {
	s := ui.TextInputState{Text: "a🙂bc🙂d", Cursor: len("a🙂bc"), Anchor: len("a🙂b")}
	text, cursor, anchor, ok := surroundingText(s, len(s.Text))
	if !ok || text != s.Text || cursor != s.Cursor || anchor != s.Anchor {
		t.Fatal("window smaller than limit was unexpectedly shortened")
	}
	text, cursor, anchor, ok = surroundingText(s, 7)
	if !ok || !utf8.ValidString(text) || text[anchor:cursor] != "c" || len(text) > 7 {
		t.Fatal("bounded surrounding text split UTF-8 or selection")
	}
}

func TestWaylandEditorKeyTranslation(t *testing.T) {
	for _, tc := range []struct {
		code uint32
		shift bool
		key ui.Key
		r rune
	}{
		{15, false, ui.KeyTab, 0}, {104, false, ui.KeyPageUp, 0},
		{109, false, ui.KeyPageDown, 0}, {110, false, ui.KeyInsert, 0},
		{59, false, ui.KeyF1, 0}, {65, false, ui.KeyF7, 0},
		{67, false, ui.KeyF9, 0}, {88, false, ui.KeyF12, 0},
		{11, false, ui.KeyDigit0, '0'}, {2, true, ui.KeyDigit1, '!'},
		{48, true, ui.KeyB, 'B'}, {23, false, ui.KeyI, 'i'},
		{12, true, ui.KeyMinus, '_'}, {26, true, ui.KeyLeftBracket, '{'},
		{43, true, ui.KeyBackslash, '|'}, {40, false, ui.KeyQuote, '\''},
		{53, true, ui.KeySlash, '?'},
	} {
		key, r := evdevUS(tc.code, tc.shift)
		if key != tc.key || r != tc.r {
			t.Errorf("evdev %d shift=%v: got (%v, %q), want (%v, %q)", tc.code, tc.shift, key, r, tc.key, tc.r)
		}
	}
}
