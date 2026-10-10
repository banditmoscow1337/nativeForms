package nativeforms

import "testing"

func TestPointerMoveHitsAndBubblesWithoutCapture(t *testing.T) {
	root := NewPanel(Overlay)
	child := NewPanel()
	child.SetPreferredSize(Vec2{X: 50, Y: 50})
	root.Add(child)
	childMoves, parentMoves, unhandled := 0, 0, 0
	child.SetEventHandler(func(e Event) bool {
		if e.Type == PointerMove {
			childMoves++
		}
		return false
	})
	root.SetEventHandler(func(e Event) bool {
		if e.Type == PointerMove {
			parentMoves++
			return true
		}
		return false
	})
	manager := New()
	manager.SetInteractive(true)
	manager.SetRoot(root)
	manager.SetUnhandledEventHandler(func(e Event) bool {
		unhandled++
		return false
	})
	manager.BeginFrame(100, 100, 0)
	if !manager.HandleEvent(Event{Type: PointerMove, X: 10, Y: 10}) {
		t.Fatal("pointer move was not handled by the hit target's parent")
	}
	if childMoves != 1 || parentMoves != 1 || unhandled != 0 || manager.captured != nil {
		t.Fatalf("routing: child=%d parent=%d unhandled=%d capture=%v", childMoves, parentMoves, unhandled, manager.captured)
	}
}

func TestMenuHoverWithoutPressedButtonAndCapturedDrag(t *testing.T) {
	manager := New()
	manager.SetInteractive(true)
	manager.SetRoot(NewPanel())
	selected := ""
	menu := NewMenu([]MenuItem{{ID: "one", Label: "One"}, {ID: "disabled", Label: "Disabled", Disabled: true}, {ID: "three", Label: "Three"}}, func(id string) { selected = id })
	id := manager.OpenPopup(menu, Rect{X: 10, Y: 10, W: 150, H: 140}, false)
	manager.BeginFrame(320, 240, 0)
	h := manager.Theme().ControlHeight
	if !manager.HandleEvent(Event{Type: PointerMove, X: 30, Y: 10 + 4 + 2*h + 10}) || menu.active != 2 {
		t.Fatal("hover without capture did not select the third menu row")
	}
	manager.BeginFrame(320, 240, 0)
	manager.HandleEvent(Event{Type: PointerMove, X: 30, Y: 10 + 4 + 2*h + 10})
	if manager.NeedsFrame() {
		t.Fatal("unchanged menu hover requested another frame")
	}
	manager.HandleEvent(Event{Type: PointerMove, X: 30, Y: 10 + 4 + h + 10})
	if menu.active != -1 {
		t.Fatal("disabled menu row remained active on hover")
	}
	manager.HandleEvent(Event{Type: PointerDown, Button: MouseLeft, X: 30, Y: 10 + 4 + h + 10})
	manager.HandleEvent(Event{Type: PointerUp, Button: MouseLeft, X: 30, Y: 10 + 4 + h + 10})
	if selected != "" || manager.captured != nil || len(manager.popups) != 1 {
		t.Fatal("disabled row activated or kept capture")
	}
	manager.HandleEvent(Event{Type: PointerDown, Button: MouseLeft, X: 30, Y: 24})
	manager.HandleEvent(Event{Type: PointerMove, X: 300, Y: 200})
	if menu.active != -1 || manager.captured != menu {
		t.Fatal("captured menu did not receive a drag outside its bounds")
	}
	manager.HandleEvent(Event{Type: PointerUp, Button: MouseLeft, X: 300, Y: 200})
	if manager.captured != nil {
		t.Fatal("release outside menu kept pointer capture")
	}
	manager.HandleEvent(Event{Type: PointerMove, X: 30, Y: 10 + 4 + 2*h + 10})
	manager.HandleEvent(Event{Type: PointerDown, Button: MouseLeft, X: 30, Y: 10 + 4 + 2*h + 10})
	manager.HandleEvent(Event{Type: PointerUp, Button: MouseLeft, X: 30, Y: 10 + 4 + 2*h + 10})
	if selected != "three" || len(manager.popups) != 0 || manager.ClosePopup(id) {
		t.Fatalf("hovered item did not activate and close its popup: selected=%q, remaining=%d", selected, len(manager.popups))
	}
}

func TestEscapeCancelsIMEBeforeClosingModal(t *testing.T) {
	manager := New()
	manager.SetInteractive(true)
	manager.SetRoot(NewPanel())
	field := NewTextField("original", nil)
	manager.OpenPopup(field, Rect{X: 10, Y: 10, W: 240, H: 48}, true)
	manager.BeginFrame(320, 240, 0)
	if manager.focused != field {
		t.Fatal("modal did not focus its editor")
	}
	manager.HandleEvent(Event{Type: CompositionStart})
	manager.HandleEvent(Event{Type: CompositionUpdate, Text: "пример"})
	if !field.composing || field.Editor().Preedit == "" {
		t.Fatal("IME preedit did not start")
	}
	if !manager.HandleEvent(Event{Type: KeyDown, Key: KeyEscape}) {
		t.Fatal("first Escape did not cancel IME")
	}
	if len(manager.popups) != 1 || field.composing || field.Editor().Preedit != "" || field.Text != "original" {
		t.Fatal("IME Escape closed the modal or committed preedit")
	}
	if !manager.HandleEvent(Event{Type: KeyDown, Key: KeyEscape}) || len(manager.popups) != 0 {
		t.Fatal("second Escape did not close the modal")
	}
}

func TestOpeningModalCancelsBackgroundPointerCapture(t *testing.T) {
	manager := New()
	manager.SetInteractive(true)
	root := NewPanel()
	backgroundMoves := 0
	root.SetEventHandler(func(e Event) bool {
		if e.Type == PointerMove {
			backgroundMoves++
		}
		return true
	})
	manager.SetRoot(root)
	manager.BeginFrame(320, 240, 0)
	manager.HandleEvent(Event{Type: PointerDown, Button: MouseLeft, X: 5, Y: 5})
	if manager.captured != root {
		t.Fatal("background did not capture the initial drag")
	}
	manager.OpenPopup(NewButton("Dialog", nil), Rect{X: 80, Y: 80, W: 160, H: 60}, true)
	manager.BeginFrame(320, 240, 0)
	if manager.captured != nil || root.Pressed() {
		t.Fatal("opening a modal left background capture active")
	}
	if !manager.HandleEvent(Event{Type: PointerMove, X: 5, Y: 5}) || backgroundMoves != 0 {
		t.Fatal("pointer move leaked through the modal to the background")
	}
}

func TestEscapeCancelsEmptyCompositionStart(t *testing.T) {
	manager := New()
	manager.SetInteractive(true)
	manager.SetRoot(NewPanel())
	field := NewTextField("", nil)
	manager.OpenPopup(field, Rect{X: 10, Y: 10, W: 200, H: 48}, true)
	manager.HandleEvent(Event{Type: CompositionStart})
	if field.Editor().Preedit != "" || !field.composing {
		t.Fatal("empty composition start was not tracked")
	}
	manager.HandleEvent(Event{Type: KeyDown, Key: KeyEscape})
	if len(manager.popups) != 1 || field.composing {
		t.Fatal("Escape closed a modal while an empty preedit was starting")
	}
}

func TestMenuKeyboardSkipsDisabledItems(t *testing.T) {
	selected := ""
	menu := NewMenu([]MenuItem{{ID: "a", Label: "A"}, {ID: "b", Label: "B", Disabled: true}, {ID: "c", Label: "C"}}, func(id string) { selected = id })
	if menu.active != 0 {
		t.Fatal("menu did not select the first enabled item")
	}
	menu.Handle(Event{Type: KeyDown, Key: KeyDownArrow})
	if menu.active != 2 {
		t.Fatal("Down selected a disabled item")
	}
	menu.Handle(Event{Type: KeyDown, Key: KeyHome})
	if menu.active != 0 {
		t.Fatal("Home did not select the first enabled item")
	}
	menu.Handle(Event{Type: KeyDown, Key: KeyEnd})
	if menu.active != 2 {
		t.Fatal("End did not select the last enabled item")
	}
	menu.Handle(Event{Type: KeyDown, Key: KeyEnter})
	if selected != "c" {
		t.Fatal("Enter did not activate the keyboard selection")
	}
	called := false
	disabled := NewMenu([]MenuItem{{ID: "disabled", Disabled: true, Action: func() { called = true }}}, nil)
	disabled.Handle(Event{Type: KeyDown, Key: KeyDownArrow})
	if disabled.active != -1 || !disabled.Handle(Event{Type: KeyDown, Key: KeyEnter}) || called {
		t.Fatal("all-disabled menu activated an item")
	}
}

func TestComboBoxKeyboardNavigationAndFocusRestore(t *testing.T) {
	manager := New()
	manager.SetInteractive(true)
	root := NewStack(Vertical, 0)
	combo := NewComboBox([]RadioOption{{ID: "a", Label: "Alpha"}, {ID: "b", Label: "Beta"}, {ID: "c", Label: "Gamma"}}, "b", nil)
	other := NewButton("Next", nil)
	root.Add(combo, other)
	manager.SetRoot(root)
	manager.BeginFrame(320, 240, 0)
	manager.setFocus(combo)
	if !manager.HandleEvent(Event{Type: KeyDown, Key: KeyDownArrow}) || len(manager.popups) != 1 {
		t.Fatal("keyboard did not open ComboBox menu")
	}
	menu, ok := manager.popups[0].content.(*Menu)
	if !ok || menu.active != 1 || manager.focused != menu {
		t.Fatal("menu did not start at selected option with keyboard focus")
	}
	manager.HandleEvent(Event{Type: KeyDown, Key: KeyDownArrow})
	manager.HandleEvent(Event{Type: KeyDown, Key: KeyEnter})
	if combo.SelectedID != "c" || len(manager.popups) != 0 || manager.focused != combo {
		t.Fatal("keyboard selection did not close menu and restore focus")
	}
	manager.HandleEvent(Event{Type: KeyDown, Key: KeyEnter, Repeat: true})
	if len(manager.popups) != 0 {
		t.Fatal("key repeat reopened the ComboBox")
	}
	manager.HandleEvent(Event{Type: KeyDown, Key: KeyTab})
	if manager.focused != other {
		t.Fatal("Tab did not advance to the next control")
	}
}

func TestEmptyComboBoxDoesNotOpenPopup(t *testing.T) {
	manager := New()
	manager.SetInteractive(true)
	combo := NewComboBox(nil, "", nil)
	manager.SetRoot(combo)
	manager.BeginFrame(200, 60, 0)
	manager.setFocus(combo)
	manager.HandleEvent(Event{Type: KeyDown, Key: KeyDownArrow})
	if len(manager.popups) != 0 {
		t.Fatal("empty ComboBox opened an unselectable popup")
	}
}

func TestModalTabSkipsDisabledAndRestoresPreviousFocus(t *testing.T) {
	manager := New()
	manager.SetInteractive(true)
	root := NewPanel()
	before := NewButton("Before", nil)
	root.Add(before)
	manager.SetRoot(root)
	manager.setFocus(before)
	dialog := NewStack(Vertical, 0)
	first := NewButton("First", nil)
	disabled := NewButton("Disabled", nil)
	disabled.SetEnabled(false)
	last := NewButton("Last", nil)
	dialog.Add(first, disabled, last)
	manager.OpenPopup(dialog, Rect{X: 20, Y: 20, W: 280, H: 180}, true)
	if manager.focused != first {
		t.Fatal("modal did not focus first available control")
	}
	manager.HandleEvent(Event{Type: KeyDown, Key: KeyTab})
	if manager.focused != last {
		t.Fatal("Tab did not skip disabled control")
	}
	manager.HandleEvent(Event{Type: KeyDown, Key: KeyTab})
	if manager.focused != first {
		t.Fatal("Tab left the modal instead of wrapping")
	}
	manager.HandleEvent(Event{Type: KeyDown, Key: KeyTab, Mods: ModShift})
	if manager.focused != last {
		t.Fatal("Shift+Tab did not wrap within the modal")
	}
	manager.HandleEvent(Event{Type: KeyDown, Key: KeyEscape})
	if manager.focused != before || len(manager.popups) != 0 {
		t.Fatal("closing modal did not restore earlier focus")
	}
}

type iconTreeModel struct{}

func (iconTreeModel) Roots() int             { return 1 }
func (iconTreeModel) Root(int) string        { return "folder" }
func (iconTreeModel) Label(id string) string { return id }
func (iconTreeModel) Children(id string) int {
	if id == "folder" {
		return 1
	}
	return 0
}
func (iconTreeModel) Child(string, int) string { return "leaf" }

func frameLines(frame Frame) []DrawCommand {
	var lines []DrawCommand
	for _, command := range frame.Commands {
		if command.Kind == CommandLine {
			lines = append(lines, command)
		}
	}
	return lines
}

func TestComboAndTreeChevronsUseGeometry(t *testing.T) {
	manager := New()
	combo := NewComboBox([]RadioOption{{ID: "a", Label: "Alpha"}}, "a", nil)
	manager.SetRoot(combo)
	manager.BeginFrame(200, 60, 0)
	lines := frameLines(manager.Frame())
	if len(lines) != 2 || lines[0].From.Y >= lines[0].To.Y || lines[1].From.Y <= lines[1].To.Y {
		t.Fatal("ComboBox did not draw a downward chevron with line commands")
	}
	tree := NewVirtualTree(iconTreeModel{})
	manager.SetRoot(tree)
	manager.BeginFrame(200, 80, 0)
	lines = frameLines(manager.Frame())
	if len(lines) != 2 || lines[0].From.Y >= lines[0].To.Y || lines[1].From.Y >= lines[1].To.Y {
		t.Fatal("collapsed tree did not draw a rightward chevron")
	}
	tree.SetExpanded("folder", true)
	manager.BeginFrame(200, 80, 0)
	lines = frameLines(manager.Frame())
	if len(lines) != 2 || lines[0].From.Y >= lines[0].To.Y || lines[1].From.Y <= lines[1].To.Y {
		t.Fatal("expanded tree did not draw a downward chevron")
	}
}
