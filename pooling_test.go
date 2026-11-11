package nativeforms

import (
	"fmt"
	"strings"
	"testing"

	"github.com/banditmoscow1337/nativeForms/editor"
)

func TestCodeEditorPoolNeverOverwritesLiveEditorsAndRecyclesOnClose(t *testing.T) {
	const count = 48
	editors := make([]*CodeEditor, count)
	seen := make(map[*CodeEditor]bool, count)
	for i := 0; i < count; i++ {
		doc, err := editor.NewDocument(fmt.Sprintf("package tab%d\n", i))
		if err != nil {
			t.Fatal(err)
		}
		ed := NewCodeEditor(doc)
		if seen[ed] {
			t.Fatalf("NewCodeEditor overwrote live editor at index %d", i)
		}
		seen[ed] = true
		editors[i] = ed
	}
	for i, ed := range editors {
		want := fmt.Sprintf("package tab%d\n", i)
		if got := ed.Text(); got != want {
			t.Fatalf("editor %d corrupted: got %q, want %q", i, got, want)
		}
	}
	for _, ed := range editors {
		ed.Close()
	}
	reused := NewCodeEditor(nil)
	if !seen[reused] {
		t.Fatal("expected closed CodeEditor to be reused from pool")
	}
	reused.Close()
}

func TestTextFieldSmallAndLargePoolsNeverOverwriteLiveFields(t *testing.T) {
	const count = 80
	largePrefix := strings.Repeat("0123456789abcdef", 300) // 4800 bytes > 4096
	smallFields := make([]*TextField, count)
	largeFields := make([]*TextField, count)
	seenSmall := make(map[*TextField]bool, count)
	seenLarge := make(map[*TextField]bool, count)

	for i := 0; i < count; i++ {
		sf := NewTextField(fmt.Sprintf("small-%d", i), nil)
		if seenSmall[sf] {
			t.Fatalf("duplicate small TextField pointer at %d", i)
		}
		seenSmall[sf] = true
		smallFields[i] = sf

		lf := NewTextArea(fmt.Sprintf("%s-%d", largePrefix, i), nil)
		if seenLarge[lf] {
			t.Fatalf("duplicate large TextArea pointer at %d", i)
		}
		seenLarge[lf] = true
		largeFields[i] = lf
	}

	for i := 0; i < count; i++ {
		if got, want := smallFields[i].Text, fmt.Sprintf("small-%d", i); got != want {
			t.Fatalf("small field %d corrupted: got %q, want %q", i, got, want)
		}
		wantSuffix := fmt.Sprintf("-%d", i)
		if !strings.HasSuffix(largeFields[i].Text, wantSuffix) {
			t.Fatalf("large field %d corrupted: missing suffix %q", i, wantSuffix)
		}
	}

	for i := 0; i < count; i++ {
		smallFields[i].Close()
		largeFields[i].Close()
	}
}

func TestManagerPoolNeverOverwritesActiveManagersAndRecyclesOnClose(t *testing.T) {
	const count = 80
	managers := make([]*Manager, count)
	seen := make(map[*Manager]bool, count)
	for i := 0; i < count; i++ {
		m := New()
		m.SetRoot(NewLabel(fmt.Sprintf("mgr-%d", i)))
		m.BeginFrame(200, 100, 0)
		if seen[m] {
			t.Fatalf("New() overwrote active Manager at index %d", i)
		}
		seen[m] = true
		managers[i] = m
	}
	for i, m := range managers {
		lbl, ok := m.Root().(*Label)
		if !ok || lbl.Text != fmt.Sprintf("mgr-%d", i) {
			t.Fatalf("manager %d root corrupted: %+v", i, m.Root())
		}
	}
	for _, m := range managers {
		m.Close()
	}
	reused := New()
	if !seen[reused] {
		t.Fatal("expected Manager.Close() to return instance to freeManagers pool")
	}
	reused.Close()
}

func TestReleaseComponentRecyclesNestedWidgetsInCompositeRow(t *testing.T) {
	row := NewStack(Horizontal, 8)
	lbl := NewLabel("Title")
	tf := NewTextField("Nested value", nil)
	row.Add(lbl, tf)

	if row.ChildCount() != 2 || row.ChildAt(0) != lbl || row.ChildAt(1) != tf || row.ChildAt(2) != nil {
		t.Fatal("ChildCount / ChildAt returned unexpected results")
	}

	// While mounted in row, tf.Close() is a no-op.
	tf.Close()
	if tf.inPool {
		t.Fatal("mounted TextField must not enter pool while parent != nil")
	}

	// ReleaseComponent on the detached root row recursively unbinds and recycles tf.
	ReleaseComponent(row)
	if !tf.inPool {
		t.Fatal("ReleaseComponent did not recycle nested TextField inside Panel")
	}
}
