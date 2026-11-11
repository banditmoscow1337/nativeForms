package goide

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	ui "github.com/banditmoscow1337/nativeForms"
	"github.com/banditmoscow1337/nativeForms/editor"
)

// Complete triggers code completion at the active editor caret via gopls.
func (a *App) Complete() {
	file := a.active()
	if file == nil || a.lsp == nil || !a.lsp.ready {
		a.setStatus("gopls ещё не готов")
		return
	}
	a.lsp.completion(file)
}

// HoverAtCaret requests symbol hover documentation from gopls or local AST.
func (a *App) HoverAtCaret() {
	file := a.active()
	if file == nil {
		return
	}
	view := a.activeEditorView(file)
	if line, err := file.document.LineAtOffset(view.Caret); err == nil {
		if diag, ok := view.DiagnosticAtLine(line); ok && diag.Message != "" {
			caret := view.CaretRect()
			a.manager.ShowTooltip(diag.Message, ui.Rect{X: caret.X, Y: caret.Y + caret.H + 4})
			a.setStatus(diag.Message)
			return
		}
	}
	if a.lsp != nil && a.lsp.ready {
		a.lsp.hover(file)
		return
	}
	sym, _, _ := view.SymbolAtCaret()
	if sym == "" {
		return
	}
	if def, ok := FindWorkspaceDefinition(a.project.root, sym, a.files); ok {
		info := fmt.Sprintf("%s %s %s (%s:%d)", def.kind, def.name, def.detail, fileName(def.path), def.line+1)
		caret := view.CaretRect()
		a.manager.ShowTooltip(info, ui.Rect{X: caret.X, Y: caret.Y + caret.H + 4})
		a.setStatus(info)
	}
}

// GotoDefinition jumps to the definition of the identifier under the caret.
func (a *App) GotoDefinition() {
	file := a.active()
	if file == nil {
		return
	}
	view := a.activeEditorView(file)
	sym, start, _ := view.SymbolAtCaret()
	if sym == "" {
		a.setStatus("Курсор не находится на идентификаторе Go")
		return
	}
	a.gotoDefinitionFrom(file, sym, start)
}

func (a *App) gotoDefinitionFrom(file *openDocument, symbol string, offset int) {
	view := a.activeEditorView(file)
	localFallback := func() {
		if def, ok := FindWorkspaceDefinition(a.project.root, symbol, a.files); ok {
			a.jumpToLocation(def.path, def.line, def.column)
			a.setStatus(fmt.Sprintf("Определение (AST): %s -> %s:%d", symbol, fileName(def.path), def.line+1))
			return
		}
		if defOff, ok := view.FindDefinitionOffset(symbol); ok {
			if ln, err := file.document.LineAtOffset(defOff); err == nil {
				view.CenterOnLine(ln)
			}
			view.SetSelection(defOff, defOff+len(symbol))
			a.manager.Focus(view)
			view.ResetCaretBlink()
			a.updateCaretStatus()
			a.setStatus(fmt.Sprintf("Определение (локально): %s", symbol))
			return
		}
		a.setStatus(fmt.Sprintf("Определение для %q не найдено", symbol))
	}
	if a.lsp != nil && a.lsp.ready {
		a.lsp.definition(file, offset, localFallback)
		return
	}
	localFallback()
}

// ToggleBreakpointAtCaret toggles a Delve breakpoint on the current caret line (F9).
func (a *App) ToggleBreakpointAtCaret() {
	file := a.active()
	if file == nil {
		return
	}
	view := a.activeEditorView(file)
	line, err := file.document.LineAtOffset(view.Caret)
	if err != nil {
		return
	}
	view.SetBreakpoint(line, !view.Breakpoint(line))
}

// JumpNextDiagnostic jumps to the next problem in the Diagnostics table (F8).
func (a *App) JumpNextDiagnostic() {
	if len(a.diagnostics.rows) == 0 {
		a.setStatus("Проблем не обнаружено")
		return
	}
	curID := a.diagnosticsView.SelectedID
	nextIdx := 0
	for i, r := range a.diagnostics.rows {
		if r.id == curID {
			nextIdx = (i + 1) % len(a.diagnostics.rows)
			break
		}
	}
	row := a.diagnostics.rows[nextIdx]
	a.diagnosticsView.SelectedID = row.id
	a.diagnosticsView.UIState().InvalidatePaint()
	a.jumpToLocation(row.path, row.line, row.column)
	a.setStatus(fmt.Sprintf("[%d/%d] %s:%d — %s", nextIdx+1, len(a.diagnostics.rows), fileName(row.path), row.line+1, row.message))
}

func (a *App) jumpToLocation(path string, line, column int) {
	if path == "" || (a.project.root != "" && !within(a.project.root, path)) {
		return
	}
	if err := a.openFile(path); err != nil {
		a.setStatus(err.Error())
		return
	}
	cleanPath := filepath.Clean(path)
	file := a.files[cleanPath]
	if file == nil {
		file = a.files[path]
	}
	if file == nil {
		return
	}
	view := a.activeEditorView(file)
	if view == nil {
		return
	}
	targetLine := min(max(0, line), file.document.LineCount()-1)
	start, end, err := file.document.LineRange(targetLine)
	if err != nil {
		return
	}
	lineText, _ := file.document.Slice(start, end)
	firstNonWS := 0
	for firstNonWS < len(lineText) && (lineText[firstNonWS] == ' ' || lineText[firstNonWS] == '\t') {
		firstNonWS++
	}

	var at int
	if off, err := file.document.ByteOffset(editor.Position{Line: targetLine, Character: max(0, column)}); err == nil {
		at = off
	} else {
		// Fallback if column was a UTF-8 byte offset or exceeded line length
		clampedByte := min(max(0, column), len(lineText))
		at = file.document.ValidCaretOffset(start + clampedByte)
	}
	// If column was 0 or landed inside leading indentation, place the cursor right
	// at the first non-whitespace token on the line so it never lands in blank margin.
	if firstNonWS < len(lineText) && (column <= 0 || at-start < firstNonWS) {
		at = start + firstNonWS
	}

	view.CenterOnLine(targetLine)
	view.SetSelection(at, at)
	a.manager.Focus(view)
	view.ResetCaretBlink()
	a.updateCaretStatus()
}

func (a *App) setDiagnostics(uri string, version *int, diagnostics []lspDiagnostic) {
	path := uriPath(uri)
	if path == "" || !within(a.project.root, path) {
		return
	}
	if file := a.files[path]; file != nil && version != nil && *version < file.lspVersion {
		return
	}
	a.diagnosticByURI[uri] = diagnostics
	a.rebuildDiagnosticsTable()
}

func (a *App) setToolDiagnostics(source string, rows []diagnosticRow) {
	if len(rows) == 0 {
		delete(a.toolDiagsBySource, source)
	} else {
		a.toolDiagsBySource[source] = rows
	}
	a.rebuildDiagnosticsTable()
}

func (a *App) rebuildDiagnosticsTable() {
	a.diagnostics.rows = nil
	errorsCount, warnsCount := 0, 0
	type fileLineKey struct {
		path string
		line int
	}
	seenLine := make(map[fileLineKey]bool)

	// 1. LSP diagnostics (keep exact ordering/fields expected by tests).
	uris := make([]string, 0, len(a.diagnosticByURI))
	for uri := range a.diagnosticByURI {
		uris = append(uris, uri)
	}
	sort.Strings(uris)
	for _, uri := range uris {
		list := a.diagnosticByURI[uri]
		path := filepath.Clean(uriPath(uri))
		for i, d := range list {
			sev := d.Severity
			if sev <= 0 {
				sev = 1
			}
			if sev == 1 {
				errorsCount++
			} else {
				warnsCount++
			}
			src := d.Source
			if src == "" {
				src = "gopls"
			}
			seenLine[fileLineKey{path: path, line: d.Range.Start.Line}] = true
			a.diagnostics.rows = append(a.diagnostics.rows, diagnosticRow{
				id:        fmt.Sprintf("%s:%d:%d:%d", path, d.Range.Start.Line, d.Range.Start.Character, i),
				path:      path,
				line:      d.Range.Start.Line,
				column:    d.Range.Start.Character,
				endLine:   d.Range.End.Line,
				endColumn: d.Range.End.Character,
				message:   strings.ReplaceAll(d.Message, "\n", " "),
				severity:  sev,
				source:    src,
			})
		}
	}

	// 2. Local AST syntax diagnostics (always included if not already reported on the same line by LSP).
	syntaxPaths := make([]string, 0, len(a.syntaxDiagsByPath))
	for p := range a.syntaxDiagsByPath {
		syntaxPaths = append(syntaxPaths, p)
	}
	sort.Strings(syntaxPaths)
	for _, path := range syntaxPaths {
		cleanP := filepath.Clean(path)
		for _, r := range a.syntaxDiagsByPath[path] {
			if seenLine[fileLineKey{path: cleanP, line: r.line}] {
				continue
			}
			seenLine[fileLineKey{path: cleanP, line: r.line}] = true
			errorsCount++
			r.path = cleanP
			a.diagnostics.rows = append(a.diagnostics.rows, r)
		}
	}

	// 3. Go CLI tool diagnostics (go build / go vet / go test).
	toolSources := make([]string, 0, len(a.toolDiagsBySource))
	for src := range a.toolDiagsBySource {
		toolSources = append(toolSources, src)
	}
	sort.Strings(toolSources)
	for _, src := range toolSources {
		for _, r := range a.toolDiagsBySource[src] {
			cleanP := filepath.Clean(r.path)
			if seenLine[fileLineKey{path: cleanP, line: r.line}] {
				continue
			}
			seenLine[fileLineKey{path: cleanP, line: r.line}] = true
			if r.severity == 1 {
				errorsCount++
			} else {
				warnsCount++
			}
			r.path = cleanP
			a.diagnostics.rows = append(a.diagnostics.rows, r)
		}
	}

	if a.diagnosticsView != nil {
		a.diagnosticsView.ModelChanged()
	}
	if a.problemsBadge != nil {
		a.problemsBadge.SetText(fmt.Sprintf("⊗ %d  ⚠ %d", errorsCount, warnsCount))
		if errorsCount > 0 {
			a.problemsBadge.SetBackground(ui.RGBA(205, 58, 58, 220))
		} else if warnsCount > 0 {
			a.problemsBadge.SetBackground(ui.RGBA(185, 130, 30, 220))
		} else {
			a.problemsBadge.SetBackground(ui.RGBA(20, 24, 32, 160))
		}
	}
	a.syncEditorAndTreeDiagnostics()
}

func (a *App) syncEditorAndTreeDiagnostics() {
	perFileDiags := make(map[string][]ui.CodeDiagnostic)
	fileErrors := make(map[string]int)
	fileWarns := make(map[string]int)

	for _, r := range a.diagnostics.rows {
		cleanP := filepath.Clean(r.path)
		sev := r.severity
		if sev <= 0 {
			sev = 1
		}
		if sev == 1 {
			fileErrors[cleanP]++
		} else {
			fileWarns[cleanP]++
		}
		perFileDiags[cleanP] = append(perFileDiags[cleanP], ui.CodeDiagnostic{
			Line:      r.line,
			Column:    r.column,
			EndLine:   r.endLine,
			EndColumn: r.endColumn,
			Severity:  sev,
			Message:   r.message,
			Source:    r.source,
		})
	}

	for path, doc := range a.files {
		if doc == nil {
			continue
		}
		cleanP := filepath.Clean(path)
		diags := perFileDiags[cleanP]
		for _, v := range doc.allViews() {
			v.SetDiagnostics(diags)
		}
		hasErr := fileErrors[cleanP] > 0
		for g := 0; g < len(a.editorGroups); g++ {
			if a.editorGroups[g] != nil {
				a.editorGroups[g].SetTabError(path, hasErr)
			}
		}
	}

	if a.project != nil {
		a.project.SetDiagnosticsCounts(fileErrors, fileWarns)
	}
	if a.fileTree != nil {
		a.fileTree.UIState().InvalidatePaint()
	}
	if a.openEditors != nil {
		for i := range a.openEditors.items {
			cleanP := filepath.Clean(a.openEditors.items[i].path)
			a.openEditors.items[i].errors = fileErrors[cleanP]
			a.openEditors.items[i].warns = fileWarns[cleanP]
		}
		if a.openEditorsList != nil {
			a.openEditorsList.UIState().InvalidatePaint()
		}
	}
}

func (a *App) rebuildOpenEditorsList() {
	if a.openEditors == nil {
		return
	}
	a.openEditors.items = a.openEditors.items[:0]
	seen := make(map[string]bool)
	for g := 0; g < len(a.editorGroups); g++ {
		if a.editorGroups[g] == nil {
			continue
		}
		for _, tab := range a.editorGroups[g].Tabs {
			if seen[tab.ID] {
				continue
			}
			if f := a.files[tab.ID]; f != nil {
				seen[tab.ID] = true
				cleanP := filepath.Clean(f.path)
				var errs, warns int
				if a.project != nil {
					errs = a.project.fileErrors[cleanP]
					warns = a.project.fileWarns[cleanP]
				}
				a.openEditors.items = append(a.openEditors.items, openEditorEntry{
					path:   f.path,
					title:  fileName(f.path),
					dirty:  f.dirty,
					errors: errs,
					warns:  warns,
				})
			}
		}
	}
	if a.openEditorsList != nil {
		a.openEditorsList.ModelChanged()
	}
}
