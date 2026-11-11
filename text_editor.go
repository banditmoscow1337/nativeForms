package nativeforms

import (
	"unicode"
	"unicode/utf8"
)

type TextEditor struct {
	Text                        string
	Caret, Anchor               int
	Preedit                     string
	PreeditCaret, PreeditAnchor int
	undo, redo                  []editSnapshot
	Limit                       int
	UndoByteLimit               int
	version                     uint64
	lastEdit                    textEdit
}

type textEdit struct {
	base, version uint64
	start, end    int // rune offsets in the previous text
	insert        string
}

type editSnapshot struct {
	text          string
	caret, anchor int
}

func NewTextEditor(text string) *TextEditor {
	n := utf8.RuneCountInString(text)
	return &TextEditor{Text: text, Caret: n, Anchor: n}
}

func (e *TextEditor) SetText(text string) {
	e.version++
	e.lastEdit = textEdit{}
	e.Text = text
	e.Caret = utf8.RuneCountInString(text)
	e.Anchor = e.Caret
	e.Preedit = ""
	e.undo = nil
	e.redo = nil
}

func (e *TextEditor) selection() (int, int) {
	return min(e.Anchor, e.Caret), max(e.Anchor, e.Caret)
}

func (e *TextEditor) Selection() string {
	a, b := e.selection()
	r := []rune(e.Text)
	a = min(max(a, 0), len(r))
	b = min(max(b, a), len(r))
	return string(r[a:b])
}

func (e *TextEditor) SelectAll() { e.Anchor = 0; e.Caret = utf8.RuneCountInString(e.Text) }

func (e *TextEditor) moveRunes(r []rune, to int, extend bool) {
	to = nearestGrapheme(r, min(max(to, 0), len(r)))
	e.Caret = to
	if !extend {
		e.Anchor = to
	}
}

func (e *TextEditor) Move(to int, extend bool) {
	e.moveRunes([]rune(e.Text), to, extend)
}

func (e *TextEditor) Step(direction int, word, extend bool) {
	r := []rune(e.Text)
	if !extend && e.Caret != e.Anchor && !word {
		a, b := e.selection()
		if direction < 0 {
			e.moveRunes(r, a, false)
		} else {
			e.moveRunes(r, b, false)
		}
		return
	}
	if word {
		e.moveRunes(r, wordBoundary(r, e.Caret, direction), extend)
		return
	}
	if direction < 0 {
		e.moveRunes(r, previousGrapheme(r, e.Caret), extend)
	} else {
		e.moveRunes(r, nextGrapheme(r, e.Caret), extend)
	}
}

func (e *TextEditor) replaceRunes(r []rune, value string) bool {
	a, b := e.selection()
	a = min(max(a, 0), len(r))
	b = min(max(b, a), len(r))
	if a == b && value == "" {
		return false
	}
	updated := string(r[:a]) + value + string(r[b:])
	if updated == e.Text {
		return false
	}
	e.record()
	base := e.version
	e.version++
	e.lastEdit = textEdit{base: base, version: e.version, start: a, end: b, insert: value}
	e.Text = updated
	e.Caret = a + utf8.RuneCountInString(value)
	updatedRunes := []rune(updated)
	if !graphemeBreak(updatedRunes, e.Caret) {
		e.Caret = nextGrapheme(updatedRunes, e.Caret)
	}
	e.Anchor = e.Caret
	e.Preedit = ""
	return true
}

func (e *TextEditor) Replace(value string) bool {
	return e.replaceRunes([]rune(e.Text), value)
}

func (e *TextEditor) Delete(backward, word bool) bool {
	r := []rune(e.Text)
	if e.Caret == e.Anchor {
		pos := e.Caret
		if word {
			dir := 1
			if backward {
				dir = -1
			}
			pos = wordBoundary(r, pos, dir)
		} else if backward {
			pos = previousGrapheme(r, pos)
		} else {
			pos = nextGrapheme(r, pos)
		}
		e.Anchor = pos
	}
	return e.replaceRunes(r, "")
}

func (e *TextEditor) record() {
	limit := e.Limit
	if limit <= 0 {
		limit = 128
	}
	e.undo = append(e.undo, editSnapshot{e.Text, e.Caret, e.Anchor})
	e.trimHistory(&e.undo, limit)
	e.redo = nil
	e.trimByteBudget()
}

func (e *TextEditor) trimHistory(history *[]editSnapshot, limit int) {
	for len(*history) > limit {
		dropOldestSnapshot(history)
	}
}

func dropOldestSnapshot(history *[]editSnapshot) {
	copy((*history)[0:], (*history)[1:])
	(*history)[len(*history)-1] = editSnapshot{}
	*history = (*history)[:len(*history)-1]
}

func (e *TextEditor) trimByteBudget() {
	byteLimit := e.UndoByteLimit
	if byteLimit <= 0 {
		byteLimit = 8 << 20
	}
	used := 0
	for _, snapshot := range e.undo {
		used += len(snapshot.text)
	}
	for _, snapshot := range e.redo {
		used += len(snapshot.text)
	}
	for used > byteLimit {
		if len(e.redo) > 0 {
			used -= len(e.redo[0].text)
			dropOldestSnapshot(&e.redo)
		} else if len(e.undo) > 0 {
			used -= len(e.undo[0].text)
			dropOldestSnapshot(&e.undo)
		} else {
			break
		}
	}
}

func (e *TextEditor) Undo() bool { return e.restore(&e.undo, &e.redo) }
func (e *TextEditor) Redo() bool { return e.restore(&e.redo, &e.undo) }
func (e *TextEditor) restore(from, to *[]editSnapshot) bool {
	if len(*from) == 0 {
		return false
	}
	*to = append(*to, editSnapshot{e.Text, e.Caret, e.Anchor})
	limit := e.Limit
	if limit <= 0 {
		limit = 128
	}
	last := len(*from) - 1
	s := (*from)[last]
	(*from)[last] = editSnapshot{}
	*from = (*from)[:last]
	e.trimHistory(to, limit)
	e.trimByteBudget()
	e.Text, e.Caret, e.Anchor = s.text, s.caret, s.anchor
	e.version++
	e.lastEdit = textEdit{}
	e.Preedit = ""
	return true
}

func (e *TextEditor) CommitComposition(value string) bool {
	e.Preedit = ""
	return e.Replace(value)
}

func previousGrapheme(r []rune, at int) int {
	at = min(max(at, 0), len(r))
	for i := at - 1; i >= 0; i-- {
		if graphemeBreak(r, i) {
			return i
		}
	}
	return 0
}
func nextGrapheme(r []rune, at int) int {
	at = min(max(at, 0), len(r))
	for i := at + 1; i < len(r); i++ {
		if graphemeBreak(r, i) {
			return i
		}
	}
	return len(r)
}
func nearestGrapheme(r []rune, at int) int {
	if at <= 0 {
		return 0
	}
	if at >= len(r) {
		return len(r)
	}
	if graphemeBreak(r, at) {
		return at
	}
	return previousGrapheme(r, at)
}

func graphemeBreak(r []rune, i int) bool {
	if i <= 0 || i >= len(r) {
		return true
	}
	a, b := r[i-1], r[i]
	// Fast path for printable ASCII and precomposed Latin/Cyrillic runes (< U+0300,
	// or U+0400..U+0482 before Cyrillic combining marks).
	if a >= 0x20 && b >= 0x20 {
		if a < 0x7f && b < 0x7f {
			return true
		}
		if (a < 0x0300 || (a >= 0x0400 && a <= 0x0482)) &&
			(b < 0x0300 || (b >= 0x0400 && b <= 0x0482)) &&
			(a < 0x7f || a > 0x9f) && a != 0xad &&
			(b < 0x7f || b > 0x9f) && b != 0xad {
			return true
		}
	}
	if a == '\r' && b == '\n' {
		return false
	}
	if a == '\r' || a == '\n' || b == '\r' || b == '\n' || unicode.IsControl(a) || unicode.IsControl(b) {
		return true
	}
	if hangulLink(a, b) {
		return false
	}
	if unicode.Is(unicode.Mn, b) || unicode.Is(unicode.Me, b) || unicode.Is(unicode.Mc, b) || b == 0x200d || isVariation(b) || isSkinTone(b) {
		return false
	}
	if a == 0x200d && isPictograph(b) {
		for j := i - 2; j >= 0; j-- {
			if unicode.Is(unicode.Mn, r[j]) || isVariation(r[j]) {
				continue
			}
			if isPictograph(r[j]) {
				return false
			}
			break
		}
	}
	if isRegional(a) && isRegional(b) {
		n := 0
		for j := i - 1; j >= 0 && isRegional(r[j]); j-- {
			n++
		}
		return n%2 == 0
	}
	return true
}
func isVariation(r rune) bool  { return r >= 0xfe00 && r <= 0xfe0f || r >= 0xe0100 && r <= 0xe01ef }
func isSkinTone(r rune) bool   { return r >= 0x1f3fb && r <= 0x1f3ff }
func isRegional(r rune) bool   { return r >= 0x1f1e6 && r <= 0x1f1ff }
func isPictograph(r rune) bool { return r >= 0x1f000 && r <= 0x1faff || r >= 0x2600 && r <= 0x27bf }
func hangulClass(r rune) int {
	switch {
	case r >= 0x1100 && r <= 0x115f || r >= 0xa960 && r <= 0xa97c:
		return 1
	case r >= 0x1160 && r <= 0x11a7 || r >= 0xd7b0 && r <= 0xd7c6:
		return 2
	case r >= 0x11a8 && r <= 0x11ff || r >= 0xd7cb && r <= 0xd7fb:
		return 3
	case r >= 0xac00 && r <= 0xd7a3:
		if (r-0xac00)%28 == 0 {
			return 4
		}
		return 5
	}
	return 0
}
func hangulLink(a, b rune) bool {
	x, y := hangulClass(a), hangulClass(b)
	return x == 1 && (y == 1 || y == 2 || y == 4 || y == 5) ||
		(x == 2 || x == 4) && (y == 2 || y == 3) || (x == 3 || x == 5) && y == 3
}
func wordRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || unicode.IsMark(r) || r == '_'
}
func wordBoundary(r []rune, at, direction int) int {
	at = min(max(at, 0), len(r))
	if direction < 0 {
		for at > 0 && !wordRune(r[at-1]) {
			at--
		}
		for at > 0 && wordRune(r[at-1]) {
			at--
		}
		return nearestGrapheme(r, at)
	}
	for at < len(r) && !wordRune(r[at]) {
		at++
	}
	for at < len(r) && wordRune(r[at]) {
		at++
	}
	return nearestGrapheme(r, at)
}
