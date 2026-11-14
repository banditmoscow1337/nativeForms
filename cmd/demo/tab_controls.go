package main

import (
	"fmt"
	"strconv"

	ui "github.com/banditmoscow1337/nativeForms"
)

func (d *demoApp) buildControlsTab() ui.Component {
	d.buildControlsLeftPane()
	rightScroll := d.buildControlsRightPane()
	d.controlsSplit = ui.NewSplitPane(ui.Horizontal, d.scrollForm, rightScroll)
	d.controlsSplit.Ratio = 0.48
	d.controlsSplit.UIState().SetFlex(1)
	return d.controlsSplit
}

func (d *demoApp) updateCounterLabels() {
	if d.btnCounter != nil {
		d.btnCounter.SetText(d.trf("ctrl.btn_counter", d.clickCount))
	}
	if d.statusBadge != nil {
		d.statusBadge.SetText(d.trf("ctrl.badge_clicks", d.clickCount))
	}
}

func (d *demoApp) buildControlsLeftPane() {
	leftStack := ui.NewStack(ui.Vertical, 8)
	leftStack.Padding = ui.All(8)

	secTitle := newSectionHeaderKey("ctrl.sec_left")

	d.topToolbar = ui.NewToolbar(
		ui.ToolbarItem{
			ID:         "click",
			LabelKey:   "tb.click",
			TooltipKey: "tb.tip_click",
			Icon:       d.icons[0],
			Action: func() {
				if d.btnCounter != nil && d.btnCounter.OnClick != nil {
					d.btnCounter.OnClick()
				}
			},
		},
		ui.ToolbarItem{
			ID:         "indeterminate",
			LabelKey:   "tb.indeterminate",
			TooltipKey: "tb.tip_indeterminate",
			Icon:       d.icons[1],
			Toggle:     true,
			OnToggle: func(active bool) {
				if d.checkBox != nil {
					d.checkBox.SetIndeterminate(active)
				}
				if d.progressBar != nil {
					d.progressBar.SetIndeterminate(active)
				}
				if d.indeterminateTgl != nil && d.indeterminateTgl.Value != active {
					d.indeterminateTgl.SetValue(active)
				}
				d.logf("Toolbar Toggle Indeterminate = %v", active)
			},
		},
		ui.ToolbarItem{Separator: true},
		ui.ToolbarItem{
			ID:         "selftest",
			LabelKey:   "tb.selftest",
			TooltipKey: "tb.tip_selftest",
			Icon:       d.icons[2],
			Action:     d.runSelfTests,
		},
		ui.ToolbarItem{Spacer: true},
		ui.ToolbarItem{
			ID:         "lang",
			LabelKey:   "tb.lang",
			TooltipKey: "tb.tip_lang",
			Icon:       d.icons[0],
			Action:     d.toggleLanguage,
		},
		ui.ToolbarItem{
			ID:         "theme",
			LabelKey:   "tb.theme",
			TooltipKey: "tb.tip_theme",
			Icon:       d.icons[3],
			Action:     d.toggleTheme,
		},
	)

	counterRow := ui.NewStack(ui.Horizontal, 8)
	counterRow.Align = ui.AlignCenter
	d.btnCounter = ui.NewButton("", func() {
		d.clickCount++
		d.updateCounterLabels()
		if !d.progressBar.Indeterminate {
			d.progressBar.SetValue(float32(d.clickCount%20) / 20.0)
		} else {
			d.progressBar.StepPhase(0.1)
		}
		d.tabs.SetDirty("controls", d.clickCount%2 == 1)
		d.logf("Button click #%d", d.clickCount)
	})
	d.btnCounter.SetIcon(d.icons[0])
	d.btnCounter.UIState().SetFlex(1)
	d.btnCounter.SetTooltipKey("ctrl.tip_counter")
	d.btnCounter.SetOnLocaleChange(func(_ *ui.LanguageManager) {
		d.updateCounterLabels()
	})

	d.statusBadge = ui.NewBadge("")
	d.statusBadge.SetTooltipKey("ctrl.tip_badge")
	d.statusBadge.SetOnLocaleChange(func(_ *ui.LanguageManager) {
		d.updateCounterLabels()
	})
	counterRow.Add(d.btnCounter, d.statusBadge)

	d.spinBox = ui.NewSpinBox("", 0, 100, 35, 5, func(val float32) {
		if d.sliderStep != nil && d.sliderStep.Value != val {
			d.sliderStep.SetValue(val)
		}
		d.progressBar.SetValue(val / 100.0)
		d.logf("SpinBox value: %.0f%%", val)
	})
	d.spinBox.Suffix = "%"
	d.spinBox.SetTextKey("ctrl.spinbox")
	d.spinBox.SetTooltipKey("ctrl.tip_spinbox")

	d.segmentedCtrl = ui.NewSegmentedControl([]ui.RadioOption{
		{ID: "contain", Label: "Contain", Icon: d.icons[0]},
		{ID: "cover", Label: "Cover", Icon: d.icons[1]},
		{ID: "stretch", Label: "Stretch", Icon: d.icons[2]},
	}, "contain", func(id string) {
		if d.radioGroup != nil && d.radioGroup.SelectedID != id {
			d.radioGroup.SetSelected(id)
		}
		switch id {
		case "contain":
			d.imageWidget.SetFit(ui.ImageContain)
		case "cover":
			d.imageWidget.SetFit(ui.ImageCover)
		case "stretch":
			d.imageWidget.SetFit(ui.ImageStretch)
		}
		d.logf("SegmentedControl -> ImageFit(%s)", id)
	})
	d.segmentedCtrl.SetTooltipKey("ctrl.tip_segmented")

	d.labeledSep = ui.NewLabeledSeparator("")
	d.labeledSep.SetTextKey("ctrl.sep_geometry")

	var pxLabels [21]string
	for i := range pxLabels {
		pxLabels[i] = fmt.Sprintf("%d px", i)
	}
	d.radiusSlider = ui.NewSlider("", 0, 20, 6, 1, func(val float32) {
		d.setControlsCornerRadius(val)
	})
	d.radiusSlider.SetTextKey("ctrl.slider_radius")
	d.radiusSlider.FormatValue = func(v float32) string {
		idx := min(len(pxLabels)-1, max(0, int(v+0.5)))
		return pxLabels[idx]
	}

	d.autoSizeToggle = ui.NewToggleKey("ctrl.tgl_autosize", true, func(v bool) {
		d.setControlsAutoSize(v)
	})
	d.autoSizeToggle.SetIcon(d.icons[2])

	sampleKeys := [...]string{
		"ctrl.autosize_sample_0",
		"ctrl.autosize_sample_1",
		"ctrl.autosize_sample_2",
	}
	d.autoSizeBtn = ui.NewButton("", func() {
		d.autoTextStep = (d.autoTextStep + 1) % len(sampleKeys)
		d.autoSizeBtn.SetTextKey(sampleKeys[d.autoTextStep])
		sz := d.autoSizeBtn.SizeToText()
		d.autoSizeLabel.SetText(d.trf("ctrl.autosize_badge_fmt", d.autoTextStep+1, sz.X, sz.Y))
		d.autoSizeLabel.SizeToText()
		d.logf("SizeToText(): button=%.0fx%.0f, text=%q", sz.X, sz.Y, d.autoSizeBtn.Text)
	})
	d.autoSizeBtn.SetTextKey(sampleKeys[d.autoTextStep])
	d.autoSizeBtn.SetLocaleSizePolicy(ui.LocaleSizeCurrent)
	d.autoSizeBtn.SetCornerRadius(8)
	d.autoSizeBtn.SetTooltipKey("ctrl.tip_autosize_btn")

	d.autoSizeLabel = ui.NewLabelKey("ctrl.autosize_badge_init")
	d.autoSizeLabel.Background = d.darkTheme.Control
	d.autoSizeLabel.Border = d.darkTheme.Accent
	d.autoSizeLabel.Padding = ui.Symmetric(10, 4)
	d.autoSizeLabel.SetCornerRadius(8)
	d.autoSizeLabel.SetLocaleSizePolicy(ui.LocaleSizeCurrent)

	d.framelessCtrlTgl = ui.NewToggleKey("ctrl.tgl_frameless", false, func(v bool) {
		d.setFramelessControls(v)
	})
	d.framelessCtrlTgl.SetIcon(d.icons[3])

	d.toggleSwitch = ui.NewToggleKey("ctrl.tgl_gamenav", false, func(v bool) {
		d.manager.SetGameNavigation(v)
		d.logf("SetGameNavigation = %v", v)
	})
	d.toggleSwitch.SetIcon(d.icons[1])

	d.checkBox = ui.NewCheckboxKey("ctrl.chk_enable", true, func(checked bool) {
		d.validField.UIState().SetEnabled(checked)
		d.sliderStep.UIState().SetEnabled(checked)
		d.spinBox.UIState().SetEnabled(checked)
		d.segmentedCtrl.UIState().SetEnabled(checked)
		d.comboBox.UIState().SetEnabled(checked)
		d.colorPicker.UIState().SetEnabled(checked)
		d.compactPicker.UIState().SetEnabled(checked)
		d.logf("Form controls enabled: %v", checked)
	})
	d.checkBox.SetIcon(d.icons[2])

	d.indeterminateTgl = ui.NewToggleKey("ctrl.tgl_indeterminate", false, func(v bool) {
		d.checkBox.SetIndeterminate(v)
		d.progressBar.SetIndeterminate(v)
		if d.topToolbar != nil {
			d.topToolbar.SetItemActive("indeterminate", v)
		}
		d.logf("Indeterminate mode: %v", v)
	})

	d.radioGroup = ui.NewRadioGroup([]ui.RadioOption{
		{ID: "contain", Label: "ImageFit: ImageContain", Icon: d.icons[0]},
		{ID: "cover", Label: "ImageFit: ImageCover", Icon: d.icons[1]},
		{ID: "stretch", Label: "ImageFit: ImageStretch", Icon: d.icons[2]},
	}, "contain", func(id string) {
		if d.segmentedCtrl != nil && d.segmentedCtrl.SelectedID != id {
			d.segmentedCtrl.SetSelected(id)
		}
		switch id {
		case "contain":
			d.imageWidget.SetFit(ui.ImageContain)
		case "cover":
			d.imageWidget.SetFit(ui.ImageCover)
		case "stretch":
			d.imageWidget.SetFit(ui.ImageStretch)
		}
		d.logf("Image.SetFit(%s)", id)
	})

	comboOpts := make([]ui.RadioOption, 16)
	for i := range comboOpts {
		scale := 1.0 + float64(i)*0.125
		comboOpts[i] = ui.RadioOption{
			ID:    fmt.Sprintf("%.3f", scale),
			Label: fmt.Sprintf("Option #%02d — DisplayScale %.3fx", i+1, scale),
			Icon:  d.icons[i%len(d.icons)],
		}
	}
	initScaleID := fmt.Sprintf("%.3f", d.manager.DisplayScale())
	d.comboBox = ui.NewComboBox(comboOpts, initScaleID, func(id string) {
		if val, err := strconv.ParseFloat(id, 32); err == nil {
			d.setDisplayScale(float32(val))
		}
	})
	d.comboBox.SetTooltipKey("ctrl.tip_combo")
	d.comboBox.SetOnLocaleChange(func(lm *ui.LanguageManager) {
		for i := range d.comboBox.Options {
			scale := 1.0 + float64(i)*0.125
			d.comboBox.Options[i].Label = lm.Trf("ctrl.combo_opt_fmt", i+1, scale)
		}
	})

	var pctLabels [21]string
	for i := 0; i <= 20; i++ {
		pctLabels[i] = fmt.Sprintf("%d%%", i*5)
	}
	d.sliderStep = ui.NewSlider("", 0, 100, 35, 5, func(val float32) {
		if d.spinBox != nil && d.spinBox.Value != val {
			d.spinBox.SetValue(val)
		}
		d.progressBar.SetValue(val / 100.0)
	})
	d.sliderStep.SetTextKey("ctrl.slider_step")
	d.sliderStep.FormatValue = func(v float32) string {
		idx := min(20, max(0, int(v/5+0.5)))
		return pctLabels[idx]
	}

	var radLabels [64]string
	for i := range radLabels {
		radLabels[i] = fmt.Sprintf("%.2f rad", float64(i)*0.1)
	}
	d.sliderCont = ui.NewSlider("", 0, 6.28318, 0.8, 0.1, func(val float32) {
		if d.canvasWidget != nil {
			d.canvasWidget.phase = val
			d.manager.InvalidatePaint()
		}
	})
	d.sliderCont.SetTextKey("ctrl.slider_wave")
	d.sliderCont.FormatValue = func(v float32) string {
		idx := min(len(radLabels)-1, max(0, int(v*10+0.5)))
		return radLabels[idx]
	}

	d.progressBar = ui.NewProgressBar(0.35)
	d.progressBar.ShowValue = true

	// Accordion with Collapsible sections and Hyperlink
	accBody1 := ui.NewStack(ui.Vertical, 4)
	accLbl1 := ui.NewLabelKey("ctrl.acc_sec1_body")
	accLbl1.SetLocaleSizePolicy(ui.LocaleSizeWrapHeight)
	d.docHyperlink = ui.NewHyperlink("", "https://github.com/banditmoscow1337/nativeForms", func(url string) {
		d.manager.Notifyf("Opening link: %s", url)
		d.logf("Hyperlink activated: %s", url)
	})
	d.docHyperlink.SetTextKey("ctrl.acc_link")
	accBody1.Add(accLbl1, d.docHyperlink)

	accBody2 := ui.NewLabelKey("ctrl.acc_sec2_body")
	accBody2.SetLocaleSizePolicy(ui.LocaleSizeWrapHeight)

	sec1 := ui.NewCollapsible("", accBody1, true, nil)
	sec1.SetTextKey("ctrl.acc_sec1_title")
	sec1.Icon = d.icons[0]
	sec2 := ui.NewCollapsible("", accBody2, false, nil)
	sec2.SetTextKey("ctrl.acc_sec2_title")
	sec2.Icon = d.icons[1]
	d.accordion = ui.NewAccordion(true, sec1, sec2)

	// Demonstrate text clipping inside constrained bounds.
	d.overflowLabel = ui.NewLabelKey("ctrl.overflow_lbl")
	d.overflowLabel.Font = ui.FontSpec{Size: 22, Weight: 700}
	d.overflowLabel.Ellipsis = true
	d.overflowLabel.UIState().SetPreferredSize(ui.Vec2{X: 320, Y: 34})

	d.overflowBtn = ui.NewButton("", func() {
		d.logf("Overflow clipping button clicked")
	})
	d.overflowBtn.SetTextKey("ctrl.overflow_btn")
	d.overflowBtn.Font = ui.FontSpec{Size: 20, Weight: 600}

	leftStack.Add(
		secTitle,
		d.topToolbar,
		counterRow,
		d.spinBox,
		d.segmentedCtrl,
		d.labeledSep,
		d.radiusSlider,
		d.autoSizeToggle,
		d.autoSizeBtn,
		d.autoSizeLabel,
		d.framelessCtrlTgl,
		d.toggleSwitch,
		d.checkBox,
		d.indeterminateTgl,
		d.radioGroup,
		d.comboBox,
		d.sliderStep,
		d.sliderCont,
		d.progressBar,
		d.accordion,
		d.overflowLabel,
		d.overflowBtn,
		ui.NewSpacer(1, 8),
	)
	d.scrollForm = ui.NewScrollPanel(leftStack)
	d.scrollForm.UIState().SetFlex(1)
}
