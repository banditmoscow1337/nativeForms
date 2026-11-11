package main

import (
	"fmt"
	"strings"

	ui "github.com/banditmoscow1337/nativeForms"
	"github.com/banditmoscow1337/nativeForms/editor"
)

const sampleGoSource = `package showcase

import (
	"fmt"
	"time"
)

// WidgetMetrics stores per-frame CPU and memory diagnostics.
type WidgetMetrics struct {
	FrameID     uint64
	Duration    time.Duration
	AllocBytes  uint64
	Description string
}

// ComputeScore evaluates synthetic layout throughput.
func ComputeScore(items int, scale float64) (float64, error) {
	var metrics WidgetMetrics
	metrics.Description = "benchmark"
	if items <= 0 {
		return 0, fmt.Errorf("items must be positive: %d", items)
	}
	total := 0.0
	for i := 0; i < items; i++ {
		total += float64(i) * scale
	}
	return total / float64(items), nil
}

func RunBenchmark() {
	score, err := ComputeScore(128, 1.5)
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(score)
}
`

func (d *demoApp) buildEditorTab() ui.Component {
	panel := ui.NewStack(ui.Vertical, 8)
	panel.Padding = ui.All(6)

	toolbar := ui.NewStack(ui.Horizontal, 6)
	toolbar.UIState().SetFlexShrink(0)
	toolbar.UIState().SetMinimumSize(ui.Vec2{Y: 38})
	btnUndo := ui.NewButton("Undo", func() {
		d.codeEditor.Handle(ui.Event{Type: ui.KeyDown, Key: ui.KeyZ, Mods: ui.ModControl})
		d.updateEditorInfo()
	})
	btnUndo.UIState().SetMinimumSize(ui.Vec2{X: 80})

	btnRedo := ui.NewButton("Redo", func() {
		d.codeEditor.Handle(ui.Event{Type: ui.KeyDown, Key: ui.KeyY, Mods: ui.ModControl})
		d.updateEditorInfo()
	})
	btnRedo.UIState().SetMinimumSize(ui.Vec2{X: 80})

	btnCtxMenu := ui.NewButton("Контекстное меню (ПКМ / Shift+F10)", func() {
		d.manager.PerformAccessibilityAction(d.codeEditor.UIState().AccessibilityID(), ui.AccessibilityFocus, "")
		caret := d.codeEditor.CaretRect()
		d.codeEditor.OpenContextMenu(caret.X, caret.Y+caret.H)
	})
	btnCtxMenu.UIState().SetMinimumSize(ui.Vec2{X: 230})

	btnGoToDef := ui.NewButton("Go to Definition (F12)", func() {
		if !d.codeEditor.GoToDefinition() {
			// If caret is not on an identifier, position on ComputeScore call and jump.
			if idx := strings.Index(d.codeDoc.Text(), "ComputeScore(128"); idx >= 0 {
				d.codeEditor.SetSelection(idx, idx)
				d.codeEditor.GoToDefinition()
			}
		}
		d.updateEditorInfo()
	})
	btnGoToDef.UIState().SetMinimumSize(ui.Vec2{X: 170})

	btnCompl := ui.NewButton("Автодополнение", func() {
		d.manager.PerformAccessibilityAction(d.codeEditor.UIState().AccessibilityID(), ui.AccessibilityFocus, "")
		d.showEditorCompletions()
	})
	btnCompl.UIState().SetMinimumSize(ui.Vec2{X: 145})

	btnBp := ui.NewButton("Breakpoint (Line 17)", func() {
		cur := d.codeEditor.Breakpoint(16)
		d.codeEditor.SetBreakpoint(16, !cur)
		d.updateEditorInfo()
	})
	btnBp.UIState().SetMinimumSize(ui.Vec2{X: 155})

	btnTokens := ui.NewButton("LSP Tokens", func() {
		d.applySampleSemanticTokens()
	})
	btnTokens.UIState().SetMinimumSize(ui.Vec2{X: 115})

	btnLargeDoc := ui.NewButton("5 000 строк", func() {
		var b strings.Builder
		b.WriteString(sampleGoSource)
		for i := 0; i < 5000; i++ {
			fmt.Fprintf(&b, "func generatedHelper_%04d(x int) int { return x + %d } // line %d\n", i, i*3, i+30)
		}
		doc, err := editor.NewDocument(b.String())
		if err == nil {
			d.codeDoc = doc
			d.codeEditor.SetDocument(doc)
			d.codeEditor.SetBreakpoint(2, true)
			d.codeEditor.ScrollToLine(2500)
			d.updateEditorInfo()
			d.logf("Загружен документ на %d строк (%d байт)", doc.LineCount(), doc.Len())
		}
	})
	btnLargeDoc.UIState().SetMinimumSize(ui.Vec2{X: 120})

	toolbar.Add(btnUndo, btnRedo, btnCtxMenu, btnGoToDef, btnCompl, btnBp, btnTokens, btnLargeDoc)

	doc, _ := editor.NewDocument(sampleGoSource)
	d.codeDoc = doc
	d.codeEditor = ui.NewCodeEditor(doc)
	d.codeEditor.UIState().SetFlex(1)
	d.codeEditor.SetBreakpoint(7, true)
	d.codeEditor.SetBreakpoint(16, true)
	d.codeEditor.Palette = darkSyntaxPalette()
	d.codeEditor.OnChange = func(ch editor.Change) {
		if !d.stressRunning.Load() {
			d.updateEditorInfo()
		}
	}
	d.codeEditor.OnBreakpointChange = func(line int, enabled bool) {
		d.updateEditorInfo()
		d.logf("Breakpoint на строке %d: %v", line+1, enabled)
	}
	d.codeEditor.OnGoToDefinition = func(symbol string, offset int) {
		if defOffset, ok := d.codeEditor.FindDefinitionOffset(symbol); ok {
			d.codeEditor.SetSelection(defOffset, defOffset+len(symbol))
			pos, _ := d.codeDoc.UTF16Position(defOffset)
			d.updateEditorInfo()
			d.logf("Go to Definition: %q -> L%d:C%d (offset %d)", symbol, pos.Line+1, pos.Character+1, defOffset)
			d.manager.Notifyf("Переход к определению %s (строка %d)", symbol, pos.Line+1)
		} else {
			d.logf("Определение для %q не найдено в текущем документе", symbol)
		}
	}

	d.editorInfo = ui.NewLabel("")
	d.editorInfo.Ellipsis = true
	d.editorInfo.UIState().SetFlexShrink(0)
	d.editorInfo.UIState().SetMinimumSize(ui.Vec2{Y: 24})
	d.editorInfo.UIState().SetPreferredSize(ui.Vec2{X: 360, Y: 24})

	panel.Add(toolbar, d.codeEditor, d.editorInfo)
	return panel
}

func (d *demoApp) updateEditorInfo() {
	if d.codeEditor == nil || d.editorInfo == nil {
		return
	}
	first, last := d.codeEditor.VisibleLines()
	bps := d.codeEditor.BreakpointLines()
	pos, _ := d.codeDoc.UTF16Position(d.codeEditor.Caret)
	sym, _, _ := d.codeEditor.SymbolAtCaret()
	if sym == "" {
		sym = "-"
	}
	d.editorInfo.SetText(fmt.Sprintf(
		"Документ v%d | Строк: %d | Каретка: L%d:C%d (символ: %s) | Видимые: [%d..%d) | Breakpoints: %v | ПКМ / Shift+F10 — контекстное меню",
		d.codeDoc.Version(), d.codeDoc.LineCount(),
		pos.Line+1, pos.Character+1, sym, first, last, bps,
	))
}

func (d *demoApp) showEditorCompletions() {
	insertSnippet := func(snippet string) {
		at := d.codeEditor.Caret
		d.codeEditor.ReplaceTextRange(at, at, snippet)
		d.updateEditorInfo()
	}
	items := []ui.MenuItem{
		{ID: "fmt", Label: "fmt.Println(\"debug metric\")", Icon: d.icons[0], Action: func() { insertSnippet("fmt.Println(\"debug metric\")") }},
		{ID: "for", Label: "for i := 0; i < count; i++ { ... }", Icon: d.icons[1], Action: func() { insertSnippet("for i := 0; i < count; i++ {\n\t\n}") }},
		{ID: "err", Label: "if err != nil { return err }", Icon: d.icons[2], Action: func() { insertSnippet("if err != nil {\n\treturn err\n}") }},
		{ID: "disabled", Label: "Недоступное предложение (Disabled)", Icon: d.icons[3], Disabled: true},
	}
	d.codeEditor.ShowCompletions(items)
}

func (d *demoApp) applySampleSemanticTokens() {
	ver := d.codeDoc.Version()
	legend := []string{"namespace", "struct", "function", "variable"}
	data := []uint32{
		0, 8, 8, 0, 0,
	}
	if err := d.codeEditor.SetSemanticTokens(ver, legend, data); err != nil {
		d.logf("SetSemanticTokens error: %v", err)
		return
	}
	_ = d.codeEditor.SetStyleRanges(ver, []ui.StyledRange{
		{Start: 0, End: 7, Style: ui.CodeKeyword},
	})
	d.logf("Применены SetSemanticTokens и SetStyleRanges для документа v%d", ver)
}

func (d *demoApp) buildVirtualTab() ui.Component {
	root := ui.NewStack(ui.Vertical, 8)
	root.Padding = ui.All(6)

	bar := ui.NewStack(ui.Horizontal, 8)
	bar.UIState().SetFlexShrink(0)
	bar.UIState().SetMinimumSize(ui.Vec2{Y: 38})
	btn1k := ui.NewButton("1 000 записей", func() { d.resizeVirtualModels(1000) })
	btn10k := ui.NewButton("10 000 записей", func() { d.resizeVirtualModels(10000) })
	btn50k := ui.NewButton("50 000 записей", func() { d.resizeVirtualModels(50000) })
	btnExpand := ui.NewButton("Развернуть/Свернуть дерево", func() {
		exp := !d.vTree.Expanded["w-controls"]
		for _, k := range []string{"root-widgets", "w-controls", "w-layouts", "w-popups", "root-virtual", "root-editors", "root-platform"} {
			d.vTree.SetExpanded(k, exp)
		}
	})
	btnLog := ui.NewButton("Добавить 100 строк в OutputPanel", func() {
		var chunk strings.Builder
		for i := 0; i < 100; i++ {
			fmt.Fprintf(&chunk, "[BUILD %04d] Compiling widget module... OK (%.2fms)\n", i, 0.4+float64(i%9)*0.15)
		}
		d.outputPanel.AppendText(chunk.String())
	})
	bar.Add(btn1k, btn10k, btn50k, btnExpand, btnLog)

	upper := ui.NewStack(ui.Horizontal, 8)
	upper.UIState().SetFlex(1)

	treeBox := ui.NewStack(ui.Vertical, 4)
	treeBox.UIState().SetFlex(1)
	treeBox.Add(ui.NewLabel("VirtualTree (ПКМ для контекстного меню):"))
	d.vTreeModel = newDemoTreeModel(d.icons)
	d.vTree = ui.NewVirtualTree(d.vTreeModel)
	d.vTree.UIState().SetFlex(1)
	for _, id := range []string{"root-widgets", "w-controls", "root-virtual", "root-editors", "root-platform"} {
		d.vTree.SetExpanded(id, true)
	}
	d.vTree.OnSelect = func(id string) {
		d.logf("VirtualTree выбран узел: %s (%s)", id, d.vTreeModel.Label(id))
	}
	d.vTree.UIState().SetContextMenuBuilder(func(_, _ float32) []ui.MenuItem {
		sel := d.vTree.SelectedID
		hasChildren := sel != "" && d.vTreeModel.Children(sel) > 0
		return []ui.MenuItem{
			{ID: "copy-node", Label: "Copy Node Label", Disabled: sel == "", Action: func() {
				if d.window != nil && sel != "" {
					_ = d.window.SetClipboardText(d.vTreeModel.Label(sel))
				}
				d.logf("Скопирован узел дерева: %s", d.vTreeModel.Label(sel))
			}},
			{ID: "toggle-node", Label: "Expand / Collapse Node", Disabled: !hasChildren, Action: func() {
				d.vTree.SetExpanded(sel, !d.vTree.Expanded[sel])
			}},
			{Separator: true},
			{ID: "expand-all", Label: "Expand All Groups", Action: func() {
				for _, k := range []string{"root-widgets", "w-controls", "w-layouts", "w-popups", "root-virtual", "root-editors", "root-platform"} {
					d.vTree.SetExpanded(k, true)
				}
			}},
			{ID: "collapse-all", Label: "Collapse All Groups", Action: func() {
				for _, k := range []string{"root-widgets", "w-controls", "w-layouts", "w-popups", "root-virtual", "root-editors", "root-platform"} {
					d.vTree.SetExpanded(k, false)
				}
			}},
		}
	})
	treeBox.Add(d.vTree)

	listBox := ui.NewStack(ui.Vertical, 4)
	listBox.UIState().SetFlex(1)
	listBox.Add(ui.NewLabel("VirtualList (ПКМ для контекстного меню):"))
	d.vListModel = newDemoListModel(d.flags.count, d.icons)
	d.vList = ui.NewVirtualList(d.vListModel)
	d.vList.UIState().SetFlex(1)
	d.vList.OnSelect = func(id string) {
		d.logf("VirtualList выбрана строка ID=%s", id)
	}
	d.vList.UIState().SetContextMenuBuilder(func(_, _ float32) []ui.MenuItem {
		sel := d.vList.SelectedID
		return []ui.MenuItem{
			{ID: "copy-id", Label: "Copy Selected ID (" + nonEmpty(sel, "none") + ")", Disabled: sel == "", Action: func() {
				if d.window != nil && sel != "" {
					_ = d.window.SetClipboardText(sel)
				}
				d.logf("VirtualList скопирован ID=%s", sel)
			}},
			{ID: "top", Label: "Scroll to Top", Action: func() { d.vList.ScrollTo(0) }},
			{ID: "bottom", Label: "Scroll to Bottom", Action: func() { d.vList.ScrollTo(max(0, d.vListModel.Len()-1)) }},
		}
	})
	listBox.Add(d.vList)

	stackBox := ui.NewStack(ui.Vertical, 4)
	stackBox.UIState().SetFlex(1)
	stackBox.Add(ui.NewLabel("VirtualStack (StackModel + StackModelUpdater):"))
	d.vStackModel = newDemoStackModel(d.flags.count)
	d.vStack = ui.NewVirtualStack(d.vStackModel)
	d.vStackScroll = ui.NewScrollPanel(d.vStack)
	d.vStackScroll.UIState().SetFlex(1)
	stackBox.Add(d.vStackScroll)

	upper.Add(treeBox, listBox, stackBox)

	lower := ui.NewStack(ui.Horizontal, 8)
	lower.UIState().SetFlex(1)

	tableBox := ui.NewStack(ui.Vertical, 4)
	tableBox.UIState().SetFlex(1)
	tableBox.Add(ui.NewLabel("VirtualTable (2D Scroll + ПКМ меню):"))
	d.vTableModel = newDemoTableModel(d.flags.count, d.icons)
	d.vTable = ui.NewVirtualTable(d.vTableModel)
	d.vTable.ColumnWidth = 140
	d.vTable.UIState().SetFlex(1)
	d.vTable.OnSelect = func(id string) {
		d.logf("VirtualTable выбрана строка: %s", id)
	}
	d.vTable.UIState().SetContextMenuBuilder(func(_, _ float32) []ui.MenuItem {
		sel := d.vTable.SelectedID
		return []ui.MenuItem{
			{ID: "copy-row", Label: "Copy Row ID (" + nonEmpty(sel, "none") + ")", Disabled: sel == "", Action: func() {
				if d.window != nil && sel != "" {
					_ = d.window.SetClipboardText(sel)
				}
				d.logf("VirtualTable скопирована строка: %s", sel)
			}},
			{ID: "reset-scroll", Label: "Reset Scroll Position", Action: func() {
				d.vTable.Offset, d.vTable.OffsetX = 0, 0
				d.vTable.ModelChanged()
			}},
		}
	})
	tableBox.Add(d.vTable)

	outBox := ui.NewStack(ui.Vertical, 4)
	outBox.UIState().SetFlex(1)
	outHeader := ui.NewStack(ui.Horizontal, 6)
	outHeader.UIState().SetFlexShrink(0)
	outHeader.UIState().SetMinimumSize(ui.Vec2{Y: 38})
	outHeader.Add(ui.NewLabel("OutputPanel (потоковый лог с ПКМ меню):"))
	btnClearOut := ui.NewButton("Очистить (Clear)", func() {
		d.outputPanel.Clear()
	})
	btnClearOut.UIState().SetMinimumSize(ui.Vec2{X: 140})
	outHeader.Add(btnClearOut)

	d.outputPanel = ui.NewOutputPanel()
	d.outputPanel.SetMaxLines(500)
	d.outputPanel.UIState().SetFlex(1)
	d.outputPanel.AppendText("=== Инициализация OutputPanel завершена ===\n")
	d.outputPanel.List.UIState().SetContextMenuBuilder(func(_, _ float32) []ui.MenuItem {
		lines := d.outputPanel.Lines()
		return []ui.MenuItem{
			{ID: "copy-all", Label: "Copy All Log Lines", Disabled: len(lines) == 0, Action: func() {
				if d.window != nil {
					_ = d.window.SetClipboardText(strings.Join(lines, "\n"))
				}
				d.manager.Notify("Лог скопирован в буфер обмена")
			}},
			{ID: "scroll-end", Label: "Scroll to End", Disabled: len(lines) == 0, Action: func() {
				d.outputPanel.ScrollToEnd()
			}},
			{Separator: true},
			{ID: "clear-log", Label: "Clear Output", Disabled: len(lines) == 0, Action: func() {
				d.outputPanel.Clear()
			}},
		}
	})
	outBox.Add(outHeader, d.outputPanel)

	lower.Add(tableBox, outBox)

	d.virtualSplit = ui.NewSplitPane(ui.Vertical, upper, lower)
	d.virtualSplit.Ratio = 0.52
	d.virtualSplit.UIState().SetFlex(1)

	root.Add(bar, d.virtualSplit)
	return root
}

func (d *demoApp) resizeVirtualModels(count int) {
	d.vListModel.resize(count)
	d.vList.ModelChanged()

	d.vTableModel.resize(count)
	d.vTable.ModelChanged()

	d.vStackModel.resize(count, fmt.Sprintf("Row[%dk]", count/1000))
	d.vStack.ModelChanged()

	d.logf("Размер виртуальных моделей обновлён до %d элементов", count)
}
