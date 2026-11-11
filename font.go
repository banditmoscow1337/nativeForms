package nativeforms

import (
	"fmt"
	"image"
	"image/color"
	"os"
	"runtime"
	"strings"
	"sync"
	"unicode"
)

const (
	fontAtlasSize     = 2048
	fontCellSize      = 64
	fontReferenceSize = 48
)

type glyphInfo struct {
	cellX, cellY int
	advance      float32
	face         *TrueTypeFace
	id           uint16
	visible      bool
}

const fastGlyphLimit = 0x530

var scalableGlyphs map[rune]glyphInfo
var fastGlyphs [fastGlyphLimit]glyphInfo
var fastGlyphsValid [fastGlyphLimit]bool
var metricsOnce sync.Once
var atlasOnce sync.Once
var atlasImage image.Image
var fontMu sync.Mutex
var selectedFace *TrueTypeFace
var fallbackFaces []*TrueTypeFace
var metricsReady bool
var atlasReady bool

func SetFontData(data []byte) error {
	face, err := ParseTrueType(data)
	if err != nil {
		return err
	}
	fontMu.Lock()
	defer fontMu.Unlock()
	if atlasReady {
		return fmt.Errorf("font atlas has already been initialized")
	}
	selectedFace = face
	if metricsReady {
		rebuildFontMetricsLocked()
	}
	return nil
}

func SetFallbackFontData(fonts ...[]byte) error {
	faces := make([]*TrueTypeFace, 0, len(fonts))
	for i, data := range fonts {
		face, err := ParseTrueType(data)
		if err != nil {
			return fmt.Errorf("fallback font %d: %w", i, err)
		}
		faces = append(faces, face)
	}
	fontMu.Lock()
	defer fontMu.Unlock()
	if atlasReady {
		return fmt.Errorf("font atlas has already been initialized")
	}
	fallbackFaces = faces
	if metricsReady {
		rebuildFontMetricsLocked()
	}
	return nil
}

func currentAtlasFacesLocked() []*TrueTypeFace {
	var faces []*TrueTypeFace
	if selectedFace != nil {
		faces = append(faces, selectedFace)
	}
	faces = append(faces, fallbackFaces...)
	if len(faces) == 0 {
		faces = defaultSystemFaces()
	}
	return faces
}

func ensureFontMetrics() {
	metricsOnce.Do(func() {
		fontMu.Lock()
		defer fontMu.Unlock()
		if !metricsReady {
			rebuildFontMetricsLocked()
		}
	})
}

func rebuildFontMetricsLocked() {
	faces := currentAtlasFacesLocked()
	characters := atlasCharacterSet()
	cellsPerRow := fontAtlasSize / fontCellSize
	scalableGlyphs = make(map[rune]glyphInfo, len(characters))
	clear(fastGlyphs[:])
	clear(fastGlyphsValid[:])
	for index, character := range characters {
		if index >= cellsPerRow*cellsPerRow {
			break
		}
		x, y := index%cellsPerRow, index/cellsPerRow
		glyph := glyphInfo{cellX: x, cellY: y, advance: 0.75, visible: !unicode.IsSpace(character)}
		for _, face := range faces {
			id := face.glyphIndex(character)
			if id == 0 {
				continue
			}
			if !face.canDrawGlyph(id) {
				continue
			}
			glyph.advance, glyph.face, glyph.id = face.advance(id), face, id
			break
		}
		scalableGlyphs[character] = glyph
		if character >= 0 && character < fastGlyphLimit {
			fastGlyphs[character] = glyph
			fastGlyphsValid[character] = true
		}
	}
	metricsReady = true
}

func atlasCharacterSet() []rune {
	characters := make([]rune, 0, 800)
	appendRange := func(first, last rune) {
		for character := first; character <= last; character++ {
			characters = append(characters, character)
		}
	}
	appendRange(32, 126)
	appendRange(160, 383)
	appendRange(0x370, 0x52f) // Greek, Coptic and Cyrillic.
	characters = append(characters, '‐', '‑', '–', '—', '‘', '’', '“', '”', '„', '…', '№', '€', '₽', '•', '−')
	return characters
}

func FontAtlas() image.Image {
	ensureFontMetrics()
	atlasOnce.Do(func() {
		fontMu.Lock()
		defer fontMu.Unlock()
		faces := currentAtlasFacesLocked()
		atlasImage = buildFontAtlas(faces)
		atlasReady = true
	})
	return atlasImage
}

type systemFontCandidate struct {
	path   string
	family string
	weight int
	italic bool
}

func systemFontCandidates() []systemFontCandidate {
	switch runtime.GOOS {
	case "windows":
		windir := os.Getenv("WINDIR")
		if windir == "" {
			windir = "C:\\Windows"
		}
		f := windir + "\\Fonts\\"
		return []systemFontCandidate{
			{path: f + "segoeui.ttf", family: "", weight: 400},
			{path: f + "segoeuib.ttf", family: "", weight: 700},
			{path: f + "arial.ttf", family: "", weight: 400},
			{path: f + "arialbd.ttf", family: "", weight: 700},
			{path: f + "consola.ttf", family: "monospace", weight: 400},
			{path: f + "consolab.ttf", family: "monospace", weight: 700},
			{path: f + "cour.ttf", family: "monospace", weight: 400},
			{path: f + "courbd.ttf", family: "monospace", weight: 700},
		}
	case "darwin":
		return []systemFontCandidate{
			{path: "/System/Library/Fonts/Supplemental/Arial.ttf", family: "", weight: 400},
			{path: "/System/Library/Fonts/Supplemental/Arial Bold.ttf", family: "", weight: 700},
			{path: "/Library/Fonts/Arial.ttf", family: "", weight: 400},
			{path: "/Library/Fonts/Arial Bold.ttf", family: "", weight: 700},
			{path: "/System/Library/Fonts/SFNSMono.ttf", family: "monospace", weight: 400},
			{path: "/System/Library/Fonts/Monaco.ttf", family: "monospace", weight: 400},
			{path: "/System/Library/Fonts/Supplemental/Andale Mono.ttf", family: "monospace", weight: 400},
			{path: "/System/Library/Fonts/Supplemental/Courier New.ttf", family: "monospace", weight: 400},
			{path: "/System/Library/Fonts/Supplemental/Courier New Bold.ttf", family: "monospace", weight: 700},
		}
	default:
		return []systemFontCandidate{
			{path: "/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf", family: "", weight: 400},
			{path: "/usr/share/fonts/truetype/dejavu/DejaVuSans-Bold.ttf", family: "", weight: 700},
			{path: "/usr/share/fonts/truetype/liberation2/LiberationSans-Regular.ttf", family: "", weight: 400},
			{path: "/usr/share/fonts/truetype/liberation2/LiberationSans-Bold.ttf", family: "", weight: 700},
			{path: "/usr/share/fonts/truetype/dejavu/DejaVuSansMono.ttf", family: "monospace", weight: 400},
			{path: "/usr/share/fonts/truetype/dejavu/DejaVuSansMono-Bold.ttf", family: "monospace", weight: 700},
			{path: "/usr/share/fonts/truetype/liberation2/LiberationMono-Regular.ttf", family: "monospace", weight: 400},
			{path: "/usr/share/fonts/truetype/liberation2/LiberationMono-Bold.ttf", family: "monospace", weight: 700},
		}
	}
}

func defaultSystemFaces() []*TrueTypeFace {
	shared := getSharedSystemFaces()
	for _, f := range shared {
		if f.family == "" && f.weight == 400 && !f.italic {
			return []*TrueTypeFace{f.face}
		}
	}
	var faces []*TrueTypeFace
	for _, c := range systemFontCandidates() {
		if c.family != "" || c.weight != 400 || c.italic {
			continue
		}
		data, err := os.ReadFile(c.path)
		if err != nil {
			continue
		}
		face, err := ParseTrueType(data)
		if err == nil {
			faces = append(faces, face)
			break
		}
	}
	return faces
}

var glyphRows = map[byte][7]byte{
	'!':  {0x04, 0x04, 0x04, 0x04, 0x04, 0x00, 0x04},
	'"':  {0x0A, 0x0A, 0x0A},
	'#':  {0x0A, 0x1F, 0x0A, 0x0A, 0x1F, 0x0A},
	'$':  {0x04, 0x0F, 0x14, 0x0E, 0x05, 0x1E, 0x04},
	'%':  {0x19, 0x19, 0x02, 0x04, 0x08, 0x13, 0x13},
	'&':  {0x0C, 0x12, 0x14, 0x08, 0x15, 0x12, 0x0D},
	'\'': {0x04, 0x04, 0x08},
	'(':  {0x02, 0x04, 0x08, 0x08, 0x08, 0x04, 0x02},
	')':  {0x08, 0x04, 0x02, 0x02, 0x02, 0x04, 0x08},
	'*':  {0x00, 0x15, 0x0E, 0x1F, 0x0E, 0x15},
	'+':  {0x00, 0x04, 0x04, 0x1F, 0x04, 0x04},
	',':  {0x00, 0x00, 0x00, 0x00, 0x04, 0x04, 0x08},
	'-':  {0x00, 0x00, 0x00, 0x1F},
	'.':  {0x00, 0x00, 0x00, 0x00, 0x00, 0x06, 0x06},
	'/':  {0x01, 0x02, 0x02, 0x04, 0x08, 0x08, 0x10},
	'0':  {0x0E, 0x11, 0x13, 0x15, 0x19, 0x11, 0x0E},
	'1':  {0x04, 0x0C, 0x14, 0x04, 0x04, 0x04, 0x1F},
	'2':  {0x0E, 0x11, 0x01, 0x02, 0x04, 0x08, 0x1F},
	'3':  {0x1E, 0x01, 0x01, 0x0E, 0x01, 0x01, 0x1E},
	'4':  {0x02, 0x06, 0x0A, 0x12, 0x1F, 0x02, 0x02},
	'5':  {0x1F, 0x10, 0x10, 0x1E, 0x01, 0x01, 0x1E},
	'6':  {0x06, 0x08, 0x10, 0x1E, 0x11, 0x11, 0x0E},
	'7':  {0x1F, 0x01, 0x02, 0x04, 0x08, 0x08, 0x08},
	'8':  {0x0E, 0x11, 0x11, 0x0E, 0x11, 0x11, 0x0E},
	'9':  {0x0E, 0x11, 0x11, 0x0F, 0x01, 0x02, 0x0C},
	':':  {0x00, 0x06, 0x06, 0x00, 0x06, 0x06},
	';':  {0x00, 0x06, 0x06, 0x00, 0x04, 0x04, 0x08},
	'<':  {0x02, 0x04, 0x08, 16, 0x08, 0x04, 0x02},
	'=':  {0x00, 0x00, 0x1F, 0x00, 0x1F},
	'>':  {0x08, 0x04, 0x02, 0x01, 0x02, 0x04, 0x08},
	'?':  {0x0E, 0x11, 0x01, 0x02, 0x04, 0x00, 0x04},
	'@':  {0x0E, 0x11, 0x17, 0x15, 0x17, 0x10, 0x0E},
	'A':  {0x0E, 0x11, 0x11, 0x1F, 0x11, 0x11, 0x11},
	'B':  {0x1E, 0x11, 0x11, 0x1E, 0x11, 0x11, 0x1E},
	'C':  {0x0E, 0x11, 0x10, 0x10, 0x10, 0x11, 0x0E},
	'D':  {0x1E, 0x11, 0x11, 0x11, 0x11, 0x11, 0x1E},
	'E':  {0x1F, 0x10, 0x10, 0x1E, 0x10, 0x10, 0x1F},
	'F':  {0x1F, 0x10, 0x10, 0x1E, 0x10, 0x10, 0x10},
	'G':  {0x0E, 0x11, 0x10, 0x17, 0x11, 0x11, 0x0F},
	'H':  {0x11, 0x11, 0x11, 0x1F, 0x11, 0x11, 0x11},
	'I':  {0x0E, 0x04, 0x04, 0x04, 0x04, 0x04, 0x0E},
	'J':  {0x07, 0x02, 0x02, 0x02, 0x12, 0x12, 0x0C},
	'K':  {0x11, 0x12, 0x14, 0x18, 0x14, 0x12, 0x11},
	'L':  {0x10, 0x10, 0x10, 0x10, 0x10, 0x10, 0x1F},
	'M':  {0x11, 0x1B, 0x15, 0x15, 0x11, 0x11, 0x11},
	'N':  {0x11, 0x19, 0x19, 0x15, 0x13, 0x13, 0x11},
	'O':  {0x0E, 0x11, 0x11, 0x11, 0x11, 0x11, 0x0E},
	'P':  {0x1E, 0x11, 0x11, 0x1E, 0x10, 0x10, 0x10},
	'Q':  {0x0E, 0x11, 0x11, 0x11, 0x15, 0x12, 0x0D},
	'R':  {0x1E, 0x11, 0x11, 0x1E, 0x14, 0x12, 0x11},
	'S':  {0x0F, 0x10, 0x10, 0x0E, 0x01, 0x01, 0x1E},
	'T':  {0x1F, 0x04, 0x04, 0x04, 0x04, 0x04, 0x04},
	'U':  {0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x0E},
	'V':  {0x11, 0x11, 0x11, 0x11, 0x11, 0x0A, 0x04},
	'W':  {0x11, 0x11, 0x11, 0x15, 0x15, 0x15, 0x0A},
	'X':  {0x11, 0x11, 0x0A, 0x04, 0x0A, 0x11, 0x11},
	'Y':  {0x11, 0x11, 0x0A, 0x04, 0x04, 0x04, 0x04},
	'Z':  {0x1F, 0x01, 0x02, 0x04, 0x08, 0x10, 0x1F},
	'[':  {0x0E, 0x08, 0x08, 0x08, 0x08, 0x08, 0x0E},
	'\\': {0x10, 0x08, 0x08, 0x04, 0x02, 0x02, 0x01},
	']':  {0x0E, 0x02, 0x02, 0x02, 0x02, 0x02, 0x0E},
	'^':  {0x04, 0x0A, 0x11},
	'_':  {0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x1F},
	'`':  {0x08, 0x04, 0x02},
	'{':  {0x02, 0x04, 0x04, 0x08, 0x04, 0x04, 0x02},
	'|':  {0x04, 0x04, 0x04, 0x04, 0x04, 0x04, 0x04},
	'}':  {0x08, 0x04, 0x04, 0x02, 0x04, 0x04, 0x08},
	'~':  {0x00, 0x00, 0x09, 0x16},
}

func buildFontAtlas(faces []*TrueTypeFace) image.Image {
	atlas := image.NewNRGBA(image.Rect(0, 0, fontAtlasSize, fontAtlasSize))
	atlas.SetNRGBA(0, 0, color.NRGBA{R: 255, G: 255, B: 255, A: 255})

	characters := atlasCharacterSet()
	cellsPerRow := fontAtlasSize / fontCellSize
	scalableGlyphs = make(map[rune]glyphInfo, len(characters))
	for index, character := range characters {
		if index >= cellsPerRow*cellsPerRow {
			break
		}
		x, y := index%cellsPerRow, index/cellsPerRow
		glyph := glyphInfo{cellX: x, cellY: y, advance: 0.75, visible: !unicode.IsSpace(character)}
		found := false
		for _, face := range faces {
			id := face.glyphIndex(character)
			if id == 0 {
				continue
			}
			if err := face.drawGlyph(atlas, id, x*fontCellSize, y*fontCellSize); err != nil {
				continue
			}
			glyph.advance, glyph.face, glyph.id = face.advance(id), face, id
			found = true
			break
		}
		if !found {
			key := unicode.ToUpper(character)
			if _, ok := glyphRows[byte(key)]; (key > 127 || !ok) && character != ' ' {
				continue
			}
			drawBitmapGlyph(atlas, glyphRows[byte(key)], x*fontCellSize, y*fontCellSize)
		}
		scalableGlyphs[character] = glyph
		if character >= 0 && character < fastGlyphLimit {
			fastGlyphs[character] = glyph
			fastGlyphsValid[character] = true
		}
	}
	metricsReady = true
	return atlas
}

func drawBitmapGlyph(atlas *image.NRGBA, rows [7]byte, x, y int) {
	for row, bits := range rows {
		for col := 0; col < 5; col++ {
			if bits&(1<<uint(4-col)) == 0 {
				continue
			}
			for yy := 0; yy < 6; yy++ {
				for xx := 0; xx < 6; xx++ {
					atlas.SetNRGBA(x+10+col*6+xx, y+10+row*6+yy,
						color.NRGBA{R: 255, G: 255, B: 255, A: 255})
				}
			}
		}
	}
}

func glyphForRune(character rune) glyphInfo {
	if !metricsReady {
		ensureFontMetrics()
	}
	if character >= 32 && character < fastGlyphLimit {
		if character == '\u0085' || character == '\u00ad' {
			return glyphInfo{}
		}
		if character == '\u00a0' {
			return fastGlyphs[' ']
		}
		if fastGlyphsValid[character] {
			return fastGlyphs[character]
		}
	}
	switch character {
	case '\r', '\n', '\u0085', '\u2028', '\u2029', '\u00ad', '\u200b', '\u200c', '\u200d', '\u2060', '\ufeff':
		return glyphInfo{}
	case '\t':
		return glyphInfo{advance: fastGlyphs[' '].advance * 4}
	case '\u202f', '\u2009':
		return glyphInfo{advance: fastGlyphs[' '].advance * 0.6}
	}
	if glyph, ok := scalableGlyphs[character]; ok {
		return glyph
	}
	if fastGlyphsValid['?'] {
		return fastGlyphs['?']
	}
	return glyphInfo{advance: 0.6}
}

func MeasureText(text string, size float32) Vec2 {
	if size <= 0 {
		return Vec2{}
	}
	text = normalizeTextBreaks(text)
	maxWidth := float32(0)
	lineCount := 0
	for start := 0; ; {
		lineCount++
		end := strings.IndexByte(text[start:], '\n')
		var line string
		if end < 0 {
			line = text[start:]
		} else {
			line = text[start : start+end]
		}
		if w := measureTextLine(line, size); w > maxWidth {
			maxWidth = w
		}
		if end < 0 {
			break
		}
		start += end + 1
	}
	return Vec2{
		X: maxWidth,
		Y: float32(lineCount) * size * 1.2,
	}
}

func measureTextLine(text string, size float32) float32 {
	if shaped, ok := shapeText(text, size); ok {
		return shaped.Advance
	}
	return walkAtlasText(text, size, nil, nil)
}
