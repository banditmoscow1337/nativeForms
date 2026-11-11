package goide

import (
	"bufio"
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	ui "github.com/banditmoscow1337/nativeForms"
	"github.com/banditmoscow1337/nativeForms/editor"
	"github.com/banditmoscow1337/nativeForms/platform"
)

type testHost struct{}

func (testHost) Capabilities() platform.Capabilities {
	return platform.Capabilities{FileDialog: true, Clipboard: true}
}
func (testHost) OpenFolder() (string, error)     { return "", nil }
func (testHost) OpenFile() (string, error)       { return "", nil }
func (testHost) SaveFile(string) (string, error) { return "", nil }

func TestProtocolFraming(t *testing.T) {
	body := []byte(`{"type":"request","command":"initialize","text":"😀"}`)
	var buffer bytes.Buffer
	if err := writeFrame(&buffer, body); err != nil {
		t.Fatal(err)
	}
	got, err := readFrame(bufio.NewReader(&buffer))
	if err != nil || !bytes.Equal(got, body) {
		t.Fatalf("frame round trip: %s, %v", got, err)
	}
	if _, err := readFrame(bufio.NewReader(strings.NewReader("Content-Length: 16777217\r\n\r\n"))); err == nil {
		t.Fatal("accepted oversized frame")
	}
	if _, err := readFrame(bufio.NewReader(strings.NewReader("Content-Length: 4\r\n\r\nhi"))); err == nil {
		t.Fatal("accepted truncated frame")
	}
}

func TestIncrementalEditsUseOldUTF16Positions(t *testing.T) {
	old := "a😀\nимя\n"
	start := strings.Index(old, "имя")
	edits := []editor.Edit{{Start: 1, End: len("a😀"), Text: "X"}, {Start: start, End: start + len("имя"), Text: "new"}}
	changes := incrementalChanges(old, edits)
	if len(changes) != 2 {
		t.Fatalf("changes: %d", len(changes))
	}
	first, _ := json.Marshal(changes[0])
	second, _ := json.Marshal(changes[1])
	if !bytes.Contains(first, []byte(`"line":1,"character":0`)) ||
		!bytes.Contains(second, []byte(`"line":0,"character":1`)) ||
		!bytes.Contains(second, []byte(`"line":0,"character":3`)) {
		t.Fatalf("UTF-16 ranges/order: %s then %s", first, second)
	}
	same := incrementalChanges("x", []editor.Edit{{Start: 1, End: 1, Text: "A"}, {Start: 1, End: 1, Text: "B"}})
	if same[0].(map[string]any)["text"] != "B" || same[1].(map[string]any)["text"] != "A" {
		t.Fatal("same-position insertions were not sent in reverse order")
	}
}

func TestCompletionAppliesImportAndUTF16Edit(t *testing.T) {
	source := "package main\n\nfunc main() { 😀 }\n"
	doc, err := editor.NewDocument(source)
	if err != nil {
		t.Fatal(err)
	}
	file := &openDocument{document: doc, view: ui.NewCodeEditor(doc)}
	// UTF-16 column 16 is immediately after the emoji (column 14..15).
	item := lspCompletion{
		Label: "print",
		TextEdit: &lspTextEdit{
			Range: &lspRange{
				Start: lspPosition{Line: 2, Character: 16},
				End:   lspPosition{Line: 2, Character: 16},
			},
			NewText: "fmt.Println()",
		},
		AdditionalTextEdits: []lspTextEdit{{
			Range: &lspRange{
				Start: lspPosition{Line: 1, Character: 0},
				End:   lspPosition{Line: 1, Character: 0},
			},
			NewText: "import \"fmt\"\n",
		}},
	}
	if !applyCompletion(file, item, doc.Version()) {
		t.Fatal("completion edit rejected")
	}
	if !strings.Contains(doc.Text(), "import \"fmt\"") || !strings.Contains(doc.Text(), "fmt.Println()") {
		t.Fatalf("completion lost edits: %q", doc.Text())
	}
	if applyCompletion(file, item, 0) {
		t.Fatal("stale completion was applied")
	}
}

func TestProjectOpensSeveralFilesAndSavesDirtyTab(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/demo\n\ngo 1.22\n"), 0644); err != nil {
		t.Fatal(err)
	}
	one, two := filepath.Join(dir, "one.go"), filepath.Join(dir, "two.go")
	for _, path := range []string{one, two} {
		if err := os.WriteFile(path, []byte("package demo\n"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	manager := ui.New()
	app := New(manager, testHost{})
	manager.SetRoot(app.Root)
	app.project = newProjectTree(dir) // Exercise the UI without requiring gopls.
	app.fileTree.Model = app.project
	app.fileTree.SetExpanded(dir, true)
	if err := app.openFile(one); err != nil {
		t.Fatal(err)
	}
	if err := app.openFile(two); err != nil {
		t.Fatal(err)
	}
	if len(app.files) != 2 || app.editorTabs.SelectedID != two || !reflect.DeepEqual(app.fileTree.Expanded, map[string]bool{dir: true}) {
		t.Fatal("module tree or two document tabs were not populated")
	}
	view := app.files[two].view
	if !view.ReplaceTextRange(len("package demo\n"), len("package demo\n"), "var число = 1\n") {
		t.Fatal("editor rejected Cyrillic edit")
	}
	if !app.files[two].dirty || !app.editorTabs.Tabs[1].Dirty || app.editorTabs.Close(two) {
		t.Fatal("dirty tab closed or lost its indicator")
	}
	view.SetBreakpoint(1, true)
	if !app.breakpoints[two][1] {
		t.Fatal("breakpoint not stored for Delve")
	}
	if err := app.SaveActive(); err != nil {
		t.Fatal(err)
	}
	saved, err := os.ReadFile(two)
	if err != nil || !strings.Contains(string(saved), "число") {
		t.Fatalf("save failed: %v %q", err, saved)
	}
	if !app.editorTabs.Close(two) || len(app.files) != 1 {
		t.Fatal("saved tab did not close")
	}
	app.Close()
}

func TestLanguageAndDebugEventsPopulatePanels(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "main.go")
	app := New(ui.New(), testHost{})
	app.project = newProjectTree(dir)
	lsp := &lspClient{app: app, pending: make(map[int]func(json.RawMessage, error))}
	app.lsp = lsp
	message, _ := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"method":  "textDocument/publishDiagnostics",
		"params": map[string]any{
			"uri": fileURI(path),
			"diagnostics": []any{
				map[string]any{
					"range": map[string]any{
						"start": map[string]int{"line": 2, "character": 4},
						"end":   map[string]int{"line": 2, "character": 8},
					},
					"severity": 1,
					"message":  "undefined: value",
				},
			},
		},
	})
	lsp.receive(message)
	if app.diagnostics.Len() != 1 || app.diagnostics.rows[0].line != 2 ||
		app.diagnostics.rows[0].message != "undefined: value" {
		t.Fatalf("diagnostics panel: %+v", app.diagnostics.rows)
	}
	dap := &debugSession{app: app, pending: make(map[int]func(json.RawMessage, error))}
	app.debug = dap
	dap.receive(json.RawMessage(`{"type":"event","event":"output","body":{"output":"debug output\n"}}`))
	if got := app.output.Lines(); !reflect.DeepEqual(got, []string{"debug output"}) {
		t.Fatalf("debug output panel: %q", got)
	}
	app.lsp = nil
	app.debug = nil
	app.Close()
}

func TestSelectingNewFrameIgnoresOldVariables(t *testing.T) {
	app := New(ui.New(), testHost{})
	dap := &debugSession{
		app:     app,
		pending: make(map[int]func(json.RawMessage, error)),
		conn:    &protocolConn{queue: make(chan []byte, 4), done: make(chan struct{})},
	}
	app.debug = dap
	dap.fetchScopes(10)
	old := dap.pending[1]
	dap.fetchScopes(20)
	old(json.RawMessage(`{"scopes":[{"name":"old","variablesReference":1}]}`), nil)
	if dap.next != 2 || app.variables.Len() != 0 {
		t.Fatal("stale scope response added variables")
	}
	dap.pending[2](json.RawMessage(`{"scopes":[{"name":"new","variablesReference":2}]}`), nil)
	if dap.next != 3 {
		t.Fatal("new frame did not request variables")
	}
	dap.pending[3](json.RawMessage(`{"variables":[{"name":"answer","value":"42","type":"int"}]}`), nil)
	if app.variables.Len() != 1 || app.variables.rows[0].value != "42" {
		t.Fatalf("wrong frame variables: %+v", app.variables.rows)
	}
	app.debug = nil
	app.Close()
}

func TestEditorSplitsCloseSavedAndCloseAll(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/splits\n\ngo 1.24\n"), 0644); err != nil {
		t.Fatal(err)
	}
	f1 := filepath.Join(dir, "a.go")
	f2 := filepath.Join(dir, "b.go")
	f3 := filepath.Join(dir, "c.go")
	for _, p := range []string{f1, f2, f3} {
		if err := os.WriteFile(p, []byte("package splits\n"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	mgr := ui.New()
	app := New(mgr, testHost{})
	defer app.Close()
	mgr.SetRoot(app.Root)
	app.project = newProjectTree(dir)
	app.fileTree.Model = app.project

	if err := app.openFile(f1); err != nil {
		t.Fatal(err)
	}
	if err := app.openFile(f2); err != nil {
		t.Fatal(err)
	}
	if err := app.openFile(f3); err != nil {
		t.Fatal(err)
	}

	// Test 2 and 3 side-by-side editor splits
	app.SetEditorSplits(2)
	mgr.BeginFrame(1440, 900, 0.016)
	if app.splitCount != 2 || !app.editorGroups[1].UIState().Visible() || app.editorGroups[2].UIState().Visible() {
		t.Fatalf("expected 2 visible editor splits, got splitCount=%d", app.splitCount)
	}
	app.SetEditorSplits(3)
	mgr.BeginFrame(1440, 900, 0.016)
	if app.splitCount != 3 || !app.editorGroups[1].UIState().Visible() || !app.editorGroups[2].UIState().Visible() {
		t.Fatalf("expected 3 visible editor splits, got splitCount=%d", app.splitCount)
	}

	// Mark f2 dirty, keep f1 and f3 saved; CloseSavedTabs should close f1 and f3 but keep f2 open.
	app.files[f2].view.ReplaceTextRange(0, 0, "// edited\n")
	if !app.files[f2].dirty {
		t.Fatal("expected f2 to be dirty")
	}
	closedSaved := app.CloseSavedTabs()
	if closedSaved != 2 || len(app.files) != 1 || app.files[f2] == nil {
		t.Fatalf("CloseSavedTabs: closed=%d remaining=%d", closedSaved, len(app.files))
	}

	// CloseAllTabsForce should close even dirty files.
	app.CloseAllTabsForce()
	if len(app.files) != 0 {
		t.Fatalf("expected 0 open files after CloseAllTabsForce, got %d", len(app.files))
	}
}

func TestVSCodeWorkbenchASTFormatAndGoToolsIntegration(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/vscodego\n\ngo 1.24\n"), 0644); err != nil {
		t.Fatal(err)
	}
	unformatted := "package main\nimport \"fmt\"\ntype Server struct {\nPort int\n}\nfunc (s *Server) Start(addr string) error {\nfmt.Println(addr)\nreturn nil\n}\nfunc main(){srv := &Server{Port: 8080}; _ = srv.Start(\":8080\")}\n"
	mainPath := filepath.Join(dir, "main.go")
	if err := os.WriteFile(mainPath, []byte(unformatted), 0644); err != nil {
		t.Fatal(err)
	}

	mgr := ui.New()
	app := New(mgr, testHost{})
	defer app.Close()
	mgr.SetRoot(app.Root)
	mgr.BeginFrame(1440, 900, 0.016)

	app.project = newProjectTree(dir)
	app.fileTree.Model = app.project
	app.fileTree.SetExpanded(dir, true)
	if err := app.openFile(mainPath); err != nil {
		t.Fatal(err)
	}

	// 1. Verify AST Outline symbols were extracted automatically (package, Server struct, (*Server).Start, main).
	if app.outline.Len() < 4 {
		t.Fatalf("expected >= 4 AST outline symbols, got %d: %+v", app.outline.Len(), app.outline.rows)
	}

	// 2. Verify in-process go/format (FormatActiveDocument) formats unformatted Go code.
	if !app.FormatActiveDocument() {
		t.Fatal("FormatActiveDocument failed")
	}
	formattedText := app.active().document.Text()
	if !strings.Contains(formattedText, "\tPort int") || !strings.Contains(formattedText, "func main() {\n") {
		t.Fatalf("code was not gofmt-formatted: %q", formattedText)
	}

	// 3. Verify Workspace AST Go-to-Definition resolves (*Server).Start across module files.
	def, ok := FindWorkspaceDefinition(dir, "Start", app.files)
	if !ok || def.kind != "method" || def.path != mainPath {
		t.Fatalf("FindWorkspaceDefinition failed for method Start: ok=%v, def=%+v", ok, def)
	}

	// 4. Verify Go compiler / go vet output parser populates Problems table.
	compilerLog := "main.go:7:2: undefined: missingSymbol\nmain.go:9:5: unreachable code\n"
	parsed := parseGoCompilerOutput(dir, compilerLog, "go build")
	if len(parsed) != 2 || parsed[0].line != 6 || parsed[0].column != 1 || parsed[0].message != "undefined: missingSymbol" {
		t.Fatalf("unexpected parsed compiler diagnostics: %+v", parsed)
	}
	app.setToolDiagnostics("go build", parsed)
	if app.diagnostics.Len() != 2 {
		t.Fatalf("expected 2 diagnostics in Problems table, got %d", app.diagnostics.Len())
	}

	// 5. Verify go test -json event handler populates Test Results table.
	app.handleTestJSONEvent(goTestEvent{
		Action:  "pass",
		Package: "example.com/vscodego",
		Test:    "TestServerStart",
		Elapsed: 0.042,
	})
	if app.testResults.Len() != 1 || app.testResults.rows[0].status != "PASS" || app.testResults.rows[0].name != "TestServerStart" {
		t.Fatalf("unexpected testResults: %+v", app.testResults.rows)
	}

	// 6. Verify VS Code Activity Bar, Sidebar switching, Theme toggle, and Command Palette dialog.
	for _, viewID := range []string{"search", "outline", "debug", "gotools", "settings", "explorer"} {
		app.SelectSidebarView(viewID)
		mgr.BeginFrame(1440, 900, 0.016)
		if app.activeSidebarID != viewID {
			t.Fatalf("sidebar view did not switch to %s", viewID)
		}
	}
	app.ToggleTheme()
	if !mgr.Theme().IsLight() {
		t.Fatal("expected Light+ theme after ToggleTheme")
	}
	app.ToggleTheme()
	if mgr.Theme().IsLight() {
		t.Fatal("expected Dark+ theme after second ToggleTheme")
	}
	app.OpenCommandPalette()
	mgr.BeginFrame(1440, 900, 0.016)
	mgr.CloseTopPopup()
}

func TestSanitizeGoProxy(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"", "https://proxy.golang.org,direct"},
		{"   ", "https://proxy.golang.org,direct"},
		{",,,", "https://proxy.golang.org,direct"},
		{" , \t , ", "https://proxy.golang.org,direct"},
		{"\"\"", "https://proxy.golang.org,direct"},
		{"https://goproxy.io,direct", "https://goproxy.io,direct"},
	}
	for _, tc := range cases {
		if got := sanitizeGoProxy(tc.in); got != tc.want {
			t.Fatalf("sanitizeGoProxy(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestDiagnosticsInEditorTreeAndJumpFromProblems(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/diagtest\n\ngo 1.24\n"), 0644); err != nil {
		t.Fatal(err)
	}
	brokenSrc := "package main\n\ntype Config struct {\n\tPort int\n}\n\nfunc main() {\n\tcfg := Config{Port: }\n}\n"
	mainPath := filepath.Join(dir, "main.go")
	if err := os.WriteFile(mainPath, []byte(brokenSrc), 0644); err != nil {
		t.Fatal(err)
	}

	mgr := ui.New()
	app := New(mgr, testHost{})
	defer app.Close()
	mgr.SetRoot(app.Root)
	mgr.BeginFrame(1440, 900, 0.016)

	app.project = newProjectTree(dir)
	app.fileTree.Model = app.project
	app.fileTree.SetExpanded(dir, true)
	if err := app.openFile(mainPath); err != nil {
		t.Fatal(err)
	}

	// 1. Verify syntax error is detected and pushed to CodeEditor per-line diagnostics
	view := app.activeEditorView(app.active())
	if len(view.Diagnostics()) == 0 {
		t.Fatal("expected per-line diagnostics in CodeEditor for broken syntax")
	}

	// 2. Verify Explorer projectTree marks the file and root folder in red (Danger)
	col, ok := app.project.NodeColor(mainPath, mgr.Theme())
	if !ok || col != mgr.Theme().Danger {
		t.Fatalf("expected file with error to be colored Danger in file tree, got ok=%v col=%+v", ok, col)
	}
	badgeText, _, _, badgeOK := app.project.NodeBadge(mainPath, mgr.Theme())
	if !badgeOK || !strings.Contains(badgeText, "⊗") {
		t.Fatalf("expected error badge on file in tree, got ok=%v text=%q", badgeOK, badgeText)
	}

	// 3. Verify jumping from Problems places caret on the error line and focuses the CodeEditor
	view.SetSelection(0, 0)
	if app.diagnostics.Len() == 0 {
		t.Fatal("expected diagnostics in Problems table")
	}
	firstDiag := app.diagnostics.rows[0]
	app.diagnosticsView.OnSelect(firstDiag.id)
	start, _, _ := view.Document.LineRange(firstDiag.line)
	if view.Caret < start || !view.UIState().Focused() {
		t.Fatalf("expected caret on diagnostic line %d (start=%d, got caret=%d) and editor focused=%v", firstDiag.line, start, view.Caret, view.UIState().Focused())
	}

	// 4. Verify AST & lexer highlight user-defined types (Config) as CodeType
	styles := ExtractGoASTStyleRanges("package main\ntype Config struct { Next *Config }\nfunc New(c Config) *Config { return nil }\n")
	hasType := false
	for _, r := range styles {
		if r.Style == ui.CodeType {
			hasType = true
			break
		}
	}
	if !hasType {
		t.Fatalf("expected CodeType ranges from ExtractGoASTStyleRanges, got %+v", styles)
	}
}
