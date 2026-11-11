package goide

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	ui "github.com/banditmoscow1337/nativeForms"
	"github.com/banditmoscow1337/nativeForms/editor"
)

func (a *App) createEditorViewForDocument(file *openDocument, group int) *ui.CodeEditor {
	view := ui.NewCodeEditor(file.document)
	view.Font = ui.FontSpec{Family: "monospace", Size: a.editorFontSize}
	view.TabWidth = a.editorTabWidth
	if a.isLight {
		view.Palette = VSCodeLightSyntaxPalette()
	} else {
		view.Palette = VSCodeDarkSyntaxPalette()
	}
	path := file.path
	for line := range a.breakpoints[path] {
		view.SetBreakpoint(line, true)
	}

	view.OnChange = func(change editor.Change) {
		a.activeGroup = group
		file.view = view
		file.dirty = true
		for g := 0; g < len(a.editorGroups); g++ {
			if a.editorGroups[g] != nil {
				a.editorGroups[g].SetDirty(path, true)
			}
		}
		lines := make(map[int]bool)
		for _, line := range view.BreakpointLines() {
			lines[line] = true
		}
		if len(lines) > 0 {
			a.breakpoints[path] = lines
		} else {
			delete(a.breakpoints, path)
		}
		for _, other := range file.views {
			if other != nil && other != view {
				other.UIState().InvalidatePaint()
			}
		}
		a.rebuildBreakpointsList()
		a.rebuildOpenEditorsList()
		if a.debug != nil && a.debug.configured {
			a.debug.setBreakpoints(path)
		}
		if a.lsp != nil {
			a.lsp.change(file, change)
		} else {
			file.lastText = file.document.Text()
		}
		a.refreshActiveOutlineAndSyntax(file)
		a.updateCaretStatus()
	}

	view.OnBreakpointChange = func(line int, enabled bool) {
		lines := a.breakpoints[path]
		if lines == nil {
			lines = make(map[int]bool)
			a.breakpoints[path] = lines
		}
		if enabled {
			lines[line] = true
		} else {
			delete(lines, line)
		}
		if len(lines) == 0 {
			delete(a.breakpoints, path)
		}
		for _, other := range file.views {
			if other != nil && other != view {
				other.SetBreakpoint(line, enabled)
			}
		}
		a.rebuildBreakpointsList()
		if a.debug != nil && a.debug.configured {
			a.debug.setBreakpoints(path)
		}
	}

	view.OnGoToDefinition = func(symbol string, offset int) {
		a.activeGroup = group
		a.gotoDefinitionFrom(file, symbol, offset)
	}

	view.ContextMenuItems = func(defaultItems []ui.MenuItem) []ui.MenuItem {
		extra := []ui.MenuItem{
			{
				ID:     "hover_doc",
				Label:  "Показать документацию (Hover)",
				Icon:   a.icons.Info,
				Action: a.HoverAtCaret,
			},
			{
				ID:       "format_doc",
				Label:    "Форматировать документ (gofmt)",
				Shortcut: "Shift+Alt+F",
				Icon:     a.icons.Format,
				Disabled: !strings.HasSuffix(path, ".go"),
				Action:   func() { a.FormatActiveDocument() },
			},
			{
				ID:       "split_right",
				Label:    "Разделить редактор вправо (Split Right)",
				Shortcut: "Ctrl+\\",
				Icon:     a.icons.GoFile,
				Action:   func() { a.SplitEditorRightWithFile(path) },
			},
			{
				ID:       "find_in_file",
				Label:    "Найти и заменить...",
				Shortcut: "Ctrl+F",
				Icon:     a.icons.Search,
				Action:   a.ToggleFindBar,
			},
			{Separator: true},
			{
				ID:     "run_pkg",
				Label:  "Запустить пакет (go run)",
				Icon:   a.icons.Run,
				Action: a.RunProject,
			},
			{
				ID:     "test_pkg",
				Label:  "Тестировать пакет (go test)",
				Icon:   a.icons.Test,
				Action: a.TestActivePackage,
			},
			{Separator: true},
			{
				ID:       "close_saved",
				Label:    "Закрыть только сохранённые файлы",
				Shortcut: "Ctrl+Alt+W",
				Action:   func() { a.CloseSavedTabs() },
			},
			{
				ID:       "close_all",
				Label:    "Закрыть все открытые файлы",
				Shortcut: "Ctrl+Shift+W",
				Action:   a.CloseAllTabs,
			},
			{Separator: true},
		}
		return append(extra, defaultItems...)
	}

	file.shortcuts = append(file.shortcuts,
		a.manager.AddShortcut(ui.Shortcut{
			Key:   ui.KeySpace,
			Mods:  ui.ModControl,
			Scope: view,
			Action: func() {
				a.activeGroup = group
				if a.editorGroups[group] != nil {
					a.editorGroups[group].Select(path)
				}
				a.Complete()
			},
		}),
	)
	return view
}

func (a *App) openFile(path string) error {
	return a.openFileInGroup(path, a.activeGroup)
}

func (a *App) openFileInGroup(path string, group int) error {
	if group < 0 || group >= len(a.editorGroups) {
		group = 0
	}
	path, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	path = filepath.Clean(path)
	if !within(a.project.root, path) {
		return fmt.Errorf("%s: файл вне модуля", path)
	}
	tabs := a.editorGroups[group]
	if tabs == nil {
		tabs = a.editorTabs
	}

	file := a.files[path]
	isNewDoc := false
	if file == nil {
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		document, err := editor.NewDocument(string(data))
		if err != nil {
			return err
		}
		file = &openDocument{
			path:     path,
			document: document,
			lastText: string(data),
		}
		a.files[path] = file
		isNewDoc = true
	}

	view := file.views[group]
	if view == nil {
		view = a.createEditorViewForDocument(file, group)
		file.views[group] = view
	}
	if file.view == nil || group == a.activeGroup {
		file.view = view
	}

	if a.isTabInGroup(group, path) {
		tabs.Select(path)
	} else {
		tabs.Open(ui.Tab{
			ID:      path,
			Title:   fileName(path),
			Icon:    iconForPath(a.icons, path, false),
			Content: view,
			Dirty:   file.dirty,
		})
	}

	a.rebuildOpenEditorsList()
	if isNewDoc && a.lsp != nil {
		a.lsp.open(file)
	}
	a.onActiveTabChanged(path)
	a.manager.Focus(view)
	view.ResetCaretBlink()
	return nil
}
