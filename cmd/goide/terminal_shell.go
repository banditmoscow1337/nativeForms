package goide

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// DetectSystemShell returns the default system shell path (zsh, bash, sh, pwsh, powershell, cmd).
func DetectSystemShell() string {
	if runtime.GOOS == "windows" {
		for _, candidate := range []string{"pwsh.exe", "powershell.exe", "cmd.exe"} {
			if p, err := exec.LookPath(candidate); err == nil {
				return p
			}
		}
		if comspec := os.Getenv("COMSPEC"); comspec != "" {
			return comspec
		}
		return "powershell.exe"
	}
	if sh := os.Getenv("SHELL"); sh != "" && isExecutableFile(sh) {
		return sh
	}
	for _, candidate := range []string{"/bin/zsh", "/usr/bin/zsh", "/bin/bash", "/usr/bin/bash", "/bin/sh"} {
		if isExecutableFile(candidate) {
			return candidate
		}
	}
	if p, err := exec.LookPath("sh"); err == nil {
		return p
	}
	return "/bin/sh"
}

// AvailableSystemShells lists all system shells available on the current OS.
func AvailableSystemShells() []string {
	var shells []string
	seen := make(map[string]bool)
	add := func(p string) {
		if p == "" {
			return
		}
		if full, err := exec.LookPath(p); err == nil {
			p = full
		}
		if !isExecutableFile(p) {
			return
		}
		if !seen[p] {
			seen[p] = true
			shells = append(shells, p)
		}
	}
	if runtime.GOOS == "windows" {
		for _, c := range []string{"pwsh.exe", "powershell.exe", "cmd.exe", "bash.exe"} {
			add(c)
		}
	} else {
		add(os.Getenv("SHELL"))
		for _, c := range []string{"/bin/zsh", "/usr/bin/zsh", "/bin/bash", "/usr/bin/bash", "/bin/fish", "/usr/local/bin/fish", "/bin/sh"} {
			add(c)
		}
	}
	if len(shells) == 0 {
		shells = append(shells, DetectSystemShell())
	}
	return shells
}

func stripANSI(s string) string {
	if !strings.Contains(s, "\x1b") {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		if s[i] == '\x1b' && i+1 < len(s) && s[i+1] == '[' {
			i += 2
			for i < len(s) && !((s[i] >= 'A' && s[i] <= 'Z') || (s[i] >= 'a' && s[i] <= 'z')) {
				i++
			}
			continue
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

func shellCommandArgs(shellPath, rawCmd string) (string, []string) {
	if shellPath == "" {
		shellPath = DetectSystemShell()
	}
	base := strings.ToLower(filepath.Base(shellPath))
	switch {
	case strings.Contains(base, "powershell") || strings.Contains(base, "pwsh"):
		return shellPath, []string{"-NoLogo", "-NoProfile", "-Command", rawCmd}
	case base == "cmd" || base == "cmd.exe":
		return shellPath, []string{"/C", rawCmd}
	default:
		return shellPath, []string{"-c", rawCmd}
	}
}

func (a *App) effectiveTerminalCwd() string {
	if a.terminalCwd != "" {
		if info, err := os.Stat(a.terminalCwd); err == nil && info.IsDir() {
			return a.terminalCwd
		}
	}
	if a.project != nil && a.project.root != "" {
		a.terminalCwd = a.project.root
		return a.terminalCwd
	}
	if wd, err := os.Getwd(); err == nil {
		a.terminalCwd = wd
		return wd
	}
	return "."
}

func (a *App) updateTerminalPromptBadge() {
	shName := filepath.Base(nonEmptyStr(a.terminalShell, DetectSystemShell()))
	shName = strings.TrimSuffix(shName, ".exe")
	cwd := filepath.Base(a.effectiveTerminalCwd())
	if cwd == "" || cwd == "." {
		cwd = "~"
	}
	promptStr := fmt.Sprintf("%s (%s) $ ", shName, cwd)
	if a.terminalPromptBadge != nil {
		a.terminalPromptBadge.SetText(strings.TrimSpace(promptStr))
	}
	if a.terminalOutput != nil {
		a.terminalOutput.Prompt = promptStr
		a.terminalOutput.UIState().InvalidatePaint()
	}
}

// SetTerminalShell switches the system shell used by the bottom Terminal panel.
func (a *App) SetTerminalShell(shellPath string) {
	if shellPath == "" {
		return
	}
	a.terminalShell = shellPath
	a.updateTerminalPromptBadge()
	a.appendLogf("terminal", "\n[Терминал] Активная системная оболочка: %s (рабочая папка: %s)\n", shellPath, a.effectiveTerminalCwd())
	a.setStatus("Оболочка терминала: " + filepath.Base(shellPath))
}

// RunTerminalCommand executes any system shell command (zsh/bash/powershell/cmd)
// in the bottom Terminal panel, supporting `cd`, `clear`, stdin forwarding to
// running commands, and full PATH integration with the selected Go toolchain.
func (a *App) RunTerminalCommand(raw string) {
	rawTrimmed := strings.TrimSpace(raw)
	if rawTrimmed == "" {
		return
	}
	// If an interactive command is currently running and waiting for stdin, forward the line to it.
	a.termMu.Lock()
	stdin := a.termStdin
	running := a.termRunning
	a.termMu.Unlock()
	if running && stdin != nil {
		if rawTrimmed == "Ctrl+C" || rawTrimmed == ":kill" {
			a.stopRunningTask()
			return
		}
		_, _ = io.WriteString(stdin, raw+"\n")
		return
	}

	fields := strings.Fields(rawTrimmed)
	if len(fields) == 0 {
		return
	}
	if fields[0] == "clear" || fields[0] == "cls" {
		if a.terminalOutput != nil {
			a.terminalOutput.Clear()
		}
		return
	}
	// Handle `cd <dir>` persistently across terminal commands.
	if fields[0] == "cd" {
		target := ""
		if len(fields) == 1 {
			if h, err := os.UserHomeDir(); err == nil {
				target = h
			}
		} else {
			target = strings.Trim(strings.TrimSpace(strings.TrimPrefix(rawTrimmed, "cd")), `"'`)
			if strings.HasPrefix(target, "~") {
				if h, err := os.UserHomeDir(); err == nil {
					target = filepath.Join(h, strings.TrimPrefix(target, "~"))
				}
			}
		}
		if !filepath.IsAbs(target) {
			target = filepath.Join(a.effectiveTerminalCwd(), target)
		}
		target = filepath.Clean(target)
		info, err := os.Stat(target)
		if err != nil || !info.IsDir() {
			a.appendLogf("terminal", "cd: директория не найдена: %s\n", target)
			return
		}
		a.terminalCwd = target
		a.updateTerminalPromptBadge()
		return
	}

	cwd := a.effectiveTerminalCwd()
	shPath, shArgs := shellCommandArgs(a.terminalShell, rawTrimmed)
	shBase := strings.TrimSuffix(filepath.Base(shPath), ".exe")

	a.stopRunningTask()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	a.taskCancel = cancel
	a.setTaskBusy(true, shBase+": "+fields[0])
	if a.bottomTabs != nil {
		a.bottomTabs.Select("terminal")
	}

	env := a.toolEnv()
	root := a.project.root
	if root == "" {
		root = cwd
	}

	go func() {
		defer cancel()
		start := time.Now()
		cmd := exec.CommandContext(ctx, shPath, shArgs...)
		cmd.Dir = cwd
		cmd.Env = env

		stdinPipe, _ := cmd.StdinPipe()
		stdout, err1 := cmd.StdoutPipe()
		stderr, err2 := cmd.StderrPipe()
		if err1 != nil || err2 != nil {
			a.manager.Post(func() {
				if !a.closed {
					a.setTaskBusy(false, "")
					a.appendLogf("terminal", "Ошибка инициализации потоков ввода/вывода терминала\n")
				}
			})
			return
		}

		if err := cmd.Start(); err != nil {
			a.manager.Post(func() {
				if !a.closed {
					a.setTaskBusy(false, "")
					a.appendLogf("terminal", "Не удалось запустить оболочку %s: %v\n", shPath, err)
					a.setStatus("Ошибка терминала: " + err.Error())
				}
			})
			return
		}

		a.termMu.Lock()
		a.termStdin = stdinPipe
		a.termRunning = true
		a.termMu.Unlock()

		var combined bytes.Buffer
		doneStream := make(chan struct{}, 2)
		streamPipe := func(r *bufio.Scanner) {
			defer func() { doneStream <- struct{}{} }()
			for r.Scan() {
				clean := stripANSI(r.Text())
				combined.WriteString(clean + "\n")
				a.manager.Post(func() {
					if !a.closed && a.terminalOutput != nil {
						a.terminalOutput.AppendText(clean + "\n")
					}
				})
			}
		}

		go streamPipe(bufio.NewScanner(stdout))
		go streamPipe(bufio.NewScanner(stderr))
		<-doneStream
		<-doneStream

		waitErr := cmd.Wait()
		a.termMu.Lock()
		if a.termStdin == stdinPipe {
			a.termStdin = nil
			a.termRunning = false
		}
		a.termMu.Unlock()

		elapsed := time.Since(start).Round(time.Millisecond)
		fullOut := combined.String()
		parsedDiags := parseGoCompilerOutput(root, fullOut, "terminal")

		a.manager.Post(func() {
			if a.closed {
				return
			}
			a.setTaskBusy(false, "")
			if len(parsedDiags) > 0 {
				a.setToolDiagnostics("terminal", parsedDiags)
			}
			if waitErr != nil {
				msg := fmt.Sprintf("[exit: %v, %s]", waitErr, elapsed)
				a.appendLogf("terminal", "%s\n", msg)
				a.setStatus("Терминал: " + msg)
			} else {
				a.setStatus(fmt.Sprintf("Команда выполнена (%s)", elapsed))
			}
		})
	}()
}
