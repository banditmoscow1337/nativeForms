package goide

import (
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/scanner"
	"go/token"
	"sort"
	"strings"
	"unicode"
	"unicode/utf16"
	"unicode/utf8"

	ui "github.com/banditmoscow1337/nativeForms"
)

// FormatGoSource formats Go source code in-memory using the standard library's go/format.
func FormatGoSource(source string) (string, error) {
	formatted, err := format.Source([]byte(source))
	if err != nil {
		return "", err
	}
	return string(formatted), nil
}

// ParseGoOutline parses Go source code with go/parser and go/ast, returning
// all top-level symbols (package, func, method, type, struct, interface, const, var)
// and any syntax diagnostics.
func byteOffsetToUTF16Col(lineText string, byteCol int) (int, int) {
	lineText = strings.TrimSuffix(lineText, "\r")
	if len(lineText) == 0 {
		return 0, 0
	}
	if byteCol < 0 {
		byteCol = 0
	}
	if byteCol > len(lineText) {
		byteCol = len(lineText)
	}
	u16 := 0
	for idx, r := range lineText {
		if idx >= byteCol {
			break
		}
		u16 += utf16.RuneLen(r)
	}
	endU16 := u16
	if byteCol < len(lineText) {
		cur := byteCol
		r, sz := utf8.DecodeRuneInString(lineText[cur:])
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' {
			endU16 += utf16.RuneLen(r)
			cur += sz
			for cur < len(lineText) {
				nr, nsz := utf8.DecodeRuneInString(lineText[cur:])
				if !unicode.IsLetter(nr) && !unicode.IsDigit(nr) && nr != '_' {
					break
				}
				endU16 += utf16.RuneLen(nr)
				cur += nsz
			}
		} else {
			endU16 += utf16.RuneLen(r)
		}
	}
	return u16, endU16
}

// ExtractGoASTStyleRanges parses Go source code with go/parser and extracts
// non-overlapping StyledRange slices for type expressions, declarations, and function calls.
func ExtractGoASTStyleRanges(source string) []ui.StyledRange {
	if len(source) == 0 {
		return nil
	}
	fset := token.NewFileSet()
	file, _ := parser.ParseFile(fset, "active.go", source, parser.SkipObjectResolution|parser.AllErrors)
	if file == nil {
		return nil
	}
	base := fset.File(file.Pos()).Base()
	var raw []ui.StyledRange
	addIdent := func(id *ast.Ident, style ui.CodeStyle) {
		if id == nil || id.Name == "" || id.Name == "_" {
			return
		}
		start := int(id.Pos()) - base
		end := start + len(id.Name)
		if start >= 0 && end > start && end <= len(source) {
			raw = append(raw, ui.StyledRange{Start: start, End: end, Style: style})
		}
	}
	var markTypeExpr func(expr ast.Expr)
	markTypeExpr = func(expr ast.Expr) {
		if expr == nil {
			return
		}
		switch t := expr.(type) {
		case *ast.Ident:
			addIdent(t, ui.CodeType)
		case *ast.SelectorExpr:
			addIdent(t.Sel, ui.CodeType)
		case *ast.StarExpr:
			markTypeExpr(t.X)
		case *ast.ArrayType:
			markTypeExpr(t.Elt)
		case *ast.MapType:
			markTypeExpr(t.Key)
			markTypeExpr(t.Value)
		case *ast.ChanType:
			markTypeExpr(t.Value)
		case *ast.Ellipsis:
			markTypeExpr(t.Elt)
		case *ast.ParenExpr:
			markTypeExpr(t.X)
		case *ast.IndexExpr:
			markTypeExpr(t.X)
			markTypeExpr(t.Index)
		case *ast.IndexListExpr:
			markTypeExpr(t.X)
			for _, ix := range t.Indices {
				markTypeExpr(ix)
			}
		case *ast.StructType:
			if t.Fields != nil {
				for _, f := range t.Fields.List {
					for _, n := range f.Names {
						addIdent(n, ui.CodeVariable)
					}
					markTypeExpr(f.Type)
				}
			}
		case *ast.InterfaceType:
			if t.Methods != nil {
				for _, m := range t.Methods.List {
					for _, n := range m.Names {
						addIdent(n, ui.CodeFunction)
					}
					markTypeExpr(m.Type)
				}
			}
		case *ast.FuncType:
			if t.TypeParams != nil {
				for _, f := range t.TypeParams.List {
					for _, n := range f.Names {
						addIdent(n, ui.CodeType)
					}
					markTypeExpr(f.Type)
				}
			}
			if t.Params != nil {
				for _, f := range t.Params.List {
					markTypeExpr(f.Type)
				}
			}
			if t.Results != nil {
				for _, f := range t.Results.List {
					markTypeExpr(f.Type)
				}
			}
		}
	}

	ast.Inspect(file, func(n ast.Node) bool {
		if n == nil {
			return false
		}
		switch node := n.(type) {
		case *ast.TypeSpec:
			addIdent(node.Name, ui.CodeType)
			if node.TypeParams != nil {
				for _, f := range node.TypeParams.List {
					for _, id := range f.Names {
						addIdent(id, ui.CodeType)
					}
					markTypeExpr(f.Type)
				}
			}
			markTypeExpr(node.Type)
		case *ast.FuncDecl:
			addIdent(node.Name, ui.CodeFunction)
			if node.Recv != nil {
				for _, f := range node.Recv.List {
					markTypeExpr(f.Type)
				}
			}
			if node.Type != nil {
				markTypeExpr(node.Type)
			}
		case *ast.ValueSpec:
			markTypeExpr(node.Type)
		case *ast.CompositeLit:
			markTypeExpr(node.Type)
		case *ast.TypeAssertExpr:
			markTypeExpr(node.Type)
		case *ast.CallExpr:
			switch fn := node.Fun.(type) {
			case *ast.Ident:
				if !token.Lookup(fn.Name).IsKeyword() {
					addIdent(fn, ui.CodeFunction)
				}
			case *ast.SelectorExpr:
				addIdent(fn.Sel, ui.CodeFunction)
			}
		}
		return true
	})

	if len(raw) == 0 {
		return nil
	}
	sort.Slice(raw, func(i, j int) bool {
		if raw[i].Start != raw[j].Start {
			return raw[i].Start < raw[j].Start
		}
		return raw[i].End < raw[j].End
	})
	dedup := make([]ui.StyledRange, 0, len(raw))
	for _, r := range raw {
		if len(dedup) > 0 && r.Start < dedup[len(dedup)-1].End {
			continue
		}
		dedup = append(dedup, r)
	}
	return dedup
}

func ParseGoOutline(filePath, source string) ([]outlineSymbol, []diagnosticRow) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, filePath, source, parser.ParseComments|parser.AllErrors)
	sourceLines := strings.Split(source, "\n")
	var diags []diagnosticRow
	if err != nil {
		var errList scanner.ErrorList
		if ok := asScannerErrorList(err, &errList); ok {
			for i, e := range errList {
				line := max(0, e.Pos.Line-1)
				if len(sourceLines) > 0 && line >= len(sourceLines) {
					line = len(sourceLines) - 1
				}
				byteCol := max(0, e.Pos.Column-1)
				col, endCol := byteCol, byteCol+1
				if line >= 0 && line < len(sourceLines) {
					col, endCol = byteOffsetToUTF16Col(sourceLines[line], byteCol)
				}
				diags = append(diags, diagnosticRow{
					id:        fmt.Sprintf("syntax:%s:%d:%d:%d", filePath, line, col, i),
					path:      filePath,
					line:      line,
					column:    col,
					endLine:   line,
					endColumn: endCol,
					message:   e.Msg,
					severity:  1,
					source:    "go/parser",
				})
			}
		} else {
			diags = append(diags, diagnosticRow{
				id:       fmt.Sprintf("syntax:%s:0:0:0", filePath),
				path:     filePath,
				line:     0,
				column:   0,
				message:  err.Error(),
				severity: 1,
				source:   "go/parser",
			})
		}
	}
	if file == nil {
		return nil, diags
	}

	var symbols []outlineSymbol
	if file.Name != nil {
		pos := fset.Position(file.Name.Pos())
		symbols = append(symbols, outlineSymbol{
			id:     fmt.Sprintf("%s:pkg:%s", filePath, file.Name.Name),
			name:   file.Name.Name,
			detail: "package",
			kind:   "package",
			path:   filePath,
			line:   max(0, pos.Line-1),
			column: max(0, pos.Column-1),
			offset: max(0, pos.Offset),
		})
	}

	for _, decl := range file.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			if d.Name == nil {
				continue
			}
			pos := fset.Position(d.Name.Pos())
			kind := "func"
			name := d.Name.Name
			detail := formatFuncSignature(d)
			if d.Recv != nil && len(d.Recv.List) > 0 {
				kind = "method"
				recvType := exprToString(d.Recv.List[0].Type)
				name = fmt.Sprintf("(%s).%s", recvType, d.Name.Name)
			}
			symbols = append(symbols, outlineSymbol{
				id:     fmt.Sprintf("%s:func:%s:%d", filePath, name, pos.Offset),
				name:   name,
				detail: detail,
				kind:   kind,
				path:   filePath,
				line:   max(0, pos.Line-1),
				column: max(0, pos.Column-1),
				offset: max(0, pos.Offset),
			})
		case *ast.GenDecl:
			for _, spec := range d.Specs {
				switch s := spec.(type) {
				case *ast.TypeSpec:
					if s.Name == nil {
						continue
					}
					pos := fset.Position(s.Name.Pos())
					kind := "type"
					detail := "type"
					switch s.Type.(type) {
					case *ast.StructType:
						kind = "struct"
						detail = "struct"
					case *ast.InterfaceType:
						kind = "interface"
						detail = "interface"
					}
					symbols = append(symbols, outlineSymbol{
						id:     fmt.Sprintf("%s:type:%s:%d", filePath, s.Name.Name, pos.Offset),
						name:   s.Name.Name,
						detail: detail,
						kind:   kind,
						path:   filePath,
						line:   max(0, pos.Line-1),
						column: max(0, pos.Column-1),
						offset: max(0, pos.Offset),
					})
				case *ast.ValueSpec:
					kind := "var"
					if d.Tok == token.CONST {
						kind = "const"
					}
					valType := exprToString(s.Type)
					for _, ident := range s.Names {
						if ident == nil || ident.Name == "_" {
							continue
						}
						pos := fset.Position(ident.Pos())
						symbols = append(symbols, outlineSymbol{
							id:     fmt.Sprintf("%s:%s:%s:%d", filePath, kind, ident.Name, pos.Offset),
							name:   ident.Name,
							detail: nonEmptyStr(valType, kind),
							kind:   kind,
							path:   filePath,
							line:   max(0, pos.Line-1),
							column: max(0, pos.Column-1),
							offset: max(0, pos.Offset),
						})
					}
				}
			}
		}
	}
	return symbols, diags
}

func asScannerErrorList(err error, target *scanner.ErrorList) bool {
	if list, ok := err.(scanner.ErrorList); ok {
		*target = list
		return true
	}
	return false
}

func formatFuncSignature(fn *ast.FuncDecl) string {
	if fn.Type == nil {
		return "func()"
	}
	var b strings.Builder
	b.WriteByte('(')
	if fn.Type.Params != nil {
		for i, field := range fn.Type.Params.List {
			if i > 0 {
				b.WriteString(", ")
			}
			tStr := exprToString(field.Type)
			if len(field.Names) > 0 {
				names := make([]string, 0, len(field.Names))
				for _, n := range field.Names {
					names = append(names, n.Name)
				}
				b.WriteString(strings.Join(names, ", "))
				if tStr != "" {
					b.WriteByte(' ')
					b.WriteString(tStr)
				}
			} else {
				b.WriteString(tStr)
			}
		}
	}
	b.WriteByte(')')
	if fn.Type.Results != nil && len(fn.Type.Results.List) > 0 {
		b.WriteByte(' ')
		if len(fn.Type.Results.List) > 1 {
			b.WriteByte('(')
		}
		for i, field := range fn.Type.Results.List {
			if i > 0 {
				b.WriteString(", ")
			}
			b.WriteString(exprToString(field.Type))
		}
		if len(fn.Type.Results.List) > 1 {
			b.WriteByte(')')
		}
	}
	return b.String()
}

func exprToString(expr ast.Expr) string {
	if expr == nil {
		return ""
	}
	switch e := expr.(type) {
	case *ast.Ident:
		return e.Name
	case *ast.StarExpr:
		return "*" + exprToString(e.X)
	case *ast.SelectorExpr:
		return exprToString(e.X) + "." + e.Sel.Name
	case *ast.ArrayType:
		return "[]" + exprToString(e.Elt)
	case *ast.MapType:
		return "map[" + exprToString(e.Key) + "]" + exprToString(e.Value)
	case *ast.Ellipsis:
		return "..." + exprToString(e.Elt)
	case *ast.InterfaceType:
		return "interface{}"
	case *ast.StructType:
		return "struct{}"
	case *ast.FuncType:
		return "func(...)"
	case *ast.ChanType:
		return "chan " + exprToString(e.Value)
	default:
		return ""
	}
}

func nonEmptyStr(s, fallback string) string {
	if s == "" {
		return fallback
	}
	return s
}
