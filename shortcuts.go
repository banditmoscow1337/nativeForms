package nativeforms

type ShortcutID uint64

type Shortcut struct {
	Key          Key
	Mods         int
	AllowInModal bool
	Repeat       bool
	Action       func()
}

// AddShortcut registers an application action. Focused controls receive keys
// first, so editor commands such as copy/paste keep their local meaning.
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

func (manager *Manager) handleShortcut(event Event) bool {
	// Iterating by ID gives a defined priority to the oldest binding.
	for id := ShortcutID(1); id <= manager.nextShortcut; id++ {
		shortcut, ok := manager.shortcuts[id]
		if !ok || shortcut.Key != event.Key || shortcut.Mods != event.Mods || (event.Repeat && !shortcut.Repeat) {
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
