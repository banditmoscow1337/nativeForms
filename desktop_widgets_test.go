package nativeforms

import (
	"fmt"
	"testing"
)

func TestComboBoxMenuScrollAndSelection(t *testing.T) {
	options := make([]RadioOption, 24)
	for i := range options {
		options[i] = RadioOption{ID: fmt.Sprint(i), Label: fmt.Sprintf("Option %d", i)}
	}
	manager := New()
	manager.SetInteractive(true)
	combo := NewComboBox(options, "22", nil)
	manager.SetRoot(combo)
	manager.BeginFrame(240, 200, 0)
	manager.setFocus(combo)
	manager.HandleEvent(Event{Type: KeyDown, Key: KeyDownArrow})
	manager.BeginFrame(240, 200, 0)
	menu := manager.popups[0].content.(*Menu)
	if menu.MaxVisibleItems != 8 || menu.visibleRows() >= len(options) || menu.firstVisible == 0 || menu.active != 22 {
		t.Fatalf("selected item not initially visible: rows=%d first=%d active=%d", menu.visibleRows(), menu.firstVisible, menu.active)
	}
	first := menu.firstVisible
	x, y := menu.rect.X+10, menu.rect.Y+10
	if !manager.HandleEvent(Event{Type: PointerScroll, X: x, Y: y, Scroll: Vec2{Y: 1}}) {
		t.Fatal("wheel scroll was not handled")
	}
	if menu.firstVisible != first-1 {
		t.Fatalf("wheel did not move list: %d -> %d", first, menu.firstVisible)
	}
	manager.BeginFrame(240, 200, 0)
	if menu.firstVisible != first-1 {
		t.Fatal("painting reset the user's scroll position to the active item")
	}
	want := options[menu.firstVisible].ID
	manager.HandleEvent(Event{Type: PointerDown, Button: MouseLeft, X: x, Y: y})
	manager.HandleEvent(Event{Type: PointerUp, Button: MouseLeft, X: x, Y: y})
	if combo.SelectedID != want || len(manager.popups) != 0 {
		t.Fatalf("scrolled row was not selected: got %q, want %q", combo.SelectedID, want)
	}
}

func TestNonClosableTabsKeepSelectionAndCanBeChanged(t *testing.T) {
	guardCalls := 0
	tabs := NewTabs([]Tab{
		{ID: "fixed", Title: "Fixed", NonClosable: true, Content: NewPanel()},
		{ID: "file", Title: "File", Content: NewPanel()},
	}, "file", nil)
	tabs.OnCloseRequest = func(string) bool { guardCalls++; return true }
	manager := New()
	manager.SetInteractive(true)
	manager.SetRoot(tabs)
	manager.BeginFrame(400, 200, 0)
	if tabs.Close("fixed") || guardCalls != 0 {
		t.Fatal("programmatic close removed a fixed tab or called its close guard")
	}
	click := func(x float32) {
		manager.HandleEvent(Event{Type: PointerDown, Button: MouseLeft, X: x, Y: 15})
		manager.HandleEvent(Event{Type: PointerUp, Button: MouseLeft, X: x, Y: 15})
	}
	click(90) // The fixed tab has no close hit area, even on its right edge.
	if tabs.SelectedID != "fixed" || len(tabs.Tabs) != 2 {
		t.Fatal("clicking the fixed tab's right edge did not select it")
	}
	click(190)
	if len(tabs.Tabs) != 1 || guardCalls != 1 {
		t.Fatal("closable tab's close button did not close it")
	}
	if !tabs.SetNonClosable("fixed", false) || tabs.SetNonClosable("fixed", false) {
		t.Fatal("changing closeability did not update the tab")
	}
	manager.BeginFrame(400, 200, 0)
	click(90)
	if len(tabs.Tabs) != 0 || guardCalls != 2 {
		t.Fatal("tab did not become closable after SetNonClosable")
	}
}
