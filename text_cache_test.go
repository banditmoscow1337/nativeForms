package nativeforms

import (
	"math/rand"
	"reflect"
	"strings"
	"testing"
)

func TestTextFieldIncrementalWrapMatchesColdLayout(t *testing.T) {
	field := NewTextArea("one two three\r\nfour five\nшесть🙂 семь\u2028end", nil)
	spec := FontSpec{Size: 14}
	width := float32(64)
	_, _ = field.cachedLines(width, spec)
	for _, text := range []string{
		"one two three\r\nfour FIVE\nшесть🙂 семь\u2028end",
		"one two three\r\nfour FIVE\nnew line\nшесть🙂 семь\u2028end",
		"one two three\r\nfour FIVE\nnew line\nшесть🙂 семь\u2028",
		"\r\nstart\nшесть🙂 семь\u2028",
		"",
		"a\n\n\r\n🙂\u2028b\n",
		"a\n\r\n🙂b\n",
	} {
		field.SetText(text)
		r, got := field.cachedLines(width, spec)
		want := field.lines([]rune(text), width, spec)
		if string(r) != text || !reflect.DeepEqual(got, want) {
			t.Fatalf("incremental wrap for %q: got %#v, want %#v", text, got, want)
		}
	}
	field.SetText("a\nline")
	_, got := field.cachedLines(20, spec)
	want := field.lines([]rune(field.Text), 20, spec)
	if !reflect.DeepEqual(got, want) {
		t.Fatal("width change did not rebuild wrap")
	}
}

func TestTextFieldIncrementalWrapManyEdits(t *testing.T) {
	rng := rand.New(rand.NewSource(17))
	field := NewTextArea("alpha\r\nbeta\n🙂 end\n", nil)
	spec := FontSpec{Size: 14}
	field.cachedLines(42, spec)
	characters := []rune{'a', ' ', '\n', '\r', '🙂', '\u2028'}
	for n := 0; n < 150; n++ {
		r := []rune(field.Text)
		at := rng.Intn(len(r) + 1)
		if len(r) > 0 && rng.Intn(3) == 0 {
			r = append(r[:at], r[min(len(r), at+1):]...)
		} else {
			r = append(r[:at], append([]rune{characters[rng.Intn(len(characters))]}, r[at:]...)...)
		}
		field.SetText(string(r))
		_, got := field.cachedLines(42, spec)
		want := field.lines(r, 42, spec)
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("step %d, text %q: got %#v, want %#v", n, field.Text, got, want)
		}
	}
}

func TestTextAreaPaintsOnlyVisibleLines(t *testing.T) {
	field := NewTextArea(strings.Repeat("row\n", 100000), nil)
	manager := New()
	manager.SetRoot(field)
	manager.BeginFrame(400, 300, 0)
	first := len(manager.Frame().Commands)
	if first > 500 {
		t.Fatalf("painted %d commands for offscreen lines", first)
	}
	field.scrollY = 80000
	manager.BeginFrame(400, 300, 0)
	if got := len(manager.Frame().Commands); got > 500 {
		t.Fatalf("scrolled paint emitted %d commands, first=%d", got, first)
	}
}

func TestFontLayoutCacheOwnershipAndLimit(t *testing.T) {
	fonts := NewFontContext()
	spec := FontSpec{Size: 14}
	line := fonts.Layout("abc", spec)
	bytes, entries := fonts.LayoutCacheUsage()
	if bytes <= 0 || entries != 1 {
		t.Fatalf("layout cache did not retain line: %d bytes, %d entries", bytes, entries)
	}
	line.Carets[0] = 999
	line.Glyphs[0].X = 999
	again := fonts.Layout("abc", spec)
	if again.Carets[0] == 999 || again.Glyphs[0].X == 999 {
		t.Fatal("public Layout exposed mutable cached slices")
	}
	fonts.SetLayoutCacheLimit(0)
	if bytes, entries := fonts.LayoutCacheUsage(); bytes != 0 || entries != 0 {
		t.Fatalf("layout cache did not evict: %d bytes, %d entries", bytes, entries)
	}
}
