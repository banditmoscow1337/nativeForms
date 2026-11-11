package goide

import (
	"fmt"
	"strings"

	ui "github.com/banditmoscow1337/nativeForms"
)

type paletteCommand struct {
	id       string
	label    string
	shortcut string
	icon     *ui.ImageResource
	action   func()
}

type paletteListModel struct {
	items []paletteCommand
}

func (m *paletteListModel) Len() int        { return len(m.items) }
func (m *paletteListModel) ID(i int) string { return m.items[i].id }
func (m *paletteListModel) Label(i int) string {
	it := m.items[i]
	if it.shortcut != "" {
		return it.label + "   [" + it.shortcut + "]"
	}
	return it.label
}
func (m *paletteListModel) Icon(i int) *ui.ImageResource {
	return m.items[i].icon
}

func (a *App) allPaletteCommands() []paletteCommand {
	ic := a.icons
	return []paletteCommand{
		{id: "go.build", label: "Go: Build Module (go build ./...)", shortcut: "Ctrl+Shift+B", icon: ic.Build, action: a.BuildProject},
		{id: "go.run", label: "Go: Run Package (go run .)", shortcut: "Ctrl+F5", icon: ic.Run, action: a.RunProject},
		{id: "go.test", label: "Go: Test Module (go test -json ./...)", shortcut: "Ctrl+Shift+T", icon: ic.Test, action: a.TestProject},
		{id: "go.test.pkg", label: "Go: Test Active Package (go test -v)", icon: ic.Test, action: a.TestActivePackage},
		{id: "go.vet", label: "Go: Vet Module (go vet ./...)", icon: ic.Vet, action: a.VetProject},
		{id: "go.fmt", label: "Go: Format Document (gofmt / go/format)", shortcut: "Shift+Alt+F", icon: ic.Format, action: func() { a.FormatActiveDocument() }},
		{id: "go.tidy", label: "Go: Tidy Module (go mod tidy)", icon: ic.GoMod, action: a.TidyModule},
		{id: "go.env", label: "Go: Show Go Environment (go env)", icon: ic.GoTools, action: a.GoEnv},
		{id: "go.toolchain.manage", label: "Go: Manage Go SDK & Tool Versions (go, gopls, dlv, goimports, staticcheck)...", icon: ic.GoTools, action: a.OpenToolchainManagerDialog},
		{id: "go.toolchain.install_all", label: "Go: Install / Update All Go Tools Automatically", icon: ic.GoTools, action: a.EnsureAllGoToolsAsync},
		{id: "go.toolchain.discover", label: "Go: Auto-Discover Installed Go SDKs and Tools", icon: ic.Search, action: func() { a.AutoDiscoverToolchain(true) }},
		{id: "view.split.right", label: "View: Split Editor Right (Разделить на 2/3 среза)", shortcut: "Ctrl+\\", icon: ic.GoFile, action: a.SplitEditorRight},
		{id: "view.split.1", label: "View: Single Editor Layout (1 срез)", shortcut: "Ctrl+1", icon: ic.GoFile, action: func() { a.SetEditorSplits(1) }},
		{id: "view.split.2", label: "View: Two Side-by-Side Editors (2 среза рядом)", shortcut: "Ctrl+2", icon: ic.GoFile, action: func() { a.SetEditorSplits(2) }},
		{id: "view.split.3", label: "View: Three Side-by-Side Editors (3 среза рядом)", shortcut: "Ctrl+3", icon: ic.GoFile, action: func() { a.SetEditorSplits(3) }},
		{id: "file.close_saved", label: "File: Close Saved Editors (Закрыть только сохранённые вкладки)", shortcut: "Ctrl+Alt+W", icon: ic.File, action: func() { a.CloseSavedTabs() }},
		{id: "file.close_all", label: "File: Close All Editors (Закрыть все открытые файлы)", shortcut: "Ctrl+Shift+W", icon: ic.File, action: a.CloseAllTabs},
		{id: "file.open_project", label: "File: Open Go Module Folder...", shortcut: "Ctrl+O", icon: ic.Folder, action: a.PromptOpenProject},
		{id: "file.quick_open", label: "File: Quick Open File in Module...", shortcut: "Ctrl+P", icon: ic.GoFile, action: a.OpenQuickFilePicker},
		{id: "file.new", label: "File: New Go Source File...", shortcut: "Ctrl+N", icon: ic.GoFile, action: a.OpenNewFileDialog},
		{id: "file.save", label: "File: Save Active File", shortcut: "Ctrl+S", icon: ic.File, action: func() {
			if err := a.SaveActive(); err != nil {
				a.setStatus(err.Error())
			}
		}},
		{id: "file.save_all", label: "File: Save All Modified Files", shortcut: "Ctrl+Shift+S", icon: ic.File, action: func() {
			if err := a.saveAll(); err != nil {
				a.setStatus(err.Error())
			}
		}},
		{id: "editor.goto_def", label: "Go: Go to Definition", shortcut: "F12", icon: ic.SymFunc, action: a.GotoDefinition},
		{id: "editor.hover", label: "Go: Show Hover Documentation at Caret", icon: ic.Info, action: a.HoverAtCaret},
		{id: "editor.complete", label: "Go: Trigger Suggest / Completion (gopls)", shortcut: "Ctrl+Space", icon: ic.SymFunc, action: a.Complete},
		{id: "editor.symbols", label: "Go: Go to Symbol in Workspace...", shortcut: "Ctrl+Shift+O", icon: ic.Outline, action: a.OpenWorkspaceSymbolPicker},
		{id: "editor.goto_line", label: "Go: Go to Line / Column...", shortcut: "Ctrl+G", icon: ic.Search, action: a.OpenGoToLineDialog},
		{id: "editor.find", label: "Edit: Find & Replace in Active File", shortcut: "Ctrl+F", icon: ic.Search, action: a.ToggleFindBar},
		{id: "debug.start", label: "Debug: Start Debugging Package (dlv dap)", shortcut: "F5", icon: ic.Debug, action: func() { a.StartDebug("debug") }},
		{id: "debug.test", label: "Debug: Debug Package Tests (dlv dap test)", icon: ic.Test, action: func() { a.StartDebug("test") }},
		{id: "debug.stop", label: "Debug: Stop Debugging Session", shortcut: "Shift+F5", icon: ic.Stop, action: a.StopDebug},
		{id: "view.sidebar", label: "View: Toggle Primary Side Bar", shortcut: "Ctrl+B", icon: ic.Explorer, action: a.ToggleSideBar},
		{id: "view.panel", label: "View: Toggle Bottom Panel (Problems / Terminal)", shortcut: "Ctrl+J", icon: ic.Command, action: a.ToggleBottomPanel},
		{id: "view.theme", label: "Preferences: Toggle Color Theme (VS Code Dark+ / Light+)", shortcut: "F2", icon: ic.Theme, action: a.ToggleTheme},
	}
}

// OpenToolchainManagerDialog opens a modal dialog for automatic discovery, downloading,
// and manual version switching of Go SDKs and Go tools (gopls, dlv, goimports, staticcheck).
func (a *App) OpenToolchainManagerDialog() {
	st := CheckGoTools()
	var popupID ui.PopupID

	body := ui.NewStack(ui.Vertical, 8)
	infoLbl := ui.NewLabel("Автоматический поиск, скачивание и ручное переключение версий Go SDK и инструментов (gopls, dlv, goimports, staticcheck).")
	infoLbl.Wrap = true

	// 1. Go SDK selector / custom version downloader
	sdkRow := ui.NewStack(ui.Horizontal, 6)
	sdkRow.Align = ui.AlignCenter
	sdkField := ui.NewTextField(nonEmptyStr(a.selectedGoSDK, nonEmptyStr(st.GoPath, "go1.24.1")), nil)
	sdkField.Placeholder = "Версия Go (например: go1.24.1, go1.23.6, go1.22.12) или путь к go..."
	sdkField.UIState().SetFlex(1)

	btnSwitchSDK := ui.NewButton("Переключить / Скачать Go", func() {
		val := strings.TrimSpace(sdkField.Text)
		if val != "" {
			a.SetGoSDK(val)
		}
	})
	btnSwitchSDK.SetIcon(a.icons.GoTools)
	sdkRow.Add(sdkField, btnSwitchSDK)

	// 2. Tool version inputs
	makeToolRow := func(toolName, defaultVer, foundPath string) *ui.Panel {
		row := ui.NewStack(ui.Horizontal, 6)
		row.Align = ui.AlignCenter
		lbl := ui.NewLabel(fmt.Sprintf("%-12s", toolName+":"))
		lbl.Font = ui.FontSpec{Family: "monospace", Size: 14, Weight: 700}
		curVer := nonEmptyStr(a.selectedToolVersions[toolName], defaultVer)
		verInput := ui.NewTextField(curVer, nil)
		verInput.Placeholder = "Версия (latest, v0.18.1...) или путь к бинарнику"
		verInput.UIState().SetFlex(1)
		btnInstall := ui.NewButton("Установить / Выбрать", func() {
			v := strings.TrimSpace(verInput.Text)
			if v == "" {
				v = "latest"
			}
			a.SetToolVersionOrPath(toolName, v)
		})
		row.Add(lbl, verInput, btnInstall)
		return row
	}

	rowGopls := makeToolRow("gopls", "latest", st.GoplsPath)
	rowDlv := makeToolRow("dlv", "latest", st.DlvPath)
	rowGoimports := makeToolRow("goimports", "latest", st.GoimportsPath)
	rowStaticcheck := makeToolRow("staticcheck", "latest", st.StaticcheckPath)

	statusBox := ui.NewLabel("")
	statusBox.Wrap = true
	if a.toolsSummaryLabel != nil {
		statusBox.SetText(a.toolsSummaryLabel.Text)
	}

	body.Add(
		infoLbl,
		ui.NewLabeledSeparator("Версия Go SDK (локальная или скачивание с go.dev/dl)"),
		sdkRow,
		ui.NewLabeledSeparator("Версии инструментов Go (go install pkg@version)"),
		rowGopls,
		rowDlv,
		rowGoimports,
		rowStaticcheck,
		ui.NewLabeledSeparator("Обнаруженное окружение"),
		statusBox,
	)

	btnInstallAll := ui.NewButton("Скачать и установить все (@latest)", func() {
		a.manager.ClosePopup(popupID)
		a.EnsureAllGoToolsAsync()
	})
	btnAutoScan := ui.NewButton("Автопоиск на диске", func() {
		a.AutoDiscoverToolchain(false)
		if a.toolsSummaryLabel != nil {
			statusBox.SetText(a.toolsSummaryLabel.Text)
		}
	})
	btnClose := ui.NewButton("Закрыть", func() {
		a.manager.ClosePopup(popupID)
	})

	dlg := ui.NewDialog("Управление версиями Go SDK и инструментов", body, btnInstallAll, btnAutoScan, btnClose)
	popupID = a.manager.OpenDialog(dlg, 680, 480)
}

// OpenCommandPalette opens the VS Code Command Palette (F1 / Ctrl+Shift+P).
func (a *App) OpenCommandPalette() {
	all := a.allPaletteCommands()
	model := &paletteListModel{items: append([]paletteCommand(nil), all...)}
	list := ui.NewVirtualList(model)
	list.RowHeight = 32
	list.UIState().SetFlex(1)
	if len(model.items) > 0 {
		list.SetSelected(model.items[0].id)
	}

	var popupID ui.PopupID
	runSelected := func(id string) {
		for _, cmd := range model.items {
			if cmd.id == id {
				a.manager.ClosePopup(popupID)
				if cmd.action != nil {
					cmd.action()
				}
				return
			}
		}
	}

	input := ui.NewTextField("", func(query string) {
		q := strings.ToLower(strings.TrimSpace(query))
		q = strings.TrimPrefix(q, ">")
		q = strings.TrimSpace(q)
		model.items = model.items[:0]
		for _, cmd := range all {
			if q == "" || strings.Contains(strings.ToLower(cmd.label), q) || strings.Contains(strings.ToLower(cmd.id), q) {
				model.items = append(model.items, cmd)
			}
		}
		list.ModelChanged()
		if len(model.items) > 0 {
			list.SetSelected(model.items[0].id)
		} else {
			list.SetSelected("")
		}
	})
	input.Placeholder = "> Введите название команды (build, split, close, версии go, определение, тема)..."
	input.OnSubmit = func(_ string) {
		if list.SelectedID != "" {
			runSelected(list.SelectedID)
		} else if len(model.items) > 0 {
			runSelected(model.items[0].id)
		}
	}

	list.OnSelect = func(id string) {
		runSelected(id)
	}

	body := ui.NewStack(ui.Vertical, 8)
	body.Add(input, list)

	btnRun := ui.NewButton("Выполнить", func() {
		if list.SelectedID != "" {
			runSelected(list.SelectedID)
		}
	})
	btnClose := ui.NewButton("Закрыть (Esc)", func() {
		a.manager.ClosePopup(popupID)
	})
	dlg := ui.NewDialog("Палитра команд (VS Code Command Palette)", body, btnRun, btnClose)
	popupID = a.manager.OpenDialog(dlg, 640, 440)
}
