package goide

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	ui "github.com/banditmoscow1337/nativeForms"
)

func (a *App) ToggleFindBar() {
	if a.findBar == nil {
		return
	}
	next := !a.findBar.UIState().Visible()
	a.findBar.UIState().SetVisible(next)
	if next {
		if f := a.active(); f != nil {
			view := a.activeEditorView(f)
			if sym, _, _ := view.SymbolAtCaret(); sym != "" && a.findInput.Text == "" {
				a.findInput.SetText(sym)
			}
		}
		a.highlightFindMatches(false)
	} else if f := a.active(); f != nil {
		a.refreshActiveOutlineAndSyntax(f)
	}
	a.manager.InvalidateLayout()
}

func (a *App) highlightFindMatches(jumpFirst bool) {
	file := a.active()
	if file == nil || a.findInput == nil {
		return
	}
	q := a.findInput.Text
	if q == "" {
		a.refreshActiveOutlineAndSyntax(file)
		return
	}
	text := file.document.Text()
	var ranges []ui.StyledRange
	for offset := 0; offset+len(q) <= len(text); {
		idx := strings.Index(text[offset:], q)
		if idx < 0 {
			break
		}
		start := offset + idx
		end := start + len(q)
		ranges = append(ranges, ui.StyledRange{Start: start, End: end, Style: ui.CodeFunction})
		offset = end
		if len(ranges) >= 250 {
			break
		}
	}
	for _, v := range file.allViews() {
		_ = v.SetStyleRanges(file.document.Version(), ranges)
	}
	if jumpFirst && len(ranges) > 0 {
		view := a.activeEditorView(file)
		view.SetSelection(ranges[0].Start, ranges[0].End)
		a.updateCaretStatus()
	}
}

func (a *App) findNextInActive(dir int) {
	file := a.active()
	if file == nil || a.findInput == nil || a.findInput.Text == "" {
		return
	}
	view := a.activeEditorView(file)
	q := a.findInput.Text
	text := file.document.Text()
	var matches []int
	for offset := 0; offset+len(q) <= len(text); {
		idx := strings.Index(text[offset:], q)
		if idx < 0 {
			break
		}
		matches = append(matches, offset+idx)
		offset += idx + len(q)
	}
	if len(matches) == 0 {
		a.setStatus(fmt.Sprintf("Совпадений для %q не найдено", q))
		return
	}
	caret := view.Caret
	chosen := matches[0]
	if dir >= 0 {
		for _, m := range matches {
			if m > caret {
				chosen = m
				break
			}
		}
	} else {
		chosen = matches[len(matches)-1]
		for i := len(matches) - 1; i >= 0; i-- {
			if matches[i] < caret-len(q) {
				chosen = matches[i]
				break
			}
		}
	}
	if line, err := file.document.LineAtOffset(chosen); err == nil {
		view.CenterOnLine(line)
	}
	view.SetSelection(chosen, chosen+len(q))
	a.highlightFindMatches(false)
	a.updateCaretStatus()
}

func (a *App) replaceCurrentMatch() {
	file := a.active()
	if file == nil || a.findInput == nil || a.findInput.Text == "" {
		return
	}
	view := a.activeEditorView(file)
	q := a.findInput.Text
	repl := ""
	if a.replaceInput != nil {
		repl = a.replaceInput.Text
	}
	s, e := min(view.Anchor, view.Caret), max(view.Anchor, view.Caret)
	if cur, err := file.document.Slice(s, e); err == nil && cur == q {
		view.ReplaceTextRange(s, e, repl)
	}
	a.findNextInActive(1)
}

func (a *App) replaceAllMatches() {
	file := a.active()
	if file == nil || a.findInput == nil || a.findInput.Text == "" {
		return
	}
	view := a.activeEditorView(file)
	q := a.findInput.Text
	repl := ""
	if a.replaceInput != nil {
		repl = a.replaceInput.Text
	}
	text := file.document.Text()
	count := strings.Count(text, q)
	if count == 0 {
		return
	}
	updated := strings.ReplaceAll(text, q, repl)
	view.ReplaceTextRange(0, file.document.Len(), updated)
	a.highlightFindMatches(false)
	a.setStatus(fmt.Sprintf("Заменено вхождений: %d", count))
}

// PromptOpenProject opens the native folder dialog and loads the selected Go module.
func (a *App) PromptOpenProject() {
	if a.host == nil || !a.host.Capabilities().FileDialog {
		a.setStatus("Системный диалог выбора папки недоступен")
		return
	}
	path, err := a.host.OpenFolder()
	if err != nil {
		a.setStatus(err.Error())
		return
	}
	if path != "" {
		if err := a.OpenProject(path); err != nil {
			a.setStatus(err.Error())
		}
	}
}

// RefreshProjectTree clears cached directory listings, scans project syntax errors, and refreshes the Explorer tree.
func (a *App) RefreshProjectTree() {
	if a.project != nil {
		a.project.Refresh()
	}
	a.scanWorkspaceSyntaxDiagnostics()
	if a.fileTree != nil {
		a.fileTree.ModelChanged()
	}
	a.refreshToolsSummary()
}

// scanWorkspaceSyntaxDiagnostics checks .go files in the module root for syntax errors
// so files with errors are highlighted in red in the Explorer file tree immediately.
func (a *App) scanWorkspaceSyntaxDiagnostics() {
	if a.project == nil || a.project.root == "" {
		return
	}
	root := a.project.root
	count := 0
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || count > 250 {
			return nil
		}
		if d.IsDir() {
			name := d.Name()
			if (strings.HasPrefix(name, ".") && path != root) || name == "vendor" || name == "node_modules" || name == "bin" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(d.Name(), ".go") {
			return nil
		}
		count++
		cleanP := filepath.Clean(path)
		if openDoc := a.files[cleanP]; openDoc != nil {
			_, diags := ParseGoOutline(cleanP, openDoc.document.Text())
			if len(diags) > 0 {
				a.syntaxDiagsByPath[cleanP] = diags
			} else {
				delete(a.syntaxDiagsByPath, cleanP)
			}
			return nil
		}
		data, err := os.ReadFile(cleanP)
		if err != nil {
			return nil
		}
		_, diags := ParseGoOutline(cleanP, string(data))
		if len(diags) > 0 {
			a.syntaxDiagsByPath[cleanP] = diags
		} else {
			delete(a.syntaxDiagsByPath, cleanP)
		}
		return nil
	})
	a.rebuildDiagnosticsTable()
}

func (a *App) refreshToolsSummary() {
	if a.toolsSummaryLabel == nil {
		return
	}
	st := CheckGoTools()
	modName, goVer := parseGoModInfo(a.project.root)
	var b strings.Builder
	if modName != "" {
		fmt.Fprintf(&b, "Модуль: %s (go %s)\n", modName, nonEmptyStr(goVer, "n/a"))
	}
	activeGo := nonEmptyStr(a.selectedGoSDK, st.GoPath)
	fmt.Fprintf(&b, "Активный Go SDK: %s\n", nonEmptyStr(activeGo, "встроенный go/parser + go/format"))
	if len(st.InstalledSDKs) > 0 {
		var sdks []string
		for _, s := range st.InstalledSDKs {
			sdks = append(sdks, s.Version)
		}
		fmt.Fprintf(&b, "Найденные Go SDK: %s\n", strings.Join(sdks, ", "))
	}
	fmt.Fprintf(&b, "gofmt: %s\n", nonEmptyStr(a.selectedToolPaths["gofmt"], nonEmptyStr(st.GofmtPath, "in-process go/format")))
	fmt.Fprintf(&b, "gopls (LSP): %s (%s)\n", nonEmptyStr(a.selectedToolPaths["gopls"], nonEmptyStr(st.GoplsPath, "не найден — автоустановка")), nonEmptyStr(a.selectedToolVersions["gopls"], "latest"))
	fmt.Fprintf(&b, "dlv (DAP): %s (%s)\n", nonEmptyStr(a.selectedToolPaths["dlv"], nonEmptyStr(st.DlvPath, "не найден — автоустановка")), nonEmptyStr(a.selectedToolVersions["dlv"], "latest"))
	fmt.Fprintf(&b, "goimports: %s\n", nonEmptyStr(a.selectedToolPaths["goimports"], nonEmptyStr(st.GoimportsPath, "не установлен")))
	fmt.Fprintf(&b, "staticcheck: %s\n", nonEmptyStr(a.selectedToolPaths["staticcheck"], nonEmptyStr(st.StaticcheckPath, "не установлен")))
	fmt.Fprintf(&b, "Оболочка терминала: %s", nonEmptyStr(a.terminalShell, DetectSystemShell()))
	a.toolsSummaryLabel.SetText(b.String())
}

func (a *App) restartLSP() {
	if a.lsp != nil {
		old := a.lsp
		a.lsp = nil
		old.Close()
	}
	if a.project.root == "" {
		return
	}
	client, err := startLSP(a)
	if err != nil {
		a.output.AppendText("gopls: " + err.Error() + "\n")
		a.updateLSPBadge("gopls: AST mode", false)
		return
	}
	a.lsp = client
	a.updateLSPBadge("gopls: starting", true)
	for _, f := range a.files {
		client.open(f)
	}
}

// OpenProject opens a Go module directory containing go.mod, resets editors, and starts gopls.
func (a *App) OpenProject(path string) error {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	absolute, err = filepath.EvalSymlinks(absolute)
	if err != nil {
		return err
	}
	info, err := os.Stat(filepath.Join(absolute, "go.mod"))
	if err != nil || info.IsDir() {
		return fmt.Errorf("%s: go.mod не найден", absolute)
	}
	if err := a.saveAll(); err != nil {
		return err
	}
	a.stopRunningTask()
	a.StopDebug()
	if a.lsp != nil {
		old := a.lsp
		a.lsp = nil
		old.Close()
	}
	a.CloseAllTabsForce()
	a.files = make(map[string]*openDocument)
	a.breakpoints = make(map[string]map[int]bool)
	a.rebuildBreakpointsList()
	a.rebuildOpenEditorsList()

	a.project = newProjectTree(absolute)
	a.terminalCwd = absolute
	a.updateTerminalPromptBadge()
	a.fileTree.Model = a.project
	a.fileTree.Expanded = make(map[string]bool)
	a.fileTree.SetExpanded(absolute, true)

	a.diagnosticByURI = make(map[string][]lspDiagnostic)
	a.syntaxDiagsByPath = make(map[string][]diagnosticRow)
	a.toolDiagsBySource = make(map[string][]diagnosticRow)
	a.scanWorkspaceSyntaxDiagnostics()
	a.fileTree.ModelChanged()

	modPath, goVer := parseGoModInfo(absolute)
	branch := detectGitBranch(absolute)
	if a.branchBadge != nil {
		a.branchBadge.SetText("git: " + branch)
	}
	if a.goVersionBadge != nil && goVer != "" {
		a.goVersionBadge.SetText("Go " + goVer)
	}
	if a.breadcrumbLabel != nil {
		a.breadcrumbLabel.SetText("Модуль: " + modPath + " (" + absolute + ")")
	}
	a.setStatus("Модуль: " + absolute)
	st := a.AutoDiscoverToolchain(a.AutoInstallTools)

	client, err := startLSP(a)
	if err != nil {
		a.output.AppendText("gopls: " + err.Error() + "\n")
		a.updateLSPBadge("gopls: AST mode", false)
		if a.AutoInstallTools && st.GoPath != "" && st.GoplsPath == "" {
			a.InstallGoToolAsync("gopls", nonEmptyStr(a.selectedToolVersions["gopls"], "latest"))
		}
		return nil // Editing, AST outline, go/format, and local Go colors remain available.
	}
	a.lsp = client
	a.updateLSPBadge("gopls: starting", true)
	return nil
}
