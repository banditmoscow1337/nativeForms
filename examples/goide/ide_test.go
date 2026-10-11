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
func (testHost) Capabilities() platform.Capabilities { return platform.Capabilities{FileDialog:true} }
func (testHost) OpenFolder() (string,error) { return "",nil }
func (testHost) OpenFile() (string,error) { return "",nil }
func (testHost) SaveFile(string) (string,error) { return "",nil }

func TestProtocolFraming(t *testing.T) {
	body:=[]byte(`{"type":"request","command":"initialize","text":"😀"}`)
	var buffer bytes.Buffer
	if err:=writeFrame(&buffer,body);err!=nil { t.Fatal(err) }
	got,err:=readFrame(bufio.NewReader(&buffer))
	if err!=nil || !bytes.Equal(got,body) { t.Fatalf("frame round trip: %s, %v",got,err) }
	if _,err:=readFrame(bufio.NewReader(strings.NewReader("Content-Length: 16777217\r\n\r\n")));err==nil {
		t.Fatal("accepted oversized frame")
	}
	if _,err:=readFrame(bufio.NewReader(strings.NewReader("Content-Length: 4\r\n\r\nhi")));err==nil {
		t.Fatal("accepted truncated frame")
	}
}

func TestIncrementalEditsUseOldUTF16Positions(t *testing.T) {
	old:="a😀\nимя\n"
	start:=strings.Index(old,"имя")
	edits:=[]editor.Edit{{Start:1,End:len("a😀"),Text:"X"},{Start:start,End:start+len("имя"),Text:"new"}}
	changes:=incrementalChanges(old,edits)
	if len(changes)!=2 { t.Fatalf("changes: %d",len(changes)) }
	first,_:=json.Marshal(changes[0])
	second,_:=json.Marshal(changes[1])
	if !bytes.Contains(first,[]byte(`"line":1,"character":0`)) ||
		!bytes.Contains(second,[]byte(`"line":0,"character":1`)) ||
		!bytes.Contains(second,[]byte(`"line":0,"character":3`)) {
		t.Fatalf("UTF-16 ranges/order: %s then %s",first,second)
	}
	same:=incrementalChanges("x",[]editor.Edit{{Start:1,End:1,Text:"A"},{Start:1,End:1,Text:"B"}})
	if same[0].(map[string]any)["text"]!="B" || same[1].(map[string]any)["text"]!="A" {
		t.Fatal("same-position insertions were not sent in reverse order")
	}
}

func TestCompletionAppliesImportAndUTF16Edit(t *testing.T) {
	source:="package main\n\nfunc main() { 😀 }\n"
	doc,err:=editor.NewDocument(source)
	if err!=nil { t.Fatal(err) }
	file:=&openDocument{document:doc,view:ui.NewCodeEditor(doc)}
	// UTF-16 column 16 is immediately after the emoji (column 14..15).
	item:=lspCompletion{
		Label:"print", TextEdit:&lspTextEdit{Range:&lspRange{
			Start:lspPosition{Line:2,Character:16},End:lspPosition{Line:2,Character:16},
		},NewText:"fmt.Println()"},
		AdditionalTextEdits:[]lspTextEdit{{Range:&lspRange{
			Start:lspPosition{Line:1,Character:0},End:lspPosition{Line:1,Character:0},
		},NewText:"import \"fmt\"\n"}},
	}
	if !applyCompletion(file,item,doc.Version()) { t.Fatal("completion edit rejected") }
	if !strings.Contains(doc.Text(),"import \"fmt\"") || !strings.Contains(doc.Text(),"fmt.Println()") {
		t.Fatalf("completion lost edits: %q",doc.Text())
	}
	if applyCompletion(file,item,0) { t.Fatal("stale completion was applied") }
}

func TestProjectOpensSeveralFilesAndSavesDirtyTab(t *testing.T) {
	dir:=t.TempDir()
	if err:=os.WriteFile(filepath.Join(dir,"go.mod"),[]byte("module example.com/demo\n\ngo 1.22\n"),0644);err!=nil { t.Fatal(err) }
	one,two:=filepath.Join(dir,"one.go"),filepath.Join(dir,"two.go")
	for _,path:=range []string{one,two} {
		if err:=os.WriteFile(path,[]byte("package demo\n"),0644);err!=nil { t.Fatal(err) }
	}
	manager:=ui.New()
	app:=New(manager,testHost{})
	manager.SetRoot(app.Root)
	app.project=newProjectTree(dir) // Exercise the UI without requiring gopls.
	app.fileTree.Model=app.project
	app.fileTree.SetExpanded(dir,true)
	if err:=app.openFile(one);err!=nil { t.Fatal(err) }
	if err:=app.openFile(two);err!=nil { t.Fatal(err) }
	if len(app.files)!=2 || app.editorTabs.SelectedID!=two || !reflect.DeepEqual(app.fileTree.Expanded,map[string]bool{dir:true}) {
		t.Fatal("module tree or two document tabs were not populated")
	}
	view:=app.files[two].view
	if !view.ReplaceTextRange(len("package demo\n"),len("package demo\n"),"var число = 1\n") {
		t.Fatal("editor rejected Cyrillic edit")
	}
	if !app.files[two].dirty || !app.editorTabs.Tabs[1].Dirty || app.editorTabs.Close(two) {
		t.Fatal("dirty tab closed or lost its indicator")
	}
	view.SetBreakpoint(1,true)
	if !app.breakpoints[two][1] { t.Fatal("breakpoint not stored for Delve") }
	if err:=app.SaveActive();err!=nil { t.Fatal(err) }
	saved,err:=os.ReadFile(two)
	if err!=nil || !strings.Contains(string(saved),"число") { t.Fatalf("save failed: %v %q",err,saved) }
	if !app.editorTabs.Close(two) || len(app.files)!=1 { t.Fatal("saved tab did not close") }
	app.Close()
}

func TestLanguageAndDebugEventsPopulatePanels(t *testing.T) {
	dir:=t.TempDir()
	path:=filepath.Join(dir,"main.go")
	app:=New(ui.New(),testHost{})
	app.project=newProjectTree(dir)
	lsp:=&lspClient{app:app,pending:make(map[int]func(json.RawMessage,error))}
	app.lsp=lsp
	message,_:=json.Marshal(map[string]any{
		"jsonrpc":"2.0","method":"textDocument/publishDiagnostics",
		"params":map[string]any{"uri":fileURI(path),"diagnostics":[]any{
			map[string]any{"range":map[string]any{
				"start":map[string]int{"line":2,"character":4},
				"end":map[string]int{"line":2,"character":8}},
				"severity":1,"message":"undefined: value"},
		}},
	})
	lsp.receive(message)
	if app.diagnostics.Len()!=1 || app.diagnostics.rows[0].line!=2 ||
		app.diagnostics.rows[0].message!="undefined: value" {
		t.Fatalf("diagnostics panel: %+v",app.diagnostics.rows)
	}
	dap:=&debugSession{app:app,pending:make(map[int]func(json.RawMessage,error))}
	app.debug=dap
	dap.receive(json.RawMessage(`{"type":"event","event":"output","body":{"output":"debug output\n"}}`))
	if got:=app.output.Lines(); !reflect.DeepEqual(got,[]string{"debug output"}) {
		t.Fatalf("debug output panel: %q",got)
	}
	app.lsp=nil
	app.debug=nil
	app.Close()
}

func TestSelectingNewFrameIgnoresOldVariables(t *testing.T) {
	app:=New(ui.New(),testHost{})
	dap:=&debugSession{app:app,pending:make(map[int]func(json.RawMessage,error)),
		conn:&protocolConn{queue:make(chan []byte,4),done:make(chan struct{})}}
	app.debug=dap
	dap.fetchScopes(10)
	old:=dap.pending[1]
	dap.fetchScopes(20)
	old(json.RawMessage(`{"scopes":[{"name":"old","variablesReference":1}]}`),nil)
	if dap.next!=2 || app.variables.Len()!=0 { t.Fatal("stale scope response added variables") }
	dap.pending[2](json.RawMessage(`{"scopes":[{"name":"new","variablesReference":2}]}`),nil)
	if dap.next!=3 { t.Fatal("new frame did not request variables") }
	dap.pending[3](json.RawMessage(`{"variables":[{"name":"answer","value":"42","type":"int"}]}`),nil)
	if app.variables.Len()!=1 || app.variables.rows[0].value!="42" {
		t.Fatalf("wrong frame variables: %+v",app.variables.rows)
	}
	app.debug=nil
	app.Close()
}
