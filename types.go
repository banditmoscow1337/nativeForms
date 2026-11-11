package nativeforms

import (
	"fmt"
	"math"
	"strings"
)

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

// RGBA8 converts normalized float channels to 8-bit [0..255] values.
func (c Color) RGBA8() (r, g, b, a uint8) {
	r = uint8(clampFloat32(c.R, 0, 1)*255 + 0.5)
	g = uint8(clampFloat32(c.G, 0, 1)*255 + 0.5)
	b = uint8(clampFloat32(c.B, 0, 1)*255 + 0.5)
	a = uint8(clampFloat32(c.A, 0, 1)*255 + 0.5)
	return r, g, b, a
}

// Hex formats the RGB channels as #RRGGBB.
func (c Color) Hex() string {
	r, g, b, _ := c.RGBA8()
	return fmt.Sprintf("#%02X%02X%02X", r, g, b)
}

// HexAlpha formats the RGBA channels as #RRGGBBAA.
func (c Color) HexAlpha() string {
	r, g, b, a := c.RGBA8()
	return fmt.Sprintf("#%02X%02X%02X%02X", r, g, b, a)
}

// HSV constructs an opaque Color from hue [0..360), saturation [0..1], and value [0..1].
func HSV(h, s, v float32) Color {
	return HSVA(h, s, v, 1)
}

// HSVA constructs a Color from hue [0..360), saturation [0..1], value [0..1], and alpha [0..1].
func HSVA(h, s, v, a float32) Color {
	s = clampFloat32(s, 0, 1)
	v = clampFloat32(v, 0, 1)
	a = clampFloat32(a, 0, 1)
	if math.IsNaN(float64(h)) || math.IsInf(float64(h), 0) {
		h = 0
	}
	h = float32(math.Mod(float64(h), 360))
	if h < 0 {
		h += 360
	}
	if s <= 0 {
		return Color{R: v, G: v, B: v, A: a}
	}
	sector := h / 60
	i := int(sector)
	f := sector - float32(i)
	p := v * (1 - s)
	q := v * (1 - s*f)
	t := v * (1 - s*(1-f))
	switch i % 6 {
	case 0:
		return Color{R: v, G: t, B: p, A: a}
	case 1:
		return Color{R: q, G: v, B: p, A: a}
	case 2:
		return Color{R: p, G: v, B: t, A: a}
	case 3:
		return Color{R: p, G: q, B: v, A: a}
	case 4:
		return Color{R: t, G: p, B: v, A: a}
	default:
		return Color{R: v, G: p, B: q, A: a}
	}
}

// ColorToHSV converts a Color into hue [0..360), saturation [0..1], and value [0..1].
func ColorToHSV(c Color) (h, s, v float32) {
	r := clampFloat32(c.R, 0, 1)
	g := clampFloat32(c.G, 0, 1)
	b := clampFloat32(c.B, 0, 1)
	maxC := maxFloat32(r, maxFloat32(g, b))
	minC := minFloat32(r, minFloat32(g, b))
	delta := maxC - minC
	v = maxC
	if maxC > 0 {
		s = delta / maxC
	}
	if delta <= 1e-6 {
		return 0, s, v
	}
	switch maxC {
	case r:
		h = 60 * ((g - b) / delta)
		if h < 0 {
			h += 360
		}
	case g:
		h = 60*(((b-r)/delta)+2)
	default:
		h = 60*(((r-g)/delta)+4)
	}
	return h, s, v
}

func parseHexNibble(b byte) (uint8, bool) {
	switch {
	case b >= '0' && b <= '9':
		return b - '0', true
	case b >= 'a' && b <= 'f':
		return b - 'a' + 10, true
	case b >= 'A' && b <= 'F':
		return b - 'A' + 10, true
	default:
		return 0, false
	}
}

func parseHexByte(hi, lo byte) (uint8, bool) {
	h, ok1 := parseHexNibble(hi)
	l, ok2 := parseHexNibble(lo)
	return (h << 4) | l, ok1 && ok2
}

// ParseHexColor parses #RGB, #RGBA, #RRGGBB, or #RRGGBBAA (with or without '#').
func ParseHexColor(s string) (Color, bool) {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "#")
	switch len(s) {
	case 3:
		r, ok1 := parseHexNibble(s[0])
		g, ok2 := parseHexNibble(s[1])
		b, ok3 := parseHexNibble(s[2])
		if !ok1 || !ok2 || !ok3 {
			return Color{}, false
		}
		return RGBA(r*17, g*17, b*17, 255), true
	case 4:
		r, ok1 := parseHexNibble(s[0])
		g, ok2 := parseHexNibble(s[1])
		b, ok3 := parseHexNibble(s[2])
		a, ok4 := parseHexNibble(s[3])
		if !ok1 || !ok2 || !ok3 || !ok4 {
			return Color{}, false
		}
		return RGBA(r*17, g*17, b*17, a*17), true
	case 6:
		r, ok1 := parseHexByte(s[0], s[1])
		g, ok2 := parseHexByte(s[2], s[3])
		b, ok3 := parseHexByte(s[4], s[5])
		if !ok1 || !ok2 || !ok3 {
			return Color{}, false
		}
		return RGBA(r, g, b, 255), true
	case 8:
		r, ok1 := parseHexByte(s[0], s[1])
		g, ok2 := parseHexByte(s[2], s[3])
		b, ok3 := parseHexByte(s[4], s[5])
		a, ok4 := parseHexByte(s[6], s[7])
		if !ok1 || !ok2 || !ok3 || !ok4 {
			return Color{}, false
		}
		return RGBA(r, g, b, a), true
	default:
		return Color{}, false
	}
}

var Transparent = Color{}

type Constraints struct {
	Min                Vec2
	Max                Vec2
	BoundedX, BoundedY bool
}

func (c Constraints) HasMaxX() bool { return c.BoundedX || c.Max.X > 0 }
func (c Constraints) HasMaxY() bool { return c.BoundedY || c.Max.Y > 0 }

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
	Selection      Color
	Tooltip        Color
	Scrim          Color
	Header         Color
	FontSize       float32
	ControlHeight  float32
	CornerRadius   float32
}

func DefaultTheme() Theme {
	return Theme{
		Text:           RGBA(240, 242, 245, 255),
		MutedText:      RGBA(160, 168, 178, 255),
		Panel:          RGBA(15, 18, 24, 255),
		PanelAlt:       RGBA(25, 30, 39, 255),
		Border:         RGBA(72, 82, 98, 255),
		Accent:         RGBA(226, 174, 58, 255),
		AccentHover:    RGBA(245, 197, 82, 255),
		Control:        RGBA(40, 47, 59, 255),
		ControlHover:   RGBA(52, 61, 76, 255),
		ControlPressed: RGBA(31, 37, 47, 255),
		Disabled:       RGBA(75, 79, 86, 210),
		Danger:         RGBA(205, 68, 68, 255),
		Selection:      RGBA(63, 86, 119, 255),
		Tooltip:        RGBA(37, 43, 54, 255),
		Scrim:          RGBA(0, 0, 0, 150),
		Header:         RGBA(31, 38, 49, 255),
		FontSize:       18,
		ControlHeight:  44,
	}
}

func LightTheme() Theme {
	return Theme{
		Text:           RGBA(24, 28, 36, 255),
		MutedText:      RGBA(92, 102, 118, 255),
		Panel:          RGBA(244, 246, 250, 255),
		PanelAlt:       RGBA(230, 234, 242, 255),
		Border:         RGBA(175, 184, 198, 255),
		Accent:         RGBA(34, 108, 214, 255),
		AccentHover:    RGBA(22, 88, 186, 255),
		Control:        RGBA(255, 255, 255, 255),
		ControlHover:   RGBA(235, 241, 252, 255),
		ControlPressed: RGBA(216, 226, 244, 255),
		Disabled:       RGBA(200, 205, 214, 210),
		Danger:         RGBA(205, 52, 52, 255),
		Selection:      RGBA(188, 214, 252, 255),
		Tooltip:        RGBA(255, 253, 235, 255),
		Scrim:          RGBA(15, 20, 30, 110),
		Header:         RGBA(218, 224, 235, 255),
		FontSize:       18,
		ControlHeight:  44,
	}
}

func (t Theme) IsLight() bool {
	return t.Panel.R*0.299+t.Panel.G*0.587+t.Panel.B*0.114 > 0.5
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
