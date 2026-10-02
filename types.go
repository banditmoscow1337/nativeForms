package nativeforms

import "math"

type Vec2 struct {
	X float32
	Y float32
}

type Rect struct {
	X float32
	Y float32
	W float32
	H float32
}

func (r Rect) Contains(x, y float32) bool {
	return x >= r.X && y >= r.Y && x < r.X+r.W && y < r.Y+r.H
}

func (r Rect) Empty() bool { return r.W <= 0 || r.H <= 0 }

func (r Rect) Inset(in Insets) Rect {
	return Rect{
		X: r.X + in.Left,
		Y: r.Y + in.Top,
		W: maxFloat32(0, r.W-in.Left-in.Right),
		H: maxFloat32(0, r.H-in.Top-in.Bottom),
	}
}

func intersect(a, b Rect) Rect {
	x0 := maxFloat32(a.X, b.X)
	y0 := maxFloat32(a.Y, b.Y)
	x1 := minFloat32(a.X+a.W, b.X+b.W)
	y1 := minFloat32(a.Y+a.H, b.Y+b.H)
	return Rect{X: x0, Y: y0, W: maxFloat32(0, x1-x0), H: maxFloat32(0, y1-y0)}
}

type Insets struct {
	Top    float32
	Right  float32
	Bottom float32
	Left   float32
}

func All(value float32) Insets {
	return Insets{Top: value, Right: value, Bottom: value, Left: value}
}

func Symmetric(horizontal, vertical float32) Insets {
	return Insets{Top: vertical, Right: horizontal, Bottom: vertical, Left: horizontal}
}

type Color struct {
	R float32
	G float32
	B float32
	A float32
}

func RGBA(r, g, b, a uint8) Color {
	const scale = 1.0 / 255.0
	return Color{
		R: float32(r) * scale,
		G: float32(g) * scale,
		B: float32(b) * scale,
		A: float32(a) * scale,
	}
}

func (c Color) WithAlpha(alpha float32) Color {
	c.A = clampFloat32(alpha, 0, 1)
	return c
}

var Transparent = Color{}

type Constraints struct {
	Min Vec2
	Max Vec2
	// A zero Max component is unbounded unless its Bounded flag is true.
	BoundedX, BoundedY bool
}

func (c Constraints) HasMaxX() bool { return c.BoundedX || c.Max.X>0 }
func (c Constraints) HasMaxY() bool { return c.BoundedY || c.Max.Y>0 }

func (constraints Constraints) Constrain(size Vec2) Vec2 {
	size.X = maxFloat32(size.X, constraints.Min.X)
	size.Y = maxFloat32(size.Y, constraints.Min.Y)
	if constraints.HasMaxX() {
		size.X = minFloat32(size.X, constraints.Max.X)
	}
	if constraints.HasMaxY() {
		size.Y = minFloat32(size.Y, constraints.Max.Y)
	}
	return size
}

type Direction uint8

const (
	Overlay Direction = iota
	Horizontal
	Vertical
)

type Align uint8

const (
	AlignStart Align = iota
	AlignCenter
	AlignEnd
	AlignStretch
	AlignBaseline
)

type Anchor uint8

const (
	AnchorTopLeft Anchor = iota
	AnchorTop
	AnchorTopRight
	AnchorLeft
	AnchorCenter
	AnchorRight
	AnchorBottomLeft
	AnchorBottom
	AnchorBottomRight
	AnchorFill
)

type TextAlign uint8

const (
	TextLeft TextAlign = iota
	TextCenter
	TextRight
)

type Theme struct {
	Text           Color
	MutedText      Color
	Panel          Color
	PanelAlt       Color
	Border         Color
	Accent         Color
	AccentHover    Color
	Control        Color
	ControlHover   Color
	ControlPressed Color
	Disabled       Color
	Danger         Color
	FontSize       float32
	ControlHeight  float32
}

func DefaultTheme() Theme {
	return Theme{
		Text:           RGBA(240, 242, 245, 255),
		MutedText:      RGBA(160, 168, 178, 255),
		Panel:          RGBA(15, 18, 24, 232),
		PanelAlt:       RGBA(25, 30, 39, 235),
		Border:         RGBA(72, 82, 98, 255),
		Accent:         RGBA(226, 174, 58, 255),
		AccentHover:    RGBA(245, 197, 82, 255),
		Control:        RGBA(40, 47, 59, 245),
		ControlHover:   RGBA(52, 61, 76, 250),
		ControlPressed: RGBA(31, 37, 47, 255),
		Disabled:       RGBA(75, 79, 86, 210),
		Danger:         RGBA(205, 68, 68, 255),
		FontSize:       18,
		ControlHeight:  44,
	}
}

func clampFloat32(value, low, high float32) float32 {
	return minFloat32(maxFloat32(value, low), high)
}

func maxFloat32(a, b float32) float32 {
	if a > b {
		return a
	}
	return b
}

func minFloat32(a, b float32) float32 {
	if a < b {
		return a
	}
	return b
}

func roundToStep(value, step float32) float32 {
	if step <= 0 {
		return value
	}
	return float32(math.Round(float64(value/step))) * step
}
