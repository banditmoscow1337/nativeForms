package nativeforms

import "unicode/utf8"

func decodeComposedTextRune(text string, i int) (character rune, width, count int) {
	character, width = utf8.DecodeRuneInString(text[i:])
	count = 1
	if i+width < len(text) && (character == 'Е' || character == 'е' || character == 'И' || character == 'и') {
		nextRune, nextWidth := utf8.DecodeRuneInString(text[i+width:])
		if composed := composeRussian(character, nextRune); composed != 0 {
			character, count = composed, 2
			width += nextWidth
		}
	}
	return character, width, count
}

func decodeComposedSliceRune(runes []rune, i int) (character rune, count int) {
	character, count = runes[i], 1
	if i+1 < len(runes) {
		if composed := composeRussian(character, runes[i+1]); composed != 0 {
			character, count = composed, 2
		}
	}
	return character, count
}

func stepAtlasGlyph(previous glyphInfo, character rune, size, x float32) (glyph glyphInfo, glyphX, nextX float32) {
	glyph = glyphForRune(character)
	if glyph.face != nil && glyph.face == previous.face {
		x += maxFloat32(-previous.advance*size, glyph.face.atlasKerning(previous.id, glyph.id)*size)
	}
	return glyph, x, x + maxFloat32(0, glyph.advance*size)
}

func walkAtlasText(text string, size float32, carets []float32, emit func(glyphInfo, float32)) float32 {
	var previous glyphInfo
	x := float32(0)
	runeIdx := 0
	for i := 0; i < len(text); {
		character, width, count := decodeComposedTextRune(text, i)
		glyph, gx, nextX := stepAtlasGlyph(previous, character, size, x)
		if carets != nil {
			for j := 0; j < count && runeIdx+j < len(carets); j++ {
				carets[runeIdx+j] = gx
			}
		}
		if emit != nil && glyph.visible {
			emit(glyph, gx)
		}
		previous, x = glyph, nextX
		i += width
		runeIdx += count
	}
	if carets != nil && runeIdx < len(carets) {
		carets[runeIdx] = x
	}
	return x
}

func walkAtlasCaretX(text string, size float32, runeIndex int) float32 {
	if runeIndex <= 0 {
		return 0
	}
	var previous glyphInfo
	x := float32(0)
	runeIdx := 0
	for i := 0; i < len(text); {
		character, width, count := decodeComposedTextRune(text, i)
		glyph, gx, nextX := stepAtlasGlyph(previous, character, size, x)
		if runeIndex < runeIdx+count {
			return gx
		}
		previous, x = glyph, nextX
		i += width
		runeIdx += count
	}
	return x
}

func walkAtlasRunesCaretX(runes []rune, size float32, runeIndex int) float32 {
	if runeIndex <= 0 {
		return 0
	}
	var previous glyphInfo
	x := float32(0)
	for i := 0; i < len(runes); {
		character, count := decodeComposedSliceRune(runes, i)
		glyph, gx, nextX := stepAtlasGlyph(previous, character, size, x)
		if runeIndex < i+count {
			return gx
		}
		previous, x = glyph, nextX
		i += count
	}
	return x
}

func walkAtlasRunes(runes []rune, size float32, carets []float32, emit func(glyphInfo, float32)) float32 {
	var previous glyphInfo
	x := float32(0)
	for i := 0; i < len(runes); {
		character, count := decodeComposedSliceRune(runes, i)
		glyph, gx, nextX := stepAtlasGlyph(previous, character, size, x)
		if carets != nil {
			for j := 0; j < count; j++ {
				carets[i+j] = gx
			}
		}
		if emit != nil && glyph.visible {
			emit(glyph, gx)
		}
		previous, x = glyph, nextX
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
