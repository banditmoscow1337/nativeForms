package nativeforms

type ImageFit uint8

const (
	ImageContain ImageFit = iota
	ImageCover
	ImageStretch
)

// Image displays an ImageResource inside its layout bounds.
type Image struct {
	State
	Source *ImageResource
	Fit    ImageFit
	Filter ImageFilter
	Tint   Color
}

func NewImage(source *ImageResource) *Image {
	widget := &Image{Source: source, Tint: RGBA(255, 255, 255, 255)}
	widget.owner = widget
	return widget
}

func (widget *Image) SetImage(source *ImageResource) {
	if widget.Source == source {
		return
	}
	widget.Source = source
	widget.invalidateLayout()
}

func (widget *Image) Measure(constraints Constraints) Vec2 {
	size := Vec2{}
	if widget.Source != nil {
		size = Vec2{X: float32(widget.Source.width), Y: float32(widget.Source.height)}
	}
	return measureWithState(&widget.State, size, constraints)
}

func (widget *Image) Paint(canvas *Canvas) {
	if widget.Source == nil || widget.rect.Empty() {
		return
	}
	source := Rect{W: float32(widget.Source.width), H: float32(widget.Source.height)}
	dest := widget.rect
	switch widget.Fit {
	case ImageContain:
		scale := minFloat32(dest.W/source.W, dest.H/source.H)
		width, height := source.W*scale, source.H*scale
		dest = Rect{X: dest.X + (dest.W-width)*0.5, Y: dest.Y + (dest.H-height)*0.5, W: width, H: height}
	case ImageCover:
		scale := maxFloat32(dest.W/source.W, dest.H/source.H)
		width, height := dest.W/scale, dest.H/scale
		source = Rect{X: (source.W - width) * 0.5, Y: (source.H - height) * 0.5, W: width, H: height}
	case ImageStretch:
		// The entire source fills the layout rectangle.
	default:
		return
	}
	canvas.PushClip(widget.rect)
	canvas.Image(widget.Source, dest, source, widget.Tint, widget.Filter)
	canvas.PopClip()
}
