package nativeforms

import (
	"image"
	"image/color"
	"math"
	"sync"
)

type colorDragZone uint8

const (
	colorDragNone colorDragZone = iota
	colorDragSV
	colorDragHue
	colorDragAlpha
)

var (
	sharedHueOnce  sync.Once
	sharedHueStrip *ImageResource
	sharedChkOnce  sync.Once
	sharedChkStrip *ImageResource
)

func getHueStripResource() *ImageResource {
	sharedHueOnce.Do(func() {
		const w, h = 96, 2
		img := image.NewNRGBA(image.Rect(0, 0, w, h))
		for x := 0; x < w; x++ {
			hue := float32(x) * 360.0 / float32(w-1)
			c := HSV(hue, 1, 1)
			r, g, b, _ := c.RGBA8()
			px := color.NRGBA{R: r, G: g, B: b, A: 255}
			img.SetNRGBA(x, 0, px)
			img.SetNRGBA(x, 1, px)
		}
		sharedHueStrip, _ = NewImageResource(img)
	})
	return sharedHueStrip
}

func getCheckerStripResource() *ImageResource {
	sharedChkOnce.Do(func() {
		const w, h = 64, 8
		img := image.NewNRGBA(image.Rect(0, 0, w, h))
		c1 := color.NRGBA{R: 195, G: 200, B: 208, A: 255}
		c2 := color.NRGBA{R: 120, G: 126, B: 136, A: 255}
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				if ((x/4)+(y/4))%2 == 0 {
					img.SetNRGBA(x, y, c1)
				} else {
					img.SetNRGBA(x, y, c2)
				}
			}
		}
		sharedChkStrip, _ = NewImageResource(img)
	})
	return sharedChkStrip
}

func defaultColorPresets() []Color {
	return []Color{
		RGBA(230, 72, 72, 255),
		RGBA(235, 145, 52, 255),
		RGBA(226, 174, 58, 255),
		RGBA(72, 196, 118, 255),
		RGBA(54, 178, 205, 255),
		RGBA(52, 120, 226, 255),
		RGBA(148, 88, 224, 255),
		RGBA(220, 82, 168, 255),
		RGBA(240, 242, 245, 255),
		RGBA(40, 47, 59, 255),
	}
}

const svCacheSlots = 32

type svCacheEntry struct {
	qHue int
	res  *ImageResource
}

// ColorPicker is a focusable widget for selecting RGBA/HSVA colors.
// When Compact is false, it renders an interactive 2D Saturation-Value square,
// Hue bar, optional Alpha bar, preset color swatches, and hex indicator.
// When Compact is true, it renders a single-line color swatch button that opens
// a popup ColorPicker when activated.
type ColorPicker struct {
	State
	Text      string
	Value     Color
	Presets   []Color
	ShowAlpha bool
	ShowHex   bool
	Compact   bool
	OnChange  func(Color)

	hue, sat, val float32
	dragZone      colorDragZone
	svImg         *image.RGBA
	svRes         *ImageResource
	svRenderedHue int
	svCache       [svCacheSlots]svCacheEntry
	svCacheCount  int
	svCacheHand   int
	cachedRGBA    [4]uint8
	cachedWithA   bool
	cachedHex     string
	popupID       PopupID
}

func NewColorPicker(text string, initial Color, onChange func(Color)) *ColorPicker {
	if initial.A <= 0 && initial.R == 0 && initial.G == 0 && initial.B == 0 {
		initial = RGBA(226, 174, 58, 255)
	}
	h, s, v := ColorToHSV(initial)
	cp := &ColorPicker{
		Text:          text,
		Value:         initial,
		Presets:       defaultColorPresets(),
		ShowAlpha:     true,
		ShowHex:       true,
		OnChange:      onChange,
		hue:           h,
		sat:           s,
		val:           v,
		svRenderedHue: -1,
	}
	cp.owner = cp
	cp.focusable = true
	cp.minimum = Vec2{X: 240}
	return cp
}

func NewCompactColorPicker(text string, initial Color, onChange func(Color)) *ColorPicker {
	cp := NewColorPicker(text, initial, onChange)
	cp.Compact = true
	cp.minimum = Vec2{X: 180}
	return cp
}

func (cp *ColorPicker) SetText(text string) {
	if cp.Text == text {
		return
	}
	cp.Text = text
	cp.invalidateLayout()
}

func (cp *ColorPicker) SetCompact(compact bool) {
	if cp.Compact == compact {
		return
	}
	cp.Compact = compact
	cp.invalidateLayout()
}

func (cp *ColorPicker) SetShowAlpha(show bool) {
	if cp.ShowAlpha == show {
		return
	}
	cp.ShowAlpha = show
	cp.cachedHex = ""
	cp.invalidateLayout()
}

// SetValue updates the selected color and synchronizes HSV components.
func (cp *ColorPicker) SetValue(c Color) {
	cp.setValueInternal(c, true, true)
}

// SetColor is an alias for SetValue.
func (cp *ColorPicker) SetColor(c Color) {
	cp.SetValue(c)
}

func (cp *ColorPicker) setValueInternal(c Color, syncHSV, notify bool) {
	c.R = clampFloat32(c.R, 0, 1)
	c.G = clampFloat32(c.G, 0, 1)
	c.B = clampFloat32(c.B, 0, 1)
	c.A = clampFloat32(c.A, 0, 1)
	if syncHSV {
		h, s, v := ColorToHSV(c)
		if s > 1e-4 {
			cp.hue = h
		}
		if v > 1e-4 {
			cp.sat = s
		}
		cp.val = v
	}
	if cp.Value == c {
		return
	}
	cp.Value = c
	cp.cachedHex = ""
	if cp.autoSize {
		cp.invalidateLayout()
	} else {
		cp.InvalidatePaint()
	}
	if notify && cp.OnChange != nil {
		cp.OnChange(c)
	}
}

func (cp *ColorPicker) SetHSV(h, s, v float32) {
	cp.SetHSVA(h, s, v, cp.Value.A)
}

func (cp *ColorPicker) SetHSVA(h, s, v, a float32) {
	if math.IsNaN(float64(h)) || math.IsInf(float64(h), 0) {
		h = 0
	}
	h = float32(math.Mod(float64(h), 360))
	if h < 0 {
		h += 360
	}
	s = clampFloat32(s, 0, 1)
	v = clampFloat32(v, 0, 1)
	a = clampFloat32(a, 0, 1)
	cp.hue, cp.sat, cp.val = h, s, v
	cp.setValueInternal(HSVA(h, s, v, a), false, true)
}

func (cp *ColorPicker) HSV() (h, s, v float32) {
	return cp.hue, cp.sat, cp.val
}

func (cp *ColorPicker) HSVA() (h, s, v, a float32) {
	return cp.hue, cp.sat, cp.val, cp.Value.A
}

func (cp *ColorPicker) Hex() string {
	r, g, b, a := cp.Value.RGBA8()
	rgba := [4]uint8{r, g, b, a}
	withA := cp.ShowAlpha && a < 255
	if cp.cachedHex != "" && cp.cachedRGBA == rgba && cp.cachedWithA == withA {
		return cp.cachedHex
	}
	cp.cachedRGBA = rgba
	cp.cachedWithA = withA
	if withA {
		cp.cachedHex = cp.Value.HexAlpha()
	} else {
		cp.cachedHex = cp.Value.Hex()
	}
	return cp.cachedHex
}

func (cp *ColorPicker) SetHex(hex string) bool {
	parsed, ok := ParseHexColor(hex)
	if !ok {
		return false
	}
	if !cp.ShowAlpha {
		parsed.A = 1
	}
	cp.SetValue(parsed)
	return true
}

func fillSVPixels(pix []byte, stride int, size int, hue float32) {
	pure := HSV(hue, 1, 1)
	for y := 0; y < size; y++ {
		v := 1 - float32(y)/float32(size-1)
		for x := 0; x < size; x++ {
			s := float32(x) / float32(size-1)
			r := v * ((1 - s) + s*pure.R)
			g := v * ((1 - s) + s*pure.G)
			b := v * ((1 - s) + s*pure.B)
			off := y*stride + x*4
			pix[off+0] = uint8(r*255 + 0.5)
			pix[off+1] = uint8(g*255 + 0.5)
			pix[off+2] = uint8(b*255 + 0.5)
			pix[off+3] = 255
		}
	}
}

func (cp *ColorPicker) ensureSVResource() *ImageResource {
	qHue := int(cp.hue*4 + 0.5)
	if cp.svRes != nil && cp.svRenderedHue == qHue {
		return cp.svRes
	}
	for i := 0; i < cp.svCacheCount; i++ {
		if cp.svCache[i].qHue == qHue && cp.svCache[i].res != nil {
			cp.svRes = cp.svCache[i].res
			cp.svRenderedHue = qHue
			return cp.svRes
		}
	}
	const size = 16
	if cp.svCacheCount < svCacheSlots {
		if cp.svImg == nil {
			cp.svImg = image.NewRGBA(image.Rect(0, 0, size, size))
		}
		fillSVPixels(cp.svImg.Pix, cp.svImg.Stride, size, cp.hue)
		res, _ := NewImageResource(cp.svImg)
		idx := cp.svCacheCount
		cp.svCacheCount++
		cp.svCache[idx] = svCacheEntry{qHue: qHue, res: res}
		cp.svRes = res
		cp.svRenderedHue = qHue
		return cp.svRes
	}
	// Reuse the oldest slot in-place without allocating a new pixel slice.
	slot := &cp.svCache[cp.svCacheHand]
	cp.svCacheHand = (cp.svCacheHand + 1) % svCacheSlots
	res := slot.res
	fillSVPixels(res.pixels, size*4, size, cp.hue)
	res.version = res.versions.Add(1)
	slot.qHue = qHue
	cp.svRes = res
	cp.svRenderedHue = qHue
	return cp.svRes
}
