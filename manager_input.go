package nativeforms

import "time"

func (manager *Manager) HandleEvent(event Event) bool {
	manager.ProcessPending()
	manager.mu.RLock()
	root := manager.root
	visible := manager.visible
	interactive := manager.interactive
	unhandled := manager.unhandled
	consumeUnhandled := manager.consumeUnhandled
	gameNavigation := manager.gameNavigation
	manager.mu.RUnlock()

	if event.IsPointer() {
		manager.pointer = Vec2{X: event.X, Y: event.Y}
	}
	handled := false
	if visible && interactive && root != nil {
		handled = manager.handleTreeEvent(root, event, gameNavigation)
	}
	if !handled && unhandled != nil {
		handled = unhandled(event)
	}
	if visible && interactive && root != nil && (event.Type == PointerDown || event.Type == PointerUp) {
		manager.InvalidatePaint()
	}
	return handled || (visible && interactive && root != nil && consumeUnhandled)
}

func (manager *Manager) SetConsumeUnhandledInput(consume bool) {
	manager.mu.Lock()
	manager.consumeUnhandled = consume
	manager.mu.Unlock()
}

func (manager *Manager) SetGameNavigation(enabled bool) {
	manager.mu.Lock()
	manager.gameNavigation = enabled
	manager.mu.Unlock()
}

func (manager *Manager) handleTreeEvent(root Component, event Event, gameNavigation bool) bool {
	switch event.Type {
	case PointerMove:
		target, blocked := manager.popupHit(event.X, event.Y)
		manager.setHovered(target)
		if manager.captured != nil {
			target = manager.captured
		}
		return dispatchEvent(target, event) || blocked
	case PointerDown:
		if manager.captured != nil && event.Button != manager.capturedButton {
			return dispatchEvent(manager.captured, event)
		}
		target, blocked := manager.popupHit(event.X, event.Y)
		if top := manager.topInteractivePopup(); !blocked && top != nil && !top.modal && !containsComponent(top.content, target) {
			manager.ClosePopup(top.id)
			target, blocked = manager.popupHit(event.X, event.Y)
		}
		manager.setHovered(target)
		if target == nil {
			if !blocked {
				manager.setFocus(nil)
			}
			return blocked
		}
		if manager.unfocusedPopupFor(target) == nil {
			if target.UIState().Focusable() {
				manager.setFocus(target)
			} else {
				manager.setFocus(nil)
			}
		}
		manager.captured = target
		manager.capturedButton = event.Button
		target.UIState().pressed = event.Button == MouseLeft
		return dispatchEvent(target, event)
	case PointerUp:
		if manager.captured != nil && event.Button != manager.capturedButton {
			return dispatchEvent(manager.captured, event)
		}
		target := manager.captured
		blocked := false
		if target == nil {
			target, blocked = manager.popupHit(event.X, event.Y)
		}
		handled := dispatchEvent(target, event)
		if manager.captured != nil {
			manager.captured.UIState().pressed = false
		}
		manager.captured = nil
		return handled || blocked
	case PointerScroll:
		target, blocked := manager.popupHit(event.X, event.Y)
		if target == nil {
			return blocked
		}
		return dispatchScroll(target, event)
	case TextInput, CompositionStart, CompositionUpdate, CompositionEnd:
		return dispatchEvent(manager.focused, event)
	case KeyDown, KeyUp:
		if event.Type == KeyDown && event.Key == KeyEscape {
			if client, ok := manager.textInputClient(); ok && client.TextInputComposing() && dispatchEvent(manager.focused, event) {
				return true
			}
		}
		if event.Type == KeyDown && manager.popupKey(event) {
			return true
		}
		if event.Type == KeyDown && event.Key == KeyEscape {
			if top := manager.topInteractivePopup(); top != nil {
				return manager.ClosePopup(top.id)
			}
		}
		for target := manager.focused; target != nil; target = target.UIState().parent {
			if !target.UIState().Enabled() {
				continue
			}
			if target.Handle(event) {
				return true
			}
			if event.Type == KeyDown && manager.handleShortcut(event, target) {
				return true
			}
		}
		if event.Type == KeyDown {
			if manager.handleShortcut(event, nil) {
				return true
			}
			if modal := manager.topModal(); modal != nil {
				root = modal.content
			} else if top := manager.topInteractivePopup(); top != nil {
				root = top.content
			}
			switch event.Key {
			case KeyTab:
				if event.Mods&ModShift != 0 {
					return manager.focusNext(root, -1)
				}
				return manager.focusNext(root, 1)
			case KeyDownArrow, KeyS:
				if gameNavigation && event.Mods == 0 {
					return manager.focusNext(root, 1)
				}
			case KeyUpArrow, KeyW:
				if gameNavigation && event.Mods == 0 {
					return manager.focusNext(root, -1)
				}
			}
		}
		if manager.focused == nil {
			return dispatchEvent(root, event)
		}
	}
	return false
}

func hitTest(component Component, x, y float32) Component {
	if component == nil {
		return nil
	}
	state := component.UIState()
	if !state.Visible() || !state.Enabled() || !state.rect.Contains(x, y) {
		return nil
	}
	for index := len(state.children) - 1; index >= 0; index-- {
		if target := hitTest(state.children[index], x, y); target != nil {
			return target
		}
	}
	return component
}

func dispatchEvent(target Component, event Event) bool {
	for target != nil {
		if target.UIState().Enabled() && target.Handle(event) {
			return true
		}
		target = target.UIState().parent
	}
	return false
}

func dispatchScroll(target Component, event Event) bool {
	remaining := Vec2{X: -event.Scroll.X * 40, Y: -event.Scroll.Y * 40}
	used := false
	for target != nil {
		if target.UIState().Enabled() {
			if panel, ok := target.(*ScrollPanel); ok {
				before := remaining
				remaining = panel.ScrollBy(remaining.X, remaining.Y)
				if remaining != before {
					used = true
				}
				if remaining == (Vec2{}) {
					return true
				}
			} else {
				event.Scroll = Vec2{X: -remaining.X / 40, Y: -remaining.Y / 40}
				if target.Handle(event) {
					return true
				}
			}
		}
		target = target.UIState().parent
	}
	return used
}

func (manager *Manager) setHovered(component Component) {
	if sameComponent(manager.hovered, component) {
		return
	}
	if manager.hovered != nil {
		manager.hovered.UIState().hovered = false
	}
	if manager.tooltipID != 0 {
		manager.ClosePopup(manager.tooltipID)
		manager.tooltipID = 0
	}
	manager.hovered = component
	if component != nil {
		component.UIState().hovered = true
		if value := component.UIState().tooltip; value != "" && manager.topModal() == nil {
			manager.tooltipID = manager.ShowTooltip(value, Rect{X: manager.pointer.X + 14, Y: manager.pointer.Y + 18})
		}
	}
	manager.InvalidatePaint()
}

// Focus assigns keyboard and text input focus to component (or clears focus if nil).
func (manager *Manager) Focus(component Component) {
	if manager == nil {
		return
	}
	manager.setFocus(component)
}

func (manager *Manager) setFocus(component Component) {
	if modal := manager.topModal(); modal != nil && component != nil && !containsComponent(modal.content, component) {
		return
	}
	if sameComponent(manager.focused, component) {
		if client, ok := manager.textInputClient(); ok {
			client.ResetCaretBlink()
			manager.caretDue = manager.now().Add(500 * time.Millisecond)
		}
		manager.InvalidatePaint()
		return
	}
	if manager.focused != nil {
		manager.focused.UIState().focused = false
		manager.focused.Handle(Event{Type: FocusLost})
	}
	manager.focused = component
	if component != nil {
		component.UIState().focused = true
		for ancestor := component.UIState().parent; ancestor != nil; ancestor = ancestor.UIState().parent {
			if scroll, ok := ancestor.(*ScrollPanel); ok {
				scroll.ScrollIntoView(component)
			}
		}
	}
	manager.closeUnfocusedPopupsOutside(component)
	manager.caretDue = time.Time{}
	if client, ok := manager.textInputClient(); ok {
		client.ResetCaretBlink()
		manager.caretDue = manager.now().Add(500 * time.Millisecond)
	}
	manager.InvalidatePaint()
}

func (manager *Manager) focusNext(root Component, direction int) bool {
	var stackComponents [32]Component
	components := stackComponents[:0]
	collectFocusable(root, &components)
	if len(components) == 0 {
		return false
	}
	index := -1
	for candidateIndex, candidate := range components {
		if sameComponent(candidate, manager.focused) {
			index = candidateIndex
			break
		}
	}
	if direction < 0 {
		if index < 0 {
			index = 0
		}
		index = (index - 1 + len(components)) % len(components)
	} else {
		index = (index + 1) % len(components)
	}
	manager.setFocus(components[index])
	return true
}

func sameComponent(left, right Component) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return left.UIState() == right.UIState()
}

func collectFocusable(component Component, result *[]Component) {
	if component == nil {
		return
	}
	state := component.UIState()
	if !state.Visible() || !state.Enabled() {
		return
	}
	if state.Focusable() {
		*result = append(*result, component)
	}
	for _, child := range state.children {
		collectFocusable(child, result)
	}
}

func (manager *Manager) clearInteractionLocked() Component {
	blurred := manager.focused
	if manager.focused != nil {
		manager.focused.UIState().focused = false
	}
	if manager.hovered != nil {
		manager.hovered.UIState().hovered = false
	}
	if manager.captured != nil {
		manager.captured.UIState().pressed = false
	}
	manager.focused = nil
	manager.hovered = nil
	manager.captured = nil
	manager.caretDue = time.Time{}
	return blurred
}

func (manager *Manager) ClearInteraction() {
	manager.mu.Lock()
	blurred := manager.clearInteractionLocked()
	manager.mu.Unlock()
	manager.finishClearInteraction(blurred)
}

func stateInSubtree(root *State, component Component) bool {
	if root == nil || component == nil {
		return false
	}
	for state := component.UIState(); state != nil; {
		if state == root {
			return true
		}
		if state.parent == nil {
			break
		}
		state = state.parent.UIState()
	}
	return false
}

func (manager *Manager) clearSubtreeInteraction(root *State) {
	if stateInSubtree(root, manager.focused) {
		manager.setFocus(nil)
	}
	if stateInSubtree(root, manager.hovered) {
		manager.setHovered(nil)
	}
	if stateInSubtree(root, manager.captured) {
		manager.captured.UIState().pressed = false
		manager.captured = nil
	}
	manager.InvalidatePaint()
}

func (manager *Manager) TextInputFocused() bool {
	_, ok := manager.textInputClient()
	return ok
}

func (manager *Manager) FocusedCaretRect() (Rect, bool) {
	client, ok := manager.textInputClient()
	if !ok {
		return Rect{}, false
	}
	return client.CaretRect(), true
}

func (manager *Manager) HoveredTextInput() bool {
	_, ok := manager.hovered.(TextInputClient)
	return ok
}

func (manager *Manager) CancelPointerCapture() {
	if manager.captured != nil {
		manager.captured.UIState().pressed = false
		manager.captured = nil
		manager.InvalidatePaint()
	}
}
