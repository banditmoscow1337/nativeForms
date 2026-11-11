package nativeforms

import (
	"math/rand"
	"reflect"
	"strings"
	"testing"
)

func TestTextFieldIncrementalWrapRandomEdits(t *testing.T) {
	rng := rand.New(rand.NewSource(391))
	field := NewTextArea("ab\r\ncd ef\n🙂", nil)
	spec := field.fontSpec()
	const width float32 = 35
	field.layoutFor(width, spec)
	values := []string{"", "x", " ", "\n", "\r", "\r\n", "аб", "🙂"}
	for step := 0; step < 500; step++ {
		e := field.Editor()
		n := len([]rune(e.Text))
		a := rng.Intn(n + 1)
		b := a + rng.Intn(n-a+1)
		e.Anchor, e.Caret = a, b
		if !e.Replace(values[rng.Intn(len(values))]) {
			continue
		}
		field.CommitEditor()
		got := field.layoutFor(width, spec)
		want := field.lines([]rune(field.Text), width, spec)
		if !reflect.DeepEqual(got.lines, want) || string(got.runes) != field.Text {
			t.Fatalf("step %d: incremental lines differ for %q", step, field.Text)
		}
	}
}

func TestTextFieldIncrementalWrapMatchesFullLayout(t *testing.T) {
	field := NewTextArea("alpha beta gamma\r\nnext line\nthird\n", nil)
	spec := field.fontSpec()
	for _, width := range []float32{42, 240} {
		field.layoutCache = nil
		field.layoutFor(width, spec)
		for _, edit := range []struct {
			a, b int
			text string
		}{
			{1, 1, "ZZ"}, {2, 4, "\n"}, {7, 7, "\r"}, {0, 3, ""},
			{len([]rune(field.Text)), len([]rune(field.Text)), "tail"},
		} {
			e := field.Editor()
			e.Anchor, e.Caret = edit.a, edit.b
			if !e.Replace(edit.text) {
				continue
			}
			before := field.layoutCache
			field.CommitEditor()
			got := field.layoutFor(width, spec)
			if got != before {
				t.Fatalf("width %v edit %+v rebuilt all lines", width, edit)
			}
			want := field.lines([]rune(field.Text), width, spec)
			if !reflect.DeepEqual(got.lines, want) || string(got.runes) != field.Text {
				t.Fatalf("width %v edit %+v: incremental lines differ from full layout", width, edit)
			}
		}
	}
}

func TestTextFieldScrollReusesLargeLineIndex(t *testing.T) {
	field := NewTextArea(strings.Repeat("visible row\n", 100000), nil)
	manager := New()
	manager.SetRoot(field)
	manager.BeginFrame(640, 480, 0)
	cache := field.layoutCache
	if cache == nil || len(cache.lines) != 100001 {
		t.Fatal("missing initial line index")
	}
	field.scrollY = 90000 * field.lineHeight(field.fontSpec())
	field.InvalidatePaint()
	manager.BeginFrame(640, 480, 0)
	if field.layoutCache != cache {
		t.Fatal("scroll rebuilt the document")
	}
	if len(manager.Frame().Commands) > 500 {
		t.Fatal("offscreen rows generated commands")
	}
}

func TestTextFieldCompositionReusesAdjustedLineIndex(t *testing.T) {
	field := NewTextArea(strings.Repeat("alpha beta\n", 10000), nil)
	manager := New()
	manager.SetRoot(field)
	manager.BeginFrame(640, 480, 0)
	field.editor.Anchor, field.editor.Caret = 7, 11
	field.Cursor = 11
	field.editor.Preedit = "日本\n語"
	_, displayed, lines, _ := field.compositionGeometry()
	want := []rune(field.Text)
	want = append(append(append([]rune(nil), want[:7]...), []rune(field.editor.Preedit)...), want[11:]...)
	if string(displayed) != string(want) {
		t.Fatalf("composition text differs: %q vs %q", string(displayed[:min(30, len(displayed))]), string(want[:min(30, len(want))]))
	}
	expected := field.lines(want, 620, field.fontSpec())
	if !reflect.DeepEqual(lines, expected) {
		for i := range lines {
			if i >= len(expected) || lines[i] != expected[i] {
				t.Fatalf("composition line %d of %d/%d: %+v vs %+v", i, len(lines), len(expected), lines[i], expected[i])
			}
		}
		t.Fatalf("composition has %d lines, expected %d", len(lines), len(expected))
	}
	cached := field.compositionCache
	field.compositionGeometry()
	if cached != field.compositionCache {
		t.Fatal("unchanged preedit rebuilt the document")
	}
}
