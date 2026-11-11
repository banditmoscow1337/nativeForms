package goide

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/banditmoscow1337/nativeForms/editor"
)

type lspPosition struct {
	Line      int `json:"line"`
	Character int `json:"character"`
}

type lspRange struct {
	Start lspPosition `json:"start"`
	End   lspPosition `json:"end"`
}

type lspLocation struct {
	URI   string   `json:"uri"`
	Range lspRange `json:"range"`
}

type lspLocationLink struct {
	TargetURI            string   `json:"targetUri"`
	TargetSelectionRange lspRange `json:"targetSelectionRange"`
	TargetRange          lspRange `json:"targetRange"`
}

type lspDiagnostic struct {
	Range    lspRange `json:"range"`
	Message  string   `json:"message"`
	Severity int      `json:"severity"`
	Source   string   `json:"source"`
}

type lspTextEdit struct {
	Range   *lspRange `json:"range,omitempty"`
	Insert  *lspRange `json:"insert,omitempty"`
	Replace *lspRange `json:"replace,omitempty"`
	NewText string    `json:"newText"`
}

type lspCompletion struct {
	Label               string        `json:"label"`
	Kind                int           `json:"kind"`
	Detail              string        `json:"detail"`
	InsertText          string        `json:"insertText"`
	TextEdit            *lspTextEdit  `json:"textEdit"`
	AdditionalTextEdits []lspTextEdit `json:"additionalTextEdits"`
}

type lspClient struct {
	app         *App
	cmd         *exec.Cmd
	conn        *protocolConn
	next        int
	pending     map[int]func(json.RawMessage, error)
	ready       bool
	deferred    []any
	syncKind    int
	tokenLegend []string
}

func startLSP(app *App) (*lspClient, error) {
	goplsBin := app.goplsBinary()
	cmd := exec.Command(goplsBin, "serve")
	cmd.Dir = app.project.root
	cmd.Env = app.toolEnv()
	in, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	cmd.Stderr = app.logWriter("gopls")
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	l := &lspClient{
		app:      app,
		cmd:      cmd,
		pending:  make(map[int]func(json.RawMessage, error)),
		syncKind: 2,
		tokenLegend: []string{
			"namespace", "type", "class", "enum", "interface",
			"struct", "typeParameter", "parameter", "variable", "property",
			"enumMember", "event", "function", "method", "macro",
			"keyword", "modifier", "comment", "string", "number",
			"regexp", "operator",
		},
	}
	l.conn = newProtocolConn(out, in, app.manager.Post, l.receive, func(err error) {
		if app.lsp == l && !app.closed {
			app.setStatus("gopls: " + err.Error())
			app.updateLSPBadge("gopls: off", false)
		}
	})
	go func() { _ = cmd.Wait() }()

	l.call("initialize", map[string]any{
		"processId": os.Getpid(),
		"rootUri":   fileURI(app.project.root),
		"workspaceFolders": []any{
			map[string]any{"uri": fileURI(app.project.root), "name": app.project.name()},
		},
		"initializationOptions": map[string]any{
			"semanticTokens": true,
		},
		"capabilities": map[string]any{
			"workspace": map[string]any{
				"workspaceFolders": true,
				"configuration":    true,
			},
			"textDocument": map[string]any{
				"completion": map[string]any{
					"completionItem": map[string]any{"snippetSupport": false},
				},
				"hover": map[string]any{
					"contentFormat": []string{"plaintext", "markdown"},
				},
				"definition": map[string]any{"linkSupport": true},
				"semanticTokens": map[string]any{
					"requests":   map[string]any{"full": true},
					"tokenTypes": l.tokenLegend,
					"tokenModifiers": []string{
						"declaration", "definition", "readonly", "static",
						"deprecated", "abstract", "async", "modification",
					},
					"formats": []string{"relative"},
				},
			},
		},
	}, func(result json.RawMessage, err error) {
		if app.lsp != l {
			return
		}
		if err != nil {
			app.setStatus("gopls initialize: " + err.Error())
			app.updateLSPBadge("gopls: err", false)
			return
		}
		var response struct {
			Capabilities struct {
				TextDocumentSync       json.RawMessage `json:"textDocumentSync"`
				SemanticTokensProvider struct {
					Legend struct {
						TokenTypes []string `json:"tokenTypes"`
					} `json:"legend"`
				} `json:"semanticTokensProvider"`
			} `json:"capabilities"`
		}
		if json.Unmarshal(result, &response) == nil {
			if len(response.Capabilities.TextDocumentSync) != 0 {
				if response.Capabilities.TextDocumentSync[0] == '{' {
					var sync struct {
						Change int `json:"change"`
					}
					if json.Unmarshal(response.Capabilities.TextDocumentSync, &sync) == nil {
						l.syncKind = sync.Change
					}
				} else {
					_ = json.Unmarshal(response.Capabilities.TextDocumentSync, &l.syncKind)
				}
			}
			if len(response.Capabilities.SemanticTokensProvider.Legend.TokenTypes) > 0 {
				l.tokenLegend = response.Capabilities.SemanticTokensProvider.Legend.TokenTypes
			}
		}
		l.ready = true
		_ = l.conn.send(map[string]any{"jsonrpc": "2.0", "method": "initialized", "params": map[string]any{}})
		for _, message := range l.deferred {
			_ = l.conn.send(message)
		}
		l.deferred = nil
		app.setStatus("gopls готов")
		app.updateLSPBadge("gopls: ready", true)
		if active := app.active(); active != nil {
			l.semanticTokens(active)
		}
	})
	return l, nil
}

func (l *lspClient) Close() {
	if l == nil {
		return
	}
	if l.conn != nil {
		l.conn.Close()
	}
	if l.cmd != nil && l.cmd.Process != nil {
		_ = l.cmd.Process.Kill()
	}
}

func (l *lspClient) call(method string, params any, callback func(json.RawMessage, error)) {
	if l == nil || l.conn == nil {
		if callback != nil {
			callback(nil, io.ErrClosedPipe)
		}
		return
	}
	l.next++
	id := l.next
	if callback != nil {
		l.pending[id] = callback
	}
	if err := l.conn.send(map[string]any{"jsonrpc": "2.0", "id": id, "method": method, "params": params}); err != nil {
		delete(l.pending, id)
		if callback != nil {
			callback(nil, err)
		}
	}
}

func (l *lspClient) notify(method string, params any) {
	if l == nil || l.conn == nil {
		return
	}
	message := map[string]any{"jsonrpc": "2.0", "method": method, "params": params}
	if !l.ready {
		l.deferred = append(l.deferred, message)
		return
	}
	if err := l.conn.send(message); err != nil {
		l.app.setStatus("gopls: " + err.Error())
	}
}

func (l *lspClient) receive(data json.RawMessage) {
	if l.app == nil || l.app.lsp != l {
		return
	}
	var envelope struct {
		ID     json.RawMessage `json:"id"`
		Method string          `json:"method"`
		Params json.RawMessage `json:"params"`
		Result json.RawMessage `json:"result"`
		Error  *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if json.Unmarshal(data, &envelope) != nil {
		return
	}
	if envelope.Method != "" {
		if len(envelope.ID) > 0 {
			var result any
			switch envelope.Method {
			case "workspace/configuration":
				var params struct {
					Items []any `json:"items"`
				}
				_ = json.Unmarshal(envelope.Params, &params)
				items := make([]any, len(params.Items))
				for i := range items {
					items[i] = map[string]any{"semanticTokens": true}
				}
				result = items
			case "workspace/applyEdit":
				result = map[string]any{"applied": false}
			default:
				result = nil
			}
			if l.conn != nil {
				_ = l.conn.send(map[string]any{"jsonrpc": "2.0", "id": envelope.ID, "result": result})
			}
			return
		}
		if envelope.Method == "textDocument/publishDiagnostics" {
			var params struct {
				URI         string          `json:"uri"`
				Version     *int            `json:"version"`
				Diagnostics []lspDiagnostic `json:"diagnostics"`
			}
			if json.Unmarshal(envelope.Params, &params) == nil {
				l.app.setDiagnostics(params.URI, params.Version, params.Diagnostics)
			}
		}
		return
	}
	var id int
	if json.Unmarshal(envelope.ID, &id) != nil {
		return
	}
	callback := l.pending[id]
	delete(l.pending, id)
	if callback != nil {
		if envelope.Error != nil {
			callback(nil, fmt.Errorf("%s", envelope.Error.Message))
		} else {
			callback(envelope.Result, nil)
		}
	}
}

func (l *lspClient) open(file *openDocument) {
	file.lspVersion = 1
	file.lastText = file.document.Text()
	language := "go"
	if fileName(file.path) == "go.mod" {
		language = "go.mod"
	}
	l.notify("textDocument/didOpen", map[string]any{
		"textDocument": map[string]any{
			"uri":        fileURI(file.path),
			"languageId": language,
			"version":    file.lspVersion,
			"text":       file.lastText,
		},
	})
	if l.ready && language == "go" {
		l.semanticTokens(file)
	}
}

func (l *lspClient) change(file *openDocument, change editor.Change) {
	previous := file.lastText
	file.lastText = file.document.Text()
	file.lspVersion++
	if l.syncKind == 0 {
		return
	}
	var changes []any
	if l.syncKind == 1 {
		changes = []any{map[string]any{"text": file.lastText}}
	} else {
		changes = incrementalChanges(previous, change.Edits)
	}
	l.notify("textDocument/didChange", map[string]any{
		"textDocument":   map[string]any{"uri": fileURI(file.path), "version": file.lspVersion},
		"contentChanges": changes,
	})
	if l.ready && strings.HasSuffix(file.path, ".go") {
		l.semanticTokens(file)
	}
}

func (l *lspClient) didSave(file *openDocument) {
	l.notify("textDocument/didSave", map[string]any{
		"textDocument": map[string]any{"uri": fileURI(file.path)},
		"text":         file.document.Text(),
	})
	if l.ready && strings.HasSuffix(file.path, ".go") {
		l.semanticTokens(file)
	}
}

func (l *lspClient) closeFile(path string) {
	l.notify("textDocument/didClose", map[string]any{
		"textDocument": map[string]any{"uri": fileURI(path)},
	})
}
