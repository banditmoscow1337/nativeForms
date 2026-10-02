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
	panel.Add(content)
	return panel
}

func (panel *ScrollPanel) Arrange(rect Rect) {
	panel.rect = rect
	size := panel.Content.Measure(Constraints{Max: Vec2{X: rect.W}})
	panel.height = size.Y
	panel.Offset = clampFloat32(panel.Offset, 0, maxFloat32(0, panel.height-rect.H))
	panel.Content.Arrange(Rect{X: rect.X, Y: rect.Y - panel.Offset, W: rect.W, H: maxFloat32(rect.H, panel.height)})
}

func (panel *ScrollPanel) Scroll(pixels float32) {
	panel.Offset = clampFloat32(panel.Offset+pixels, 0, maxFloat32(0, panel.height-panel.rect.H))
	panel.Arrange(panel.rect)
}

func (panel *ScrollPanel) Handle(event Event) bool {
	if event.Type == PointerScroll {
		panel.Scroll(-event.Scroll.Y * 40)
		return true
	}
	return panel.State.Handle(event)
}
