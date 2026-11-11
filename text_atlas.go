package nativeforms

import "unicode/utf8"

func decodeComposedTextRune(text string, i int) (character rune, width, count int) {
	character, width = utf8.DecodeRuneInString(text[i:])
	count = 1
	// In UTF-8, Combining Diacritical Marks (U+0300..U+033F) start with lead byte 0xCC.
	if i+width < len(text) && text[i+width] == 0xcc {
		nextRune, nextWidth := utf8.DecodeRuneInString(text[i+width:])
		if composed := composeCombiningRune(character, nextRune); composed != 0 {
			character, count = composed, 2
			width += nextWidth
		}
	}
	return character, width, count
}

func decodeComposedSliceRune(runes []rune, i int) (character rune, count int) {
	character, count = runes[i], 1
	if i+1 < len(runes) {
		if composed := composeCombiningRune(character, runes[i+1]); composed != 0 {
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

// composeCombiningRune maps common NFD base + combining diacritical mark pairs
// (Cyrillic and Latin) to their precomposed NFC codepoints.
func composeCombiningRune(base, mark rune) rune {
	switch mark {
	case '\u0300': // Combining Grave Accent
		switch base {
		case 'A':
			return 'À'
		case 'a':
			return 'à'
		case 'E':
			return 'È'
		case 'e':
			return 'è'
		case 'I':
			return 'Ì'
		case 'i':
			return 'ì'
		case 'O':
			return 'Ò'
		case 'o':
			return 'ò'
		case 'U':
			return 'Ù'
		case 'u':
			return 'ù'
		case 'Е':
			return 'Ѐ'
		case 'е':
			return 'ѐ'
		case 'И':
			return 'Ѝ'
		case 'и':
			return 'ѝ'
		}
	case '\u0301': // Combining Acute Accent
		switch base {
		case 'A':
			return 'Á'
		case 'a':
			return 'á'
		case 'C':
			return 'Ć'
		case 'c':
			return 'ć'
		case 'E':
			return 'É'
		case 'e':
			return 'é'
		case 'I':
			return 'Í'
		case 'i':
			return 'í'
		case 'N':
			return 'Ń'
		case 'n':
			return 'ń'
		case 'O':
			return 'Ó'
		case 'o':
			return 'ó'
		case 'S':
			return 'Ś'
		case 's':
			return 'ś'
		case 'U':
			return 'Ú'
		case 'u':
			return 'ú'
		case 'Y':
			return 'Ý'
		case 'y':
			return 'ý'
		case 'Z':
			return 'Ź'
		case 'z':
			return 'ź'
		case 'Г':
			return 'Ѓ'
		case 'г':
			return 'ѓ'
		case 'К':
			return 'Ќ'
		case 'к':
			return 'ќ'
		}
	case '\u0302': // Combining Circumflex Accent
		switch base {
		case 'A':
			return 'Â'
		case 'a':
			return 'â'
		case 'E':
			return 'Ê'
		case 'e':
			return 'ê'
		case 'I':
			return 'Î'
		case 'i':
			return 'î'
		case 'O':
			return 'Ô'
		case 'o':
			return 'ô'
		case 'U':
			return 'Û'
		case 'u':
			return 'û'
		}
	case '\u0303': // Combining Tilde
		switch base {
		case 'A':
			return 'Ã'
		case 'a':
			return 'ã'
		case 'N':
			return 'Ñ'
		case 'n':
			return 'ñ'
		case 'O':
			return 'Õ'
		case 'o':
			return 'õ'
		}
	case '\u0304': // Combining Macron
		switch base {
		case 'A':
			return 'Ā'
		case 'a':
			return 'ā'
		case 'E':
			return 'Ē'
		case 'e':
			return 'ē'
		case 'I':
			return 'Ī'
		case 'i':
			return 'ī'
		case 'O':
			return 'Ō'
		case 'o':
			return 'ō'
		case 'U':
			return 'Ū'
		case 'u':
			return 'ū'
		}
	case '\u0306': // Combining Breve
		switch base {
		case 'И':
			return 'Й'
		case 'и':
			return 'й'
		case 'У':
			return 'Ў'
		case 'у':
			return 'ў'
		case 'Ж':
			return 'Ӂ'
		case 'ж':
			return 'ӂ'
		case 'A':
			return 'Ă'
		case 'a':
			return 'ă'
		case 'G':
			return 'Ğ'
		case 'g':
			return 'ğ'
		}
	case '\u0307': // Combining Dot Above
		switch base {
		case 'E':
			return 'Ė'
		case 'e':
			return 'ė'
		case 'I':
			return 'İ'
		case 'Z':
			return 'Ż'
		case 'z':
			return 'ż'
		}
	case '\u0308': // Combining Diaeresis
		switch base {
		case 'Е':
			return 'Ё'
		case 'е':
			return 'ё'
		case 'І':
			return 'Ї'
		case 'і':
			return 'ї'
		case 'A':
			return 'Ä'
		case 'a':
			return 'ä'
		case 'E':
			return 'Ë'
		case 'e':
			return 'ë'
		case 'I':
			return 'Ï'
		case 'i':
			return 'ï'
		case 'O':
			return 'Ö'
		case 'o':
			return 'ö'
		case 'U':
			return 'Ü'
		case 'u':
			return 'ü'
		case 'Y':
			return 'Ÿ'
		case 'y':
			return 'ÿ'
		}
	case '\u030a': // Combining Ring Above
		switch base {
		case 'A':
			return 'Å'
		case 'a':
			return 'å'
		case 'U':
			return 'Ů'
		case 'u':
			return 'ů'
		}
	case '\u030c': // Combining Caron
		switch base {
		case 'C':
			return 'Č'
		case 'c':
			return 'č'
		case 'D':
			return 'Ď'
		case 'd':
			return 'ď'
		case 'E':
			return 'Ě'
		case 'e':
			return 'ě'
		case 'N':
			return 'Ň'
		case 'n':
			return 'ň'
		case 'R':
			return 'Ř'
		case 'r':
			return 'ř'
		case 'S':
			return 'Š'
		case 's':
			return 'š'
		case 'T':
			return 'Ť'
		case 't':
			return 'ť'
		case 'Z':
			return 'Ž'
		case 'z':
			return 'ž'
		}
	case '\u0327': // Combining Cedilla
		switch base {
		case 'C':
			return 'Ç'
		case 'c':
			return 'ç'
		case 'S':
			return 'Ş'
		case 's':
			return 'ş'
		case 'T':
			return 'Ţ'
		case 't':
			return 'ţ'
		}
	}
	return 0
}

func composeRussian(base, mark rune) rune {
	return composeCombiningRune(base, mark)
}
