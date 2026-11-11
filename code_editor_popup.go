package nativeforms

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// ShowCompletions opens a menu at the caret while native text input remains
// focused on the editor. MenuItem actions apply the chosen completion; callers
// can replace the list after a language server response by calling this again.
func (e *CodeEditor) ShowCompletions(items []MenuItem) PopupID {
	e.HideCompletions()
	if len(items) == 0 || e.manager == nil || !e.Focused() || e.composing {
		return 0
	}
	menu := NewMenu(items, nil)
	menu.MaxVisibleItems = 8
	id := e.manager.OpenUnfocusedPopup(menu, PopupOptions{
		Owner:  e,
		Anchor: e.CaretRect,
		OnKeyDown: func(event Event) bool {
			if event.Mods != 0 {
				return false
			}
			switch event.Key {
			case KeyUpArrow, KeyDownArrow, KeyHome, KeyEnd, KeyEnter:
				return menu.Handle(event)
			}
			return false
		},
	})
	e.completionPopup = id
	return id
}

// HideCompletions closes the current completion menu, if it is still open.
func (e *CodeEditor) HideCompletions() bool {
	id := e.completionPopup
	e.completionPopup = 0
	return id != 0 && e.manager != nil && e.manager.ClosePopup(id)
}

func isIdentRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_'
}

// SymbolAtOffset returns the identifier surrounding byteOffset in the document,
// along with its [start, end) byte range.
func (e *CodeEditor) SymbolAtOffset(byteOffset int) (symbol string, start, end int) {
	e.syncDocument()
	byteOffset = e.validCaret(byteOffset)
	line, err := e.Document.LineAtOffset(byteOffset)
	if err != nil {
		return "", byteOffset, byteOffset
	}
	lineStart, lineEnd, err := e.Document.LineRange(line)
	if err != nil || lineStart >= lineEnd {
		return "", byteOffset, byteOffset
	}
	text := e.sliceLine(line, lineStart, lineEnd, false)
	local := min(max(0, byteOffset-lineStart), len(text))
	if local == len(text) && local > 0 {
		if r, sz := utf8.DecodeLastRuneInString(text[:local]); isIdentRune(r) {
			local -= sz
		}
	} else if local < len(text) {
		r, _ := utf8.DecodeRuneInString(text[local:])
		if !isIdentRune(r) && local > 0 {
			if prev, sz := utf8.DecodeLastRuneInString(text[:local]); isIdentRune(prev) {
				local -= sz
			}
		}
	}
	if local >= len(text) {
		return "", byteOffset, byteOffset
	}
	r, _ := utf8.DecodeRuneInString(text[local:])
	if !isIdentRune(r) || unicode.IsDigit(r) && local == 0 {
		return "", byteOffset, byteOffset
	}
	s := local
	for s > 0 {
		prev, sz := utf8.DecodeLastRuneInString(text[:s])
		if !isIdentRune(prev) {
			break
		}
		s -= sz
	}
	en := local
	for en < len(text) {
		next, sz := utf8.DecodeRuneInString(text[en:])
		if !isIdentRune(next) {
			break
		}
		en += sz
	}
	firstRune, _ := utf8.DecodeRuneInString(text[s:en])
	if unicode.IsDigit(firstRune) {
		return "", byteOffset, byteOffset
	}
	return text[s:en], lineStart + s, lineStart + en
}

// SymbolAtCaret returns the identifier under the caret or the current selection
// if the selection is a single identifier.
func (e *CodeEditor) SymbolAtCaret() (symbol string, start, end int) {
	e.syncDocument()
	a, b := e.selection()
	if a < b && b-a <= 128 {
		if sel, err := e.Document.Slice(a, b); err == nil && isValidIdentifier(sel) {
			return sel, a, b
		}
	}
	return e.SymbolAtOffset(e.Caret)
}

func isValidIdentifier(s string) bool {
	if s == "" {
		return false
	}
	for i, r := range s {
		if i == 0 && unicode.IsDigit(r) {
			return false
		}
		if !isIdentRune(r) {
			return false
		}
	}
	return true
}

// FindDefinitionOffset searches the document for a declaration of symbol
// (such as func, type, var, const, or := assignment) and returns its byte offset.
func (e *CodeEditor) FindDefinitionOffset(symbol string) (int, bool) {
	e.syncDocument()
	if !isValidIdentifier(symbol) {
		return 0, false
	}
	foundOffset := -1
	fallbackOffset := -1
	lineOffset := 0
	e.scratchLineBytes = e.Document.ScanLinesFrom(0, e.scratchLineBytes[:0], func(line int, text string) bool {
		if idx := matchGoDeclarationInLine(text, symbol); idx >= 0 {
			start, _, err := e.Document.LineRange(line)
			if err == nil {
				foundOffset = start + idx
				return false
			}
		}
		if fallbackOffset < 0 {
			if idx := indexIdentifierInLine(text, symbol); idx >= 0 {
				start, _, err := e.Document.LineRange(line)
				if err == nil {
					fallbackOffset = start + idx
				}
			}
		}
		lineOffset++
		return true
	})
	_ = lineOffset
	if foundOffset >= 0 {
		return foundOffset, true
	}
	if fallbackOffset >= 0 {
		return fallbackOffset, true
	}
	return 0, false
}

func matchGoDeclarationInLine(line, symbol string) int {
	trimmed := strings.TrimLeft(line, " \t")
	lead := len(line) - len(trimmed)
	for _, kw := range [...]string{"func ", "type ", "var ", "const "} {
		if strings.HasPrefix(trimmed, kw) {
			rest := trimmed[len(kw):]
			restOffset := lead + len(kw)
			if kw == "func " && strings.HasPrefix(strings.TrimLeft(rest, " \t"), "(") {
				if closeIdx := strings.IndexByte(rest, ')'); closeIdx >= 0 {
					restOffset += closeIdx + 1
					rest = rest[closeIdx+1:]
				}
			}
			trimmedRest := strings.TrimLeft(rest, " \t")
			restOffset += len(rest) - len(trimmedRest)
			if strings.HasPrefix(trimmedRest, symbol) {
				after := trimmedRest[len(symbol):]
				if after == "" {
					return restOffset
				}
				r, _ := utf8.DecodeRuneInString(after)
				if !isIdentRune(r) {
					return restOffset
				}
			}
		}
	}
	// Check short variable declaration or struct field: symbol := or symbol :
	if idx := indexIdentifierInLine(line, symbol); idx >= 0 {
		after := strings.TrimLeft(line[idx+len(symbol):], " \t")
		if strings.HasPrefix(after, ":=") {
			return idx
		}
	}
	return -1
}

func indexIdentifierInLine(line, symbol string) int {
	for offset := 0; offset+len(symbol) <= len(line); {
		idx := strings.Index(line[offset:], symbol)
		if idx < 0 {
			return -1
		}
		pos := offset + idx
		beforeOK := pos == 0
		if !beforeOK {
			r, _ := utf8.DecodeLastRuneInString(line[:pos])
			beforeOK = !isIdentRune(r)
		}
		afterPos := pos + len(symbol)
		afterOK := afterPos == len(line)
		if !afterOK {
			r, _ := utf8.DecodeRuneInString(line[afterPos:])
			afterOK = !isIdentRune(r)
		}
		if beforeOK && afterOK {
			return pos
		}
		offset = pos + 1
	}
	return -1
}

// GoToDefinition jumps to the definition of the identifier under the caret (or
// invokes OnGoToDefinition if set) and returns true when a symbol was resolved.
func (e *CodeEditor) GoToDefinition() bool {
	symbol, start, _ := e.SymbolAtCaret()
	if symbol == "" {
		return false
	}
	if e.OnGoToDefinition != nil {
		e.OnGoToDefinition(symbol, start)
		return true
	}
	defOffset, ok := e.FindDefinitionOffset(symbol)
	if !ok {
		return false
	}
	e.SetSelection(defOffset, defOffset+len(symbol))
	return true
}

// Copy copies the current selection to the clipboard.
func (e *CodeEditor) Copy() bool {
	e.syncDocument()
	if e.Caret == e.Anchor || e.manager == nil || e.manager.clipboardWrite == nil {
		return false
	}
	return e.manager.clipboardWrite(e.selectedText()) == nil
}

// Cut copies the current selection to the clipboard and removes it from the document.
func (e *CodeEditor) Cut() bool {
	e.syncDocument()
	if e.ReadOnly || e.Caret == e.Anchor || e.manager == nil || e.manager.clipboardWrite == nil {
		return false
	}
	sel := e.selectedText()
	if sel == "" || e.manager.clipboardWrite(sel) != nil {
		return false
	}
	return e.replaceSelection("")
}

// Paste inserts the clipboard contents at the current selection.
func (e *CodeEditor) Paste() bool {
	e.syncDocument()
	if e.ReadOnly || e.manager == nil || e.manager.clipboardRead == nil {
		return false
	}
	val, err := e.manager.clipboardRead()
	if err != nil || val == "" {
		return false
	}
	return e.replaceSelection(val)
}

// SelectAll selects the entire document.
func (e *CodeEditor) SelectAll() {
	e.syncDocument()
	e.SetSelection(0, e.Document.Len())
}

// BuildContextMenu constructs the context menu items for the current editor state.
func (e *CodeEditor) BuildContextMenu() []MenuItem {
	e.syncDocument()
	if custom := e.State.buildContextMenuItems(0, 0); len(custom) > 0 {
		return custom
	}
	hasSel := e.Caret != e.Anchor
	canModify := !e.ReadOnly
	symbol, _, _ := e.SymbolAtCaret()
	canGoToDef := symbol != ""
	if canGoToDef && e.OnGoToDefinition == nil {
		_, canGoToDef = e.FindDefinitionOffset(symbol)
	}
	goToLabel := "Go to Definition"
	if symbol != "" {
		goToLabel = "Go to Definition (" + symbol + ")"
	}
	line, _, _ := e.lineAtCaret()
	hasBp := e.Breakpoint(line)

	items := []MenuItem{
		{
			ID:       "goto_definition",
			Label:    goToLabel,
			Shortcut: "F12",
			Disabled: !canGoToDef,
			Action:   func() { e.GoToDefinition() },
		},
		{Separator: true},
		{
			ID:       "cut",
			Label:    "Cut",
			Shortcut: "Ctrl+X",
			Disabled: !hasSel || !canModify,
			Action:   func() { e.Cut() },
		},
		{
			ID:       "copy",
			Label:    "Copy",
			Shortcut: "Ctrl+C",
			Disabled: !hasSel,
			Action:   func() { e.Copy() },
		},
		{
			ID:       "paste",
			Label:    "Paste",
			Shortcut: "Ctrl+V",
			Disabled: !canModify,
			Action:   func() { e.Paste() },
		},
		{
			ID:       "select_all",
			Label:    "Select All",
			Shortcut: "Ctrl+A",
			Disabled: e.Document.Len() == 0,
			Action:   func() { e.SelectAll() },
		},
		{Separator: true},
		{
			ID:       "undo",
			Label:    "Undo",
			Shortcut: "Ctrl+Z",
			Disabled: !canModify || !e.Document.CanUndo(),
			Action:   func() { e.history(false) },
		},
		{
			ID:       "redo",
			Label:    "Redo",
			Shortcut: "Ctrl+Y",
			Disabled: !canModify || !e.Document.CanRedo(),
			Action:   func() { e.history(true) },
		},
		{Separator: true},
		{
			ID:       "toggle_breakpoint",
			Label:    "Toggle Breakpoint",
			Shortcut: "F9",
			Checked:  hasBp,
			Action:   func() { e.SetBreakpoint(line, !e.Breakpoint(line)) },
		},
	}
	if e.ContextMenuItems != nil {
		items = e.ContextMenuItems(items)
	}
	return items
}

// OpenContextMenu opens the editor's context menu at (x, y).
func (e *CodeEditor) OpenContextMenu(x, y float32) PopupID {
	if e.manager == nil || !e.Enabled() {
		return 0
	}
	e.HideCompletions()
	items := e.BuildContextMenu()
	if len(items) == 0 {
		return 0
	}
	return e.manager.OpenContextMenu(items, x, y)
}
