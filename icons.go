package nativeforms

// paintChevron uses the same line commands on software and Vulkan backends.
// Portable fonts do not need arrow glyphs for ComboBox or VirtualTree.
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
