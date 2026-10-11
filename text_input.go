package nativeforms

import (
	"unicode/utf8"
)

const textInputWindowBytes = 16000

// TextInputState is the surrounding text exposed to a platform input method.
// Cursor and Anchor are UTF-8 byte offsets within Text. Base is the byte offset
// of Text in the document; UTF16Base is its absolute UTF-16 code-unit offset.
// A client may return more than the requested window size to include a selection.
type TextInputState struct {
	Text                string
	Cursor, Anchor      int
	Base, UTF16Base     int
	Password, Multiline bool
	Caret               Rect
	Identity            *State
}

// TextInputClient is implemented by a focusable text widget. Byte ranges in
// ReplaceTextRange and SelectTextRange are absolute document offsets. The
// snapshot's window is only for the platform IME and must include the caret
// and anchor. The widget owns validation, edits, selection and composition.
type TextInputClient interface {
	TextInputSnapshot(maxBytes int) (TextInputState, bool)
	ReplaceTextRange(start, end int, value string) bool
	SelectTextRange(start, end int) bool
	CaretRect() Rect
	TextInputComposing() bool
	ResetCaretBlink()
	BlinkCaret()
}

func (m *Manager) textInputClient() (TextInputClient, bool) {
	client, ok := m.focused.(TextInputClient)
	return client, ok
}

func (m *Manager) TextInputState() (TextInputState, bool) {
	client, ok := m.textInputClient()
	if !ok {
		return TextInputState{}, false
	}
	state, ok := client.TextInputSnapshot(textInputWindowBytes)
	if !ok {
		return TextInputState{}, false
	}
	state.Identity = m.focused.UIState()
	state.Caret = client.CaretRect()
	return state, true
}

func (m *Manager) ReplaceTextRange(start, end int, value string) bool {
	client, ok := m.textInputClient()
	if !ok || !utf8.ValidString(value) {
		return false
	}
	if _, ok := client.TextInputSnapshot(textInputWindowBytes); !ok {
		return false
	}
	return client.ReplaceTextRange(start, end, value)
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
	return m.ReplaceTextRange(s.Base+a-before, s.Base+b+after, "")
}

func (m *Manager) SelectTextRange(start, end int) bool {
	client, ok := m.textInputClient()
	if !ok {
		return false
	}
	if _, ok := client.TextInputSnapshot(textInputWindowBytes); !ok {
		return false
	}
	return client.SelectTextRange(start, end)
}

func (m *Manager) CommitTextInput(before, after int, value string) bool {
	s, ok := m.TextInputState()
	if !ok || before < 0 || after < 0 || !utf8.ValidString(value) {
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
	return m.ReplaceTextRange(s.Base+a-before, s.Base+b+after, value)
}

// surroundingWindow returns a UTF-8-aligned window containing both selection
// endpoints. maxBytes is a soft limit when the selection itself is larger.
func surroundingWindow(text string, cursor, anchor, maxBytes int) (window string, base int) {
	a, b := min(cursor, anchor), max(cursor, anchor)
	if maxBytes <= 0 || maxBytes > len(text) {
		maxBytes = len(text)
	}
	maxBytes = max(maxBytes, b-a)
	start := max(0, a-(maxBytes-(b-a))/2)
	start = min(start, len(text)-maxBytes)
	end := start + maxBytes
	for start > 0 && !utf8.RuneStart(text[start]) {
		start--
	}
	for end < len(text) && !utf8.RuneStart(text[end]) {
		end++
	}
	return text[start:end], start
}

func textByteOffset(text string, at int) int {
	if at <= 0 {
		return 0
	}
	count := 0
	for offset := range text {
		if count == at {
			return offset
		}
		count++
	}
	return len(text)
}

func utf16Units(text string) int {
	n := 0
	for _, r := range text {
		n++
		if r > 0xffff {
			n++
		}
	}
	return n
}
