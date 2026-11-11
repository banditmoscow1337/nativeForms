package nativeforms

import (
	"fmt"
	"strings"
	"testing"
)

func TestCodeEditorCompletionPopupKeepsFocus(t *testing.T) {
	editor := NewCodeEditor(codeTestDocument(t, "abc\ndef"))
	manager := New()
	manager.SetInteractive(true)
	manager.SetRoot(editor)
	manager.BeginFrame(420, 600, 0)
	manager.setFocus(editor)
	if !editor.SetSelection(2, 2) {
		t.Fatal("could not position the caret")
	}
	chosen := -1
	items := make([]MenuItem, 12)
	for i := range items {
		index := i
		items[i] = MenuItem{ID: fmt.Sprint(i), Label: fmt.Sprintf("candidate %d", i), Action: func() { chosen = index }}
	}
	id := editor.ShowCompletions(items)
	if id == 0 || manager.focused != editor || !manager.TextInputFocused() {
		t.Fatal("completion popup stole text input focus")
	}
	manager.BeginFrame(420, 600, 0)
	menu, ok := manager.popups[0].content.(*Menu)
	if !ok || menu.rect.X != editor.CaretRect().X {
		t.Fatal("completion menu did not open at the caret")
	}
	for i := 0; i < 9; i++ {
		if !manager.HandleEvent(Event{Type: KeyDown, Key: KeyDownArrow}) {
			t.Fatal("completion navigation was not handled")
		}
	}
	if menu.active != 9 || menu.firstVisible != 2 || editor.Caret != 2 {
		t.Fatalf("navigation moved the editor or hid the active item: active=%d first=%d caret=%d", menu.active, menu.firstVisible, editor.Caret)
	}
	manager.HandleEvent(Event{Type: KeyDown, Key: KeyEnter})
	if chosen != 9 || len(manager.popups) != 0 || manager.focused != editor {
		t.Fatalf("Enter did not accept the completion: chosen=%d popups=%d", chosen, len(manager.popups))
	}
	editor.ShowCompletions(items)
	manager.HandleEvent(Event{Type: KeyDown, Key: KeyEscape})
	if len(manager.popups) != 0 || manager.focused != editor || chosen != 9 {
		t.Fatal("Escape did not dismiss the popup without changing focus")
	}
	editor.ShowCompletions(items)
	manager.HandleEvent(Event{Type: CompositionStart})
	if len(manager.popups) != 0 || !editor.TextInputComposing() || manager.focused != editor {
		t.Fatal("IME composition did not dismiss completions")
	}
}

func TestEditorPopupAnchorPointerAndFocusLifetime(t *testing.T) {
	editor := NewCodeEditor(codeTestDocument(t, "hello"))
	manager := New()
	manager.SetInteractive(true)
	manager.SetRoot(editor)
	manager.BeginFrame(400, 300, 0)
	manager.setFocus(editor)
	anchor := Rect{X: 390, Y: 275, W: 1, H: 16}
	label := NewLabel("signature")
	id := manager.OpenUnfocusedPopup(label, PopupOptions{
		Owner: editor, Anchor: func() Rect { return anchor }, Passive: true,
	})
	if id == 0 || manager.focused != editor {
		t.Fatal("passive popup could not keep editor focus")
	}
	manager.BeginFrame(400, 300, 0)
	r := label.UIState().rect
	if r.Y >= anchor.Y || r.X+r.W > 400 {
		t.Fatalf("popup did not fit above caret and inside viewport: %+v", r)
	}
	anchor = Rect{X: 30, Y: 20, W: 1, H: 16}
	manager.BeginFrame(400, 300, 0)
	r = label.UIState().rect
	if r.X != 30 || r.Y != 36 {
		t.Fatalf("popup did not follow its anchor: %+v", r)
	}
	manager.HandleEvent(Event{Type: PointerDown, X: r.X + 2, Y: r.Y + 2, Button: MouseLeft})
	manager.HandleEvent(Event{Type: PointerUp, X: r.X + 2, Y: r.Y + 2, Button: MouseLeft})
	if manager.focused != editor || len(manager.popups) != 1 {
		t.Fatal("passive popup blocked editor pointer input")
	}
	manager.setFocus(nil)
	if len(manager.popups) != 0 || manager.ClosePopup(id) {
		t.Fatal("popup survived its owner's loss of focus")
	}
}

func TestCodeEditorCompletionPointerSelection(t *testing.T) {
	editor := NewCodeEditor(codeTestDocument(t, "text"))
	manager := New()
	manager.SetInteractive(true)
	manager.SetRoot(editor)
	manager.BeginFrame(400, 300, 0)
	manager.setFocus(editor)
	selected := ""
	editor.ShowCompletions([]MenuItem{
		{ID: "first", Label: "First", Action: func() { selected = "first" }},
		{ID: "second", Label: "Second", Action: func() { selected = "second" }},
	})
	manager.BeginFrame(400, 300, 0)
	menu := manager.popups[0].content.(*Menu)
	x := menu.rect.X + 10
	y := menu.rect.Y + 4 + manager.Theme().ControlHeight + 5
	manager.HandleEvent(Event{Type: PointerMove, X: x, Y: y})
	manager.HandleEvent(Event{Type: PointerDown, X: x, Y: y, Button: MouseLeft})
	if manager.focused != editor {
		t.Fatal("clicking a completion stole editor focus")
	}
	manager.HandleEvent(Event{Type: PointerUp, X: x, Y: y, Button: MouseLeft})
	if selected != "second" || len(manager.popups) != 0 || manager.focused != editor {
		t.Fatalf("pointer did not accept completion: selected=%q", selected)
	}
}

func TestScopedShortcutPriority(t *testing.T) {
	manager := New()
	manager.SetInteractive(true)
	panel := NewPanel(Overlay)
	editor := NewCodeEditor(codeTestDocument(t, "a"))
	panel.Add(editor)
	manager.SetRoot(panel)
	manager.BeginFrame(400, 300, 0)
	manager.setFocus(editor)
	calls := []string{}
	app := manager.AddShortcut(Shortcut{Key: KeyF5, Action: func() { calls = append(calls, "app") }})
	parent := manager.AddShortcut(Shortcut{Key: KeyF5, Scope: panel, Action: func() { calls = append(calls, "panel") }})
	child := manager.AddShortcut(Shortcut{Key: KeyF5, Scope: editor, Action: func() { calls = append(calls, "editor") }})
	manager.HandleEvent(Event{Type: KeyDown, Key: KeyF5})
	manager.RemoveShortcut(child)
	manager.HandleEvent(Event{Type: KeyDown, Key: KeyF5})
	manager.RemoveShortcut(parent)
	manager.HandleEvent(Event{Type: KeyDown, Key: KeyF5})
	if got := fmt.Sprint(calls); got != "[editor panel app]" {
		t.Fatalf("unexpected shortcut order: %s", got)
	}
	manager.RemoveShortcut(app)
	manager.AddShortcut(Shortcut{Key: KeySpace, Mods: ModControl, Scope: editor,
		Action: func() { calls = append(calls, "completion") }})
	manager.HandleEvent(Event{Type: KeyDown, Key: KeySpace, Mods: ModControl})
	if calls[len(calls)-1] != "completion" {
		t.Fatal("editor command did not receive its scoped shortcut")
	}
}

func TestCodeEditorContextMenuAndGoToDefinition(t *testing.T) {
	src := "package main\n\nfunc helper(x int) int {\n\treturn x + 1\n}\n\nfunc main() {\n\t_ = helper(42)\n}\n"
	ed := NewCodeEditor(codeTestDocument(t, src))
	mgr := New()
	mgr.SetInteractive(true)
	var clip string
	mgr.SetClipboardHandlers(
		func() (string, error) { return clip, nil },
		func(s string) error { clip = s; return nil },
	)
	mgr.SetRoot(ed)
	mgr.BeginFrame(600, 320, 0)

	callPos := strings.Index(src, "helper(42)")
	if !ed.SetSelection(callPos, callPos+len("helper")) {
		t.Fatal("failed to select identifier")
	}
	caret := ed.CaretRect()
	if !mgr.HandleEvent(Event{Type: PointerDown, Button: MouseRight, X: caret.X, Y: caret.Y + 4}) {
		t.Fatal("right-click did not open CodeEditor context menu")
	}
	mgr.BeginFrame(600, 320, 0)
	if len(mgr.popups) != 1 {
		t.Fatalf("expected 1 context menu popup, got %d", len(mgr.popups))
	}
	menu, ok := mgr.popups[0].content.(*Menu)
	if !ok {
		t.Fatal("popup content is not *Menu")
	}
	// Activate "Copy" from context menu
	foundCopy := false
	for i, item := range menu.Items {
		if item.ID == "copy" && !item.Disabled {
			menu.active = i
			foundCopy = menu.activate()
			break
		}
	}
	if !foundCopy || clip != "helper" {
		t.Fatalf("Copy action failed: found=%v clip=%q", foundCopy, clip)
	}

	// Re-open context menu and trigger Go to Definition
	ed.SetSelection(callPos+2, callPos+2)
	if ed.OpenContextMenu(caret.X, caret.Y+4) == 0 {
		t.Fatal("OpenContextMenu failed")
	}
	menu = mgr.popups[0].content.(*Menu)
	if menu.Items[0].ID != "goto_definition" || menu.Items[0].Disabled {
		t.Fatalf("expected enabled goto_definition item, got %+v", menu.Items[0])
	}
	menu.active = 0
	menu.activate()
	defPos := strings.Index(src, "helper(x int)")
	if ed.Anchor != defPos || ed.Caret != defPos+len("helper") {
		t.Fatalf("GoToDefinition did not select declaration: anchor=%d caret=%d want=%d..%d",
			ed.Anchor, ed.Caret, defPos, defPos+len("helper"))
	}
}

func TestTextFieldAndComponentContextMenu(t *testing.T) {
	mgr := New()
	mgr.SetInteractive(true)
	var clip string
	mgr.SetClipboardHandlers(
		func() (string, error) { return clip, nil },
		func(s string) error { clip = s; return nil },
	)
	field := NewTextField("hello world", nil)
	btn := NewButton("Action", nil)
	customTriggered := false
	btn.SetContextMenu([]MenuItem{
		{ID: "custom", Label: "Custom Action", Action: func() { customTriggered = true }},
	})
	stack := NewStack(Vertical, 8)
	stack.Add(field, btn)
	mgr.SetRoot(stack)
	mgr.BeginFrame(400, 200, 0)

	field.SelectTextRange(0, 5)
	fb := field.Bounds()
	if !mgr.HandleEvent(Event{Type: PointerDown, Button: MouseRight, X: fb.X + 8, Y: fb.Y + 8}) || len(mgr.popups) != 1 {
		t.Fatal("right-click on TextField did not open context menu")
	}
	menu := mgr.popups[0].content.(*Menu)
	for i, item := range menu.Items {
		if item.ID == "copy" {
			menu.active = i
			menu.activate()
			break
		}
	}
	if clip != "hello" {
		t.Fatalf("TextField context menu Copy failed: clip=%q", clip)
	}

	bb := btn.Bounds()
	if !mgr.HandleEvent(Event{Type: PointerDown, Button: MouseRight, X: bb.X + 10, Y: bb.Y + 10}) || len(mgr.popups) != 1 {
		t.Fatal("right-click on Button with SetContextMenu did not open menu")
	}
	menu = mgr.popups[0].content.(*Menu)
	menu.active = 0
	menu.activate()
	if !customTriggered {
		t.Fatal("custom context menu item did not fire")
	}
}
