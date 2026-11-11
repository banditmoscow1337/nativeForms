package goide

import (
	"fmt"
	"strconv"

	ui "github.com/banditmoscow1337/nativeForms"
)

func (a *App) buildDebugSidebar() ui.Component {
	root := ui.NewStack(ui.Vertical, 6)
	root.Padding = ui.All(6)

	hdr := ui.NewLabel("ЗАПУСК И ОТЛАДКА (DELVE DAP)")
	hdr.Font = ui.FontSpec{Size: 13, Weight: 700}

	launchRow := ui.NewStack(ui.Horizontal, 6)
	btnDbgMain := ui.NewButton("Debug Main", func() { a.StartDebug("debug") })
	btnDbgMain.SetIcon(a.icons.Debug)
	btnDbgMain.UIState().SetMinimumSize(ui.Vec2{X: 110})
	btnDbgMain.UIState().SetFlex(1)

	btnDbgTest := ui.NewButton("Debug Tests", func() { a.StartDebug("test") })
	btnDbgTest.SetIcon(a.icons.Test)
	btnDbgTest.UIState().SetMinimumSize(ui.Vec2{X: 110})
	btnDbgTest.UIState().SetFlex(1)
	launchRow.Add(btnDbgMain, btnDbgTest)

	stepToolbar := ui.NewToolbar(
		ui.ToolbarItem{ID: "cont", Label: "Cont", Tooltip: "Continue (F5)", Icon: a.icons.Continue, Action: func() { a.debugControl("continue") }},
		ui.ToolbarItem{ID: "next", Label: "Next", Tooltip: "Step Over (F10)", Icon: a.icons.StepOver, Action: func() { a.debugControl("next") }},
		ui.ToolbarItem{ID: "in", Label: "In", Tooltip: "Step In (F11)", Icon: a.icons.StepInto, Action: func() { a.debugControl("stepIn") }},
		ui.ToolbarItem{ID: "out", Label: "Out", Tooltip: "Step Out (Shift+F11)", Icon: a.icons.StepOut, Action: func() { a.debugControl("stepOut") }},
		ui.ToolbarItem{ID: "stop", Label: "Stop", Tooltip: "Stop Delve (Shift+F5)", Icon: a.icons.Stop, Action: a.StopDebug},
	)

	a.breakpointsModel = &breakpointsModel{icons: a.icons}
	a.breakpointsView = ui.NewVirtualList(a.breakpointsModel)
	a.breakpointsView.RowHeight = 26
	a.breakpointsView.UIState().SetPreferredSize(ui.Vec2{X: 200, Y: 90})
	a.breakpointsView.OnSelect = func(id string) {
		for _, bp := range a.breakpointsModel.rows {
			if bp.id == id {
				a.jumpToLocation(bp.path, bp.line, 0)
				return
			}
		}
	}
	bpSec := ui.NewCollapsible("ТОЧКИ ОСТАНОВА (BREAKPOINTS)", a.breakpointsView, true, nil)
	bpSec.HeaderHeight = 30

	a.stackView = ui.NewVirtualTable(a.stack)
	a.stackView.RowHeight = 28
	a.stackView.HeaderHeight = 30
	a.stackView.ColumnWidth = 95
	a.stackView.UIState().SetFlex(1)
	a.stackView.OnSelect = func(id string) {
		for _, row := range a.stack.rows {
			if strconv.Itoa(row.id) == id {
				a.showFrame(row)
				if a.debug != nil {
					a.debug.fetchScopes(row.id)
				}
				return
			}
		}
	}

	a.variablesView = ui.NewVirtualTable(a.variables)
	a.variablesView.RowHeight = 28
	a.variablesView.HeaderHeight = 30
	a.variablesView.ColumnWidth = 95
	a.variablesView.UIState().SetFlex(1)

	debugTabs := ui.NewTabs([]ui.Tab{
		{ID: "stack", Title: "Call stack", Icon: a.icons.SymFunc, Content: a.stackView, NonClosable: true},
		{ID: "variables", Title: "Variables", Icon: a.icons.SymVar, Content: a.variablesView, NonClosable: true},
	}, "stack", nil)
	debugTabs.UIState().SetFlex(1)

	root.Add(hdr, launchRow, stepToolbar, bpSec, debugTabs)
	return root
}

func (a *App) buildGoToolsSidebar() ui.Component {
	stack := ui.NewStack(ui.Vertical, 8)
	stack.Padding = ui.All(8)

	hdr := ui.NewLabel("ИНСТРУМЕНТЫ GO И ВЕРСИИ SDK")
	hdr.Font = ui.FontSpec{Size: 13, Weight: 700}

	makeToolBtn := func(label, tooltip string, icon *ui.ImageResource, fn func()) *ui.Button {
		b := ui.NewButton(label, fn)
		b.SetIcon(icon)
		b.UIState().SetMinimumSize(ui.Vec2{X: 160})
		b.UIState().SetTooltip(tooltip)
		return b
	}

	btnAutoDetect := makeToolBtn("Автопоиск и установка инструментов", "Автоматически найти Go SDK и установить/обновить gopls, dlv, goimports, staticcheck", a.icons.GoTools, func() {
		a.AutoDiscoverToolchain(true)
		a.EnsureAllGoToolsAsync()
	})
	btnToolchainMgr := makeToolBtn("Менеджер версий Go и инструментов...", "Открыть диалог выбора и скачивания версий Go, gopls, dlv, goimports, staticcheck", a.icons.Settings, a.OpenToolchainManagerDialog)

	st := CheckGoTools()
	sdkOptions := []ui.RadioOption{
		{ID: "go1.24.1", Label: "Go SDK: go1.24.1 (авто-скачивание / локальный)"},
		{ID: "go1.23.6", Label: "Go SDK: go1.23.6 (авто-скачивание / локальный)"},
		{ID: "go1.22.12", Label: "Go SDK: go1.22.12 (авто-скачивание / локальный)"},
	}
	selectedSDKID := "go1.24.1"
	for _, sdk := range st.InstalledSDKs {
		sdkOptions = append([]ui.RadioOption{{
			ID:    sdk.GoBin,
			Label: fmt.Sprintf("Go %s (%s)", sdk.Version, sdk.GoBin),
		}}, sdkOptions...)
		if selectedSDKID == "go1.24.1" {
			selectedSDKID = sdk.GoBin
		}
	}
	a.sdkCombo = ui.NewComboBox(sdkOptions, selectedSDKID, func(id string) {
		a.SetGoSDK(id)
	})

	a.goplsVerCombo = ui.NewComboBox([]ui.RadioOption{
		{ID: "latest", Label: "gopls: @latest (рекомендуется)"},
		{ID: "v0.18.1", Label: "gopls: @v0.18.1"},
		{ID: "v0.17.1", Label: "gopls: @v0.17.1"},
		{ID: "v0.16.2", Label: "gopls: @v0.16.2"},
	}, "latest", func(ver string) {
		a.SetToolVersionOrPath("gopls", ver)
	})

	a.dlvVerCombo = ui.NewComboBox([]ui.RadioOption{
		{ID: "latest", Label: "dlv (Delve): @latest (рекомендуется)"},
		{ID: "v1.24.1", Label: "dlv (Delve): @v1.24.1"},
		{ID: "v1.23.1", Label: "dlv (Delve): @v1.23.1"},
	}, "latest", func(ver string) {
		a.SetToolVersionOrPath("dlv", ver)
	})

	autoInstallChk := ui.NewCheckbox("Автоустановка недостающих инструментов Go", a.AutoInstallTools, func(v bool) {
		a.AutoInstallTools = v
	})

	btnBuild := makeToolBtn("go build ./... (Сборка)", "Компилировать все пакеты модуля и проверить ошибки", a.icons.Build, a.BuildProject)
	btnRun := makeToolBtn("go run . (Запуск)", "Запустить пакет в рабочей директории", a.icons.Run, a.RunProject)
	btnTestAll := makeToolBtn("go test -json ./... (Все тесты)", "Запустить все тесты с разбором результатов в таблице Test Results", a.icons.Test, a.TestProject)
	btnTestPkg := makeToolBtn("go test -v (Текущий пакет)", "Запустить тесты пакета активного файла", a.icons.Test, a.TestActivePackage)
	btnVet := makeToolBtn("go vet ./... (Статический анализ)", "Запустить анализатор go vet и вывести замечания в Problems", a.icons.Vet, a.VetProject)
	btnFmt := makeToolBtn("gofmt / go/format (Форматировать)", "Отформатировать текущий Go-файл по стандарту gofmt", a.icons.Format, func() { a.FormatActiveDocument() })
	btnTidy := makeToolBtn("go mod tidy (Зависимости)", "Очистить и синхронизировать go.mod / go.sum", a.icons.GoMod, a.TidyModule)
	btnEnv := makeToolBtn("go env (Окружение Go)", "Показать переменные окружения Go в терминале", a.icons.Info, a.GoEnv)

	a.toolsSummaryLabel = ui.NewLabel("")
	a.toolsSummaryLabel.Wrap = true
	a.refreshToolsSummary()

	docLink := ui.NewHyperlink("Официальные релизы Go SDK (go.dev/dl)", "https://go.dev/dl/", func(url string) {
		a.manager.Notify("Ссылка: " + url)
		a.setStatus("Открыта ссылка: " + url)
	})

	stack.Add(
		hdr,
		btnAutoDetect,
		btnToolchainMgr,
		ui.NewLabeledSeparator("Переключение версий Go и инструментов"),
		a.sdkCombo,
		a.goplsVerCombo,
		a.dlvVerCombo,
		autoInstallChk,
		ui.NewLabeledSeparator("Команды модуля"),
		btnBuild,
		btnRun,
		btnTestAll,
		btnTestPkg,
		btnVet,
		btnFmt,
		btnTidy,
		btnEnv,
		ui.NewLabeledSeparator("Статус окружения"),
		a.toolsSummaryLabel,
		docLink,
	)
	scroll := ui.NewScrollPanel(stack)
	scroll.UIState().SetFlex(1)
	return scroll
}

func (a *App) buildSettingsSidebar() ui.Component {
	stack := ui.NewStack(ui.Vertical, 8)
	stack.Padding = ui.All(8)

	hdr := ui.NewLabel("НАСТРОЙКИ IDE И ОФОРМЛЕНИЕ")
	hdr.Font = ui.FontSpec{Size: 13, Weight: 700}

	themeSeg := ui.NewSegmentedControl([]ui.RadioOption{
		{ID: "dark", Label: "Dark+", Icon: a.icons.Theme},
		{ID: "light", Label: "Light+", Icon: a.icons.Theme},
	}, "dark", func(id string) {
		wantLight := id == "light"
		if a.isLight != wantLight {
			a.ToggleTheme()
		}
	})

	splitSeg := ui.NewSegmentedControl([]ui.RadioOption{
		{ID: "1", Label: "1 срез"},
		{ID: "2", Label: "2 среза"},
		{ID: "3", Label: "3 среза"},
	}, "1", func(id string) {
		if n, err := strconv.Atoi(id); err == nil {
			a.SetEditorSplits(n)
		}
	})

	accentPicker := ui.NewCompactColorPicker("Акцент VS Code", a.darkTheme.Accent, func(c ui.Color) {
		hover := ui.Color{
			R: min(1, c.R+0.1),
			G: min(1, c.G+0.1),
			B: min(1, c.B+0.1),
			A: 1,
		}
		a.darkTheme.Accent = c
		a.darkTheme.AccentHover = hover
		a.lightTheme.Accent = c
		a.lightTheme.AccentHover = hover
		if a.statusBar != nil {
			a.statusBar.Background = c
		}
		a.applyCurrentTheme()
	})
	accentPicker.UIState().SetMinimumSize(ui.Vec2{X: 160})

	fontSpin := ui.NewSpinBox("Шрифт кода:", 12, 26, a.editorFontSize, 1, func(val float32) {
		a.editorFontSize = val
		for _, f := range a.files {
			for _, v := range f.allViews() {
				v.Font.Size = val
			}
		}
		a.manager.InvalidateLayout()
	})
	fontSpin.Suffix = " px"
	fontSpin.UIState().SetMinimumSize(ui.Vec2{X: 160})

	tabSpin := ui.NewSpinBox("Ширина Tab:", 2, 8, float32(a.editorTabWidth), 1, func(val float32) {
		a.editorTabWidth = int(val)
		for _, f := range a.files {
			for _, v := range f.allViews() {
				v.TabWidth = a.editorTabWidth
			}
		}
		a.manager.InvalidatePaint()
	})
	tabSpin.Suffix = " sp"
	tabSpin.UIState().SetMinimumSize(ui.Vec2{X: 160})

	scaleCombo := ui.NewComboBox([]ui.RadioOption{
		{ID: "1.00", Label: "Масштаб DPI: 100% (1.0x)"},
		{ID: "1.25", Label: "Масштаб DPI: 125% (1.25x)"},
		{ID: "1.50", Label: "Масштаб DPI: 150% (1.5x)"},
		{ID: "2.00", Label: "Масштаб DPI: 200% (2.0x)"},
	}, "1.00", func(id string) {
		if v, err := strconv.ParseFloat(id, 32); err == nil {
			a.manager.SetDisplayScale(float32(v))
		}
	})

	radiusSlider := ui.NewSlider("Скругление углов (CornerRadius)", 0, 12, 4, 1, func(val float32) {
		a.darkTheme.CornerRadius = val
		a.lightTheme.CornerRadius = val
		a.applyCurrentTheme()
	})
	radiusSlider.UIState().SetMinimumSize(ui.Vec2{X: 160})

	fmtOnSaveChk := ui.NewCheckbox("Форматировать Go-код при сохранении (Format on Save)", a.FormatOnSave, func(v bool) {
		a.FormatOnSave = v
	})

	stack.Add(
		hdr,
		themeSeg,
		ui.NewLabeledSeparator("Разделение среды редактирования"),
		splitSeg,
		accentPicker,
		fontSpin,
		tabSpin,
		scaleCombo,
		radiusSlider,
		fmtOnSaveChk,
	)
	scroll := ui.NewScrollPanel(stack)
	scroll.UIState().SetFlex(1)
	return scroll
}
