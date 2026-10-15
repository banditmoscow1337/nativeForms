package nativeforms

import "sort"

// textFieldLayout owns an index of wrapped visual lines. Scrolling and caret
// queries reuse it; Replace only rewraps the paragraphs touched by the edit.
// The suffix offsets are adjusted in place, without reshaping those lines.
type textFieldLayout struct {
	runes             []rune
	lines             []textLine
	width             float32
	spec              FontSpec
	fonts             *FontContext
	fontGeneration    uint64
	version           uint64
	multiline, masked bool
}

func (f *TextField) layoutFor(width float32, spec FontSpec) *textFieldLayout {
	fonts := f.fontContext()
	version := fonts.layoutGeneration()
	c := f.layoutCache
	if c != nil && c.version == f.editor.version && c.width == width && c.spec == spec &&
		c.fonts == fonts && c.fontGeneration == version && c.multiline == f.Multiline && c.masked == f.Password {
		return c
	}
	c = &textFieldLayout{width: width, spec: spec, fonts: fonts, fontGeneration: version,
		version: f.editor.version, multiline: f.Multiline, masked: f.Password}
	c.runes = []rune(f.editor.Text)
	if f.Password {
		maskPasswordRunes(c.runes, fonts != nil)
	}
	c.lines = f.lines(c.runes, width, spec)
	f.layoutCache = c
	return c
}

func (c *textFieldLayout) applyEdit(f *TextField, edit textEdit) bool {
	if c == nil || edit.version == 0 || c.version != edit.base || edit.version != f.editor.version ||
		c.multiline != f.Multiline || c.masked || c.fonts != f.fontContext() ||
		c.fontGeneration != c.fonts.layoutGeneration() {
		return false
	}
	old := c.runes
	a, b := edit.start, edit.end
	if a < 0 || b < a || b > len(old) {
		return false
	}
	c.replaceRange(f, a, b, []rune(edit.insert))
	c.version = edit.version
	return true
}

func (c *textFieldLayout) replaceRange(f *TextField, a, b int, insert []rune) {
	old := c.runes
	// Include the neighboring paragraph to handle edits at a hard break,
	// including a split or merge of CRLF.
	oldStart := max(0, a-1)
	for oldStart > 0 {
		if n := hardTextBreak(old, oldStart-1); n > 0 && oldStart-1+n == oldStart {
			break
		}
		oldStart--
	}
	oldStop := min(len(old), b+1)
	for oldStop < len(old) && hardTextBreak(old, oldStop) == 0 {
		oldStop++
	}
	if oldStop < len(old) {
		oldStop += hardTextBreak(old, oldStop)
	}
	first := sort.Search(len(c.lines), func(i int) bool { return c.lines[i].start >= oldStart })
	last := len(c.lines)
	if oldStop < len(old) {
		last = sort.Search(len(c.lines), func(i int) bool { return c.lines[i].start >= oldStop })
	}
	delta := len(insert) - (b - a)
	newLen := len(old) + delta
	if newLen > len(old) {
		c.runes = append(c.runes, make([]rune, delta)...)
	}
	copy(c.runes[a+len(insert):], old[b:])
	copy(c.runes[a:], insert)
	c.runes = c.runes[:newLen]
	newStop := oldStop + delta
	updated := f.lines(c.runes[oldStart:newStop], c.width, c.spec)
	if newStop < newLen && len(updated) > 0 && updated[len(updated)-1].start == newStop-oldStart {
		updated = updated[:len(updated)-1]
	}
	for i := range updated {
		updated[i].start += oldStart
		updated[i].end += oldStart
	}
	oldCount := len(c.lines)
	newCount := oldCount - (last - first) + len(updated)
	if newCount > oldCount {
		c.lines = append(c.lines, make([]textLine, newCount-oldCount)...)
	}
	copy(c.lines[first+len(updated):], c.lines[last:oldCount])
	c.lines = c.lines[:newCount]
	copy(c.lines[first:], updated)
	for i := first + len(updated); i < len(c.lines); i++ {
		c.lines[i].start += delta
		c.lines[i].end += delta
	}
}
