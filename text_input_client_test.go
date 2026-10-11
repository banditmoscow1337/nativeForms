package nativeforms

import (
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

type sampleTextInput struct {
	State
	snapshot TextInputState
	active   bool
	composes bool
	requested int
	start, end int
	value string
	selectedStart, selectedEnd int
	resets, blinks int
}

func (c *sampleTextInput) TextInputSnapshot(maxBytes int) (TextInputState, bool) {
	c.requested = maxBytes
	return c.snapshot, c.active
}
func (c *sampleTextInput) ReplaceTextRange(start, end int, value string) bool {
	c.start, c.end, c.value = start, end, value
	return true
}
func (c *sampleTextInput) SelectTextRange(start, end int) bool {
	c.selectedStart, c.selectedEnd = start, end
	return true
}
func (c *sampleTextInput) CaretRect() Rect { return Rect{X: 12, Y: 34, W: 1, H: 18} }
func (c *sampleTextInput) TextInputComposing() bool { return c.composes }
func (c *sampleTextInput) ResetCaretBlink() { c.resets++ }
func (c *sampleTextInput) BlinkCaret() { c.blinks++ }
func (c *sampleTextInput) Handle(event Event) bool {
	if event.Type == KeyDown && event.Key == KeyEscape && c.composes {
		c.composes = false
		return true
	}
	return false
}

func TestManagerUsesTextInputClientWithoutTextField(t *testing.T) {
	c := &sampleTextInput{active: true, snapshot: TextInputState{
		Text: "pq🚀rst", Cursor: 6, Anchor: 6, Base: 100, UTF16Base: 90,
	}}
	c.SetFocusable(true)
	m := New()
	m.SetRoot(c)
	m.setFocus(c)
	m.hovered = c
	s, ok := m.TextInputState()
	if !ok || s.Text != c.snapshot.Text || s.Base != 100 || s.UTF16Base != 90 ||
		s.Identity != c.UIState() || s.Caret != c.CaretRect() || c.requested != textInputWindowBytes {
		t.Fatalf("client snapshot not exposed: %+v, ok=%v", s, ok)
	}
	if !m.TextInputFocused() || !m.HoveredTextInput() || c.resets != 1 {
		t.Fatal("generic input did not participate in focus and hover")
	}
	if r, ok := m.FocusedCaretRect(); !ok || r != c.CaretRect() {
		t.Fatal("generic client caret was not used")
	}
	m.caretDue = m.now().Add(-time.Millisecond)
	m.BeginFrame(200, 80, 0)
	if c.blinks != 1 {
		t.Fatal("generic client caret did not blink")
	}
	if !m.DeleteSurroundingText(4, 1) || c.start != 102 || c.end != 107 || c.value != "" {
		t.Fatalf("deletion used the window's relative offsets: %d..%d %q", c.start, c.end, c.value)
	}
	if !m.CommitTextInput(0, 2, "✨") || c.start != 106 || c.end != 108 || c.value != "✨" {
		t.Fatalf("commit used the window's relative offsets: %d..%d %q", c.start, c.end, c.value)
	}
	if !m.SelectTextRange(101, 107) || c.selectedStart != 101 || c.selectedEnd != 107 {
		t.Fatal("selection was not delegated in absolute document offsets")
	}
	if m.DeleteSurroundingText(7, 0) || m.CommitTextInput(0, 4, "x") || m.ReplaceTextRange(0, 0, "\xff") {
		t.Fatal("invalid IME range or invalid UTF-8 was accepted")
	}
	c.active = false
	if _, ok := m.TextInputState(); ok || m.ReplaceTextRange(0, 0, "x") || m.SelectTextRange(0, 0) {
		t.Fatal("inactive client accepted a text operation")
	}
}

func TestGenericTextInputCompositionTakesEscapeBeforePopup(t *testing.T) {
	m := New()
	m.SetInteractive(true)
	m.SetRoot(NewPanel())
	c := &sampleTextInput{active: true, composes: true}
	c.SetFocusable(true)
	m.OpenPopup(c, Rect{X: 10, Y: 10, W: 150, H: 40}, true)
	if m.focused != c || !m.HandleEvent(Event{Type: KeyDown, Key: KeyEscape}) ||
		c.composes || len(m.popups) != 1 {
		t.Fatal("first Escape did not cancel generic IME composition")
	}
	if !m.HandleEvent(Event{Type: KeyDown, Key: KeyEscape}) || len(m.popups) != 0 {
		t.Fatal("second Escape did not close the popup")
	}
}

func TestTextFieldSnapshotIsBoundedAndUsesUTF16Offsets(t *testing.T) {
	prefix := strings.Repeat("a", 20000)
	f := NewTextArea(prefix+"🙂z"+strings.Repeat("b", 20000), nil)
	e := f.Editor()
	e.Anchor = 20000
	e.Caret = 20001
	f.Cursor = e.Caret
	s, ok := f.TextInputSnapshot(64)
	if !ok || s.Base <= 0 || len(s.Text) > 68 || !utf8.ValidString(s.Text) ||
		s.Text[s.Anchor:s.Cursor] != "🙂" || s.Base+s.Anchor != len(prefix) ||
		s.UTF16Base != utf16Units(f.Text[:s.Base]) {
		t.Fatalf("incorrect window or offsets: base=%d utf16=%d cursor=%d anchor=%d length=%d", s.Base, s.UTF16Base, s.Cursor, s.Anchor, len(s.Text))
	}
	f.Password = true
	secret, ok := f.TextInputSnapshot(64)
	if !ok || !secret.Password || secret.Text != "" || secret.Cursor != 0 || secret.Base != 0 {
		t.Fatal("password snapshot exposed document text")
	}
	f.ReadOnly = true
	if _, ok := f.TextInputSnapshot(64); ok || f.ReplaceTextRange(0, 0, "x") || f.SelectTextRange(0, 0) {
		t.Fatal("read-only TextField accepted IME operations")
	}
}

func TestTextFieldInputRangeKeepsValidationAndUTF8Boundaries(t *testing.T) {
	f := NewTextField("a🙂b", nil)
	f.Validate = func(value string) bool { return !strings.Contains(value, "!") }
	if f.ReplaceTextRange(2, 2, "x") || f.ReplaceTextRange(1, 5, "!") || f.Text != "a🙂b" {
		t.Fatal("invalid range or rejected edit changed the field")
	}
	if !f.ReplaceTextRange(1, 5, "z\n") || f.Text != "azb" {
		t.Fatalf("valid replacement did not retain single-line behavior: %q", f.Text)
	}
	if !f.SelectTextRange(1, 2) || f.Editor().Anchor != 1 || f.Editor().Caret != 2 {
		t.Fatal("selection was not converted from byte offsets to rune offsets")
	}
}

func TestSurroundingWindowIncludesLongSelection(t *testing.T) {
	value := strings.Repeat("🙂", 40)
	window, base := surroundingWindow(value, 4, len(value)-4, 16)
	if base > 4 || base+len(window) < len(value)-4 || !utf8.ValidString(window) ||
		!utf8.ValidString(value[:base]) || !utf8.ValidString(value[:base+len(window)]) {
		t.Fatal("large selection was truncated or UTF-8 was split")
	}
}
