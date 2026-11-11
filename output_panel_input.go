package nativeforms

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

func (p *OutputPanel) setScrollFromThumbY(thumbTopY float32, track, thumb Rect) {
	travel := maxFloat32(1, (track.H-4)-thumb.H)
	ratio := clampFloat32((thumbTopY-(track.Y+2))/travel, 0, 1)
	nextY := clampFloat32(ratio*p.maxScrollY(), 0, p.maxScrollY())
	p.targetScrollY = nextY
	p.smoothScrolling = false
	if p.scrollY != nextY {
		p.scrollY = nextY
		p.scrollX = clampFloat32(p.scrollX, 0, p.maxScrollX())
		p.syncListOffsetFromScrollY()
		p.InvalidatePaint()
	}
}

func (p *OutputPanel) syncListOffsetFromScrollY() {
	maxY := p.maxScrollY()
	listMax := maxFloat32(0, float32(len(p.lines))*p.List.rowHeight()-p.List.rect.H)
	if maxY > 0 {
		p.List.Offset = clampFloat32((p.scrollY/maxY)*listMax, 0, listMax)
	} else {
		p.List.Offset = 0
	}
}

func (p *OutputPanel) insertAtPrompt(s string) {
	if s == "" {
		return
	}
	runes := []rune(p.InputText)
	caret := min(max(0, p.InputCaret), len(runes))
	inRunes := []rune(s)
	updated := make([]rune, 0, len(runes)+len(inRunes))
	updated = append(updated, runes[:caret]...)
	updated = append(updated, inRunes...)
	updated = append(updated, runes[caret:]...)
	p.InputText = string(updated)
	p.InputCaret = caret + len(inRunes)
	p.selAnchor, p.selCaret = OutputTextPos{}, OutputTextPos{}
	p.ScrollToEnd()
}

func (p *OutputPanel) submitPrompt() {
	cmd := p.InputText
	prefix := p.promptPrefixForLastLine()
	if p.partial && len(p.lines) > 0 {
		p.lines[len(p.lines)-1] = prefix + cmd
		p.partial = false
	} else {
		p.lines = append(p.lines, prefix+cmd)
	}
	p.trim()
	if strings.TrimSpace(cmd) != "" {
		if len(p.history) == 0 || p.history[len(p.history)-1] != cmd {
			p.history = append(p.history, cmd)
		}
	}
	p.historyIdx = len(p.history)
	p.InputText = ""
	p.InputCaret = 0
	p.preedit = ""
	p.composing = false
	p.selAnchor, p.selCaret = OutputTextPos{}, OutputTextPos{}
	p.List.ModelChanged()
	p.ScrollToEnd()
	if p.OnCommand != nil {
		p.OnCommand(cmd)
	}
}

func (p *OutputPanel) buildContextMenu(x, y float32) []MenuItem {
	items := []MenuItem{
		{
			ID:       "copy",
			Label:    "Копировать выделенное (Copy)",
			Shortcut: "Ctrl+C",
			Disabled: !p.HasSelection(),
			Action:   func() { p.Copy() },
		},
		{
			ID:       "copy_all",
			Label:    "Копировать весь вывод (Copy All)",
			Disabled: len(p.lines) == 0,
			Action:   func() { p.CopyAll() },
		},
		{
			ID:       "select_all",
			Label:    "Выделить всё (Select All)",
			Shortcut: "Ctrl+A",
			Disabled: p.displayLineCount() == 0,
			Action:   func() { p.SelectAll() },
		},
	}
	if p.TerminalMode {
		items = append(items, MenuItem{
			ID:       "paste",
			Label:    "Вставить в строку ввода (Paste)",
			Shortcut: "Ctrl+V",
			Action:   func() { p.Paste() },
		})
	}
	custom := p.State.buildContextMenuItems(x, y)
	if len(custom) == 0 && p.List != nil {
		custom = p.List.UIState().buildContextMenuItems(x, y)
	}
	if len(custom) > 0 {
		items = append(items, MenuItem{Separator: true})
		items = append(items, custom...)
	} else {
		items = append(items,
			MenuItem{Separator: true},
			MenuItem{
				ID:       "scroll_end",
				Label:    "Прокрутить в конец",
				Disabled: len(p.lines) == 0,
				Action:   func() { p.ScrollToEnd() },
			},
			MenuItem{
				ID:       "clear",
				Label:    "Очистить (Clear)",
				Disabled: len(p.lines) == 0,
				Action:   func() { p.Clear() },
			},
		)
	}
	return items
}

func (p *OutputPanel) Handle(event Event) bool {
	switch event.Type {
	case FocusLost:
		p.selecting = false
		p.scrollDrag = false
		p.composing = false
		p.preedit = ""
		p.InvalidatePaint()
		return true
	case PointerDown:
		if event.Button == MouseRight {
			if p.manager != nil {
				items := p.buildContextMenu(event.X, event.Y)
				if len(items) > 0 {
					p.manager.OpenContextMenu(items, event.X, event.Y)
					return true
				}
			}
			return true
		}
		if event.Button != MouseLeft {
			break
		}
		track, thumb := p.scrollbarThumbRect()
		if !track.Empty() && track.Contains(event.X, event.Y) {
			p.scrollDrag = true
			if thumb.Contains(event.X, event.Y) {
				p.scrollDragOff = event.Y - thumb.Y
			} else {
				p.scrollDragOff = thumb.H * 0.5
				p.setScrollFromThumbY(event.Y-p.scrollDragOff, track, thumb)
			}
			p.InvalidatePaint()
			return true
		}
		pos := p.posAt(event.X, event.Y)
		switch updateClickCount(event, &p.lastClick, &p.lastPoint, &p.clicks) {
		case 2:
			lineRunes := []rune(p.displayLine(pos.Line))
			ws, we := selectedWord(lineRunes, min(pos.Col, len(lineRunes)))
			p.selAnchor = OutputTextPos{Line: pos.Line, Col: ws}
			p.selCaret = OutputTextPos{Line: pos.Line, Col: we}
			p.selecting = false
		case 3:
			lineLen := utf8.RuneCountInString(p.displayLine(pos.Line))
			p.selAnchor = OutputTextPos{Line: pos.Line, Col: 0}
			p.selCaret = OutputTextPos{Line: pos.Line, Col: lineLen}
			p.selecting = false
		default:
			if event.Mods&ModShift != 0 {
				p.selCaret = pos
			} else {
				p.selAnchor = pos
				p.selCaret = pos
			}
			p.selecting = true
		}
		p.InvalidatePaint()
		return true
	case PointerMove:
		track, thumb := p.scrollbarThumbRect()
		hover := !track.Empty() && track.Contains(event.X, event.Y)
		if hover != p.scrollHover {
			p.scrollHover = hover
			p.InvalidatePaint()
		}
		if p.scrollDrag {
			if !track.Empty() {
				p.setScrollFromThumbY(event.Y-p.scrollDragOff, track, thumb)
			}
			return true
		}
		if p.selecting && p.Pressed() {
			view := p.contentRect()
			if event.Y < view.Y {
				p.scrollY = clampFloat32(p.scrollY-(view.Y-event.Y)*0.5, 0, p.maxScrollY())
				p.targetScrollY = p.scrollY
			} else if event.Y > view.Y+view.H {
				p.scrollY = clampFloat32(p.scrollY+(event.Y-view.Y-view.H)*0.5, 0, p.maxScrollY())
				p.targetScrollY = p.scrollY
			}
			p.selCaret = p.posAt(event.X, event.Y)
			p.InvalidatePaint()
			return true
		}
	case PointerUp:
		if event.Button == MouseLeft {
			wasDrag := p.scrollDrag || p.selecting
			p.scrollDrag = false
			p.selecting = false
			if wasDrag {
				p.InvalidatePaint()
				return true
			}
		}
	case PointerScroll:
		beforeX, beforeY := p.scrollX, p.scrollY
		maxY := p.maxScrollY()
		maxX := p.maxScrollX()
		if !p.smoothScrolling {
			p.targetScrollY = p.scrollY
		}
		p.targetScrollY = clampFloat32(p.targetScrollY-event.Scroll.Y*48, 0, maxY)
		p.scrollX = clampFloat32(p.scrollX-event.Scroll.X*40, 0, maxX)
		if p.manager == nil {
			p.scrollY = p.targetScrollY
			p.smoothScrolling = false
		} else {
			p.scrollY = clampFloat32(p.scrollY+(p.targetScrollY-p.scrollY)*0.42, 0, maxY)
			p.smoothScrolling = absFloat32(p.targetScrollY-p.scrollY) > 0.35
		}
		p.syncListOffsetFromScrollY()
		if beforeX != p.scrollX || beforeY != p.scrollY || p.smoothScrolling {
			p.InvalidatePaint()
			return true
		}
	case CompositionStart:
		if p.TerminalMode {
			p.composing = true
			p.preedit = ""
			p.InvalidatePaint()
			return true
		}
	case CompositionUpdate:
		if p.TerminalMode {
			p.composing = true
			p.preedit = event.Text
			p.InvalidatePaint()
			return true
		}
	case CompositionEnd:
		if p.TerminalMode {
			p.composing = false
			p.preedit = ""
			if event.Text != "" {
				p.insertAtPrompt(event.Text)
			}
			p.InvalidatePaint()
			return true
		}
	case TextInput:
		if p.TerminalMode && event.Rune >= 32 && event.Rune != 127 {
			p.insertAtPrompt(string(event.Rune))
			return true
		}
	case KeyDown:
		mod := event.Mods&(ModControl|ModSuper) != 0
		if mod {
			switch event.Key {
			case KeyC:
				if p.HasSelection() {
					p.Copy()
					return true
				}
				if p.TerminalMode && event.Mods&ModControl != 0 && p.OnInterrupt != nil {
					p.OnInterrupt()
					return true
				}
				p.CopyAll()
				return true
			case KeyA:
				p.SelectAll()
				return true
			case KeyV:
				if p.TerminalMode {
					p.Paste()
					return true
				}
			case KeyL:
				if p.TerminalMode {
					p.Clear()
					return true
				}
			}
		}
		if p.TerminalMode && !mod {
			runes := []rune(p.InputText)
			caret := min(max(0, p.InputCaret), len(runes))
			switch event.Key {
			case KeyEnter:
				p.submitPrompt()
				return true
			case KeyBackspace:
				if caret > 0 {
					p.InputText = string(append(runes[:caret-1], runes[caret:]...))
					p.InputCaret = caret - 1
					p.ScrollToEnd()
				}
				return true
			case KeyDelete:
				if caret < len(runes) {
					p.InputText = string(append(runes[:caret], runes[caret+1:]...))
					p.InputCaret = caret
					p.ScrollToEnd()
				}
				return true
			case KeyLeftArrow:
				if caret > 0 {
					p.InputCaret = caret - 1
					p.InvalidatePaint()
				}
				return true
			case KeyRightArrow:
				if caret < len(runes) {
					p.InputCaret = caret + 1
					p.InvalidatePaint()
				}
				return true
			case KeyHome:
				p.InputCaret = 0
				p.InvalidatePaint()
				return true
			case KeyEnd:
				p.InputCaret = len(runes)
				p.InvalidatePaint()
				return true
			case KeyUpArrow:
				if len(p.history) > 0 {
					if p.historyIdx > 0 {
						p.historyIdx--
					}
					p.InputText = p.history[p.historyIdx]
					p.InputCaret = utf8.RuneCountInString(p.InputText)
					p.ScrollToEnd()
				}
				return true
			case KeyDownArrow:
				if len(p.history) > 0 {
					if p.historyIdx < len(p.history)-1 {
						p.historyIdx++
						p.InputText = p.history[p.historyIdx]
					} else {
						p.historyIdx = len(p.history)
						p.InputText = ""
					}
					p.InputCaret = utf8.RuneCountInString(p.InputText)
					p.ScrollToEnd()
				}
				return true
			}
			// Swallow printable keystrokes so global shortcuts without modifiers do not steal terminal typing
			if event.Key == KeySpace || (event.Key >= KeyA && event.Key <= KeySlash) {
				return true
			}
		}
		switch event.Key {
		case KeyPageUp:
			p.smoothScrolling = false
			p.scrollY = clampFloat32(p.scrollY-maxFloat32(40, p.contentRect().H-p.visualLineHeight()), 0, p.maxScrollY())
			p.targetScrollY = p.scrollY
			p.syncListOffsetFromScrollY()
			p.InvalidatePaint()
			return true
		case KeyPageDown:
			p.smoothScrolling = false
			p.scrollY = clampFloat32(p.scrollY+maxFloat32(40, p.contentRect().H-p.visualLineHeight()), 0, p.maxScrollY())
			p.targetScrollY = p.scrollY
			p.syncListOffsetFromScrollY()
			p.InvalidatePaint()
			return true
		}
	}
	return p.State.Handle(event)
}

// TextInputClient implementation for interactive TerminalMode.
func (p *OutputPanel) TextInputSnapshot(maxBytes int) (TextInputState, bool) {
	if !p.TerminalMode {
		return TextInputState{}, false
	}
	byteCaret := textByteOffset(p.InputText, p.InputCaret)
	return TextInputState{
		Text:      p.InputText,
		Cursor:    byteCaret,
		Anchor:    byteCaret,
		Base:      0,
		UTF16Base: 0,
		Multiline: false,
	}, true
}

func (p *OutputPanel) ReplaceTextRange(start, end int, value string) bool {
	if !p.TerminalMode || !utf8.ValidString(value) {
		return false
	}
	start = min(max(0, start), len(p.InputText))
	end = min(max(start, end), len(p.InputText))
	p.InputText = p.InputText[:start] + value + p.InputText[end:]
	p.InputCaret = utf8.RuneCountInString(p.InputText[:start+len(value)])
	p.ScrollToEnd()
	return true
}

func (p *OutputPanel) SelectTextRange(start, end int) bool {
	if !p.TerminalMode {
		return false
	}
	end = min(max(0, end), len(p.InputText))
	p.InputCaret = utf8.RuneCountInString(p.InputText[:end])
	p.InvalidatePaint()
	return true
}

func (p *OutputPanel) CaretRect() Rect {
	view := p.contentRect()
	h := p.visualLineHeight()
	promptIdx := max(0, p.promptLineIndex())
	lineText := p.displayLine(promptIdx)
	prefixRunes := utf8.RuneCountInString(p.promptPrefixForLastLine())
	caretCol := prefixRunes + min(max(0, p.InputCaret), utf8.RuneCountInString(p.InputText))
	cx := view.X - p.scrollX + p.colX(lineText, caretCol, p.fontSpec())
	cy := view.Y + float32(promptIdx)*h - p.scrollY
	return Rect{X: cx, Y: cy, W: 2, H: h}
}

func (p *OutputPanel) TextInputComposing() bool { return p.composing }
func (p *OutputPanel) ResetCaretBlink()         { p.caretVisible = true; p.InvalidatePaint() }
func (p *OutputPanel) BlinkCaret()              { p.caretVisible = !p.caretVisible; p.InvalidatePaint() }

func isWordRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '.' || r == '/' || r == '-'
}
