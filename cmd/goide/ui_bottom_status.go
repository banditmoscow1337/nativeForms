package goide

import (
	"fmt"
	"path/filepath"
	"runtime"
	"time"

	ui "github.com/banditmoscow1337/nativeForms"
)

func (a *App) buildFindBar() *ui.Panel {
	bar := ui.NewStack(ui.Horizontal, 6)
	bar.Padding = ui.Symmetric(8, 4)
	bar.Background = a.darkTheme.PanelAlt
	bar.Border = a.darkTheme.Border
	bar.BorderWidth = 1
	bar.Align = ui.AlignCenter
	bar.UIState().SetFlexShrink(0)
	bar.UIState().SetVisible(false)

	a.findInput = ui.NewTextField("", func(_ string) {
		a.highlightFindMatches(false)
	})
	a.findInput.Placeholder = "Найти в файле (Ctrl+F)..."
	a.findInput.UIState().SetFlex(1)
	a.findInput.OnSubmit = func(_ string) {
		a.findNextInActive(1)
	}

	a.replaceInput = ui.NewTextField("", nil)
	a.replaceInput.Placeholder = "Заменить на..."
	a.replaceInput.UIState().SetFlex(1)

	btnNext := ui.NewButton("Далее", func() { a.findNextInActive(1) })
	btnNext.UIState().SetMinimumSize(ui.Vec2{X: 74, Y: 28})
	btnNext.UIState().SetPreferredSize(ui.Vec2{X: 74, Y: 28})

	btnPrev := ui.NewButton("Назад", func() { a.findNextInActive(-1) })
	btnPrev.UIState().SetMinimumSize(ui.Vec2{X: 74, Y: 28})
	btnPrev.UIState().SetPreferredSize(ui.Vec2{X: 74, Y: 28})

	btnReplace := ui.NewButton("Заменить", a.replaceCurrentMatch)
	btnReplace.UIState().SetMinimumSize(ui.Vec2{X: 92, Y: 28})
	btnReplace.UIState().SetPreferredSize(ui.Vec2{X: 92, Y: 28})

	btnReplaceAll := ui.NewButton("Все", a.replaceAllMatches)
	btnReplaceAll.UIState().SetMinimumSize(ui.Vec2{X: 64, Y: 28})
	btnReplaceAll.UIState().SetPreferredSize(ui.Vec2{X: 64, Y: 28})

	btnClose := ui.NewButton("×", a.ToggleFindBar)
	btnClose.UIState().SetMinimumSize(ui.Vec2{X: 38, Y: 28})
	btnClose.UIState().SetPreferredSize(ui.Vec2{X: 38, Y: 28})

	bar.Add(a.findInput, a.replaceInput, btnPrev, btnNext, btnReplace, btnReplaceAll, btnClose)
	return bar
}

func (a *App) buildBottomPanel() *ui.Tabs {
	ic := a.icons

	// 1. Problems tab
	a.diagnosticsView = ui.NewVirtualTable(a.diagnostics)
	a.diagnosticsView.RowHeight = 28
	a.diagnosticsView.HeaderHeight = 32
	a.diagnosticsView.ColumnWidth = 175
	a.diagnosticsView.UIState().SetFlex(1)
	a.diagnosticsView.OnSelect = func(id string) {
		for _, row := range a.diagnostics.rows {
			if row.id != id {
				continue
			}
			a.jumpToLocation(row.path, row.line, row.column)
			a.setStatus(fmt.Sprintf("%s:%d:%d — %s", fileName(row.path), row.line+1, row.column+1, row.message))
			return
		}
	}
	a.diagnosticsView.UIState().SetContextMenuBuilder(func(_, _ float32) []ui.MenuItem {
		sel := a.diagnosticsView.SelectedID
		return []ui.MenuItem{
			{ID: "jump", Label: "Перейти к строке с ошибкой", Disabled: sel == "", Action: func() {
				if sel != "" && a.diagnosticsView.OnSelect != nil {
					a.diagnosticsView.OnSelect(sel)
				}
			}},
			{ID: "vet", Label: "Запустить go vet ./...", Icon: ic.Vet, Action: a.VetProject},
			{ID: "build", Label: "Запустить go build ./...", Icon: ic.Build, Action: a.BuildProject},
		}
	})

	// 2. Output tab — continuous selectable monospace text stream
	a.output = ui.NewOutputPanel()
	a.output.Font = ui.FontSpec{Family: "monospace", Size: 14}
	a.output.UIState().SetFlex(1)

	// 3. System Shell Terminal tab — continuous sequential terminal buffer with
	// direct inline prompt typing, mouse text selection, Ctrl+C copy, and Ctrl+V paste.
	defaultSh := nonEmptyStr(a.terminalShell, DetectSystemShell())
	a.terminalOutput = ui.NewOutputPanel()
	a.terminalOutput.Font = ui.FontSpec{Family: "monospace", Size: 14}
	a.terminalOutput.TerminalMode = true
	a.terminalOutput.UIState().SetFlex(1)
	a.terminalOutput.AppendText(fmt.Sprintf("Системный терминал (%s) — вводите команды прямо после приглашения `$` (выделение мышью, Ctrl+C копирование, Ctrl+V вставка)\n", defaultSh))
	a.terminalOutput.OnCommand = func(cmd string) {
		a.RunTerminalCommand(cmd)
	}
	a.terminalOutput.OnInterrupt = func() {
		a.stopRunningTask()
		a.appendLogf("terminal", "^C\n")
	}
	a.updateTerminalPromptBadge()

	// Keep a.terminalInput initialized for programmatic compatibility
	a.terminalInput = ui.NewTextField("", nil)
	a.terminalInput.OnSubmit = func(cmd string) {
		a.RunTerminalCommand(cmd)
	}

	// 4. Test Results tab
	a.testResults = &testResultsModel{icons: ic}
	a.testResultsView = ui.NewVirtualTable(a.testResults)
	a.testResultsView.RowHeight = 28
	a.testResultsView.HeaderHeight = 32
	a.testResultsView.ColumnWidth = 175
	a.testResultsView.UIState().SetFlex(1)
	a.testResultsView.OnSelect = func(id string) {
		for _, r := range a.testResults.rows {
			if r.id == id && r.filePath != "" {
				a.jumpToLocation(r.filePath, r.line, 0)
				return
			}
		}
	}

	// 5. Debug Console tab — continuous selectable text stream + Delve REPL
	a.debugOutput = ui.NewOutputPanel()
	a.debugOutput.Font = ui.FontSpec{Family: "monospace", Size: 14}
	a.debugOutput.TerminalMode = true
	a.debugOutput.Prompt = "dlv > "
	a.debugOutput.UIState().SetFlex(1)
	a.debugOutput.AppendText("Delve DAP Debug Console — запустите отладку (F5) и вводите выражения прямо в консоли.\n")
	a.debugOutput.OnCommand = func(expr string) {
		if a.debug == nil {
			a.setStatus("Отладка Delve не запущена (нажмите F5)")
			return
		}
		a.debug.evaluate(expr)
	}
	a.debugEvalInput = ui.NewTextField("", nil)

	bottom := ui.NewTabs([]ui.Tab{
		{ID: "problems", Title: "Problems", Icon: ic.Warning, Content: a.diagnosticsView, NonClosable: true},
		{ID: "output", Title: "Output", Icon: ic.Info, Content: a.output, NonClosable: true},
		{ID: "terminal", Title: "Terminal (" + filepath.Base(defaultSh) + ")", Icon: ic.Command, Content: a.terminalOutput, NonClosable: true},
		{ID: "tests", Title: "Test Results", Icon: ic.Test, Content: a.testResultsView, NonClosable: true},
		{ID: "debug_console", Title: "Debug Console", Icon: ic.Debug, Content: a.debugOutput, NonClosable: true},
	}, "problems", nil)
	return bottom
}

func (a *App) buildStatusBar() *ui.Panel {
	bar := ui.NewStack(ui.Horizontal, 10)
	bar.Padding = ui.Symmetric(10, 3)
	bar.Background = a.darkTheme.Accent
	bar.Align = ui.AlignCenter
	bar.UIState().SetFlexShrink(0)
	bar.UIState().SetMinimumSize(ui.Vec2{Y: 30})
	bar.UIState().SetPreferredSize(ui.Vec2{Y: 30})

	a.branchBadge = ui.NewBadge("git: main", ui.RGBA(20, 24, 32, 160))
	a.lspBadge = ui.NewBadge("gopls: idle", ui.RGBA(20, 24, 32, 160))
	a.problemsBadge = ui.NewBadge("⊗ 0  ⚠ 0", ui.RGBA(20, 24, 32, 160))
	a.debugBadge = ui.NewBadge("Debug: idle", ui.RGBA(20, 24, 32, 160))

	a.taskProgress = ui.NewProgressBar(0)
	a.taskProgress.SetIndeterminate(true)
	a.taskProgress.UIState().SetPreferredSize(ui.Vec2{X: 90, Y: 16})
	a.taskProgress.UIState().SetMinimumSize(ui.Vec2{X: 90, Y: 16})
	a.taskProgress.UIState().SetVisible(false)

	a.status = ui.NewLabel("Откройте Go-модуль")
	a.status.Color = ui.RGBA(255, 255, 255, 255)
	a.status.Font = ui.FontSpec{Size: 13, Weight: 600}
	a.status.Ellipsis = true
	a.status.UIState().SetFlex(1)

	a.caretLabel = ui.NewLabel("Ln 1, Col 1")
	a.caretLabel.Color = ui.RGBA(255, 255, 255, 255)
	a.caretLabel.Font = ui.FontSpec{Size: 13}
	a.caretLabel.SetAutoSize(true)

	a.goVersionBadge = ui.NewBadge("Go "+runtime.Version(), ui.RGBA(20, 24, 32, 160))
	a.goVersionBadge.UIState().SetTooltip("Нажмите Go › Управление версиями Go и инструментов для смены SDK")

	var lastTelem time.Time
	var cachedTelem string
	a.telemetryLabel = ui.NewLabel("")
	a.telemetryLabel.Color = ui.RGBA(240, 245, 255, 230)
	a.telemetryLabel.Font = ui.FontSpec{Family: "monospace", Size: 12}
	a.telemetryLabel.SetAutoSize(true)
	a.telemetryLabel.TextFunc = func() string {
		now := time.Now()
		if cachedTelem != "" && now.Sub(lastTelem) < 500*time.Millisecond {
			return cachedTelem
		}
		lastTelem = now
		a.perfMu.Lock()
		frames := a.framesTotal
		buildDur := a.lastBuildDur
		totalDur := a.lastTotalDur
		a.perfMu.Unlock()
		if totalDur > buildDur {
			cachedTelem = fmt.Sprintf("UTF-8 | Build: %v Total: %v (#%d)", buildDur.Round(time.Microsecond), totalDur.Round(time.Microsecond), frames)
		} else {
			cachedTelem = fmt.Sprintf("UTF-8 | Frame: %v (#%d)", buildDur.Round(time.Microsecond), frames)
		}
		return cachedTelem
	}

	bar.Add(
		a.branchBadge,
		a.lspBadge,
		a.problemsBadge,
		a.debugBadge,
		a.taskProgress,
		a.status,
		a.caretLabel,
		a.goVersionBadge,
		a.telemetryLabel,
	)
	return bar
}
