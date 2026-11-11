package goide

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// AutoDiscoverToolchain scans the host for Go SDKs and tools, selects active binaries,
// and optionally triggers background installation of missing tools.
func (a *App) AutoDiscoverToolchain(installMissing bool) GoToolStatus {
	st := CheckGoTools()
	if a.selectedGoSDK == "" && st.GoPath != "" {
		a.selectedGoSDK = st.GoPath
	}
	if a.selectedToolPaths == nil {
		a.selectedToolPaths = make(map[string]string)
	}
	if a.selectedToolVersions == nil {
		a.selectedToolVersions = map[string]string{
			"gopls":       "latest",
			"dlv":         "latest",
			"goimports":   "latest",
			"staticcheck": "latest",
		}
	}
	for name, list := range st.InstalledTools {
		if a.selectedToolPaths[name] == "" && len(list) > 0 {
			a.selectedToolPaths[name] = list[0].Path
		}
	}
	if a.goVersionBadge != nil {
		if a.selectedGoSDK != "" {
			v := extractGoShortVersion(probeBinaryVersion(a.selectedGoSDK, "version"))
			a.goVersionBadge.SetText("Go: " + v)
		} else {
			a.goVersionBadge.SetText("Go: AST (" + runtime.Version() + ")")
		}
	}
	a.refreshToolsSummary()
	if installMissing && a.selectedGoSDK != "" && (a.selectedToolPaths["gopls"] == "" || a.selectedToolPaths["dlv"] == "") {
		a.EnsureAllGoToolsAsync()
	}
	return st
}

// SetGoSDK switches the active Go binary/version used by build, run, test, gopls, and terminal.
func (a *App) SetGoSDK(goBinOrVersion string) {
	goBinOrVersion = strings.TrimSpace(goBinOrVersion)
	if goBinOrVersion == "" {
		return
	}
	st := CheckGoTools()
	chosenBin := ""
	for _, sdk := range st.InstalledSDKs {
		if sdk.GoBin == goBinOrVersion || sdk.Version == goBinOrVersion {
			chosenBin = sdk.GoBin
			break
		}
	}
	if chosenBin == "" && isExecutableFile(goBinOrVersion) {
		chosenBin = goBinOrVersion
	}
	if chosenBin == "" {
		// Not yet installed locally — download this Go SDK version automatically!
		a.DownloadAndInstallGoSDKAsync(goBinOrVersion)
		return
	}
	a.selectedGoSDK = chosenBin
	ver := extractGoShortVersion(probeBinaryVersion(chosenBin, "version"))
	if a.goVersionBadge != nil {
		a.goVersionBadge.SetText("Go: " + ver)
	}
	// Also switch gofmt from the same SDK bin directory if present
	sdkGofmt := filepath.Join(filepath.Dir(chosenBin), exeName("gofmt"))
	if isExecutableFile(sdkGofmt) {
		a.selectedToolPaths["gofmt"] = sdkGofmt
	}
	a.refreshToolsSummary()
	a.setStatus(fmt.Sprintf("Активная версия Go переключена на %s (%s)", ver, chosenBin))
	if a.project.root != "" && a.lsp != nil {
		a.restartLSP()
	}
}

// SetToolVersionOrPath switches a Go tool (gopls, dlv, goimports, staticcheck) to a specific
// binary path or installs & activates the requested version tag.
func (a *App) SetToolVersionOrPath(toolName, pathOrVersion string) {
	pathOrVersion = strings.TrimSpace(pathOrVersion)
	if pathOrVersion == "" {
		return
	}
	if isExecutableFile(pathOrVersion) {
		a.selectedToolPaths[toolName] = pathOrVersion
		a.refreshToolsSummary()
		a.setStatus(fmt.Sprintf("Инструмент %s переключён на %s", toolName, pathOrVersion))
		if toolName == "gopls" && a.project.root != "" {
			a.restartLSP()
		}
		return
	}
	// Check if already installed under ~/.goide/tools/<toolName>@<ver>/bin
	safeVer := strings.ReplaceAll(pathOrVersion, "/", "_")
	candidate := filepath.Join(goideHomeDir(), "tools", toolName+"@"+safeVer, "bin", exeName(toolName))
	if isExecutableFile(candidate) {
		a.selectedToolPaths[toolName] = candidate
		a.selectedToolVersions[toolName] = pathOrVersion
		a.refreshToolsSummary()
		a.setStatus(fmt.Sprintf("Активирован %s (%s)", toolName, candidate))
		if toolName == "gopls" && a.project.root != "" {
			a.restartLSP()
		}
		return
	}
	a.selectedToolVersions[toolName] = pathOrVersion
	a.InstallGoToolAsync(toolName, pathOrVersion)
}

func (a *App) goBinary() string {
	if a.selectedGoSDK != "" && isExecutableFile(a.selectedGoSDK) {
		return a.selectedGoSDK
	}
	st := CheckGoTools()
	if st.GoPath != "" {
		a.selectedGoSDK = st.GoPath
		return st.GoPath
	}
	return "go"
}

func (a *App) goplsBinary() string {
	if p := a.selectedToolPaths["gopls"]; p != "" && isExecutableFile(p) {
		return p
	}
	st := CheckGoTools()
	if st.GoplsPath != "" {
		a.selectedToolPaths["gopls"] = st.GoplsPath
		return st.GoplsPath
	}
	return "gopls"
}

func (a *App) dlvBinary() string {
	if p := a.selectedToolPaths["dlv"]; p != "" && isExecutableFile(p) {
		return p
	}
	st := CheckGoTools()
	if st.DlvPath != "" {
		a.selectedToolPaths["dlv"] = st.DlvPath
		return st.DlvPath
	}
	return "dlv"
}

func envKeyMatch(entry, key string) bool {
	eq := strings.IndexByte(entry, '=')
	if eq <= 0 {
		return false
	}
	if runtime.GOOS == "windows" {
		return strings.EqualFold(entry[:eq], key)
	}
	return entry[:eq] == key
}

func getEnvVar(env []string, key string) string {
	val := ""
	for _, entry := range env {
		if envKeyMatch(entry, key) {
			eq := strings.IndexByte(entry, '=')
			val = entry[eq+1:]
		}
	}
	return val
}

func setEnvVar(env []string, key, value string) []string {
	out := make([]string, 0, len(env)+1)
	replaced := false
	for _, entry := range env {
		if envKeyMatch(entry, key) {
			if !replaced {
				out = append(out, key+"="+value)
				replaced = true
			}
			continue
		}
		out = append(out, entry)
	}
	if !replaced {
		out = append(out, key+"="+value)
	}
	return out
}

// sanitizeGoProxy validates a GOPROXY value and returns a guaranteed-valid non-empty
// GOPROXY list so `go install` / `go mod` never fails with
// "goproxy list is not the empty string, but contains no entries".
func sanitizeGoProxy(raw string) string {
	cleaned := strings.TrimSpace(strings.Trim(raw, `"'`))
	if cleaned == "off" || cleaned == "direct" {
		return cleaned
	}
	var validParts []string
	for _, part := range strings.FieldsFunc(cleaned, func(r rune) bool { return r == ',' || r == '|' }) {
		p := strings.TrimSpace(strings.Trim(part, `"'`))
		if p == "direct" || p == "off" ||
			strings.HasPrefix(p, "https://") ||
			strings.HasPrefix(p, "http://") ||
			strings.HasPrefix(p, "file://") {
			validParts = append(validParts, p)
		}
	}
	if len(validParts) == 0 {
		return "https://proxy.golang.org,direct"
	}
	return strings.Join(validParts, ",")
}

func (a *App) toolEnv() []string {
	env := os.Environ()
	var extraPaths []string
	if a.selectedGoSDK != "" && isExecutableFile(a.selectedGoSDK) {
		sdkBin := filepath.Dir(a.selectedGoSDK)
		goroot := filepath.Dir(sdkBin)
		extraPaths = append(extraPaths, sdkBin)
		env = setEnvVar(env, "GOROOT", goroot)
	}
	for _, p := range a.selectedToolPaths {
		if p != "" {
			extraPaths = append(extraPaths, filepath.Dir(p))
		}
	}
	extraPaths = append(extraPaths, filepath.Join(goideHomeDir(), "tools", "bin"))
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		extraPaths = append(extraPaths, filepath.Join(home, "go", "bin"))
	}
	oldPath := getEnvVar(env, "PATH")
	newPath := strings.Join(extraPaths, string(os.PathListSeparator))
	if oldPath != "" {
		newPath = newPath + string(os.PathListSeparator) + oldPath
	}
	env = setEnvVar(env, "PATH", newPath)

	// Always set a sanitized, non-empty GOPROXY and GOSUMDB in cmd.Env so broken
	// host environment variables or malformed ~/.config/go/env files never cause
	// "goproxy list is not the empty string, but contains no entries".
	env = setEnvVar(env, "GOPROXY", sanitizeGoProxy(getEnvVar(env, "GOPROXY")))
	gosumdb := strings.TrimSpace(strings.Trim(getEnvVar(env, "GOSUMDB"), `"'`))
	if gosumdb == "" {
		gosumdb = "sum.golang.org"
	}
	env = setEnvVar(env, "GOSUMDB", gosumdb)
	return env
}
