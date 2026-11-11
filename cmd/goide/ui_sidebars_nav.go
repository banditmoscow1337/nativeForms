package goide

import (
	"fmt"
	"os"
	"strings"

	ui "github.com/banditmoscow1337/nativeForms"
)

func (a *App) buildActivityBar() *ui.Panel {
	bar := ui.NewStack(ui.Vertical, 6)
	bar.Padding = ui.Symmetric(4, 8)
	bar.Background = a.darkTheme.Header
	bar.Border = a.darkTheme.Border
	bar.BorderWidth = 1
	bar.UIState().SetPreferredSize(ui.Vec2{X: 52, Y: 400})
	bar.UIState().SetMinimumSize(ui.Vec2{X: 52, Y: 200})
	bar.UIState().SetMaximumSize(ui.Vec2{X: 52})
	bar.UIState().SetFlexShrink(0)

	type navSpec struct {
		id      string
		tooltip string
		icon    *ui.ImageResource
	}
	specs := []navSpec{
		{id: "explorer", tooltip: "Проводник (Explorer — Ctrl+Shift+E)", icon: a.icons.Explorer},
		{id: "search", tooltip: "Поиск по проекту (Search — Ctrl+Shift+F)", icon: a.icons.Search},
		{id: "outline", tooltip: "Символы Go AST (Outline — Ctrl+Shift+O)", icon: a.icons.Outline},
		{id: "debug", tooltip: "Запуск и отладка Delve (Run & Debug — Ctrl+Shift+D)", icon: a.icons.Debug},
		{id: "gotools", tooltip: "Инструменты Go и управление версиями SDK", icon: a.icons.GoTools},
		{id: "settings", tooltip: "Настройки IDE и оформление", icon: a.icons.Settings},
	}

	a.activityButtons = make(map[string]*ui.Button, len(specs))
	for _, sp := range specs {
		spec := sp
		btn := ui.NewButton("", func() {
			if a.activeSidebarID == spec.id && a.sidebarVisible {
				a.ToggleSideBar()
			} else {
				a.SelectSidebarView(spec.id)
			}
		})
		btn.SetIcon(spec.icon)
		btn.UIState().SetMinimumSize(ui.Vec2{X: 42, Y: 40})
		btn.UIState().SetPreferredSize(ui.Vec2{X: 42, Y: 40})
		btn.UIState().SetCornerRadius(6)
		btn.UIState().SetTooltip(spec.tooltip)
		a.activityButtons[spec.id] = btn
		bar.Add(btn)
	}
	return bar
}

func (a *App) buildExplorerSidebar() ui.Component {
	root := ui.NewStack(ui.Vertical, 6)
	root.Padding = ui.All(6)

	hdr := ui.NewStack(ui.Horizontal, 6)
	hdr.Align = ui.AlignCenter
	hdrTitle := ui.NewLabel("ПРОВОДНИК: GO MODULE")
	hdrTitle.Font = ui.FontSpec{Size: 13, Weight: 700}
	hdrTitle.Ellipsis = true
	hdrTitle.UIState().SetFlex(1)

	btnNewFile := ui.NewButton("+ Файл", a.OpenNewFileDialog)
	btnNewFile.UIState().SetMinimumSize(ui.Vec2{X: 76, Y: 28})
	btnNewFile.UIState().SetPreferredSize(ui.Vec2{X: 76, Y: 28})
	btnNewFile.UIState().SetTooltip("Создать новый .go файл (Ctrl+N)")

	btnRefresh := ui.NewButton("Обн.", a.RefreshProjectTree)
	btnRefresh.UIState().SetMinimumSize(ui.Vec2{X: 60, Y: 28})
	btnRefresh.UIState().SetPreferredSize(ui.Vec2{X: 60, Y: 28})
	btnRefresh.UIState().SetTooltip("Обновить дерево файлов")
	hdr.Add(hdrTitle, btnNewFile, btnRefresh)

	filterInput := ui.NewTextField("", func(text string) {
		a.project.SetFilter(text)
		a.fileTree.ModelChanged()
	})
	filterInput.Placeholder = "Фильтр файлов (*.go, go.mod)..."

	a.openEditors = &openEditorsModel{icons: a.icons}
	a.openEditorsList = ui.NewVirtualList(a.openEditors)
	a.openEditorsList.RowHeight = 28
	a.openEditorsList.UIState().SetPreferredSize(ui.Vec2{X: 200, Y: 108})
	a.openEditorsList.OnSelect = func(path string) {
		if a.files[path] != nil {
			_ = a.openFileInGroup(path, a.activeGroup)
		}
	}
	a.openEditorsList.UIState().SetContextMenuBuilder(func(_, _ float32) []ui.MenuItem {
		sel := a.openEditorsList.SelectedID
		return []ui.MenuItem{
			{ID: "save", Label: "Сохранить файл", Disabled: sel == "", Action: func() {
				if f := a.files[sel]; f != nil {
					_ = a.save(f)
				}
			}},
			{ID: "split-right", Label: "Открыть в срезе справа (Split Right)", Disabled: sel == "", Action: func() {
				if sel != "" {
					a.SplitEditorRightWithFile(sel)
				}
			}},
			{Separator: true},
			{ID: "close", Label: "Закрыть выбранный файл", Disabled: sel == "", Action: func() {
				if sel != "" {
					a.requestCloseTab(sel)
				}
			}},
			{ID: "close-saved", Label: "Закрыть только сохранённые файлы (Ctrl+Alt+W)", Action: func() {
				a.CloseSavedTabs()
			}},
			{ID: "close-all", Label: "Закрыть все открытые файлы (Ctrl+Shift+W)", Action: a.CloseAllTabs},
		}
	})

	openEdActions := ui.NewStack(ui.Horizontal, 4)
	btnCloseSavedSide := ui.NewButton("Закрыть сохр.", func() { a.CloseSavedTabs() })
	btnCloseSavedSide.UIState().SetMinimumSize(ui.Vec2{X: 105, Y: 26})
	btnCloseSavedSide.UIState().SetFlex(1)
	btnCloseSavedSide.UIState().SetTooltip("Закрыть все сохранённые вкладки, оставив несохранённые")

	btnCloseAllSide := ui.NewButton("Закрыть все", a.CloseAllTabs)
	btnCloseAllSide.UIState().SetMinimumSize(ui.Vec2{X: 95, Y: 26})
	btnCloseAllSide.UIState().SetFlex(1)
	btnCloseAllSide.UIState().SetTooltip("Закрыть все открытые вкладки (Ctrl+Shift+W)")
	openEdActions.Add(btnCloseSavedSide, btnCloseAllSide)

	openEdBody := ui.NewStack(ui.Vertical, 4)
	openEdBody.Add(openEdActions, a.openEditorsList)
	openEdSection := ui.NewCollapsible("ОТКРЫТЫЕ РЕДАКТОРЫ", openEdBody, true, nil)
	openEdSection.HeaderHeight = 30

	a.fileTree = ui.NewVirtualTree(a.project)
	a.fileTree.RowHeight = 28
	a.fileTree.UIState().SetFlex(1)
	a.fileTree.OnSelect = func(path string) {
		info, err := os.Stat(path)
		if err == nil && !info.IsDir() {
			if err := a.openFile(path); err != nil {
				a.setStatus(err.Error())
			}
		}
	}
	a.fileTree.UIState().SetContextMenuBuilder(func(_, _ float32) []ui.MenuItem {
		sel := a.fileTree.SelectedID
		isGo := strings.HasSuffix(sel, ".go")
		return []ui.MenuItem{
			{ID: "open", Label: "Открыть в активном срезе", Icon: a.icons.GoFile, Disabled: sel == "", Action: func() {
				if sel != "" {
					_ = a.openFile(sel)
				}
			}},
			{ID: "open-split", Label: "Открыть в срезе справа (Split Right)", Icon: a.icons.GoFile, Disabled: sel == "", Action: func() {
				if sel != "" {
					a.SplitEditorRightWithFile(sel)
				}
			}},
			{ID: "new-file", Label: "Новый Go-файл в этой папке...", Icon: a.icons.GoFile, Action: a.OpenNewFileDialog},
			{Separator: true},
			{ID: "fmt", Label: "Форматировать (gofmt)", Icon: a.icons.Format, Disabled: !isGo, Action: func() {
				if isGo {
					_ = a.openFile(sel)
					a.FormatActiveDocument()
				}
			}},
			{ID: "test-pkg", Label: "Запустить тесты пакета (go test)", Icon: a.icons.Test, Action: a.TestActivePackage},
			{ID: "run-pkg", Label: "Запустить пакет (go run)", Icon: a.icons.Run, Action: a.RunProject},
			{Separator: true},
			{ID: "refresh", Label: "Обновить дерево", Icon: a.icons.Explorer, Action: a.RefreshProjectTree},
		}
	})

	root.Add(hdr, filterInput, openEdSection, a.fileTree)
	return root
}

func (a *App) buildSearchSidebar() ui.Component {
	root := ui.NewStack(ui.Vertical, 6)
	root.Padding = ui.All(6)

	hdr := ui.NewLabel("ПОИСК ПО МОДУЛЮ GO")
	hdr.Font = ui.FontSpec{Size: 13, Weight: 700}

	var caseSensitive bool
	a.searchInput = ui.NewTextField("", nil)
	a.searchInput.Placeholder = "Поиск текста или идентификатора во всех файлах..."

	runSearch := func() {
		q := strings.TrimSpace(a.searchInput.Text)
		if q == "" {
			a.searchResults.rows = nil
			a.searchResultsView.ModelChanged()
			return
		}
		a.searchResults.rows = SearchProjectText(a.project.root, q, caseSensitive, a.files)
		a.searchResultsView.ModelChanged()
		a.setStatus(fmt.Sprintf("Поиск %q: найдено совпадений: %d", q, len(a.searchResults.rows)))
	}
	a.searchInput.OnChange = func(_ string) { runSearch() }
	a.searchInput.OnSubmit = func(_ string) { runSearch() }

	caseChk := ui.NewCheckbox("Учитывать регистр (Aa)", false, func(checked bool) {
		caseSensitive = checked
		runSearch()
	})

	btnSearch := ui.NewButton("Найти во всех файлах модуля", runSearch)
	btnSearch.SetIcon(a.icons.Search)
	btnSearch.UIState().SetMinimumSize(ui.Vec2{X: 160})

	a.searchResults = &searchModel{icons: a.icons}
	a.searchResultsView = ui.NewVirtualTable(a.searchResults)
	a.searchResultsView.RowHeight = 28
	a.searchResultsView.HeaderHeight = 32
	a.searchResultsView.ColumnWidth = 110
	a.searchResultsView.UIState().SetFlex(1)
	a.searchResultsView.OnSelect = func(id string) {
		for _, m := range a.searchResults.rows {
			if m.id == id {
				a.jumpToLocation(m.path, m.line, m.column)
				return
			}
		}
	}

	root.Add(hdr, a.searchInput, caseChk, btnSearch, a.searchResultsView)
	return root
}

func (a *App) buildOutlineSidebar() ui.Component {
	root := ui.NewStack(ui.Vertical, 6)
	root.Padding = ui.All(6)

	hdr := ui.NewLabel("СИМВОЛЫ AST (OUTLINE)")
	hdr.Font = ui.FontSpec{Size: 13, Weight: 700}

	filter := ui.NewTextField("", func(text string) {
		a.outline.filter = text
		a.outline.rebuild()
		a.outlineView.ModelChanged()
	})
	filter.Placeholder = "Фильтр символов (func, struct, type)..."

	btnWorkspaceSyms := ui.NewButton("Поиск символа во всём модуле (Ctrl+T)", a.OpenWorkspaceSymbolPicker)
	btnWorkspaceSyms.SetIcon(a.icons.Outline)
	btnWorkspaceSyms.UIState().SetMinimumSize(ui.Vec2{X: 160})

	a.outline = &outlineModel{icons: a.icons}
	a.outlineView = ui.NewVirtualList(a.outline)
	a.outlineView.RowHeight = 28
	a.outlineView.UIState().SetFlex(1)
	a.outlineView.OnSelect = func(id string) {
		for _, s := range a.outline.rows {
			if s.id == id {
				a.jumpToLocation(s.path, s.line, s.column)
				return
			}
		}
	}

	root.Add(hdr, filter, btnWorkspaceSyms, a.outlineView)
	return root
}
