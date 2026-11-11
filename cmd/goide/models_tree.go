package goide

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	ui "github.com/banditmoscow1337/nativeForms"
)

func fileName(path string) string { return filepath.Base(path) }

func relPath(root, path string) string {
	if root == "" {
		return fileName(path)
	}
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return fileName(path)
	}
	return filepath.ToSlash(rel)
}

func fileURI(path string) string {
	path = filepath.ToSlash(path)
	if runtime.GOOS == "windows" && !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return (&url.URL{Scheme: "file", Path: path}).String()
}

func uriPath(uri string) string {
	u, err := url.Parse(uri)
	if err != nil || u.Scheme != "file" || u.Host != "" {
		return ""
	}
	path := u.Path
	if runtime.GOOS == "windows" && len(path) > 2 && path[0] == '/' && path[2] == ':' {
		path = path[1:]
	}
	return filepath.Clean(filepath.FromSlash(path))
}

func within(root, path string) bool {
	if root == "" || path == "" {
		return false
	}
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func isProjectSourceFile(name string) bool {
	if strings.HasSuffix(name, ".go") {
		return true
	}
	switch name {
	case "go.mod", "go.sum", "go.work", "Makefile", "README.md", "Dockerfile", ".gitignore":
		return true
	}
	ext := strings.ToLower(filepath.Ext(name))
	switch ext {
	case ".md", ".json", ".yaml", ".yml", ".toml", ".txt", ".sh", ".sql", ".proto":
		return true
	}
	return false
}

func iconForPath(icons *Icons, path string, isDir bool) *ui.ImageResource {
	if icons == nil {
		return nil
	}
	if isDir {
		return icons.Folder
	}
	name := fileName(path)
	switch {
	case strings.HasSuffix(name, "_test.go"):
		return icons.GoTestFile
	case strings.HasSuffix(name, ".go"):
		return icons.GoFile
	case name == "go.mod" || name == "go.sum" || name == "go.work":
		return icons.GoMod
	default:
		return icons.File
	}
}

func warningThemeColor(theme ui.Theme) ui.Color {
	if theme.IsLight() {
		return ui.RGBA(195, 118, 0, 255)
	}
	return ui.RGBA(238, 178, 42, 255)
}

// projectTree implements ui.TreeModel, ui.TreeIconModel, and ui.TreeDecorationModel for the VS Code Explorer.
type projectTree struct {
	root       string
	filter     string
	icons      *Icons
	entries    map[string][]string
	dirs       map[string]bool
	fileErrors map[string]int
	fileWarns  map[string]int
}

func newProjectTree(root string) *projectTree {
	return &projectTree{
		root:       root,
		icons:      DefaultIcons(),
		entries:    make(map[string][]string),
		dirs:       make(map[string]bool),
		fileErrors: make(map[string]int),
		fileWarns:  make(map[string]int),
	}
}

func (m *projectTree) Refresh() {
	clear(m.entries)
	clear(m.dirs)
	if m.root != "" {
		m.dirs[m.root] = true
	}
}

func (m *projectTree) SetDiagnosticsCounts(errs, warns map[string]int) {
	if m.fileErrors == nil {
		m.fileErrors = make(map[string]int)
	} else {
		clear(m.fileErrors)
	}
	if m.fileWarns == nil {
		m.fileWarns = make(map[string]int)
	} else {
		clear(m.fileWarns)
	}
	for k, v := range errs {
		if v > 0 {
			m.fileErrors[filepath.Clean(k)] = v
		}
	}
	for k, v := range warns {
		if v > 0 {
			m.fileWarns[filepath.Clean(k)] = v
		}
	}
}

func (m *projectTree) countForNode(id string) (errs, warns int, isDir bool) {
	cleanID := filepath.Clean(id)
	if cleanID == filepath.Clean(m.root) || m.dirs[cleanID] {
		isDir = true
	} else if e, ok := m.fileErrors[cleanID]; ok {
		return e, m.fileWarns[cleanID], false
	} else if w, ok := m.fileWarns[cleanID]; ok {
		return 0, w, false
	}
	if !isDir {
		return m.fileErrors[cleanID], m.fileWarns[cleanID], false
	}
	for p, e := range m.fileErrors {
		if p == cleanID || within(cleanID, p) {
			errs += e
		}
	}
	for p, w := range m.fileWarns {
		if p == cleanID || within(cleanID, p) {
			warns += w
		}
	}
	return errs, warns, true
}

func (m *projectTree) NodeColor(id string, theme ui.Theme) (ui.Color, bool) {
	errs, warns, _ := m.countForNode(id)
	if errs > 0 {
		return theme.Danger, true
	}
	if warns > 0 {
		return warningThemeColor(theme), true
	}
	return theme.Text, false
}

func (m *projectTree) NodeBadge(id string, theme ui.Theme) (string, ui.Color, ui.Color, bool) {
	errs, warns, isDir := m.countForNode(id)
	if errs > 0 {
		if isDir {
			return fmt.Sprintf("⊗ %d", errs), theme.Danger, theme.Danger.WithAlpha(0.18), true
		}
		return fmt.Sprintf("⊗ %d", errs), ui.RGBA(255, 255, 255, 255), theme.Danger, true
	}
	if warns > 0 {
		wCol := warningThemeColor(theme)
		if isDir {
			return fmt.Sprintf("⚠ %d", warns), wCol, wCol.WithAlpha(0.18), true
		}
		return fmt.Sprintf("⚠ %d", warns), ui.RGBA(255, 255, 255, 255), wCol, true
	}
	return "", ui.Color{}, ui.Color{}, false
}

func (m *projectTree) SetFilter(filter string) {
	f := strings.ToLower(strings.TrimSpace(filter))
	if m.filter == f {
		return
	}
	m.filter = f
	m.Refresh()
}

func (m *projectTree) name() string { return fileName(m.root) }
func (m *projectTree) Roots() int {
	if m.root == "" {
		return 0
	}
	return 1
}
func (m *projectTree) Root(int) string        { return m.root }
func (m *projectTree) Label(id string) string { return fileName(id) }
func (m *projectTree) Children(id string) int { return len(m.list(id)) }
func (m *projectTree) Child(id string, index int) string {
	return m.list(id)[index]
}
func (m *projectTree) Icon(id string) *ui.ImageResource {
	if id == m.root {
		return m.icons.Folder
	}
	cleanID := filepath.Clean(id)
	if m.fileErrors[cleanID] > 0 && !m.dirs[cleanID] && m.icons != nil && m.icons.Error != nil {
		return m.icons.Error
	}
	if isDir, ok := m.dirs[id]; ok {
		return iconForPath(m.icons, id, isDir)
	}
	info, err := os.Stat(id)
	return iconForPath(m.icons, id, err == nil && info.IsDir())
}

func (m *projectTree) list(id string) []string {
	if cached, ok := m.entries[id]; ok {
		return cached
	}
	info, err := os.Stat(id)
	if err != nil || !info.IsDir() {
		m.entries[id] = nil
		return nil
	}
	m.dirs[id] = true
	entries, err := os.ReadDir(id)
	if err != nil {
		m.entries[id] = nil
		return nil
	}
	var dirs, files []string
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasPrefix(name, ".") && name != ".gitignore" {
			continue
		}
		path := filepath.Join(id, name)
		if entry.IsDir() {
			if name == "vendor" || name == "node_modules" || name == "bin" {
				continue
			}
			m.dirs[path] = true
			dirs = append(dirs, path)
		} else if isProjectSourceFile(name) {
			if m.filter != "" && !strings.Contains(strings.ToLower(name), m.filter) {
				continue
			}
			m.dirs[path] = false
			files = append(files, path)
		}
	}
	sort.Strings(dirs)
	sort.Strings(files)
	m.entries[id] = append(dirs, files...)
	return m.entries[id]
}

// openEditorsModel implements ui.ListModel, ui.ListIconModel, and ui.ListDecorationModel for the "OPEN EDITORS" section.
type openEditorEntry struct {
	path   string
	title  string
	dirty  bool
	errors int
	warns  int
}

type openEditorsModel struct {
	icons *Icons
	items []openEditorEntry
}

func (m *openEditorsModel) Len() int        { return len(m.items) }
func (m *openEditorsModel) ID(i int) string { return m.items[i].path }
func (m *openEditorsModel) Label(i int) string {
	it := m.items[i]
	if it.dirty {
		return "● " + it.title
	}
	return it.title
}
func (m *openEditorsModel) Icon(i int) *ui.ImageResource {
	if m.items[i].errors > 0 && m.icons != nil && m.icons.Error != nil {
		return m.icons.Error
	}
	return iconForPath(m.icons, m.items[i].path, false)
}
func (m *openEditorsModel) ItemColor(i int, theme ui.Theme) (ui.Color, bool) {
	if i < 0 || i >= len(m.items) {
		return theme.Text, false
	}
	if m.items[i].errors > 0 {
		return theme.Danger, true
	}
	if m.items[i].warns > 0 {
		return warningThemeColor(theme), true
	}
	return theme.Text, false
}
func (m *openEditorsModel) ItemBadge(i int, theme ui.Theme) (string, ui.Color, ui.Color, bool) {
	if i < 0 || i >= len(m.items) {
		return "", ui.Color{}, ui.Color{}, false
	}
	if m.items[i].errors > 0 {
		return fmt.Sprintf("⊗ %d", m.items[i].errors), ui.RGBA(255, 255, 255, 255), theme.Danger, true
	}
	if m.items[i].warns > 0 {
		wCol := warningThemeColor(theme)
		return fmt.Sprintf("⚠ %d", m.items[i].warns), ui.RGBA(255, 255, 255, 255), wCol, true
	}
	return "", ui.Color{}, ui.Color{}, false
}
