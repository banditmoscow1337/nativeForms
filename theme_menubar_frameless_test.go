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

	rolePanel := NewStack(Vertical, 4)
	rolePanel.SetBackgroundRole(ThemeRoleAccent)
	rolePanel.SetBorderRole(ThemeRoleBorder, 1)

	roleLabel := NewLabel("Themed label")
	roleLabel.SetColorRole(ThemeRoleMutedText)

	root.Add(header, card, rolePanel, roleLabel)
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
	if rolePanel.Background != light.Accent {
		t.Fatalf("rolePanel.Background = %+v, want %+v", rolePanel.Background, light.Accent)
	}
	if roleLabel.Color != light.MutedText {
		t.Fatalf("roleLabel.Color = %+v, want %+v", roleLabel.Color, light.MutedText)
	}

	mgr.SetTheme(DefaultTheme())
	if root.Background != DefaultTheme().Panel {
		t.Fatalf("root.Background after restore = %+v, want %+v", root.Background, DefaultTheme().Panel)
	}
	if rolePanel.Background != DefaultTheme().Accent {
		t.Fatalf("rolePanel.Background after restore = %+v, want %+v", rolePanel.Background, DefaultTheme().Accent)
	}
}

func TestCompactButtonAndReactiveSetters(t *testing.T) {
	mgr := New()
	compact := NewCompactButton("x", nil)
	if compact.MinimumSize().X != 0 {
		t.Fatalf("expected NewCompactButton to have 0 minimum width, got %v", compact.MinimumSize().X)
	}
	sp := NewSplitPane(Horizontal, NewPanel(), NewPanel())
	sp.SetRatio(0.35)
	sp.SetMinSizes(120, 140)
	sp.SetDivider(8)
	if sp.Ratio != 0.35 || sp.MinFirst != 120 || sp.MinSecond != 140 || sp.Divider != 8 {
		t.Fatalf("SplitPane setters did not apply: %+v", sp)
	}
	mgr.Close()
}

func TestCombiningDiacriticsComposition(t *testing.T) {
	cases := []struct {
		in   string
		want rune
		size int
	}{
		{"и\u0306", 'й', 4},
		{"И\u0306", 'Й', 4},
		{"е\u0308", 'ё', 4},
		{"Е\u0308", 'Ё', 4},
		{"e\u0301", 'é', 3},
		{"a\u0308", 'ä', 3},
		{"n\u0303", 'ñ', 3},
		{"c\u0327", 'ç', 3},
	}
	for _, tc := range cases {
		gotRune, gotSize, gotCount := decodeComposedTextRune(tc.in, 0)
		if gotRune != tc.want || gotSize != tc.size || gotCount != 2 {
			t.Fatalf("decodeComposedTextRune(%q, 0) = (%U, %d, %d), want (%U, %d, 2)", tc.in, gotRune, gotSize, gotCount, tc.want, tc.size)
		}
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
