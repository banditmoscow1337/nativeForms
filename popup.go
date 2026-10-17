package nativeforms

type PopupID uint64

// PopupOptions configures an overlay that stays owned by the focused component.
// Anchor is evaluated during layout, so a popup follows a moving caret. Bounds
// supplies the size; when either dimension is zero the content is measured.
// Passive popups let pointer events reach the content underneath them.
type PopupOptions struct {
	Owner     Component
	Anchor    func() Rect
	Bounds    Rect
	Passive   bool
	OnKeyDown func(Event) bool
}

type popupEntry struct {
	id        PopupID
	content   Component
	bounds    Rect
	modal     bool
	previous  Component
	passive   bool
	keepFocus bool
	owner     Component
	anchor    func() Rect
	onKeyDown func(Event) bool
}

func (manager *Manager) OpenPopup(content Component, bounds Rect, modal bool) PopupID {
	if content == nil {
		return 0
	}
	if modal {
		manager.CancelPointerCapture()
		manager.setHovered(nil)
	}
	if manager.tooltipID != 0 {
		manager.ClosePopup(manager.tooltipID)
		manager.tooltipID = 0
	}
	manager.nextPopup++
	id := manager.nextPopup
	bindTree(content, nil, manager)
	applyTheme(content, manager.theme)
	manager.popups = append(manager.popups, popupEntry{id: id, content: content, bounds: bounds,
		modal: modal, previous: manager.focused})
	if menu, ok := content.(*Menu); ok {
		menu.popup = id
	}
	var focusables []Component
	collectFocusable(content, &focusables)
	if len(focusables) > 0 {
		manager.setFocus(focusables[0])
	} else {
		manager.setFocus(nil)
	}
	manager.InvalidateLayout()
	return id
}

// OpenUnfocusedPopup keeps keyboard and text input focus on options.Owner.
// The overlay may still receive pointer events and selected keyboard commands.
func (manager *Manager) OpenUnfocusedPopup(content Component, options PopupOptions) PopupID {
	if content == nil || options.Owner == nil || options.Owner.UIState().manager != manager ||
		!manager.visible || !manager.interactive || !containsComponent(options.Owner, manager.focused) {
		return 0
	}
	if manager.tooltipID != 0 {
		manager.ClosePopup(manager.tooltipID)
		manager.tooltipID = 0
	}
	manager.nextPopup++
	id := manager.nextPopup
	bindTree(content, nil, manager)
	applyTheme(content, manager.theme)
	manager.popups = append(manager.popups, popupEntry{
		id: id, content: content, bounds: options.Bounds, passive: options.Passive,
		keepFocus: true, owner: options.Owner, anchor: options.Anchor, onKeyDown: options.OnKeyDown,
	})
	if menu, ok := content.(*Menu); ok {
		menu.popup = id
	}
	manager.InvalidateLayout()
	return id
}

func (manager *Manager) ClosePopup(id PopupID) bool {
	for i := len(manager.popups) - 1; i >= 0; i-- {
		if manager.popups[i].id != id {
			continue
		}
		entry := manager.popups[i]
		manager.popups = append(manager.popups[:i], manager.popups[i+1:]...)
		if manager.tooltipID == id {
			manager.tooltipID = 0
		}
		manager.clearSubtreeInteraction(entry.content.UIState())
		bindTree(entry.content, nil, nil)
		previous := entry.previous
		if !entry.keepFocus && previous != nil && previous.UIState().manager == manager && previous.UIState().Visible() && previous.UIState().Enabled() {
			manager.setFocus(previous)
		} else if !entry.keepFocus && manager.topModal() != nil {
			var focusables []Component
			collectFocusable(manager.topModal().content, &focusables)
			if len(focusables) > 0 {
				manager.setFocus(focusables[0])
			}
		}
		manager.InvalidatePaint()
		return true
	}
	return false
}

func (manager *Manager) CloseTopPopup() bool {
	if len(manager.popups) == 0 {
		return false
	}
	return manager.ClosePopup(manager.popups[len(manager.popups)-1].id)
}

func (manager *Manager) ShowTooltip(text string, bounds Rect) PopupID {
	label := NewLabel(text)
	label.SetMargin(Symmetric(8, 4))
	panel := NewPanel(Overlay)
	panel.Background = manager.theme.Tooltip
	panel.Border, panel.BorderWidth = manager.theme.Border, 1
	panel.Padding = Symmetric(8, 4)
	panel.Add(label)
	manager.nextPopup++
	id := manager.nextPopup
	bindTree(panel, nil, manager)
	applyTheme(panel, manager.theme)
	manager.popups = append(manager.popups, popupEntry{id: id, content: panel, bounds: bounds, passive: true})
	manager.InvalidatePaint()
	return id
}

func (manager *Manager) OpenDialog(content Component, width, height float32) PopupID {
	w, h := float32(manager.layoutWidth), float32(manager.layoutHeight)
	if width <= 0 {
		width = minFloat32(w-32, 480)
	}
	if height <= 0 {
		height = minFloat32(h-32, 320)
	}
	return manager.OpenPopup(content, Rect{X: (w - width) / 2, Y: (h - height) / 2, W: width, H: height}, true)
}

func (manager *Manager) topModal() *popupEntry {
	for i := len(manager.popups) - 1; i >= 0; i-- {
		if manager.popups[i].modal {
			return &manager.popups[i]
		}
	}
	return nil
}

func (manager *Manager) topInteractivePopup() *popupEntry {
	for i := len(manager.popups) - 1; i >= 0; i-- {
		if !manager.popups[i].passive {
			return &manager.popups[i]
		}
	}
	return nil
}

func containsComponent(root, target Component) bool {
	if root == nil || target == nil {
		return false
	}
	for component := target; component != nil; component = component.UIState().parent {
		if sameComponent(root, component) {
			return true
		}
	}
	return false
}

func (manager *Manager) unfocusedPopupFor(target Component) *popupEntry {
	for i := len(manager.popups) - 1; i >= 0; i-- {
		entry := &manager.popups[i]
		if entry.keepFocus && containsComponent(entry.content, target) {
			return entry
		}
	}
	return nil
}

func (manager *Manager) popupKey(event Event) bool {
	for i := len(manager.popups) - 1; i >= 0; i-- {
		entry := manager.popups[i]
		if entry.keepFocus && entry.onKeyDown != nil && containsComponent(entry.owner, manager.focused) {
			return entry.onKeyDown(event)
		}
	}
	return false
}

func (manager *Manager) closeUnfocusedPopupsOutside(focused Component) {
	for i := len(manager.popups) - 1; i >= 0; i-- {
		entry := manager.popups[i]
		if entry.keepFocus && !containsComponent(entry.owner, focused) {
			manager.ClosePopup(entry.id)
		}
	}
}

func (manager *Manager) popupHit(x, y float32) (Component, bool) {
	for i := len(manager.popups) - 1; i >= 0; i-- {
		entry := &manager.popups[i]
		if entry.passive {
			continue
		}
		if target := hitTest(entry.content, x, y); target != nil {
			return target, true
		}
		if entry.modal {
			return nil, true
		}
	}
	return hitTest(manager.root, x, y), false
}

func (manager *Manager) layoutPopups(viewport Rect) {
	for i := range manager.popups {
		entry := &manager.popups[i]
		bounds := entry.bounds
		var anchor Rect
		if entry.anchor != nil {
			anchor = entry.anchor()
			bounds.X, bounds.Y = anchor.X, anchor.Y+anchor.H
		}
		maxWidth := maxFloat32(0, viewport.W)
		maxHeight := maxFloat32(0, viewport.H)
		if bounds.W <= 0 || bounds.H <= 0 {
			size := entry.content.Measure(Constraints{Max: Vec2{X: maxWidth, Y: maxHeight}, BoundedX: true, BoundedY: true})
			if bounds.W <= 0 {
				bounds.W = size.X
			}
			if bounds.H <= 0 {
				bounds.H = size.Y
			}
		}
		bounds.W = minFloat32(maxWidth, maxFloat32(0, bounds.W))
		bounds.H = minFloat32(maxHeight, maxFloat32(0, bounds.H))
		if entry.anchor != nil && bounds.Y+bounds.H > viewport.Y+viewport.H &&
			anchor.Y-bounds.H >= viewport.Y {
			bounds.Y = anchor.Y - bounds.H
		}
		bounds.X = clampFloat32(bounds.X, viewport.X, viewport.X+viewport.W-bounds.W)
		bounds.Y = clampFloat32(bounds.Y, viewport.Y, viewport.Y+viewport.H-bounds.H)
		entry.content.Arrange(bounds)
	}
}
