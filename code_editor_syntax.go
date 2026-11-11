package nativeforms

import (
	"sort"

	"github.com/banditmoscow1337/nativeForms/editor"
)

func (e *CodeEditor) SetStyleRanges(version uint64, ranges []StyledRange) error {
	e.syncDocument()
	validated, err := e.validateRanges(version, ranges)
	if err != nil {
		return err
	}
	e.styles = validated
	e.InvalidatePaint()
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
	e.InvalidatePaint()
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
	if theme.IsLight() {
		switch style {
		case CodeKeyword:
			return RGBA(175, 0, 219, 255)
		case CodeComment:
			return RGBA(0, 128, 0, 255)
		case CodeString:
			return RGBA(163, 21, 21, 255)
		case CodeNumber:
			return RGBA(9, 134, 88, 255)
		case CodeType:
			return RGBA(38, 127, 153, 255)
		case CodeFunction:
			return RGBA(121, 94, 38, 255)
		case CodeVariable:
			return RGBA(0, 16, 128, 255)
		default:
			return theme.Text
		}
	}
	switch style {
	case CodeKeyword:
		return RGBA(197, 134, 192, 255)
	case CodeComment:
		return RGBA(106, 153, 85, 255)
	case CodeString:
		return RGBA(206, 145, 120, 255)
	case CodeNumber:
		return RGBA(181, 206, 168, 255)
	case CodeType:
		return RGBA(78, 201, 176, 255)
	case CodeFunction:
		return RGBA(220, 220, 170, 255)
	case CodeVariable:
		return RGBA(156, 220, 254, 255)
	default:
		return theme.Text
	}
}
