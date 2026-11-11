package goide

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"
)

// GoSDKInfo describes a discovered or downloaded Go SDK installation.
type GoSDKInfo struct {
	Version string // e.g. "go1.24.1"
	GoBin   string // absolute path to go(.exe)
	GoRoot  string // GOROOT directory
}

// ToolBinaryInfo describes a discovered or installed Go tool binary.
type ToolBinaryInfo struct {
	Name    string // e.g. "gopls", "dlv", "goimports", "staticcheck", "gofmt"
	Version string // version string or "installed"
	Path    string // absolute path to executable
}

// GoToolStatus summarizes which Go development tools and SDKs are available.
type GoToolStatus struct {
	GoPath          string
	GoVersion       string
	GofmtPath       string
	GoplsPath       string
	GoplsVersion    string
	DlvPath         string
	DlvVersion      string
	GoimportsPath   string
	StaticcheckPath string
	InstalledSDKs   []GoSDKInfo
	InstalledTools  map[string][]ToolBinaryInfo
}

var goToolPackages = map[string]string{
	"gopls":       "golang.org/x/tools/gopls",
	"dlv":         "github.com/go-delve/delve/cmd/dlv",
	"goimports":   "golang.org/x/tools/cmd/goimports",
	"staticcheck": "honnef.co/go/tools/cmd/staticcheck",
}

func exeName(base string) string {
	if runtime.GOOS == "windows" && !strings.HasSuffix(strings.ToLower(base), ".exe") {
		return base + ".exe"
	}
	return base
}

func goideHomeDir() string {
	if h, err := os.UserHomeDir(); err == nil && h != "" {
		return filepath.Join(h, ".goide")
	}
	return filepath.Join(os.TempDir(), ".goide")
}

func candidateBinDirs() []string {
	var dirs []string
	seen := make(map[string]bool)
	addDir := func(d string) {
		d = strings.TrimSpace(d)
		if d == "" {
			return
		}
		d = filepath.Clean(d)
		if !seen[d] {
			seen[d] = true
			dirs = append(dirs, d)
		}
	}

	// 1. GoIDE managed SDKs & tools directories
	ideHome := goideHomeDir()
	addDir(filepath.Join(ideHome, "tools", "bin"))
	if matches, err := filepath.Glob(filepath.Join(ideHome, "tools", "*", "bin")); err == nil {
		sort.Sort(sort.Reverse(sort.StringSlice(matches)))
		for _, m := range matches {
			addDir(m)
		}
	}
	if matches, err := filepath.Glob(filepath.Join(ideHome, "sdk", "go*", "bin")); err == nil {
		sort.Sort(sort.Reverse(sort.StringSlice(matches)))
		for _, m := range matches {
			addDir(m)
		}
	}

	// 2. Environment variables
	if gobin := os.Getenv("GOBIN"); gobin != "" {
		addDir(gobin)
	}
	if gopath := os.Getenv("GOPATH"); gopath != "" {
		for _, p := range filepath.SplitList(gopath) {
			addDir(filepath.Join(p, "bin"))
		}
	}
	if goroot := os.Getenv("GOROOT"); goroot != "" {
		addDir(filepath.Join(goroot, "bin"))
	}

	// 3. User home SDKs & ~/go/bin
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		addDir(filepath.Join(home, "go", "bin"))
		for _, pattern := range []string{
			filepath.Join(home, "sdk", "go*", "bin"),
			filepath.Join(home, ".asdf", "installs", "golang", "*", "go", "bin"),
			filepath.Join(home, ".asdf", "installs", "golang", "*", "packages", "bin"),
			filepath.Join(home, ".gvm", "gos", "go*", "bin"),
			filepath.Join(home, ".local", "share", "mise", "installs", "go", "*", "bin"),
			filepath.Join(home, "scoop", "apps", "go", "current", "bin"),
		} {
			if matches, err := filepath.Glob(pattern); err == nil {
				sort.Sort(sort.Reverse(sort.StringSlice(matches)))
				for _, m := range matches {
					addDir(m)
				}
			}
		}
	}

	// 4. PATH entries
	for _, p := range filepath.SplitList(os.Getenv("PATH")) {
		addDir(p)
	}

	// 5. Standard OS installation locations
	if runtime.GOOS == "windows" {
		addDir(`C:\Program Files\Go\bin`)
		addDir(`C:\Go\bin`)
		if localApp := os.Getenv("LOCALAPPDATA"); localApp != "" {
			addDir(filepath.Join(localApp, "Programs", "Go", "bin"))
		}
	} else {
		addDir("/usr/local/go/bin")
		addDir("/opt/homebrew/bin")
		addDir("/opt/homebrew/opt/go/libexec/bin")
		addDir("/usr/local/opt/go/libexec/bin")
		addDir("/usr/local/bin")
		addDir("/usr/bin")
		addDir("/snap/bin")
	}
	return dirs
}

func isExecutableFile(p string) bool {
	info, err := os.Stat(p)
	if err != nil || info.IsDir() {
		return false
	}
	if runtime.GOOS == "windows" {
		return true
	}
	return info.Mode()&0111 != 0
}

func probeBinaryVersion(binPath string, args ...string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
	defer cancel()
	cmd := exec.CommandContext(ctx, binPath, args...)
	out, err := cmd.CombinedOutput()
	if err != nil && len(out) == 0 {
		return ""
	}
	line := strings.TrimSpace(string(out))
	if idx := strings.IndexByte(line, '\n'); idx >= 0 {
		line = strings.TrimSpace(line[:idx])
	}
	return line
}

func extractGoShortVersion(raw string) string {
	for _, field := range strings.Fields(raw) {
		if strings.HasPrefix(field, "go1.") || strings.HasPrefix(field, "go2.") {
			return field
		}
	}
	if raw != "" {
		return raw
	}
	return "go"
}

// CheckGoTools automatically searches PATH and standard SDK/GOPATH/GoIDE directories
// for go, gofmt, gopls, dlv, goimports, and staticcheck.
func CheckGoTools() GoToolStatus {
	st := GoToolStatus{
		InstalledTools: make(map[string][]ToolBinaryInfo),
	}
	dirs := candidateBinDirs()
	seenSDK := make(map[string]bool)
	seenToolPath := make(map[string]bool)

	recordGo := func(bin string) {
		if !isExecutableFile(bin) {
			return
		}
		realBin := bin
		if eval, err := filepath.EvalSymlinks(bin); err == nil {
			realBin = eval
		}
		if seenSDK[realBin] {
			return
		}
		seenSDK[realBin] = true
		verOut := probeBinaryVersion(bin, "version")
		shortVer := extractGoShortVersion(verOut)
		goroot := filepath.Dir(filepath.Dir(realBin))
		sdk := GoSDKInfo{
			Version: shortVer,
			GoBin:   bin,
			GoRoot:  goroot,
		}
		st.InstalledSDKs = append(st.InstalledSDKs, sdk)
		if st.GoPath == "" {
			st.GoPath = bin
			st.GoVersion = nonEmptyStr(verOut, shortVer)
		}
	}

	if p, err := exec.LookPath("go"); err == nil {
		recordGo(p)
	}
	for _, d := range dirs {
		recordGo(filepath.Join(d, exeName("go")))
	}

	toolNames := []string{"gofmt", "gopls", "dlv", "goimports", "staticcheck"}
	for _, name := range toolNames {
		recordTool := func(p string) {
			if !isExecutableFile(p) {
				return
			}
			cleanP := filepath.Clean(p)
			if seenToolPath[cleanP] {
				return
			}
			seenToolPath[cleanP] = true
			ver := "installed"
			switch name {
			case "gopls":
				if v := probeBinaryVersion(cleanP, "version"); v != "" {
					ver = v
				}
			case "dlv":
				if v := probeBinaryVersion(cleanP, "version"); v != "" {
					ver = v
				}
			case "staticcheck":
				if v := probeBinaryVersion(cleanP, "-version"); v != "" {
					ver = v
				}
			}
			info := ToolBinaryInfo{Name: name, Version: ver, Path: cleanP}
			st.InstalledTools[name] = append(st.InstalledTools[name], info)
			switch name {
			case "gofmt":
				if st.GofmtPath == "" {
					st.GofmtPath = cleanP
				}
			case "gopls":
				if st.GoplsPath == "" {
					st.GoplsPath = cleanP
					st.GoplsVersion = ver
				}
			case "dlv":
				if st.DlvPath == "" {
					st.DlvPath = cleanP
					st.DlvVersion = ver
				}
			case "goimports":
				if st.GoimportsPath == "" {
					st.GoimportsPath = cleanP
				}
			case "staticcheck":
				if st.StaticcheckPath == "" {
					st.StaticcheckPath = cleanP
				}
			}
		}
		if p, err := exec.LookPath(name); err == nil {
			recordTool(p)
		}
		for _, d := range dirs {
			recordTool(filepath.Join(d, exeName(name)))
		}
	}
	return st
}
