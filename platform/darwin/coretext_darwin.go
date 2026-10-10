//go:build darwin && (amd64 || arm64)

package darwin

import (
	"fmt"
	"image"
	"math"
	"runtime"
	"strings"
	"sync"
	"unicode/utf8"
	"unsafe"

	ui "github.com/banditmoscow1337/nativeForms"
	"github.com/ebitengine/purego"
)

var coreTextOnce sync.Once
var coreTextError error
var systemShaper *coreTextShaper

type shapeKey struct {
	text string
	size float32
}
type coreTextShaper struct {
	sync.Mutex
	cache          map[shapeKey]ui.ShapedText
	bytes          int
	fontKey        uintptr
	font           func(uint32, float64, uintptr) uintptr
	line           func(uintptr) uintptr
	bounds         func(uintptr, *float64, *float64, *float64) float64
	ink            func(uintptr, uintptr) rect
	offset         func(uintptr, int64, *float64) float64
	draw           func(uintptr, uintptr)
	release        func(uintptr)
	bitmap         func(unsafe.Pointer, uintptr, uintptr, uintptr, uintptr, uintptr, uint32) uintptr
	translate      func(uintptr, float64, float64)
	textPosition   func(uintptr, float64, float64)
	fill           func(uintptr, float64, float64, float64, float64)
	contextRelease func(uintptr)
}

func EnableCoreText() error {
	if err := loadAppKit(); err != nil {
		return err
	}
	coreTextOnce.Do(func() {
		ct, err := purego.Dlopen("/System/Library/Frameworks/CoreText.framework/CoreText", purego.RTLD_LOCAL|purego.RTLD_LAZY)
		if err != nil {
			coreTextError = err
			return
		}
		cg, err := purego.Dlopen("/System/Library/Frameworks/CoreGraphics.framework/CoreGraphics", purego.RTLD_LOCAL|purego.RTLD_LAZY)
		if err != nil {
			coreTextError = err
			return
		}
		s := &coreTextShaper{cache: make(map[shapeKey]ui.ShapedText)}
		for _, entry := range []struct {
			lib  uintptr
			name string
			fn   any
		}{
			{ct, "CTFontCreateUIFontForLanguage", &s.font}, {ct, "CTLineCreateWithAttributedString", &s.line},
			{ct, "CTLineGetTypographicBounds", &s.bounds}, {ct, "CTLineGetImageBounds", &s.ink}, {ct, "CTLineGetOffsetForStringIndex", &s.offset},
			{ct, "CTLineDraw", &s.draw}, {ct, "CFRelease", &s.release},
			{cg, "CGBitmapContextCreate", &s.bitmap}, {cg, "CGContextTranslateCTM", &s.translate}, {cg, "CGContextSetTextPosition", &s.textPosition},
			{cg, "CGContextSetRGBFillColor", &s.fill}, {cg, "CGContextRelease", &s.contextRelease},
		} {
			address, e := purego.Dlsym(entry.lib, entry.name)
			if e != nil {
				coreTextError = e
				return
			}
			purego.RegisterFunc(entry.fn, address)
		}
		key, err := purego.Dlsym(ct, "kCTFontAttributeName")
		if err != nil {
			coreTextError = err
			return
		}
		s.fontKey = *(*uintptr)(unsafe.Pointer(key))
		systemShaper = s
	})
	if coreTextError != nil {
		return fmt.Errorf("darwin: Core Text: %w", coreTextError)
	}
	ui.SetTextShaper(systemShaper)
	return nil
}
func (s *coreTextShaper) Shape(text string, fontSize float32) (ui.ShapedText, bool) {
	if fontSize <= 0 || fontSize > 512 || len(text) > 64<<10 || !utf8.ValidString(text) {
		return ui.ShapedText{}, false
	}
	if text == "" {
		return ui.ShapedText{Carets: []float32{0}}, true
	}
	s.Lock()
	defer s.Unlock()
	key := shapeKey{text, fontSize}
	if cached, ok := s.cache[key]; ok {
		return cached, true
	}
	pool := class("NSAutoreleasePool").Send(selector("new"))
	defer pool.Send(selector("drain"))
	font := s.font(2, float64(fontSize), 0)
	if font == 0 {
		return ui.ShapedText{}, false
	}
	defer s.release(font)
	attributes := class("NSDictionary").Send(selector("dictionaryWithObject:forKey:"), font, s.fontKey)
	value := class("NSAttributedString").Send(selector("alloc")).Send(selector("initWithString:attributes:"), nsString(strings.ReplaceAll(text, "\x00", "\ufffd")), attributes)
	if value == 0 {
		return ui.ShapedText{}, false
	}
	defer value.Send(selector("release"))
	line := s.line(uintptr(value))
	if line == 0 {
		return ui.ShapedText{}, false
	}
	defer s.release(line)
	var ascent, descent, leading float64
	advance := s.bounds(line, &ascent, &descent, &leading)
	if math.IsNaN(advance) || math.IsInf(advance, 0) || advance < 0 {
		return ui.ShapedText{}, false
	}
	result := ui.ShapedText{Advance: float32(advance), Carets: make([]float32, 0, utf8.RuneCountInString(text)+1)}
	index := int64(0)
	for _, r := range text {
		result.Carets = append(result.Carets, float32(s.offset(line, index, nil)))
		index++
		if r > 0xffff {
			index++
		}
	}
	result.Carets = append(result.Carets, float32(s.offset(line, index, nil)))
	ink := s.ink(line, 0)
	if ink.Size.W > 0 && ink.Size.H > 0 {
		x0, y0 := math.Floor(ink.Origin.X)-1, math.Floor(ink.Origin.Y)-1
		x1, y1 := math.Ceil(ink.Origin.X+ink.Size.W)+1, math.Ceil(ink.Origin.Y+ink.Size.H)+1
		width, height := int(x1-x0), int(y1-y0)
		if width <= 0 || height <= 0 || width > 65536 || height > 2048 || int64(width)*int64(height) > 4<<20 {
			return ui.ShapedText{}, false
		}
		pixels := make([]byte, width*height)
		var pin runtime.Pinner
		pin.Pin(&pixels[0])
		defer pin.Unpin()
		context := s.bitmap(unsafe.Pointer(&pixels[0]), uintptr(width), uintptr(height), 8, uintptr(width), 0, 7)
		if context == 0 {
			return ui.ShapedText{}, false
		}
		s.translate(context, -x0, -y0)
		s.textPosition(context, 0, 0)
		s.fill(context, 1, 1, 1, 1)
		s.draw(line, context)
		s.contextRelease(context)
		runtime.KeepAlive(pixels)
		mask := coreTextMask(pixels, width, height)
		baseline := (float64(fontSize)*1.2-ascent-descent)/2 + ascent
		result.Mask = mask
		result.Offset = ui.Vec2{X: float32(x0), Y: float32(baseline - y1)}
	}
	cost := len(text) + len(result.Carets)*4
	if result.Mask != nil {
		cost += len(result.Mask.Pix)
	}
	if len(s.cache) >= 512 || s.bytes+cost > 8<<20 {
		s.cache = make(map[shapeKey]ui.ShapedText)
		s.bytes = 0
	}
	s.cache[key] = result
	s.bytes += cost
	return result, true
}

func coreTextMask(pixels []byte, width, height int) *image.Alpha {
	mask := image.NewAlpha(image.Rect(0, 0, width, height))
	copy(mask.Pix, pixels)
	return mask
}