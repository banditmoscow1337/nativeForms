package nativeforms

import (
	"errors"
	"go/token"
	"sort"
	"unicode"
	"unicode/utf8"

	"github.com/banditmoscow1337/nativeForms/editor"
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

func scanGoLineInto(line string, state goLexState, styles bool, spans []StyledRange) ([]StyledRange, goLexState) {
	spans = spans[:0]
	if !styles {
		return spans, advanceGoLexState(line, state)
	}
	appendStyle := func(start, end int, style CodeStyle) {
		if start < end {
			spans = append(spans, StyledRange{Start: start, End: end, Style: style})
		}
	}
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
			continue
		}
		r, size := utf8.DecodeRuneInString(line[at:])
		if unicode.IsLetter(r) || r == '_' {
			at += size
			for at < len(line) {
				r, size = utf8.DecodeRuneInString(line[at:])
				if !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '_' {
					break
				}
				at += size
			}
			word := line[start:at]
			switch {
			case token.Lookup(word).IsKeyword():
				appendStyle(start, at, CodeKeyword)
			case word == "true" || word == "false" || word == "nil" || word == "iota":
				appendStyle(start, at, CodeNumber)
			case goBuiltinType(word):
				appendStyle(start, at, CodeType)
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
			continue
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

func (e *CodeEditor) SetStyleRanges(version uint64, ranges []StyledRange) error {
	e.syncDocument()
	validated, err := e.validateRanges(version, ranges)
	if err != nil {
		return err
	}
	e.styles = validated
	e.invalidatePaint()
	return nil
}

// SetSemanticTokens accepts LSP's five-integer delta encoding. Positions and
// lengths are UTF-16 code units; legend indexes name gopls token types.
// An obsolete or invalid response leaves the current styles untouched.
func (e *CodeEditor) SetSemanticTokens(version uint64, legend []string, data []uint32) error {
	e.syncDocument()
	if version != e.Document.Version() {
		return ErrStaleCodeStyle
	}
	if len(data)%5 != 0 {
		return ErrInvalidCodeStyle
	}
	ranges := make([]StyledRange, 0, len(data)/5)
	line, column := 0, 0
	for at := 0; at < len(data); at += 5 {
		line += int(data[at])
		if data[at] != 0 {
			column = 0
		}
		column += int(data[at+1])
		length := int(data[at+2])
		kind := int(data[at+3])
		if line < 0 || line >= e.Document.LineCount() || length <= 0 || kind >= len(legend) ||
			column < 0 || column > int(^uint(0)>>1)-length {
			return ErrInvalidCodeStyle
		}
		start, err := e.Document.ByteOffset(editor.Position{Line: line, Character: column})
		if err != nil {
			return ErrInvalidCodeStyle
		}
		end, err := e.Document.ByteOffset(editor.Position{Line: line, Character: column + length})
		if err != nil {
			return ErrInvalidCodeStyle
		}
		ranges = append(ranges, StyledRange{Start: start, End: end, Style: semanticCodeStyle(legend[kind])})
	}
	validated, err := e.validateRanges(version, ranges)
	if err != nil {
		return err
	}
	e.semantic = validated
	e.invalidatePaint()
	return nil
}

func semanticCodeStyle(name string) CodeStyle {
	switch name {
	case "keyword", "modifier":
		return CodeKeyword
	case "comment":
		return CodeComment
	case "string":
		return CodeString
	case "number":
		return CodeNumber
	case "type", "class", "struct", "interface", "enum", "typeParameter", "namespace":
		return CodeType
	case "function", "method":
		return CodeFunction
	default:
		return CodeVariable
	}
}

func (e *CodeEditor) validateRanges(version uint64, ranges []StyledRange) ([]StyledRange, error) {
	if version != e.Document.Version() {
		return nil, ErrStaleCodeStyle
	}
	copyRanges := append([]StyledRange(nil), ranges...)
	sort.Slice(copyRanges, func(i, j int) bool { return copyRanges[i].Start < copyRanges[j].Start })
	for i, r := range copyRanges {
		if r.Start < 0 || r.End <= r.Start || r.End > e.Document.Len() ||
			r.Style > CodeVariable || i > 0 && copyRanges[i-1].End > r.Start {
			return nil, ErrInvalidCodeStyle
		}
		if _, err := e.Document.Slice(r.Start, r.End); err != nil {
			return nil, ErrInvalidCodeStyle
		}
	}
	return copyRanges, nil
}

func codeStyleColor(style CodeStyle, theme Theme) Color {
	switch style {
	case CodeKeyword:
		return theme.Accent
	case CodeComment:
		return theme.MutedText
	case CodeString:
		return RGBA(153, 207, 157, 255)
	case CodeNumber:
		return RGBA(202, 170, 224, 255)
	case CodeType:
		return RGBA(131, 196, 217, 255)
	case CodeFunction:
		return RGBA(235, 205, 138, 255)
	default:
		return theme.Text
	}
}
