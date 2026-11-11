package goide

import (
	"fmt"

	ui "github.com/banditmoscow1337/nativeForms"
)

func (a *App) activeEditorTabs() *ui.Tabs {
	if a.activeGroup >= 0 && a.activeGroup < len(a.editorGroups) && a.editorGroups[a.activeGroup] != nil {
		return a.editorGroups[a.activeGroup]
	}
	return a.editorTabs
}

func (a *App) activeEditorView(file *openDocument) *ui.CodeEditor {
	if file == nil {
		return nil
	}
	if a.activeGroup >= 0 && a.activeGroup < len(file.views) && file.views[a.activeGroup] != nil {
		return file.views[a.activeGroup]
	}
	if file.view != nil {
		return file.view
	}
	for _, v := range file.views {
		if v != nil {
			return v
		}
	}
	return nil
}

func (a *App) updateSplitBadge() {
	if a.splitBadge == nil {
		return
	}
	if a.splitCount <= 1 {
		a.splitBadge.SetText("1 срез")
	} else {
		a.splitBadge.SetText(fmt.Sprintf("Срез %d/%d", a.activeGroup+1, a.splitCount))
	}
}

// CycleEditorSplits cycles editor split mode between 1 -> 2 -> 3 -> 1 slices.
func (a *App) CycleEditorSplits() {
	next := a.splitCount + 1
	if next > 3 {
		next = 1
	}
	a.SetEditorSplits(next)
}

// SplitEditorRight splits the editor area to the right (up to 3 slices) and opens the active file there.
func (a *App) SplitEditorRight() {
	activeFile := a.active()
	path := ""
	if activeFile != nil {
		path = activeFile.path
	}
	a.SplitEditorRightWithFile(path)
}

// SplitEditorRightWithFile opens path in the adjacent right split group (expanding from 1->2 or 2->3 slices).
func (a *App) SplitEditorRightWithFile(path string) {
	if path == "" {
		if f := a.active(); f != nil {
			path = f.path
		}
	}
	targetGroup := a.activeGroup + 1
	if a.splitCount < 3 && targetGroup >= a.splitCount {
		a.SetEditorSplits(a.splitCount + 1)
	}
	if targetGroup >= 3 {
		targetGroup = 2
	}
	a.activeGroup = targetGroup
	if path != "" {
		_ = a.openFileInGroup(path, targetGroup)
	}
	a.updateSplitBadge()
	a.manager.InvalidateLayout()
}

// FocusEditorGroup focuses editor slice index (0, 1, or 2), expanding splits if needed.
func (a *App) FocusEditorGroup(group int) {
	if group < 0 || group >= 3 {
		return
	}
	if group+1 > a.splitCount {
		a.SetEditorSplits(group + 1)
	}
	a.activeGroup = group
	a.updateSplitBadge()
	if grp := a.editorGroups[group]; grp != nil {
		a.onActiveTabChanged(grp.SelectedID)
		if f := a.files[grp.SelectedID]; f != nil {
			if v := a.activeEditorView(f); v != nil {
				a.manager.Focus(v)
			}
		}
	}
	a.manager.InvalidatePaint()
}

// CloseEditorSplit closes the active secondary split group and reduces splitCount.
func (a *App) CloseEditorSplit() {
	if a.splitCount <= 1 {
		return
	}
	a.SetEditorSplits(a.splitCount - 1)
}

// SetEditorSplits configures 1, 2, or 3 side-by-side editor slices (VS Code Split Editor Right).
func (a *App) SetEditorSplits(count int) {
	count = min(max(1, count), 3)
	if a.editorMainSplit == nil || a.editorRightSplit == nil {
		a.splitCount = count
		return
	}

	// Preserve files open in collapsing split groups by migrating them to visible groups.
	if count < 3 && a.editorGroups[2] != nil {
		var ids []string
		for _, t := range a.editorGroups[2].Tabs {
			ids = append(ids, t.ID)
		}
		targetGrp := max(0, count-1)
		for _, id := range ids {
			if !a.isTabInGroup(targetGrp, id) {
				_ = a.openFileInGroup(id, targetGrp)
			}
			a.editorGroups[2].Close(id)
		}
	}
	if count < 2 && a.editorGroups[1] != nil {
		var ids []string
		for _, t := range a.editorGroups[1].Tabs {
			ids = append(ids, t.ID)
		}
		for _, id := range ids {
			if !a.isTabInGroup(0, id) {
				_ = a.openFileInGroup(id, 0)
			}
			a.editorGroups[1].Close(id)
		}
	}

	a.splitCount = count
	switch count {
	case 1:
		a.editorGroups[0].UIState().SetVisible(true)
		a.editorGroups[1].UIState().SetVisible(false)
		a.editorGroups[2].UIState().SetVisible(false)
		a.editorRightSplit.UIState().SetVisible(false)
		a.activeGroup = 0
	case 2:
		a.editorGroups[0].UIState().SetVisible(true)
		a.editorGroups[1].UIState().SetVisible(true)
		a.editorGroups[2].UIState().SetVisible(false)
		a.editorRightSplit.UIState().SetVisible(true)
		a.editorMainSplit.Ratio = 0.50
		if len(a.editorGroups[1].Tabs) == 0 {
			if seed := a.pickSeedFileForSplit(1); seed != "" {
				_ = a.openFileInGroup(seed, 1)
			}
		}
		if a.activeGroup >= 2 {
			a.activeGroup = 1
		}
	case 3:
		a.editorGroups[0].UIState().SetVisible(true)
		a.editorGroups[1].UIState().SetVisible(true)
		a.editorGroups[2].UIState().SetVisible(true)
		a.editorRightSplit.UIState().SetVisible(true)
		a.editorMainSplit.Ratio = 0.34
		a.editorRightSplit.Ratio = 0.50
		if len(a.editorGroups[1].Tabs) == 0 {
			if seed := a.pickSeedFileForSplit(1); seed != "" {
				_ = a.openFileInGroup(seed, 1)
			}
		}
		if len(a.editorGroups[2].Tabs) == 0 {
			if seed := a.pickSeedFileForSplit(2); seed != "" {
				_ = a.openFileInGroup(seed, 2)
			}
		}
	}
	a.updateSplitBadge()
	a.setStatus(fmt.Sprintf("Режим редактора: %d срез(а) рядом", count))
	a.manager.InvalidateLayout()
}

func (a *App) pickSeedFileForSplit(groupIdx int) string {
	if a.editorGroups[0] != nil {
		if len(a.editorGroups[0].Tabs) > groupIdx {
			return a.editorGroups[0].Tabs[groupIdx].ID
		}
		if a.editorGroups[0].SelectedID != "" {
			return a.editorGroups[0].SelectedID
		}
	}
	for path := range a.files {
		return path
	}
	return ""
}

func (a *App) isTabInGroup(group int, path string) bool {
	if group < 0 || group >= len(a.editorGroups) || a.editorGroups[group] == nil {
		return false
	}
	for _, t := range a.editorGroups[group].Tabs {
		if t.ID == path {
			return true
		}
	}
	return false
}

func (a *App) isFileOpenInOtherGroup(path string, excludeGroup int) bool {
	for g := 0; g < len(a.editorGroups); g++ {
		if g == excludeGroup {
			continue
		}
		if a.isTabInGroup(g, path) {
			return true
		}
	}
	return false
}

func (a *App) handleGroupTabClose(groupIdx int, path string) bool {
	file := a.files[path]
	if file == nil {
		return true
	}
	if a.isFileOpenInOtherGroup(path, groupIdx) {
		file.views[groupIdx] = nil
		for g := 0; g < len(file.views); g++ {
			if file.views[g] != nil {
				file.view = file.views[g]
				break
			}
		}
		return true
	}
	if file.dirty && !a.closingForce {
		a.setStatus("Сначала сохраните " + fileName(path))
		return false
	}
	if a.lsp != nil {
		a.lsp.closeFile(path)
	}
	for _, id := range file.shortcuts {
		a.manager.RemoveShortcut(id)
	}
	delete(a.files, path)
	a.rebuildOpenEditorsList()
	a.rebuildDiagnosticsTable()
	a.onActiveTabChanged(a.activeEditorTabs().SelectedID)
	return true
}

// SelectSidebarView switches the active VS Code sidebar panel and makes sure it is visible.
func (a *App) SelectSidebarView(id string) {
	if _, ok := a.sidebarViews[id]; !ok {
		return
	}
	a.activeSidebarID = id
	for k, view := range a.sidebarViews {
		view.UIState().SetVisible(k == id)
	}
	if !a.sidebarVisible {
		a.sidebarVisible = true
		a.sidebarContainer.UIState().SetVisible(true)
		if a.savedSidebarRatio <= 0.05 {
			a.savedSidebarRatio = 0.22
		}
		a.mainSplit.Ratio = a.savedSidebarRatio
	}
	a.updateActivityBarHighlight()
	if id == "gotools" {
		a.refreshToolsSummary()
	}
	a.manager.InvalidateLayout()
}

func (a *App) updateActivityBarHighlight() {
	for id, btn := range a.activityButtons {
		active := a.sidebarVisible && id == a.activeSidebarID
		btn.UIState().SetFrameless(!active)
	}
}

// ToggleSideBar shows or collapses the Primary Side Bar (Ctrl+B).
func (a *App) ToggleSideBar() {
	a.sidebarVisible = !a.sidebarVisible
	if !a.sidebarVisible {
		if a.mainSplit.Ratio > 0.05 {
			a.savedSidebarRatio = a.mainSplit.Ratio
		}
		a.sidebarContainer.UIState().SetVisible(false)
		a.mainSplit.Ratio = 0
	} else {
		a.sidebarContainer.UIState().SetVisible(true)
		if a.savedSidebarRatio <= 0.05 {
			a.savedSidebarRatio = 0.22
		}
		a.mainSplit.Ratio = a.savedSidebarRatio
	}
	a.updateActivityBarHighlight()
	a.manager.InvalidateLayout()
}

// ToggleBottomPanel shows or collapses the Bottom Panel (Problems / Output / Terminal — Ctrl+J).
func (a *App) ToggleBottomPanel() {
	a.bottomVisible = !a.bottomVisible
	if !a.bottomVisible {
		if a.centerSplit.Ratio < 0.95 {
			a.savedBottomRatio = a.centerSplit.Ratio
		}
		a.bottomTabs.UIState().SetVisible(false)
		a.centerSplit.Ratio = 1.0
	} else {
		a.bottomTabs.UIState().SetVisible(true)
		if a.savedBottomRatio >= 0.95 || a.savedBottomRatio <= 0.1 {
			a.savedBottomRatio = 0.70
		}
		a.centerSplit.Ratio = a.savedBottomRatio
	}
	a.manager.InvalidateLayout()
}

// ToggleTheme switches between VS Code Dark+ and VS Code Light+ themes (F2).
func (a *App) ToggleTheme() {
	a.isLight = !a.isLight
	a.applyCurrentTheme()
	if a.isLight {
		a.setStatus("Тема: VS Code Light+")
	} else {
		a.setStatus("Тема: VS Code Dark+")
	}
}

func (a *App) applyCurrentTheme() {
	pal := VSCodeDarkSyntaxPalette()
	theme := a.darkTheme
	if a.isLight {
		pal = VSCodeLightSyntaxPalette()
		theme = a.lightTheme
	}
	for _, f := range a.files {
		for _, v := range f.allViews() {
			v.Palette = pal
		}
	}
	a.manager.SetTheme(theme)
}

// ToggleFindBar shows or hides the inline Find & Replace bar (Ctrl+F).
