package main

import (
	"fmt"
	"runtime"
	"time"

	ui "github.com/banditmoscow1337/nativeForms"
	"github.com/banditmoscow1337/nativeForms/platform"
)

func newSectionHeader(text string) *ui.Label {
	lbl := ui.NewLabel(text)
	lbl.Font = ui.FontSpec{Size: 15, Weight: 700}
	lbl.Wrap = true
	return lbl
}

func newFlexButton(text string, onClick func()) *ui.Button {
	btn := ui.NewButton(text, onClick)
	btn.UIState().SetFlex(1)
	return btn
}

func newMinWidthButton(text string, minWidth float32, onClick func()) *ui.Button {
	btn := ui.NewButton(text, onClick)
	btn.UIState().SetMinimumSize(ui.Vec2{X: minWidth})
	return btn
}

func (d *demoApp) buildMenuBar() *ui.MenuBar {
	return ui.NewMenuBar(
		ui.MenuBarItem{
			ID:    "file",
			Label: "Файл",
			Icon:  d.icons[0],
			Items: []ui.MenuItem{
				{ID: "new-tab", Label: "Новая динамическая вкладка", Icon: d.icons[2], Shortcut: "Ctrl+T", Action: d.openDynamicTab},
				{ID: "open-file", Label: "Открыть файл (OpenFile)...", Icon: d.icons[1], Action: func() {
					if d.window != nil {
						path, err := d.window.OpenFile()
						d.logf("MenuBar OpenFile: path=%q err=%v", path, err)
					} else {
						d.logf("MenuBar OpenFile (headless режим)")
					}
				}},
				{ID: "save-heap", Label: "Сохранить Heap Profile (heap.pprof)", Icon: d.icons[0], Action: func() {
					_ = writeHeapProfile("heap.pprof")
					d.manager.Notify("Сохранён heap.pprof")
				}},
				{Separator: true},
				{ID: "quit", Label: "Выход", Icon: d.icons[3], Shortcut: "Cmd/Ctrl+Q", Action: func() {
					if d.app != nil {
						d.app.Quit()
					}
				}},
			},
		},
		ui.MenuBarItem{
			ID:    "edit",
			Label: "Правка",
			Items: []ui.MenuItem{
				{ID: "undo", Label: "Отменить (CodeEditor Undo)", Shortcut: "Ctrl+Z", Action: func() {
					d.codeEditor.Handle(ui.Event{Type: ui.KeyDown, Key: ui.KeyZ, Mods: ui.ModControl})
					d.updateEditorInfo()
				}},
				{ID: "redo", Label: "Повторить (CodeEditor Redo)", Shortcut: "Ctrl+Y", Action: func() {
					d.codeEditor.Handle(ui.Event{Type: ui.KeyDown, Key: ui.KeyY, Mods: ui.ModControl})
					d.updateEditorInfo()
				}},
				{Separator: true},
				{ID: "copy", Label: "Скопировать тестовую строку в буфер", Shortcut: "Ctrl+C", Action: func() {
					if d.window != nil && d.window.Capabilities().Supports(platform.FeatureClipboard) {
						_ = d.window.SetClipboardText(d.clipField.Text)
						d.manager.Notify("Скопировано в системный буфер обмена")
					}
				}},
			},
		},
		ui.MenuBarItem{
			ID:    "view",
			Label: "Вид",
			Items: []ui.MenuItem{
				{ID: "theme", Label: "Переключить тему (Dark / Light)", Icon: d.icons[0], Shortcut: "F2", Action: d.toggleTheme},
				{ID: "frameless-controls", Label: "Переключить рамки контролов (SetFrameless)", Icon: d.icons[1], Action: func() {
					next := !d.framelessCtrlTgl.Value
					d.framelessCtrlTgl.SetValue(next)
					d.setFramelessControls(next)
				}},
				{ID: "rounded-controls", Label: "Переключить скругление углов (0px / 10px)", Icon: d.icons[2], Action: func() {
					next := float32(10)
					if d.radiusSlider != nil && d.radiusSlider.Value > 0 {
						next = 0
					}
					if d.radiusSlider != nil {
						d.radiusSlider.SetValue(next)
					}
					d.setControlsCornerRadius(next)
				}},
				{ID: "autosize-controls", Label: "Авторазмер элементов под текст (SetAutoSize)", Icon: d.icons[3], Action: func() {
					next := !d.autoSizeToggle.Value
					d.autoSizeToggle.SetValue(next)
					d.setControlsAutoSize(next)
				}},
				{Separator: true},
				{ID: "scale-100", Label: "Масштаб DPI 1.0x", Action: func() { d.setDisplayScale(1.0) }},
				{ID: "scale-125", Label: "Масштаб DPI 1.25x", Action: func() { d.setDisplayScale(1.25) }},
				{ID: "scale-150", Label: "Масштаб DPI 1.5x", Action: func() { d.setDisplayScale(1.5) }},
				{ID: "scale-200", Label: "Масштаб DPI 2.0x", Action: func() { d.setDisplayScale(2.0) }},
			},
		},
		ui.MenuBarItem{
			ID:    "window",
			Label: "Окно и Трей",
			Items: []ui.MenuItem{
				{ID: "win-frameless", Label: "Окно без стандартной рамки ОС (Window.SetFrameless)", Action: func() {
					next := !d.winFramelessToggle.Value
					d.winFramelessToggle.SetValue(next)
					d.setWindowFrameless(next)
				}},
				{ID: "win-controls", Label: "Показать/скрыть кнопки окна ОС (SetWindowControls)", Action: func() {
					next := !d.winControlsToggle.Value
					d.winControlsToggle.SetValue(next)
					d.setWindowControlsVisible(next)
				}},
				{ID: "win-min", Label: "Свернуть окно (Minimize)", Action: func() {
					if d.window != nil {
						d.window.Minimize()
					}
				}},
				{ID: "win-max", Label: "Развернуть / Zoom (Maximize)", Action: func() {
					if d.window != nil {
						d.window.Maximize()
					}
				}},
				{Separator: true},
				{ID: "tray-toggle", Label: "Включить/выключить иконку в системном трее (SetTray)", Icon: d.icons[2], Action: func() {
					d.setTrayEnabled(!d.trayActive)
				}},
				{ID: "second-win", Label: "Открыть/закрыть дополнительное окно (App.NewWindow)", Icon: d.icons[1], Action: d.toggleSecondWindow},
			},
		},
		ui.MenuBarItem{
			ID:    "tools",
			Label: "Инструменты",
			Items: []ui.MenuItem{
				{ID: "selftest", Label: "Запустить самопроверку API", Icon: d.icons[2], Shortcut: "F5", Action: d.runSelfTests},
				{ID: "stress", Label: "Стресс-тест вкл/выкл", Icon: d.icons[3], Shortcut: "F6", Action: func() {
					d.setStressMode(!d.stressRunning.Load())
				}},
				{ID: "gc", Label: "Выполнить runtime.GC()", Shortcut: "Ctrl+G", Action: func() {
					runtime.GC()
					d.manager.Notify("Выполнен runtime.GC()")
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
		Language:    "ru",
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

	title := ui.NewLabel("nativeForms — Полное демо: ColorPicker, Скругления, Авторазмер, MenuBar, Трей и Профилировщик")
	title.Font = ui.FontSpec{Size: 16, Weight: 700}
	title.Ellipsis = true
	title.UIState().SetFlex(1)

	btnTheme := ui.NewButton("Тема: Dark/Light (F2)", func() {
		d.toggleTheme()
	})
	btnTheme.SetIcon(d.icons[0])
	btnTheme.UIState().SetMinimumSize(ui.Vec2{X: 180})
	btnTheme.UIState().SetTooltip("Переключить цветовую тему (SetTheme)")

	btnSelfTest := ui.NewButton("Авто-тест API (F5)", func() {
		d.runSelfTests()
	})
	btnSelfTest.SetIcon(d.icons[2])
	btnSelfTest.UIState().SetMinimumSize(ui.Vec2{X: 170})
	btnSelfTest.UIState().SetTooltip("Проверить корректность всех виджетов, ColorPicker, скруглений, авторазмера, MenuBar и виртуализации")

	btnStress := ui.NewButton("Стресс-тест (F6)", func() {
		next := !d.stressRunning.Load()
		d.setStressMode(next)
	})
	btnStress.SetIcon(d.icons[3])
	btnStress.UIState().SetMinimumSize(ui.Vec2{X: 165})
	btnStress.UIState().SetTooltip("Запустить/остановить автоматическую нагрузку на все вкладки для профилирования")

	header.Add(title, btnTheme, btnSelfTest, btnStress)

	d.tabs = ui.NewTabs([]ui.Tab{
		{
			ID:          "controls",
			Title:       "1. Виджеты и Формы",
			Icon:        d.icons[0],
			NonClosable: true,
			Content:     d.buildControlsTab(),
		},
		{
			ID:          "editor",
			Title:       "2. CodeEditor и Текст",
			Icon:        d.icons[1],
			NonClosable: true,
			Content:     d.buildEditorTab(),
		},
		{
			ID:          "virtual",
			Title:       "3. Виртуализация и Логи",
			Icon:        d.icons[2],
			NonClosable: true,
			Content:     d.buildVirtualTab(),
		},
		{
			ID:          "popups",
			Title:       "4. Попапы, Трей, Окно и ОС",
			Icon:        d.icons[3],
			NonClosable: true,
			Content:     d.buildPopupsAndPlatformTab(),
		},
		{
			ID:          "profiler",
			Title:       "5. Профилирование CPU/RAM",
			Icon:        d.icons[1],
			NonClosable: true,
			Content:     d.buildProfilerTab(),
		},
	}, "controls", func(selected string) {
		if !d.stressRunning.Load() {
			d.logf("Выбрана вкладка: %s", selected)
		}
	})
	d.tabs.UIState().SetFlex(1)
	d.tabs.OnReorder = func(id string, index int) {
		d.logf("Вкладка %q перемещена на позицию %d", id, index)
	}
	d.tabs.OnCloseRequest = func(id string) bool {
		d.logf("Закрытие динамической вкладки %q", id)
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

	d.statusLabel = ui.NewLabel("Status: Готово. Нажмите F2 для смены темы, F5 для самопроверки или F6 для стресс-теста.")
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
	d.logf("Масштаб DPI (DisplayScale): %.3fx", scale)
}

func (d *demoApp) setFramelessControls(frameless bool) {
	for _, comp := range d.framelessControls {
		if comp != nil {
			comp.UIState().SetFrameless(frameless)
		}
	}
	d.logf("Режим контролов без рамок (State.SetFrameless): %v", frameless)
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
	d.logf("Скругление углов элементов (SetCornerRadius): %.0f px", radius)
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
	d.logf("Автоматический размер под текст (SetAutoSize / SizeToText): %v", enabled)
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
		d.logf("ColorPicker выбрал акцентный цвет: %s", c.HexAlpha())
	}
}

func (d *demoApp) openDynamicTab() {
	id := fmt.Sprintf("dyn-%d", time.Now().UnixNano()%100000)
	content := ui.NewStack(ui.Vertical, 8)
	content.Padding = ui.All(16)
	var nonClosable bool
	content.Add(
		ui.NewLabel(fmt.Sprintf("Динамическая вкладка %s (можно перетаскивать заголовок или закрыть крестиком ×)", id)),
		ui.NewButton("Пометить вкладку изменённой (SetDirty)", func() {
			d.tabs.SetDirty(id, true)
		}),
		ui.NewButton("Переключить запрет закрытия (SetNonClosable)", func() {
			nonClosable = !nonClosable
			d.tabs.SetNonClosable(id, nonClosable)
			d.logf("Tabs.SetNonClosable(%s, %v)", id, nonClosable)
		}),
	)
	d.tabs.Open(ui.Tab{
		ID:      id,
		Title:   "Вкладка " + id,
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
			d.manager.Notify("Выполнен runtime.GC()")
		},
	})
}

func (d *demoApp) toggleTheme() {
	d.isLight = !d.isLight
	if d.isLight {
		d.codeEditor.Palette = lightSyntaxPalette()
		d.manager.SetTheme(d.lightTheme)
		d.logf("Установлена светлая тема (Light Theme)")
	} else {
		d.codeEditor.Palette = darkSyntaxPalette()
		d.manager.SetTheme(d.darkTheme)
		d.logf("Установлена тёмная тема (Dark Theme)")
	}
}
