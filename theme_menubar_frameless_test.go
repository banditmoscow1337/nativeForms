package nativeforms

import "testing"

func TestLightThemeAndPanelMigration(t *testing.T) {
	mgr := New()
	root := NewStack(Vertical, 8)
	root.Background = DefaultTheme().Panel
	root.Border = DefaultTheme().Border
	root.BorderWidth = 1

	header := NewStack(Horizontal, 4)
	header.Background = DefaultTheme().Header
	header.Border = DefaultTheme().Border
	header.BorderWidth = 1

	card := NewStack(Vertical, 4)
	card.Background = DefaultTheme().PanelAlt
	root.Add(header, card)
	mgr.SetRoot(root)

	light := LightTheme()
	if !light.IsLight() {
		t.Fatal("expected LightTheme().IsLight() to be true")
	}
	if DefaultTheme().IsLight() {
		t.Fatal("expected DefaultTheme().IsLight() to be false")
	}

	mgr.SetTheme(light)
	if root.Background != light.Panel {
		t.Fatalf("root.Background = %+v, want %+v", root.Background, light.Panel)
	}
	if header.Background != light.Header {
		t.Fatalf("header.Background = %+v, want %+v", header.Background, light.Header)
	}
	if card.Background != light.PanelAlt {
		t.Fatalf("card.Background = %+v, want %+v", card.Background, light.PanelAlt)
	}

	mgr.SetTheme(DefaultTheme())
	if root.Background != DefaultTheme().Panel {
		t.Fatalf("root.Background after restore = %+v, want %+v", root.Background, DefaultTheme().Panel)
	}
}

func TestMenuBarOpenAndSelect(t *testing.T) {
	mgr := New()
	mgr.SetInteractive(true)
	triggered := false
	bar := NewMenuBar(
		MenuBarItem{
			ID:    "file",
			Label: "File",
			Items: []MenuItem{
				{ID: "open", Label: "Open", Shortcut: "Ctrl+O", Action: func() { triggered = true }},
				{Separator: true},
				{ID: "exit", Label: "Exit"},
			},
		},
		MenuBarItem{
			ID:    "view",
			Label: "View",
			Items: []MenuItem{
				{ID: "theme", Label: "Light Theme", Checked: true},
			},
		},
	)
	root := NewStack(Vertical, 0)
	root.Add(bar)
	mgr.SetRoot(root)
	mgr.BeginFrame(800, 600, 0.016)

	if !bar.OpenMenuAt(0) {
		t.Fatal("expected OpenMenuAt(0) to open dropdown menu")
	}
	if bar.ActiveIndex() != 0 {
		t.Fatalf("expected ActiveIndex() == 0, got %d", bar.ActiveIndex())
	}
	mgr.BeginFrame(800, 600, 0.016)
	bar.CloseMenu()
	if bar.ActiveIndex() != -1 {
		t.Fatalf("expected ActiveIndex() == -1 after CloseMenu, got %d", bar.ActiveIndex())
	}
	_ = triggered
}

func TestFramelessControlsOmitBackgroundAndBorder(t *testing.T) {
	mgr := New()
	btn := NewButton("Frameless", nil)
	btn.SetFrameless(true)
	mgr.SetRoot(btn)
	mgr.BeginFrame(200, 60, 0.016)
	framelessCmds := len(mgr.Frame().Commands)

	btn.SetFrameless(false)
	mgr.BeginFrame(200, 60, 0.016)
	normalCmds := len(mgr.Frame().Commands)

	if framelessCmds >= normalCmds {
		t.Fatalf("expected frameless button to emit fewer draw commands than normal button: frameless=%d normal=%d", framelessCmds, normalCmds)
	}
}

func TestTextClippingBounds(t *testing.T) {
	mgr := New()
	lbl := NewLabel("Very long text that exceeds the narrow widget width by a large margin")
	lbl.Font = FontSpec{Size: 32}
	lbl.SetPreferredSize(Vec2{X: 80, Y: 28})
	root := NewStack(Vertical, 0)
	root.Add(lbl)
	mgr.SetRoot(root)
	mgr.BeginFrame(80, 28, 0.016)

	for i, cmd := range mgr.Frame().Commands {
		if cmd.Clip.X+cmd.Clip.W > 80.5 || cmd.Clip.Y+cmd.Clip.H > 28.5 {
			t.Fatalf("command %d clip %+v exceeds widget bounds 80x28", i, cmd.Clip)
		}
	}
}
