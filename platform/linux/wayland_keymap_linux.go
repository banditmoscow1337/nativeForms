//go:build linux

package linux

import (
	"regexp"
	"strconv"
	"strings"
	"sync"
	"unicode"
)

type xkbType struct {
	mask   uint32
	levels map[uint32]int
}
type xkbGroup struct {
	symbols   []uint32
	sequences [][]uint32
	typ       xkbType
}
type xkbKey struct {
	groups   []xkbGroup
	repeat   bool
	clamp    bool
	redirect int
}
type waylandSymbols map[uint32]xkbKey

var keyNames map[string]uint32
var keyRunes map[uint32]rune
var keyNamesOnce sync.Once

func initKeyNames() {
	keyNamesOnce.Do(func() {
		keyNames = make(map[string]uint32)
		keyRunes = make(map[uint32]rune)
		p := strings.Fields(xKeysymData)
		for i := 0; i+2 < len(p); i += 3 {
			n, _ := strconv.ParseUint(p[i+1], 16, 32)
			keyNames[p[i]] = uint32(n)
			if p[i+2] != "-" {
				r, _ := strconv.ParseUint(p[i+2], 16, 32)
				keyRunes[uint32(n)] = rune(r)
			}
		}
	})
}
func keysym(name string) uint32 {
	initKeyNames()
	if n, ok := keyNames[name]; ok {
		return n
	}
	if strings.HasPrefix(name, "U") {
		if v, e := strconv.ParseUint(name[1:], 16, 32); e == nil && v <= unicode.MaxRune {
			return uint32(v) | 0x1000000
		}
	}
	if strings.HasPrefix(name, "0x") {
		v, _ := strconv.ParseUint(name[2:], 16, 32)
		return uint32(v)
	}
	return 0
}
func keysymRune(sym uint32) rune {
	initKeyNames()
	if sym&0xff000000 == 0x1000000 {
		r := rune(sym & 0xffffff)
		if r <= unicode.MaxRune && (r < 0xd800 || r > 0xdfff) {
			return r
		}
		return 0
	}
	return keyRunes[sym]
}

var (
	xkbCode       = regexp.MustCompile(`<([A-Za-z0-9_]+)>\s*=\s*([0-9]+)\s*;`)
	xkbAlias      = regexp.MustCompile(`alias\s*<([A-Za-z0-9_]+)>\s*=\s*<([A-Za-z0-9_]+)>`)
	xkbRecord     = regexp.MustCompile(`\bkey\s*<([A-Za-z0-9_]+)>\s*\{`)
	xkbActions    = regexp.MustCompile(`(?s)actions\s*\[\s*Group[0-9]+\s*\]\s*=\s*\[[^]]*\]`)
	xkbTypeRecord = regexp.MustCompile(`(?s)\btype\s*"([^"]+)"\s*\{([^}]+)\}`)
	xkbMod        = regexp.MustCompile(`modifiers\s*=\s*([^;]+)`)
	xkbMap        = regexp.MustCompile(`map\[([^]]+)\]\s*=\s*Level([0-9]+)`)
	xkbVMods      = regexp.MustCompile(`virtual_modifiers\s+([^;]+)`)
	xkbSymbols    = regexp.MustCompile(`(?:symbols\s*\[\s*Group([0-9]+)\s*\]\s*=\s*)?\[([^\[\]]*)\]`)
	xkbTypeName   = regexp.MustCompile(`type(?:\s*\[\s*Group([0-9]+)\s*\])?\s*=\s*"([^"]+)"`)
	xkbRedirect   = regexp.MustCompile(`groupsRedirect\s*=\s*Group([0-9]+)`)
	xkbNoRepeat   = regexp.MustCompile(`(?i)repeat\s*=\s*(?:No|False)`)
	xkbComments   = regexp.MustCompile(`(?m)//[^\n]*|(?s:/\*.*?\*/)`)
)

func parseWaylandKeymap(data []byte) waylandSymbols {
	text := xkbComments.ReplaceAllString(strings.TrimRight(string(data), "\x00"), "")
	mods := map[string]uint32{"Shift": 1, "Lock": 2, "Control": 4, "Mod1": 8, "Mod2": 16, "Mod3": 32, "Mod4": 64, "Mod5": 128}
	if match := xkbVMods.FindStringSubmatch(text); len(match) > 0 {
		for i, s := range strings.Split(match[1], ",") {
			if i < 24 {
				mods[strings.TrimSpace(s)] = 1 << uint(i+8)
			}
		}
	}
	mask := func(s string) uint32 {
		var m uint32
		for _, p := range strings.Split(s, "+") {
			m |= mods[strings.TrimSpace(p)]
		}
		return m
	}
	types := make(map[string]xkbType)
	for _, m := range xkbTypeRecord.FindAllStringSubmatch(text, -1) {
		t := xkbType{levels: map[uint32]int{0: 0}}
		if a := xkbMod.FindStringSubmatch(m[2]); len(a) > 0 {
			t.mask = mask(a[1])
		}
		for _, a := range xkbMap.FindAllStringSubmatch(m[2], -1) {
			n, _ := strconv.Atoi(a[2])
			if n > 0 && n <= 32 {
				t.levels[mask(a[1])] = n - 1
			}
		}
		types[m[1]] = t
	}
	codes := make(map[string]uint32)
	for _, m := range xkbCode.FindAllStringSubmatch(text, -1) {
		n, _ := strconv.ParseUint(m[2], 10, 32)
		codes[m[1]] = uint32(n)
	}
	for _, m := range xkbAlias.FindAllStringSubmatch(text, -1) {
		codes[m[1]] = codes[m[2]]
	}
	result := make(waylandSymbols)
	for _, loc := range xkbRecord.FindAllStringSubmatchIndex(text, -1) {
		body := xkbBlock(text, loc[1]-1)
		m := []string{"", text[loc[2]:loc[3]], xkbActions.ReplaceAllString(body, "")}
		code := codes[m[1]]
		if code == 0 {
			continue
		}
		key := xkbKey{repeat: !xkbNoRepeat.MatchString(m[2]), clamp: strings.Contains(m[2], "groupsClamp"), redirect: -1}
		if match := xkbRedirect.FindStringSubmatch(m[2]); len(match) > 0 {
			key.redirect, _ = strconv.Atoi(match[1])
			key.redirect--
		}
		typeNames := map[int]string{}
		for _, a := range xkbTypeName.FindAllStringSubmatch(m[2], -1) {
			g := 0
			if a[1] != "" {
				g, _ = strconv.Atoi(a[1])
				g--
			}
			typeNames[g] = a[2]
		}
		next := 0
		for _, a := range xkbSymbols.FindAllStringSubmatch(m[2], -1) {
			if strings.HasPrefix(strings.TrimSpace(a[2]), "Group") {
				continue
			}
			g := next
			if a[1] != "" {
				g, _ = strconv.Atoi(a[1])
				g--
			}
			if g < 0 || g > 3 {
				continue
			}
			next = g + 1
			for len(key.groups) <= g {
				key.groups = append(key.groups, xkbGroup{})
			}
			group := xkbGroup{}
			group.sequences = xkbLevels(a[2])
			for _, level := range group.sequences {
				sym := uint32(0)
				if len(level) > 0 {
					sym = level[0]
				}
				group.symbols = append(group.symbols, sym)
			}
			name := typeNames[g]
			if name == "" {
				name = typeNames[0]
			}
			if name == "" {
				name = "ONE_LEVEL"
				if len(group.symbols) >= 2 {
					name = "TWO_LEVEL"
					lo, hi := keysymRune(group.symbols[0]), keysymRune(group.symbols[1])
					if unicode.IsLower(lo) && unicode.ToUpper(lo) == hi {
						name = "ALPHABETIC"
					}
				}
				if len(group.symbols) > 2 {
					name = "FOUR_LEVEL"
				}
			}
			group.typ = types[name]
			if group.typ.levels == nil {
				group.typ = xkbType{mask: 1, levels: map[uint32]int{0: 0, 1: 1}}
				if name == "ALPHABETIC" {
					group.typ = xkbType{mask: 3, levels: map[uint32]int{0: 0, 1: 1, 2: 1, 3: 0}}
				}
			}
			key.groups[g] = group
		}
		if len(key.groups) > 0 {
			if len(key.groups[0].symbols) > 0 {
				s := key.groups[0].symbols[0]
				if s >= 0xffe1 && s <= 0xffee || s == 0xff7f || s == 0xfe03 {
					key.repeat = false
				}
			}
			result[code] = key
		}
	}
	if len(result) == 0 {
		return nil
	}
	return result
}
func (k waylandSymbols) symbols(code, group, mods uint32) []uint32 {
	key := k[code+8]
	if len(key.groups) == 0 {
		return nil
	}
	index := int(group)
	if index >= len(key.groups) {
		if key.clamp {
			index = len(key.groups) - 1
		} else if key.redirect >= 0 && key.redirect < len(key.groups) {
			index = key.redirect
		} else {
			index %= len(key.groups)
		}
	}
	g := key.groups[index]
	if len(g.symbols) == 0 {
		return nil
	}
	level := g.typ.levels[mods&g.typ.mask]
	if level >= len(g.symbols) {
		level = 0
	}
	return g.sequences[level]
}
func (k waylandSymbols) symbol(code, group, mods uint32) uint32 {
	syms := k.symbols(code, group, mods)
	if len(syms) > 0 {
		return syms[0]
	}
	return 0
}
func xkbLevels(text string) [][]uint32 {
	var levels [][]uint32
	start, depth := 0, 0
	appendLevel := func(s string) {
		s = strings.TrimSpace(s)
		s = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(s, "{"), "}"))
		var symbols []uint32
		for _, name := range strings.Split(s, ",") {
			symbols = append(symbols, keysym(strings.TrimSpace(name)))
		}
		levels = append(levels, symbols)
	}
	for i, c := range text {
		if c == '{' {
			depth++
		}
		if c == '}' {
			depth--
		}
		if c == ',' && depth == 0 {
			appendLevel(text[start:i])
			start = i + 1
		}
	}
	appendLevel(text[start:])
	return levels
}

func xkbBlock(text string, open int) string {
	depth := 0
	quoted := false
	escaped := false
	for i := open; i < len(text); i++ {
		c := text[i]
		if quoted {
			if escaped {
				escaped = false
			} else if c == '\\' {
				escaped = true
			} else if c == '"' {
				quoted = false
			}
			continue
		}
		if c == '"' {
			quoted = true
			continue
		}
		if c == '{' {
			depth++
		}
		if c == '}' {
			depth--
			if depth == 0 {
				return text[open+1 : i]
			}
		}
	}
	return ""
}
