package nativeforms

import (
	"strings"
	"time"
	"unicode/utf8"
)

func lineBoundaries(r []rune, pos int) (int, int) {
	pos = min(max(pos, 0), len(r))
	start := 0
	for at := 0; at < len(r); {
		if count := hardTextBreak(r, at); count > 0 {
			if pos < at+count {
				return start, at
			}
			at += count
			start = at
		} else {
			at++
		}
	}
	return start, len(r)
}

func selectedWord(r []rune, pos int) (int, int) {
	if len(r) == 0 {
		return 0, 0
	}
	pos = min(max(pos, 0), len(r))
	if pos == len(r) || pos > 0 && !wordRune(r[pos]) && wordRune(r[pos-1]) {
		pos--
	}
	kind := wordRune(r[pos])
	start, end := pos, pos+1
	for start > 0 && wordRune(r[start-1]) == kind && hardTextBreak(r, start-1) == 0 {
		start--
	}
	for end < len(r) && wordRune(r[end]) == kind && hardTextBreak(r, end) == 0 {
		end++
	}
	return nearestGrapheme(r, start), nextGrapheme(r, end-1)
}

func absFloat32(x float32) float32 {
	if x < 0 {
		return -x
	}
	return x
}

func (f *TextField) TextInputSnapshot(maxBytes int) (TextInputState, bool) {
	if f.ReadOnly {
		return TextInputState{}, false
	}
	e := f.Editor()
	if f.Password {
		return TextInputState{Password: true, Multiline: f.Multiline}, true
	}
	if f.snapValid && f.snapVersion == e.version && f.snapCaret == e.Caret &&
		f.snapAnchor == e.Anchor && f.snapMax == maxBytes && f.snapState.Multiline == f.Multiline {
		return f.snapState, true
	}
	state := TextInputState{Password: f.Password, Multiline: f.Multiline}
	cursor := textByteOffset(e.Text, e.Caret)
	anchor := textByteOffset(e.Text, e.Anchor)
	state.Text, state.Base = surroundingWindow(e.Text, cursor, anchor, maxBytes)
	state.Cursor = cursor - state.Base
	state.Anchor = anchor - state.Base
	state.UTF16Base = utf16Units(e.Text[:state.Base])
	f.snapValid = true
	f.snapVersion = e.version
	f.snapCaret = e.Caret
	f.snapAnchor = e.Anchor
	f.snapMax = maxBytes
	f.snapState = state
	return state, true
}

func (f *TextField) validByteRange(text string, start, end int) bool {
	return start >= 0 && end >= start && end <= len(text) &&
		utf8.ValidString(text[:start]) && utf8.ValidString(text[:end])
}

func (f *TextField) ReplaceTextRange(start, end int, value string) bool {
	if f.ReadOnly || !utf8.ValidString(value) {
		return false
	}
	e := f.Editor()
	if !f.validByteRange(e.Text, start, end) {
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
	f.InvalidatePaint()
	return false
}

func (f *TextField) SelectTextRange(start, end int) bool {
	if f.ReadOnly {
		return false
	}
	e := f.Editor()
	if !f.validByteRange(e.Text, start, end) {
		return false
	}
	e.Anchor = utf8.RuneCountInString(e.Text[:start])
	e.Caret = utf8.RuneCountInString(e.Text[:end])
	f.moved()
	return true
}

func (f *TextField) TextInputComposing() bool { return f.composing }

func (f *TextField) ResetCaretBlink() {
	f.caretVisible = true
	f.InvalidatePaint()
}

func (f *TextField) BlinkCaret() { f.caretVisible = !f.caretVisible; f.InvalidatePaint() }

func (f *TextField) changed() {
	f.compositionCache = nil
	if f.layoutCache != nil && !f.layoutCache.applyEdit(f, f.editor.lastEdit) {
		f.layoutCache = nil
	}
	f.Text = f.editor.Text
	f.Cursor = f.editor.Caret
	f.caretVisible = true
	if f.manager != nil && f.Focused() {
		f.manager.caretDue = f.manager.now().Add(500 * time.Millisecond)
	}
	if f.Multiline || f.autoSize {
		f.invalidateLayout()
	} else {
		f.InvalidatePaint()
	}
	if f.OnChange != nil {
		f.OnChange(f.Text)
	}
}

func (f *TextField) moved() {
	f.Cursor = f.editor.Caret
	f.caretVisible = true
	f.InvalidatePaint()
	if f.manager != nil && f.Focused() {
		f.manager.caretDue = f.manager.now().Add(500 * time.Millisecond)
	}
}

func (f *TextField) insert(s string) bool {
	if f.ReadOnly {
		return false
	}
	if !f.Multiline {
		s = strings.ReplaceAll(strings.ReplaceAll(s, "\r", ""), "\n", "")
	}
	r := []rune(f.editor.Text)
	if f.Validate != nil {
		a, b := f.editor.selection()
		if !f.Validate(string(r[:a]) + s + string(r[b:])) {
			return false
		}
	}
	if f.editor.replaceRunes(r, s) {
		f.changed()
		return true
	}
	return false
}

func (f *TextField) delete(backward, word bool) bool {
	if f.ReadOnly {
		return false
	}
	if f.Validate == nil {
		if f.editor.Delete(backward, word) {
			f.changed()
			return true
		}
		return false
	}
	a, b := f.editor.selection()
	r := []rune(f.editor.Text)
	if a == b {
		if word {
			if backward {
				a = wordBoundary(r, a, -1)
			} else {
				b = wordBoundary(r, b, 1)
			}
		} else if backward {
			a = previousGrapheme(r, a)
		} else {
			b = nextGrapheme(r, b)
		}
	}
	if !f.Validate(string(r[:a]) + string(r[b:])) {
		return false
	}
	f.editor.Anchor, f.editor.Caret = a, b
	if f.editor.replaceRunes(r, "") {
		f.changed()
		return true
	}
	return false
}
