package goide

import (
	ui "github.com/banditmoscow1337/nativeForms"
)

func (a *App) buildWorkbench() {
	a.menuBar = a.buildMenuBar()
	a.activityBar = a.buildActivityBar()
	a.statusBar = a.buildStatusBar()

	// Build Sidebar views
	a.sidebarViews = map[string]ui.Component{
		"explorer": a.buildExplorerSidebar(),
		"search":   a.buildSearchSidebar(),
		"outline":  a.buildOutlineSidebar(),
		"debug":    a.buildDebugSidebar(),
		"gotools":  a.buildGoToolsSidebar(),
		"settings": a.buildSettingsSidebar(),
	}
	a.sidebarContainer = ui.NewStack(ui.Vertical, 0)
	a.sidebarContainer.Background = a.darkTheme.PanelAlt
	a.sidebarContainer.Border = a.darkTheme.Border
	a.sidebarContainer.BorderWidth = 1
	a.sidebarContainer.UIState().SetFlex(1)
	for id, view := range a.sidebarViews {
		view.UIState().SetFlex(1)
		view.UIState().SetVisible(id == "explorer")
		a.sidebarContainer.Add(view)
	}
	a.activeSidebarID = "explorer"
	a.updateActivityBarHighlight()

	// Build Editor Area (Breadcrumbs + FindBar + 1/2/3 Split Editor Groups)
	a.breadcrumbBar = ui.NewStack(ui.Horizontal, 6)
	a.breadcrumbBar.Padding = ui.Symmetric(10, 4)
	a.breadcrumbBar.Background = a.darkTheme.PanelAlt
	a.breadcrumbBar.Border = a.darkTheme.Border
	a.breadcrumbBar.BorderWidth = 1
	a.breadcrumbBar.Align = ui.AlignCenter
	a.breadcrumbBar.UIState().SetFlexShrink(0)
	a.breadcrumbBar.UIState().SetMinimumSize(ui.Vec2{Y: 28})

	a.breadcrumbLabel = ui.NewLabel("Модуль не открыт — выберите Файл › Открыть Go-модуль или нажмите Ctrl+O")
	a.breadcrumbLabel.Font = ui.FontSpec{Size: 13}
	a.breadcrumbLabel.Ellipsis = true
	a.breadcrumbLabel.UIState().SetFlex(1)

	a.activeSymbolBadge = ui.NewBadge("Go AST")
	a.activeSymbolBadge.UIState().SetTooltip("Текущий символ под курсором (F12 — перейти к определению)")

	a.splitBadge = ui.NewBadge("1 срез")
	a.splitBadge.UIState().SetTooltip("Активный режим разделения редактора (1 / 2 / 3 среза рядом)")

	btnSplitRight := ui.NewButton("◫ Разделить (1/2/3)", a.CycleEditorSplits)
	btnSplitRight.UIState().SetMinimumSize(ui.Vec2{X: 150, Y: 24})
	btnSplitRight.UIState().SetPreferredSize(ui.Vec2{X: 150, Y: 24})
	btnSplitRight.UIState().SetTooltip("Разделить среду редактирования на 2 / 3 среза рядом (VS Code Split Right)")

	btnCloseSaved := ui.NewButton("× Сохр.", func() { a.CloseSavedTabs() })
	btnCloseSaved.UIState().SetMinimumSize(ui.Vec2{X: 78, Y: 24})
	btnCloseSaved.UIState().SetPreferredSize(ui.Vec2{X: 78, Y: 24})
	btnCloseSaved.UIState().SetTooltip("Закрыть только сохранённые вкладки (Ctrl+Alt+W)")

	btnCloseAll := ui.NewButton("× Все", a.CloseAllTabs)
	btnCloseAll.UIState().SetMinimumSize(ui.Vec2{X: 68, Y: 24})
	btnCloseAll.UIState().SetPreferredSize(ui.Vec2{X: 68, Y: 24})
	btnCloseAll.UIState().SetTooltip("Закрыть все открытые вкладки (Ctrl+Shift+W)")

	btnFindToggle := ui.NewButton("Поиск (Ctrl+F)", a.ToggleFindBar)
	btnFindToggle.SetIcon(a.icons.Search)
	btnFindToggle.UIState().SetMinimumSize(ui.Vec2{X: 125, Y: 24})
	btnFindToggle.UIState().SetPreferredSize(ui.Vec2{X: 125, Y: 24})

	btnGotoDef := ui.NewButton("Опред. (F12)", a.GotoDefinition)
	btnGotoDef.SetIcon(a.icons.SymFunc)
	btnGotoDef.UIState().SetMinimumSize(ui.Vec2{X: 120, Y: 24})
	btnGotoDef.UIState().SetPreferredSize(ui.Vec2{X: 120, Y: 24})

	a.breadcrumbBar.Add(
		a.breadcrumbLabel,
		a.activeSymbolBadge,
		a.splitBadge,
		btnSplitRight,
		btnCloseSaved,
		btnCloseAll,
		btnFindToggle,
		btnGotoDef,
	)

	a.findBar = a.buildFindBar()

	for g := 0; g < 3; g++ {
		groupIdx := g
		tabs := ui.NewTabs(nil, "", func(selected string) {
			a.activeGroup = groupIdx
			a.onActiveTabChanged(selected)
		})
		tabs.UIState().SetFlex(1)
		tabs.OnFocus = func() {
			if a.activeGroup != groupIdx {
				a.activeGroup = groupIdx
				a.updateSplitBadge()
				a.onActiveTabChanged(a.editorGroups[groupIdx].SelectedID)
			}
		}
		tabs.OnTabContextMenu = func(tabPath string, _, _ float32) []ui.MenuItem {
			a.activeGroup = groupIdx
			return a.buildTabContextMenu(tabPath, groupIdx)
		}
		tabs.OnCloseRequest = func(path string) bool {
			return a.handleGroupTabClose(groupIdx, path)
		}
		a.editorGroups[g] = tabs
	}
	a.editorTabs = a.editorGroups[0]

	a.editorGroups[1].UIState().SetVisible(false)
	a.editorGroups[2].UIState().SetVisible(false)

	a.editorRightSplit = ui.NewSplitPane(ui.Horizontal, a.editorGroups[1], a.editorGroups[2])
	a.editorRightSplit.Ratio = 0.5
	a.editorRightSplit.MinFirst = 140
	a.editorRightSplit.MinSecond = 140
	a.editorRightSplit.UIState().SetFlex(1)
	a.editorRightSplit.UIState().SetVisible(false)

	a.editorMainSplit = ui.NewSplitPane(ui.Horizontal, a.editorGroups[0], a.editorRightSplit)
	a.editorMainSplit.Ratio = 0.5
	a.editorMainSplit.MinFirst = 140
	a.editorMainSplit.MinSecond = 140
	a.editorMainSplit.UIState().SetFlex(1)

	editorColumn := ui.NewStack(ui.Vertical, 0)
	editorColumn.UIState().SetFlex(1)
	editorColumn.Add(a.breadcrumbBar, a.findBar, a.editorMainSplit)

	a.bottomTabs = a.buildBottomPanel()

	a.centerSplit = ui.NewSplitPane(ui.Vertical, editorColumn, a.bottomTabs)
	a.centerSplit.Ratio = a.savedBottomRatio
	a.centerSplit.MinFirst = 140
	a.centerSplit.UIState().SetFlex(1)

	a.mainSplit = ui.NewSplitPane(ui.Horizontal, a.sidebarContainer, a.centerSplit)
	a.mainSplit.Ratio = a.savedSidebarRatio
	a.mainSplit.UIState().SetFlex(1)

	workbenchRow := ui.NewStack(ui.Horizontal, 0)
	workbenchRow.UIState().SetFlex(1)
	workbenchRow.Add(a.activityBar, a.mainSplit)

	// Topbar is removed: MenuBar directly hosts all menus and quick actions.
	root := ui.NewStack(ui.Vertical, 0)
	root.Background = a.darkTheme.Panel
	root.Add(a.menuBar, workbenchRow, a.statusBar)
	a.Root = root
}

func (a *App) buildTabContextMenu(path string, groupIdx int) []ui.MenuItem {
	hasPath := path != "" && a.files[path] != nil
	return []ui.MenuItem{
		{ID: "save", Label: "Сохранить файл", Icon: a.icons.File, Shortcut: "Ctrl+S", Disabled: !hasPath, Action: func() {
			if f := a.files[path]; f != nil {
				_ = a.save(f)
			}
		}},
		{Separator: true},
		{ID: "split-right", Label: "Открыть в срезе справа (Split Editor Right)", Icon: a.icons.GoFile, Shortcut: "Ctrl+\\", Disabled: !hasPath, Action: func() {
			a.SplitEditorRightWithFile(path)
		}},
		{ID: "splits-1", Label: "Режим: 1 срез редактора", Checked: a.splitCount == 1, Action: func() { a.SetEditorSplits(1) }},
		{ID: "splits-2", Label: "Режим: 2 среза рядом (Split 2)", Checked: a.splitCount == 2, Action: func() { a.SetEditorSplits(2) }},
		{ID: "splits-3", Label: "Режим: 3 среза рядом (Split 3)", Checked: a.splitCount == 3, Action: func() { a.SetEditorSplits(3) }},
		{Separator: true},
		{ID: "close", Label: "Закрыть вкладку", Shortcut: "Ctrl+W", Disabled: !hasPath, Action: func() {
			if hasPath {
				a.requestCloseTabInGroup(groupIdx, path)
			}
		}},
		{ID: "close-others", Label: "Закрыть другие вкладки", Disabled: !hasPath, Action: func() {
			a.CloseOtherTabs(path)
		}},
		{ID: "close-saved", Label: "Закрыть только сохранённые вкладки", Shortcut: "Ctrl+Alt+W", Action: func() {
			a.CloseSavedTabs()
		}},
		{ID: "close-all", Label: "Закрыть все открытые файлы", Shortcut: "Ctrl+Shift+W", Action: a.CloseAllTabs},
	}
}

func (a *App) registerGlobalShortcuts() {
	m := a.manager
	for _, mod := range []int{ui.ModControl, ui.ModSuper} {
		m.AddShortcut(ui.Shortcut{Key: ui.KeyS, Mods: mod, Action: func() { _ = a.SaveActive() }})
		m.AddShortcut(ui.Shortcut{Key: ui.KeyS, Mods: mod | ui.ModShift, Action: func() { _ = a.saveAll() }})
		m.AddShortcut(ui.Shortcut{Key: ui.KeyP, Mods: mod, Action: a.OpenQuickFilePicker})
		m.AddShortcut(ui.Shortcut{Key: ui.KeyP, Mods: mod | ui.ModShift, Action: a.OpenCommandPalette})
		m.AddShortcut(ui.Shortcut{Key: ui.KeyO, Mods: mod, Action: a.PromptOpenProject})
		m.AddShortcut(ui.Shortcut{Key: ui.KeyO, Mods: mod | ui.ModShift, Action: a.OpenWorkspaceSymbolPicker})
		m.AddShortcut(ui.Shortcut{Key: ui.KeyT, Mods: mod, Action: a.OpenWorkspaceSymbolPicker})
		m.AddShortcut(ui.Shortcut{Key: ui.KeyG, Mods: mod, Action: a.OpenGoToLineDialog})
		m.AddShortcut(ui.Shortcut{Key: ui.KeyF, Mods: mod, Action: a.ToggleFindBar})
		m.AddShortcut(ui.Shortcut{Key: ui.KeyB, Mods: mod, Action: a.ToggleSideBar})
		m.AddShortcut(ui.Shortcut{Key: ui.KeyJ, Mods: mod, Action: a.ToggleBottomPanel})
		m.AddShortcut(ui.Shortcut{Key: ui.KeyN, Mods: mod, Action: a.OpenNewFileDialog})
		m.AddShortcut(ui.Shortcut{Key: ui.KeyBackslash, Mods: mod, Action: a.SplitEditorRight})
		m.AddShortcut(ui.Shortcut{Key: ui.KeyDigit1, Mods: mod, Action: func() { a.FocusEditorGroup(0) }})
		m.AddShortcut(ui.Shortcut{Key: ui.KeyDigit2, Mods: mod, Action: func() { a.FocusEditorGroup(1) }})
		m.AddShortcut(ui.Shortcut{Key: ui.KeyDigit3, Mods: mod, Action: func() { a.FocusEditorGroup(2) }})
		m.AddShortcut(ui.Shortcut{Key: ui.KeyW, Mods: mod, Action: func() {
			grp := a.activeEditorTabs()
			if grp.SelectedID != "" {
				a.requestCloseTabInGroup(a.activeGroup, grp.SelectedID)
			}
		}})
		m.AddShortcut(ui.Shortcut{Key: ui.KeyW, Mods: mod | ui.ModShift, Action: a.CloseAllTabs})
		m.AddShortcut(ui.Shortcut{Key: ui.KeyW, Mods: mod | ui.ModAlt, Action: func() { a.CloseSavedTabs() }})
		m.AddShortcut(ui.Shortcut{Key: ui.KeyB, Mods: mod | ui.ModShift, Action: a.BuildProject})
		m.AddShortcut(ui.Shortcut{Key: ui.KeyT, Mods: mod | ui.ModShift, Action: a.TestProject})
		m.AddShortcut(ui.Shortcut{Key: ui.KeyE, Mods: mod | ui.ModShift, Action: func() { a.SelectSidebarView("explorer") }})
		m.AddShortcut(ui.Shortcut{Key: ui.KeyF, Mods: mod | ui.ModShift, Action: func() { a.SelectSidebarView("search") }})
		m.AddShortcut(ui.Shortcut{Key: ui.KeyD, Mods: mod | ui.ModShift, Action: func() { a.SelectSidebarView("debug") }})
	}
	m.AddShortcut(ui.Shortcut{Key: ui.KeyF1, Action: a.OpenCommandPalette})
	m.AddShortcut(ui.Shortcut{Key: ui.KeyF2, Action: a.ToggleTheme})
	m.AddShortcut(ui.Shortcut{Key: ui.KeyF5, Action: func() {
		if a.debug != nil && a.debug.threadID != 0 {
			a.debugControl("continue")
		} else {
			a.StartDebug("debug")
		}
	}})
	m.AddShortcut(ui.Shortcut{Key: ui.KeyF5, Mods: ui.ModControl, Action: a.RunProject})
	m.AddShortcut(ui.Shortcut{Key: ui.KeyF5, Mods: ui.ModShift, Action: a.StopDebug})
	m.AddShortcut(ui.Shortcut{Key: ui.KeyF8, Action: a.JumpNextDiagnostic})
	m.AddShortcut(ui.Shortcut{Key: ui.KeyF10, Action: func() { a.debugControl("next") }})
	m.AddShortcut(ui.Shortcut{Key: ui.KeyF11, Action: func() { a.debugControl("stepIn") }})
	m.AddShortcut(ui.Shortcut{Key: ui.KeyF11, Mods: ui.ModShift, Action: func() { a.debugControl("stepOut") }})
	m.AddShortcut(ui.Shortcut{Key: ui.KeyF, Mods: ui.ModShift | ui.ModAlt, Action: func() { a.FormatActiveDocument() }})
}
