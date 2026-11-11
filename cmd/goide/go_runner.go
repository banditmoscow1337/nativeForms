package goide

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// FormatActiveDocument formats the currently active Go document using go/format.Source.
func (a *App) FormatActiveDocument() bool {
	file := a.active()
	if file == nil {
		a.setStatus("Нет активного файла для форматирования")
		return false
	}
	if !strings.HasSuffix(file.path, ".go") {
		a.setStatus("Форматирование gofmt поддерживается только для .go файлов")
		return false
	}
	original := file.document.Text()
	formatted, err := FormatGoSource(original)
	if err != nil {
		a.setStatus("Ошибка синтаксиса при форматировании: " + err.Error())
		a.refreshActiveOutlineAndSyntax(file)
		return false
	}
	if formatted == original {
		a.setStatus("Код уже отформатирован: " + fileName(file.path))
		return true
	}
	view := a.activeEditorView(file)
	caretPos, _ := file.document.UTF16Position(view.Caret)
	if !view.ReplaceTextRange(0, file.document.Len(), formatted) {
		return false
	}
	if nextOffset, err := file.document.ByteOffset(caretPos); err == nil {
		view.SetSelection(nextOffset, nextOffset)
	} else {
		clamped := min(view.Caret, file.document.Len())
		view.SetSelection(clamped, clamped)
	}
	a.setStatus("Отформатировано (gofmt): " + fileName(file.path))
	return true
}

// RunGoTool executes a `go` command asynchronously inside the open module root
// using the selected Go SDK binary and toolchain environment.
func (a *App) RunGoTool(label string, args ...string) {
	if a.project.root == "" {
		a.setStatus("Сначала откройте Go-модуль")
		a.manager.Notify("Откройте Go-модуль для запуска " + label)
		return
	}
	if a.FormatOnSave {
		for _, f := range a.files {
			if f.dirty && strings.HasSuffix(f.path, ".go") {
				if formatted, err := FormatGoSource(f.document.Text()); err == nil && formatted != f.document.Text() {
					f.view.ReplaceTextRange(0, f.document.Len(), formatted)
				}
			}
		}
	}
	_ = a.saveAll()

	a.stopRunningTask()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	a.taskCancel = cancel
	a.setTaskBusy(true, label)

	goBin := a.goBinary()
	cmdLine := filepath.Base(goBin) + " " + strings.Join(args, " ")
	a.appendLogf("output", "\n$ %s  (в %s)\n", cmdLine, a.project.root)
	a.appendLogf("terminal", "\n$ %s\n", cmdLine)

	isTestJSON := len(args) >= 2 && args[0] == "test" && slicesContains(args, "-json")
	if isTestJSON {
		a.testResults.rows = nil
		if a.testResultsView != nil {
			a.testResultsView.ModelChanged()
		}
		if a.bottomTabs != nil {
			a.bottomTabs.Select("tests")
		}
	} else if a.bottomTabs != nil && len(args) > 0 && (args[0] == "run" || args[0] == "env" || args[0] == "mod" || args[0] == "version") {
		a.bottomTabs.Select("terminal")
	} else if a.bottomTabs != nil {
		a.bottomTabs.Select("output")
	}

	root := a.project.root
	env := a.toolEnv()
	go func() {
		defer cancel()
		start := time.Now()
		cmd := exec.CommandContext(ctx, goBin, args...)
		cmd.Dir = root
		cmd.Env = env

		var combined bytes.Buffer
		stdout, err1 := cmd.StdoutPipe()
		stderr, err2 := cmd.StderrPipe()
		if err1 != nil || err2 != nil {
			a.manager.Post(func() {
				if a.closed {
					return
				}
				a.setTaskBusy(false, "")
				a.setStatus(fmt.Sprintf("%s: ошибка инициализации пайпа", label))
			})
			return
		}

		if err := cmd.Start(); err != nil {
			a.manager.Post(func() {
				if a.closed {
					return
				}
				a.setTaskBusy(false, "")
				msg := fmt.Sprintf("%s: не удалось запустить %q (%v)\n", label, goBin, err)
				a.output.AppendText(msg)
				if a.terminalOutput != nil {
					a.terminalOutput.AppendText(msg)
				}
				a.setStatus(label + ": " + err.Error())
			})
			return
		}

		doneStream := make(chan struct{}, 2)
		streamPipe := func(r *bufio.Scanner, isStdout bool) {
			defer func() { doneStream <- struct{}{} }()
			for r.Scan() {
				line := r.Text()
				if isTestJSON && isStdout && strings.HasPrefix(strings.TrimSpace(line), "{") {
					var ev goTestEvent
					if json.Unmarshal([]byte(line), &ev) == nil {
						if ev.Output != "" {
							combined.WriteString(ev.Output)
						}
						a.manager.Post(func() {
							if !a.closed {
								a.handleTestJSONEvent(ev)
							}
						})
						continue
					}
				}
				combined.WriteString(line + "\n")
				a.manager.Post(func() {
					if !a.closed {
						a.output.AppendText(line + "\n")
						if a.terminalOutput != nil {
							a.terminalOutput.AppendText(line + "\n")
						}
					}
				})
			}
		}

		go streamPipe(bufio.NewScanner(stdout), true)
		go streamPipe(bufio.NewScanner(stderr), false)
		<-doneStream
		<-doneStream

		waitErr := cmd.Wait()
		elapsed := time.Since(start).Round(time.Millisecond)
		fullOut := combined.String()

		var parsedDiags []diagnosticRow
		if len(args) > 0 && (args[0] == "build" || args[0] == "vet" || args[0] == "test" || args[0] == "run") {
			parsedDiags = parseGoCompilerOutput(root, fullOut, "go "+args[0])
		}

		a.manager.Post(func() {
			if a.closed {
				return
			}
			a.setTaskBusy(false, "")
			if len(args) > 0 && (args[0] == "build" || args[0] == "vet" || args[0] == "test") {
				a.setToolDiagnostics("go "+args[0], parsedDiags)
			}
			if len(args) > 0 && args[0] == "mod" {
				a.RefreshProjectTree()
			}
			if waitErr != nil {
				summary := fmt.Sprintf("✖ %s завершился с ошибкой (%v, %s)", label, waitErr, elapsed)
				a.appendLogf("output", "%s\n", summary)
				a.appendLogf("terminal", "%s\n", summary)
				a.setStatus(summary)
				if len(parsedDiags) > 0 && a.bottomTabs != nil {
					a.bottomTabs.Select("problems")
				}
			} else {
				summary := fmt.Sprintf("✔ %s успешно выполнен (%s)", label, elapsed)
				a.appendLogf("output", "%s\n", summary)
				a.appendLogf("terminal", "%s\n", summary)
				a.setStatus(summary)
				a.manager.Notify(summary)
			}
		})
	}()
}

func (a *App) handleTestJSONEvent(ev goTestEvent) {
	if ev.Output != "" {
		a.output.AppendText(ev.Output)
		if a.terminalOutput != nil {
			a.terminalOutput.AppendText(ev.Output)
		}
	}
	if ev.Action != "pass" && ev.Action != "fail" && ev.Action != "skip" {
		return
	}
	status := strings.ToUpper(ev.Action)
	id := ev.Package + "::" + ev.Test
	filePath := ""
	line := 0
	if ev.Test != "" {
		if sym, ok := FindWorkspaceDefinition(a.project.root, ev.Test, a.files); ok {
			filePath = sym.path
			line = sym.line
		}
	}
	row := testResultRow{
		id:       id,
		pkg:      ev.Package,
		name:     ev.Test,
		status:   status,
		elapsed:  ev.Elapsed,
		filePath: filePath,
		line:     line,
	}
	for i := range a.testResults.rows {
		if a.testResults.rows[i].id == id {
			a.testResults.rows[i] = row
			if a.testResultsView != nil {
				a.testResultsView.ModelChanged()
			}
			return
		}
	}
	a.testResults.rows = append(a.testResults.rows, row)
	if a.testResultsView != nil {
		a.testResultsView.ModelChanged()
	}
}

func slicesContains(ss []string, target string) bool {
	for _, s := range ss {
		if s == target {
			return true
		}
	}
	return false
}

// BuildProject runs `go build ./...` in the active module.
func (a *App) BuildProject() {
	a.RunGoTool("go build", "build", "./...")
}

// RunProject runs `go run .` (or the directory of the active file if it is package main).
func (a *App) RunProject() {
	target := "."
	if file := a.active(); file != nil && within(a.project.root, file.path) {
		dir := filepath.Dir(file.path)
		if rel, err := filepath.Rel(a.project.root, dir); err == nil && rel != "." {
			target = "./" + filepath.ToSlash(rel)
		}
	}
	a.RunGoTool("go run "+target, "run", target)
}

// TestProject runs `go test -json ./...` across the module and populates the Test Results tab.
func (a *App) TestProject() {
	a.RunGoTool("go test", "test", "-json", "./...")
}

// TestActivePackage runs `go test -json -v` for the package of the currently active file.
func (a *App) TestActivePackage() {
	target := "./..."
	if file := a.active(); file != nil && within(a.project.root, file.path) {
		dir := filepath.Dir(file.path)
		if rel, err := filepath.Rel(a.project.root, dir); err == nil {
			if rel == "." {
				target = "."
			} else {
				target = "./" + filepath.ToSlash(rel)
			}
		}
	}
	a.RunGoTool("go test "+target, "test", "-json", "-v", target)
}

// VetProject runs `go vet ./...` and reports issues in the Problems tab.
func (a *App) VetProject() {
	a.RunGoTool("go vet", "vet", "./...")
}

// TidyModule runs `go mod tidy` and refreshes the project file tree.
func (a *App) TidyModule() {
	a.RunGoTool("go mod tidy", "mod", "tidy")
}

// GoEnv prints the Go environment in the Terminal panel.
func (a *App) GoEnv() {
	a.RunGoTool("go env", "env")
}
