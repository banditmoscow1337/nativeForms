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
	if _, err := e.Document.UTF16Position(anchor); err != nil {
		return false
	}
	if _, err := e.Document.UTF16Position(caret); err != nil {
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

func (e *CodeEditor) ReplaceTextRange(start, end int, value string) bool {
	e.syncDocument()
	if e.ReadOnly || !utf8.ValidString(value) || start < 0 || end < start {
		return false
	}
	removed, err := e.Document.Slice(start, end)
	if err != nil {
		return false
	}
	pos, err := e.Document.UTF16Position(start)
	if err != nil {
		return false
	}
	change, err := e.Document.Apply([]editor.Edit{{Start: start, End: end, Text: value}})
	if err != nil || change.Version == change.BeforeVersion {
		return false
	}
	e.shiftBreakpoints(pos.Line, strings.Count(removed, "\n"), strings.Count(value, "\n"))
	e.Caret = e.validCaret(start+len(value))
	e.Anchor = e.Caret
	e.composing, e.preedit = false, ""
	e.changed(change, pos.Line)
	return true
}

func (e *CodeEditor) shiftBreakpoints(first, removed, inserted int) {
	if removed == inserted || len(e.breakpoints) == 0 {
		return
	}
	updated := make(map[int]bool, len(e.breakpoints))
	for line := range e.breakpoints {
		switch {
		case line <= first:
			updated[line] = true
		case line > first+removed:
			updated[line+inserted-removed] = true
		default:
			updated[first] = true
		}
	}
	e.breakpoints = updated
}

func (e *CodeEditor) changed(change editor.Change, fromLine int) {
	e.lastVersion = e.Document.Version()
	e.styles, e.semantic = nil, nil
	if fromLine < 0 || len(e.lexStates) == 0 {
		e.lexStates = []goLexState{goNormal}
	} else {
		e.lexStates = e.lexStates[:min(len(e.lexStates), fromLine+1)]
	}
	e.moved(false)
	if e.OnChange != nil {
		e.OnChange(change)
	}
}

func (e *CodeEditor) moved(keepX bool) {
	e.retainX = keepX
	e.caretVisible = true
	if e.manager != nil && e.Focused() {
		e.manager.caretDue = e.manager.now().Add(500 * time.Millisecond)
	}
	e.revealCaret()
	e.invalidatePaint()
}

func (e *CodeEditor) revealCaret() {
	view := e.contentRect()
	if view.Empty() {
		return
	}
	r := e.CaretRect()
	if r.Y < view.Y {
		e.scrollY = maxFloat32(0, e.scrollY-(view.Y-r.Y))
	} else if r.Y+r.H > view.Y+view.H {
		e.scrollY = minFloat32(e.maxScrollY(), e.scrollY+r.Y+r.H-view.Y-view.H)
	}
	if r.X < view.X+2 {
		e.scrollX = maxFloat32(0, e.scrollX-(view.X+2-r.X))
	} else if r.X > view.X+view.W-2 {
		e.scrollX += r.X-view.X-view.W+2
	}
}

func (e *CodeEditor) offsetAt(x, y float32) int {
	view := e.contentRect()
	line := min(e.Document.LineCount()-1, max(0, int((y-view.Y+e.scrollY)/e.rowHeight())))
	start, end, _ := e.Document.LineRange(line)
	text, _ := e.Document.Slice(start, end)
	layout := e.layoutLine(text)
	return start+layout.byteAtX(text, x-view.X+e.scrollX)
}

func (e *CodeEditor) horizontalDestination(direction int, word bool) int {
	line, start, end := e.lineAtCaret()
	text, _ := e.Document.Slice(start, end)
	at := utf8.RuneCountInString(text[:min(max(0, e.Caret-start), len(text))])
	runes := []rune(text)
	if direction < 0 && at == 0 && line > 0 {
		_, previousEnd, _ := e.Document.LineRange(line-1)
		return previousEnd
	}
	if direction > 0 && at == len(runes) && line+1 < e.Document.LineCount() {
		nextStart, _, _ := e.Document.LineRange(line+1)
		return nextStart
	}
	if word {
		at = wordBoundary(runes, at, direction)
	} else if direction < 0 {
		at = previousGrapheme(runes, at)
	} else {
		at = nextGrapheme(runes, at)
	}
	return start+textByteOffset(text, at)
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
		e.desiredX = e.CaretRect().X-e.contentRect().X+e.scrollX
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
	first, err := e.Document.UTF16Position(a)
	if err != nil {
		return
	}
	last, err := e.Document.UTF16Position(b)
	if err != nil {
		return
	}
	lastStart, _, _ := e.Document.LineRange(last.Line)
	if b > a && b == lastStart {
		last.Line--
	}
	if !outdent && first.Line == last.Line && a == b {
		e.ReplaceTextRange(a, b, "\t")
		return
	}
	var edits []editor.Edit
	for line := first.Line; line <= last.Line; line++ {
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
			edits = append(edits, editor.Edit{Start: start, End: start+remove})
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
	e.changed(change, first.Line)
}

func remapEditorOffset(at int, edits []editor.Edit) int {
	shift := 0
	for _, edit := range edits {
		if at < edit.Start {
			break
		}
		if at <= edit.End {
			return edit.Start+shift+len(edit.Text)
		}
		shift += len(edit.Text)-(edit.End-edit.Start)
	}
	return at+shift
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
		e.Caret = e.validCaret(last.Start+len(last.Text))
	}
	e.Anchor = e.Caret
	e.changed(change, -1)
}

func (e *CodeEditor) Handle(event Event) bool {
	e.syncDocument()
	switch event.Type {
	case FocusLost:
		e.composing, e.preedit = false, ""
		e.caretVisible = true
		e.invalidatePaint()
		return true
	case PointerDown:
		if event.Button != MouseLeft {
			break
		}
		view := e.contentRect()
		if event.X < view.X {
			line := min(e.Document.LineCount()-1, max(0, int((event.Y-view.Y+e.scrollY)/e.rowHeight())))
			e.SetBreakpoint(line, !e.breakpoints[line])
			return true
		}
		point := Vec2{X: event.X, Y: event.Y}
		if event.Clicks > 0 {
			e.clicks = event.Clicks
		} else if time.Since(e.lastClick) < 500*time.Millisecond &&
			absFloat32(point.X-e.lastPoint.X) < 5 && absFloat32(point.Y-e.lastPoint.Y) < 5 {
			e.clicks = e.clicks%3+1
		} else {
			e.clicks = 1
		}
		e.lastClick, e.lastPoint = time.Now(), point
		at := e.offsetAt(event.X, event.Y)
		if e.clicks == 2 || e.clicks == 3 {
			pos, _ := e.Document.UTF16Position(at)
			start, end, _ := e.Document.LineRange(pos.Line)
			if e.clicks == 3 {
				e.Anchor, e.Caret = start, end
			} else {
				text, _ := e.Document.Slice(start, end)
				wordStart, wordEnd := selectedWord([]rune(text), utf8.RuneCountInString(text[:at-start]))
				e.Anchor, e.Caret = start+textByteOffset(text, wordStart), start+textByteOffset(text, wordEnd)
			}
			e.moved(false)
		} else {
			e.move(at, event.Mods&ModShift != 0, false)
		}
		return true
	case PointerMove:
		if e.Pressed() {
			e.move(e.offsetAt(event.X, event.Y), true, false)
			return true
		}
	case PointerScroll:
		beforeX, beforeY := e.scrollX, e.scrollY
		e.scrollX = maxFloat32(0, e.scrollX-event.Scroll.X*40)
		e.scrollY = clampFloat32(e.scrollY-event.Scroll.Y*40, 0, e.maxScrollY())
		if beforeX != e.scrollX || beforeY != e.scrollY {
			e.invalidatePaint()
			return true
		}
	case CompositionStart:
		e.composing = !e.ReadOnly
		e.preedit = ""
		e.invalidatePaint()
		return true
	case CompositionUpdate:
		if !e.ReadOnly {
			e.composing = true
			e.preedit = event.Text
			e.preeditCaret = utf8.RuneCountInString(event.Text)
			if event.CompositionSelection {
				e.preeditCaret = min(max(0, event.CompositionCursor), e.preeditCaret)
			}
			e.invalidatePaint()
		}
		return true
	case CompositionEnd:
		e.composing, e.preedit = false, ""
		e.invalidatePaint()
		if event.Text != "" {
			a, b := e.selection()
			e.ReplaceTextRange(a, b, event.Text)
		}
		return true
	case TextInput:
		if event.Rune >= 32 && event.Rune != 127 {
			a, b := e.selection()
			e.ReplaceTextRange(a, b, string(event.Rune))
			return true
		}
	case KeyDown:
		if event.Key == KeyEscape && e.composing {
			e.composing, e.preedit = false, ""
			e.invalidatePaint()
			return true
		}
		mod := event.Mods&(ModControl|ModSuper) != 0
		extend := event.Mods&ModShift != 0
		switch {
		case mod && event.Key == KeyA:
			e.SetSelection(0, e.Document.Len())
			return true
		case mod && event.Key == KeyC:
			if e.manager != nil && e.manager.clipboardWrite != nil {
				_ = e.manager.clipboardWrite(e.selectedText())
			}
			return true
		case mod && event.Key == KeyX:
			if !e.ReadOnly && e.Caret != e.Anchor && e.manager != nil && e.manager.clipboardWrite != nil &&
				e.manager.clipboardWrite(e.selectedText()) == nil {
				a, b := e.selection()
				e.ReplaceTextRange(a, b, "")
			}
			return true
		case mod && event.Key == KeyV:
			if !e.ReadOnly && e.manager != nil && e.manager.clipboardRead != nil {
				if value, err := e.manager.clipboardRead(); err == nil {
					a, b := e.selection()
					e.ReplaceTextRange(a, b, value)
				}
			}
			return true
		case mod && event.Key == KeyZ:
			e.history(extend)
			return true
		case mod && event.Key == KeyY:
			e.history(true)
			return true
		}
		switch event.Key {
		case KeyLeftArrow:
			e.moveHorizontal(-1, mod, extend)
			return true
		case KeyRightArrow:
			e.moveHorizontal(1, mod, extend)
			return true
		case KeyUpArrow, KeyDownArrow, KeyPageUp, KeyPageDown:
			rows := 1
			if event.Key == KeyPageUp || event.Key == KeyPageDown {
				rows = max(1, int(e.contentRect().H/e.rowHeight())-1)
			}
			if event.Key == KeyUpArrow || event.Key == KeyPageUp {
				rows = -rows
			}
			e.moveVertical(rows, extend)
			return true
		case KeyHome, KeyEnd:
			_, start, end := e.lineAtCaret()
			if mod {
				start, end = 0, e.Document.Len()
			}
			if event.Key == KeyHome {
				e.move(start, extend, false)
			} else {
				e.move(end, extend, false)
			}
			return true
		case KeyBackspace, KeyDelete:
			e.delete(event.Key == KeyBackspace, mod)
			return true
		case KeyEnter:
			if !e.ReadOnly {
				a, b := e.selection()
				e.ReplaceTextRange(a, b, "\n")
			}
			return true
		case KeyTab:
			e.indent(extend)
			return true
		case KeyF9:
			line, _, _ := e.lineAtCaret()
			e.SetBreakpoint(line, !e.Breakpoint(line))
			return true
		}
		if !mod {
			switch event.Key {
			case KeySpace, KeyW, KeyA, KeyS, KeyD, KeyQ, KeyE, KeyR, KeyF,
				KeyZ, KeyY, KeyC, KeyV, KeyX:
				return true
			}
			if event.Key >= KeyB && event.Key <= KeySlash {
				return true
			}
		}
	}
	return e.State.Handle(event)
}
