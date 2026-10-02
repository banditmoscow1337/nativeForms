package nativeforms

type ScrollPanel struct {
	State
	Content Component
	Offset  float32
	height  float32
}

func NewScrollPanel(content Component) *ScrollPanel {
	panel := &ScrollPanel{Content: content}
	panel.owner = panel
	panel.clip = true
	if content != nil { panel.Add(content) }
	return panel
}

func (panel *ScrollPanel) Measure(constraints Constraints) Vec2 {
	if panel.Content == nil { return panel.State.Measure(constraints) }
	content := panel.Content.Measure(Constraints{Max: Vec2{X: constraints.Max.X}})
	return measureWithState(&panel.State, content, constraints)
}

func (panel *ScrollPanel) Arrange(rect Rect) {
	panel.rect = rect
	if panel.Content == nil { return }
	size := panel.Content.Measure(Constraints{Max: Vec2{X: rect.W}})
	panel.height = size.Y
	panel.Offset = clampFloat32(panel.Offset, 0, maxFloat32(0, panel.height-rect.H))
	panel.Content.Arrange(Rect{X: rect.X, Y: rect.Y - panel.Offset, W: rect.W, H: maxFloat32(rect.H, panel.height)})
}

func (panel *ScrollPanel) Scroll(pixels float32) {
	before := panel.Offset
	panel.Offset = clampFloat32(panel.Offset+pixels, 0, maxFloat32(0, panel.height-panel.rect.H))
	if panel.Offset != before {
		panel.Arrange(panel.rect)
		panel.invalidatePaint()
	}
}

func (panel *ScrollPanel) Handle(event Event) bool {
	if event.Type == PointerScroll {
		before := panel.Offset
		panel.Scroll(-event.Scroll.Y * 40)
		if panel.Offset != before { return true }
	}
	return panel.State.Handle(event)
}
