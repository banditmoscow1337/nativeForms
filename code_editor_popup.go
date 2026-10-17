package nativeforms

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
