package nativeforms

import (
	"encoding/binary"
	"math"
	"sort"
	"testing"
)

func stageFiveTestFace(characters map[rune]uint16) *TrueTypeFace {
	put16 := binary.BigEndian.PutUint16
	put32 := binary.BigEndian.PutUint32
	hhea := make([]byte, 36)
	put16(hhea[4:], 800)
	put16(hhea[6:], 0xff38)
	put16(hhea[8:], 100)
	put16(hhea[34:], 6)
	hmtx := make([]byte, 24)
	for id := 0; id < 6; id++ {
		advance := uint16(600)
		if id == 5 {
			advance = 0
		}
		put16(hmtx[id*4:], advance)
	}
	glyph := func(width, height int16) []byte {
		b := make([]byte, 34)
		put16(b[0:], 1)
		put16(b[6:], uint16(width))
		put16(b[8:], uint16(height))
		put16(b[10:], 3)
		for i := 0; i < 4; i++ {
			b[14+i] = 1
		}
		put16(b[20:], uint16(width))
		put16(b[24:], uint16(-width))
		put16(b[30:], uint16(height))
		return b
	}
	var glyf []byte
	loca := make([]byte, 28)
	for id := 1; id < 6; id++ {
		put32(loca[(id+1)*4:], uint32(len(glyf)+34))
		if id == 5 {
			glyf = append(glyf, glyph(200, 100)...)
		} else {
			glyf = append(glyf, glyph(500, 700)...)
		}
	}
	keys := make([]rune, 0, len(characters))
	for character := range characters {
		keys = append(keys, character)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
	cmap := make([]byte, 16+len(keys)*12)
	put16(cmap[0:], 12)
	put32(cmap[4:], uint32(len(cmap)))
	put32(cmap[12:], uint32(len(keys)))
	for i, character := range keys {
		p := 16 + i*12
		put32(cmap[p:], uint32(character))
		put32(cmap[p+4:], uint32(character))
		put32(cmap[p+8:], uint32(characters[character]))
	}
	return &TrueTypeFace{units: 1000, glyphCount: 6, metrics: 6, locaFormat: 1, cmap: cmap,
		tables: map[string][]byte{"hhea": hhea, "hmtx": hmtx, "loca": loca, "glyf": glyf}}
}

func stageFiveTestContext() *FontContext {
	ctx := NewFontContext()
	ctx.faces = append(ctx.faces,
		registeredFace{family: "primary", weight: 400, face: stageFiveTestFace(map[rune]uint16{'A': 1, 'Е': 2, 'А': 4})},
		registeredFace{family: "fallback", weight: 400, face: stageFiveTestFace(map[rune]uint16{'V': 1, 'Ё': 3, '\u0301': 5})},
	)
	return ctx
}

func stageFiveSFNT(face *TrueTypeFace) []byte {
	put16 := binary.BigEndian.PutUint16
	put32 := binary.BigEndian.PutUint32
	head := make([]byte, 54)
	put16(head[18:], face.units)
	put16(head[50:], 1)
	maxp := make([]byte, 6)
	put16(maxp[4:], face.glyphCount)
	cmap := make([]byte, 12+len(face.cmap))
	put16(cmap[2:], 1)
	put16(cmap[4:], 3)
	put16(cmap[6:], 10)
	put32(cmap[8:], 12)
	copy(cmap[12:], face.cmap)
	tables := []struct {
		tag  string
		data []byte
	}{
		{"head", head}, {"hhea", face.tables["hhea"]}, {"maxp", maxp},
		{"hmtx", face.tables["hmtx"]}, {"loca", face.tables["loca"]},
		{"glyf", face.tables["glyf"]}, {"cmap", cmap},
	}
	data := make([]byte, 12+len(tables)*16)
	put32(data[0:], 0x00010000)
	put16(data[4:], uint16(len(tables)))
	for i, table := range tables {
		p := 12 + i*16
		copy(data[p:], table.tag)
		put32(data[p+8:], uint32(len(data)))
		put32(data[p+12:], uint32(len(table.data)))
		data = append(data, table.data...)
	}
	return data
}

func nearStageFive(a, b float32) bool { return math.Abs(float64(a-b)) < 0.01 }

func TestFontContextClusterFallbackAndCaretPositions(t *testing.T) {
	ctx := stageFiveTestContext()
	spec := FontSpec{Family: "primary", Size: 16}
	line := ctx.Layout("AV", spec)
	if len(line.Glyphs) != 2 || line.Glyphs[0].Face != 0 || line.Glyphs[1].Face != 1 {
		t.Fatalf("fallback faces: %+v", line.Glyphs)
	}
	if !nearStageFive(line.Carets[0], 0) || !nearStageFive(line.Carets[1], 9.6) || !nearStageFive(line.Carets[2], 19.2) ||
		!nearStageFive(line.Advance, ctx.Measure("AV", spec).X) || !nearStageFive(ctx.CaretX("AV", spec, 2), line.Advance) {
		t.Fatalf("inconsistent positions: %v, advance %v", line.Carets, line.Advance)
	}
	composed := ctx.Layout("Е\u0308", spec)
	if len(composed.Glyphs) != 1 || composed.Glyphs[0].Face != 1 || composed.Glyphs[0].ID != 3 ||
		composed.Carets[1] != composed.Carets[0] || composed.Carets[2] != composed.Advance {
		t.Fatalf("decomposed Cyrillic cluster: %+v", composed)
	}
	accent := ctx.Layout("А\u0301", spec)
	if len(accent.Glyphs) != 2 || accent.Glyphs[0].Face != 0 || accent.Glyphs[1].Face != 1 ||
		accent.Carets[1] != accent.Carets[0] || accent.Carets[2] != accent.Advance {
		t.Fatalf("combining accent fallback: %+v", accent)
	}
}

func TestFontContextPhysicalScaleStyleAndEviction(t *testing.T) {
	ctx := stageFiveTestContext()
	regular := ctx.Layout("A", FontSpec{Size: 16})
	again := ctx.Layout("A", FontSpec{Size: 16})
	if regular.Glyphs[0].Mask != again.Glyphs[0].Mask {
		t.Fatal("same physical glyph should reuse its mask")
	}
	scaled := ctx.Layout("A", FontSpec{Size: 16, Scale: 2})
	if scaled.Glyphs[0].Mask.Bounds().Dx() <= regular.Glyphs[0].Mask.Bounds().Dx() ||
		!nearStageFive(scaled.Advance, regular.Advance) {
		t.Fatal("physical raster size changed logical advance")
	}
	styled := ctx.Layout("A", FontSpec{Size: 16, Weight: 700, Italic: true})
	if styled.Glyphs[0].Mask == regular.Glyphs[0].Mask || styled.Glyphs[0].Mask.Bounds().Dx() <= regular.Glyphs[0].Mask.Bounds().Dx() {
		t.Fatal("synthetic style did not produce a separate raster")
	}
	ctx.SetCacheLimit(1)
	if used, count := ctx.CacheUsage(); used > 1 || count != 0 {
		t.Fatalf("cache budget: %d bytes, %d entries", used, count)
	}
	if regular.Glyphs[0].Mask.Pix[0] != again.Glyphs[0].Mask.Pix[0] {
		t.Fatal("eviction mutated an in-flight mask")
	}
	if ctx.Layout("A", FontSpec{Size: 16}).Glyphs[0].Mask == regular.Glyphs[0].Mask {
		t.Fatal("evicted mask reused")
	}
}

func TestFontContextRegistrationStyleAndWrap(t *testing.T) {
	data := stageFiveSFNT(stageFiveTestFace(map[rune]uint16{'A': 1, 'V': 2}))
	ctx := NewFontContext()
	if err := ctx.AddFont("sample", 400, false, data); err != nil {
		t.Fatal(err)
	}
	data[0] = 0
	bold := stageFiveSFNT(stageFiveTestFace(map[rune]uint16{'A': 1, 'V': 2}))
	if err := ctx.AddFont("sample", 700, false, bold); err != nil {
		t.Fatal(err)
	}
	if err := ctx.AddFont("broken", 400, false, nil); err == nil {
		t.Fatal("invalid font accepted")
	}
	spec := FontSpec{Family: "sample", Weight: 700, Size: 16}
	line := ctx.Layout("AV", spec)
	if len(line.Glyphs) != 2 || line.Glyphs[0].Face != 1 || line.Glyphs[1].Face != 1 {
		t.Fatalf("requested registered style was not selected: %+v", line.Glyphs)
	}
	wrapped := wrapTextWith("AV AV", 20, 16, func(s string, _ float32) float32 { return ctx.Layout(s, spec).Advance })
	if wrapped != "AV\nAV" {
		t.Fatalf("font-aware wrap: %q", wrapped)
	}
}

func TestFontContextEllipsisPreservesCluster(t *testing.T) {
	ctx := stageFiveTestContext()
	spec := FontSpec{Size: 16}
	text := ctx.Ellipsize("Е\u0308VVVV", 40, spec)
	if text != "Е\u0308..." || ctx.Layout(text, spec).Advance > 40 {
		t.Fatalf("ellipsis broke a grapheme or exceeded width: %q", text)
	}
}

func TestFontContextCanvasAndTextFieldUseSameLayout(t *testing.T) {
	ctx := stageFiveTestContext()
	spec := FontSpec{Family: "primary", Size: 16, Scale: 2}
	var commands []DrawCommand
	theme := DefaultTheme()
	canvas := Canvas{commands: &commands, clips: []Rect{{W: 200, H: 60}}, theme: &theme, fonts: ctx}
	canvas.TextStyled("AV", Rect{X: 10, Y: 0, W: 100, H: 40}, spec, TextLeft, theme.Text)
	line := ctx.Layout("AV", spec)
	if len(commands) != 2 || commands[0].Kind != CommandMask || commands[1].Kind != CommandMask ||
		!nearStageFive(commands[1].Rect.X-commands[0].Rect.X, line.Glyphs[1].X-line.Glyphs[0].X) {
		t.Fatalf("paint positions differ from the layout: %+v", commands)
	}
	field := NewTextField("AV", nil)
	field.Font = spec
	manager := New()
	manager.SetFontContext(ctx)
	manager.SetRoot(field)
	manager.BeginFrame(240, 48, 0)
	inner, r, lines, h := field.textGeometry()
	field.editor.Move(1, false)
	point := field.caretPoint(inner, r, lines, h)
	if !nearStageFive(point.X-inner.X, ctx.CaretX("AV", spec, 1)) || field.positionAt(point.X, point.Y) != 1 {
		t.Fatalf("caret/hit-test drift: point %v, caret %v", point, ctx.CaretX("AV", spec, 1))
	}
	if len(manager.Frame().Commands) == 0 || ctx.Measure("AV", spec).X != line.Advance {
		t.Fatal("manager font context was ignored")
	}
}

func TestTextEditorUndoByteLimit(t *testing.T) {
	e := NewTextEditor("initial document")
	e.UndoByteLimit = 4
	e.SelectAll()
	e.Replace("ab")
	e.SelectAll()
	e.Replace("cd")
	e.SelectAll()
	e.Replace("ef")
	if !e.Undo() || e.Text != "cd" || !e.Undo() || e.Text != "ab" || e.Undo() {
		t.Fatalf("undo exceeded byte limit: text %q, snapshots %d", e.Text, len(e.undo))
	}
	used := 0
	for _, snapshot := range e.undo {
		used += len(snapshot.text)
	}
	for _, snapshot := range e.redo {
		used += len(snapshot.text)
	}
	if used > e.UndoByteLimit {
		t.Fatalf("combined undo/redo memory exceeded budget: %d", used)
	}
	if !e.Redo() || e.Text != "cd" || !e.Redo() || e.Text != "ef" {
		t.Fatal("redo history lost within the budget")
	}
}

func TestPasswordClusterOffsetsWithFontContext(t *testing.T) {
	ctx := stageFiveTestContext()
	field := NewTextField("Е\u0308A", nil)
	field.Password = true
	manager := New()
	manager.SetFontContext(ctx)
	manager.SetRoot(field)
	masked := field.displayRunes()
	if string(masked) != "*\u034f*" || nextGrapheme(masked, 0) != 2 {
		t.Fatalf("password split a grapheme cluster: %q", string(masked))
	}
	line := ctx.Layout(string(masked), FontSpec{Size: field.Theme().FontSize})
	if len(line.Carets) != 4 || line.Carets[0] != line.Carets[1] || line.Carets[2] <= line.Carets[1] {
		t.Fatalf("password caret positions: %v", line.Carets)
	}
}
