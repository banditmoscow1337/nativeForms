package goide

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	ui "github.com/banditmoscow1337/nativeForms"
)

func (a *App) rebuildBreakpointsList() {
	if a.breakpointsModel == nil {
		return
	}
	a.breakpointsModel.rows = a.breakpointsModel.rows[:0]
	paths := make([]string, 0, len(a.breakpoints))
	for p := range a.breakpoints {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	for _, p := range paths {
		var lines []int
		for ln := range a.breakpoints[p] {
			lines = append(lines, ln)
		}
		sort.Ints(lines)
		for _, ln := range lines {
			a.breakpointsModel.rows = append(a.breakpointsModel.rows, breakpointRow{
				id:   fmt.Sprintf("%s:%d", p, ln),
				path: p,
				line: ln,
			})
		}
	}
	if a.breakpointsView != nil {
		a.breakpointsView.ModelChanged()
	}
}

func (a *App) clearAllDebugLines() {
	for _, f := range a.files {
		if f != nil {
			for _, v := range f.allViews() {
				v.SetDebugLine(-1)
			}
		}
	}
}

func (a *App) showFrame(row stackRow) {
	if row.path == "" || !within(a.project.root, row.path) {
		return
	}
	a.clearAllDebugLines()
	if err := a.openFile(row.path); err != nil {
		a.setStatus(err.Error())
		return
	}
	file := a.files[filepath.Clean(row.path)]
	if file == nil {
		file = a.files[row.path]
	}
	if file == nil {
		return
	}
	line := min(max(0, row.line-1), file.document.LineCount()-1)
	for _, v := range file.allViews() {
		v.SetDebugLine(line)
	}
	view := a.activeEditorView(file)
	view.CenterOnLine(line)
	start, end, err := file.document.LineRange(line)
	if err == nil {
		lineText, _ := file.document.Slice(start, end)
		firstNonWS := 0
		for firstNonWS < len(lineText) && (lineText[firstNonWS] == ' ' || lineText[firstNonWS] == '\t') {
			firstNonWS++
		}
		at := start
		if firstNonWS < len(lineText) {
			at = start + firstNonWS
		}
		view.SetSelection(at, at)
	}
	a.manager.Focus(view)
	view.ResetCaretBlink()
	a.updateCaretStatus()
}

// StartDebug launches a local Delve DAP session ("debug" or "test").
func (a *App) StartDebug(mode string) {
	if a.project.root == "" {
		a.setStatus("Сначала откройте Go-модуль")
		return
	}
	if err := a.saveAll(); err != nil {
		a.setStatus(err.Error())
		return
	}
	a.StopDebug()
	a.SelectSidebarView("debug")
	if !a.bottomVisible {
		a.ToggleBottomPanel()
	}
	if a.bottomTabs != nil {
		a.bottomTabs.Select("debug_console")
	}
	d, err := startDebug(a, mode)
	if err != nil {
		a.setStatus("dlv: " + err.Error())
		return
	}
	a.debug = d
	a.updateDebugBadge("Debug: building", true)
	a.setStatus("Запуск отладки (" + mode + ") — сборка пакета...")
}

// StopDebug terminates any active Delve DAP session and clears stack/variable tables.
func (a *App) StopDebug() {
	if a.debug != nil {
		old := a.debug
		a.debug = nil
		old.Close()
	}
	a.clearAllDebugLines()
	a.stack.rows = nil
	a.variables.rows = nil
	if a.stackView != nil {
		a.stackView.ModelChanged()
	}
	if a.variablesView != nil {
		a.variablesView.ModelChanged()
	}
	a.updateDebugBadge("Debug: idle", false)
}

func (a *App) debugControl(command string) {
	if a.debug == nil {
		a.setStatus("Отладка не запущена")
		return
	}
	a.debug.control(command)
}

// OpenDropped handles folders or Go source files dropped onto the application window.
func (a *App) OpenDropped(paths []string) {
	for _, path := range paths {
		info, err := os.Stat(path)
		if err != nil {
			continue
		}
		if info.IsDir() {
			_ = a.OpenProject(path)
			return
		}
		if a.project.root != "" && within(a.project.root, path) {
			_ = a.openFile(path)
		}
	}
}

func (a *App) setStatus(text string) {
	if a.status != nil {
		a.status.SetText(text)
	}
}

func (a *App) updateLSPBadge(text string, active bool) {
	if a.lspBadge == nil {
		return
	}
	a.lspBadge.SetText(text)
	if active {
		a.lspBadge.SetBackground(ui.RGBA(28, 128, 76, 210))
	} else {
		a.lspBadge.SetBackground(ui.RGBA(20, 24, 32, 160))
	}
}

func (a *App) updateDebugBadge(text string, active bool) {
	if a.debugBadge == nil {
		return
	}
	a.debugBadge.SetText(text)
	if active {
		a.debugBadge.SetBackground(ui.RGBA(205, 92, 28, 225))
	} else {
		a.debugBadge.SetBackground(ui.RGBA(20, 24, 32, 160))
	}
}

func (a *App) setTaskBusy(busy bool, label string) {
	if a.taskProgress != nil {
		a.taskProgress.SetText(label)
		a.taskProgress.UIState().SetVisible(busy)
	}
	if busy {
		a.setStatus("Выполняется: " + label + "...")
	}
}

func (a *App) stopRunningTask() {
	a.termMu.Lock()
	if a.termStdin != nil {
		_ = a.termStdin.Close()
		a.termStdin = nil
	}
	a.termRunning = false
	a.termMu.Unlock()
	if a.taskCancel != nil {
		a.taskCancel()
		a.taskCancel = nil
	}
	a.setTaskBusy(false, "")
}

func (a *App) appendLogf(target, format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	switch target {
	case "terminal":
		if a.terminalOutput != nil {
			a.terminalOutput.AppendText(msg)
		}
	case "debug":
		if a.debugOutput != nil {
			a.debugOutput.AppendText(msg)
		}
	default:
		if a.output != nil {
			a.output.AppendText(msg)
		}
	}
}

// Close stops background tasks, Delve DAP, and gopls LSP sessions cleanly.
func (a *App) Close() {
	a.closed = true
	a.stopRunningTask()
	a.StopDebug()
	if a.lsp != nil {
		old := a.lsp
		a.lsp = nil
		old.Close()
	}
}
