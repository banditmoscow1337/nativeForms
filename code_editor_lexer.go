package nativeforms

import (
	"errors"
	"go/token"
	"unicode"
	"unicode/utf8"
)

// CodeStyle names the foreground style of a source range. CodePlain uses the
// current theme's text color; CodeEditor.Palette can override any style.
type CodeStyle uint8

const (
	CodePlain CodeStyle = iota
	CodeKeyword
	CodeComment
	CodeString
	CodeNumber
	CodeType
	CodeFunction
	CodeVariable
)

// StyledRange uses absolute UTF-8 byte offsets in the editor's Document.
type StyledRange struct {
	Start, End int
	Style      CodeStyle
}

var ErrStaleCodeStyle = errors.New("nativeforms: styles refer to an older document version")
var ErrInvalidCodeStyle = errors.New("nativeforms: invalid style or semantic token range")

type goLexState uint8

const (
	goNormal goLexState = iota
	goBlockComment
	goRawString
)

// lexGoLine returns local byte ranges and the state carried to the next line.
// Keeping the state per line lets the widget tokenize only newly visible lines.
func lexGoLine(line string, state goLexState) ([]StyledRange, goLexState) {
	return scanGoLine(line, state, true)
}

func scanGoLine(line string, state goLexState, styles bool) ([]StyledRange, goLexState) {
	var spans []StyledRange
	if styles {
		spans = make([]StyledRange, 0, 8)
	}
	return scanGoLineInto(line, state, styles, spans)
}

// advanceGoLexState computes the next line's carry state without allocating
// spans, decoding UTF-8 runes, or looking up Go keywords.
func advanceGoLexState(line string, state goLexState) goLexState {
	if state == goNormal {
		hasSpecial := false
		for i := 0; i < len(line); i++ {
			b := line[i]
			if b == '`' || (b == '/' && i+1 < len(line) && line[i+1] == '*') {
				hasSpecial = true
				break
			}
		}
		if !hasSpecial {
			return goNormal
		}
	}
	for at := 0; at < len(line); {
		switch state {
		case goBlockComment:
			for at < len(line) && (at+1 >= len(line) || line[at] != '*' || line[at+1] != '/') {
				at++
			}
			if at+1 < len(line) {
				at += 2
				state = goNormal
			}
			continue
		case goRawString:
			for at < len(line) && line[at] != '`' {
				at++
			}
			if at < len(line) {
				at++
				state = goNormal
			}
			continue
		}
		b := line[at]
		if b == '/' && at+1 < len(line) {
			if line[at+1] == '/' {
				break
			}
			if line[at+1] == '*' {
				at += 2
				state = goBlockComment
				continue
			}
		}
		if b == '`' {
			at++
			for at < len(line) && line[at] != '`' {
				at++
			}
			if at < len(line) {
				at++
			} else {
				state = goRawString
			}
			continue
		}
		if b == '"' || b == '\'' {
			quote := b
			at++
			for at < len(line) {
				if line[at] == '\\' {
					at = min(len(line), at+2)
					continue
				}
				at++
				if line[at-1] == quote {
					break
				}
			}
			continue
		}
		at++
	}
	return state
}

func skipSpaces(line string, pos int) int {
	for pos < len(line) && (line[pos] == ' ' || line[pos] == '\t') {
		pos++
	}
	return pos
}

func isUpperCamelIdentifier(word string) bool {
	if len(word) < 2 {
		return false
	}
	r, sz := utf8.DecodeRuneInString(word)
	if !unicode.IsUpper(r) {
		return false
	}
	hasLower := false
	for i := sz; i < len(word); {
		nr, nsz := utf8.DecodeRuneInString(word[i:])
		if unicode.IsLower(nr) {
			hasLower = true
			break
		}
		i += nsz
	}
	return hasLower
}

func isAllCapsConstant(word string) bool {
	if len(word) < 2 {
		return false
	}
	hasUpper := false
	for _, r := range word {
		if unicode.IsLower(r) {
			return false
		}
		if unicode.IsUpper(r) {
			hasUpper = true
		}
	}
	return hasUpper
}

// isPrecededByTypeOperator reports whether byte offset `start` on `line` is
// preceded by a Go type operator (`*`, `[]`, `]`, `...`, `.(`) or identifier
// in a declaration/parameter/field signature.
func isPrecededByTypeContext(line string, start int, prevKeyword string, prevWasIdent bool) bool {
	if prevKeyword == "type" || prevKeyword == "chan" {
		return true
	}
	i := start
	for i > 0 && (line[i-1] == ' ' || line[i-1] == '\t') {
		i--
	}
	// Skip optional package qualifier `pkg.`
	if i >= 2 && line[i-1] == '.' && line[i-2] != '.' && line[i-2] != '(' {
		j := i - 1
		for j > 0 && (unicode.IsLetter(rune(line[j-1])) || unicode.IsDigit(rune(line[j-1])) || line[j-1] == '_') {
			j--
		}
		if j < i-1 {
			i = j
			for i > 0 && (line[i-1] == ' ' || line[i-1] == '\t') {
				i--
			}
		}
	}
	// Skip pointer `*`, slice `[]`, or variadic `...` prefixes
	hadTypePrefix := false
	for i > 0 {
		if line[i-1] == '*' {
			// Check what precedes `*`: if it's whitespace, `(`, `[`, `,`, `]`, `:`, or start of line, it's a pointer type!
			if i-1 == 0 {
				return true
			}
			prev := line[i-2]
			if prev == ' ' || prev == '\t' || prev == '(' || prev == '[' || prev == ',' || prev == ']' || prev == ':' {
				return true
			}
			break
		}
		if line[i-1] == ']' {
			return true
		}
		if i >= 3 && line[i-3:i] == "..." {
			return true
		}
		if i >= 2 && line[i-2:i] == ".(" {
			return true
		}
		if i >= 2 && line[i-2:i] == "<-" {
			return true
		}
		break
	}
	if hadTypePrefix {
		return true
	}
	if prevWasIdent {
		return true
	}
	return false
}

func scanGoLineInto(line string, state goLexState, styles bool, spans []StyledRange) ([]StyledRange, goLexState) {
	spans = spans[:0]
	if !styles {
		return spans, advanceGoLexState(line, state)
	}
	appendStyle := func(start, end int, style CodeStyle) {
		if start < end && style != CodePlain {
			spans = append(spans, StyledRange{Start: start, End: end, Style: style})
		}
	}
	prevKeyword := ""
	prevWasIdent := false
	for at := 0; at < len(line); {
		start := at
		switch state {
		case goBlockComment:
			for at < len(line) && (at+1 >= len(line) || line[at] != '*' || line[at+1] != '/') {
				at++
			}
			if at+1 < len(line) {
				at += 2
				state = goNormal
			}
			appendStyle(start, at, CodeComment)
			prevKeyword = ""
			prevWasIdent = false
			continue
		case goRawString:
			for at < len(line) && line[at] != '`' {
				at++
			}
			if at < len(line) {
				at++
				state = goNormal
			}
			appendStyle(start, at, CodeString)
			prevKeyword = ""
			prevWasIdent = false
			continue
		}
		if at+1 < len(line) && line[at] == '/' && line[at+1] == '/' {
			appendStyle(at, len(line), CodeComment)
			break
		}
		if at+1 < len(line) && line[at] == '/' && line[at+1] == '*' {
			state = goBlockComment
			continue
		}
		if line[at] == '`' {
			at++
			for at < len(line) && line[at] != '`' {
				at++
			}
			if at < len(line) {
				at++
			} else {
				state = goRawString
			}
			appendStyle(start, at, CodeString)
			prevKeyword = ""
			prevWasIdent = false
			continue
		}
		if line[at] == '"' || line[at] == '\'' {
			quote := line[at]
			at++
			for at < len(line) {
				if line[at] == '\\' {
					at = min(len(line), at+2)
					continue
				}
				at++
				if line[at-1] == quote {
					break
				}
			}
			appendStyle(start, at, CodeString)
			prevKeyword = ""
			prevWasIdent = false
			continue
		}
		r, size := utf8.DecodeRuneInString(line[at:])
		if unicode.IsLetter(r) || r == '_' {
			firstRune := r
			at += size
			for at < len(line) {
				r, size = utf8.DecodeRuneInString(line[at:])
				if !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '_' {
					break
				}
				at += size
			}
			word := line[start:at]
			after := skipSpaces(line, at)
			nextByte := byte(0)
			if after < len(line) {
				nextByte = line[after]
			}
			switch {
			case token.Lookup(word).IsKeyword():
				appendStyle(start, at, CodeKeyword)
				prevKeyword = word
				prevWasIdent = false
			case word == "true" || word == "false" || word == "nil" || word == "iota":
				appendStyle(start, at, CodeNumber)
				prevKeyword = ""
				prevWasIdent = false
			case goBuiltinType(word):
				appendStyle(start, at, CodeType)
				prevKeyword = ""
				prevWasIdent = false
			case goBuiltinFunction(word) && nextByte == '(':
				appendStyle(start, at, CodeFunction)
				prevKeyword = ""
				prevWasIdent = false
			case nextByte == '(':
				// Function declaration, method declaration, or function/method call
				appendStyle(start, at, CodeFunction)
				prevKeyword = ""
				prevWasIdent = false
			case nextByte == '{' && unicode.IsUpper(firstRune):
				// Composite struct literal: e.g. Rect{...}, ui.Theme{...}, MenuItem{...}
				appendStyle(start, at, CodeType)
				prevKeyword = ""
				prevWasIdent = false
			case isPrecededByTypeContext(line, start, prevKeyword, prevWasIdent):
				// If followed by `.` (e.g. `ui.Rect` in `x ui.Rect`), `ui` is package qualifier, and `Rect` will be CodeType
				if nextByte != '.' {
					appendStyle(start, at, CodeType)
					prevKeyword = ""
					prevWasIdent = false
				} else {
					prevWasIdent = true
				}
			case isAllCapsConstant(word):
				appendStyle(start, at, CodeNumber)
				prevKeyword = ""
				prevWasIdent = false
			case isUpperCamelIdentifier(word) && nextByte != ':':
				// Exported PascalCase type/symbol identifier (e.g. CodeEditor, VirtualTree, FontSpec, Color)
				appendStyle(start, at, CodeType)
				prevKeyword = ""
				prevWasIdent = true
			case nextByte == ':' && (after+1 >= len(line) || line[after+1] != '='):
				// Struct field key in composite literal: `Field: value`
				appendStyle(start, at, CodeVariable)
				prevKeyword = ""
				prevWasIdent = false
			case start > 0 && line[start-1] == '.':
				// Selector field access: `obj.field`
				appendStyle(start, at, CodeVariable)
				prevKeyword = ""
				prevWasIdent = true
			default:
				if prevKeyword != "var" && prevKeyword != "const" && prevKeyword != "package" {
					prevWasIdent = true
				} else {
					prevWasIdent = false
				}
				prevKeyword = ""
			}
			continue
		}
		if unicode.IsDigit(r) {
			at += size
			for at < len(line) {
				r, size = utf8.DecodeRuneInString(line[at:])
				if !unicode.IsDigit(r) && !unicode.IsLetter(r) && r != '_' && r != '.' {
					break
				}
				at += size
			}
			appendStyle(start, at, CodeNumber)
			prevKeyword = ""
			prevWasIdent = false
			continue
		}
		if r != ' ' && r != '\t' && r != '*' && r != '[' && r != ']' && r != '.' {
			if r == ',' {
				// In `a, b MyType`, keep identifier context across commas on signature/field lines
			} else if r == ')' {
				// After closing parameter list `func foo(a int) MyReturnType`, next ident is a return type
				prevWasIdent = true
				prevKeyword = ""
			} else {
				prevWasIdent = false
				prevKeyword = ""
			}
		}
		at += size
	}
	return spans, state
}

func goBuiltinType(word string) bool {
	switch word {
	case "any", "bool", "byte", "complex64", "complex128", "error", "float32", "float64",
		"int", "int8", "int16", "int32", "int64", "rune", "string", "uint", "uint8",
		"uint16", "uint32", "uint64", "uintptr", "comparable":
		return true
	}
	return false
}

func goBuiltinFunction(word string) bool {
	switch word {
	case "append", "cap", "clear", "close", "complex", "copy", "delete", "imag",
		"len", "make", "max", "min", "new", "panic", "print", "println", "real", "recover":
		return true
	}
	return false
}
