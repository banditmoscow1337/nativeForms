package nativeforms

import "unicode/utf8"

func (e *CodeEditor) setScrollFromThumbY(thumbTopY float32, track, thumb Rect) {
	travel := maxFloat32(1, (track.H-4)-thumb.H)
	ratio := clampFloat32((thumbTopY-(track.Y+2))/travel, 0, 1)
	nextY := clampFloat32(ratio*e.maxScrollY(), 0, e.maxScrollY())
	e.scrollTargetY = nextY
	e.smoothScrolling = false
	if e.scrollY != nextY {
		e.scrollY = nextY
		e.visibleMaxLineW = 0
		e.scrollX = clampFloat32(e.scrollX, 0, e.maxScrollX())
		e.scrollTargetX = e.scrollX
		e.caretRectValid = false
		e.InvalidatePaint()
	}
}

func (e *CodeEditor) Handle(event Event) bool {
	e.syncDocument()
	switch event.Type {
	case FocusLost:
		e.HideCompletions()
		e.composing, e.preedit = false, ""
		e.caretVisible = true
		e.scrollDrag = false
		e.InvalidatePaint()
		return true
	case PointerDown:
		if event.Button == MouseRight {
			if e.State.onEvent != nil && e.State.onEvent(event) {
				return true
			}
			view := e.contentRect()
			if event.X >= view.X && event.X < view.X+view.W {
				at := e.offsetAt(event.X, event.Y)
				a, b := e.selection()
				if a == b || at < a || at > b {
					e.move(at, false, false)
				}
			} else if event.X < view.X {
				line := min(e.Document.LineCount()-1, max(0, int((event.Y-view.Y+e.scrollY)/e.rowHeight())))
				start, _, _ := e.Document.LineRange(line)
				e.move(start, false, false)
			}
			return e.OpenContextMenu(event.X, event.Y) != 0
		}
		if event.Button != MouseLeft {
			break
		}
		track, thumb := e.scrollbarThumbRect()
		if !track.Empty() && track.Contains(event.X, event.Y) {
			e.scrollDrag = true
			if thumb.Contains(event.X, event.Y) {
				e.scrollDragOffset = event.Y - thumb.Y
			} else {
				e.scrollDragOffset = thumb.H * 0.5
				e.setScrollFromThumbY(event.Y-e.scrollDragOffset, track, thumb)
			}
			e.InvalidatePaint()
			return true
		}
		view := e.contentRect()
		if event.X < view.X {
			line := min(e.Document.LineCount()-1, max(0, int((event.Y-view.Y+e.scrollY)/e.rowHeight())))
			e.SetBreakpoint(line, !e.breakpoints[line])
			return true
		}
		at := e.offsetAt(event.X, event.Y)
		switch updateClickCount(event, &e.lastClick, &e.lastPoint, &e.clicks) {
		case 2, 3:
			line, _ := e.Document.LineAtOffset(at)
			start, end, _ := e.Document.LineRange(line)
			if e.clicks == 3 {
				e.Anchor, e.Caret = start, end
			} else {
				text, _ := e.Document.Slice(start, end)
				wordStart, wordEnd := selectedWord([]rune(text), utf8.RuneCountInString(text[:at-start]))
				e.Anchor, e.Caret = start+textByteOffset(text, wordStart), start+textByteOffset(text, wordEnd)
			}
			e.moved(false)
		default:
			e.move(at, event.Mods&ModShift != 0, false)
		}
		return true
	case PointerMove:
		track, thumb := e.scrollbarThumbRect()
		hover := !track.Empty() && track.Contains(event.X, event.Y)
		if hover != e.scrollHover {
			e.scrollHover = hover
			e.InvalidatePaint()
		}
		if e.scrollDrag {
			if !track.Empty() {
				e.setScrollFromThumbY(event.Y-e.scrollDragOffset, track, thumb)
			}
			return true
		}
		if e.Pressed() {
			e.move(e.offsetAt(event.X, event.Y), true, false)
			return true
		}
	case PointerUp:
		if e.scrollDrag && event.Button == MouseLeft {
			e.scrollDrag = false
			e.InvalidatePaint()
			return true
		}
	case PointerScroll:
		beforeX, beforeY := e.scrollX, e.scrollY
		maxY := e.maxScrollY()
		maxX := e.maxScrollX()
		if !e.smoothScrolling {
			e.scrollTargetX = e.scrollX
			e.scrollTargetY = e.scrollY
		}
		e.scrollTargetX = clampFloat32(e.scrollTargetX-event.Scroll.X*48, 0, maxX)
		e.scrollTargetY = clampFloat32(e.scrollTargetY-event.Scroll.Y*48, 0, maxY)
		if e.manager == nil {
			e.scrollX = e.scrollTargetX
			e.scrollY = e.scrollTargetY
			e.smoothScrolling = false
		} else {
			e.scrollX = clampFloat32(e.scrollX+(e.scrollTargetX-e.scrollX)*0.42, 0, maxX)
			e.scrollY = clampFloat32(e.scrollY+(e.scrollTargetY-e.scrollY)*0.42, 0, maxY)
			e.smoothScrolling = absFloat32(e.scrollTargetX-e.scrollX) > 0.35 || absFloat32(e.scrollTargetY-e.scrollY) > 0.35
		}
		if beforeY != e.scrollY {
			e.visibleMaxLineW = 0
		}
		if beforeX != e.scrollX || beforeY != e.scrollY || e.smoothScrolling {
			e.caretRectValid = false
			e.InvalidatePaint()
			return true
		}
	case CompositionStart:
		e.HideCompletions()
		e.composing = !e.ReadOnly
		e.preedit = ""
		e.InvalidatePaint()
		return true
	case CompositionUpdate:
		e.HideCompletions()
		if !e.ReadOnly {
			e.composing = true
			e.preedit = event.Text
			e.preeditCaret = utf8.RuneCountInString(event.Text)
			if event.CompositionSelection {
				e.preeditCaret = min(max(0, event.CompositionCursor), e.preeditCaret)
			}
			e.InvalidatePaint()
		}
		return true
	case CompositionEnd:
		e.composing, e.preedit = false, ""
		e.InvalidatePaint()
		if event.Text != "" {
			e.replaceSelection(event.Text)
		}
		return true
	case TextInput:
		if event.Rune >= 32 && event.Rune != 127 {
			e.replaceSelection(string(event.Rune))
			return true
		}
	case KeyDown:
		if event.Key == KeyEscape && e.composing {
			e.composing, e.preedit = false, ""
			e.InvalidatePaint()
			return true
		}
		mod := event.Mods&(ModControl|ModSuper) != 0
		extend := event.Mods&ModShift != 0
		if mod && handleEditorClipboardShortcut(e.manager, event.Key, true, !e.ReadOnly, e.selectedText, func(s string) { e.replaceSelection(s) }) {
			return true
		}
		switch {
		case mod && event.Key == KeyA:
			e.SetSelection(0, e.Document.Len())
			return true
		case mod && event.Key == KeyZ:
			e.history(extend)
			return true
		case mod && event.Key == KeyY:
			e.history(true)
			return true
		case !mod && extend && event.Key == KeyF10:
			caret := e.CaretRect()
			return e.OpenContextMenu(caret.X, caret.Y+caret.H) != 0
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
				e.replaceSelection("\n")
			}
			return true
		case KeyTab:
			e.indent(extend)
			return true
		case KeyF9:
			line, _, _ := e.lineAtCaret()
			e.SetBreakpoint(line, !e.Breakpoint(line))
			return true
		case KeyF12:
			if e.GoToDefinition() {
				return true
			}
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
