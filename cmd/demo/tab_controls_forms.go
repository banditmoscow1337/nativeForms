package main

import ui "github.com/banditmoscow1337/nativeForms"

func (d *demoApp) buildControlsRightPane() *ui.ScrollPanel {
	rightStack := ui.NewStack(ui.Vertical, 8)
	rightStack.Padding = ui.All(8)

	formTitle := newSectionHeaderKey("ctrl.sec_right")

	d.colorPicker = ui.NewColorPicker("", d.darkTheme.Accent, func(c ui.Color) {
		d.applyPickedAccentColor(c)
	})
	d.colorPicker.SetTextKey("ctrl.colorpicker")
	d.colorPicker.SetCornerRadius(8)
	d.colorPicker.SetTooltipKey("ctrl.tip_colorpicker")

	// Use NewLocalizedForm so column 0 automatically sizes to the widest
	// registered translation across EN and RU via LocaleSizeMaxEnvelope.
	form := ui.NewLocalizedForm()

	d.compactPicker = ui.NewCompactColorPicker("", ui.RGBA(52, 120, 226, 255), func(c ui.Color) {
		if d.autoSizeLabel != nil {
			d.autoSizeLabel.Background = c.WithAlpha(0.45)
			d.manager.InvalidatePaint()
		}
		d.logf("Compact ColorPicker selected color: %s", c.HexAlpha())
	})
	d.compactPicker.SetTextKey("form.btn_popup_color")
	d.compactPicker.SetCornerRadius(6)
	d.compactPicker.SetTooltipKey("form.tip_popup_color")

	d.autoSizeField = ui.NewTextField("", func(s string) {
		d.logf("AutoSize TextField (%d chars): %q", len(s), s)
	})
	d.autoSizeField.SetTextKey("form.val_autofield")
	d.autoSizeField.SetPlaceholderKey("form.ph_autofield")
	d.autoSizeField.SetAutoSize(true)
	d.autoSizeField.SetCornerRadius(6)

	d.validField = ui.NewTextField("12345", func(s string) {
		d.logf("Digits field: %q", s)
	})
	d.validField.SetPlaceholderKey("form.ph_digits")
	d.validField.Validate = func(next string) bool {
		for _, r := range next {
			if r < '0' || r > '9' {
				return false
			}
		}
		return true
	}
	d.validField.OnSubmit = func(s string) {
		d.manager.Notifyf("Submitted value: %s", s)
	}

	d.passwordField = ui.NewTextField("secret2026", nil)
	d.passwordField.Password = true
	d.passwordField.SetPlaceholderKey("form.ph_password")

	d.readOnlyField = ui.NewTextField("", nil)
	d.readOnlyField.ReadOnly = true
	d.readOnlyField.SetTextKey("form.val_readonly")

	form.AddLocalizedRow("form.lbl_popup_color", d.compactPicker)
	form.AddLocalizedRow("form.lbl_autofield", d.autoSizeField)
	lblDigits := form.AddLocalizedRow("form.lbl_digits", d.validField)
	form.AddLocalizedRow("form.lbl_password", d.passwordField)
	form.AddLocalizedRow("form.lbl_readonly", d.readOnlyField)

	d.validField.UIState().SetAccessibility(ui.AccessibilityInfo{
		Role:        ui.RoleTextField,
		LabelledBy:  lblDigits,
		Description: d.tr("form.a11y_digits"),
	})

	d.notesArea = ui.NewTextArea("", func(s string) {
		d.tabs.SetDirty("controls", true)
	})
	d.notesArea.SetTextKey("form.val_notes")
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
	btnSwapImg := ui.NewButton("", func() {
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
		d.logf("ImageResource updated (Version=%d)", d.bannerImg.Version())
	})
	btnSwapImg.SetTextKey("ctrl.btn_swap_img")

	btnScrollTop := ui.NewButton("", func() {
		d.scrollForm.ScrollIntoView(d.btnCounter)
	})
	btnScrollTop.SetTextKey("ctrl.btn_scroll_top")
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
