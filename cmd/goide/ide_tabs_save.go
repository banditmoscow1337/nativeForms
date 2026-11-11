package goide

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	ui "github.com/banditmoscow1337/nativeForms"
)

func (a *App) requestCloseTab(path string) {
	a.requestCloseTabInGroup(a.activeGroup, path)
}

func (a *App) requestCloseTabInGroup(group int, path string) {
	if group < 0 || group >= len(a.editorGroups) || a.editorGroups[group] == nil {
		group = 0
	}
	file := a.files[path]
	if file == nil || !file.dirty || a.isFileOpenInOtherGroup(path, group) {
		a.editorGroups[group].Close(path)
		return
	}
	var popupID ui.PopupID
	lbl := ui.NewLabel(fmt.Sprintf("Файл %q содержит несохранённые изменения. Сохранить перед закрытием?", fileName(path)))
	lbl.Wrap = true
	btnSave := ui.NewButton("Сохранить и закрыть", func() {
		a.manager.ClosePopup(popupID)
		if err := a.save(file); err == nil {
			a.editorGroups[group].Close(path)
		} else {
			a.setStatus(err.Error())
		}
	})
	btnDiscard := ui.NewButton("Не сохранять", func() {
		a.manager.ClosePopup(popupID)
		file.dirty = false
		for g := 0; g < len(a.editorGroups); g++ {
			if a.editorGroups[g] != nil {
				a.editorGroups[g].SetDirty(path, false)
			}
		}
		a.editorGroups[group].Close(path)
	})
	btnCancel := ui.NewButton("Отмена", func() {
		a.manager.ClosePopup(popupID)
	})
	dlg := ui.NewDialog("Несохранённые изменения", lbl, btnSave, btnDiscard, btnCancel)
	popupID = a.manager.OpenDialog(dlg, 520, 210)
}

// CloseSavedTabs closes all open tabs that have no unsaved modifications,
// keeping dirty tabs open across all editor split groups.
func (a *App) CloseSavedTabs() int {
	var savedPaths []string
	for path, file := range a.files {
		if file != nil && !file.dirty {
			savedPaths = append(savedPaths, path)
		}
	}
	sort.Strings(savedPaths)
	closed := 0
	for _, path := range savedPaths {
		closedAny := false
		for g := 0; g < len(a.editorGroups); g++ {
			if a.editorGroups[g] != nil && a.isTabInGroup(g, path) {
				if a.editorGroups[g].Close(path) {
					closedAny = true
				}
			}
		}
		if closedAny {
			closed++
		}
	}
	a.rebuildOpenEditorsList()
	if closed > 0 {
		a.setStatus(fmt.Sprintf("Закрыто сохранённых вкладок: %d", closed))
	} else {
		a.setStatus("Нет сохранённых вкладок для закрытия")
	}
	return closed
}

// CloseOtherTabs closes all saved tabs except keepPath.
func (a *App) CloseOtherTabs(keepPath string) int {
	var paths []string
	for path, file := range a.files {
		if path != keepPath && file != nil && !file.dirty {
			paths = append(paths, path)
		}
	}
	sort.Strings(paths)
	closed := 0
	for _, path := range paths {
		for g := 0; g < len(a.editorGroups); g++ {
			if a.editorGroups[g] != nil && a.isTabInGroup(g, path) {
				if a.editorGroups[g].Close(path) {
					closed++
				}
			}
		}
	}
	a.rebuildOpenEditorsList()
	return closed
}

// CloseAllTabsForce closes all open files across all editor split groups immediately,
// discarding any unsaved changes.
func (a *App) CloseAllTabsForce() int {
	a.closingForce = true
	defer func() { a.closingForce = false }()

	var paths []string
	for path := range a.files {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	for _, path := range paths {
		if f := a.files[path]; f != nil {
			f.dirty = false
		}
		for g := 0; g < len(a.editorGroups); g++ {
			if a.editorGroups[g] != nil {
				a.editorGroups[g].SetDirty(path, false)
				a.editorGroups[g].Close(path)
			}
		}
	}
	a.rebuildOpenEditorsList()
	a.setStatus("Все открытые файлы закрыты")
	return len(paths)
}

// CloseAllTabs closes all open files; if any file has unsaved changes, it prompts
// whether to save all, discard changes, or close only saved files.
func (a *App) CloseAllTabs() {
	if len(a.files) == 0 {
		a.setStatus("Нет открытых файлов")
		return
	}
	if !a.HasDirtyFiles() {
		a.CloseAllTabsForce()
		return
	}
	var dirtyNames []string
	for _, f := range a.files {
		if f != nil && f.dirty {
			dirtyNames = append(dirtyNames, fileName(f.path))
		}
	}
	sort.Strings(dirtyNames)

	var popupID ui.PopupID
	lbl := ui.NewLabel(fmt.Sprintf("Есть несохранённые файлы (%s). Как закрыть открытые вкладки?", strings.Join(dirtyNames, ", ")))
	lbl.Wrap = true

	btnSaveAndClose := ui.NewButton("Сохранить все и закрыть", func() {
		a.manager.ClosePopup(popupID)
		if err := a.saveAll(); err != nil {
			a.setStatus(err.Error())
			return
		}
		a.CloseAllTabsForce()
	})
	btnDiscardAll := ui.NewButton("Закрыть без сохранения", func() {
		a.manager.ClosePopup(popupID)
		a.CloseAllTabsForce()
	})
	btnCloseSavedOnly := ui.NewButton("Только сохранённые", func() {
		a.manager.ClosePopup(popupID)
		a.CloseSavedTabs()
	})
	btnCancel := ui.NewButton("Отмена", func() {
		a.manager.ClosePopup(popupID)
	})
	dlg := ui.NewDialog("Закрытие всех открытых файлов", lbl, btnSaveAndClose, btnDiscardAll, btnCloseSavedOnly, btnCancel)
	popupID = a.manager.OpenDialog(dlg, 640, 220)
}

func (a *App) onActiveTabChanged(path string) {
	file := a.files[path]
	a.updateSplitBadge()
	if file == nil {
		if a.breadcrumbLabel != nil {
			if a.project.root != "" {
				a.breadcrumbLabel.SetText("Модуль: " + a.project.name() + " (" + a.project.root + ")")
			} else {
				a.breadcrumbLabel.SetText("Модуль не открыт")
			}
		}
		if a.activeSymbolBadge != nil {
			a.activeSymbolBadge.SetText("Go AST")
		}
		if a.outline != nil {
			a.outline.all = nil
			a.outline.rebuild()
			if a.outlineView != nil {
				a.outlineView.ModelChanged()
			}
		}
		return
	}
	if v := file.views[a.activeGroup]; v != nil {
		file.view = v
	}
	if a.openEditorsList != nil && a.openEditorsList.SelectedID != path {
		a.openEditorsList.SetSelected(path)
	}
	if a.fileTree != nil && a.fileTree.SelectedID != path {
		a.fileTree.SetSelected(path)
	}
	a.refreshActiveOutlineAndSyntax(file)
	a.updateCaretStatus()
}

func (a *App) refreshActiveOutlineAndSyntax(file *openDocument) {
	if file == nil {
		return
	}
	if !strings.HasSuffix(file.path, ".go") {
		if a.outline != nil {
			a.outline.all = nil
			a.outline.rebuild()
			if a.outlineView != nil {
				a.outlineView.ModelChanged()
			}
		}
		return
	}
	src := file.document.Text()
	syms, synDiags := ParseGoOutline(file.path, src)
	if a.outline != nil && a.active() == file {
		a.outline.all = syms
		a.outline.rebuild()
		if a.outlineView != nil {
			a.outlineView.ModelChanged()
		}
	}
	// Apply AST-derived syntax style ranges when the Find bar is not overriding styles
	if a.findBar == nil || !a.findBar.UIState().Visible() || a.findInput == nil || a.findInput.Text == "" {
		astStyles := ExtractGoASTStyleRanges(src)
		ver := file.document.Version()
		for _, v := range file.allViews() {
			_ = v.SetStyleRanges(ver, astStyles)
		}
	}
	cleanPath := filepath.Clean(file.path)
	if len(synDiags) > 0 {
		a.syntaxDiagsByPath[cleanPath] = synDiags
	} else {
		delete(a.syntaxDiagsByPath, cleanPath)
	}
	a.rebuildDiagnosticsTable()
}

func (a *App) updateCaretStatus() {
	file := a.active()
	if file == nil {
		return
	}
	view := a.activeEditorView(file)
	pos, err := file.document.UTF16Position(view.Caret)
	if err == nil && a.caretLabel != nil {
		a.caretLabel.SetText(fmt.Sprintf("Ln %d, Col %d", pos.Line+1, pos.Character+1))
	}
	sym, _, _ := view.SymbolAtCaret()
	if a.activeSymbolBadge != nil {
		if sym != "" {
			a.activeSymbolBadge.SetText("sym: " + sym)
		} else {
			a.activeSymbolBadge.SetText("Go AST")
		}
	}
	if a.breadcrumbLabel != nil {
		rel := strings.ReplaceAll(relPath(a.project.root, file.path), "/", " › ")
		if a.project.root != "" {
			rel = a.project.name() + " › " + rel
		}
		if sym != "" {
			rel += " › " + sym
		}
		a.breadcrumbLabel.SetText(rel)
	}
}

func (a *App) active() *openDocument {
	grp := a.activeEditorTabs()
	if grp != nil && grp.SelectedID != "" {
		if f := a.files[grp.SelectedID]; f != nil {
			return f
		}
	}
	if a.editorTabs != nil && a.editorTabs.SelectedID != "" {
		return a.files[a.editorTabs.SelectedID]
	}
	return nil
}

// HasDirtyFiles reports whether any open document has unsaved edits.
func (a *App) HasDirtyFiles() bool {
	for _, f := range a.files {
		if f.dirty {
			return true
		}
	}
	return false
}

// SaveActive saves the currently selected editor document to disk.
func (a *App) SaveActive() error {
	if file := a.active(); file != nil {
		return a.save(file)
	}
	return nil
}

func (a *App) save(file *openDocument) error {
	if !file.dirty {
		return nil
	}
	view := a.activeEditorView(file)
	if a.FormatOnSave && strings.HasSuffix(file.path, ".go") {
		if formatted, err := FormatGoSource(file.document.Text()); err == nil && formatted != file.document.Text() {
			_ = view.ReplaceTextRange(0, file.document.Len(), formatted)
		}
	}
	if err := os.WriteFile(file.path, []byte(file.document.Text()), 0644); err != nil {
		return err
	}
	file.dirty = false
	for g := 0; g < len(a.editorGroups); g++ {
		if a.editorGroups[g] != nil {
			a.editorGroups[g].SetDirty(file.path, false)
		}
	}
	a.rebuildOpenEditorsList()
	if a.lsp != nil {
		a.lsp.didSave(file)
	}
	a.setStatus("Сохранено: " + fileName(file.path))
	return nil
}

func (a *App) saveAll() error {
	for _, file := range a.files {
		if err := a.save(file); err != nil {
			return err
		}
	}
	return nil
}
