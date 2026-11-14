package main

import ui "github.com/banditmoscow1337/nativeForms"

func buildDemoLanguageManager(defaultLocale string) *ui.LanguageManager {
	if defaultLocale != "ru" && defaultLocale != "en" {
		defaultLocale = "en"
	}
	lm := ui.NewLanguageManager(defaultLocale)
	lm.SetFallback("en")

	lm.Register("en", map[string]string{
		// Window & Shell
		"app.window_title":   "nativeForms — Widget Showcase & CPU/Memory Profiler",
		"app.second_title":   "nativeForms — Secondary Window",
		"shell.header_title": "nativeForms — Full Showcase: ColorPicker, CornerRadius, AutoSize, MenuBar, Tray & Profiler",
		"shell.btn_theme":    "Theme: Dark/Light (F2)",
		"shell.tip_theme":    "Switch color theme (SetTheme)",
		"shell.btn_selftest": "API Self-Test (F5)",
		"shell.tip_selftest": "Verify all widgets, ColorPicker, CornerRadius, AutoSize, i18n, MenuBar, and virtualization",
		"shell.btn_stress":   "Stress Test (F6)",
		"shell.tip_stress":   "Start/stop automated stress workload across all tabs for profiling",
		"shell.tip_lang":     "Switch UI language (F3) — LanguageManager & LocaleSizeMaxEnvelope",
		"shell.status_ready": "Status: Ready. Press F2 for theme, F3 for language (EN/RU), F5 for self-test, or F6 for stress test.",

		// MenuBar
		"menu.file":               "File",
		"menu.file.new_tab":       "New Dynamic Tab",
		"menu.file.open":          "Open File (OpenFile)...",
		"menu.file.save_heap":     "Save Heap Profile (heap.pprof)",
		"menu.file.quit":          "Quit",
		"menu.edit":               "Edit",
		"menu.edit.undo":          "Undo (CodeEditor Undo)",
		"menu.edit.redo":          "Redo (CodeEditor Redo)",
		"menu.edit.copy":          "Copy Sample Text to Clipboard",
		"menu.view":               "View",
		"menu.view.theme":         "Toggle Theme (Dark / Light)",
		"menu.view.lang":          "Switch Language (EN / RU)",
		"menu.view.frameless":     "Toggle Control Borders (SetFrameless)",
		"menu.view.rounded":       "Toggle Corner Radius (0px / 10px)",
		"menu.view.autosize":      "Auto-Size Controls to Text (SetAutoSize)",
		"menu.view.scale_100":     "DPI Scale 1.0x",
		"menu.view.scale_125":     "DPI Scale 1.25x",
		"menu.view.scale_150":     "DPI Scale 1.5x",
		"menu.view.scale_200":     "DPI Scale 2.0x",
		"menu.window":             "Window & Tray",
		"menu.window.frameless":   "Frameless OS Window (Window.SetFrameless)",
		"menu.window.controls":    "Show/Hide OS Window Buttons (SetWindowControls)",
		"menu.window.minimize":    "Minimize Window (Minimize)",
		"menu.window.maximize":    "Maximize / Zoom Window (Maximize)",
		"menu.window.tray":        "Toggle System Tray Icon (SetTray)",
		"menu.window.second":      "Toggle Secondary Window (App.NewWindow)",
		"menu.tools":              "Tools",
		"menu.tools.selftest":     "Run API Self-Test",
		"menu.tools.stress":       "Toggle Stress Test",
		"menu.tools.gc":           "Run runtime.GC()",

		// Tabs
		"tab.controls": "1. Widgets & Forms",
		"tab.editor":   "2. CodeEditor & Text",
		"tab.virtual":  "3. Virtualization & Logs",
		"tab.popups":   "4. Popups, Tray, Window & OS",
		"tab.profiler": "5. CPU/RAM Profiler",
		"tab.dyn_title": "Tab %s",
		"tab.dyn_info":  "Dynamic tab %s (drag header to reorder or close with ×)",
		"tab.dyn_dirty": "Mark Tab Modified (SetDirty)",
		"tab.dyn_lock":  "Toggle Close Lock (SetNonClosable)",

		// Tab 1: Controls Left Pane
		"ctrl.sec_left":          "Core Controls, Toolbar, SpinBox, SegmentedControl & Accordion",
		"tb.click":               "+1 Click",
		"tb.tip_click":           "Increment click counter",
		"tb.indeterminate":       "Indeterminate",
		"tb.tip_indeterminate":   "Toggle indeterminate state on Checkbox and ProgressBar",
		"tb.selftest":            "Self-Test",
		"tb.tip_selftest":        "Run widget self-test suite (F5)",
		"tb.theme":               "Theme",
		"tb.tip_theme":           "Switch Dark / Light theme (F2)",
		"tb.lang":                "EN/RU",
		"tb.tip_lang":            "Switch UI language (F3)",
		"ctrl.btn_counter":       "Click counter: %d",
		"ctrl.tip_counter":       "Button with icon and tab Dirty-state tracking",
		"ctrl.badge_clicks":      "Clicks: %d",
		"ctrl.tip_badge":         "Compact Badge indicator with AutoSize",
		"ctrl.spinbox":           "Numeric field (SpinBox):",
		"ctrl.tip_spinbox":       "SpinBox: −/+ buttons, mouse wheel, Up/Down/PageUp/PageDown/Home/End keys",
		"ctrl.tip_segmented":     "SegmentedControl: horizontal mode switcher with icons and arrow-key navigation",
		"ctrl.sep_geometry":      "Geometry, Corner Radius & AutoSize",
		"ctrl.slider_radius":     "Control corner radius (SetCornerRadius)",
		"ctrl.tgl_autosize":      "Auto-size controls to fit text (SetAutoSize)",
		"ctrl.autosize_sample_0": "OK",
		"ctrl.autosize_sample_1": "Short text (SizeToText)",
		"ctrl.autosize_sample_2": "Button automatically expands and shrinks to fit its current localized text!",
		"ctrl.tip_autosize_btn":  "Click to cycle text length and trigger automatic size fitting (SizeToText / AutoSize)",
		"ctrl.autosize_badge_init": "AutoSize badge: step #2 (auto-width to text)",
		"ctrl.autosize_badge_fmt":  "AutoSize badge: step #%d (%.0fx%.0f px)",
		"ctrl.tgl_frameless":     "Disable standard control borders/background (SetFrameless)",
		"ctrl.tgl_gamenav":       "Gamepad / keyboard navigation mode (SetGameNavigation)",
		"ctrl.chk_enable":        "Enable form controls (SetEnabled / Indeterminate)",
		"ctrl.tgl_indeterminate": "Indeterminate mode (Checkbox + ProgressBar)",
		"ctrl.combo_opt_fmt":     "Option #%02d — DisplayScale %.3fx",
		"ctrl.tip_combo":         "Scrollable ComboBox (16 items, MaxVisibleItems=8)",
		"ctrl.slider_step":       "Progress (5% step)",
		"ctrl.slider_wave":       "Canvas wave phase (continuous)",
		"ctrl.acc_sec1_title":    "1. Help & Quick Links (Collapsible + Hyperlink)",
		"ctrl.acc_sec1_body":     "Accordion Section #1: supports keyboard (Enter/Space/Left/Right) and SingleOpen mode.",
		"ctrl.acc_link":          "Open nativeForms documentation (github.com/banditmoscow1337/nativeForms)",
		"ctrl.acc_sec2_title":    "2. Additional Accordion Container Details",
		"ctrl.acc_sec2_body":     "Accordion Section #2: expanding this section automatically collapses the first one.",
		"ctrl.overflow_lbl":      "Long text clipping check (Font 24px): this label stays strictly inside its widget bounds at any size",
		"ctrl.overflow_btn":      "Button with a very long label (22px) that clips cleanly inside the button border",

		// Tab 1: Controls Right Pane (Forms & Graphics)
		"ctrl.sec_right":         "ColorPicker Widget (HSV/Alpha/Presets + Popup), Localized Forms & Graphics",
		"ctrl.colorpicker":       "Accent color (ColorPicker HSV + Alpha):",
		"ctrl.tip_colorpicker":   "Interactive ColorPicker: Saturation/Value square, Hue & Alpha bars, presets, and Hex copy/paste (Ctrl+C / Ctrl+V)",
		"form.lbl_popup_color":   "Color (Popup):",
		"form.btn_popup_color":   "Pick color",
		"form.tip_popup_color":   "Compact ColorPicker: opens popup HSV picker on click or Enter/Space",
		"form.lbl_autofield":     "Auto-size input:",
		"form.val_autofield":     "Type here — field grows!",
		"form.ph_autofield":      "AutoSize...",
		"form.lbl_digits":        "Digits only:",
		"form.ph_digits":         "Enter number...",
		"form.a11y_digits":       "Accepts only digits 0-9",
		"form.lbl_password":      "Password:",
		"form.ph_password":       "Password...",
		"form.lbl_readonly":      "Read-only:",
		"form.val_readonly":      "ReadOnly TextField (cannot be modified)",
		"form.val_notes":         "Multiline TextArea (Undo/Redo, word selection, soft wrap).\nSecond line with UTF-8 & Cyrillic symbols: № € ₽ — \"test\".",
		"ctrl.btn_swap_img":      "ImageResource.Replace / SetFilter",
		"ctrl.btn_scroll_top":    "ScrollIntoView (to counter)",

		// Tab 2: CodeEditor & Text
		"ed.btn_undo":      "Undo",
		"ed.btn_redo":      "Redo",
		"ed.btn_ctx":       "Context Menu (Right-Click / Shift+F10)",
		"ed.btn_gotodef":   "Go to Definition (F12)",
		"ed.btn_compl":     "Autocomplete",
		"ed.btn_bp":        "Breakpoint (Line 17)",
		"ed.btn_tokens":    "LSP Tokens",
		"ed.btn_5k":        "5,000 Lines",
		"ed.compl_disabled": "Unavailable suggestion (Disabled)",
		"ed.info_fmt":      "Document v%d | Lines: %d | Caret: L%d:C%d (symbol: %s) | Visible: [%d..%d) | Breakpoints: %v | Right-Click / Shift+F10 — context menu",

		// Tab 3: Virtualized Views & Output
		"virt.btn_1k":     "1,000 rows",
		"virt.btn_10k":    "10,000 rows",
		"virt.btn_50k":    "50,000 rows",
		"virt.btn_expand": "Expand / Collapse Tree",
		"virt.btn_log":    "Append 100 Lines to OutputPanel",
		"virt.lbl_tree":   "VirtualTree (Right-Click for context menu):",
		"virt.lbl_list":   "VirtualList (Right-Click for context menu):",
		"virt.lbl_stack":  "VirtualStack (StackModel + StackModelUpdater):",
		"virt.lbl_table":  "VirtualTable (2D Scroll + Right-Click menu):",
		"virt.lbl_output": "OutputPanel (streaming log with Right-Click menu):",
		"virt.btn_clear":  "Clear",
		"virt.init_log":   "=== OutputPanel initialization complete ===\n",

		// Tab 4: Popups, System Tray, Window & Platform
		"plat.sec_popups":       "Overlays: Dialog, ContextMenu, OpenMenu, UnfocusedPopup, Tooltip, Notifications",
		"plat.btn_modal":        "Open Modal Dialog (OpenDialog / NewDialog)",
		"plat.dlg_title":        "Modal Dialog Verification (RoleDialog)",
		"plat.dlg_body":         "A modal dialog blocks outside clicks and focuses nested action buttons. Press Escape or click a button below to close.",
		"plat.dlg_confirm":      "Confirm",
		"plat.dlg_cancel":       "Cancel",
		"plat.dlg_confirmed":    "Modal dialog confirmed",
		"plat.btn_ctx_menu":     "OpenContextMenu",
		"plat.ctx_notify":       "Show Notification",
		"plat.ctx_notify_msg":   "Menu action triggered!",
		"plat.ctx_newtab":       "Open New Tab",
		"plat.ctx_checked":      "Checked Item (Checked)",
		"plat.ctx_disabled":     "Disabled Item (Disabled)",
		"plat.ctx_clear":        "Clear Notifications",
		"plat.btn_anchored":     "OpenMenu (Anchored)",
		"plat.anc_item1":        "Anchored Menu Item #1",
		"plat.anc_item2":        "Anchored Menu Item #2",
		"plat.btn_unfocused":    "OpenUnfocusedPopup (Inspector)",
		"plat.unfocused_body":   "Floating popup that keeps input focus on the owner widget (OpenUnfocusedPopup)",
		"plat.unfocused_close":  "Close Popup",
		"plat.btn_tooltip":      "ShowTooltip",
		"plat.tooltip_text":     "Passive tooltip (passes pointer clicks through)",
		"plat.btn_notify":       "Notify / ShowMessage",
		"plat.notify_fmt":       "Toast notification (%f sec)",
		"plat.btn_suspend":      "Suspend / Restore Notifications",
		"plat.btn_tab_open":     "Tabs.Open (New Tab)",
		"plat.btn_tab_move":     "Tabs.Move (Move Tab)",
		"plat.sec_tray":         "System Tray (App.SetTray) & OS Window Frame/Controls (Window)",
		"plat.tgl_tray":         "System tray / macOS menu bar icon (App.SetTray / RemoveTray)",
		"plat.tgl_win_frame":    "Disable standard OS window frame & titlebar (Window.SetFrameless)",
		"plat.tgl_win_controls": "Show standard OS window control buttons (Window.SetWindowControls)",
		"plat.btn_hide_show":    "Hide() for 1s -> Show()",
		"plat.btn_second_win":   "Secondary Window (NewWindow)",
		"plat.sub_title":        "Secondary Window (MultipleWindows / App.NewWindow)",
		"plat.sub_info":         "Each window runs its own independent ui.Manager, focus state, language bindings, and Frameless mode.",
		"plat.sub_btn_frame":    "Toggle Frameless Mode (SetFrameless)",
		"plat.sub_btn_close":    "Close Secondary Window",
		"plat.sec_policy":       "Window Close Policy (OnCloseRequest), Accessibility & Clipboard",
		"plat.pol_quit":         "OnCloseRequest: platform.QuitApp (quit application)",
		"plat.pol_hide":         "OnCloseRequest: platform.HideWindow (minimize to system tray)",
		"plat.pol_cancel":       "OnCloseRequest: platform.CancelClose (prevent closing)",
		"plat.a11y_hint":        "Click \"Capture AccessibilitySnapshot\" to inspect the accessibility tree.",
		"plat.btn_a11y":         "Capture AccessibilitySnapshot + PerformAccessibilityAction",
		"plat.a11y_fmt":         "AccessibilityTree: roots=%d, nodes=%d, focused ID=%d | Button=%d, TextField=%d, Slider=%d, Menu=%d | PerformAction(Slider++): %v",
		"plat.clip_init":        "UTF-8 clipboard test text (press Ctrl+K inside this field to clear)",
		"plat.btn_copy":         "Copy to Clipboard (SetClipboardText)",
		"plat.btn_paste":        "Paste from Clipboard (ClipboardText)",
		"plat.cap_headless":     "Capabilities: Headless Software Renderer (no native OS window)",
		"plat.tray_show":        "Show Main Window",
		"plat.tray_hide":        "Hide Main Window (Hide)",
		"plat.tray_theme":       "Toggle Dark/Light Theme",
		"plat.tray_lang":        "Switch Language (EN / RU)",
		"plat.tray_selftest":    "Run API Self-Test",
		"plat.tray_quit":        "Quit Application",

		// Tab 5: CPU & Memory Profiling
		"prof.title":        "CPU & Memory Profiling (FrameObserver, runtime/metrics, pprof, FontContext Cache)",
		"prof.tgl_allocs":   "Track per-frame allocations (FrameObserverOptions.TrackAllocations)",
		"prof.tgl_stress":   "Automated stress workload generator (Stress Monkey)",
		"prof.btn_gc":       "Force GC + FreeOSMemory",
		"prof.btn_heap":     "Save Heap Profile (heap.pprof)",
		"prof.btn_allocs":   "Save Allocs/Goroutine Profile",
		"prof.btn_reset":    "Reset Peak Frame Metrics",
		"prof.card1_title":  "Frame & Renderer Timings (FrameSample)",
		"prof.card1_fmt": "Frames: %d | Backend: %s\n" +
			"Size: %dx%d (Damage: %d px)\n" +
			"Build: %v (avg %v, max %v)\n" +
			"Layout: %v (max %v)\n" +
			"Paint: %v (max %v)\n" +
			"TextLayout: %v | Raster: %v\n" +
			"Upload: %v | Present: %v\n" +
			"Commands: %d | Masks: %d | Images: %d\n" +
			"Glyphs: %d | TextRunes: %d\n" +
			"Batches: %d | Vertices: %d | Uploads: %d (%s)",
		"prof.card2_title": "Memory & GC (runtime/metrics — 0 STW)",
		"prof.card2_fmt": "HeapAlloc (live heap): %s\n" +
			"HeapInuse: %s | HeapIdle: %s\n" +
			"HeapSys: %s | TotalSys: %s\n" +
			"HeapObjects: %d\n" +
			"TotalAlloc (cumulative): %s\n" +
			"Mallocs: %d | Frees: %d\n" +
			"GC Cycles (NumGC): %d\n" +
			"Goroutines: %d | GOMAXPROCS: %d\n" +
			"Frame Allocs: %s (%d obj)\n" +
			"Cumulative Frame Allocs: %s (%d obj)",
		"prof.card3_title": "Font Cache & Stress Test (FontContext)",
		"prof.card3_fmt": "Glyph Mask Cache: %s (%d glyphs)\n" +
			"Text Layout Cache: %s (%d lines)\n" +
			"Layout Hits: %d | Misses: %d (%.1f%%)\n" +
			"Last Frame: +%d hits / +%d misses\n" +
			"Stress Mode Active: %v\n" +
			"Stress Steps: %d\n" +
			"DisplayScale: %.2fx | Locale: %s\n" +
			"Profiling Tips:\n" +
			"  go tool pprof -http=:8080 cpu.pprof\n" +
			"  go tool pprof -http=:8080 mem.pprof\n" +
			"  python3 tools/summarize_perftrace.py frames.jsonl",
		"selftest.done_fmt": "Self-test complete: %d PASS, %d FAIL",
	})

	lm.Register("ru", map[string]string{
		// Window & Shell
		"app.window_title":   "nativeForms — Демо виджетов и Профилировщик CPU/Памяти",
		"app.second_title":   "nativeForms — Второе окно",
		"shell.header_title": "nativeForms — Полное демо: ColorPicker, Скругления, Авторазмер, MenuBar, Трей и Профилировщик",
		"shell.btn_theme":    "Тема: Dark/Light (F2)",
		"shell.tip_theme":    "Переключить цветовую тему (SetTheme)",
		"shell.btn_selftest": "Авто-тест API (F5)",
		"shell.tip_selftest": "Проверить корректность всех виджетов, ColorPicker, скруглений, авторазмера, i18n, MenuBar и виртуализации",
		"shell.btn_stress":   "Стресс-тест (F6)",
		"shell.tip_stress":   "Запустить/остановить автоматическую нагрузку на все вкладки для профилирования",
		"shell.tip_lang":     "Переключить язык интерфейса (F3) — LanguageManager и LocaleSizeMaxEnvelope",
		"shell.status_ready": "Status: Готово. Нажмите F2 для смены темы, F3 для смены языка (EN/RU), F5 для самопроверки или F6 для стресс-теста.",

		// MenuBar
		"menu.file":               "Файл",
		"menu.file.new_tab":       "Новая динамическая вкладка",
		"menu.file.open":          "Открыть файл (OpenFile)...",
		"menu.file.save_heap":     "Сохранить Heap Profile (heap.pprof)",
		"menu.file.quit":          "Выход",
		"menu.edit":               "Правка",
		"menu.edit.undo":          "Отменить (CodeEditor Undo)",
		"menu.edit.redo":          "Повторить (CodeEditor Redo)",
		"menu.edit.copy":          "Скопировать тестовую строку в буфер",
		"menu.view":               "Вид",
		"menu.view.theme":         "Переключить тему (Dark / Light)",
		"menu.view.lang":          "Переключить язык (EN / RU)",
		"menu.view.frameless":     "Переключить рамки контролов (SetFrameless)",
		"menu.view.rounded":       "Переключить скругление углов (0px / 10px)",
		"menu.view.autosize":      "Авторазмер элементов под текст (SetAutoSize)",
		"menu.view.scale_100":     "Масштаб DPI 1.0x",
		"menu.view.scale_125":     "Масштаб DPI 1.25x",
		"menu.view.scale_150":     "Масштаб DPI 1.5x",
		"menu.view.scale_200":     "Масштаб DPI 2.0x",
		"menu.window":             "Окно и Трей",
		"menu.window.frameless":   "Окно без стандартной рамки ОС (Window.SetFrameless)",
		"menu.window.controls":    "Показать/скрыть кнопки окна ОС (SetWindowControls)",
		"menu.window.minimize":    "Свернуть окно (Minimize)",
		"menu.window.maximize":    "Развернуть / Zoom (Maximize)",
		"menu.window.tray":        "Включить/выключить иконку в системном трее (SetTray)",
		"menu.window.second":      "Открыть/закрыть дополнительное окно (App.NewWindow)",
		"menu.tools":              "Инструменты",
		"menu.tools.selftest":     "Запустить самопроверку API",
		"menu.tools.stress":       "Стресс-тест вкл/выкл",
		"menu.tools.gc":           "Выполнить runtime.GC()",

		// Tabs
		"tab.controls":  "1. Виджеты и Формы",
		"tab.editor":    "2. CodeEditor и Текст",
		"tab.virtual":   "3. Виртуализация и Логи",
		"tab.popups":    "4. Попапы, Трей, Окно и ОС",
		"tab.profiler":  "5. Профилирование CPU/RAM",
		"tab.dyn_title": "Вкладка %s",
		"tab.dyn_info":  "Динамическая вкладка %s (можно перетаскивать заголовок или закрыть крестиком ×)",
		"tab.dyn_dirty": "Пометить вкладку изменённой (SetDirty)",
		"tab.dyn_lock":  "Переключить запрет закрытия (SetNonClosable)",

		// Tab 1: Controls Left Pane
		"ctrl.sec_left":            "Базовые контролы, Toolbar, SpinBox, SegmentedControl и Accordion",
		"tb.click":                 "+1 Клик",
		"tb.tip_click":             "Увеличить счётчик кликов",
		"tb.indeterminate":         "Indeterminate",
		"tb.tip_indeterminate":     "Переключить неопределённое состояние Checkbox и ProgressBar",
		"tb.selftest":              "Самопроверка",
		"tb.tip_selftest":          "Запустить проверку всех виджетов (F5)",
		"tb.theme":                 "Тема",
		"tb.tip_theme":             "Переключить Dark / Light тему (F2)",
		"tb.lang":                  "EN/RU",
		"tb.tip_lang":              "Переключить язык интерфейса (F3)",
		"ctrl.btn_counter":         "Счётчик кликов: %d",
		"ctrl.tip_counter":         "Кнопка с иконкой и отслеживанием состояния Dirty на вкладке",
		"ctrl.badge_clicks":        "Кликов: %d",
		"ctrl.tip_badge":           "Компактный индикатор Badge с авторазмером",
		"ctrl.spinbox":             "Числовое поле (SpinBox):",
		"ctrl.tip_spinbox":         "SpinBox: кнопки −/+, колесо мыши, клавиши Up/Down/PageUp/PageDown/Home/End",
		"ctrl.tip_segmented":       "SegmentedControl: горизонтальный переключатель режимов с иконками и навигацией стрелками",
		"ctrl.sep_geometry":        "Геометрия, Скругления и Авторазмер",
		"ctrl.slider_radius":       "Радиус скругления элементов (SetCornerRadius)",
		"ctrl.tgl_autosize":        "Авторазмер контролов под размер текста (SetAutoSize)",
		"ctrl.autosize_sample_0":   "OK",
		"ctrl.autosize_sample_1":   "Короткий текст (SizeToText)",
		"ctrl.autosize_sample_2":   "Кнопка автоматически расширяется и сжимается строго под длину своего текста!",
		"ctrl.tip_autosize_btn":    "Нажмите, чтобы изменить длину текста и выполнить автоматический подгон размера (SizeToText / AutoSize)",
		"ctrl.autosize_badge_init": "Бейдж авторазмера: шаг #2 (авто-ширина под текст)",
		"ctrl.autosize_badge_fmt":  "Бейдж авторазмера: шаг #%d (%.0fx%.0f px)",
		"ctrl.tgl_frameless":       "Отключить стандартные рамки/фон контролов (SetFrameless)",
		"ctrl.tgl_gamenav":         "Режим геймпад/клавиатурной навигации (SetGameNavigation)",
		"ctrl.chk_enable":          "Активировать контролы формы (SetEnabled / Indeterminate)",
		"ctrl.tgl_indeterminate":   "Режим Indeterminate (Checkbox + ProgressBar)",
		"ctrl.combo_opt_fmt":       "Опция #%02d — DisplayScale %.3fx",
		"ctrl.tip_combo":           "ComboBox с прокруткой (16 элементов, MaxVisibleItems=8)",
		"ctrl.slider_step":         "Прогресс (шаг 5%)",
		"ctrl.slider_wave":         "Фаза волны Canvas (непрерывный)",
		"ctrl.acc_sec1_title":      "1. Справка и быстрые ссылки (Collapsible + Hyperlink)",
		"ctrl.acc_sec1_body":       "Секция Accordion #1: поддерживает клавиатуру (Enter/Space/Left/Right) и режим SingleOpen.",
		"ctrl.acc_link":            "Открыть документацию nativeForms (github.com/banditmoscow1337/nativeForms)",
		"ctrl.acc_sec2_title":      "2. Дополнительные сведения о контейнере Accordion",
		"ctrl.acc_sec2_body":       "Секция Accordion #2: при открытии этой секции первая автоматически сворачивается.",
		"ctrl.overflow_lbl":        "Проверка обрезки длинного текста (Font 24px): этот текст гарантированно не выходит за границы виджета при любом размере",
		"ctrl.overflow_btn":        "Кнопка с очень длинным текстом (22px), который аккуратно обрезается внутри границ кнопки",

		// Tab 1: Controls Right Pane (Forms & Graphics)
		"ctrl.sec_right":       "Виджет ColorPicker (HSV/Alpha/Presets + Popup), Локализованные Формы и Графика",
		"ctrl.colorpicker":     "Акцентный цвет (ColorPicker HSV + Alpha):",
		"ctrl.tip_colorpicker": "Интерактивный ColorPicker: квадрат Saturation/Value, полосы Hue и Alpha, пресеты и копирование Hex (Ctrl+C / Ctrl+V)",
		"form.lbl_popup_color": "Цвет (Popup):",
		"form.btn_popup_color": "Выбрать цвет",
		"form.tip_popup_color": "Компактный ColorPicker: открывает всплывающий HSV-пикер по клику или Enter/Space",
		"form.lbl_autofield":   "Авто-поле ввода:",
		"form.val_autofield":   "Наберите текст — поле растет!",
		"form.ph_autofield":    "Авторазмер...",
		"form.lbl_digits":      "Только цифры:",
		"form.ph_digits":       "Введите число...",
		"form.a11y_digits":     "Принимает только цифры 0-9",
		"form.lbl_password":    "Пароль (Password):",
		"form.ph_password":     "Пароль...",
		"form.lbl_readonly":    "Только чтение:",
		"form.val_readonly":    "ReadOnly TextField (нельзя изменить)",
		"form.val_notes":       "Многострочное поле TextArea (Undo/Redo, выделение слов, автоперенос).\nВторая строка с кириллицей и UTF-8 символами: № € ₽ — «тест».",
		"ctrl.btn_swap_img":    "ImageResource.Replace / SetFilter",
		"ctrl.btn_scroll_top":  "ScrollIntoView (к счётчику)",

		// Tab 2: CodeEditor & Text
		"ed.btn_undo":       "Undo",
		"ed.btn_redo":       "Redo",
		"ed.btn_ctx":        "Контекстное меню (ПКМ / Shift+F10)",
		"ed.btn_gotodef":    "Go to Definition (F12)",
		"ed.btn_compl":      "Автодополнение",
		"ed.btn_bp":         "Breakpoint (Line 17)",
		"ed.btn_tokens":     "LSP Tokens",
		"ed.btn_5k":         "5 000 строк",
		"ed.compl_disabled": "Недоступное предложение (Disabled)",
		"ed.info_fmt":       "Документ v%d | Строк: %d | Каретка: L%d:C%d (символ: %s) | Видимые: [%d..%d) | Breakpoints: %v | ПКМ / Shift+F10 — контекстное меню",

		// Tab 3: Virtualized Views & Output
		"virt.btn_1k":     "1 000 записей",
		"virt.btn_10k":    "10 000 записей",
		"virt.btn_50k":    "50 000 записей",
		"virt.btn_expand": "Развернуть/Свернуть дерево",
		"virt.btn_log":    "Добавить 100 строк в OutputPanel",
		"virt.lbl_tree":   "VirtualTree (ПКМ для контекстного меню):",
		"virt.lbl_list":   "VirtualList (ПКМ для контекстного меню):",
		"virt.lbl_stack":  "VirtualStack (StackModel + StackModelUpdater):",
		"virt.lbl_table":  "VirtualTable (2D Scroll + ПКМ меню):",
		"virt.lbl_output": "OutputPanel (потоковый лог с ПКМ меню):",
		"virt.btn_clear":  "Очистить (Clear)",
		"virt.init_log":   "=== Инициализация OutputPanel завершена ===\n",

		// Tab 4: Popups, System Tray, Window & Platform
		"plat.sec_popups":       "Оверлеи: Dialog, ContextMenu, OpenMenu, UnfocusedPopup, Tooltip, Notifications",
		"plat.btn_modal":        "Открыть модальный диалог (OpenDialog / NewDialog)",
		"plat.dlg_title":        "Проверка модального диалога (RoleDialog)",
		"plat.dlg_body":         "Модальное окно блокирует клики снаружи и фокусирует вложенные кнопки. Нажмите Escape или кнопку ниже для закрытия.",
		"plat.dlg_confirm":      "Подтвердить",
		"plat.dlg_cancel":       "Отмена",
		"plat.dlg_confirmed":    "Модальный диалог подтверждён",
		"plat.btn_ctx_menu":     "OpenContextMenu",
		"plat.ctx_notify":       "Показать уведомление",
		"plat.ctx_notify_msg":   "Выбрано действие из меню!",
		"plat.ctx_newtab":       "Открыть новую вкладку",
		"plat.ctx_checked":      "Пункт с галочкой (Checked)",
		"plat.ctx_disabled":     "Неактивный пункт (Disabled)",
		"plat.ctx_clear":        "Очистить уведомления",
		"plat.btn_anchored":     "OpenMenu (у кнопки)",
		"plat.anc_item1":        "Привязанное меню #1",
		"plat.anc_item2":        "Привязанное меню #2",
		"plat.btn_unfocused":    "OpenUnfocusedPopup (инспектор)",
		"plat.unfocused_body":   "Плавающий попап без перехвата фокуса ввода (OpenUnfocusedPopup)",
		"plat.unfocused_close":  "Закрыть попап",
		"plat.btn_tooltip":      "ShowTooltip",
		"plat.tooltip_text":     "Пассивный тултип (пропускает клики сквозь себя)",
		"plat.btn_notify":       "Notify / ShowMessage",
		"plat.notify_fmt":       "Тост-уведомление (%f сек)",
		"plat.btn_suspend":      "Suspend / Restore Notifications",
		"plat.btn_tab_open":     "Tabs.Open (новая вкладка)",
		"plat.btn_tab_move":     "Tabs.Move (сдвинуть вкладку)",
		"plat.sec_tray":         "Системный трей (App.SetTray) и Управление рамкой/кнопками окна (Window)",
		"plat.tgl_tray":         "Иконка в системном трее / строке меню macOS (App.SetTray / RemoveTray)",
		"plat.tgl_win_frame":    "Отключить стандартную рамку и заголовок окна ОС (Window.SetFrameless)",
		"plat.tgl_win_controls": "Показывать стандартные кнопки управления окном (Window.SetWindowControls)",
		"plat.btn_hide_show":    "Hide() на 1 сек -> Show()",
		"plat.btn_second_win":   "Второе окно (NewWindow)",
		"plat.sub_title":        "Дополнительное окно (MultipleWindows / App.NewWindow)",
		"plat.sub_info":         "У каждого окна собственный ui.Manager, независимый фокус, привязки языка и поддержка Frameless-режима.",
		"plat.sub_btn_frame":    "Переключить рамку этого окна (SetFrameless)",
		"plat.sub_btn_close":    "Закрыть дополнительное окно",
		"plat.sec_policy":       "Политика закрытия окна (OnCloseRequest), Accessibility и Буфер обмена",
		"plat.pol_quit":         "OnCloseRequest: platform.QuitApp (завершить приложение)",
		"plat.pol_hide":         "OnCloseRequest: platform.HideWindow (свернуть в системный трей)",
		"plat.pol_cancel":       "OnCloseRequest: platform.CancelClose (заблокировать закрытие)",
		"plat.a11y_hint":        "Нажмите «Снимок AccessibilitySnapshot», чтобы проинспектировать дерево доступности.",
		"plat.btn_a11y":         "Снимок AccessibilitySnapshot + PerformAccessibilityAction",
		"plat.a11y_fmt":         "AccessibilityTree: корней=%d, узлов=%d, фокус ID=%d | Button=%d, TextField=%d, Slider=%d, Menu=%d | PerformAction(Slider++): %v",
		"plat.clip_init":        "Текст для проверки буфера обмена UTF-8 (Ctrl+K в этом поле очищает текст)",
		"plat.btn_copy":         "В буфер (SetClipboardText)",
		"plat.btn_paste":        "Из буфера (ClipboardText)",
		"plat.cap_headless":     "Capabilities: Headless Software Renderer (без нативного окна ОС)",
		"plat.tray_show":        "Показать главное окно",
		"plat.tray_hide":        "Скрыть главное окно (Hide)",
		"plat.tray_theme":       "Переключить тему Dark/Light",
		"plat.tray_lang":        "Переключить язык (EN / RU)",
		"plat.tray_selftest":    "Запустить самопроверку API",
		"plat.tray_quit":        "Выход из приложения",

		// Tab 5: CPU & Memory Profiling
		"prof.title":       "Профилирование CPU и Памяти (FrameObserver, runtime/metrics, pprof, FontContext Cache)",
		"prof.tgl_allocs":  "Отслеживать аллокации на каждый кадр (FrameObserverOptions.TrackAllocations)",
		"prof.tgl_stress":  "Автоматический стресс-генератор нагрузки (Stress Monkey)",
		"prof.btn_gc":      "Принудительный GC + FreeOSMemory",
		"prof.btn_heap":    "Сохранить Heap Profile (heap.pprof)",
		"prof.btn_allocs":  "Сохранить Allocs/Goroutine Profile",
		"prof.btn_reset":   "Сбросить пиковые метрики кадра",
		"prof.card1_title": "Тайминги кадра и рендерера (FrameSample)",
		"prof.card1_fmt": "Кадров: %d | Backend: %s\n" +
			"Размер: %dx%d (Damage: %d px)\n" +
			"Build: %v (avg %v, max %v)\n" +
			"Layout: %v (max %v)\n" +
			"Paint: %v (max %v)\n" +
			"TextLayout: %v | Raster: %v\n" +
			"Upload: %v | Present: %v\n" +
			"Commands: %d | Masks: %d | Images: %d\n" +
			"Glyphs: %d | TextRunes: %d\n" +
			"Batches: %d | Vertices: %d | Uploads: %d (%s)",
		"prof.card2_title": "Память и GC (runtime/metrics — 0 STW)",
		"prof.card2_fmt": "HeapAlloc (живая куча): %s\n" +
			"HeapInuse: %s | HeapIdle: %s\n" +
			"HeapSys: %s | TotalSys: %s\n" +
			"HeapObjects: %d\n" +
			"TotalAlloc (за всё время): %s\n" +
			"Mallocs: %d | Frees: %d\n" +
			"Циклов GC (NumGC): %d\n" +
			"Горутин: %d | GOMAXPROCS: %d\n" +
			"Аллокации кадра: %s (%d obj)\n" +
			"Сумма аллокаций кадров: %s (%d obj)",
		"prof.card3_title": "Кэш шрифтов и Стресс-тест (FontContext)",
		"prof.card3_fmt": "Glyph Mask Cache: %s (%d глифов)\n" +
			"Text Layout Cache: %s (%d строк)\n" +
			"Layout Hits: %d | Misses: %d (%.1f%%)\n" +
			"За последний кадр: +%d hits / +%d misses\n" +
			"Стресс-режим активен: %v\n" +
			"Шагов стресс-теста: %d\n" +
			"Масштаб (DisplayScale): %.2fx | Язык: %s\n" +
			"Совет по профилированию:\n" +
			"  go tool pprof -http=:8080 cpu.pprof\n" +
			"  go tool pprof -http=:8080 mem.pprof\n" +
			"  python3 tools/summarize_perftrace.py frames.jsonl",
		"selftest.done_fmt": "Самопроверка завершена: %d PASS, %d FAIL",
	})

	return lm
}
