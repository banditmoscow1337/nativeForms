//go:build darwin && (amd64 || arm64)

package darwin

import (
	"strings"
	"testing"

	ui "github.com/banditmoscow1337/nativeForms"
)

func TestEditorKeyTranslation(t *testing.T) {
	for _, tc := range []struct {
		code uint16
		want ui.Key
	}{
		{48, ui.KeyTab}, {116, ui.KeyPageUp}, {121, ui.KeyPageDown},
		{114, ui.KeyInsert}, {122, ui.KeyF1}, {98, ui.KeyF7},
		{101, ui.KeyF9}, {111, ui.KeyF12}, {29, ui.KeyDigit0},
		{25, ui.KeyDigit9}, {11, ui.KeyB}, {34, ui.KeyI},
		{27, ui.KeyMinus}, {33, ui.KeyLeftBracket},
		{42, ui.KeyBackslash}, {39, ui.KeyQuote}, {44, ui.KeySlash},
	} {
		if got := macKey(tc.code); got != tc.want {
			t.Errorf("mac key %d: got %v, want %v", tc.code, got, tc.want)
		}
	}
}

func TestUTF16RangeInWindow(t *testing.T) {
	value := "a🙂bc"
	start, end, ok := utf16RangeBytes(value, nsRange{Location: 1, Length: 2})
	if !ok || value[start:end] != "🙂" {
		t.Fatal("UTF-16 range did not include the complete supplementary rune")
	}
	if _, _, ok := utf16RangeBytes(value, nsRange{Location: 2, Length: 1}); ok {
		t.Fatal("UTF-16 range split a surrogate pair")
	}
}

func TestNativeReplacementUsesAbsoluteUTF16Coordinates(t *testing.T) {
	prefix := strings.Repeat("🙂", 9000)
	f := ui.NewTextField(prefix+"xy", nil)
	m := ui.New()
	m.SetRoot(ui.NewPanel())
	m.OpenPopup(f, ui.Rect{W: 220, H: 40}, true)
	e := f.Editor()
	e.Anchor, e.Caret = 9000, 9001
	f.Cursor = e.Caret
	w := &Window{manager: m}
	selection := w.committedSelection()
	if selection.Location != 18000 || selection.Length != 1 {
		t.Fatalf("selection was relative to IME window: %+v", selection)
	}
	w.nativeReplacement(nsRange{Location: 18001, Length: 1})
	if e.Anchor != 9001 || e.Caret != 9002 {
		t.Fatalf("absolute replacement selected the wrong runes: %d..%d", e.Anchor, e.Caret)
	}
}
