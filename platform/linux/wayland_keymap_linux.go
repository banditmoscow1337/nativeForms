//go:build linux

package linux

import (
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

// A small first-party reader for the ordinary keycode/symbol records in an
// XKB text keymap. Unknown symbols do not produce text; no host XKB library
// is loaded. This is intentionally limited to one symbol per level.
type waylandSymbols map[uint32][][]rune

var (
	waylandKeycodeRecord = regexp.MustCompile(`<([A-Za-z0-9_]+)>\s*=\s*([0-9]+)\s*;`)
	waylandSymbolRecord  = regexp.MustCompile(`(?s)\bkey\s*<([A-Za-z0-9_]+)>\s*\{([^}]*)\}`)
	waylandSymbolGroup   = regexp.MustCompile(`\[([^]]*)\]`)
)

func keymapSection(text, name string) string {
	start := strings.Index(text, name)
	if start < 0 {
		return ""
	}
	open := strings.IndexByte(text[start:], '{')
	if open < 0 {
		return ""
	}
	start += open + 1
	depth := 1
	for i := start; i < len(text); i++ {
		switch text[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return text[start:i]
			}
		}
	}
	return ""
}

func parseWaylandKeymap(data []byte) waylandSymbols {
	text := strings.TrimRight(string(data), "\x00")
	codes, symbols := keymapSection(text, "xkb_keycodes"), keymapSection(text, "xkb_symbols")
	if codes == "" || symbols == "" {
		return nil
	}
	byName := make(map[string]uint32)
	for _, match := range waylandKeycodeRecord.FindAllStringSubmatch(codes, -1) {
		n, err := strconv.ParseUint(match[2], 10, 32)
		if err == nil {
			byName[match[1]] = uint32(n)
		}
	}
	result := make(waylandSymbols)
	for _, match := range waylandSymbolRecord.FindAllStringSubmatch(symbols, -1) {
		code, ok := byName[match[1]]
		if !ok {
			continue
		}
		var groups [][]rune
		for _, group := range waylandSymbolGroup.FindAllStringSubmatch(match[2], -1) {
			parts := strings.Split(group[1], ",")
			levels := make([]rune, 0, min(2, len(parts)))
			for _, part := range parts {
				if len(levels) == 2 {
					break
				}
				levels = append(levels, waylandKeysymRune(strings.TrimSpace(part)))
			}
			if len(levels) != 0 {
				groups = append(groups, levels)
			}
		}
		if len(groups) != 0 {
			result[code] = groups
		}
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

func (symbols waylandSymbols) character(evdevCode, group uint32, shift bool) rune {
	groups := symbols[evdevCode+8]
	if len(groups) == 0 {
		return 0
	}
	if int(group) >= len(groups) {
		group = 0
	}
	level := groups[group]
	if len(level) == 0 {
		return 0
	}
	if shift && len(level) > 1 {
		return level[1]
	}
	return level[0]
}

var waylandSymbolNames = map[string]rune{
	"space": ' ', "exclam": '!', "quotedbl": '"', "numbersign": '#', "dollar": '$',
	"percent": '%', "ampersand": '&', "apostrophe": '\'', "quoteright": '\'',
	"parenleft": '(', "parenright": ')', "asterisk": '*', "plus": '+', "comma": ',',
	"minus": '-', "period": '.', "slash": '/', "colon": ':', "semicolon": ';',
	"less": '<', "equal": '=', "greater": '>', "question": '?', "at": '@',
	"bracketleft": '[', "backslash": '\\', "bracketright": ']', "asciicircum": '^',
	"underscore": '_', "grave": '`', "quoteleft": '`', "braceleft": '{',
	"bar": '|', "braceright": '}', "asciitilde": '~', "nobreakspace": '\u00a0',
	"EuroSign": '€', "Cyrillic_io": 'ё', "Cyrillic_yu": 'ю', "Cyrillic_ya": 'я',
	"Cyrillic_ie": 'е', "Cyrillic_i": 'и', "Cyrillic_shorti": 'й',
	"Cyrillic_ef": 'ф', "Cyrillic_ghe": 'г', "Cyrillic_ha": 'х',
	"Cyrillic_ka": 'к', "Cyrillic_el": 'л', "Cyrillic_em": 'м',
	"Cyrillic_en": 'н', "Cyrillic_o": 'о', "Cyrillic_pe": 'п',
	"Cyrillic_er": 'р', "Cyrillic_es": 'с', "Cyrillic_te": 'т',
	"Cyrillic_u": 'у', "Cyrillic_zhe": 'ж', "Cyrillic_ve": 'в',
	"Cyrillic_softsign": 'ь', "Cyrillic_yeru": 'ы', "Cyrillic_ze": 'з',
	"Cyrillic_sha": 'ш', "Cyrillic_e": 'э', "Cyrillic_shcha": 'щ',
	"Cyrillic_che": 'ч', "Cyrillic_hardsign": 'ъ', "Cyrillic_be": 'б',
	"Cyrillic_de": 'д', "Cyrillic_tse": 'ц', "Cyrillic_a": 'а',
}

func waylandKeysymRune(name string) rune {
	if len(name) == 1 {
		return rune(name[0])
	}
	if r, ok := waylandSymbolNames[name]; ok {
		return r
	}
	if strings.HasPrefix(name, "Cyrillic_") {
		lower := "Cyrillic_" + strings.ToLower(strings.TrimPrefix(name, "Cyrillic_"))
		if r, ok := waylandSymbolNames[lower]; ok {
			return unicode.ToUpper(r)
		}
	}
	if len(name) > 1 && name[0] == 'U' {
		if value, err := strconv.ParseUint(name[1:], 16, 32); err == nil && value <= unicode.MaxRune {
			return rune(value)
		}
	}
	return 0
}
