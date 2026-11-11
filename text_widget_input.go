package nativeforms

import (
	"time"
	"unicode/utf8"
)

func (f *TextField) positionAt(x, y float32) int {
	inner, r, lines, h := f.textGeometry()
	row := min(max(int((y-inner.Y+f.scrollY)/h), 0), len(lines)-1)
	line := lines[row]
	local := x - inner.X + f.scrollX
	text := string(r[line.start:line.end])
	positions := f.caretPositions(text, f.fontSpec())
	best, distance := line.start, float32(1e30)
	for at := line.start; ; at = nextGrapheme(r, at) {
		x := positions[at-line.start]
		if d := absFloat32(local - x); d < distance {
			best, distance = at, d
		}
		if at >= line.end {
			break
		}
	}
	return best
}

func updateClickCount(event Event, lastClick *time.Time, lastPoint *Vec2, clicks *int) int {
	point := Vec2{X: event.X, Y: event.Y}
	if event.Clicks > 0 {
		*clicks = event.Clicks
	} else if time.Since(*lastClick) < 500*time.Millisecond &&
		absFloat32(point.X-lastPoint.X) < 5 && absFloat32(point.Y-lastPoint.Y) < 5 {
		*clicks = *clicks%3 + 1
	} else {
		*clicks = 1
	}
	*lastClick, *lastPoint = time.Now(), point
	return *clicks
}

func handleEditorClipboardShortcut(manager *Manager, key Key, canCopy, canModify bool, selected func() string, replace func(string)) bool {
	switch key {
	case KeyC:
		if canCopy && manager != nil && manager.clipboardWrite != nil {
			_ = manager.clipboardWrite(selected())
		}
		return true
	case KeyX:
		if canCopy && canModify && manager != nil && manager.clipboardWrite != nil {
			if sel := selected(); sel != "" && manager.clipboardWrite(sel) == nil {
				replace("")
			}
		}
		return true
	case KeyV:
		if canModify && manager != nil && manager.clipboardRead != nil {
			if value, err := manager.clipboardRead(); err == nil {
				replace(value)
			}
		}
		return true
	}
	return false
}

// BuildContextMenu returns the context menu items for a TextField / TextArea.
func (f *TextField) BuildContextMenu() []MenuItem {
	f.syncExternal()
	if custom := f.State.buildContextMenuItems(0, 0); len(custom) > 0 {
		return custom
	}
	e := f.editor
	a, b := e.selection()
	hasSel := a < b
	canCopy := !f.Password && hasSel
	canModify := !f.ReadOnly

	return []MenuItem{
		{
			ID:       "cut",
			Label:    "Cut",
			Shortcut: "Ctrl+X",
			Disabled: !canCopy || !canModify,
			Action: func() {
				handleEditorClipboardShortcut(f.manager, KeyX, !f.Password, !f.ReadOnly, e.Selection, func(s string) { f.insert(s) })
			},
		},
		{
			ID:       "copy",
			Label:    "Copy",
			Shortcut: "Ctrl+C",
			Disabled: !canCopy,
			Action: func() {
				handleEditorClipboardShortcut(f.manager, KeyC, !f.Password, !f.ReadOnly, e.Selection, func(s string) { f.insert(s) })
			},
		},
		{
			ID:       "paste",
			Label:    "Paste",
			Shortcut: "Ctrl+V",
			Disabled: !canModify,
			Action: func() {
				handleEditorClipboardShortcut(f.manager, KeyV, !f.Password, !f.ReadOnly, e.Selection, func(s string) { f.insert(s) })
			},
		},
		{
			ID:       "select_all",
			Label:    "Select All",
			Shortcut: "Ctrl+A",
			Disabled: len(e.Text) == 0,
			Action: func() {
				e.SelectAll()
				f.moved()
			},
		},
		{Separator: true},
		{
			ID:       "undo",
			Label:    "Undo",
			Shortcut: "Ctrl+Z",
			Disabled: !canModify || len(e.undo) == 0,
			Action: func() {
				if !f.ReadOnly && e.Undo() {
					f.changed()
				}
			},
		},
		{
			ID:       "redo",
			Label:    "Redo",
			Shortcut: "Ctrl+Y",
			Disabled: !canModify || len(e.redo) == 0,
			Action: func() {
				if !f.ReadOnly && e.Redo() {
					f.changed()
				}
			},
		},
	}
}

// OpenContextMenu opens the TextField's context menu at (x, y).
func (f *TextField) OpenContextMenu(x, y float32) PopupID {
	if f.manager == nil || !f.Enabled() {
		return 0
	}
	items := f.BuildContextMenu()
	if len(items) == 0 {
		return 0
	}
	return f.manager.OpenContextMenu(items, x, y)
}

func (f *TextField) Handle(event Event) bool {
	f.syncExternal()
	e := f.editor
	switch event.Type {
	case FocusLost:
		e.Preedit = ""
		f.compositionCache = nil
		f.composing = false
		f.caretVisible = true
		if f.OnSubmit != nil {
			f.OnSubmit(f.Text)
		}
		return true
	case PointerDown:
		if event.Button == MouseRight {
			if f.State.onEvent != nil && f.State.onEvent(event) {
				return true
			}
			pos := f.positionAt(event.X, event.Y)
			a, b := e.selection()
			if a == b || pos < a || pos > b {
				e.moveRunes([]rune(e.Text), pos, false)
				f.moved()
			}
			return f.OpenContextMenu(event.X, event.Y) != 0
		}
		if event.Button != MouseLeft {
			return false
		}
		pos := f.positionAt(event.X, event.Y)
		r := []rune(e.Text)
		switch updateClickCount(event, &f.lastClick, &f.lastPoint, &f.clicks) {
		case 2:
			start, end := selectedWord(r, pos)
			e.moveRunes(r, start, false)
			e.moveRunes(r, end, true)
		case 3:
			start, end := lineBoundaries(r, pos)
			e.moveRunes(r, start, false)
			e.moveRunes(r, end, true)
		default:
			e.moveRunes(r, pos, event.Mods&ModShift != 0)
		}
		f.moved()
		return true
	case PointerMove:
		if f.Pressed() {
			inner, _, lines, h := f.textGeometry()
			if f.Multiline {
				if event.Y < inner.Y {
					f.scrollY = maxFloat32(0, f.scrollY-(inner.Y-event.Y))
				}
				if event.Y > inner.Y+inner.H {
					f.scrollY = minFloat32(maxFloat32(0, float32(len(lines))*h-inner.H), f.scrollY+event.Y-inner.Y-inner.H)
				}
			}
			if event.X < inner.X {
				f.scrollX = maxFloat32(0, f.scrollX-(inner.X-event.X))
			}
			if event.X > inner.X+inner.W {
				f.scrollX += event.X - inner.X - inner.W
			}
			f.editor.Move(f.positionAt(event.X, event.Y), true)
			f.moved()
			return true
		}
	case PointerScroll:
		if f.Multiline && event.Scroll.Y != 0 {
			inner, _, lines, h := f.textGeometry()
			before := f.scrollY
			f.scrollY = clampFloat32(f.scrollY-event.Scroll.Y*40, 0, maxFloat32(0, float32(len(lines))*h-inner.H))
			if before != f.scrollY {
				f.InvalidatePaint()
				return true
			}
		}
	case CompositionStart:
		e.Preedit = ""
		f.composing = !f.ReadOnly
		f.InvalidatePaint()
		return true
	case CompositionUpdate:
		if f.ReadOnly {
			return true
		}
		f.composing = true
		e.Preedit = event.Text
		e.PreeditCaret = utf8.RuneCountInString(event.Text)
		e.PreeditAnchor = e.PreeditCaret
		if event.CompositionSelection {
			e.PreeditCaret = min(e.PreeditCaret, max(-1, event.CompositionCursor))
			e.PreeditAnchor = min(utf8.RuneCountInString(event.Text), max(-1, event.CompositionAnchor))
		}
		f.InvalidatePaint()
		return true
	case CompositionEnd:
		f.composing = false
		e.Preedit = ""
		f.compositionCache = nil
		f.InvalidatePaint()
		if event.Text != "" {
			return f.insert(event.Text)
		}
		return true
	case TextInput:
		if event.Rune >= 32 && event.Rune != 127 {
			return f.insert(string(event.Rune))
		}
	case KeyDown:
		if event.Key == KeyEscape && f.composing {
			f.composing = false
			e.Preedit = ""
			f.InvalidatePaint()
			return true
		}
		mod := event.Mods&(ModControl|ModSuper) != 0
		if mod && handleEditorClipboardShortcut(f.manager, event.Key, !f.Password, !f.ReadOnly, e.Selection, func(s string) { f.insert(s) }) {
			return true
		}
		switch {
		case mod && event.Key == KeyA:
			e.SelectAll()
			f.moved()
			return true
		case mod && event.Key == KeyZ:
			if !f.ReadOnly {
				changed := false
				if event.Mods&ModShift != 0 {
					changed = e.Redo()
				} else {
					changed = e.Undo()
				}
				if changed {
					f.changed()
				}
			}
			return true
		case mod && event.Key == KeyY:
			if !f.ReadOnly && e.Redo() {
				f.changed()
			}
			return true
		case !mod && event.Mods&ModShift != 0 && event.Key == KeyF10:
			caret := f.CaretRect()
			return f.OpenContextMenu(caret.X, caret.Y+caret.H) != 0
		}
		switch event.Key {
		case KeyLeftArrow:
			e.Step(-1, mod, event.Mods&ModShift != 0)
			f.moved()
			return true
		case KeyRightArrow:
			e.Step(1, mod, event.Mods&ModShift != 0)
			f.moved()
			return true
		case KeyHome, KeyEnd:
			r := []rune(e.Text)
			start, end := lineBoundaries(r, e.Caret)
			if mod {
				start, end = 0, len(r)
			}
			if event.Key == KeyHome {
				e.moveRunes(r, start, event.Mods&ModShift != 0)
			} else {
				e.moveRunes(r, end, event.Mods&ModShift != 0)
			}
			f.moved()
			return true
		case KeyUpArrow, KeyDownArrow:
			if f.Multiline {
				caret := f.CaretRect()
				delta := f.Theme().FontSize * 1.2
				if event.Key == KeyUpArrow {
					delta = -delta
				}
				e.Move(f.positionAt(caret.X, caret.Y+delta), event.Mods&ModShift != 0)
				f.moved()
				return true
			}
		case KeyBackspace, KeyDelete:
			f.delete(event.Key == KeyBackspace, mod)
			return true
		case KeyEnter:
			if f.Multiline && !mod {
				f.insert("\n")
			} else if f.OnSubmit != nil {
				f.OnSubmit(f.Text)
			}
			return true
		case KeyW, KeyA, KeyS, KeyD, KeyQ, KeyE, KeyR, KeyF, KeyZ, KeyY, KeyC, KeyV, KeyX, KeySpace:
			if !mod {
				return true
			}
		}
	}
	return f.State.Handle(event)
}
