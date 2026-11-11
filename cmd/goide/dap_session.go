package goide

import (
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type debugSession struct {
	app                 *App
	mode                string
	programDir          string
	cmd                 *exec.Cmd
	listener            net.Listener
	conn                *protocolConn
	next                int
	pending             map[int]func(json.RawMessage, error)
	launchSent          bool
	initializedReceived bool
	configured          bool
	autoEntryBPPath     string
	autoEntryBPLine     int
	threadID            int
	activeFrameID       int
	scopeGeneration     uint64
}

// dirHasMainPackage reports whether dir contains at least one .go file with `package main`.
func dirHasMainPackage(dir string, openDocs map[string]*openDocument) bool {
	if dir == "" {
		return false
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	fset := token.NewFileSet()
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		fullPath := filepath.Join(dir, entry.Name())
		var src any
		if doc, ok := openDocs[fullPath]; ok && doc != nil {
			src = doc.document.Text()
		}
		f, err := parser.ParseFile(fset, fullPath, src, parser.PackageClauseOnly)
		if err == nil && f != nil && f.Name != nil && f.Name.Name == "main" {
			return true
		}
	}
	return false
}

// findMainFuncEntry searches dir for `func main()` and returns the file path and
// 0-based line number of its first executable line (or declaration line).
func findMainFuncEntry(dir string, openDocs map[string]*openDocument) (string, int, bool) {
	if dir == "" {
		return "", 0, false
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", 0, false
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		fullPath := filepath.Join(dir, entry.Name())
		var text string
		if doc, ok := openDocs[fullPath]; ok && doc != nil {
			text = doc.document.Text()
		} else if data, err := os.ReadFile(fullPath); err == nil {
			text = string(data)
		} else {
			continue
		}
		fset := token.NewFileSet()
		f, _ := parser.ParseFile(fset, fullPath, text, parser.AllErrors)
		if f == nil || f.Name == nil || f.Name.Name != "main" {
			continue
		}
		for _, decl := range f.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Name == nil || fn.Name.Name != "main" || fn.Recv != nil {
				continue
			}
			line := max(0, fset.Position(fn.Pos()).Line-1)
			if fn.Body != nil && len(fn.Body.List) > 0 {
				stmtLine := fset.Position(fn.Body.List[0].Pos()).Line - 1
				if stmtLine >= 0 {
					line = stmtLine
				}
			}
			return fullPath, line, true
		}
	}
	return "", 0, false
}

// resolveDebugTarget determines the package directory to debug for mode ("debug" or "test").
func (a *App) resolveDebugTarget(mode string) string {
	root := a.project.root
	if root == "" {
		return "."
	}
	if active := a.active(); active != nil && within(root, active.path) && strings.HasSuffix(active.path, ".go") {
		activeDir := filepath.Dir(active.path)
		if mode == "test" {
			return activeDir
		}
		if dirHasMainPackage(activeDir, a.files) {
			return activeDir
		}
	}
	if mode == "test" {
		return root
	}
	if dirHasMainPackage(root, a.files) {
		return root
	}
	// Search cmd/* first, then other subdirectories for package main
	cmdDir := filepath.Join(root, "cmd")
	if entries, err := os.ReadDir(cmdDir); err == nil {
		for _, entry := range entries {
			if entry.IsDir() {
				sub := filepath.Join(cmdDir, entry.Name())
				if dirHasMainPackage(sub, a.files) {
					return sub
				}
			}
		}
	}
	found := ""
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || found != "" {
			return nil
		}
		if !d.IsDir() {
			return nil
		}
		name := d.Name()
		if (strings.HasPrefix(name, ".") && path != root) || name == "vendor" || name == "node_modules" || name == "bin" {
			return filepath.SkipDir
		}
		if dirHasMainPackage(path, a.files) {
			found = path
			return filepath.SkipAll
		}
		return nil
	})
	if found != "" {
		return found
	}
	return root
}

func startDebug(app *App, mode string) (*debugSession, error) {
	dlvBin := app.dlvBinary()
	if !isExecutableFile(dlvBin) {
		if lp, err := exec.LookPath(dlvBin); err == nil {
			dlvBin = lp
		} else {
			app.appendLogf("debug", "[Delve] Отладчик dlv не найден — запускается автоматическая установка dlv@latest...\n")
			app.appendLogf("output", "[Delve] Отладчик dlv не найден — запускается автоматическая установка dlv@latest...\n")
			app.InstallGoToolAsync("dlv", nonEmptyStr(app.selectedToolVersions["dlv"], "latest"))
			return nil, fmt.Errorf("dlv не установлен (запущена автоустановка dlv@latest)")
		}
	}

	programDir := app.resolveDebugTarget(mode)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}

	cmd := exec.Command(dlvBin, "dap", "--client-addr="+listener.Addr().String())
	cmd.Dir = programDir
	cmd.Env = app.toolEnv()
	cmd.Stdout, cmd.Stderr = app.logWriter("dlv"), app.logWriter("dlv")
	if err := cmd.Start(); err != nil {
		_ = listener.Close()
		return nil, err
	}

	d := &debugSession{
		app:             app,
		mode:            mode,
		programDir:      programDir,
		cmd:             cmd,
		listener:        listener,
		pending:         make(map[int]func(json.RawMessage, error)),
		autoEntryBPLine: -1,
	}

	app.appendLogf("debug", "\n[Delve DAP] Сборка и запуск отладки (%s) пакета %s...\n", mode, programDir)
	app.appendLogf("output", "\n[Delve DAP] Сборка и запуск отладки (%s) пакета %s...\n", mode, programDir)
	app.setTaskBusy(true, "Delve: сборка ("+fileName(programDir)+")")

	go func() { _ = cmd.Wait() }()
	go func() {
		if tcp, ok := listener.(*net.TCPListener); ok {
			_ = tcp.SetDeadline(time.Now().Add(20 * time.Second))
		}
		conn, err := listener.Accept()
		_ = listener.Close()
		app.manager.Post(func() {
			if app.debug != d || app.closed {
				if conn != nil {
					_ = conn.Close()
				}
				return
			}
			if err != nil {
				app.setTaskBusy(false, "")
				app.setStatus("dlv: " + err.Error())
				app.appendLogf("debug", "[Delve] Ошибка подключения DAP: %v\n", err)
				d.Close()
				app.debug = nil
				app.updateDebugBadge("Debug: idle", false)
				return
			}
			d.conn = newProtocolConn(conn, conn, app.manager.Post, d.receive, func(err error) {
				if app.debug == d && !app.closed {
					app.setTaskBusy(false, "")
					app.clearAllDebugLines()
					app.setStatus("dlv: " + err.Error())
					app.updateDebugBadge("Debug: stopped", false)
				}
			})
			d.request("initialize", map[string]any{
				"clientID":        "nativeForms-goide",
				"adapterID":       "go",
				"pathFormat":      "path",
				"linesStartAt1":   true,
				"columnsStartAt1": true,
			}, func(_ json.RawMessage, err error) {
				if app.debug != d {
					return
				}
				if err != nil {
					app.setTaskBusy(false, "")
					app.setStatus("dlv initialize: " + err.Error())
					app.appendLogf("debug", "[Delve] Ошибка initialize: %v\n", err)
					return
				}
				d.launchSent = true
				d.request("launch", map[string]any{
					"name":       "Go " + mode,
					"type":       "go",
					"request":    "launch",
					"mode":       mode,
					"program":    programDir,
					"cwd":        programDir,
					"outputMode": "remote",
				}, func(_ json.RawMessage, err error) {
					if app.debug != d {
						return
					}
					app.setTaskBusy(false, "")
					if err != nil {
						msg := "dlv launch: " + err.Error()
						app.setStatus(msg)
						app.appendLogf("debug", "[Delve] Ошибка запуска: %s\n", err.Error())
						app.appendLogf("output", "[Delve] Ошибка запуска: %s\n", err.Error())
						app.updateDebugBadge("Debug: error", false)
						return
					}
					app.appendLogf("debug", "[Delve DAP] Сборка завершена, процесс запущен.\n")
				})
				if d.initializedReceived {
					d.configureAndStart()
				}
			})
		})
	}()
	return d, nil
}

func (d *debugSession) Close() {
	if d == nil {
		return
	}
	if d.listener != nil {
		_ = d.listener.Close()
	}
	if d.conn != nil {
		d.conn.Close()
	}
	if d.cmd != nil && d.cmd.Process != nil {
		_ = d.cmd.Process.Kill()
	}
}

func (d *debugSession) request(command string, args any, callback func(json.RawMessage, error)) {
	if d == nil || d.conn == nil {
		return
	}
	d.next++
	id := d.next
	if callback != nil {
		d.pending[id] = callback
	}
	err := d.conn.send(map[string]any{
		"seq":       id,
		"type":      "request",
		"command":   command,
		"arguments": args,
	})
	if err != nil {
		delete(d.pending, id)
		if callback != nil {
			callback(nil, err)
		}
	}
}

// configureAndStart sends all breakpoints (or an automatic entry breakpoint on
// func main() if the user has not set any breakpoints yet) and then sends
// configurationDone once breakpoints are registered.
func (d *debugSession) configureAndStart() {
	if d == nil || d.configured || d.app == nil || d.app.debug != d {
		return
	}
	d.configured = true

	paths := make([]string, 0, len(d.app.breakpoints))
	for path, lines := range d.app.breakpoints {
		if len(lines) > 0 {
			paths = append(paths, path)
		}
	}
	sort.Strings(paths)

	// If the user has not set any breakpoints and launched "debug", automatically
	// set an entry breakpoint at the start of func main() so debugging pauses at
	// main() after building instead of exiting silently.
	if len(paths) == 0 && d.mode == "debug" && d.programDir != "" {
		if mainPath, mainLine, ok := findMainFuncEntry(d.programDir, d.app.files); ok {
			d.autoEntryBPPath = mainPath
			d.autoEntryBPLine = mainLine
			paths = append(paths, mainPath)
			d.app.appendLogf("debug", "[Delve DAP] Точки останова не заданы — установлена автоматическая остановка на входе в main() (%s:%d)\n", fileName(mainPath), mainLine+1)
		}
	}

	sendConfigDone := func() {
		if d.app == nil || d.app.debug != d {
			return
		}
		d.request("configurationDone", map[string]any{}, func(_ json.RawMessage, err error) {
			if d.app != nil && d.app.debug == d {
				d.app.setTaskBusy(false, "")
				if err != nil {
					d.app.setStatus("dlv configuration: " + err.Error())
				}
			}
		})
		d.app.updateDebugBadge("Debug: running", true)
	}

	if len(paths) == 0 {
		sendConfigDone()
		return
	}

	remaining := len(paths)
	for _, path := range paths {
		d.setBreakpointsWithCallback(path, func() {
			remaining--
			if remaining == 0 {
				sendConfigDone()
			}
		})
	}
}
