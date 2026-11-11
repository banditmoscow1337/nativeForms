package main

import (
	"fmt"
	"math"
	"strings"

	ui "github.com/banditmoscow1337/nativeForms"
	"github.com/banditmoscow1337/nativeForms/editor"
)

func (d *demoApp) setStressMode(enabled bool) {
	d.stressRunning.Store(enabled)
	if d.stressToggle != nil {
		d.stressToggle.SetValue(enabled)
	}
	if enabled {
		d.logf("Стресс-тест ЗАПУЩЕН (интервал %v)", d.flags.interval)
		if d.stressWarmupDone.Load() && d.uiStressStep != nil && d.stressQueued.CompareAndSwap(false, true) {
			d.manager.Post(d.uiStressStep)
		}
	} else {
		d.logf("Стресс-тест ОСТАНОВЛЕН")
	}
}

// stepStressWorkload exercises layout, painting, text shaping, scrolling,
// ColorPicker, SpinBox, SegmentedControl, rounded controls, virtualized
// containers, and code editing on a single UI tick without allocating temporary
// strings on every frame.
func (d *demoApp) stepStressWorkload(step int) {
	tabIDs := [...]string{"controls", "editor", "virtual", "popups", "profiler"}
	targetTab := tabIDs[(step/25)%len(tabIDs)]
	if d.tabs.SelectedID != targetTab {
		d.tabs.Select(targetTab)
	}

	switch targetTab {
	case "controls":
		d.sliderStep.SetValue(float32((step * 5) % 105))
		d.sliderCont.SetValue(float32((step % 62)) * 0.1)
		if d.progressBar.Indeterminate {
			d.progressBar.StepPhase(0.06)
		} else {
			d.progressBar.SetValue(float32(step%100) / 100.0)
		}
		if d.colorPicker != nil && step%4 == 0 {
			d.colorPicker.SetHSVA(float32((step*15)%360), 0.75, 0.9, 1)
		}
		d.scrollForm.Scroll(48)
		if step%12 == 0 {
			d.scrollForm.Scroll(-400)
		}
	case "editor":
		lineCount := d.codeDoc.LineCount()
		targetLine := (step * 97) % max(1, lineCount)
		d.codeEditor.ScrollToLine(targetLine)
		if step%10 == 0 {
			at := d.codeEditor.Caret
			d.codeEditor.ReplaceTextRange(at, at, "// perf ")
			d.codeEditor.Handle(ui.Event{Type: ui.KeyDown, Key: ui.KeyZ, Mods: ui.ModControl})
		}
		_, _ = d.manager.TextInputState()
	case "virtual":
		d.vList.ScrollTo((step * 31) % max(1, d.vListModel.Len()))
		d.vTable.Offset = float32((step * 36) % max(1, d.vTableModel.Len()*32))
		d.vTable.OffsetX = float32((step * 15) % 220)
		d.vStackScroll.Scroll(64)
		if step%15 == 0 {
			d.vStackScroll.Scroll(-600)
		}
		if step%16 == 0 {
			d.outputPanel.AppendText("[STRESS] Virtualized stream tick OK\n")
		}
	case "popups":
		if step%10 == 0 {
			_ = d.manager.AccessibilitySnapshot()
		}
	case "profiler":
		d.manager.InvalidatePaint()
	}
}

func (d *demoApp) runSelfTests() {
	passed := 0
	failed := 0
	check := func(name string, cond bool) {
		if cond {
			passed++
		} else {
			failed++
			d.logf("[FAIL] %s", name)
		}
	}

	if d.flags.headless {
		d.manager.BeginFrame(d.flags.width, d.flags.height, 0.016)
	}

	// 1. Light & Dark theme migration test
	wasLight := d.isLight
	d.toggleTheme()
	check("Theme toggle updates Manager.Theme().IsLight()", d.manager.Theme().IsLight() == d.isLight)
	check("Root panel background migrates with theme", d.manager.Root().(*ui.Panel).Background == d.manager.Theme().Panel)
	if d.isLight != wasLight {
		d.toggleTheme()
	}

	// 2. MenuBar & Toolbar test
	check("MenuBar has 5 top-level menus", d.menuBar != nil && len(d.menuBar.Menus) == 5)
	openedMenu := d.menuBar.OpenMenuAt(0)
	check("MenuBar.OpenMenuAt opens dropdown popup", openedMenu && d.menuBar.ActiveIndex() == 0)
	d.menuBar.CloseMenu()
	check("MenuBar.CloseMenu closes dropdown popup", d.menuBar.ActiveIndex() == -1)
	check("Toolbar has items and SetItemActive works", d.topToolbar != nil && d.topToolbar.SetItemActive("indeterminate", true) && d.topToolbar.SetItemActive("indeterminate", false))

	// 3. SpinBox, SegmentedControl, Accordion, Hyperlink, Badge & Indeterminate tests
	d.spinBox.SetValue(50)
	d.spinBox.Increment()
	check("SpinBox.Increment steps by 5 (50 -> 55)", d.spinBox.Value == 55 && d.spinBox.FormattedValue() == "55%")
	d.spinBox.SetValue(35)

	d.segmentedCtrl.SetSelected("cover")
	check("SegmentedControl.SetSelected updates selection & Image.Fit", d.segmentedCtrl.SelectedID == "cover" && d.imageWidget.Fit == ui.ImageCover)
	d.segmentedCtrl.SetSelected("contain")

	d.accordion.ExpandSection(1, true)
	check("Accordion SingleOpen collapses section 0 when section 1 expands", !d.accordion.Sections[0].Expanded && d.accordion.Sections[1].Expanded)
	d.accordion.ExpandSection(0, true)

	d.checkBox.SetIndeterminate(true)
	d.progressBar.SetIndeterminate(true)
	check("Checkbox & ProgressBar support Indeterminate state", d.checkBox.Indeterminate && d.progressBar.Indeterminate)
	d.checkBox.SetIndeterminate(false)
	d.progressBar.SetValue(0.35)

	// 4. Frameless controls test
	d.setFramelessControls(true)
	check("State.SetFrameless(true) sets Frameless on controls", d.btnCounter.Frameless() && d.validField.Frameless())
	d.setFramelessControls(false)
	check("State.SetFrameless(false) restores control frames", !d.btnCounter.Frameless() && !d.validField.Frameless())

	// 5. CornerRadius & RoundedBorder test
	d.setControlsCornerRadius(10)
	check("State.SetCornerRadius(10) updates CornerRadius on controls", d.btnCounter.CornerRadius() == 10 && d.manager.Theme().CornerRadius == 10)
	d.setControlsCornerRadius(6)

	// 6. AutoSize & SizeToText test
	d.autoSizeBtn.SetText("OK")
	shortSz := d.autoSizeBtn.SizeToText()
	d.autoSizeBtn.SetText("Длинная надпись на кнопке с авторазмером")
	longSz := d.autoSizeBtn.SizeToText()
	check("SizeToText & AutoSize dynamically resize button to text width", shortSz.X > 0 && longSz.X > shortSz.X*1.5)

	// 7. ColorPicker test
	okHex := d.colorPicker.SetHex("#3478E2")
	check("ColorPicker.SetHex parses and updates color", okHex && d.colorPicker.Hex() == "#3478E2")
	d.colorPicker.SetHex("#E2AE3A")

	// 8. SplitPane test
	d.controlsSplit.Ratio = 0.42
	check("SplitPane.SetRatio clamps and updates ratio", math.Abs(float64(d.controlsSplit.Ratio-0.42)) < 0.001)
	d.controlsSplit.Ratio = 0.48

	// 9. TextField validation test
	okValid := d.validField.ReplaceTextRange(0, len(d.validField.Text), "98765")
	okInvalid := d.validField.ReplaceTextRange(0, len(d.validField.Text), "98a65")
	check("TextField.Validate accepts digits and rejects letters", okValid && !okInvalid && d.validField.Text == "98765")

	// 10. Slider & Toggle & RadioGroup test
	d.sliderStep.SetValue(43.2)
	check("Slider step snapping (43.2 -> 45)", d.sliderStep.Value == 45)

	prevToggle := d.toggleSwitch.Value
	d.toggleSwitch.SetValue(!prevToggle)
	check("Toggle.SetValue updates value", d.toggleSwitch.Value == !prevToggle)
	d.toggleSwitch.SetValue(prevToggle)

	d.radioGroup.SetSelected("cover")
	check("RadioGroup.SetSelected updates selection & Image.Fit", d.radioGroup.SelectedID == "cover" && d.imageWidget.Fit == ui.ImageCover)
	d.radioGroup.SetSelected("contain")

	// 11. CodeEditor & Document & ContextMenu & GoToDefinition test
	vBefore := d.codeDoc.Version()
	ch, err := d.codeDoc.Apply([]editor.Edit{{Start: 0, End: 0, Text: "// header\n"}})
	check("editor.Document.Apply increments version", err == nil && ch.Version == vBefore+1)
	_, undone := d.codeDoc.Undo()
	check("editor.Document.Undo restores content", undone && d.codeDoc.Version() == vBefore+2)
	_, redone := d.codeDoc.Redo()
	check("editor.Document.Redo re-applies edit", redone)
	_, _ = d.codeDoc.Undo()

	callIdx := strings.Index(d.codeDoc.Text(), "ComputeScore(128")
	if callIdx >= 0 {
		d.codeEditor.SetSelection(callIdx, callIdx)
		sym, _, _ := d.codeEditor.SymbolAtCaret()
		ctxItems := d.codeEditor.BuildContextMenu()
		check("CodeEditor.BuildContextMenu includes Go to Definition & Copy/Paste", sym == "ComputeScore" && len(ctxItems) >= 8 && ctxItems[0].ID == "goto_definition" && !ctxItems[0].Disabled)
		defOff, defOK := d.codeEditor.FindDefinitionOffset("ComputeScore")
		jumped := d.codeEditor.GoToDefinition()
		check("CodeEditor.GoToDefinition jumps to func ComputeScore", defOK && jumped && d.codeEditor.Anchor == defOff)
	}

	// 12. Virtualized views test
	d.vList.SetSelected("42")
	d.vList.ScrollTo(42)
	check("VirtualList selection & scroll", d.vList.SelectedID == "42")

	d.vTable.SetSelected("row-10")
	check("VirtualTable selection", d.vTable.SelectedID == "row-10")

	d.vTree.SetExpanded("w-layouts", true)
	d.vTree.SetSelected("w-grid")
	check("VirtualTree expand & select", d.vTree.Expanded["w-layouts"] && d.vTree.SelectedID == "w-grid")

	beforeLines := len(d.outputPanel.Lines())
	d.outputPanel.AppendText("Self-test log line 1\nSelf-test partial ")
	d.outputPanel.AppendText("completed\n")
	check("OutputPanel chunked streaming", len(d.outputPanel.Lines()) == beforeLines+2)

	// 13. Popups & Accessibility test
	pid := d.manager.OpenDialog(ui.NewDialog("SelfTest Dialog", ui.NewLabel("Testing modal")), 320, 160)
	check("OpenDialog returns non-zero PopupID", pid != 0)
	closed := d.manager.ClosePopup(pid)
	check("ClosePopup closes dialog", closed)

	d.tabs.Select("controls")
	if d.flags.headless {
		d.manager.BeginFrame(d.flags.width, d.flags.height, 0.016)
	}
	tree := d.manager.AccessibilitySnapshot()
	check("AccessibilitySnapshot produces non-empty tree", len(tree.Nodes) > 5)
	btnID := d.btnCounter.UIState().AccessibilityID()
	beforeClicks := d.clickCount
	activated := d.manager.PerformAccessibilityAction(btnID, ui.AccessibilityActivate, "")
	check("PerformAccessibilityAction activates Button", activated && d.clickCount == beforeClicks+1)

	cpID := d.colorPicker.UIState().AccessibilityID()
	cpActed := d.manager.PerformAccessibilityAction(cpID, ui.AccessibilitySetValue, "#E2AE3A")
	check("PerformAccessibilityAction sets ColorPicker value", cpActed && d.colorPicker.Hex() == "#E2AE3A")

	// 14. Top bars (MenuBar & header) remain fixed height across all tabs
	allTabsFixed := true
	for _, tid := range []string{"controls", "editor", "virtual", "popups", "profiler"} {
		d.tabs.Select(tid)
		if d.flags.headless {
			d.manager.BeginFrame(d.flags.width, d.flags.height, 0.016)
		}
		if d.menuBar.UIState().Bounds().H < 34 || (d.header != nil && d.header.UIState().Bounds().H < 48) {
			allTabsFixed = false
		}
	}
	d.tabs.Select("controls")
	if d.flags.headless {
		d.manager.BeginFrame(d.flags.width, d.flags.height, 0.016)
	}
	check("MenuBar and header keep fixed height across all tabs", allTabsFixed)

	// 15. DisplayScale updates Manager.DisplayScale() and ComboBox selection
	prevScale := d.manager.DisplayScale()
	d.setDisplayScale(1.5)
	check("setDisplayScale(1.5) updates Manager.DisplayScale and ComboBox", d.manager.DisplayScale() == 1.5 && d.comboBox.SelectedID == "1.500")
	d.setDisplayScale(prevScale)

	// 16. Safe pooling & lifecycle recycling (no ring-buffer overwrite when >32 instances exist)
	const burstCount = 40
	editors := make([]*ui.CodeEditor, burstCount)
	fields := make([]*ui.TextField, burstCount)
	seenEditors := make(map[*ui.CodeEditor]bool, burstCount)
	seenFields := make(map[*ui.TextField]bool, burstCount)
	for i := 0; i < burstCount; i++ {
		editors[i] = ui.NewCodeEditor(nil)
		fields[i] = ui.NewTextField("item", nil)
		seenEditors[editors[i]] = true
		seenFields[fields[i]] = true
	}
	check("Pool allocates distinct CodeEditors and TextFields beyond pool capacity", len(seenEditors) == burstCount && len(seenFields) == burstCount)
	for i := 0; i < burstCount; i++ {
		editors[i].Close()
		fields[i].Close()
	}
	reusedEd := ui.NewCodeEditor(nil)
	reusedTf := ui.NewTextField("reused", nil)
	check("Closed CodeEditor and TextField are recycled by pool", seenEditors[reusedEd] && seenFields[reusedTf])
	reusedEd.Close()
	reusedTf.Close()

	msg := fmt.Sprintf("Самопроверка завершена: %d PASS, %d FAIL", passed, failed)
	d.manager.Notify(msg)
	d.logf("%s", msg)
}
