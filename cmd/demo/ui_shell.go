package main

import (
	"fmt"
	"runtime"
	"time"

	ui "github.com/banditmoscow1337/nativeForms"
	"github.com/banditmoscow1337/nativeForms/platform"
)

func newSectionHeaderKey(key string) *ui.Label {
	lbl := ui.NewLabelKey(key)
	lbl.Font = ui.FontSpec{Size: 15, Weight: 700}
	lbl.Wrap = true
	return lbl
}

func newFlexButtonKey(key string, onClick func()) *ui.Button {
	btn := ui.NewButton(key, onClick)
	btn.SetTextKey(key)
	btn.UIState().SetFlex(1)
	return btn
}

func (d *demoApp) buildMenuBar() *ui.MenuBar {
	return ui.NewMenuBar(
		ui.MenuBarItem{
			ID:       "file",
			LabelKey: "menu.file",
			Icon:     d.icons[0],
			Items: []ui.MenuItem{
				{ID: "new-tab", LabelKey: "menu.file.new_tab", Icon: d.icons[2], Shortcut: "Ctrl+T", Action: d.openDynamicTab},
				{ID: "open-file", LabelKey: "menu.file.open", Icon: d.icons[1], Action: func() {
					if d.window != nil {
						path, err := d.window.OpenFile()
						d.logf("MenuBar OpenFile: path=%q err=%v", path, err)
					} else {
						d.logf("MenuBar OpenFile (headless)")
					}
				}},
				{ID: "save-heap", LabelKey: "menu.file.save_heap", Icon: d.icons[0], Action: func() {
					_ = writeHeapProfile("heap.pprof")
					d.manager.Notify("heap.pprof saved")
				}},
				{Separator: true},
				{ID: "quit", LabelKey: "menu.file.quit", Icon: d.icons[3], Shortcut: "Cmd/Ctrl+Q", Action: func() {
					if d.app != nil {
						d.app.Quit()
					}
				}},
			},
		},
		ui.MenuBarItem{
			ID:       "edit",
			LabelKey: "menu.edit",
			Items: []ui.MenuItem{
				{ID: "undo", LabelKey: "menu.edit.undo", Shortcut: "Ctrl+Z", Action: func() {
					d.codeEditor.Handle(ui.Event{Type: ui.KeyDown, Key: ui.KeyZ, Mods: ui.ModControl})
					d.updateEditorInfo()
				}},
				{ID: "redo", LabelKey: "menu.edit.redo", Shortcut: "Ctrl+Y", Action: func() {
					d.codeEditor.Handle(ui.Event{Type: ui.KeyDown, Key: ui.KeyY, Mods: ui.ModControl})
					d.updateEditorInfo()
				}},
				{Separator: true},
				{ID: "copy", LabelKey: "menu.edit.copy", Shortcut: "Ctrl+C", Action: func() {
					if d.window != nil && d.window.Capabilities().Supports(platform.FeatureClipboard) {
						_ = d.window.SetClipboardText(d.clipField.Text)
						d.manager.Notify("Copied to clipboard")
					}
				}},
			},
		},
		ui.MenuBarItem{
			ID:       "view",
			LabelKey: "menu.view",
			Items: []ui.MenuItem{
				{ID: "theme", LabelKey: "menu.view.theme", Icon: d.icons[0], Shortcut: "F2", Action: d.toggleTheme},
				{ID: "lang", LabelKey: "menu.view.lang", Icon: d.icons[2], Shortcut: "F3", Action: d.toggleLanguage},
				{ID: "frameless-controls", LabelKey: "menu.view.frameless", Icon: d.icons[1], Action: func() {
					next := !d.framelessCtrlTgl.Value
					d.framelessCtrlTgl.SetValue(next)
					d.setFramelessControls(next)
				}},
				{ID: "rounded-controls", LabelKey: "menu.view.rounded", Icon: d.icons[2], Action: func() {
					next := float32(10)
					if d.radiusSlider != nil && d.radiusSlider.Value > 0 {
						next = 0
					}
					if d.radiusSlider != nil {
						d.radiusSlider.SetValue(next)
					}
					d.setControlsCornerRadius(next)
				}},
				{ID: "autosize-controls", LabelKey: "menu.view.autosize", Icon: d.icons[3], Action: func() {
					next := !d.autoSizeToggle.Value
					d.autoSizeToggle.SetValue(next)
					d.setControlsAutoSize(next)
				}},
				{Separator: true},
				{ID: "scale-100", LabelKey: "menu.view.scale_100", Action: func() { d.setDisplayScale(1.0) }},
				{ID: "scale-125", LabelKey: "menu.view.scale_125", Action: func() { d.setDisplayScale(1.25) }},
				{ID: "scale-150", LabelKey: "menu.view.scale_150", Action: func() { d.setDisplayScale(1.5) }},
				{ID: "scale-200", LabelKey: "menu.view.scale_200", Action: func() { d.setDisplayScale(2.0) }},
			},
		},
		ui.MenuBarItem{
			ID:       "window",
			LabelKey: "menu.window",
			Items: []ui.MenuItem{
				{ID: "win-frameless", LabelKey: "menu.window.frameless", Action: func() {
					next := !d.winFramelessToggle.Value
					d.winFramelessToggle.SetValue(next)
					d.setWindowFrameless(next)
				}},
				{ID: "win-controls", LabelKey: "menu.window.controls", Action: func() {
					next := !d.winControlsToggle.Value
					d.winControlsToggle.SetValue(next)
					d.setWindowControlsVisible(next)
				}},
				{ID: "win-min", LabelKey: "menu.window.minimize", Action: func() {
					if d.window != nil {
						d.window.Minimize()
					}
				}},
				{ID: "win-max", LabelKey: "menu.window.maximize", Action: func() {
					if d.window != nil {
						d.window.Maximize()
					}
				}},
				{Separator: true},
				{ID: "tray-toggle", LabelKey: "menu.window.tray", Icon: d.icons[2], Action: func() {
					d.setTrayEnabled(!d.trayActive)
				}},
				{ID: "second-win", LabelKey: "menu.window.second", Icon: d.icons[1], Action: d.toggleSecondWindow},
			},
		},
		ui.MenuBarItem{
			ID:       "tools",
			LabelKey: "menu.tools",
			Items: []ui.MenuItem{
				{ID: "selftest", LabelKey: "menu.tools.selftest", Icon: d.icons[2], Shortcut: "F5", Action: d.runSelfTests},
				{ID: "stress", LabelKey: "menu.tools.stress", Icon: d.icons[3], Shortcut: "F6", Action: func() {
					d.setStressMode(!d.stressRunning.Load())
				}},
				{ID: "gc", LabelKey: "menu.tools.gc", Shortcut: "Ctrl+G", Action: func() {
					runtime.GC()
					d.manager.Notify("runtime.GC() executed")
				}},
			},
		},
	)
}

func (d *demoApp) buildUI() {
	root := ui.NewStack(ui.Vertical, 6)
	root.Padding = ui.All(8)
	root.Background = d.darkTheme.Panel
	root.UIState().SetAccessibility(ui.AccessibilityInfo{
		Role:        ui.RoleGroup,
		Name:        "nativeForms Demo & Profiler Root",
		Description: "Root container hosting MenuBar, all widget tabs, and live telemetry",
		Language:    d.manager.Locale(),
	})

	d.menuBar = d.buildMenuBar()
	d.menuBar.UIState().SetFlexShrink(0)
	d.menuBar.UIState().SetMinimumSize(ui.Vec2{Y: 34})
	d.menuBar.UIState().SetPreferredSize(ui.Vec2{Y: 34})

	header := ui.NewStack(ui.Horizontal, 8)
	header.Padding = ui.Symmetric(10, 6)
	header.Background = d.darkTheme.Header
	header.Border = d.darkTheme.Border
	header.BorderWidth = 1
	header.CornerRadius = 6
	header.Align = ui.AlignCenter
	header.UIState().SetFlexShrink(0)
	header.UIState().SetMinimumSize(ui.Vec2{Y: 50})
	header.UIState().SetPreferredSize(ui.Vec2{Y: 50})
	d.header = header

	title := ui.NewLabelKey("shell.header_title")
	title.Font = ui.FontSpec{Size: 16, Weight: 700}
	title.Ellipsis = true
	title.UIState().SetFlex(1)

	d.langSelect = ui.NewSegmentedControl([]ui.RadioOption{
		{ID: "en", Label: "EN"},
		{ID: "ru", Label: "RU"},
	}, d.manager.Locale(), func(id string) {
		d.setLanguage(id)
	})
	d.langSelect.SetLocaleSizePolicy(ui.LocaleSizeMaxEnvelope)
	d.langSelect.SetTooltipKey("shell.tip_lang")

	btnTheme := ui.NewButtonKey("shell.btn_theme", func() {
		d.toggleTheme()
	})
	btnTheme.SetIcon(d.icons[0])
	btnTheme.SetTooltipKey("shell.tip_theme")

	btnSelfTest := ui.NewButtonKey("shell.btn_selftest", func() {
		d.runSelfTests()
	})
	btnSelfTest.SetIcon(d.icons[2])
	btnSelfTest.SetTooltipKey("shell.tip_selftest")

	btnStress := ui.NewButtonKey("shell.btn_stress", func() {
		next := !d.stressRunning.Load()
		d.setStressMode(next)
	})
	btnStress.SetIcon(d.icons[3])
	btnStress.SetTooltipKey("shell.tip_stress")

	header.Add(title, d.langSelect, btnTheme, btnSelfTest, btnStress)

	d.tabs = ui.NewTabs([]ui.Tab{
		{
			ID:          "controls",
			TitleKey:    "tab.controls",
			Icon:        d.icons[0],
			NonClosable: true,
			Content:     d.buildControlsTab(),
		},
		{
			ID:          "editor",
			TitleKey:    "tab.editor",
			Icon:        d.icons[1],
			NonClosable: true,
			Content:     d.buildEditorTab(),
		},
		{
			ID:          "virtual",
			TitleKey:    "tab.virtual",
			Icon:        d.icons[2],
			NonClosable: true,
			Content:     d.buildVirtualTab(),
		},
		{
			ID:          "popups",
			TitleKey:    "tab.popups",
			Icon:        d.icons[3],
			NonClosable: true,
			Content:     d.buildPopupsAndPlatformTab(),
		},
		{
			ID:          "profiler",
			TitleKey:    "tab.profiler",
			Icon:        d.icons[1],
			NonClosable: true,
			Content:     d.buildProfilerTab(),
		},
	}, "controls", func(selected string) {
		if !d.stressRunning.Load() {
			d.logf("Tab selected: %s", selected)
		}
	})
	d.tabs.UIState().SetFlex(1)
	d.tabs.OnReorder = func(id string, index int) {
		d.logf("Tab %q moved to index %d", id, index)
	}
	d.tabs.OnCloseRequest = func(id string) bool {
		d.logf("Closing dynamic tab %q", id)
		return true
	}

	footer := ui.NewStack(ui.Horizontal, 12)
	footer.Padding = ui.Symmetric(10, 4)
	footer.Background = d.darkTheme.Header
	footer.Border = d.darkTheme.Border
	footer.BorderWidth = 1
	footer.CornerRadius = 6
	footer.UIState().SetFlexShrink(0)
	footer.UIState().SetMinimumSize(ui.Vec2{Y: 36})
	footer.UIState().SetPreferredSize(ui.Vec2{Y: 36})
	d.footer = footer

	d.statusLabel = ui.NewLabelKey("shell.status_ready")
	d.statusLabel.Ellipsis = true
	d.statusLabel.UIState().SetFlex(1)

	d.perfBanner = ui.NewLabel("")
	d.perfBanner.TextFunc = d.shortPerfSummary
	d.perfBanner.Align = ui.TextRight
	d.perfBanner.UIState().SetPreferredSize(ui.Vec2{X: 480, Y: 26})

	footer.Add(d.statusLabel, d.perfBanner)

	root.Add(d.menuBar, header, d.tabs, footer)
	d.manager.SetRoot(root)
	d.updateEditorInfo()
}

func (d *demoApp) setLanguage(locale string) {
	if locale != "en" && locale != "ru" {
		locale = "en"
	}
	if d.manager.Locale() == locale {
		return
	}
	d.manager.SetLocale(locale)
	if d.subMgr != nil {
		d.subMgr.RefreshLocale()
	}
	if d.langSelect != nil && d.langSelect.SelectedID != locale {
		d.langSelect.SetSelected(locale)
	}
	d.mu.Lock()
	d.card1Cache, d.card2Cache, d.card3Cache = throttledText{}, throttledText{}, throttledText{}
	d.mu.Unlock()
	d.updateEditorInfo()
	if d.trayActive && d.app != nil {
		d.setTrayEnabled(true)
	}
	d.logf("Locale switched to %q (LanguageManager)", locale)
}

func (d *demoApp) toggleLanguage() {
	if d.manager.Locale() == "ru" {
		d.setLanguage("en")
	} else {
		d.setLanguage("ru")
	}
}

func (d *demoApp) setDisplayScale(scale float32) {
	if !(scale > 0 && scale <= 8) {
		scale = 1
	}
	d.manager.SetDisplayScale(scale)
	if d.comboBox != nil {
		id := fmt.Sprintf("%.3f", scale)
		if d.comboBox.SelectedID != id {
			d.comboBox.SetSelected(id)
		}
	}
	d.logf("DisplayScale: %.3fx", scale)
}

func (d *demoApp) setFramelessControls(frameless bool) {
	for _, comp := range d.framelessControls {
		if comp != nil {
			comp.UIState().SetFrameless(frameless)
		}
	}
	d.logf("State.SetFrameless: %v", frameless)
}

func (d *demoApp) setControlsCornerRadius(radius float32) {
	d.darkTheme.CornerRadius = radius
	d.lightTheme.CornerRadius = radius
	for _, comp := range d.roundedControls {
		if comp != nil {
			comp.UIState().SetCornerRadius(radius)
		}
	}
	if d.isLight {
		d.manager.SetTheme(d.lightTheme)
	} else {
		d.manager.SetTheme(d.darkTheme)
	}
	d.logf("SetCornerRadius: %.0f px", radius)
}

func (d *demoApp) setControlsAutoSize(enabled bool) {
	for _, comp := range d.autoSizeControls {
		if comp != nil {
			comp.UIState().SetAutoSize(enabled)
			if enabled {
				comp.UIState().SizeToText()
			}
		}
	}
	d.logf("SetAutoSize / SizeToText: %v", enabled)
}

func (d *demoApp) applyPickedAccentColor(c ui.Color) {
	hover := ui.Color{
		R: min(1, c.R+0.12),
		G: min(1, c.G+0.12),
		B: min(1, c.B+0.12),
		A: c.A,
	}
	d.darkTheme.Accent = c
	d.darkTheme.AccentHover = hover
	d.lightTheme.Accent = c
	d.lightTheme.AccentHover = hover
	if d.progressBar != nil {
		d.progressBar.Fill = c
	}
	if d.overlayBadge != nil {
		d.overlayBadge.Background = c.WithAlpha(0.85)
	}
	if d.isLight {
		d.manager.SetTheme(d.lightTheme)
	} else {
		d.manager.SetTheme(d.darkTheme)
	}
	if !d.stressRunning.Load() {
		d.logf("ColorPicker accent color: %s", c.HexAlpha())
	}
}

func (d *demoApp) openDynamicTab() {
	id := fmt.Sprintf("dyn-%d", time.Now().UnixNano()%100000)
	content := ui.NewStack(ui.Vertical, 8)
	content.Padding = ui.All(16)
	var nonClosable bool
	infoLbl := ui.NewLabel("")
	infoLbl.SetOnLocaleChange(func(lm *ui.LanguageManager) {
		infoLbl.SetText(lm.Trf("tab.dyn_info", id))
	})
	btnDirty := ui.NewButton("", func() {
		d.tabs.SetDirty(id, true)
	})
	btnDirty.SetTextKey("tab.dyn_dirty")
	btnLock := ui.NewButton("", func() {
		nonClosable = !nonClosable
		d.tabs.SetNonClosable(id, nonClosable)
		d.logf("Tabs.SetNonClosable(%s, %v)", id, nonClosable)
	})
	btnLock.SetTextKey("tab.dyn_lock")
	content.Add(infoLbl, btnDirty, btnLock)
	d.tabs.Open(ui.Tab{
		ID:      id,
		Title:   d.trf("tab.dyn_title", id),
		Icon:    d.icons[2],
		Content: content,
		Dirty:   true,
	})
}

func (d *demoApp) registerShortcuts() {
	d.manager.AddShortcut(ui.Shortcut{
		Key:          ui.KeyF2,
		AllowInModal: true,
		Action:       d.toggleTheme,
	})
	d.manager.AddShortcut(ui.Shortcut{
		Key:          ui.KeyF3,
		AllowInModal: true,
		Action:       d.toggleLanguage,
	})
	d.manager.AddShortcut(ui.Shortcut{
		Key:          ui.KeyF5,
		AllowInModal: true,
		Action:       d.runSelfTests,
	})
	d.manager.AddShortcut(ui.Shortcut{
		Key:          ui.KeyF6,
		AllowInModal: true,
		Action: func() {
			d.setStressMode(!d.stressRunning.Load())
		},
	})
	d.manager.AddShortcut(ui.Shortcut{
		Key:  ui.KeyT,
		Mods: ui.ModControl,
		Action: func() {
			d.openDynamicTab()
		},
	})
	d.manager.AddShortcut(ui.Shortcut{
		Key:  ui.KeyG,
		Mods: ui.ModControl,
		Action: func() {
			runtime.GC()
			d.manager.Notify("runtime.GC() executed")
		},
	})
}

func (d *demoApp) toggleTheme() {
	d.isLight = !d.isLight
	if d.isLight {
		d.codeEditor.Palette = lightSyntaxPalette()
		d.manager.SetTheme(d.lightTheme)
		d.logf("Theme switched to Light Theme")
	} else {
		d.codeEditor.Palette = darkSyntaxPalette()
		d.manager.SetTheme(d.darkTheme)
		d.logf("Theme switched to Dark Theme")
	}
}
