package goide

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// FetchAvailableGoSDKVersions queries go.dev/dl/?mode=json for official Go SDK releases,
// returning a curated fallback list if offline.
func FetchAvailableGoSDKVersions() []string {
	fallback := []string{"go1.24.1", "go1.23.6", "go1.22.12", "go1.21.13"}
	client := &http.Client{Timeout: 4 * time.Second}
	resp, err := client.Get("https://go.dev/dl/?mode=json&include=all")
	if err != nil {
		return fallback
	}
	defer resp.Body.Close()
	var releases []struct {
		Version string `json:"version"`
		Stable  bool   `json:"stable"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&releases); err != nil || len(releases) == 0 {
		return fallback
	}
	var out []string
	for _, r := range releases {
		if r.Version != "" {
			out = append(out, r.Version)
			if len(out) >= 16 {
				break
			}
		}
	}
	if len(out) == 0 {
		return fallback
	}
	return out
}

func extractTarGz(r io.Reader, destDir string) error {
	gz, err := gzip.NewReader(r)
	if err != nil {
		return err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		rel := strings.TrimPrefix(filepath.ToSlash(hdr.Name), "go/")
		if rel == "" || rel == "." || strings.Contains(rel, "..") {
			continue
		}
		target := filepath.Join(destDir, filepath.FromSlash(rel))
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				return err
			}
			mode := os.FileMode(hdr.Mode) & 0777
			if mode == 0 {
				mode = 0644
			}
			f, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
			if err != nil {
				return err
			}
			_, copyErr := io.Copy(f, tr)
			_ = f.Close()
			if copyErr != nil {
				return copyErr
			}
		}
	}
	return nil
}

func extractZipArchive(zipPath, destDir string) error {
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer zr.Close()
	for _, f := range zr.File {
		rel := strings.TrimPrefix(filepath.ToSlash(f.Name), "go/")
		if rel == "" || rel == "." || strings.Contains(rel, "..") {
			continue
		}
		target := filepath.Join(destDir, filepath.FromSlash(rel))
		if f.FileInfo().IsDir() {
			_ = os.MkdirAll(target, 0755)
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
			return err
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		mode := f.Mode() & 0777
		if mode == 0 {
			mode = 0644
		}
		out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
		if err != nil {
			_ = rc.Close()
			return err
		}
		_, copyErr := io.Copy(out, rc)
		_ = out.Close()
		_ = rc.Close()
		if copyErr != nil {
			return copyErr
		}
	}
	return nil
}

// DownloadAndInstallGoSDKAsync downloads and installs the requested Go SDK version
// (e.g. "go1.24.1") into ~/.goide/sdk/<version> and activates it in the IDE.
func (a *App) DownloadAndInstallGoSDKAsync(version string) {
	version = strings.TrimSpace(version)
	if version == "" {
		version = "go1.24.1"
	}
	if !strings.HasPrefix(version, "go") {
		version = "go" + version
	}
	sdkDir := filepath.Join(goideHomeDir(), "sdk", version)
	goBin := filepath.Join(sdkDir, "bin", exeName("go"))
	if isExecutableFile(goBin) {
		a.SetGoSDK(goBin)
		a.manager.Notify("Активирован уже установленный SDK: " + version)
		return
	}

	ext := "tar.gz"
	if runtime.GOOS == "windows" {
		ext = "zip"
	}
	archiveName := fmt.Sprintf("%s.%s-%s.%s", version, runtime.GOOS, runtime.GOARCH, ext)
	downloadURL := "https://go.dev/dl/" + archiveName

	a.setTaskBusy(true, "Скачивание "+version)
	a.appendLogf("output", "\n[Toolchain] Скачивание и установка Go SDK %s из %s...\n", version, downloadURL)
	a.appendLogf("terminal", "\n$ download %s -> %s\n", downloadURL, sdkDir)
	if a.bottomTabs != nil {
		a.bottomTabs.Select("output")
	}

	go func() {
		err := func() error {
			if err := os.MkdirAll(sdkDir, 0755); err != nil {
				return err
			}
			client := &http.Client{Timeout: 5 * time.Minute}
			resp, err := client.Get(downloadURL)
			if err != nil {
				return err
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				return fmt.Errorf("HTTP %d при скачивании %s", resp.StatusCode, downloadURL)
			}
			if ext == "zip" {
				tmpFile, err := os.CreateTemp("", "goide-sdk-*.zip")
				if err != nil {
					return err
				}
				tmpPath := tmpFile.Name()
				defer os.Remove(tmpPath)
				if _, err := io.Copy(tmpFile, resp.Body); err != nil {
					_ = tmpFile.Close()
					return err
				}
				_ = tmpFile.Close()
				return extractZipArchive(tmpPath, sdkDir)
			}
			return extractTarGz(resp.Body, sdkDir)
		}()

		a.manager.Post(func() {
			if a.closed {
				return
			}
			a.setTaskBusy(false, "")
			if err != nil {
				msg := fmt.Sprintf("✖ Не удалось скачать %s: %v", version, err)
				a.appendLogf("output", "%s\n", msg)
				a.appendLogf("terminal", "%s\n", msg)
				a.setStatus(msg)
				return
			}
			a.appendLogf("output", "✔ Go SDK %s установлен в %s\n", version, sdkDir)
			a.appendLogf("terminal", "✔ Go SDK %s готов (%s)\n", version, goBin)
			a.SetGoSDK(goBin)
			a.manager.Notify("Установлен и выбран " + version)
		})
	}()
}

// InstallGoToolAsync downloads and builds a Go tool (gopls, dlv, goimports, staticcheck)
// at the requested version (e.g. "latest", "v0.18.1") using the active Go SDK.
func (a *App) InstallGoToolAsync(toolName, version string) {
	toolName = strings.TrimSpace(toolName)
	version = strings.TrimSpace(version)
	if version == "" {
		version = "latest"
	}
	pkgPath, ok := goToolPackages[toolName]
	if !ok {
		pkgPath = toolName
	}
	goBin := a.goBinary()
	if goBin == "" || !isExecutableFile(goBin) {
		a.setStatus("Go SDK не найден — запускаю автоматическое скачивание Go SDK...")
		a.DownloadAndInstallGoSDKAsync("go1.24.1")
		return
	}

	targetBinDir := filepath.Join(goideHomeDir(), "tools", "bin")
	if version != "latest" {
		safeVer := strings.ReplaceAll(version, "/", "_")
		targetBinDir = filepath.Join(goideHomeDir(), "tools", toolName+"@"+safeVer, "bin")
	}
	_ = os.MkdirAll(targetBinDir, 0755)
	spec := pkgPath + "@" + version

	a.setTaskBusy(true, "Установка "+toolName+"@"+version)
	a.appendLogf("output", "\n[Toolchain] Установка %s (%s) в %s...\n", toolName, spec, targetBinDir)
	a.appendLogf("terminal", "\n$ GOBIN=%s %s install %s\n", targetBinDir, goBin, spec)

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		cmd := exec.CommandContext(ctx, goBin, "install", spec)
		cmd.Dir = os.TempDir()
		env := a.toolEnv()
		env = setEnvVar(env, "GOBIN", targetBinDir)
		env = setEnvVar(env, "GO111MODULE", "on")
		env = setEnvVar(env, "GOWORK", "off")
		cmd.Env = env
		out, err := cmd.CombinedOutput()
		outputText := string(out)

		a.manager.Post(func() {
			if a.closed {
				return
			}
			a.setTaskBusy(false, "")
			if outputText != "" {
				a.appendLogf("output", "%s\n", strings.TrimRight(outputText, "\n"))
				a.appendLogf("terminal", "%s\n", strings.TrimRight(outputText, "\n"))
			}
			installedBin := filepath.Join(targetBinDir, exeName(toolName))
			if err != nil || !isExecutableFile(installedBin) {
				msg := fmt.Sprintf("✖ Ошибка установки %s@%s: %v", toolName, version, err)
				a.appendLogf("output", "%s\n", msg)
				a.setStatus(msg)
				return
			}
			a.selectedToolPaths[toolName] = installedBin
			a.selectedToolVersions[toolName] = version
			a.refreshToolsSummary()
			msg := fmt.Sprintf("✔ Установлен и активирован %s (%s) -> %s", toolName, version, installedBin)
			a.appendLogf("output", "%s\n", msg)
			a.appendLogf("terminal", "%s\n", msg)
			a.setStatus(msg)
			a.manager.Notify("Установлен " + toolName + "@" + version)
			if toolName == "gopls" && a.project.root != "" {
				a.restartLSP()
			}
		})
	}()
}

// EnsureAllGoToolsAsync installs or updates gopls, dlv, goimports, and staticcheck.
func (a *App) EnsureAllGoToolsAsync() {
	goBin := a.goBinary()
	if goBin == "" || !isExecutableFile(goBin) {
		a.DownloadAndInstallGoSDKAsync("go1.24.1")
		return
	}
	tools := []string{"gopls", "dlv", "goimports", "staticcheck"}
	targetBinDir := filepath.Join(goideHomeDir(), "tools", "bin")
	_ = os.MkdirAll(targetBinDir, 0755)
	a.setTaskBusy(true, "Установка инструментов Go")
	a.appendLogf("output", "\n[Toolchain] Автоматическая проверка и установка инструментов Go (gopls, dlv, goimports, staticcheck)...\n")
	if a.bottomTabs != nil {
		a.bottomTabs.Select("output")
	}

	go func() {
		for _, name := range tools {
			ver := a.selectedToolVersions[name]
			if ver == "" {
				ver = "latest"
			}
			spec := goToolPackages[name] + "@" + ver
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
			cmd := exec.CommandContext(ctx, goBin, "install", spec)
			cmd.Dir = os.TempDir()
			env := a.toolEnv()
			env = setEnvVar(env, "GOBIN", targetBinDir)
			env = setEnvVar(env, "GO111MODULE", "on")
			env = setEnvVar(env, "GOWORK", "off")
			cmd.Env = env
			out, err := cmd.CombinedOutput()
			cancel()
			tool := name
			toolVer := ver
			outStr := string(out)
			a.manager.Post(func() {
				if a.closed {
					return
				}
				if outStr != "" {
					a.appendLogf("output", "%s\n", strings.TrimRight(outStr, "\n"))
				}
				binPath := filepath.Join(targetBinDir, exeName(tool))
				if err == nil && isExecutableFile(binPath) {
					a.selectedToolPaths[tool] = binPath
					a.appendLogf("output", "✔ %s@%s установлен (%s)\n", tool, toolVer, binPath)
				} else if err != nil {
					a.appendLogf("output", "⚠ %s@%s: %v\n", tool, toolVer, err)
				}
			})
		}
		a.manager.Post(func() {
			if a.closed {
				return
			}
			a.setTaskBusy(false, "")
			a.refreshToolsSummary()
			a.setStatus("Инструменты Go проверены и обновлены")
			a.manager.Notify("Инструменты Go готовы к работе")
			if a.project.root != "" && a.lsp == nil {
				a.restartLSP()
			}
		})
	}()
}
