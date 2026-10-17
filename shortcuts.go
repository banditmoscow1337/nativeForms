package nativeforms

type ShortcutID uint64

type Shortcut struct {
	Key  Key
	Mods int
	// Scope limits the shortcut to a focused component and its descendants.
	// A nil Scope registers an application shortcut.
	Scope        Component
	AllowInModal bool
	Repeat       bool
	Action       func()
}

func (manager *Manager) AddShortcut(shortcut Shortcut) ShortcutID {
	if shortcut.Action == nil || shortcut.Key == KeyUnknown {
		return 0
	}
	manager.nextShortcut++
	if manager.shortcuts == nil {
		manager.shortcuts = make(map[ShortcutID]Shortcut)
	}
	manager.shortcuts[manager.nextShortcut] = shortcut
	return manager.nextShortcut
}

func (manager *Manager) RemoveShortcut(id ShortcutID) { delete(manager.shortcuts, id) }

func (manager *Manager) handleShortcut(event Event, scope Component) bool {
	for id := ShortcutID(1); id <= manager.nextShortcut; id++ {
		shortcut, ok := manager.shortcuts[id]
		if !ok || shortcut.Key != event.Key || shortcut.Mods != event.Mods || (event.Repeat && !shortcut.Repeat) ||
			!sameComponent(shortcut.Scope, scope) {
			continue
		}
		if manager.topModal() != nil && !shortcut.AllowInModal {
			continue
		}
		shortcut.Action()
		return true
	}
	return false
}
