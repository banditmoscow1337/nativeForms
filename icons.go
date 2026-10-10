package nativeforms

// Icons occupy a fixed logical slot so text remains aligned across rows with
// different source image sizes. The resource itself is never resized or copied.
const iconSize float32 = 20
const iconGap float32 = 8

func iconWidth(icon *ImageResource) float32 {
	if icon != nil {
		return iconSize + iconGap
	}
	return 0
}

func paintIcon(canvas *Canvas, icon *ImageResource, slot Rect, opacity float32) {
	if icon == nil || slot.Empty() || opacity <= 0 {
		return
	}
	scale := minFloat32(slot.W/float32(icon.width), slot.H/float32(icon.height))
	w, h := float32(icon.width)*scale, float32(icon.height)*scale
	dest := Rect{X: slot.X + (slot.W-w)*0.5, Y: slot.Y + (slot.H-h)*0.5, W: w, H: h}
	canvas.PushClip(slot)
	canvas.Image(icon, dest, Rect{}, RGBA(255, 255, 255, 255).WithAlpha(opacity), FilterBilinear)
	canvas.PopClip()
}

// paintIconLabel reserves the icon column even when a particular row has no
// icon, keeping labels aligned in mixed lists and menus.
func paintIconLabel(canvas *Canvas, icon *ImageResource, label string, bounds Rect, reserve bool, tint Color, opacity float32) {
	if reserve {
		size := minFloat32(iconSize, minFloat32(bounds.W, bounds.H))
		paintIcon(canvas, icon, Rect{X: bounds.X, Y: bounds.Y + (bounds.H-size)*0.5, W: size, H: size}, opacity)
		advance := iconSize + iconGap
		bounds.X += advance
		bounds.W = maxFloat32(0, bounds.W-advance)
	}
	if bounds.Empty() || label == "" {
		return
	}
	canvas.PushClip(bounds)
	canvas.Text(label, bounds, canvas.theme.FontSize, TextLeft, tint)
	canvas.PopClip()
}

func paintChevron(canvas *Canvas, bounds Rect, down bool, tint Color) {
	if bounds.Empty() {
		return
	}
	half := minFloat32(bounds.W, bounds.H) * 0.25
	center := Vec2{X: bounds.X + bounds.W*0.5, Y: bounds.Y + bounds.H*0.5}
	if down {
		tip := Vec2{X: center.X, Y: center.Y + half*0.5}
		canvas.Line(Vec2{X: center.X - half, Y: center.Y - half*0.5}, tip, 1.5, tint)
		canvas.Line(tip, Vec2{X: center.X + half, Y: center.Y - half*0.5}, 1.5, tint)
		return
	}
	tip := Vec2{X: center.X + half*0.5, Y: center.Y}
	canvas.Line(Vec2{X: center.X - half*0.5, Y: center.Y - half}, tip, 1.5, tint)
	canvas.Line(tip, Vec2{X: center.X - half*0.5, Y: center.Y + half}, 1.5, tint)
}
