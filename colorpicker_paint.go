package nativeforms

func (cp *ColorPicker) measureTextSize() Vec2 {
	theme := cp.State.Theme()
	spec := FontSpec{Size: theme.FontSize}
	titleW := cp.textWidth(cp.Text, spec)
	hexW := float32(0)
	if cp.ShowHex {
		hexW = cp.textWidth(cp.Hex(), spec) + 12
	}
	headerW := titleW + hexW + 64
	if cp.Compact {
		return Vec2{X: maxFloat32(110, headerW), Y: maxFloat32(theme.ControlHeight, theme.FontSize*1.2+14)}
	}
	height := float32(174)
	if cp.ShowAlpha {
		height += 20
	}
	if len(cp.Presets) > 0 {
		height += 26
	}
	return Vec2{X: maxFloat32(210, headerW), Y: height}
}

func (cp *ColorPicker) Measure(constraints Constraints) Vec2 {
	desired := cp.measureTextSize()
	if !cp.autoSize && !cp.Compact && desired.X < 260 {
		desired.X = 260
	}
	return measureWithState(&cp.State, desired, constraints)
}

func (cp *ColorPicker) layoutRegions() (header, sv, hueBar, alphaBar, swatchRow Rect) {
	inner := cp.rect.Inset(All(10))
	if cp.Compact {
		return inner, Rect{}, Rect{}, Rect{}, Rect{}
	}
	headerH := float32(26)
	header = Rect{X: inner.X, Y: inner.Y, W: inner.W, H: headerH}
	bottomH := float32(14) // hue bar
	if cp.ShowAlpha {
		bottomH += 20
	}
	if len(cp.Presets) > 0 {
		bottomH += 26
	}
	svH := maxFloat32(48, inner.H-headerH-bottomH-14)
	sv = Rect{X: inner.X, Y: header.Y + header.H + 6, W: inner.W, H: svH}
	curY := sv.Y + sv.H + 8
	hueBar = Rect{X: inner.X, Y: curY, W: inner.W, H: 12}
	curY += 18
	if cp.ShowAlpha {
		alphaBar = Rect{X: inner.X, Y: curY, W: inner.W, H: 12}
		curY += 18
	}
	if len(cp.Presets) > 0 {
		swatchRow = Rect{X: inner.X, Y: curY, W: inner.W, H: 20}
	}
	return header, sv, hueBar, alphaBar, swatchRow
}

func (cp *ColorPicker) Paint(canvas *Canvas) {
	if cp.rect.Empty() {
		return
	}
	radius := cp.effectiveCornerRadius(canvas.theme)
	if cp.Compact {
		paintControlSurface(canvas, &cp.State)
		inner := cp.rect.Inset(Symmetric(10, 4))
		if inner.Empty() {
			return
		}
		swatchSize := minFloat32(22, maxFloat32(12, inner.H-6))
		swatchRect := Rect{X: inner.X, Y: inner.Y + (inner.H-swatchSize)*0.5, W: swatchSize, H: swatchSize}
		swatchR := minFloat32(radius, 5)
		if chk := getCheckerStripResource(); chk != nil && cp.Value.A < 1 {
			canvas.PushClip(swatchRect)
			canvas.Image(chk, swatchRect, Rect{W: swatchSize, H: swatchSize}, RGBA(255, 255, 255, 255), FilterNearest)
			canvas.PopClip()
		}
		canvas.RoundedRect(swatchRect, swatchR, cp.Value)
		canvas.RoundedBorder(swatchRect, swatchR, 1, canvas.theme.Border)

		textColor := canvas.theme.Text
		if !cp.Enabled() {
			textColor = canvas.theme.MutedText
		}
		hexW := float32(0)
		if cp.ShowHex {
			hexStr := cp.Hex()
			hexW = cp.textWidth(hexStr, FontSpec{Size: canvas.theme.FontSize * 0.9}) + 24
			hexBounds := Rect{X: inner.X + inner.W - hexW, Y: inner.Y, W: hexW - 18, H: inner.H}
			canvas.Text(hexStr, hexBounds, canvas.theme.FontSize*0.9, TextRight, canvas.theme.AccentHover)
		}
		titleBounds := Rect{
			X: swatchRect.X + swatchRect.W + 8,
			Y: inner.Y,
			W: maxFloat32(0, inner.W-swatchRect.W-8-hexW),
			H: inner.H,
		}
		canvas.Text(cp.Text, titleBounds, canvas.theme.FontSize, TextLeft, textColor)
		paintChevron(canvas, Rect{X: inner.X + inner.W - 16, Y: inner.Y, W: 16, H: inner.H}, true, textColor)
		return
	}

	background := canvas.theme.PanelAlt
	border := canvas.theme.Border
	if !cp.Enabled() {
		background = canvas.theme.Disabled
	} else if cp.Focused() {
		border = canvas.theme.Accent
	}
	if !cp.Frameless() {
		canvas.RoundedRect(cp.rect, radius, background)
		canvas.RoundedBorder(cp.rect, radius, 1, border)
	}
	canvas.PushClip(cp.rect)
	defer canvas.PopClip()

	header, sv, hueBar, alphaBar, swatchRow := cp.layoutRegions()
	textColor := canvas.theme.Text
	if !cp.Enabled() {
		textColor = canvas.theme.MutedText
	}

	// 1. Header: swatch + label + hex code
	swatchRect := Rect{X: header.X, Y: header.Y + (header.H-20)*0.5, W: 28, H: 20}
	swatchR := minFloat32(radius, 5)
	if chk := getCheckerStripResource(); chk != nil && cp.Value.A < 1 {
		canvas.PushClip(swatchRect)
		canvas.Image(chk, swatchRect, Rect{W: 28, H: 20}, RGBA(255, 255, 255, 255), FilterNearest)
		canvas.PopClip()
	}
	canvas.RoundedRect(swatchRect, swatchR, cp.Value)
	canvas.RoundedBorder(swatchRect, swatchR, 1, canvas.theme.Border)

	hexW := float32(0)
	if cp.ShowHex {
		hexStr := cp.Hex()
		hexW = cp.textWidth(hexStr, FontSpec{Size: canvas.theme.FontSize * 0.9}) + 10
		hexBounds := Rect{X: header.X + header.W - hexW, Y: header.Y, W: hexW, H: header.H}
		canvas.Text(hexStr, hexBounds, canvas.theme.FontSize*0.9, TextRight, canvas.theme.AccentHover)
	}
	titleBounds := Rect{
		X: swatchRect.X + swatchRect.W + 8,
		Y: header.Y,
		W: maxFloat32(0, header.W-swatchRect.W-8-hexW),
		H: header.H,
	}
	canvas.Text(cp.Text, titleBounds, canvas.theme.FontSize, TextLeft, textColor)

	// 2. 2D Saturation-Value square
	if !sv.Empty() {
		if svRes := cp.ensureSVResource(); svRes != nil {
			canvas.Image(svRes, sv, Rect{}, RGBA(255, 255, 255, 255), FilterBilinear)
		}
		canvas.Border(sv, 1, canvas.theme.Border)
		cx := sv.X + clampFloat32(cp.sat, 0, 1)*sv.W
		cy := sv.Y + (1-clampFloat32(cp.val, 0, 1))*sv.H
		knobOuter := Rect{X: cx - 6, Y: cy - 6, W: 12, H: 12}
		knobInner := Rect{X: cx - 4, Y: cy - 4, W: 8, H: 8}
		canvas.RoundedBorder(knobOuter, 6, 2, RGBA(15, 18, 24, 230))
		canvas.RoundedBorder(knobInner, 4, 1.5, RGBA(255, 255, 255, 255))
	}

	// 3. Hue rainbow bar
	if !hueBar.Empty() {
		if hueRes := getHueStripResource(); hueRes != nil {
			canvas.Image(hueRes, hueBar, Rect{}, RGBA(255, 255, 255, 255), FilterBilinear)
		}
		canvas.Border(hueBar, 1, canvas.theme.Border)
		hx := hueBar.X + clampFloat32(cp.hue/360.0, 0, 1)*hueBar.W
		thumb := Rect{X: hx - 3, Y: hueBar.Y - 2, W: 6, H: hueBar.H + 4}
		canvas.RoundedRect(thumb, 2, RGBA(255, 255, 255, 255))
		canvas.RoundedBorder(thumb, 2, 1, RGBA(20, 24, 32, 255))
	}

	// 4. Optional Alpha bar
	if cp.ShowAlpha && !alphaBar.Empty() {
		if chk := getCheckerStripResource(); chk != nil {
			canvas.Image(chk, alphaBar, Rect{}, RGBA(255, 255, 255, 255), FilterNearest)
		}
		canvas.Rect(Rect{X: alphaBar.X, Y: alphaBar.Y, W: alphaBar.W * clampFloat32(cp.Value.A, 0, 1), H: alphaBar.H}, cp.Value.WithAlpha(1))
		canvas.Border(alphaBar, 1, canvas.theme.Border)
		ax := alphaBar.X + clampFloat32(cp.Value.A, 0, 1)*alphaBar.W
		thumb := Rect{X: ax - 3, Y: alphaBar.Y - 2, W: 6, H: alphaBar.H + 4}
		canvas.RoundedRect(thumb, 2, RGBA(255, 255, 255, 255))
		canvas.RoundedBorder(thumb, 2, 1, RGBA(20, 24, 32, 255))
	}

	// 5. Preset swatches row
	if len(cp.Presets) > 0 && !swatchRow.Empty() {
		n := len(cp.Presets)
		gap := float32(4)
		cellW := maxFloat32(12, (swatchRow.W-gap*float32(n-1))/float32(n))
		swR := minFloat32(radius, 4)
		for i, preset := range cp.Presets {
			x := swatchRow.X + float32(i)*(cellW+gap)
			if x+cellW > swatchRow.X+swatchRow.W+1 {
				break
			}
			cell := Rect{X: x, Y: swatchRow.Y, W: cellW, H: swatchRow.H}
			canvas.RoundedRect(cell, swR, preset)
			if sameRGB(preset, cp.Value) {
				canvas.RoundedBorder(cell, swR, 2, canvas.theme.Text)
			} else {
				canvas.RoundedBorder(cell, swR, 1, canvas.theme.Border)
			}
		}
	}
}

func (cp *ColorPicker) openPopup() bool {
	if cp.manager == nil {
		return false
	}
	if cp.popupID != 0 && cp.manager.ClosePopup(cp.popupID) {
		cp.popupID = 0
		return true
	}
	popupPicker := NewColorPicker(cp.Text, cp.Value, func(c Color) {
		cp.SetValue(c)
	})
	popupPicker.ShowAlpha = cp.ShowAlpha
	popupPicker.ShowHex = cp.ShowHex
	popupPicker.Presets = cp.Presets
	popupPicker.SetCornerRadius(cp.CornerRadius())
	w := maxFloat32(260, cp.rect.W)
	h := popupPicker.measureTextSize().Y
	cp.popupID = cp.manager.OpenPopup(popupPicker, Rect{X: cp.rect.X, Y: cp.rect.Y + cp.rect.H + 2, W: w, H: h}, false)
	return true
}

func (cp *ColorPicker) updateFromDrag(x, y float32) {
	_, sv, hueBar, alphaBar, _ := cp.layoutRegions()
	switch cp.dragZone {
	case colorDragSV:
		if !sv.Empty() {
			s := clampFloat32((x-sv.X)/maxFloat32(1, sv.W), 0, 1)
			v := 1 - clampFloat32((y-sv.Y)/maxFloat32(1, sv.H), 0, 1)
			cp.SetHSVA(cp.hue, s, v, cp.Value.A)
		}
	case colorDragHue:
		if !hueBar.Empty() {
			h := clampFloat32((x-hueBar.X)/maxFloat32(1, hueBar.W), 0, 1) * 359.9
			cp.SetHSVA(h, cp.sat, cp.val, cp.Value.A)
		}
	case colorDragAlpha:
		if !alphaBar.Empty() {
			a := clampFloat32((x-alphaBar.X)/maxFloat32(1, alphaBar.W), 0, 1)
			cp.SetHSVA(cp.hue, cp.sat, cp.val, a)
		}
	}
}

func (cp *ColorPicker) Handle(event Event) bool {
	if !cp.Enabled() {
		return false
	}
	if cp.Compact {
		switch event.Type {
		case PointerDown:
			return event.Button == MouseLeft
		case PointerUp:
			if event.Button == MouseLeft && cp.Pressed() && cp.rect.Contains(event.X, event.Y) {
				return cp.openPopup()
			}
		case KeyDown:
			if !event.Repeat && (event.Key == KeyEnter || event.Key == KeySpace || event.Key == KeyDownArrow) {
				return cp.openPopup()
			}
		}
		return cp.State.Handle(event)
	}

	switch event.Type {
	case PointerDown:
		if event.Button != MouseLeft || !cp.rect.Contains(event.X, event.Y) {
			return false
		}
		_, sv, hueBar, alphaBar, swatchRow := cp.layoutRegions()
		if sv.Contains(event.X, event.Y) {
			cp.dragZone = colorDragSV
			cp.updateFromDrag(event.X, event.Y)
			return true
		}
		if (Rect{X: hueBar.X - 4, Y: hueBar.Y - 4, W: hueBar.W + 8, H: hueBar.H + 8}).Contains(event.X, event.Y) {
			cp.dragZone = colorDragHue
			cp.updateFromDrag(event.X, event.Y)
			return true
		}
		if cp.ShowAlpha && (Rect{X: alphaBar.X - 4, Y: alphaBar.Y - 4, W: alphaBar.W + 8, H: alphaBar.H + 8}).Contains(event.X, event.Y) {
			cp.dragZone = colorDragAlpha
			cp.updateFromDrag(event.X, event.Y)
			return true
		}
		if len(cp.Presets) > 0 && swatchRow.Contains(event.X, event.Y) {
			n := len(cp.Presets)
			gap := float32(4)
			cellW := maxFloat32(12, (swatchRow.W-gap*float32(n-1))/float32(n))
			idx := int((event.X - swatchRow.X) / (cellW + gap))
			if idx >= 0 && idx < n {
				preset := cp.Presets[idx]
				if cp.ShowAlpha {
					preset.A = cp.Value.A
				}
				cp.SetValue(preset)
				return true
			}
		}
		return true
	case PointerMove:
		if cp.Pressed() && cp.dragZone != colorDragNone {
			cp.updateFromDrag(event.X, event.Y)
			return true
		}
	case PointerUp:
		if event.Button == MouseLeft && cp.dragZone != colorDragNone {
			cp.dragZone = colorDragNone
			return true
		}
	case KeyDown:
		mod := event.Mods&(ModControl|ModSuper) != 0
		if mod && event.Key == KeyC && cp.manager != nil && cp.manager.clipboardWrite != nil {
			_ = cp.manager.clipboardWrite(cp.Hex())
			return true
		}
		if mod && event.Key == KeyV && cp.manager != nil && cp.manager.clipboardRead != nil {
			if txt, err := cp.manager.clipboardRead(); err == nil {
				cp.SetHex(txt)
			}
			return true
		}
		step := float32(0.05)
		hueStep := float32(10)
		switch event.Key {
		case KeyLeftArrow:
			if event.Mods&ModShift != 0 {
				cp.SetHSVA(cp.hue-hueStep, cp.sat, cp.val, cp.Value.A)
			} else {
				cp.SetHSVA(cp.hue, cp.sat-step, cp.val, cp.Value.A)
			}
			return true
		case KeyRightArrow:
			if event.Mods&ModShift != 0 {
				cp.SetHSVA(cp.hue+hueStep, cp.sat, cp.val, cp.Value.A)
			} else {
				cp.SetHSVA(cp.hue, cp.sat+step, cp.val, cp.Value.A)
			}
			return true
		case KeyUpArrow:
			cp.SetHSVA(cp.hue, cp.sat, cp.val+step, cp.Value.A)
			return true
		case KeyDownArrow:
			cp.SetHSVA(cp.hue, cp.sat, cp.val-step, cp.Value.A)
			return true
		}
	}
	return cp.State.Handle(event)
}
