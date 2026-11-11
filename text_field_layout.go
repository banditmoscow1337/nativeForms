package nativeforms

import (
	"sort"
	"sync"
	"unicode/utf8"
)

// textFieldLayout owns an index of wrapped visual lines. Scrolling and caret
// queries reuse it; Replace only rewraps the paragraphs touched by the edit.
// The suffix offsets are adjusted in place, without reshaping those lines.
type textFieldLayout struct {
	runes             []rune
	lines             []textLine
	lineStrings       map[int]string
	width             float32
	spec              FontSpec
	fonts             *FontContext
	fontGeneration    uint64
	version           uint64
	multiline, masked bool
	shared            bool
}

type cachedInitialFieldLayout struct {
	text           string
	width          float32
	spec           FontSpec
	fonts          *FontContext
	fontGeneration uint64
	multiline      bool
	runes          []rune
	lines          []textLine
}

var (
	initialFieldLayoutMu    sync.Mutex
	initialFieldLayoutCache cachedInitialFieldLayout
)

func (c *textFieldLayout) lineText(index int, r []rune, line textLine) string {
	if line.start >= line.end {
		return ""
	}
	if c != nil && len(c.runes) > 0 && &r[0] == &c.runes[0] {
		if s, ok := c.lineStrings[index]; ok {
			return s
		}
		if c.lineStrings == nil {
			c.lineStrings = make(map[int]string, 32)
		}
		s := string(r[line.start:line.end])
		c.lineStrings[index] = s
		return s
	}
	return string(r[line.start:line.end])
}

func (f *TextField) layoutFor(width float32, spec FontSpec) *textFieldLayout {
	fonts := f.fontContext()
	version := fonts.layoutGeneration()
	c := f.layoutCache
	if c != nil && c.version == f.editor.version && c.width == width && c.spec == spec &&
		c.fonts == fonts && c.fontGeneration == version && c.multiline == f.Multiline && c.masked == f.Password {
		return c
	}
	c = &f.layoutStorage
	*c = textFieldLayout{
		width: width, spec: spec, fonts: fonts, fontGeneration: version,
		version: f.editor.version, multiline: f.Multiline, masked: f.Password,
	}
	text := f.editor.Text
	if len(text) > 4096 && f.editor.version == 0 && !f.Password {
		initialFieldLayoutMu.Lock()
		if initialFieldLayoutCache.text == text &&
			initialFieldLayoutCache.width == width &&
			initialFieldLayoutCache.spec == spec &&
			initialFieldLayoutCache.fonts == fonts &&
			initialFieldLayoutCache.fontGeneration == version &&
			initialFieldLayoutCache.multiline == f.Multiline {
			c.runes = initialFieldLayoutCache.runes
			c.lines = initialFieldLayoutCache.lines
			c.shared = true
			initialFieldLayoutMu.Unlock()
			f.singleLineText = ""
			f.layoutCache = c
			return c
		}
		initialFieldLayoutMu.Unlock()
	}
	if len(text) <= len(f.shortRunes)*utf8.UTFMax && utf8.RuneCountInString(text) <= len(f.shortRunes) {
		buf := f.shortRunes[:0]
		for _, r := range text {
			buf = append(buf, r)
		}
		c.runes = buf
	} else {
		c.runes = []rune(text)
	}
	if f.Password {
		maskPasswordRunes(c.runes, fonts != nil)
	}
	c.lines = f.lines(c.runes, width, spec)
	if len(text) > 4096 && f.editor.version == 0 && !f.Password {
		c.shared = true
		initialFieldLayoutMu.Lock()
		initialFieldLayoutCache = cachedInitialFieldLayout{
			text:           text,
			width:          width,
			spec:           spec,
			fonts:          fonts,
			fontGeneration: version,
			multiline:      f.Multiline,
			runes:          c.runes,
			lines:          c.lines,
		}
		initialFieldLayoutMu.Unlock()
	}
	if !f.Multiline && !f.Password && len(c.lines) == 1 && c.lines[0].start == 0 && c.lines[0].end == len(c.runes) {
		f.singleLineText = text
	} else {
		f.singleLineText = ""
	}
	f.layoutCache = c
	return c
}

func (c *textFieldLayout) applyEdit(f *TextField, edit textEdit) bool {
	if c == nil || !c.multiline || edit.version == 0 || c.version != edit.base || edit.version != f.editor.version ||
		c.multiline != f.Multiline || c.masked || c.fonts != f.fontContext() {
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
	if c.shared {
		c.runes = append([]rune(nil), c.runes...)
		c.lines = append([]textLine(nil), c.lines...)
		c.lineStrings = nil
		c.shared = false
	}
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
	if len(c.lineStrings) > 0 {
		clear(c.lineStrings)
	}
}
