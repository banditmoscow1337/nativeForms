package nativeforms

import (
	"math"
	"testing"
)

func TestSpinBoxInteractionAndAccessibility(t *testing.T) {
	mgr := New()
	mgr.SetInteractive(true)
	var lastVal float32
	calls := 0
	sb := NewSpinBox("Count:", 0, 100, 10, 5, func(v float32) {
		lastVal = v
		calls++
	})
	sb.Suffix = " items"
	mgr.SetRoot(sb)
	mgr.BeginFrame(300, 60, 0.016)

	if sb.FormattedValue() != "10 items" {
		t.Fatalf("FormattedValue() = %q, want '10 items'", sb.FormattedValue())
	}

	// Click '+' button
	_, plusRect := sb.buttonRects()
	mgr.HandleEvent(Event{Type: PointerDown, Button: MouseLeft, X: plusRect.X + 4, Y: plusRect.Y + 8})
	mgr.HandleEvent(Event{Type: PointerUp, Button: MouseLeft, X: plusRect.X + 4, Y: plusRect.Y + 8})
	if sb.Value != 15 || calls != 1 || lastVal != 15 {
		t.Fatalf("after clicking '+': Value=%v calls=%d", sb.Value, calls)
	}

	// Click '-' button
	minusRect, _ := sb.buttonRects()
	mgr.HandleEvent(Event{Type: PointerDown, Button: MouseLeft, X: minusRect.X + 4, Y: minusRect.Y + 8})
	mgr.HandleEvent(Event{Type: PointerUp, Button: MouseLeft, X: minusRect.X + 4, Y: minusRect.Y + 8})
	if sb.Value != 10 || calls != 2 {
		t.Fatalf("after clicking '-': Value=%v calls=%d", sb.Value, calls)
	}

	// Wheel scroll & keyboard navigation
	mgr.HandleEvent(Event{Type: PointerScroll, X: sb.Bounds().X + 10, Y: sb.Bounds().Y + 10, Scroll: Vec2{Y: 1}})
	if sb.Value != 15 {
		t.Fatalf("after scroll up: Value=%v, want 15", sb.Value)
	}
	sb.Handle(Event{Type: KeyDown, Key: KeyPageUp})
	if sb.Value != 65 {
		t.Fatalf("after PageUp: Value=%v, want 65", sb.Value)
	}

	// Accessibility actions
	id := sb.UIState().AccessibilityID()
	if !mgr.PerformAccessibilityAction(id, AccessibilitySetValue, "80") || math.Abs(float64(sb.Value-80)) > 1e-4 {
		t.Fatalf("AccessibilitySetValue failed: Value=%v", sb.Value)
	}
	if !mgr.PerformAccessibilityAction(id, AccessibilityIncrement, "") || math.Abs(float64(sb.Value-85)) > 1e-4 {
		t.Fatalf("AccessibilityIncrement failed: Value=%v", sb.Value)
	}
}

func TestSegmentedControlAndToolbar(t *testing.T) {
	mgr := New()
	mgr.SetInteractive(true)
	var chosen string
	sc := NewSegmentedControl([]RadioOption{
		{ID: "day", Label: "Day"},
		{ID: "week", Label: "Week"},
		{ID: "month", Label: "Month"},
	}, "day", func(id string) {
		chosen = id
	})
	toggled := false
	clicked := 0
	tb := NewToolbar(
		ToolbarItem{ID: "run", Label: "Run", Action: func() { clicked++ }},
		ToolbarItem{Separator: true},
		ToolbarItem{ID: "watch", Label: "Watch", Toggle: true, OnToggle: func(v bool) { toggled = v }},
		ToolbarItem{Spacer: true},
		ToolbarItem{ID: "Disabled", Label: "Off", Disabled: true, Action: func() { clicked += 100 }},
	)
	stack := NewStack(Vertical, 6)
	stack.Add(tb, sc)
	mgr.SetRoot(stack)
	mgr.BeginFrame(360, 120, 0.016)

	// Click middle segment of SegmentedControl
	b := sc.Bounds()
	mgr.HandleEvent(Event{Type: PointerDown, Button: MouseLeft, X: b.X + b.W*0.5, Y: b.Y + b.H*0.5})
	mgr.HandleEvent(Event{Type: PointerUp, Button: MouseLeft, X: b.X + b.W*0.5, Y: b.Y + b.H*0.5})
	if sc.SelectedID != "week" || chosen != "week" {
		t.Fatalf("SegmentedControl click selected %q, want 'week'", sc.SelectedID)
	}

	// Keyboard navigation on SegmentedControl
	sc.Handle(Event{Type: KeyDown, Key: KeyRightArrow})
	if sc.SelectedID != "month" {
		t.Fatalf("SegmentedControl KeyRightArrow selected %q, want 'month'", sc.SelectedID)
	}

	// Click toolbar buttons
	r0 := tb.itemRectAt(0)
	mgr.HandleEvent(Event{Type: PointerDown, Button: MouseLeft, X: r0.X + 6, Y: r0.Y + 6})
	mgr.HandleEvent(Event{Type: PointerUp, Button: MouseLeft, X: r0.X + 6, Y: r0.Y + 6})
	r2 := tb.itemRectAt(2)
	mgr.HandleEvent(Event{Type: PointerDown, Button: MouseLeft, X: r2.X + 6, Y: r2.Y + 6})
	mgr.HandleEvent(Event{Type: PointerUp, Button: MouseLeft, X: r2.X + 6, Y: r2.Y + 6})
	if clicked != 1 || !toggled || !tb.Items[2].Active {
		t.Fatalf("Toolbar interaction failed: clicked=%d toggled=%v", clicked, toggled)
	}
}

func TestAccordionCollapsibleSeparatorHyperlinkBadgeAndIndeterminate(t *testing.T) {
	mgr := New()
	mgr.SetInteractive(true)

	sec1 := NewCollapsible("Section 1", NewLabel("Body 1"), true, nil)
	sec2 := NewCollapsible("Section 2", NewLabel("Body 2"), false, nil)
	acc := NewAccordion(true, sec1, sec2)

	openedURL := ""
	link := NewHyperlink("Docs", "https://example.com/docs", func(u string) { openedURL = u })
	badge := NewBadge("NEW")
	sep := NewLabeledSeparator("Details")
	chk := NewCheckbox("Select All", false, nil)
	chk.SetIndeterminate(true)
	bar := NewProgressBar(0.5)
	bar.SetIndeterminate(true)
	bar.StepPhase(0.25)

	root := NewStack(Vertical, 6)
	root.Add(acc, sep, link, badge, chk, bar)
	mgr.SetRoot(root)
	mgr.BeginFrame(400, 400, 0.016)

	// Expanding sec2 in SingleOpen Accordion must collapse sec1
	sec2.SetExpanded(true)
	if sec1.Expanded || !sec2.Expanded {
		t.Fatalf("SingleOpen Accordion did not collapse sec1: sec1=%v sec2=%v", sec1.Expanded, sec2.Expanded)
	}

	// Indeterminate checkbox click clears Indeterminate and checks the box
	if !mgr.PerformAccessibilityAction(chk.UIState().AccessibilityID(), AccessibilityActivate, "") {
		t.Fatal("Checkbox AccessibilityActivate failed")
	}
	if chk.Indeterminate || !chk.Checked {
		t.Fatalf("Indeterminate Checkbox after activate: Indeterminate=%v Checked=%v", chk.Indeterminate, chk.Checked)
	}

	// Hyperlink activation
	if !mgr.PerformAccessibilityAction(link.UIState().AccessibilityID(), AccessibilityActivate, "") || openedURL != "https://example.com/docs" {
		t.Fatalf("Hyperlink activate failed: openedURL=%q", openedURL)
	}
}

func TestDisplayScaleAndBackingScaleSeparation(t *testing.T) {
	mgr := New()
	mgr.SetBackingScale(2.0)
	mgr.SetDisplayScale(1.5)
	if mgr.DisplayScale() != 1.5 {
		t.Fatalf("DisplayScale overwritten by BackingScale: got %v, want 1.5", mgr.DisplayScale())
	}
	if mgr.BackingScale() != 2.0 {
		t.Fatalf("BackingScale: got %v, want 2.0", mgr.BackingScale())
	}
	if mgr.EffectiveScale() != 3.0 {
		t.Fatalf("EffectiveScale: got %v, want 3.0", mgr.EffectiveScale())
	}
}

func TestTopPanelsKeepFixedHeightOnTabSwitch(t *testing.T) {
	root := NewStack(Vertical, 6)
	menu := NewMenuBar(MenuBarItem{ID: "file", Label: "File"})
	header := NewStack(Horizontal, 8)
	header.UIState().SetPreferredSize(Vec2{Y: 50})
	header.UIState().SetMinimumSize(Vec2{Y: 50})

	tallStack := NewStack(Vertical, 8)
	for i := 0; i < 40; i++ {
		tallStack.Add(NewButton("Row", nil))
	}
	tab1 := NewScrollPanel(tallStack)
	tab1.UIState().SetFlex(1)

	tab2 := NewCodeEditor(nil)
	tab2.UIState().SetFlex(1)

	tabs := NewTabs([]Tab{
		{ID: "t1", Title: "Controls", Content: tab1},
		{ID: "t2", Title: "Editor", Content: tab2},
	}, "t1", nil)
	tabs.UIState().SetFlex(1)

	footer := NewStack(Horizontal, 8)
	footer.UIState().SetPreferredSize(Vec2{Y: 36})
	footer.UIState().SetMinimumSize(Vec2{Y: 36})

	root.Add(menu, header, tabs, footer)
	mgr := New()
	mgr.SetRoot(root)
	mgr.BeginFrame(800, 600, 0)
	if menu.Bounds().H < 34 || header.Bounds().H < 50 || footer.Bounds().H < 36 {
		t.Fatalf("top/bottom panels shrunk on tab1: menu=%v header=%v footer=%v", menu.Bounds().H, header.Bounds().H, footer.Bounds().H)
	}
	tabs.Select("t2")
	mgr.BeginFrame(800, 600, 0)
	if menu.Bounds().H < 34 || header.Bounds().H < 50 || footer.Bounds().H < 36 {
		t.Fatalf("top/bottom panels shrunk on tab2: menu=%v header=%v footer=%v", menu.Bounds().H, header.Bounds().H, footer.Bounds().H)
	}
}
