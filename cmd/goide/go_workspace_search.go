package goide

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// FindWorkspaceDefinition searches all Go files in root (preferring open in-memory
// buffers in openDocs) for a top-level declaration matching symbol.
func FindWorkspaceDefinition(root string, symbol string, openDocs map[string]*openDocument) (outlineSymbol, bool) {
	if symbol == "" {
		return outlineSymbol{}, false
	}
	matchSym := func(s outlineSymbol) bool {
		if s.name == symbol {
			return true
		}
		if s.kind == "method" && strings.HasSuffix(s.name, ")."+symbol) {
			return true
		}
		return false
	}
	for path, doc := range openDocs {
		if !strings.HasSuffix(path, ".go") {
			continue
		}
		syms, _ := ParseGoOutline(path, doc.document.Text())
		for _, s := range syms {
			if s.kind != "package" && matchSym(s) {
				return s, true
			}
		}
	}
	if root == "" {
		return outlineSymbol{}, false
	}
	var found outlineSymbol
	okFound := false
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || okFound {
			return nil
		}
		if d.IsDir() {
			name := d.Name()
			if (strings.HasPrefix(name, ".") && path != root) || name == "vendor" || name == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(d.Name(), ".go") {
			return nil
		}
		if _, isOpen := openDocs[path]; isOpen {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		syms, _ := ParseGoOutline(path, string(data))
		for _, s := range syms {
			if s.kind != "package" && matchSym(s) {
				found = s
				okFound = true
				return filepath.SkipAll
			}
		}
		return nil
	})
	return found, okFound
}

// CollectWorkspaceSymbols gathers all AST symbols across the module for Ctrl+Shift+O / Ctrl+T.
func CollectWorkspaceSymbols(root string, openDocs map[string]*openDocument) []outlineSymbol {
	var all []outlineSymbol
	seenFiles := make(map[string]bool)
	for path, doc := range openDocs {
		if !strings.HasSuffix(path, ".go") {
			continue
		}
		seenFiles[path] = true
		syms, _ := ParseGoOutline(path, doc.document.Text())
		for _, s := range syms {
			if s.kind != "package" {
				all = append(all, s)
			}
		}
	}
	if root != "" {
		_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
			if err != nil || len(all) > 2000 {
				return nil
			}
			if d.IsDir() {
				name := d.Name()
				if (strings.HasPrefix(name, ".") && path != root) || name == "vendor" || name == "node_modules" {
					return filepath.SkipDir
				}
				return nil
			}
			if !strings.HasSuffix(d.Name(), ".go") || seenFiles[path] {
				return nil
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return nil
			}
			syms, _ := ParseGoOutline(path, string(data))
			for _, s := range syms {
				if s.kind != "package" {
					all = append(all, s)
				}
			}
			return nil
		})
	}
	sort.Slice(all, func(i, j int) bool {
		if all[i].name != all[j].name {
			return all[i].name < all[j].name
		}
		return all[i].path < all[j].path
	})
	return all
}

// SearchProjectText searches all project files under root (and open buffers) for query.
func SearchProjectText(root, query string, caseSensitive bool, openDocs map[string]*openDocument) []searchMatch {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil
	}
	needle := query
	if !caseSensitive {
		needle = strings.ToLower(query)
	}
	var matches []searchMatch
	seen := make(map[string]bool)

	scanContent := func(path, text string) {
		lines := strings.Split(text, "\n")
		for lineIdx, line := range lines {
			haystack := line
			if !caseSensitive {
				haystack = strings.ToLower(line)
			}
			col := strings.Index(haystack, needle)
			if col >= 0 {
				preview := strings.TrimSpace(line)
				if len(preview) > 120 {
					preview = preview[:120] + "…"
				}
				matches = append(matches, searchMatch{
					id:      fmt.Sprintf("%s:%d:%d", path, lineIdx, col),
					path:    path,
					line:    lineIdx,
					column:  col,
					preview: preview,
				})
				if len(matches) >= 500 {
					return
				}
			}
		}
	}

	for path, doc := range openDocs {
		if root != "" && !within(root, path) {
			continue
		}
		seen[path] = true
		scanContent(path, doc.document.Text())
		if len(matches) >= 500 {
			return matches
		}
	}

	if root != "" {
		_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
			if err != nil || len(matches) >= 500 {
				return nil
			}
			if d.IsDir() {
				name := d.Name()
				if (strings.HasPrefix(name, ".") && path != root) || name == "vendor" || name == "node_modules" {
					return filepath.SkipDir
				}
				return nil
			}
			if seen[path] || !isProjectSourceFile(d.Name()) {
				return nil
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return nil
			}
			scanContent(path, string(data))
			return nil
		})
	}
	return matches
}

// parseGoCompilerOutput extracts `file.go:line:col: message` diagnostics from
// `go build`, `go vet`, or `go test` output.
func parseGoCompilerOutput(root, output, sourceTool string) []diagnosticRow {
	var rows []diagnosticRow
	lines := strings.Split(output, "\n")
	for i, raw := range lines {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "?") || strings.HasPrefix(line, "ok ") || strings.HasPrefix(line, "=== ") || strings.HasPrefix(line, "--- ") {
			continue
		}
		path, ln, col, msg, ok := parseFileLineColMessage(root, line)
		if !ok {
			continue
		}
		sev := 1
		if sourceTool == "go vet" {
			sev = 2
		}
		rows = append(rows, diagnosticRow{
			id:       fmt.Sprintf("%s:%s:%d:%d:%d", sourceTool, path, ln, col, i),
			path:     path,
			line:     ln,
			column:   col,
			message:  msg,
			severity: sev,
			source:   sourceTool,
		})
	}
	return rows
}

func parseFileLineColMessage(root, line string) (fullPath string, lineNum, colNum int, msg string, ok bool) {
	idxGo := strings.Index(line, ".go:")
	if idxGo < 0 {
		return "", 0, 0, "", false
	}
	filePart := strings.TrimSpace(line[:idxGo+3])
	rest := line[idxGo+4:]
	colon1 := strings.IndexByte(rest, ':')
	if colon1 <= 0 {
		return "", 0, 0, "", false
	}
	ln, err := strconv.Atoi(rest[:colon1])
	if err != nil || ln < 1 {
		return "", 0, 0, "", false
	}
	rest = rest[colon1+1:]
	col := 1
	colon2 := strings.IndexByte(rest, ':')
	if colon2 > 0 {
		if c, err := strconv.Atoi(rest[:colon2]); err == nil && c >= 1 {
			col = c
			rest = rest[colon2+1:]
		}
	}
	msg = strings.TrimSpace(rest)
	if msg == "" {
		return "", 0, 0, "", false
	}
	fullPath = filePart
	if !filepath.IsAbs(fullPath) && root != "" {
		candidate := filepath.Clean(filepath.Join(root, fullPath))
		if _, err := os.Stat(candidate); err == nil {
			fullPath = candidate
		} else {
			baseName := filepath.Base(filePart)
			found := ""
			_ = filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
				if err != nil || found != "" {
					return nil
				}
				if d.IsDir() {
					n := d.Name()
					if (strings.HasPrefix(n, ".") && p != root) || n == "vendor" || n == "node_modules" {
						return filepath.SkipDir
					}
					return nil
				}
				if d.Name() == baseName {
					found = p
					return filepath.SkipAll
				}
				return nil
			})
			if found != "" {
				fullPath = found
			} else {
				fullPath = candidate
			}
		}
	}
	fullPath = filepath.Clean(fullPath)
	return fullPath, ln - 1, col - 1, msg, true
}

// goTestEvent matches one JSON record emitted by `go test -json`.
type goTestEvent struct {
	Time    time.Time `json:"Time"`
	Action  string    `json:"Action"`
	Package string    `json:"Package"`
	Test    string    `json:"Test"`
	Elapsed float64   `json:"Elapsed"`
	Output  string    `json:"Output"`
}

func parseGoModInfo(root string) (modulePath, goVer string) {
	if root == "" {
		return "", ""
	}
	data, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		return fileName(root), ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "module ") {
			modulePath = strings.TrimSpace(strings.TrimPrefix(line, "module "))
		} else if strings.HasPrefix(line, "go ") {
			goVer = strings.TrimSpace(strings.TrimPrefix(line, "go "))
		}
	}
	if modulePath == "" {
		modulePath = fileName(root)
	}
	return modulePath, goVer
}

func detectGitBranch(root string) string {
	if root == "" {
		return "main"
	}
	headPath := filepath.Join(root, ".git", "HEAD")
	data, err := os.ReadFile(headPath)
	if err != nil {
		return "main"
	}
	s := strings.TrimSpace(string(data))
	if strings.HasPrefix(s, "ref: refs/heads/") {
		return strings.TrimPrefix(s, "ref: refs/heads/")
	}
	if len(s) >= 7 {
		return s[:7]
	}
	return "main"
}
