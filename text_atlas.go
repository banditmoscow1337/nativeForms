package nativeforms

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
