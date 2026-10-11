package editor

import (
	"errors"
	"strings"
	"testing"
)

func testDocument(t *testing.T, text string) *Document {
	t.Helper()
	d, err := NewDocument(text)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func TestDocumentBatchHistoryAndVersions(t *testing.T) {
	d := testDocument(t, "zero\none\n")
	if d.LineCount() != 3 || d.Version() != 1 {
		t.Fatalf("initial state: lines=%d version=%d", d.LineCount(), d.Version())
	}

	// Inputs may arrive out of order; all ranges refer to the old document.
	change, err := d.Apply([]Edit{{Start: 5, End: 8, Text: "two"}, {Start: 0, End: 4, Text: "ONE"}})
	if err != nil {
		t.Fatal(err)
	}
	if got := d.Text(); got != "ONE\ntwo\n" {
		t.Fatalf("after batch: %q", got)
	}
	if change.BeforeVersion != 1 || change.Version != 2 || len(change.Edits) != 2 || change.Edits[0].Start != 0 {
		t.Fatalf("batch change: %+v", change)
	}

	undone, ok := d.Undo()
	if !ok || d.Text() != "zero\none\n" || d.Version() != 3 {
		t.Fatalf("undo: ok=%t text=%q version=%d", ok, d.Text(), d.Version())
	}
	if undone.BeforeVersion != 2 || undone.Version != 3 || len(undone.Edits) != 2 ||
		undone.Edits[0] != (Edit{Start: 0, End: 3, Text: "zero"}) ||
		undone.Edits[1] != (Edit{Start: 4, End: 7, Text: "one"}) {
		t.Fatalf("undo sync edits: %+v", undone)
	}

	redone, ok := d.Redo()
	if !ok || d.Text() != "ONE\ntwo\n" || d.Version() != 4 || redone.Edits[1].Start != 5 {
		t.Fatalf("redo: ok=%t text=%q version=%d change=%+v", ok, d.Text(), d.Version(), redone)
	}
	d.Undo()
	if _, err := d.Apply([]Edit{{Start: 0, End: 0, Text: "!"}}); err != nil {
		t.Fatal(err)
	}
	if _, ok := d.Redo(); ok {
		t.Fatal("new edit must discard redo history")
	}
}

func TestDocumentSameOffsetInsertions(t *testing.T) {
	d := testDocument(t, "abcd")
	_, err := d.Apply([]Edit{
		{Start: 1, End: 1, Text: "A"},
		{Start: 1, End: 1, Text: "B"},
		{Start: 1, End: 3, Text: "R"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := d.Text(); got != "aABRd" {
		t.Fatalf("same-offset edit order: %q", got)
	}
	change, ok := d.Undo()
	if !ok || d.Text() != "abcd" || len(change.Edits) != 3 {
		t.Fatalf("undo same-offset edits: ok=%t text=%q change=%+v", ok, d.Text(), change)
	}
	if change.Edits[0].Start != 1 || change.Edits[1].Start != 2 || change.Edits[2].Start != 3 {
		t.Fatalf("undo offsets in resulting document: %+v", change.Edits)
	}
}

func TestDocumentLinesAndUTF16(t *testing.T) {
	d := testDocument(t, "a😀b\r\nc\u0301\n")
	if d.LineCount() != 3 {
		t.Fatalf("line count: %d", d.LineCount())
	}
	for _, tc := range []struct {
		line, start, end int
		want             string
	}{
		{0, 0, 6, "a😀b"},
		{1, 8, 11, "c\u0301"},
		{2, 12, 12, ""},
	} {
		start, end, err := d.LineRange(tc.line)
		if err != nil || start != tc.start || end != tc.end {
			t.Fatalf("line %d: range=(%d,%d) err=%v", tc.line, start, end, err)
		}
		text, err := d.Slice(start, end)
		if err != nil || text != tc.want {
			t.Fatalf("line %d: text=%q err=%v", tc.line, text, err)
		}
	}
	for _, tc := range []struct {
		offset int
		pos    Position
	}{
		{0, Position{0, 0}},
		{1, Position{0, 1}},
		{5, Position{0, 3}},
		{6, Position{0, 4}},
		{8, Position{1, 0}},
		{11, Position{1, 2}},
		{12, Position{2, 0}},
	} {
		got, err := d.UTF16Position(tc.offset)
		if err != nil || got != tc.pos {
			t.Fatalf("offset %d: pos=%+v err=%v", tc.offset, got, err)
		}
		back, err := d.ByteOffset(tc.pos)
		if err != nil || back != tc.offset {
			t.Fatalf("position %+v: offset=%d err=%v", tc.pos, back, err)
		}
	}
	if _, err := d.ByteOffset(Position{Line: 0, Character: 2}); !errors.Is(err, ErrPosition) {
		t.Fatalf("half surrogate pair: %v", err)
	}
	if _, err := d.UTF16Position(7); !errors.Is(err, ErrPosition) {
		t.Fatalf("inside CRLF: %v", err)
	}
	if _, err := d.UTF16Position(2); !errors.Is(err, ErrRange) {
		t.Fatalf("inside UTF-8 rune: %v", err)
	}
	if _, err := d.ByteOffset(Position{Line: 1, Character: 3}); !errors.Is(err, ErrPosition) {
		t.Fatalf("past line end: %v", err)
	}

	if _, err := d.Apply([]Edit{{Start: 1, End: 5, Text: "λ"}}); err != nil {
		t.Fatal(err)
	}
	if start, end, err := d.LineRange(1); err != nil || start != 6 || end != 9 {
		t.Fatalf("line index after edit: range=(%d,%d) err=%v", start, end, err)
	}
	if _, ok := d.Undo(); !ok {
		t.Fatal("missing undo")
	}
	if start, _, err := d.LineRange(1); err != nil || start != 8 {
		t.Fatalf("line index after undo: start=%d err=%v", start, err)
	}
}

func TestDocumentRejectsInvalidEditsAtomically(t *testing.T) {
	d := testDocument(t, "a😀b\n")
	for _, tc := range []struct {
		edits []Edit
		want  error
	}{
		{[]Edit{{Start: 2, End: 5, Text: "x"}}, ErrRange},
		{[]Edit{{Start: -1, End: 0, Text: "x"}}, ErrRange},
		{[]Edit{{Start: 0, End: 0, Text: string([]byte{0xff})}}, ErrInvalidUTF8},
		{[]Edit{{Start: 0, End: 5, Text: "a"}, {Start: 1, End: 6, Text: "b"}}, ErrOverlap},
		{[]Edit{{Start: 0, End: 0, Text: "x"}, {Start: 20, End: 20, Text: "y"}}, ErrRange},
	} {
		if _, err := d.Apply(tc.edits); !errors.Is(err, tc.want) {
			t.Fatalf("edits %+v: got %v, want %v", tc.edits, err, tc.want)
		}
		if d.Text() != "a😀b\n" || d.Version() != 1 {
			t.Fatalf("failed edit changed document: text=%q version=%d", d.Text(), d.Version())
		}
	}
	if _, err := NewDocument(string([]byte{0xff})); !errors.Is(err, ErrInvalidUTF8) {
		t.Fatalf("invalid initial document: %v", err)
	}
}

func TestDocumentNoOpAndHistoryLimit(t *testing.T) {
	d := testDocument(t, "abc")
	if c, err := d.Apply([]Edit{{Start: 0, End: 3, Text: "abc"}}); err != nil || c.Version != 1 {
		t.Fatalf("no-op: change=%+v err=%v", c, err)
	}
	d.HistoryLimit = 1
	for _, edit := range []Edit{{Start: 3, End: 3, Text: "d"}, {Start: 4, End: 4, Text: "e"}} {
		if _, err := d.Apply([]Edit{edit}); err != nil {
			t.Fatal(err)
		}
	}
	if _, ok := d.Undo(); !ok || d.Text() != "abcd" {
		t.Fatalf("one retained undo: ok=%t text=%q", ok, d.Text())
	}
	if _, ok := d.Undo(); ok {
		t.Fatal("history limit was not enforced")
	}
}

func TestDocumentEditAcrossPieces(t *testing.T) {
	original := strings.Repeat("a\n", 2200) + "tail"
	d := testDocument(t, original)
	want := original[:4094] + "λ\n" + original[4100:]
	if _, err := d.Apply([]Edit{{Start: 4094, End: 4100, Text: "λ\n"}}); err != nil {
		t.Fatal(err)
	}
	if got := d.Text(); got != want {
		t.Fatal("edit crossing an internal piece boundary changed other text")
	}
	if got := d.LineCount(); got != strings.Count(want, "\n")+1 {
		t.Fatalf("line index across pieces: %d", got)
	}
	part, err := d.Slice(4088, 4108)
	if err != nil || part != want[4088:4108] {
		t.Fatalf("slice across pieces: %q, err=%v", part, err)
	}
	if _, ok := d.Undo(); !ok || d.Text() != original {
		t.Fatal("undo across pieces did not restore the document")
	}
}

func BenchmarkDocumentInsertMiddle(b *testing.B) {
	d, err := NewDocument(strings.Repeat("package p\n", 100000))
	if err != nil {
		b.Fatal(err)
	}
	d.HistoryLimit = 0
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		at := d.Len() / 2
		if _, err := d.Apply([]Edit{{Start: at, End: at, Text: "x"}}); err != nil {
			b.Fatal(err)
		}
	}
}
