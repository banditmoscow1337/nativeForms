package goide

import (
	"fmt"
	"strconv"
	"strings"

	ui "github.com/banditmoscow1337/nativeForms"
)

// outlineSymbol represents a Go AST declaration (package, type, struct, interface, func, method, const, var).
type outlineSymbol struct {
	id     string
	name   string
	detail string
	kind   string // "package", "func", "method", "type", "struct", "interface", "const", "var"
	path   string
	line   int // 0-based line
	column int // 0-based UTF-16 column
	offset int // 0-based byte offset
}

type outlineModel struct {
	icons  *Icons
	filter string
	all    []outlineSymbol
	rows   []outlineSymbol
}

func (m *outlineModel) rebuild() {
	m.rows = m.rows[:0]
	q := strings.ToLower(strings.TrimSpace(m.filter))
	for _, s := range m.all {
		if q == "" || strings.Contains(strings.ToLower(s.name), q) || strings.Contains(strings.ToLower(s.detail), q) {
			m.rows = append(m.rows, s)
		}
	}
}

func (m *outlineModel) Len() int        { return len(m.rows) }
func (m *outlineModel) ID(i int) string { return m.rows[i].id }
func (m *outlineModel) Label(i int) string {
	s := m.rows[i]
	if s.detail != "" {
		return fmt.Sprintf("%s  %s  :%d", s.name, s.detail, s.line+1)
	}
	return fmt.Sprintf("%s  (%s :%d)", s.name, s.kind, s.line+1)
}
func (m *outlineModel) Icon(i int) *ui.ImageResource {
	if m.icons == nil {
		return nil
	}
	switch m.rows[i].kind {
	case "package":
		return m.icons.SymPkg
	case "func", "method":
		return m.icons.SymFunc
	case "type", "struct", "interface":
		return m.icons.SymType
	case "const":
		return m.icons.SymConst
	default:
		return m.icons.SymVar
	}
}

// diagnosticRow represents an LSP, compiler, vet, or syntax diagnostic.
type diagnosticRow struct {
	id, path, message, source string
	line, column, severity    int
	endLine, endColumn        int
}

type diagnosticsModel struct {
	icons *Icons
	rows  []diagnosticRow
}

func (m *diagnosticsModel) Len() int        { return len(m.rows) }
func (m *diagnosticsModel) ID(i int) string { return m.rows[i].id }
func (m *diagnosticsModel) Columns() int    { return 4 }
func (m *diagnosticsModel) Header(i int) string {
	return [...]string{"File", "Line:Col", "Diagnostic", "Source"}[i]
}
func (m *diagnosticsModel) Cell(row, col int) string {
	r := m.rows[row]
	switch col {
	case 0:
		return fileName(r.path)
	case 1:
		if r.column > 0 {
			return fmt.Sprintf("%d:%d", r.line+1, r.column+1)
		}
		return strconv.Itoa(r.line + 1)
	case 2:
		return r.message
	default:
		if r.source == "" {
			return "gopls"
		}
		return r.source
	}
}
func (m *diagnosticsModel) CellIcon(row, col int) *ui.ImageResource {
	if col != 0 || m.icons == nil {
		return nil
	}
	switch m.rows[row].severity {
	case 1:
		return m.icons.Error
	case 2:
		return m.icons.Warning
	default:
		return m.icons.Info
	}
}

// searchMatch represents a match in project-wide search.
type searchMatch struct {
	id      string
	path    string
	line    int
	column  int
	preview string
}

type searchModel struct {
	icons *Icons
	rows  []searchMatch
}

func (m *searchModel) Len() int        { return len(m.rows) }
func (m *searchModel) ID(i int) string { return m.rows[i].id }
func (m *searchModel) Columns() int    { return 3 }
func (m *searchModel) Header(i int) string {
	return [...]string{"File", "Line", "Code Match"}[i]
}
func (m *searchModel) Cell(row, col int) string {
	r := m.rows[row]
	switch col {
	case 0:
		return fileName(r.path)
	case 1:
		return fmt.Sprintf("%d:%d", r.line+1, r.column+1)
	default:
		return r.preview
	}
}
func (m *searchModel) CellIcon(row, col int) *ui.ImageResource {
	if col == 0 && m.icons != nil {
		return iconForPath(m.icons, m.rows[row].path, false)
	}
	return nil
}

// testResultRow represents a single test result from `go test -json`.
type testResultRow struct {
	id       string
	pkg      string
	name     string
	status   string // "PASS", "FAIL", "SKIP", "RUN"
	elapsed  float64
	filePath string
	line     int
}

type testResultsModel struct {
	icons *Icons
	rows  []testResultRow
}

func (m *testResultsModel) Len() int        { return len(m.rows) }
func (m *testResultsModel) ID(i int) string { return m.rows[i].id }
func (m *testResultsModel) Columns() int    { return 4 }
func (m *testResultsModel) Header(i int) string {
	return [...]string{"Status", "Test / Package", "Duration", "Location"}[i]
}
func (m *testResultsModel) Cell(row, col int) string {
	r := m.rows[row]
	switch col {
	case 0:
		return r.status
	case 1:
		if r.name != "" {
			return r.name + " (" + fileName(r.pkg) + ")"
		}
		return r.pkg
	case 2:
		if r.elapsed > 0 {
			return fmt.Sprintf("%.3fs", r.elapsed)
		}
		return "-"
	default:
		if r.filePath != "" {
			return fmt.Sprintf("%s:%d", fileName(r.filePath), r.line+1)
		}
		return r.pkg
	}
}
func (m *testResultsModel) CellIcon(row, col int) *ui.ImageResource {
	if col != 0 || m.icons == nil {
		return nil
	}
	switch m.rows[row].status {
	case "PASS":
		return m.icons.Pass
	case "FAIL":
		return m.icons.Error
	case "SKIP":
		return m.icons.Warning
	default:
		return m.icons.Info
	}
}

// breakpointRow represents a configured breakpoint in the project.
type breakpointRow struct {
	id   string
	path string
	line int
}

type breakpointsModel struct {
	icons *Icons
	rows  []breakpointRow
}

func (m *breakpointsModel) Len() int        { return len(m.rows) }
func (m *breakpointsModel) ID(i int) string { return m.rows[i].id }
func (m *breakpointsModel) Label(i int) string {
	r := m.rows[i]
	return fmt.Sprintf("%s : строка %d", fileName(r.path), r.line+1)
}
func (m *breakpointsModel) Icon(i int) *ui.ImageResource {
	if m.icons != nil {
		return m.icons.Breakpoint
	}
	return nil
}

type stackRow struct {
	id, line   int
	name, path string
}

type stackModel struct {
	icons *Icons
	rows  []stackRow
}

func (m *stackModel) Len() int        { return len(m.rows) }
func (m *stackModel) ID(i int) string { return strconv.Itoa(m.rows[i].id) }
func (m *stackModel) Columns() int    { return 3 }
func (m *stackModel) Header(i int) string {
	return [...]string{"Function", "File", "Line"}[i]
}
func (m *stackModel) Cell(row, col int) string {
	r := m.rows[row]
	switch col {
	case 0:
		return r.name
	case 1:
		return fileName(r.path)
	default:
		return strconv.Itoa(r.line)
	}
}
func (m *stackModel) CellIcon(row, col int) *ui.ImageResource {
	if col == 0 && m.icons != nil {
		return m.icons.SymFunc
	}
	return nil
}

type variableRow struct {
	id, name, value, kind string
}

type variablesModel struct {
	icons *Icons
	rows  []variableRow
}

func (m *variablesModel) Len() int        { return len(m.rows) }
func (m *variablesModel) ID(i int) string { return m.rows[i].id }
func (m *variablesModel) Columns() int    { return 3 }
func (m *variablesModel) Header(i int) string {
	return [...]string{"Variable", "Value", "Type"}[i]
}
func (m *variablesModel) Cell(row, col int) string {
	r := m.rows[row]
	switch col {
	case 0:
		return r.name
	case 1:
		return r.value
	default:
		return r.kind
	}
}
func (m *variablesModel) CellIcon(row, col int) *ui.ImageResource {
	if col == 0 && m.icons != nil {
		return m.icons.SymVar
	}
	return nil
}
