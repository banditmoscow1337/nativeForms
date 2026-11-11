package nativeforms

import (
	"strings"
	"time"
	"unicode/utf8"

	"github.com/banditmoscow1337/nativeForms/editor"
)

// SetSelection keeps the direction of an anchor/caret pair. Both offsets must
// be valid document text positions, including UTF-8 and CRLF boundaries.
func (e *CodeEditor) SetSelection(anchor, caret int) bool {
	e.syncDocument()
	if _, err := e.Document.LineAtOffset(anchor); err != nil {
		return false
	}
	if _, err := e.Document.LineAtOffset(caret); err != nil {
		return false
	}
	e.Anchor, e.Caret = anchor, caret
	e.moved(false)
	return true
}

func (e *CodeEditor) SelectTextRange(start, end int) bool {
	if start > end {
		return false
	}
	return e.SetSelection(start, end)
}

func (e *CodeEditor) selection() (int, int) {
	return min(e.Anchor, e.Caret), max(e.Anchor, e.Caret)
}

func (e *CodeEditor) selectedText() string {
	a, b := e.selection()
	text, _ := e.Document.Slice(a, b)
	return text
}

func (e *CodeEditor) replaceSelection(value string) bool {
	a, b := e.selection()
	return e.ReplaceTextRange(a, b, value)
}

func (e *CodeEditor) ReplaceTextRange(start, end int, value string) bool {
	e.syncDocument()
	if e.ReadOnly || !utf8.ValidString(value) || start < 0 || end < start {
		return false
	}
	var removed string
	if start < end {
		var err error
		removed, err = e.Document.Slice(start, end)
		if err != nil {
			return false
		}
	}
	line, err := e.Document.LineAtOffset(start)
	if err != nil {
		return false
	}
	change, err := e.Document.ApplySingle(editor.Edit{Start: start, End: end, Text: value})
	if err != nil || change.Version == change.BeforeVersion {
		return false
	}
	removedLines := strings.Count(removed, "\n")
	insertedLines := strings.Count(value, "\n")
	e.shiftBreakpoints(line, removedLines, insertedLines)
	e.Caret = e.validCaret(start + len(value))
	e.Anchor = e.Caret
	e.composing, e.preedit = false, ""
	e.changedWithDelta(change, line, removedLines, insertedLines)
	return true
}

func (e *CodeEditor) shiftBreakpoints(first, removed, inserted int) {
	delta := inserted - removed
	if delta == 0 {
		return
	}
	if len(e.breakpoints) > 0 {
		updated := make(map[int]bool, len(e.breakpoints))
		for line := range e.breakpoints {
			switch {
			case line <= first:
				updated[line] = true
			case line > first+removed:
				updated[line+delta] = true
			default:
				updated[first] = true
			}
		}
		e.breakpoints = updated
	}
	if e.debugLine > first+removed {
		e.debugLine += delta
	}
}

func (e *CodeEditor) finishDocChange(change editor.Change) {
	e.visibleMaxLineW = 0
	e.moved(false)
	if e.OnChange != nil {
		e.OnChange(change)
	}
}

func (e *CodeEditor) changed(change editor.Change, fromLine int) {
	e.lastVersion = e.Document.Version()
	clear(e.crossPieceLines)
	e.invalidateLineCache()
	e.styles, e.semantic = nil, nil
	e.rebuildLexStates(fromLine)
	e.finishDocChange(change)
}

func (e *CodeEditor) changedWithDelta(change editor.Change, fromLine, removedLines, insertedLines int) {
	e.lastVersion = e.Document.Version()
	clear(e.crossPieceLines)
	e.invalidateLineCache()
	e.styles, e.semantic = nil, nil
	e.updateLexStatesFrom(fromLine, removedLines, insertedLines)
	e.finishDocChange(change)
}

func (e *CodeEditor) moved(keepX bool) {
	e.retainX = keepX
	e.caretVisible = true
	if e.manager != nil && e.Focused() {
		e.manager.caretDue = e.manager.now().Add(500 * time.Millisecond)
	}
	e.revealCaret()
	e.InvalidatePaint()
}

func (e *CodeEditor) revealCaret() {
	view := e.contentRect()
	if view.Empty() {
		return
	}
	r := e.CaretRect()
	if r.Y < view.Y {
		e.scrollY = maxFloat32(0, e.scrollY-(view.Y-r.Y))
		e.scrollTargetY = e.scrollY
		e.visibleMaxLineW = 0
		e.smoothScrolling = false
	} else if r.Y+r.H > view.Y+view.H {
		e.scrollY = minFloat32(e.maxScrollY(), e.scrollY+r.Y+r.H-view.Y-view.H)
		e.scrollTargetY = e.scrollY
		e.visibleMaxLineW = 0
		e.smoothScrolling = false
	}
	maxX := e.maxScrollX()
	if r.X < view.X+2 {
		e.scrollX = clampFloat32(e.scrollX-(view.X+2-r.X), 0, maxX)
		e.scrollTargetX = e.scrollX
		e.smoothScrolling = false
	} else if r.X > view.X+view.W-2 {
		needed := e.scrollX + (r.X - view.X - view.W + 2)
		e.scrollX = clampFloat32(needed, 0, maxFloat32(maxX, needed))
		e.scrollTargetX = e.scrollX
		e.smoothScrolling = false
	} else if e.scrollX > maxX {
		e.scrollX = maxX
		e.scrollTargetX = maxX
	}
}

func (e *CodeEditor) offsetAt(x, y float32) int {
	view := e.contentRect()
	line := min(e.Document.LineCount()-1, max(0, int((y-view.Y+e.scrollY)/e.rowHeight())))
	start, end, _ := e.Document.LineRange(line)
	text, _ := e.Document.Slice(start, end)
	layout := e.layoutLine(text)
	return start + layout.byteAtX(text, x-view.X+e.scrollX)
}

func (e *CodeEditor) horizontalDestination(direction int, word bool) int {
	line, start, end := e.lineAtCaret()
	text, _ := e.Document.Slice(start, end)
	at := utf8.RuneCountInString(text[:min(max(0, e.Caret-start), len(text))])
	runes := []rune(text)
	if direction < 0 && at == 0 && line > 0 {
		_, previousEnd, _ := e.Document.LineRange(line - 1)
		return previousEnd
	}
	if direction > 0 && at == len(runes) && line+1 < e.Document.LineCount() {
		nextStart, _, _ := e.Document.LineRange(line + 1)
		return nextStart
	}
	if word {
		at = wordBoundary(runes, at, direction)
	} else if direction < 0 {
		at = previousGrapheme(runes, at)
	} else {
		at = nextGrapheme(runes, at)
	}
	return start + textByteOffset(text, at)
}

func (e *CodeEditor) move(to int, extend, keepX bool) {
	e.Caret = e.validCaret(to)
	if !extend {
		e.Anchor = e.Caret
	}
	e.moved(keepX)
}

func (e *CodeEditor) moveHorizontal(direction int, word, extend bool) {
	if e.Caret != e.Anchor && !extend && !word {
		a, b := e.selection()
		if direction < 0 {
			e.move(a, false, false)
		} else {
			e.move(b, false, false)
		}
		return
	}
	e.move(e.horizontalDestination(direction, word), extend, false)
}

func (e *CodeEditor) moveVertical(rows int, extend bool) {
	line, _, _ := e.lineAtCaret()
	if !e.retainX {
		e.desiredX = e.CaretRect().X - e.contentRect().X + e.scrollX
	}
	target := min(max(0, line+rows), e.Document.LineCount()-1)
	start, end, _ := e.Document.LineRange(target)
	text, _ := e.Document.Slice(start, end)
	offset := e.layoutLine(text).byteAtX(text, e.desiredX)
	e.move(start+offset, extend, true)
}

func (e *CodeEditor) delete(backward, word bool) {
	if e.ReadOnly {
		return
	}
	a, b := e.selection()
	if a == b {
		if backward {
			a = e.horizontalDestination(-1, word)
		} else {
			b = e.horizontalDestination(1, word)
		}
	}
	e.ReplaceTextRange(a, b, "")
}

func (e *CodeEditor) indent(outdent bool) {
	if e.ReadOnly {
		return
	}
	a, b := e.selection()
	firstLine, err := e.Document.LineAtOffset(a)
	if err != nil {
		return
	}
	lastLine, err := e.Document.LineAtOffset(b)
	if err != nil {
		return
	}
	lastStart, _, _ := e.Document.LineRange(lastLine)
	if b > a && b == lastStart {
		lastLine--
	}
	if !outdent && firstLine == lastLine && a == b {
		e.ReplaceTextRange(a, b, "\t")
		return
	}
	var edits []editor.Edit
	for line := firstLine; line <= lastLine; line++ {
		start, end, _ := e.Document.LineRange(line)
		if !outdent {
			edits = append(edits, editor.Edit{Start: start, End: start, Text: "\t"})
			continue
		}
		text, _ := e.Document.Slice(start, end)
		remove := 0
		if strings.HasPrefix(text, "\t") {
			remove = 1
		} else {
			for remove < len(text) && remove < max(1, e.TabWidth) && text[remove] == ' ' {
				remove++
			}
		}
		if remove > 0 {
			edits = append(edits, editor.Edit{Start: start, End: start + remove})
		}
	}
	if len(edits) == 0 {
		return
	}
	change, err := e.Document.Apply(edits)
	if err != nil || change.Version == change.BeforeVersion {
		return
	}
	e.Anchor = remapEditorOffset(e.Anchor, edits)
	e.Caret = remapEditorOffset(e.Caret, edits)
	e.changed(change, firstLine)
}

func remapEditorOffset(at int, edits []editor.Edit) int {
	shift := 0
	for _, edit := range edits {
		if at < edit.Start {
			break
		}
		if at <= edit.End {
			return edit.Start + shift + len(edit.Text)
		}
		shift += len(edit.Text) - (edit.End - edit.Start)
	}
	return at + shift
}

func (e *CodeEditor) history(redo bool) {
	if e.ReadOnly {
		return
	}
	var change editor.Change
	var ok bool
	if redo {
		change, ok = e.Document.Redo()
	} else {
		change, ok = e.Document.Undo()
	}
	if !ok {
		return
	}
	if len(change.Edits) > 0 {
		last := change.Edits[len(change.Edits)-1]
		e.Caret = e.validCaret(last.Start + len(last.Text))
	}
	e.Anchor = e.Caret
	if len(change.Edits) == 1 {
		edit := change.Edits[0]
		if line, err := e.Document.LineAtOffset(edit.Start); err == nil {
			if len(e.lexStates) == e.Document.LineCount()+1 && !strings.Contains(edit.Text, "\n") {
				e.changedWithDelta(change, line, 0, 0)
				return
			}
			e.changed(change, line)
			return
		}
	}
	e.changed(change, -1)
}
