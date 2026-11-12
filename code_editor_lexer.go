package nativeforms

import (
	"errors"
	"go/token"
	"path/filepath"
	"strings"
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

// SyntaxLexer tokenizes source lines for CodeEditor syntax highlighting.
// State is a per-line carry state (goNormal == 0 on line 0).
type SyntaxLexer interface {
	AdvanceState(line string, state goLexState) goLexState
	LexLine(line string, state goLexState, dst []StyledRange) (spans []StyledRange, nextState goLexState)
}

type goSyntaxLexer struct{}

func (goSyntaxLexer) AdvanceState(line string, state goLexState) goLexState {
	return advanceGoLexState(line, state)
}

func (goSyntaxLexer) LexLine(line string, state goLexState, dst []StyledRange) ([]StyledRange, goLexState) {
	return scanGoLineInto(line, state, true, dst)
}

type plainTextSyntaxLexer struct{}

func (plainTextSyntaxLexer) AdvanceState(_ string, _ goLexState) goLexState { return goNormal }
func (plainTextSyntaxLexer) LexLine(_ string, _ goLexState, dst []StyledRange) ([]StyledRange, goLexState) {
	return dst[:0], goNormal
}

type goModSyntaxLexer struct{}

func (goModSyntaxLexer) AdvanceState(_ string, _ goLexState) goLexState { return goNormal }
func (goModSyntaxLexer) LexLine(line string, _ goLexState, dst []StyledRange) ([]StyledRange, goLexState) {
	spans := dst[:0]
	appendStyle := func(start, end int, style CodeStyle) {
		if start < end && style != CodePlain {
			spans = append(spans, StyledRange{Start: start, End: end, Style: style})
		}
	}
	for at := 0; at < len(line); {
		if line[at] == ' ' || line[at] == '\t' || line[at] == '\r' || line[at] == '(' || line[at] == ')' {
			at++
			continue
		}
		if at+1 < len(line) && line[at] == '/' && line[at+1] == '/' {
			appendStyle(at, len(line), CodeComment)
			break
		}
		if line[at] == '"' || line[at] == '`' {
			q := line[at]
			start := at
			at++
			for at < len(line) && line[at] != q {
				if line[at] == '\\' && q == '"' {
					at = min(len(line), at+2)
					continue
				}
				at++
			}
			if at < len(line) {
				at++
			}
			appendStyle(start, at, CodeString)
			continue
		}
		start := at
		for at < len(line) && line[at] != ' ' && line[at] != '\t' && line[at] != '\r' && line[at] != '(' && line[at] != ')' {
			if at+1 < len(line) && line[at] == '/' && line[at+1] == '/' {
				break
			}
			at++
		}
		word := line[start:at]
		switch word {
		case "module", "go", "toolchain", "require", "replace", "exclude", "retract", "use", "=>":
			appendStyle(start, at, CodeKeyword)
		default:
			if len(word) > 0 && (word[0] == 'v' || (word[0] >= '0' && word[0] <= '9')) && strings.ContainsAny(word, "0123456789.") {
				appendStyle(start, at, CodeNumber)
			} else if strings.Contains(word, "/") || strings.Contains(word, ".") {
				appendStyle(start, at, CodeType)
			}
		}
	}
	return spans, goNormal
}

type configSyntaxLexer struct{}

func (configSyntaxLexer) AdvanceState(_ string, _ goLexState) goLexState { return goNormal }
func (configSyntaxLexer) LexLine(line string, _ goLexState, dst []StyledRange) ([]StyledRange, goLexState) {
	spans := dst[:0]
	appendStyle := func(start, end int, style CodeStyle) {
		if start < end && style != CodePlain {
			spans = append(spans, StyledRange{Start: start, End: end, Style: style})
		}
	}
	trimmedStart := skipSpaces(line, 0)
	if trimmedStart < len(line) && line[trimmedStart] == '[' {
		closeIdx := strings.IndexByte(line[trimmedStart:], ']')
		if closeIdx > 0 {
			appendStyle(trimmedStart, trimmedStart+closeIdx+1, CodeType)
			return spans, goNormal
		}
	}
	for at := 0; at < len(line); {
		b := line[at]
		if b == ' ' || b == '\t' || b == '\r' {
			at++
			continue
		}
		if b == '#' || (at+1 < len(line) && ((b == '/' && line[at+1] == '/') || (b == '-' && line[at+1] == '-'))) {
			appendStyle(at, len(line), CodeComment)
			break
		}
		if b == '"' || b == '\'' {
			start := at
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
			after := skipSpaces(line, at)
			if after < len(line) && line[after] == ':' {
				appendStyle(start, at, CodeVariable)
			} else {
				appendStyle(start, at, CodeString)
			}
			continue
		}
		r, sz := utf8.DecodeRuneInString(line[at:])
		if unicode.IsLetter(r) || r == '_' || r == '$' || r == '-' {
			start := at
			at += sz
			for at < len(line) {
				nr, nsz := utf8.DecodeRuneInString(line[at:])
				if !unicode.IsLetter(nr) && !unicode.IsDigit(nr) && nr != '_' && nr != '-' && nr != '.' {
					break
				}
				at += nsz
			}
			word := line[start:at]
			after := skipSpaces(line, at)
			if after < len(line) && (line[after] == ':' || line[after] == '=') {
				appendStyle(start, at, CodeVariable)
			} else if word == "true" || word == "false" || word == "null" || word == "nil" || word == "yes" || word == "no" {
				appendStyle(start, at, CodeNumber)
			} else if isAllCapsConstant(word) {
				appendStyle(start, at, CodeKeyword)
			}
			continue
		}
		if unicode.IsDigit(r) {
			start := at
			at += sz
			for at < len(line) {
				nr, nsz := utf8.DecodeRuneInString(line[at:])
				if !unicode.IsDigit(nr) && !unicode.IsLetter(nr) && nr != '.' && nr != '_' && nr != '-' {
					break
				}
				at += nsz
			}
			appendStyle(start, at, CodeNumber)
			continue
		}
		at += sz
	}
	return spans, goNormal
}

type markdownSyntaxLexer struct{}

func (markdownSyntaxLexer) AdvanceState(line string, state goLexState) goLexState {
	trimmed := strings.TrimSpace(line)
	if strings.HasPrefix(trimmed, "```") {
		if state == goRawString {
			return goNormal
		}
		return goRawString
	}
	return state
}

func (markdownSyntaxLexer) LexLine(line string, state goLexState, dst []StyledRange) ([]StyledRange, goLexState) {
	spans := dst[:0]
	trimmed := strings.TrimSpace(line)
	if strings.HasPrefix(trimmed, "```") {
		if len(line) > 0 {
			spans = append(spans, StyledRange{Start: 0, End: len(line), Style: CodeComment})
		}
		if state == goRawString {
			return spans, goNormal
		}
		return spans, goRawString
	}
	if state == goRawString {
		if len(line) > 0 {
			spans = append(spans, StyledRange{Start: 0, End: len(line), Style: CodeString})
		}
		return spans, goRawString
	}
	first := skipSpaces(line, 0)
	if first < len(line) && line[first] == '#' {
		spans = append(spans, StyledRange{Start: first, End: len(line), Style: CodeKeyword})
		return spans, goNormal
	}
	if first < len(line) && line[first] == '>' {
		spans = append(spans, StyledRange{Start: first, End: len(line), Style: CodeComment})
		return spans, goNormal
	}
	for at := 0; at < len(line); {
		if line[at] == '`' {
			start := at
			at++
			for at < len(line) && line[at] != '`' {
				at++
			}
			if at < len(line) {
				at++
			}
			spans = append(spans, StyledRange{Start: start, End: at, Style: CodeString})
			continue
		}
		if line[at] == '[' {
			closeBracket := strings.IndexByte(line[at:], ']')
			if closeBracket > 0 {
				spans = append(spans, StyledRange{Start: at, End: at + closeBracket + 1, Style: CodeType})
				at += closeBracket + 1
				continue
			}
		}
		at++
	}
	return spans, goNormal
}

var (
	defaultGoSyntaxLexer    SyntaxLexer = goSyntaxLexer{}
	defaultGoModSyntaxLexer SyntaxLexer = goModSyntaxLexer{}
	defaultConfigLexer      SyntaxLexer = configSyntaxLexer{}
	defaultMarkdownLexer    SyntaxLexer = markdownSyntaxLexer{}
	defaultPlainTextLexer   SyntaxLexer = plainTextSyntaxLexer{}
)

// GoSyntaxLexer returns the built-in Go syntax lexer.
func GoSyntaxLexer() SyntaxLexer { return defaultGoSyntaxLexer }

// GoModSyntaxLexer returns a syntax lexer for go.mod, go.sum, and go.work files.
func GoModSyntaxLexer() SyntaxLexer { return defaultGoModSyntaxLexer }

// ConfigSyntaxLexer returns a syntax lexer for JSON, YAML, TOML, shell, SQL, and config files.
func ConfigSyntaxLexer() SyntaxLexer { return defaultConfigLexer }

// MarkdownSyntaxLexer returns a syntax lexer for Markdown (.md) documents.
func MarkdownSyntaxLexer() SyntaxLexer { return defaultMarkdownLexer }

// PlainTextSyntaxLexer returns a no-op lexer that renders plain text without keyword coloring.
func PlainTextSyntaxLexer() SyntaxLexer { return defaultPlainTextLexer }

// LexerForFilename selects an appropriate built-in SyntaxLexer for filename.
func LexerForFilename(filename string) SyntaxLexer {
	base := filepath.Base(filename)
	if strings.HasSuffix(base, ".go") {
		return defaultGoSyntaxLexer
	}
	switch base {
	case "go.mod", "go.sum", "go.work":
		return defaultGoModSyntaxLexer
	case "Makefile", "Dockerfile", ".gitignore":
		return defaultConfigLexer
	}
	switch strings.ToLower(filepath.Ext(base)) {
	case ".md", ".markdown":
		return defaultMarkdownLexer
	case ".json", ".yaml", ".yml", ".toml", ".sh", ".sql", ".proto", ".ini", ".env":
		return defaultConfigLexer
	case ".txt", ".log":
		return defaultPlainTextLexer
	default:
		return defaultGoSyntaxLexer
	}
}

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
			switch r {
			case ',':
				// In `a, b MyType`, keep identifier context across commas on signature/field lines
			case ')':
				// After closing parameter list `func foo(a int) MyReturnType`, next ident is a return type
				prevWasIdent = true
				prevKeyword = ""
			default:
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
