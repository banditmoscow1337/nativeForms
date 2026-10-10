package nativeforms

import (
	"strings"
	"unicode/utf8"
)

// TextInputState is the committed text exposed to a native input method.
// Offsets are UTF-8 bytes. Password contents are never exposed.
type TextInputState struct {
	Text                string
	Cursor, Anchor      int
	Password, Multiline bool
	Caret               Rect
	// Identity changes when focus moves between editors, even with equal text.
	Identity *TextField
}

func (m *Manager) TextInputState() (TextInputState, bool) {
	f, ok := m.focused.(*TextField)
	if !ok || f.ReadOnly {
		return TextInputState{}, false
	}
	e := f.Editor()
	r := []rune(e.Text)
	state := TextInputState{Password: f.Password, Multiline: f.Multiline, Caret: f.CaretRect(), Identity: f}
	if !f.Password {
		state.Text = e.Text
		state.Cursor = len(string(r[:min(max(e.Caret, 0), len(r))]))
		state.Anchor = len(string(r[:min(max(e.Anchor, 0), len(r))]))
	}
	return state, true
}

// ReplaceTextRange applies a native edit as one undo operation. Byte offsets
// must fall on UTF-8 boundaries. Validation and OnChange run as for typing.
func (m *Manager) ReplaceTextRange(start, end int, value string) bool {
	f, ok := m.focused.(*TextField)
	if !ok || f.ReadOnly || !utf8.ValidString(value) {
		return false
	}
	e := f.Editor()
	if start < 0 || end < start || end > len(e.Text) || !utf8.ValidString(e.Text[:start]) || !utf8.ValidString(e.Text[:end]) {
		return false
	}
	if !f.Multiline {
		value = strings.ReplaceAll(strings.ReplaceAll(value, "\r", ""), "\n", "")
	}
	next := e.Text[:start] + value + e.Text[end:]
	if f.Validate != nil && !f.Validate(next) {
		return false
	}
	e.Anchor = utf8.RuneCountInString(e.Text[:start])
	e.Caret = utf8.RuneCountInString(e.Text[:end])
	e.Preedit = ""
	if e.Replace(value) {
		f.changed()
		return true
	}
	m.InvalidatePaint()
	return false
}

// DeleteSurroundingText excludes the selected range when counting bytes, as
// required by text-input-v3. The deletion and replacement form one edit.
func (m *Manager) DeleteSurroundingText(before, after int) bool {
	s, ok := m.TextInputState()
	if !ok || s.Password || before < 0 || after < 0 {
		return false
	}
	a, b := min(s.Cursor, s.Anchor), max(s.Cursor, s.Anchor)
	if before > a || after > len(s.Text)-b {
		return false
	}
	return m.ReplaceTextRange(a-before, b+after, "")
}

// SelectTextRange moves the native selection without changing undo history.
func (m *Manager) SelectTextRange(start, end int) bool {
	f, ok := m.focused.(*TextField)
	if !ok || f.ReadOnly {
		return false
	}
	e := f.Editor()
	if start < 0 || end < start || end > len(e.Text) || !utf8.ValidString(e.Text[:start]) || !utf8.ValidString(e.Text[:end]) {
		return false
	}
	e.Anchor = utf8.RuneCountInString(e.Text[:start])
	e.Caret = utf8.RuneCountInString(e.Text[:end])
	f.moved()
	return true
}

// CommitTextInput combines an IME's surrounding deletion and insertion into a
// single validated undo operation. Counts exclude the current selection.
func (m *Manager) CommitTextInput(before, after int, value string) bool {
	s, ok := m.TextInputState()
	if !ok || before < 0 || after < 0 {
		return false
	}
	if s.Password {
		if before != 0 || after != 0 {
			return false
		}
		return m.HandleEvent(Event{Type: CompositionEnd, Text: value})
	}
	a, b := min(s.Cursor, s.Anchor), max(s.Cursor, s.Anchor)
	if before > a || after > len(s.Text)-b {
		return false
	}
	return m.ReplaceTextRange(a-before, b+after, value)
}
