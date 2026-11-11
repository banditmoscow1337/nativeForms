package nativeforms

import (
	"slices"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

const (
	smallTextFieldPoolSize = 64
	largeTextFieldPoolSize = 32
)

var (
	freeTextFieldsMu    sync.Mutex
	smallFieldStorage   [smallTextFieldPoolSize]TextField
	largeFieldStorage   [largeTextFieldPoolSize]TextField
	freeSmallTextFields []*TextField
	freeLargeTextFields []*TextField
	textFieldPoolOnce   sync.Once
	largeRuneCountMu    sync.Mutex
	cachedRuneCountText string
	cachedRuneCountVal  int
)

func initTextFieldPool() {
	freeSmallTextFields = make([]*TextField, 0, smallTextFieldPoolSize)
	for i := smallTextFieldPoolSize - 1; i >= 0; i-- {
		f := &smallFieldStorage[i]
		f.pooled = true
		f.inPool = true
		freeSmallTextFields = append(freeSmallTextFields, f)
	}
	freeLargeTextFields = make([]*TextField, 0, largeTextFieldPoolSize)
	for i := largeTextFieldPoolSize - 1; i >= 0; i-- {
		f := &largeFieldStorage[i]
		f.pooled = true
		f.largeSlot = true
		f.inPool = true
		freeLargeTextFields = append(freeLargeTextFields, f)
	}
}

func recycleTextField(f *TextField) {
	if f == nil || f.parent != nil || f.manager != nil {
		return
	}
	freeTextFieldsMu.Lock()
	textFieldPoolOnce.Do(initTextFieldPool)
	if f.inPool {
		freeTextFieldsMu.Unlock()
		return
	}
	caretScratch := f.caretScratch[:0]
	wasPooled := f.pooled
	wasLarge := f.largeSlot
	*f = TextField{caretScratch: caretScratch, pooled: wasPooled, largeSlot: wasLarge, inPool: true}
	if wasLarge {
		if len(freeLargeTextFields) < cap(freeLargeTextFields) {
			freeLargeTextFields = append(freeLargeTextFields, f)
		}
	} else {
		if len(freeSmallTextFields) < cap(freeSmallTextFields) {
			freeSmallTextFields = append(freeSmallTextFields, f)
		}
	}
	freeTextFieldsMu.Unlock()
}

// Close releases a detached TextField back to the internal widget pool.
func (f *TextField) Close() {
	recycleTextField(f)
}

type TextField struct {
	State
	Text, Placeholder                string
	Font                             FontSpec
	Cursor                           int
	OnChange                         func(string)
	OnSubmit                         func(string)
	Multiline, ReadOnly, Password    bool
	Validate                         func(string) bool
	editor                           *TextEditor
	scrollX, scrollY                 float32
	lastClick                        time.Time
	lastPoint                        Vec2
	clicks                           int
	caretVisible                     bool
	composing                        bool
	layoutCache                      *textFieldLayout
	compositionCache                 *textFieldLayout
	compositionBase                  *textFieldLayout
	compositionText                  string
	compositionStart, compositionEnd int
	editorStorage                    TextEditor
	layoutStorage                    textFieldLayout
	singleLine                       [1]textLine
	singleLineText                   string
	shortRunes                       [32]rune
	caretScratch                     []float32
	snapValid                        bool
	snapVersion                      uint64
	snapCaret, snapAnchor, snapMax   int
	snapState                        TextInputState
	pooled                           bool
	largeSlot                        bool
	inPool                           bool
}

func NewTextField(text string, onChange func(string)) *TextField {
	var n int
	isLarge := len(text) > 4096
	if isLarge {
		largeRuneCountMu.Lock()
		if text == cachedRuneCountText {
			n = cachedRuneCountVal
			largeRuneCountMu.Unlock()
		} else {
			largeRuneCountMu.Unlock()
			n = utf8.RuneCountInString(text)
			largeRuneCountMu.Lock()
			cachedRuneCountText = text
			cachedRuneCountVal = n
			largeRuneCountMu.Unlock()
		}
	} else {
		n = utf8.RuneCountInString(text)
	}
	var f *TextField
	freeTextFieldsMu.Lock()
	textFieldPoolOnce.Do(initTextFieldPool)
	if isLarge {
		if l := len(freeLargeTextFields); l > 0 {
			f = freeLargeTextFields[l-1]
			freeLargeTextFields[l-1] = nil
			freeLargeTextFields = freeLargeTextFields[:l-1]
		}
	} else {
		if l := len(freeSmallTextFields); l > 0 {
			f = freeSmallTextFields[l-1]
			freeSmallTextFields[l-1] = nil
			freeSmallTextFields = freeSmallTextFields[:l-1]
		}
	}
	freeTextFieldsMu.Unlock()
	if f == nil {
		f = &TextField{largeSlot: isLarge}
	} else {
		caretScratch := f.caretScratch[:0]
		wasPooled := f.pooled
		wasLarge := f.largeSlot
		*f = TextField{caretScratch: caretScratch, pooled: wasPooled, largeSlot: wasLarge, inPool: false}
	}
	f.Text = text
	f.Cursor = n
	f.OnChange = onChange
	f.caretVisible = true
	f.editorStorage = TextEditor{Text: text, Caret: n, Anchor: n}
	f.editor = &f.editorStorage
	f.owner = f
	f.focusable = true
	f.minimum = Vec2{X: 180}
	return f
}

func NewTextArea(text string, onChange func(string)) *TextField {
	f := NewTextField(text, onChange)
	f.Multiline = true
	f.minimum.Y = 140
	return f
}

func (f *TextField) Editor() *TextEditor { f.syncExternal(); return f.editor }

func (f *TextField) CommitEditor() {
	if f.editor != nil && f.editor.Text != f.Text {
		if f.layoutCache != nil && f.editor.version == f.layoutCache.version {
			f.layoutCache = nil // external mutation bypassed TextEditor.Replace
		}
		f.changed()
	}
}

func (f *TextField) SetText(text string) {
	if text == f.Text {
		return
	}
	if f.editor == nil {
		f.editor = NewTextEditor(f.Text)
	}
	f.Text = text
	f.editor.SetText(text)
	f.layoutCache = nil
	f.compositionCache = nil
	f.composing = false
	f.Cursor = f.editor.Caret
	if f.Multiline || f.autoSize {
		f.invalidateLayout()
	} else {
		f.InvalidatePaint()
	}
	if f.OnChange != nil {
		f.OnChange(text)
	}
}

func (f *TextField) syncExternal() {
	if f.editor == nil {
		f.editor = NewTextEditor(f.Text)
	}
	if f.editor.Text != f.Text {
		f.editor.SetText(f.Text)
		f.layoutCache = nil
		f.compositionCache = nil
		f.composing = false
	}
	if f.Cursor != f.editor.Caret {
		f.editor.Move(f.Cursor, false)
	}
}

type textLine struct {
	start, end int
	width      float32
}

func (f *TextField) displayRunes() []rune {
	r := []rune(f.editor.Text)
	if f.Password {
		maskPasswordRunes(r, f.fontContext() != nil)
	}
	return r
}

func maskPasswordRunes(r []rune, preserveClusters bool) {
	if !preserveClusters {
		for i := range r {
			if hardTextBreak(r, i) == 0 {
				r[i] = '*'
			}
		}
		return
	}
	for i := 0; i < len(r); {
		if count := hardTextBreak(r, i); count != 0 {
			i += count
			continue
		}
		end := nextGrapheme(r, i)
		r[i] = '*'
		for j := i + 1; j < end; j++ {
			r[j] = '\u034f'
		}
		i = end
	}
}

func (f *TextField) fontSpec() FontSpec {
	spec := f.Font
	if spec.Size <= 0 {
		spec.Size = f.Theme().FontSize
	}
	return spec
}

func (f *TextField) lineHeight(spec FontSpec) float32 {
	if fonts := f.fontContext(); fonts != nil {
		return fonts.layoutShared("", f.effectiveFontSpec(spec)).Height()
	}
	return spec.Size * 1.2
}

func (f *TextField) runeSliceWidth(r []rune, start, end int, spec FontSpec, atlasOnly bool) float32 {
	if atlasOnly {
		return walkAtlasRunes(r[start:end], spec.Size, nil, nil)
	}
	if !f.Multiline && !f.Password && start == 0 && end == len(r) && len(r) == utf8.RuneCountInString(f.editor.Text) {
		return f.textWidth(f.editor.Text, spec)
	}
	return f.textWidth(string(r[start:end]), spec)
}

func (f *TextField) lines(r []rune, width float32, spec FontSpec) []textLine {
	atlasOnly := f.fontContext() == nil && !hasTextShaper()
	if !f.Multiline {
		f.singleLine[0] = textLine{0, len(r), f.runeSliceWidth(r, 0, len(r), spec, atlasOnly)}
		return f.singleLine[:1]
	}
	var result []textLine
	if len(r) > 64 {
		est := 8
		if len(r) == len(f.editor.Text) {
			est = strings.Count(f.editor.Text, "\n") + 2
		} else {
			est = max(8, len(r)/16)
		}
		result = make([]textLine, 0, est)
	}
	var measure func(string, float32) float32
	if !atlasOnly {
		measure = func(s string, _ float32) float32 { return f.textWidth(s, spec) }
	}
	prevStart, prevEnd := -1, -1
	var prevWidth float32
	for start := 0; ; {
		end := start
		for end < len(r) && hardTextBreak(r, end) == 0 {
			end++
		}
		at := start
		for at < end {
			var fullWidth float32
			if prevStart >= 0 && end-at == prevEnd-prevStart && slices.Equal(r[at:end], r[prevStart:prevEnd]) {
				fullWidth = prevWidth
			} else {
				fullWidth = f.runeSliceWidth(r, at, end, spec, atlasOnly)
				prevStart, prevEnd, prevWidth = at, end, fullWidth
			}
			if width <= 0 || fullWidth <= width {
				result = append(result, textLine{at, end, fullWidth})
				break
			}
			var limit int
			if atlasOnly {
				limit = wrappedLineEndAtlas(r, at, end, width, spec.Size)
			} else {
				limit = wrappedLineEndWith(r, at, end, width, spec.Size, measure)
			}
			lineWidth := fullWidth
			if limit < end {
				lineWidth = f.runeSliceWidth(r, at, limit, spec, atlasOnly)
			}
			result = append(result, textLine{at, limit, lineWidth})
			at = limit
		}
		if start == end {
			result = append(result, textLine{start, end, 0})
		}
		if end == len(r) {
			break
		}
		start = end + hardTextBreak(r, end)
	}
	return result
}

func (f *TextField) measureTextSize() Vec2 {
	f.syncExternal()
	spec := f.fontSpec()
	lh := f.lineHeight(spec)
	maxWrapW := float32(0)
	if f.maximum.X > 20 {
		maxWrapW = f.maximum.X - 20
	}
	lines := f.layoutFor(maxWrapW, spec).lines
	maxW := float32(0)
	for _, line := range lines {
		maxW = maxFloat32(maxW, line.width)
	}
	if len(f.editor.Text) == 0 && f.Placeholder != "" {
		maxW = maxFloat32(maxW, f.textWidth(f.Placeholder, spec))
	}
	width := maxFloat32(48, maxW+24)
	height := maxFloat32(f.Theme().ControlHeight, lh+12)
	if f.Multiline {
		height = maxFloat32(height, float32(max(1, len(lines)))*lh+12)
	}
	return Vec2{X: width, Y: height}
}

func (f *TextField) Measure(c Constraints) Vec2 {
	f.syncExternal()
	if f.autoSize {
		size := f.measureTextSize()
		if f.Multiline && c.HasMaxX() && c.Max.X > 20 && size.X > c.Max.X {
			spec := f.fontSpec()
			lines := f.layoutFor(c.Max.X-20, spec).lines
			maxW := float32(0)
			for _, line := range lines {
				maxW = maxFloat32(maxW, line.width)
			}
			size.X = minFloat32(c.Max.X, maxFloat32(48, maxW+24))
			size.Y = maxFloat32(f.Theme().ControlHeight, float32(max(1, len(lines)))*f.lineHeight(spec)+12)
		}
		return measureWithState(&f.State, size, c)
	}
	spec := f.fontSpec()
	width := maxFloat32(1, c.Max.X-20)
	if c.Max.X <= 0 {
		width = 240
	}
	lines := f.layoutFor(width, spec).lines
	height := f.Theme().ControlHeight
	if f.Multiline {
		height = maxFloat32(height, float32(len(lines))*f.lineHeight(spec)+12)
	}
	return measureWithState(&f.State, Vec2{X: 240, Y: height}, c)
}
