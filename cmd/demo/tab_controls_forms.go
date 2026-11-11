package main

import ui "github.com/banditmoscow1337/nativeForms"

func (d *demoApp) buildControlsRightPane() *ui.ScrollPanel {
	rightStack := ui.NewStack(ui.Vertical, 8)
	rightStack.Padding = ui.All(8)

	formTitle := newSectionHeader("Виджет ColorPicker (HSV/Alpha/Presets + Popup), Формы и Графика")

	d.colorPicker = ui.NewColorPicker("Акцентный цвет (ColorPicker HSV + Alpha):", d.darkTheme.Accent, func(c ui.Color) {
		d.applyPickedAccentColor(c)
	})
	d.colorPicker.SetCornerRadius(8)
	d.colorPicker.UIState().SetTooltip("Интерактивный ColorPicker: квадрат Saturation/Value, полосы Hue и Alpha, пресеты и копирование Hex (Ctrl+C / Ctrl+V)")

	form := ui.NewForm(155)
	form.ColumnGap = 8
	form.RowGap = 6

	lblCompactColor := ui.NewLabel("Цвет (Popup):")
	d.compactPicker = ui.NewCompactColorPicker("Выбрать цвет", ui.RGBA(52, 120, 226, 255), func(c ui.Color) {
		if d.autoSizeLabel != nil {
			d.autoSizeLabel.Background = c.WithAlpha(0.45)
			d.manager.InvalidatePaint()
		}
		d.logf("Compact ColorPicker выбрал цвет: %s", c.HexAlpha())
	})
	d.compactPicker.SetCornerRadius(6)
	d.compactPicker.UIState().SetTooltip("Компактный ColorPicker: открывает всплывающий HSV-пикер по клику или Enter/Space")

	lblAutoField := ui.NewLabel("Авто-поле ввода:")
	d.autoSizeField = ui.NewTextField("Наберите текст — поле растет!", func(s string) {
		d.logf("AutoSize TextField (%d симв.): %q", len(s), s)
	})
	d.autoSizeField.Placeholder = "Авторазмер..."
	d.autoSizeField.SetAutoSize(true)
	d.autoSizeField.SetCornerRadius(6)

	lblDigits := ui.NewLabel("Только цифры:")
	d.validField = ui.NewTextField("12345", func(s string) {
		d.logf("Цифровое поле: %q", s)
	})
	d.validField.Placeholder = "Введите число..."
	d.validField.Validate = func(next string) bool {
		for _, r := range next {
			if r < '0' || r > '9' {
				return false
			}
		}
		return true
	}
	d.validField.OnSubmit = func(s string) {
		d.manager.Notifyf("Отправлено значение: %s", s)
	}
	d.validField.UIState().SetAccessibility(ui.AccessibilityInfo{
		Role:        ui.RoleTextField,
		LabelledBy:  lblDigits,
		Description: "Принимает только цифры 0-9",
	})

	lblPass := ui.NewLabel("Пароль (Password):")
	d.passwordField = ui.NewTextField("secret2026", nil)
	d.passwordField.Password = true
	d.passwordField.Placeholder = "Пароль..."

	lblRO := ui.NewLabel("Только чтение:")
	d.readOnlyField = ui.NewTextField("ReadOnly TextField (нельзя изменить)", nil)
	d.readOnlyField.ReadOnly = true

	form.AddRow(lblCompactColor, d.compactPicker)
	form.AddRow(lblAutoField, d.autoSizeField)
	form.AddRow(lblDigits, d.validField)
	form.AddRow(lblPass, d.passwordField)
	form.AddRow(lblRO, d.readOnlyField)

	d.notesArea = ui.NewTextArea("Многострочное поле TextArea (Undo/Redo, выделение слов, автоперенос).\nВторая строка с кириллицей и UTF-8 символами: № € ₽ — «тест».", func(s string) {
		d.tabs.SetDirty("controls", true)
	})
	d.notesArea.Editor().Limit = 64
	d.notesArea.Editor().UndoByteLimit = 2 << 20
	d.notesArea.UIState().SetPreferredSize(ui.Vec2{X: 360, Y: 90})

	d.dynamicGrid = ui.NewGrid(
		ui.Track{Mode: ui.TrackFixed, Value: 150},
		ui.Track{Mode: ui.TrackStar, Value: 1},
	)
	d.dynamicGrid.ColumnGap = 8
	d.dynamicGrid.RowGap = 6

	overlayContainer := ui.NewPanel(ui.Overlay)
	overlayContainer.UIState().SetPreferredSize(ui.Vec2{X: 150, Y: 84})
	overlayContainer.Border = d.darkTheme.Border
	overlayContainer.BorderWidth = 1
	overlayContainer.CornerRadius = 6

	d.imageWidget = ui.NewImage(d.bannerImg)
	d.imageWidget.UIState().SetAnchor(ui.AnchorFill)

	d.overlayBadge = ui.NewLabel("OVERLAY")
	d.overlayBadge.Font = ui.FontSpec{Size: 12, Weight: 700}
	d.overlayBadge.Background = d.darkTheme.Accent.WithAlpha(0.85)
	d.overlayBadge.Color = ui.RGBA(15, 18, 24, 255)
	d.overlayBadge.Padding = ui.Symmetric(6, 2)
	d.overlayBadge.SetCornerRadius(4)
	d.overlayBadge.UIState().SetAnchor(ui.AnchorBottomRight)
	d.overlayBadge.UIState().SetOffset(ui.Vec2{X: -4, Y: -4})
	overlayContainer.Add(d.imageWidget, d.overlayBadge)

	imgActions := ui.NewStack(ui.Vertical, 4)
	btnSwapImg := ui.NewButton("ImageResource.Replace / SetFilter", func() {
		d.usingAltImg = !d.usingAltImg
		if d.usingAltImg {
			if replaced, err := d.bannerImg.Replace(d.altBanner); err == nil {
				d.bannerImg = replaced
				d.imageWidget.SetImage(replaced)
			}
			d.imageWidget.SetFilter(ui.FilterNearest)
			d.imageWidget.SetTint(ui.RGBA(230, 255, 230, 255))
		} else {
			d.imageWidget.SetFilter(ui.FilterBilinear)
			d.imageWidget.SetTint(ui.RGBA(255, 255, 255, 255))
		}
		d.logf("Обновлён ресурс изображения (Version=%d)", d.bannerImg.Version())
	})
	btnScrollTop := ui.NewButton("ScrollIntoView (к счётчику)", func() {
		d.scrollForm.ScrollIntoView(d.btnCounter)
	})
	imgActions.Add(btnSwapImg, btnScrollTop)

	d.dynamicGrid.AddAt(overlayContainer, 0, 0, 1, 1)
	d.dynamicGrid.AddAt(imgActions, 0, 1, 1, 1)

	d.canvasWidget = newCustomCanvasWidget(d.icons[1])
	d.canvasWidget.SetCornerRadius(8)

	rightStack.Add(formTitle, d.colorPicker, ui.NewSeparator(), form, d.notesArea, d.dynamicGrid, d.canvasWidget)
	rightScroll := ui.NewScrollPanel(rightStack)
	rightScroll.UIState().SetFlex(1)

	d.framelessControls = []ui.Component{
		d.topToolbar, d.btnCounter, d.statusBadge, d.spinBox, d.segmentedCtrl,
		d.radiusSlider, d.autoSizeToggle, d.autoSizeBtn, d.autoSizeLabel,
		d.toggleSwitch, d.checkBox, d.indeterminateTgl, d.radioGroup, d.comboBox,
		d.sliderStep, d.sliderCont, d.progressBar, d.overflowBtn, d.colorPicker,
		d.compactPicker, d.autoSizeField, d.validField, d.passwordField,
		d.readOnlyField, d.notesArea, d.canvasWidget,
	}

	d.roundedControls = []ui.Component{
		d.topToolbar, d.btnCounter, d.spinBox, d.segmentedCtrl, d.radiusSlider,
		d.autoSizeToggle, d.autoSizeBtn, d.autoSizeLabel, d.framelessCtrlTgl,
		d.toggleSwitch, d.checkBox, d.indeterminateTgl, d.radioGroup, d.comboBox,
		d.sliderStep, d.sliderCont, d.progressBar, d.overflowBtn, d.colorPicker,
		d.compactPicker, d.autoSizeField, d.validField, d.passwordField,
		d.readOnlyField, d.notesArea, d.canvasWidget, btnSwapImg, btnScrollTop,
	}

	d.autoSizeControls = []ui.Component{
		d.autoSizeBtn, d.autoSizeLabel, d.autoSizeField, d.statusBadge,
	}
	return rightScroll
}
