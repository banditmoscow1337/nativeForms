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

func (d *demoApp) buildControlsLeftPane() {
	leftStack := ui.NewStack(ui.Vertical, 8)
	leftStack.Padding = ui.All(8)

	secTitle := newSectionHeader("Базовые контролы, Toolbar, SpinBox, SegmentedControl и Accordion")

	d.topToolbar = ui.NewToolbar(
		ui.ToolbarItem{
			ID:      "click",
			Label:   "+1 Клик",
			Tooltip: "Увеличить счётчик кликов",
			Icon:    d.icons[0],
			Action: func() {
				if d.btnCounter != nil && d.btnCounter.OnClick != nil {
					d.btnCounter.OnClick()
				}
			},
		},
		ui.ToolbarItem{
			ID:      "indeterminate",
			Label:   "Indeterminate",
			Tooltip: "Переключить неопределённое состояние Checkbox и ProgressBar",
			Icon:    d.icons[1],
			Toggle:  true,
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
			ID:      "selftest",
			Label:   "Самопроверка",
			Tooltip: "Запустить проверку всех виджетов (F5)",
			Icon:    d.icons[2],
			Action:  d.runSelfTests,
		},
		ui.ToolbarItem{Spacer: true},
		ui.ToolbarItem{
			ID:      "theme",
			Label:   "Тема",
			Tooltip: "Переключить Dark / Light тему (F2)",
			Icon:    d.icons[3],
			Action:  d.toggleTheme,
		},
	)

	counterRow := ui.NewStack(ui.Horizontal, 8)
	counterRow.Align = ui.AlignCenter
	d.btnCounter = ui.NewButton("Счётчик кликов: 0", func() {
		d.clickCount++
		d.btnCounter.SetText(fmt.Sprintf("Счётчик кликов: %d", d.clickCount))
		if d.statusBadge != nil {
			d.statusBadge.SetText(fmt.Sprintf("Кликов: %d", d.clickCount))
		}
		if !d.progressBar.Indeterminate {
			d.progressBar.SetValue(float32(d.clickCount%20) / 20.0)
		} else {
			d.progressBar.StepPhase(0.1)
		}
		d.tabs.SetDirty("controls", d.clickCount%2 == 1)
		d.logf("Клик по кнопке #%d", d.clickCount)
	})
	d.btnCounter.SetIcon(d.icons[0])
	d.btnCounter.UIState().SetFlex(1)
	d.btnCounter.UIState().SetTooltip("Кнопка с иконкой и отслеживанием состояния Dirty на вкладке")

	d.statusBadge = ui.NewBadge("Кликов: 0")
	d.statusBadge.UIState().SetTooltip("Компактный индикатор Badge с авторазмером")
	counterRow.Add(d.btnCounter, d.statusBadge)

	d.spinBox = ui.NewSpinBox("Числовое поле (SpinBox):", 0, 100, 35, 5, func(val float32) {
		if d.sliderStep != nil && d.sliderStep.Value != val {
			d.sliderStep.SetValue(val)
		}
		d.progressBar.SetValue(val / 100.0)
		d.logf("SpinBox значение: %.0f%%", val)
	})
	d.spinBox.Suffix = "%"
	d.spinBox.UIState().SetTooltip("SpinBox: кнопки −/+, колесо мыши, клавиши Up/Down/PageUp/PageDown/Home/End")

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
	d.segmentedCtrl.UIState().SetTooltip("SegmentedControl: горизонтальный переключатель режимов с иконками и навигацией стрелками")

	d.labeledSep = ui.NewLabeledSeparator("Геометрия, Скругления и Авторазмер")

	var pxLabels [21]string
	for i := range pxLabels {
		pxLabels[i] = fmt.Sprintf("%d px", i)
	}
	d.radiusSlider = ui.NewSlider("Радиус скругления элементов (SetCornerRadius)", 0, 20, 6, 1, func(val float32) {
		d.setControlsCornerRadius(val)
	})
	d.radiusSlider.FormatValue = func(v float32) string {
		idx := min(len(pxLabels)-1, max(0, int(v+0.5)))
		return pxLabels[idx]
	}

	d.autoSizeToggle = ui.NewToggle("Авторазмер контролов под размер текста (SetAutoSize)", true, func(v bool) {
		d.setControlsAutoSize(v)
	})
	d.autoSizeToggle.SetIcon(d.icons[2])

	autoSamples := [...]string{
		"OK",
		"Короткий текст (SizeToText)",
		"Кнопка автоматически расширяется и сжимается строго под длину своего текста!",
	}
	d.autoSizeBtn = ui.NewButton(autoSamples[1], func() {
		d.autoTextStep = (d.autoTextStep + 1) % len(autoSamples)
		d.autoSizeBtn.SetText(autoSamples[d.autoTextStep])
		sz := d.autoSizeBtn.SizeToText()
		d.autoSizeLabel.SetText(fmt.Sprintf("Бейдж авторазмера: шаг #%d (%.0fx%.0f px)", d.autoTextStep+1, sz.X, sz.Y))
		d.autoSizeLabel.SizeToText()
		d.logf("SizeToText(): кнопка=%.0fx%.0f, текст=%q", sz.X, sz.Y, d.autoSizeBtn.Text)
	})
	d.autoSizeBtn.SetAutoSize(true)
	d.autoSizeBtn.SetCornerRadius(8)
	d.autoSizeBtn.UIState().SetTooltip("Нажмите, чтобы изменить длину текста и выполнить автоматический подгон размера (SizeToText / AutoSize)")

	d.autoSizeLabel = ui.NewLabel("Бейдж авторазмера: шаг #2 (авто-ширина под текст)")
	d.autoSizeLabel.Background = d.darkTheme.Control
	d.autoSizeLabel.Border = d.darkTheme.Accent
	d.autoSizeLabel.Padding = ui.Symmetric(10, 4)
	d.autoSizeLabel.SetCornerRadius(8)
	d.autoSizeLabel.SetAutoSize(true)

	d.framelessCtrlTgl = ui.NewToggle("Отключить стандартные рамки/фон контролов (SetFrameless)", false, func(v bool) {
		d.setFramelessControls(v)
	})
	d.framelessCtrlTgl.SetIcon(d.icons[3])

	d.toggleSwitch = ui.NewToggle("Режим геймпад/клавиатурной навигации (SetGameNavigation)", false, func(v bool) {
		d.manager.SetGameNavigation(v)
		d.logf("SetGameNavigation = %v", v)
	})
	d.toggleSwitch.SetIcon(d.icons[1])

	d.checkBox = ui.NewCheckbox("Активировать контролы формы (SetEnabled / Indeterminate)", true, func(checked bool) {
		d.validField.UIState().SetEnabled(checked)
		d.sliderStep.UIState().SetEnabled(checked)
		d.spinBox.UIState().SetEnabled(checked)
		d.segmentedCtrl.UIState().SetEnabled(checked)
		d.comboBox.UIState().SetEnabled(checked)
		d.colorPicker.UIState().SetEnabled(checked)
		d.compactPicker.UIState().SetEnabled(checked)
		d.logf("Состояние активности контролов: %v", checked)
	})
	d.checkBox.SetIcon(d.icons[2])

	d.indeterminateTgl = ui.NewToggle("Режим Indeterminate (Checkbox + ProgressBar)", false, func(v bool) {
		d.checkBox.SetIndeterminate(v)
		d.progressBar.SetIndeterminate(v)
		if d.topToolbar != nil {
			d.topToolbar.SetItemActive("indeterminate", v)
		}
		d.logf("Indeterminate режим: %v", v)
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
			Label: fmt.Sprintf("Опция #%02d — DisplayScale %.3fx", i+1, scale),
			Icon:  d.icons[i%len(d.icons)],
		}
	}
	initScaleID := fmt.Sprintf("%.3f", d.manager.DisplayScale())
	d.comboBox = ui.NewComboBox(comboOpts, initScaleID, func(id string) {
		if val, err := strconv.ParseFloat(id, 32); err == nil {
			d.setDisplayScale(float32(val))
		}
	})
	d.comboBox.UIState().SetTooltip("ComboBox с прокруткой (16 элементов, MaxVisibleItems=8)")

	var pctLabels [21]string
	for i := 0; i <= 20; i++ {
		pctLabels[i] = fmt.Sprintf("%d%%", i*5)
	}
	d.sliderStep = ui.NewSlider("Прогресс (шаг 5%)", 0, 100, 35, 5, func(val float32) {
		if d.spinBox != nil && d.spinBox.Value != val {
			d.spinBox.SetValue(val)
		}
		d.progressBar.SetValue(val / 100.0)
	})
	d.sliderStep.FormatValue = func(v float32) string {
		idx := min(20, max(0, int(v/5+0.5)))
		return pctLabels[idx]
	}

	var radLabels [64]string
	for i := range radLabels {
		radLabels[i] = fmt.Sprintf("%.2f rad", float64(i)*0.1)
	}
	d.sliderCont = ui.NewSlider("Фаза волны Canvas (непрерывный)", 0, 6.28318, 0.8, 0.1, func(val float32) {
		if d.canvasWidget != nil {
			d.canvasWidget.phase = val
			d.manager.InvalidatePaint()
		}
	})
	d.sliderCont.FormatValue = func(v float32) string {
		idx := min(len(radLabels)-1, max(0, int(v*10+0.5)))
		return radLabels[idx]
	}

	d.progressBar = ui.NewProgressBar(0.35)
	d.progressBar.ShowValue = true

	// Accordion with Collapsible sections and Hyperlink
	accBody1 := ui.NewStack(ui.Vertical, 4)
	accLbl1 := ui.NewLabel("Секция Accordion #1: поддерживает клавиатуру (Enter/Space/Left/Right) и режим SingleOpen.")
	accLbl1.Wrap = true
	d.docHyperlink = ui.NewHyperlink("Открыть документацию nativeForms (github.com/banditmoscow1337/nativeForms)", "https://github.com/banditmoscow1337/nativeForms", func(url string) {
		d.manager.Notifyf("Переход по ссылке: %s", url)
		d.logf("Hyperlink активирован: %s", url)
	})
	accBody1.Add(accLbl1, d.docHyperlink)

	accBody2 := ui.NewLabel("Секция Accordion #2: при открытии этой секции первая автоматически сворачивается.")
	accBody2.Wrap = true

	sec1 := ui.NewCollapsible("1. Справка и быстрые ссылки (Collapsible + Hyperlink)", accBody1, true, nil)
	sec1.Icon = d.icons[0]
	sec2 := ui.NewCollapsible("2. Дополнительные сведения о контейнере Accordion", accBody2, false, nil)
	sec2.Icon = d.icons[1]
	d.accordion = ui.NewAccordion(true, sec1, sec2)

	// Demonstrate text clipping inside constrained bounds.
	d.overflowLabel = ui.NewLabel("Проверка обрезки длинного текста (Font 24px): этот текст гарантированно не выходит за границы виджета при любом размере")
	d.overflowLabel.Font = ui.FontSpec{Size: 22, Weight: 700}
	d.overflowLabel.Ellipsis = true
	d.overflowLabel.UIState().SetPreferredSize(ui.Vec2{X: 320, Y: 34})

	d.overflowBtn = ui.NewButton("Кнопка с очень длинным текстом (22px), который аккуратно обрезается внутри границ кнопки", func() {
		d.logf("Нажата кнопка проверки обрезки длинного текста")
	})
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
