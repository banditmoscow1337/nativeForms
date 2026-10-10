package nativeforms

import (
	"strings"
	"unicode/utf8"
)

type TextInputState struct {
	Text                string
	Cursor, Anchor      int
	Password, Multiline bool
	Caret               Rect
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
