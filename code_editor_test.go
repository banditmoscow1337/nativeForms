package nativeforms

import (
	"errors"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/banditmoscow1337/nativeForms/editor"
)

func codeTestDocument(t *testing.T, text string) *editor.Document {
	t.Helper()
	d, err := editor.NewDocument(text)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func TestCodeEditorVisibleLinesAndScroll(t *testing.T) {
	e := NewCodeEditor(codeTestDocument(t, strings.Repeat("x\n", 100000)))
	e.Arrange(Rect{W: 500, H: 100})
	first, last := e.VisibleLines()
	if first != 0 || last-first > 10 || last == 0 {
		t.Fatalf("initial visible lines: %d..%d", first, last)
	}
	e.ScrollToLine(90000)
	first, last = e.VisibleLines()
	if first != 90000 || last-first > 10 {
		t.Fatalf("scrolled visible lines: %d..%d", first, last)
	}
	if e.Document.LineCount() != 100001 {
		t.Fatal("scrolling changed the document")
	}
	m := New()
	m.SetRoot(e)
	m.BeginFrame(500, 100, 0)
	if commands := len(m.Frame().Commands); commands > 1000 {
		t.Fatalf("paint generated commands for offscreen lines: %d", commands)
	}
}

func TestCodeEditorIMEWindowAndAbsoluteReplacement(t *testing.T) {
	prefix := strings.Repeat("a", 20000)
	e := NewCodeEditor(codeTestDocument(t, prefix+"🙂z"+strings.Repeat("b", 20000)))
	e.Arrange(Rect{W: 400, H: 100})
	if !e.SetSelection(20000, 20004) {
		t.Fatal("could not select the supplementary rune")
	}
	s, ok := e.TextInputSnapshot(64)
	if !ok || s.Base <= 0 || len(s.Text) > 68 || !utf8.ValidString(s.Text) ||
		s.Text[s.Anchor:s.Cursor] != "🙂" || s.Base+s.Anchor != 20000 || s.UTF16Base != s.Base {
		t.Fatalf("incorrect IME window: %+v", s)
	}
	m := New()
	m.SetInteractive(true)
	m.SetRoot(NewPanel())
	m.OpenPopup(e, Rect{W: 400, H: 100}, true)
	s, ok = m.TextInputState()
	if !ok || s.Identity != e.UIState() || !m.CommitTextInput(0, 0, "界") {
		t.Fatal("generic IME did not use the CodeEditor input client")
	}
	if got, _ := e.Document.Slice(20000, 20003); got != "界" {
		t.Fatalf("IME replaced the wrong document range: %q", got)
	}
	if e.Caret != 20003 || e.Anchor != e.Caret {
		t.Fatalf("caret after IME edit: %d..%d", e.Anchor, e.Caret)
	}
}

func TestCodeEditorIndentSelectionAndUndo(t *testing.T) {
	e := NewCodeEditor(codeTestDocument(t, "one\n\ttwo\nthree"))
	e.Arrange(Rect{W: 500, H: 120})
	changes := 0
	e.OnChange = func(change editor.Change) {
		if change.Version <= change.BeforeVersion {
			t.Fatal("non-increasing document change version")
		}
		changes++
	}
	if !e.SetSelection(0, len("one\n\ttwo")) || !e.Handle(Event{Type: KeyDown, Key: KeyTab}) ||
		e.Text() != "\tone\n\t\ttwo\nthree" {
		t.Fatalf("multi-line indentation failed: %q", e.Text())
	}
	e.Handle(Event{Type: KeyDown, Key: KeyTab, Mods: ModShift})
	if e.Text() != "one\n\ttwo\nthree" {
		t.Fatalf("multi-line outdent failed: %q", e.Text())
	}
	e.Handle(Event{Type: KeyDown, Key: KeyZ, Mods: ModControl})
	if e.Text() != "\tone\n\t\ttwo\nthree" || changes != 3 {
		t.Fatalf("undo or change notification failed: %q, %d", e.Text(), changes)
	}
}

func TestCodeEditorNavigationAndBreakpoints(t *testing.T) {
	e := NewCodeEditor(codeTestDocument(t, "a🙂\nb\nc"))
	e.Arrange(Rect{W: 350, H: 100})
	if !e.SetSelection(5, 5) {
		t.Fatal("could not place caret after emoji")
	}
	e.Handle(Event{Type: KeyDown, Key: KeyRightArrow})
	if e.Caret != 6 {
		t.Fatalf("right arrow did not cross newline: %d", e.Caret)
	}
	e.Handle(Event{Type: KeyDown, Key: KeyBackspace})
	if e.Text() != "a🙂b\nc" || e.Caret != 5 {
		t.Fatalf("backspace did not join lines: %q at %d", e.Text(), e.Caret)
	}
	e.Handle(Event{Type: KeyDown, Key: KeyZ, Mods: ModControl})
	if e.Text() != "a🙂\nb\nc" {
		t.Fatal("undo did not restore newline")
	}
	var changedLine int
	e.OnBreakpointChange = func(line int, enabled bool) {
		if enabled {
			changedLine = line
		}
	}
	if !e.SetBreakpoint(2, true) || changedLine != 2 || !e.Breakpoint(2) {
		t.Fatal("breakpoint was not set")
	}
	e.SetSelection(0, 0)
	e.ReplaceTextRange(0, 0, "\n")
	if !e.Breakpoint(3) || e.Breakpoint(2) {
		t.Fatal("breakpoint did not follow inserted line")
	}
	if e.Caret != 1 {
		t.Fatalf("caret did not move past inserted newline: %d", e.Caret)
	}
	e.Handle(Event{Type: KeyDown, Key: KeyF9})
	if !e.Breakpoint(1) || changedLine != 1 {
		t.Fatal("F9 did not toggle the line under the caret")
	}
	e.Handle(Event{Type: KeyDown, Key: KeyF9})
	if e.Breakpoint(1) {
		t.Fatal("second F9 did not clear the breakpoint")
	}
	if e.Handle(Event{Type: KeyDown, Key: KeyF5}) {
		t.Fatal("unassigned function key was swallowed by the editor")
	}
}

func TestCodeEditorGoLexerAndSemanticTokens(t *testing.T) {
	spans, state := lexGoLine("var s = `hello", goNormal)
	if state != goRawString || len(spans) != 2 || spans[0].Style != CodeKeyword || spans[1].Style != CodeString {
		t.Fatalf("raw string opening: state=%v spans=%+v", state, spans)
	}
	spans, state = lexGoLine("world` // note", state)
	if state != goNormal || len(spans) != 2 || spans[0].Style != CodeString || spans[1].Style != CodeComment {
		t.Fatalf("raw string closing: state=%v spans=%+v", state, spans)
	}
	spans, state = lexGoLine("/* block", state)
	if state != goBlockComment || len(spans) != 1 || spans[0].Start != 0 {
		t.Fatalf("block comment start: state=%v spans=%+v", state, spans)
	}
	spans, state = lexGoLine("end */ func f() {}", state)
	if state != goNormal || len(spans) < 2 || spans[0].Style != CodeComment || spans[1].Style != CodeKeyword {
		t.Fatalf("block comment end: state=%v spans=%+v", state, spans)
	}

	e := NewCodeEditor(codeTestDocument(t, "a🙂b\nfunc main() {}"))
	version := e.Document.Version()
	legend := []string{"variable", "keyword"}
	data := []uint32{0, 1, 2, 0, 0, 1, 0, 4, 1, 0}
	if err := e.SetSemanticTokens(version, legend, data); err != nil {
		t.Fatal(err)
	}
	if len(e.semantic) != 2 || e.semantic[0] != (StyledRange{1, 5, CodeVariable}) ||
		e.semantic[1] != (StyledRange{7, 11, CodeKeyword}) {
		t.Fatalf("UTF-16 semantic token conversion: %+v", e.semantic)
	}
	if err := e.SetStyleRanges(version, []StyledRange{{Start: 0, End: 1, Style: CodeComment}}); err != nil {
		t.Fatal(err)
	}
	line, _ := e.Document.Slice(0, 6)
	styles := e.rowStyles(line, e.layoutLine(line), 0, 0)
	if len(styles) != 3 || styles[0] != CodeComment || styles[1] != CodeVariable {
		t.Fatalf("paint style precedence: %+v", styles)
	}
	if err := e.SetSemanticTokens(version, legend, []uint32{0, 2, 1, 0, 0}); !errors.Is(err, ErrInvalidCodeStyle) || len(e.semantic) != 2 {
		t.Fatal("invalid surrogate token replaced valid styles")
	}
	e.ReplaceTextRange(0, 1, "x")
	if len(e.styles) != 0 || len(e.semantic) != 0 {
		t.Fatal("edit left stale styling ranges")
	}
	if err := e.SetStyleRanges(version, []StyledRange{{Start: 0, End: 1, Style: CodeString}}); !errors.Is(err, ErrStaleCodeStyle) {
		t.Fatalf("obsolete styles were accepted: %v", err)
	}
}
