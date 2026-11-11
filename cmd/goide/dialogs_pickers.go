package goide

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	ui "github.com/banditmoscow1337/nativeForms"
	"github.com/banditmoscow1337/nativeForms/editor"
)

// OpenQuickFilePicker opens the VS Code Quick Open file picker (Ctrl+P).
func (a *App) OpenQuickFilePicker() {
	if a.project.root == "" {
		a.setStatus("Сначала откройте Go-модуль")
		return
	}
	var allFiles []paletteCommand
	_ = filepath.WalkDir(a.project.root, func(path string, d os.DirEntry, err error) error {
		if err != nil || len(allFiles) >= 1000 {
			return nil
		}
		if d.IsDir() {
			name := d.Name()
			if (strings.HasPrefix(name, ".") && path != a.project.root) || name == "vendor" || name == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}
		if !isProjectSourceFile(d.Name()) {
			return nil
		}
		rel := relPath(a.project.root, path)
		p := path
		allFiles = append(allFiles, paletteCommand{
			id:    p,
			label: fmt.Sprintf("%s   — %s", d.Name(), rel),
			icon:  iconForPath(a.icons, p, false),
			action: func() {
				if err := a.openFile(p); err != nil {
					a.setStatus(err.Error())
				}
			},
		})
		return nil
	})
	sort.Slice(allFiles, func(i, j int) bool { return allFiles[i].label < allFiles[j].label })

	model := &paletteListModel{items: append([]paletteCommand(nil), allFiles...)}
	list := ui.NewVirtualList(model)
	list.RowHeight = 32
	list.UIState().SetFlex(1)
	if len(model.items) > 0 {
		list.SetSelected(model.items[0].id)
	}

	var popupID ui.PopupID
	openChosen := func(id string) {
		for _, item := range model.items {
			if item.id == id {
				a.manager.ClosePopup(popupID)
				item.action()
				return
			}
		}
	}

	input := ui.NewTextField("", func(query string) {
		q := strings.ToLower(strings.TrimSpace(query))
		model.items = model.items[:0]
		for _, item := range allFiles {
			if q == "" || strings.Contains(strings.ToLower(item.label), q) {
				model.items = append(model.items, item)
			}
		}
		list.ModelChanged()
		if len(model.items) > 0 {
			list.SetSelected(model.items[0].id)
		}
	})
	input.Placeholder = "Введите имя файла для быстрого открытия (Ctrl+P)..."
	input.OnSubmit = func(_ string) {
		if list.SelectedID != "" {
			openChosen(list.SelectedID)
		}
	}
	list.OnSelect = openChosen

	body := ui.NewStack(ui.Vertical, 8)
	body.Add(input, list)
	btnOpen := ui.NewButton("Открыть файл", func() {
		if list.SelectedID != "" {
			openChosen(list.SelectedID)
		}
	})
	btnOpenSplit := ui.NewButton("Открыть в срезе справа", func() {
		if list.SelectedID != "" {
			id := list.SelectedID
			a.manager.ClosePopup(popupID)
			a.SplitEditorRightWithFile(id)
		}
	})
	btnCancel := ui.NewButton("Отмена", func() { a.manager.ClosePopup(popupID) })
	dlg := ui.NewDialog("Быстрый переход к файлу (Quick Open — Ctrl+P)", body, btnOpen, btnOpenSplit, btnCancel)
	popupID = a.manager.OpenDialog(dlg, 640, 420)
}

// OpenWorkspaceSymbolPicker opens a modal symbol search across all Go files (Ctrl+Shift+O).
func (a *App) OpenWorkspaceSymbolPicker() {
	syms := CollectWorkspaceSymbols(a.project.root, a.files)
	all := make([]paletteCommand, 0, len(syms))
	for _, s := range syms {
		sym := s
		icon := a.icons.SymVar
		switch sym.kind {
		case "func", "method":
			icon = a.icons.SymFunc
		case "type", "struct", "interface":
			icon = a.icons.SymType
		case "const":
			icon = a.icons.SymConst
		}
		all = append(all, paletteCommand{
			id:       sym.id,
			label:    fmt.Sprintf("%s  (%s)  — %s:%d", sym.name, sym.detail, fileName(sym.path), sym.line+1),
			shortcut: sym.kind,
			icon:     icon,
			action: func() {
				a.jumpToLocation(sym.path, sym.line, sym.column)
			},
		})
	}

	model := &paletteListModel{items: append([]paletteCommand(nil), all...)}
	list := ui.NewVirtualList(model)
	list.RowHeight = 32
	list.UIState().SetFlex(1)
	if len(model.items) > 0 {
		list.SetSelected(model.items[0].id)
	}

	var popupID ui.PopupID
	jumpChosen := func(id string) {
		for _, item := range model.items {
			if item.id == id {
				a.manager.ClosePopup(popupID)
				item.action()
				return
			}
		}
	}

	input := ui.NewTextField("", func(query string) {
		q := strings.ToLower(strings.TrimSpace(query))
		model.items = model.items[:0]
		for _, item := range all {
			if q == "" || strings.Contains(strings.ToLower(item.label), q) {
				model.items = append(model.items, item)
			}
		}
		list.ModelChanged()
		if len(model.items) > 0 {
			list.SetSelected(model.items[0].id)
		}
	})
	input.Placeholder = "@ Введите имя функции, метода, структуры или интерфейса..."
	input.OnSubmit = func(_ string) {
		if list.SelectedID != "" {
			jumpChosen(list.SelectedID)
		}
	}
	list.OnSelect = jumpChosen

	body := ui.NewStack(ui.Vertical, 8)
	body.Add(input, list)
	btnGo := ui.NewButton("Перейти к символу", func() {
		if list.SelectedID != "" {
			jumpChosen(list.SelectedID)
		}
	})
	btnCancel := ui.NewButton("Отмена", func() { a.manager.ClosePopup(popupID) })
	dlg := ui.NewDialog("Перейти к символу Go (Go to Symbol — Ctrl+Shift+O)", body, btnGo, btnCancel)
	popupID = a.manager.OpenDialog(dlg, 640, 420)
}

// OpenGoToLineDialog opens the VS Code Go to Line / Column dialog (Ctrl+G).
func (a *App) OpenGoToLineDialog() {
	file := a.active()
	if file == nil {
		a.setStatus("Сначала откройте файл в редакторе")
		return
	}
	view := a.activeEditorView(file)
	totalLines := file.document.LineCount()
	pos, _ := file.document.UTF16Position(view.Caret)

	var popupID ui.PopupID
	input := ui.NewTextField(fmt.Sprintf("%d:%d", pos.Line+1, pos.Character+1), nil)
	input.Placeholder = fmt.Sprintf("Введите строку[:столбец] (1..%d)", totalLines)
	input.Validate = func(next string) bool {
		for _, r := range next {
			if (r < '0' || r > '9') && r != ':' {
				return false
			}
		}
		return true
	}

	doJump := func() {
		raw := strings.TrimSpace(input.Text)
		if raw == "" {
			return
		}
		lineStr, colStr, hasCol := strings.Cut(raw, ":")
		ln, err := strconv.Atoi(lineStr)
		if err != nil || ln < 1 {
			ln = 1
		}
		col := 1
		if hasCol && colStr != "" {
			if c, err := strconv.Atoi(colStr); err == nil && c >= 1 {
				col = c
			}
		}
		a.manager.ClosePopup(popupID)
		targetLine := min(max(0, ln-1), file.document.LineCount()-1)
		view.ScrollToLine(targetLine)
		if off, err := file.document.ByteOffset(editor.Position{Line: targetLine, Character: max(0, col-1)}); err == nil {
			view.SetSelection(off, off)
		} else if start, _, err := file.document.LineRange(targetLine); err == nil {
			view.SetSelection(start, start)
		}
		a.updateCaretStatus()
	}
	input.OnSubmit = func(_ string) { doJump() }

	body := ui.NewStack(ui.Vertical, 8)
	info := ui.NewLabel(fmt.Sprintf("Файл: %s (всего строк: %d). Формат: строка или строка:столбец.", fileName(file.path), totalLines))
	info.Wrap = true
	body.Add(info, input)

	btnJump := ui.NewButton("Перейти", doJump)
	btnCancel := ui.NewButton("Отмена", func() { a.manager.ClosePopup(popupID) })
	dlg := ui.NewDialog("Перейти к строке (Go to Line — Ctrl+G)", body, btnJump, btnCancel)
	popupID = a.manager.OpenDialog(dlg, 460, 210)
}

// OpenNewFileDialog creates a new Go source or test file in the module.
func (a *App) OpenNewFileDialog() {
	if a.project.root == "" {
		a.setStatus("Сначала откройте Go-модуль")
		return
	}
	targetDir := a.project.root
	if sel := a.fileTree.SelectedID; sel != "" && within(a.project.root, sel) {
		if info, err := os.Stat(sel); err == nil {
			if info.IsDir() {
				targetDir = sel
			} else {
				targetDir = filepath.Dir(sel)
			}
		}
	}
	pkgName := fileName(targetDir)
	if targetDir == a.project.root {
		pkgName = "main"
	}

	var popupID ui.PopupID
	nameField := ui.NewTextField("handler.go", nil)
	nameField.Placeholder = "Имя файла (например: service.go или service_test.go)"

	createFn := func() {
		name := strings.TrimSpace(nameField.Text)
		if name == "" {
			return
		}
		if filepath.Ext(name) == "" {
			name += ".go"
		}
		fullPath := filepath.Join(targetDir, name)
		if !within(a.project.root, fullPath) {
			a.setStatus("Недопустимый путь файла")
			return
		}
		if _, err := os.Stat(fullPath); err == nil {
			a.manager.ClosePopup(popupID)
			_ = a.openFile(fullPath)
			return
		}
		_ = os.MkdirAll(filepath.Dir(fullPath), 0755)
		var content string
		if strings.HasSuffix(name, "_test.go") {
			content = fmt.Sprintf("package %s\n\nimport \"testing\"\n\nfunc TestExample(t *testing.T) {\n\t\n}\n", pkgName)
		} else if pkgName == "main" {
			content = "package main\n\nimport \"fmt\"\n\nfunc main() {\n\tfmt.Println(\"Hello, Go!\")\n}\n"
		} else {
			content = fmt.Sprintf("package %s\n\n", pkgName)
		}
		if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
			a.setStatus("Ошибка создания файла: " + err.Error())
			return
		}
		a.manager.ClosePopup(popupID)
		a.RefreshProjectTree()
		_ = a.openFile(fullPath)
		a.setStatus("Создан файл: " + relPath(a.project.root, fullPath))
	}
	nameField.OnSubmit = func(_ string) { createFn() }

	body := ui.NewStack(ui.Vertical, 8)
	lbl := ui.NewLabel("Директория: " + relPath(a.project.root, targetDir))
	lbl.Wrap = true
	body.Add(lbl, nameField)

	btnCreate := ui.NewButton("Создать", createFn)
	btnCancel := ui.NewButton("Отмена", func() { a.manager.ClosePopup(popupID) })
	dlg := ui.NewDialog("Новый Go-файл (New Go File)", body, btnCreate, btnCancel)
	popupID = a.manager.OpenDialog(dlg, 480, 220)
}
