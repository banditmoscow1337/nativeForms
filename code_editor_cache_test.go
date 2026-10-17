package nativeforms

import (
	"strings"
	"testing"

	"github.com/banditmoscow1337/nativeForms/editor"
)

func TestCodeEditorDistantJumpKeepsPreparedLexerState(t *testing.T) {
	doc, err := editor.NewDocument(strings.Repeat("var x = 42\n", 100000))
	if err != nil {
		t.Fatal(err)
	}
	e := NewCodeEditor(doc)
	if len(e.lexStates) != doc.LineCount()+1 {
		t.Fatalf("prepared %d states for %d lines", len(e.lexStates), doc.LineCount())
	}
	manager := New()
	manager.SetRoot(e)
	manager.BeginFrame(800, 600, 0)
	e.ScrollToLine(90000)
	manager.BeginFrame(800, 600, 0)
	if len(e.lexStates) != doc.LineCount()+1 {
		t.Fatal("distant jump rebuilt lexer state")
	}
	if e.goStateAt(90000) != goNormal {
		t.Fatal("unexpected lexer state")
	}
}
