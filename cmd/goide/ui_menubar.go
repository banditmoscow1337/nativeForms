package goide

import (
	"path/filepath"

	ui "github.com/banditmoscow1337/nativeForms"
)

func (a *App) buildMenuBar() *ui.MenuBar {
	ic := a.icons
	hasDialog := a.host != nil && a.host.Capabilities().FileDialog
	shells := AvailableSystemShells()
	var shellItems []ui.MenuItem
	for _, sh := range shells {
		shPath := sh
		shellItems = append(shellItems, ui.MenuItem{
			ID:     "shell-" + filepath.Base(shPath),
			Label:  "Оболочка: " + filepath.Base(shPath) + " (" + shPath + ")",
			Icon:   ic.Command,
			Action: func() { a.SetTerminalShell(shPath) },
		})
	}

	return ui.NewMenuBar(
		ui.MenuBarItem{
			ID:    "file",
			Label: "Файл",
			Icon:  ic.Explorer,
			Items: []ui.MenuItem{
				{ID: "open-folder", Label: "Открыть Go-модуль (Open Folder)...", Icon: ic.Folder, Shortcut: "Ctrl+O", Disabled: !hasDialog, Action: a.PromptOpenProject},
				{ID: "quick-open", Label: "Быстрый переход к файлу...", Icon: ic.GoFile, Shortcut: "Ctrl+P", Action: a.OpenQuickFilePicker},
				{ID: "new-file", Label: "Новый Go-файл...", Icon: ic.GoFile, Shortcut: "Ctrl+N", Action: a.OpenNewFileDialog},
				{Separator: true},
				{ID: "save", Label: "Сохранить активный файл", Icon: ic.File, Shortcut: "Ctrl+S", Action: func() {
					if err := a.SaveActive(); err != nil {
						a.setStatus(err.Error())
					}
				}},
				{ID: "save-all", Label: "Сохранить все файлы", Icon: ic.File, Shortcut: "Ctrl+Shift+S", Action: func() {
					if err := a.saveAll(); err != nil {
						a.setStatus(err.Error())
					}
				}},
				{Separator: true},
				{ID: "split-right", Label: "Разделить редактор вправо (Split Right)", Icon: ic.GoFile, Shortcut: "Ctrl+\\", Action: a.SplitEditorRight},
				{ID: "close-tab", Label: "Закрыть активную вкладку", Shortcut: "Ctrl+W", Action: func() {
					grp := a.activeEditorTabs()
					if grp != nil && grp.SelectedID != "" {
						a.requestCloseTabInGroup(a.activeGroup, grp.SelectedID)
					}
				}},
				{ID: "close-saved", Label: "Закрыть только сохранённые файлы", Shortcut: "Ctrl+Alt+W", Action: func() {
					a.CloseSavedTabs()
				}},
				{ID: "close-all", Label: "Закрыть все открытые файлы", Shortcut: "Ctrl+Shift+W", Action: a.CloseAllTabs},
				{Separator: true},
				{ID: "refresh", Label: "Обновить дерево проекта", Icon: ic.Explorer, Action: a.RefreshProjectTree},
			},
		},
		ui.MenuBarItem{
			ID:    "edit",
			Label: "Правка",
			Items: []ui.MenuItem{
				{ID: "undo", Label: "Отменить (Undo)", Shortcut: "Ctrl+Z", Action: func() {
					if f := a.active(); f != nil {
						a.activeEditorView(f).Handle(ui.Event{Type: ui.KeyDown, Key: ui.KeyZ, Mods: ui.ModControl})
					}
				}},
				{ID: "redo", Label: "Повторить (Redo)", Shortcut: "Ctrl+Y", Action: func() {
					if f := a.active(); f != nil {
						a.activeEditorView(f).Handle(ui.Event{Type: ui.KeyDown, Key: ui.KeyY, Mods: ui.ModControl})
					}
				}},
				{Separator: true},
				{ID: "cut", Label: "Вырезать (Cut)", Shortcut: "Ctrl+X", Action: func() {
					if f := a.active(); f != nil {
						a.activeEditorView(f).Cut()
					}
				}},
				{ID: "copy", Label: "Копировать (Copy)", Shortcut: "Ctrl+C", Action: func() {
					if a.terminalOutput != nil && a.terminalOutput.UIState().Focused() && a.terminalOutput.HasSelection() {
						a.terminalOutput.Copy()
						return
					}
					if a.output != nil && a.output.UIState().Focused() && a.output.HasSelection() {
						a.output.Copy()
						return
					}
					if f := a.active(); f != nil {
						a.activeEditorView(f).Copy()
					}
				}},
				{ID: "paste", Label: "Вставить (Paste)", Shortcut: "Ctrl+V", Action: func() {
					if a.terminalOutput != nil && a.terminalOutput.UIState().Focused() {
						a.terminalOutput.Paste()
						return
					}
					if f := a.active(); f != nil {
						a.activeEditorView(f).Paste()
					}
				}},
				{ID: "select-all", Label: "Выделить всё (Select All)", Shortcut: "Ctrl+A", Action: func() {
					if a.terminalOutput != nil && a.terminalOutput.UIState().Focused() {
						a.terminalOutput.SelectAll()
						return
					}
					if a.output != nil && a.output.UIState().Focused() {
						a.output.SelectAll()
						return
					}
					if f := a.active(); f != nil {
						a.activeEditorView(f).SelectAll()
					}
				}},
				{Separator: true},
				{ID: "find", Label: "Найти и заменить в файле...", Icon: ic.Search, Shortcut: "Ctrl+F", Action: a.ToggleFindBar},
				{ID: "format", Label: "Форматировать документ (gofmt)", Icon: ic.Format, Shortcut: "Shift+Alt+F", Action: func() { a.FormatActiveDocument() }},
			},
		},
		ui.MenuBarItem{
			ID:    "view",
			Label: "Вид",
			Items: []ui.MenuItem{
				{ID: "palette", Label: "Палитра команд (Command Palette)...", Icon: ic.Command, Shortcut: "F1", Action: a.OpenCommandPalette},
				{Separator: true},
				{ID: "split-right", Label: "Разделить редактор вправо (Split Right)", Icon: ic.GoFile, Shortcut: "Ctrl+\\", Action: a.SplitEditorRight},
				{ID: "split-1", Label: "Режим редактора: 1 срез (Single)", Shortcut: "Ctrl+1", Action: func() { a.SetEditorSplits(1) }},
				{ID: "split-2", Label: "Режим редактора: 2 среза рядом (2 Splits)", Shortcut: "Ctrl+2", Action: func() { a.SetEditorSplits(2) }},
				{ID: "split-3", Label: "Режим редактора: 3 среза рядом (3 Splits)", Shortcut: "Ctrl+3", Action: func() { a.SetEditorSplits(3) }},
				{ID: "close-split", Label: "Закрыть дополнительный срез редактора", Action: a.CloseEditorSplit},
				{Separator: true},
				{ID: "sidebar-explorer", Label: "Боковая панель: Проводник (Explorer)", Icon: ic.Explorer, Shortcut: "Ctrl+Shift+E", Action: func() { a.SelectSidebarView("explorer") }},
				{ID: "sidebar-search", Label: "Боковая панель: Поиск по проекту (Search)", Icon: ic.Search, Shortcut: "Ctrl+Shift+F", Action: func() { a.SelectSidebarView("search") }},
				{ID: "sidebar-outline", Label: "Боковая панель: Символы AST (Outline)", Icon: ic.Outline, Shortcut: "Ctrl+Shift+O", Action: func() { a.SelectSidebarView("outline") }},
				{ID: "sidebar-debug", Label: "Боковая панель: Отладка (Run & Debug)", Icon: ic.Debug, Shortcut: "Ctrl+Shift+D", Action: func() { a.SelectSidebarView("debug") }},
				{ID: "sidebar-gotools", Label: "Боковая панель: Инструменты Go и SDK", Icon: ic.GoTools, Action: func() { a.SelectSidebarView("gotools") }},
				{ID: "sidebar-settings", Label: "Боковая панель: Настройки и Тема", Icon: ic.Settings, Action: func() { a.SelectSidebarView("settings") }},
				{Separator: true},
				{ID: "toggle-sidebar", Label: "Показать / скрыть боковую панель", Shortcut: "Ctrl+B", Action: a.ToggleSideBar},
				{ID: "toggle-bottom", Label: "Показать / скрыть нижнюю панель", Shortcut: "Ctrl+J", Action: a.ToggleBottomPanel},
				{ID: "toggle-theme", Label: "Переключить тему (VS Code Dark+ / Light+)", Icon: ic.Theme, Shortcut: "F2", Action: a.ToggleTheme},
			},
		},
		ui.MenuBarItem{
			ID:    "navigate",
			Label: "Переход",
			Items: []ui.MenuItem{
				{ID: "goto-def", Label: "Перейти к определению (Go to Definition)", Icon: ic.SymFunc, Shortcut: "F12", Action: a.GotoDefinition},
				{ID: "hover", Label: "Показать документацию под курсором (Hover)", Icon: ic.Info, Action: a.HoverAtCaret},
				{ID: "complete", Label: "Вызвать автодополнение (gopls Completion)", Icon: ic.SymFunc, Shortcut: "Ctrl+Space", Action: a.Complete},
				{Separator: true},
				{ID: "goto-symbol", Label: "Перейти к символу в модуле...", Icon: ic.Outline, Shortcut: "Ctrl+T", Action: a.OpenWorkspaceSymbolPicker},
				{ID: "goto-line", Label: "Перейти к строке / столбцу...", Icon: ic.Search, Shortcut: "Ctrl+G", Action: a.OpenGoToLineDialog},
				{ID: "next-problem", Label: "Перейти к следующей проблеме", Icon: ic.Warning, Shortcut: "F8", Action: a.JumpNextDiagnostic},
			},
		},
		ui.MenuBarItem{
			ID:    "gotools",
			Label: "Go",
			Icon:  ic.GoTools,
			Items: []ui.MenuItem{
				{ID: "go-build", Label: "Собрать модуль (go build ./...)", Icon: ic.Build, Shortcut: "Ctrl+Shift+B", Action: a.BuildProject},
				{ID: "go-run", Label: "Запустить пакет (go run .)", Icon: ic.Run, Shortcut: "Ctrl+F5", Action: a.RunProject},
				{ID: "go-test-all", Label: "Запустить все тесты (go test -json ./...)", Icon: ic.Test, Shortcut: "Ctrl+Shift+T", Action: a.TestProject},
				{ID: "go-test-pkg", Label: "Тестировать текущий пакет (go test -v)", Icon: ic.Test, Action: a.TestActivePackage},
				{ID: "go-vet", Label: "Проверить анализатором (go vet ./...)", Icon: ic.Vet, Action: a.VetProject},
				{ID: "go-fmt", Label: "Форматировать код (gofmt / go/format)", Icon: ic.Format, Shortcut: "Shift+Alt+F", Action: func() { a.FormatActiveDocument() }},
				{Separator: true},
				{ID: "go-tidy", Label: "Обновить зависимости (go mod tidy)", Icon: ic.GoMod, Action: a.TidyModule},
				{ID: "go-env", Label: "Показать окружение (go env)", Icon: ic.Info, Action: a.GoEnv},
				{ID: "go-stop", Label: "Остановить текущую задачу", Icon: ic.Stop, Action: a.stopRunningTask},
				{Separator: true},
				{ID: "go-autodiscover", Label: "Автопоиск Go SDK и инструментов", Icon: ic.Search, Action: func() {
					a.AutoDiscoverToolchain(true)
					a.setStatus("Выполнен автопоиск Go SDK и инструментов")
				}},
				{ID: "go-install-all", Label: "Скачать и установить все инструменты (gopls, dlv, goimports, staticcheck)", Icon: ic.GoTools, Action: a.EnsureAllGoToolsAsync},
				{ID: "go-toolchain-mgr", Label: "Управление версиями Go и инструментов...", Icon: ic.Settings, Action: a.OpenToolchainManagerDialog},
			},
		},
		ui.MenuBarItem{
			ID:    "debug",
			Label: "Отладка",
			Icon:  ic.Debug,
			Items: []ui.MenuItem{
				{ID: "dbg-main", Label: "Запустить отладку пакета (dlv dap debug)", Icon: ic.Debug, Shortcut: "F5", Action: func() { a.StartDebug("debug") }},
				{ID: "dbg-test", Label: "Отладить тесты пакета (dlv dap test)", Icon: ic.Test, Action: func() { a.StartDebug("test") }},
				{ID: "dbg-bp", Label: "Переключить точку останова (Breakpoint)", Icon: ic.Breakpoint, Shortcut: "F9", Action: a.ToggleBreakpointAtCaret},
				{Separator: true},
				{ID: "dbg-continue", Label: "Продолжить (Continue)", Icon: ic.Continue, Action: func() { a.debugControl("continue") }},
				{ID: "dbg-next", Label: "Шаг с обходом (Next / Step Over)", Icon: ic.StepOver, Shortcut: "F10", Action: func() { a.debugControl("next") }},
				{ID: "dbg-in", Label: "Шаг с заходом (Step In)", Icon: ic.StepInto, Shortcut: "F11", Action: func() { a.debugControl("stepIn") }},
				{ID: "dbg-out", Label: "Шаг с выходом (Step Out)", Icon: ic.StepOut, Shortcut: "Shift+F11", Action: func() { a.debugControl("stepOut") }},
				{ID: "dbg-stop", Label: "Остановить отладку (Stop)", Icon: ic.Stop, Shortcut: "Shift+F5", Action: func() {
					a.stopRunningTask()
					a.StopDebug()
				}},
			},
		},
		ui.MenuBarItem{
			ID:    "terminal",
			Label: "Терминал",
			Icon:  ic.Command,
			Items: append([]ui.MenuItem{
				{ID: "term-focus", Label: "Открыть системный терминал", Icon: ic.Command, Shortcut: "Ctrl+J", Action: func() {
					if !a.bottomVisible {
						a.ToggleBottomPanel()
					}
					if a.bottomTabs != nil {
						a.bottomTabs.Select("terminal")
					}
				}},
				{ID: "term-copy", Label: "Копировать выделенное из терминала", Shortcut: "Ctrl+C", Action: func() {
					if a.terminalOutput != nil {
						a.terminalOutput.Copy()
					}
				}},
				{ID: "term-copy-all", Label: "Копировать весь текст терминала", Action: func() {
					if a.terminalOutput != nil {
						a.terminalOutput.CopyAll()
					}
				}},
				{ID: "term-paste", Label: "Вставить команду в терминал", Shortcut: "Ctrl+V", Action: func() {
					if a.terminalOutput != nil {
						a.terminalOutput.Paste()
					}
				}},
				{ID: "term-clear", Label: "Очистить терминал (Clear)", Shortcut: "Ctrl+L", Action: func() {
					if a.terminalOutput != nil {
						a.terminalOutput.Clear()
					}
				}},
				{ID: "term-stop", Label: "Остановить процесс (SIGINT / Stop)", Icon: ic.Stop, Action: a.stopRunningTask},
				{Separator: true},
			}, shellItems...),
		},
		// Direct quick-action items integrated right into MenuBar (replacing the old topToolbar)
		ui.MenuBarItem{
			ID:     "mb-run",
			Label:  "Run",
			Icon:   ic.Run,
			Action: a.RunProject,
		},
		ui.MenuBarItem{
			ID:     "mb-build",
			Label:  "Build",
			Icon:   ic.Build,
			Action: a.BuildProject,
		},
		ui.MenuBarItem{
			ID:     "mb-test",
			Label:  "Test",
			Icon:   ic.Test,
			Action: a.TestProject,
		},
		ui.MenuBarItem{
			ID:     "mb-fmt",
			Label:  "gofmt",
			Icon:   ic.Format,
			Action: func() { a.FormatActiveDocument() },
		},
		ui.MenuBarItem{
			ID:     "mb-debug",
			Label:  "Debug",
			Icon:   ic.Debug,
			Action: func() { a.StartDebug("debug") },
		},
		ui.MenuBarItem{
			ID:    "mb-stop",
			Label: "Stop",
			Icon:  ic.Stop,
			Action: func() {
				a.stopRunningTask()
				a.StopDebug()
			},
		},
		ui.MenuBarItem{
			ID:     "mb-split",
			Label:  "Срез (1/2/3)",
			Icon:   ic.GoFile,
			Action: a.CycleEditorSplits,
		},
		ui.MenuBarItem{
			ID:     "mb-toolchain",
			Label:  "Версии Go",
			Icon:   ic.GoTools,
			Action: a.OpenToolchainManagerDialog,
		},
		ui.MenuBarItem{
			ID:     "mb-palette",
			Label:  "Команды (F1)",
			Icon:   ic.Command,
			Action: a.OpenCommandPalette,
		},
		ui.MenuBarItem{
			ID:     "mb-theme",
			Label:  "Тема",
			Icon:   ic.Theme,
			Action: a.ToggleTheme,
		},
	)
}
