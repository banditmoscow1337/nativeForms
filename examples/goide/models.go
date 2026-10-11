package goide

import (
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

func fileName(path string) string { return filepath.Base(path) }

func fileURI(path string) string {
	path = filepath.ToSlash(path)
	if runtime.GOOS == "windows" { path = "/" + path }
	return (&url.URL{Scheme:"file",Path:path}).String()
}

func uriPath(uri string) string {
	u,err := url.Parse(uri)
	if err != nil || u.Scheme != "file" || u.Host != "" { return "" }
	path := u.Path
	if runtime.GOOS == "windows" && len(path)>2 && path[0]=='/' && path[2]==':' { path=path[1:] }
	return filepath.Clean(filepath.FromSlash(path))
}

func within(root,path string) bool {
	rel,err := filepath.Rel(root,path)
	return err==nil && rel!=".." && !strings.HasPrefix(rel,".."+string(filepath.Separator))
}

type projectTree struct {
	root string
	entries map[string][]string
}

func newProjectTree(root string) *projectTree {
	return &projectTree{root:root,entries:make(map[string][]string)}
}
func (m *projectTree) name() string { return fileName(m.root) }
func (m *projectTree) Roots() int { if m.root=="" { return 0 }; return 1 }
func (m *projectTree) Root(int) string { return m.root }
func (m *projectTree) Label(id string) string { return fileName(id) }
func (m *projectTree) Children(id string) int { return len(m.list(id)) }
func (m *projectTree) Child(id string,index int) string { return m.list(id)[index] }
func (m *projectTree) list(id string) []string {
	if cached,ok := m.entries[id]; ok { return cached }
	entries,err := os.ReadDir(id)
	if err != nil { m.entries[id]=nil; return nil }
	var dirs,files []string
	for _,entry := range entries {
		if strings.HasPrefix(entry.Name(),".") { continue }
		path := filepath.Join(id,entry.Name())
		if entry.IsDir() {
			if entry.Name()!="vendor" { dirs=append(dirs,path) }
		} else if strings.HasSuffix(entry.Name(),".go") || entry.Name()=="go.mod" || entry.Name()=="go.sum" {
			files=append(files,path)
		}
	}
	m.entries[id]=append(dirs,files...)
	return m.entries[id]
}

type diagnosticRow struct {
	id,path,message string
	line,column,severity int
}
type diagnosticsModel struct { rows []diagnosticRow }
func (m *diagnosticsModel) Len() int { return len(m.rows) }
func (m *diagnosticsModel) ID(i int) string { return m.rows[i].id }
func (m *diagnosticsModel) Columns() int { return 3 }
func (m *diagnosticsModel) Header(i int) string { return []string{"File","Line","Diagnostic"}[i] }
func (m *diagnosticsModel) Cell(row,col int) string {
	r:=m.rows[row]
	switch col {
	case 0: return fileName(r.path)
	case 1: return strconv.Itoa(r.line+1)
	default: return r.message
	}
}

type stackRow struct { id,line int; name,path string }
type stackModel struct { rows []stackRow }
func (m *stackModel) Len() int { return len(m.rows) }
func (m *stackModel) ID(i int) string { return strconv.Itoa(m.rows[i].id) }
func (m *stackModel) Columns() int { return 3 }
func (m *stackModel) Header(i int) string { return []string{"Function","File","Line"}[i] }
func (m *stackModel) Cell(row,col int) string {
	r:=m.rows[row]
	switch col {
	case 0: return r.name
	case 1: return fileName(r.path)
	default: return strconv.Itoa(r.line)
	}
}

type variableRow struct { id,name,value,kind string }
type variablesModel struct { rows []variableRow }
func (m *variablesModel) Len() int { return len(m.rows) }
func (m *variablesModel) ID(i int) string { return m.rows[i].id }
func (m *variablesModel) Columns() int { return 3 }
func (m *variablesModel) Header(i int) string { return []string{"Variable","Value","Type"}[i] }
func (m *variablesModel) Cell(row,col int) string {
	r:=m.rows[row]
	switch col {
	case 0: return r.name
	case 1: return r.value
	default: return r.kind
	}
}
