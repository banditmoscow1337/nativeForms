package goide

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sort"
	"strings"
	"unicode/utf16"

	ui "github.com/banditmoscow1337/nativeForms"
	"github.com/banditmoscow1337/nativeForms/editor"
)

type lspPosition struct {
	Line int `json:"line"`
	Character int `json:"character"`
}
type lspRange struct {
	Start lspPosition `json:"start"`
	End lspPosition `json:"end"`
}
type lspDiagnostic struct {
	Range lspRange `json:"range"`
	Message string `json:"message"`
	Severity int `json:"severity"`
}
type lspTextEdit struct {
	Range *lspRange `json:"range,omitempty"`
	Insert *lspRange `json:"insert,omitempty"`
	Replace *lspRange `json:"replace,omitempty"`
	NewText string `json:"newText"`
}
type lspCompletion struct {
	Label string `json:"label"`
	InsertText string `json:"insertText"`
	TextEdit *lspTextEdit `json:"textEdit"`
	AdditionalTextEdits []lspTextEdit `json:"additionalTextEdits"`
}

type lspClient struct {
	app *App
	cmd *exec.Cmd
	conn *protocolConn
	next int
	pending map[int]func(json.RawMessage, error)
	ready bool
	deferred []any
	syncKind int
}

func startLSP(app *App) (*lspClient, error) {
	cmd := exec.Command("gopls", "serve")
	cmd.Dir = app.project.root
	in, err := cmd.StdinPipe()
	if err != nil { return nil, err }
	out, err := cmd.StdoutPipe()
	if err != nil { return nil, err }
	cmd.Stderr = app.logWriter("gopls")
	if err := cmd.Start(); err != nil { return nil, err }
	l := &lspClient{app: app, cmd: cmd, pending: make(map[int]func(json.RawMessage, error)), syncKind: 2}
	l.conn = newProtocolConn(out, in, app.manager.Post, l.receive, func(err error) {
		if app.lsp == l && !app.closed { app.status.SetText("gopls: " + err.Error()) }
	})
	go func() { _ = cmd.Wait() }()
	l.call("initialize", map[string]any{
		"processId": os.Getpid(),
		"rootUri": fileURI(app.project.root),
		"workspaceFolders": []any{map[string]any{"uri": fileURI(app.project.root), "name": app.project.name()}},
		"capabilities": map[string]any{
			"workspace": map[string]any{"workspaceFolders": true},
			"textDocument": map[string]any{"completion": map[string]any{
				"completionItem": map[string]any{"snippetSupport": false}}},
		},
	}, func(result json.RawMessage, err error) {
		if app.lsp != l { return }
		if err != nil { app.status.SetText("gopls initialize: " + err.Error()); return }
		var response struct {
			Capabilities struct { TextDocumentSync json.RawMessage `json:"textDocumentSync"` } `json:"capabilities"`
		}
		if json.Unmarshal(result, &response) == nil && len(response.Capabilities.TextDocumentSync) != 0 {
			if response.Capabilities.TextDocumentSync[0] == '{' {
				var sync struct { Change int `json:"change"` }
				if json.Unmarshal(response.Capabilities.TextDocumentSync, &sync) == nil { l.syncKind = sync.Change }
			} else {
				_ = json.Unmarshal(response.Capabilities.TextDocumentSync, &l.syncKind)
			}
		}
		l.ready = true
		_ = l.conn.send(map[string]any{"jsonrpc":"2.0", "method":"initialized", "params":map[string]any{}})
		for _, message := range l.deferred { _ = l.conn.send(message) }
		l.deferred = nil
		app.status.SetText("gopls готов")
	})
	return l, nil
}

func (l *lspClient) Close() {
	l.conn.Close()
	if l.cmd.Process != nil { _ = l.cmd.Process.Kill() }
}

func (l *lspClient) call(method string, params any, callback func(json.RawMessage, error)) {
	l.next++
	id := l.next
	l.pending[id] = callback
	if err := l.conn.send(map[string]any{"jsonrpc":"2.0", "id":id, "method":method, "params":params}); err != nil {
		delete(l.pending, id)
		callback(nil, err)
	}
}

func (l *lspClient) notify(method string, params any) {
	message := map[string]any{"jsonrpc":"2.0", "method":method, "params":params}
	if !l.ready {
		l.deferred = append(l.deferred, message)
		return
	}
	if err := l.conn.send(message); err != nil { l.app.status.SetText("gopls: " + err.Error()) }
}

func (l *lspClient) receive(data json.RawMessage) {
	if l.app.lsp != l { return }
	var envelope struct {
		ID json.RawMessage `json:"id"`
		Method string `json:"method"`
		Params json.RawMessage `json:"params"`
		Result json.RawMessage `json:"result"`
		Error *struct { Message string `json:"message"` } `json:"error"`
	}
	if json.Unmarshal(data, &envelope) != nil { return }
	if envelope.Method != "" {
		if len(envelope.ID) > 0 {
			var result any
			switch envelope.Method {
			case "workspace/configuration":
				var params struct { Items []any `json:"items"` }
				_ = json.Unmarshal(envelope.Params, &params)
				result = make([]any, len(params.Items))
			case "workspace/applyEdit":
				result = map[string]any{"applied":false}
			default:
				result = nil
			}
			_ = l.conn.send(map[string]any{"jsonrpc":"2.0", "id":envelope.ID, "result":result})
			return
		}
		if envelope.Method == "textDocument/publishDiagnostics" {
			var params struct {
				URI string `json:"uri"`
				Version *int `json:"version"`
				Diagnostics []lspDiagnostic `json:"diagnostics"`
			}
			if json.Unmarshal(envelope.Params, &params) == nil {
				l.app.setDiagnostics(params.URI, params.Version, params.Diagnostics)
			}
		}
		return
	}
	var id int
	if json.Unmarshal(envelope.ID, &id) != nil { return }
	callback := l.pending[id]
	delete(l.pending, id)
	if callback != nil {
		if envelope.Error != nil { callback(nil, fmt.Errorf("%s", envelope.Error.Message)) } else { callback(envelope.Result, nil) }
	}
}

func (l *lspClient) open(file *openDocument) {
	file.lspVersion = 1
	file.lastText = file.document.Text()
	language := "go"
	if fileName(file.path) == "go.mod" { language = "go.mod" }
	l.notify("textDocument/didOpen", map[string]any{"textDocument":map[string]any{
		"uri":fileURI(file.path), "languageId":language, "version":file.lspVersion, "text":file.lastText}})
}

func (l *lspClient) change(file *openDocument, change editor.Change) {
	previous := file.lastText
	file.lastText = file.document.Text()
	file.lspVersion++
	if l.syncKind == 0 { return }
	var changes []any
	if l.syncKind == 1 {
		changes = []any{map[string]any{"text":file.lastText}}
	} else {
		changes = incrementalChanges(previous, change.Edits)
	}
	l.notify("textDocument/didChange", map[string]any{
		"textDocument":map[string]any{"uri":fileURI(file.path), "version":file.lspVersion},
		"contentChanges":changes,
	})
}

func (l *lspClient) closeFile(path string) {
	l.notify("textDocument/didClose", map[string]any{"textDocument":map[string]any{"uri":fileURI(path)}})
}

func utf16Position(text string, offset int) lspPosition {
	pos := lspPosition{}
	for _, r := range text[:offset] {
		if r == '\n' { pos.Line++; pos.Character = 0 } else { pos.Character += utf16.RuneLen(r) }
	}
	return pos
}

// Edits refer to one pre-transaction document. Applying them in descending
// order keeps the original ranges valid for each incremental LSP change.
func incrementalChanges(previous string, edits []editor.Edit) []any {
	sorted := append([]editor.Edit(nil), edits...)
	sort.SliceStable(sorted, func(i,j int) bool { return sorted[i].Start > sorted[j].Start })
	for first:=0; first<len(sorted); {
		last:=first+1
		for last<len(sorted) && sorted[last].Start==sorted[first].Start { last++ }
		for left,right:=first,last-1; left<right; left,right=left+1,right-1 {
			sorted[left],sorted[right]=sorted[right],sorted[left]
		}
		first=last
	}
	changes := make([]any, 0, len(sorted))
	for _, edit := range sorted {
		start, end := utf16Position(previous, edit.Start), utf16Position(previous, edit.End)
		changes = append(changes, map[string]any{"range":lspRange{Start:start,End:end}, "text":edit.Text})
	}
	return changes
}

func (l *lspClient) completion(file *openDocument) {
	version := file.document.Version()
	pos, err := file.document.UTF16Position(file.view.Caret)
	if err != nil { return }
	l.call("textDocument/completion", map[string]any{
		"textDocument":map[string]any{"uri":fileURI(file.path)},
		"position":lspPosition{Line:pos.Line, Character:pos.Character},
	}, func(result json.RawMessage, err error) {
		if err != nil || l.app.lsp != l || l.app.files[file.path] != file || file.document.Version() != version { return }
		var entries []lspCompletion
		if len(result) > 0 && result[0] == '[' {
			_ = json.Unmarshal(result, &entries)
		} else {
			var list struct { Items []lspCompletion `json:"items"` }
			_ = json.Unmarshal(result, &list)
			entries = list.Items
		}
		items := make([]ui.MenuItem, 0, min(len(entries), 80))
		for i, item := range entries {
			if i == 80 { break }
			item := item
			items = append(items, ui.MenuItem{ID:fmt.Sprint(i), Label:item.Label,
				Action:func(){ applyCompletion(file, item, version) }})
		}
		file.view.ShowCompletions(items)
	})
}

func applyCompletion(file *openDocument, item lspCompletion, version uint64) bool {
	if file.document.Version() != version { return false }
	start, end := file.view.Caret, file.view.Caret
	insert := item.InsertText
	if insert == "" { insert = item.Label }
	if item.TextEdit != nil {
		var err error
		editRange:=item.TextEdit.Range
		if editRange==nil { editRange=item.TextEdit.Insert }
		if editRange!=nil {
			start, err = file.document.ByteOffset(editor.Position{Line:editRange.Start.Line, Character:editRange.Start.Character})
			if err != nil { return false }
			end, err = file.document.ByteOffset(editor.Position{Line:editRange.End.Line, Character:editRange.End.Character})
			if err != nil { return false }
		}
		insert = item.TextEdit.NewText
	}
	type replacement struct { start, end int; text string; main bool }
	edits := []replacement{{start,end,insert,true}}
	for _, extra := range item.AdditionalTextEdits {
		if extra.Range==nil { return false }
		a, err := file.document.ByteOffset(editor.Position{Line:extra.Range.Start.Line, Character:extra.Range.Start.Character})
		if err != nil { return false }
		b, err := file.document.ByteOffset(editor.Position{Line:extra.Range.End.Line, Character:extra.Range.End.Character})
		if err != nil { return false }
		edits = append(edits, replacement{a,b,extra.NewText,false})
	}
	sort.SliceStable(edits, func(i,j int) bool { return edits[i].start > edits[j].start })
	caret := start+len(insert)
	for i, edit := range edits {
		if edit.start > edit.end || (i > 0 && edit.end > edits[i-1].start) { return false }
		if !edit.main && edit.start <= start { caret += len(edit.text)-(edit.end-edit.start) }
	}
	for _, edit := range edits {
		if !file.view.ReplaceTextRange(edit.start,edit.end,edit.text) { return false }
	}
	file.view.SetSelection(caret,caret)
	return true
}

// logWriter safely forwards process stderr without touching the UI worker.
type uiLogWriter struct { app *App; prefix string }
func (w uiLogWriter) Write(p []byte) (int,error) {
	text := string(p)
	w.app.manager.Post(func() {
		if !w.app.closed { w.app.output.AppendText(w.prefix+": "+strings.TrimRight(text,"\r\n")+"\n") }
	})
	return len(p),nil
}
func (a *App) logWriter(prefix string) io.Writer { return uiLogWriter{app:a,prefix:prefix} }
