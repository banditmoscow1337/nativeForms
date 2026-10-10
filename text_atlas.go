package nativeforms

// walkAtlasText is the portable LTR positioning path shared by measurement,
// paint and hit testing. carets, when non-nil, has runeCount(text)+1 entries.
// emit receives an atlas cell and its pen position in logical pixels.
func walkAtlasText(text string, size float32, carets []float32, emit func(glyphInfo, float32)) float32 {
	runes := []rune(text)
	var previous glyphInfo
	x := float32(0)
	for i := 0; i < len(runes); {
		character, count := runes[i], 1
		if i+1 < len(runes) {
			if composed := composeRussian(character, runes[i+1]); composed != 0 {
				character, count = composed, 2
			}
		}
		glyph := glyphForRune(character)
		if glyph.face != nil && glyph.face == previous.face {
			// Keep caret positions monotone even with pathological font data.
			x += maxFloat32(-previous.advance*size, glyph.face.atlasKerning(previous.id, glyph.id)*size)
		}
		if carets != nil {
			for j := 0; j < count; j++ {
				carets[i+j] = x
			}
		}
		if emit != nil && glyph.visible {
			emit(glyph, x)
		}
		x += maxFloat32(0, glyph.advance*size)
		previous = glyph
		i += count
	}
	if carets != nil {
		carets[len(runes)] = x
	}
	return x
}

// Common canonically decomposed Russian letters can use existing atlas cells
// without changing the application's text or its rune/UTF-8 selection offsets.
// This is deliberately not a general Unicode normalizer or mark positioner.
func composeRussian(base, mark rune) rune {
	if mark == '\u0308' {
		switch base {
		case 'Е':
			return 'Ё'
		case 'е':
			return 'ё'
		}
	}
	if mark == '\u0306' {
		switch base {
		case 'И':
			return 'Й'
		case 'и':
			return 'й'
		}
	}
	return 0
}
