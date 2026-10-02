package nativeforms

import "testing"

func TestTextFieldTypingDoesNotNavigateAndBlurCommits(t *testing.T) {
	root := NewPanel()
	field := NewTextField("123", nil)
	other := NewButton("other", nil)
	root.Add(field, other)
	manager := &Manager{visible: true, interactive: true, theme: DefaultTheme()}
	manager.SetRoot(root)
	manager.setFocus(field)
	committed := ""
	field.OnSubmit = func(value string) { committed = value }
	manager.HandleEvent(Event{Type: KeyDown, Key: KeyW})
	if manager.focused != field {
		t.Fatal("typing W moved focus")
	}
	manager.HandleEvent(Event{Type: KeyDown, Key: KeyA, Mods: ModControl})
	manager.HandleEvent(Event{Type: TextInput, Rune: '7'})
	manager.setFocus(other)
	if field.Text != "7" || committed != "7" {
		t.Fatal("select-all replacement or blur commit failed")
	}
}

func TestSecondaryReleaseDoesNotDropPrimaryCapture(t *testing.T) {
	root := NewPanel()
	root.Arrange(Rect{W: 100, H: 100})
	manager := &Manager{visible: true, interactive: true, theme: DefaultTheme()}
	manager.SetRoot(root)
	manager.HandleEvent(Event{Type: PointerDown, Button: MouseLeft, X: 10, Y: 10})
	manager.HandleEvent(Event{Type: PointerDown, Button: MouseRight, X: 10, Y: 10})
	manager.HandleEvent(Event{Type: PointerUp, Button: MouseRight, X: 10, Y: 10})
	if manager.captured != root {
		t.Fatal("right release cancelled left drag")
	}
	manager.HandleEvent(Event{Type: PointerUp, Button: MouseLeft, X: 10, Y: 10})
	if manager.captured != nil {
		t.Fatal("left release did not end capture")
	}
}
