package goide

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
	"unicode/utf16"

	ui "github.com/banditmoscow1337/nativeForms"
	"github.com/banditmoscow1337/nativeForms/editor"
)

func utf16Position(text string, offset int) lspPosition {
	pos := lspPosition{}
	offset = min(max(0, offset), len(text))
	for _, r := range text[:offset] {
		if r == '\n' {
			pos.Line++
			pos.Character = 0
		} else {
			pos.Character += utf16.RuneLen(r)
		}
	}
	return pos
}

// incrementalChanges converts pre-transaction byte edits into descending UTF-16
// LSP range changes so earlier edits remain valid as they are applied in order.
func incrementalChanges(previous string, edits []editor.Edit) []any {
	sorted := append([]editor.Edit(nil), edits...)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].Start > sorted[j].Start })
	for first := 0; first < len(sorted); {
		last := first + 1
		for last < len(sorted) && sorted[last].Start == sorted[first].Start {
			last++
		}
		for left, right := first, last-1; left < right; left, right = left+1, right-1 {
			sorted[left], sorted[right] = sorted[right], sorted[left]
		}
		first = last
	}
	changes := make([]any, 0, len(sorted))
	for _, edit := range sorted {
		start, end := utf16Position(previous, edit.Start), utf16Position(previous, edit.End)
		changes = append(changes, map[string]any{
			"range": lspRange{Start: start, End: end},
			"text":  edit.Text,
		})
	}
	return changes
}

func completionIcon(icons *Icons, kind int) *ui.ImageResource {
	if icons == nil {
		return nil
	}
	switch kind {
	case 2, 3: // Method, Function
		return icons.SymFunc
	case 5, 6, 10: // Field, Variable, Property
		return icons.SymVar
	case 7, 8, 22: // Class, Interface, Struct
		return icons.SymType
	case 9: // Module / Package
		return icons.SymPkg
	case 14, 21: // Keyword, Constant
		return icons.SymConst
	default:
		return icons.GoFile
	}
}

func (l *lspClient) completion(file *openDocument) {
	version := file.document.Version()
	view := l.app.activeEditorView(file)
	pos, err := file.document.UTF16Position(view.Caret)
	if err != nil {
		return
	}
	l.call("textDocument/completion", map[string]any{
		"textDocument": map[string]any{"uri": fileURI(file.path)},
		"position":     lspPosition{Line: pos.Line, Character: pos.Character},
	}, func(result json.RawMessage, err error) {
		if err != nil || l.app.lsp != l || l.app.files[file.path] != file || file.document.Version() != version {
			return
		}
		var entries []lspCompletion
		if len(result) > 0 && result[0] == '[' {
			_ = json.Unmarshal(result, &entries)
		} else {
			var list struct {
				Items []lspCompletion `json:"items"`
			}
			_ = json.Unmarshal(result, &list)
			entries = list.Items
		}
		icons := DefaultIcons()
		items := make([]ui.MenuItem, 0, min(len(entries), 80))
		for i, item := range entries {
			if i == 80 {
				break
			}
			item := item
			label := item.Label
			if item.Detail != "" && len(item.Detail) <= 42 {
				label = item.Label + "  — " + item.Detail
			}
			items = append(items, ui.MenuItem{
				ID:    fmt.Sprint(i),
				Label: label,
				Icon:  completionIcon(icons, item.Kind),
				Action: func() {
					applyCompletion(file, item, version)
				},
			})
		}
		l.app.activeEditorView(file).ShowCompletions(items)
	})
}

func (l *lspClient) definition(file *openDocument, byteOffset int, fallback func()) {
	pos, err := file.document.UTF16Position(byteOffset)
	if err != nil {
		if fallback != nil {
			fallback()
		}
		return
	}
	l.call("textDocument/definition", map[string]any{
		"textDocument": map[string]any{"uri": fileURI(file.path)},
		"position":     lspPosition{Line: pos.Line, Character: pos.Character},
	}, func(result json.RawMessage, err error) {
		if err != nil || len(result) == 0 || string(result) == "null" || l.app.lsp != l {
			if fallback != nil {
				fallback()
			}
			return
		}
		var targetURI string
		var targetPos lspPosition
		if result[0] == '[' {
			var locs []lspLocation
			if json.Unmarshal(result, &locs) == nil && len(locs) > 0 {
				targetURI = locs[0].URI
				targetPos = locs[0].Range.Start
			} else {
				var links []lspLocationLink
				if json.Unmarshal(result, &links) == nil && len(links) > 0 {
					targetURI = links[0].TargetURI
					targetPos = links[0].TargetSelectionRange.Start
				}
			}
		} else {
			var loc lspLocation
			if json.Unmarshal(result, &loc) == nil && loc.URI != "" {
				targetURI = loc.URI
				targetPos = loc.Range.Start
			}
		}
		path := uriPath(targetURI)
		if path == "" || !within(l.app.project.root, path) {
			if fallback != nil {
				fallback()
			}
			return
		}
		l.app.jumpToLocation(path, targetPos.Line, targetPos.Character)
		l.app.setStatus(fmt.Sprintf("Определение: %s:%d:%d", fileName(path), targetPos.Line+1, targetPos.Character+1))
	})
}

func (l *lspClient) hover(file *openDocument) {
	view := l.app.activeEditorView(file)
	pos, err := file.document.UTF16Position(view.Caret)
	if err != nil {
		return
	}
	l.call("textDocument/hover", map[string]any{
		"textDocument": map[string]any{"uri": fileURI(file.path)},
		"position":     lspPosition{Line: pos.Line, Character: pos.Character},
	}, func(result json.RawMessage, err error) {
		if err != nil || len(result) == 0 || string(result) == "null" || l.app.lsp != l {
			return
		}
		var body struct {
			Contents json.RawMessage `json:"contents"`
		}
		if json.Unmarshal(result, &body) != nil || len(body.Contents) == 0 {
			return
		}
		var text string
		if body.Contents[0] == '"' {
			_ = json.Unmarshal(body.Contents, &text)
		} else if body.Contents[0] == '{' {
			var mc struct {
				Value string `json:"value"`
			}
			if json.Unmarshal(body.Contents, &mc) == nil {
				text = mc.Value
			}
		}
		text = strings.TrimSpace(text)
		if text == "" {
			return
		}
		caret := l.app.activeEditorView(file).CaretRect()
		l.app.manager.ShowTooltip(text, ui.Rect{X: caret.X, Y: caret.Y + caret.H + 4})
		l.app.appendLogf("output", "[Hover %s:%d] %s\n", fileName(file.path), pos.Line+1, strings.ReplaceAll(text, "\n", " | "))
	})
}

func (l *lspClient) semanticTokens(file *openDocument) {
	if file == nil || len(l.tokenLegend) == 0 {
		return
	}
	version := file.document.Version()
	l.call("textDocument/semanticTokens/full", map[string]any{
		"textDocument": map[string]any{"uri": fileURI(file.path)},
	}, func(result json.RawMessage, err error) {
		if err != nil || l.app.lsp != l || l.app.files[file.path] != file || file.document.Version() != version {
			return
		}
		var body struct {
			Data []uint32 `json:"data"`
		}
		if json.Unmarshal(result, &body) == nil && len(body.Data) > 0 {
			for _, v := range file.allViews() {
				_ = v.SetSemanticTokens(version, l.tokenLegend, body.Data)
			}
		}
	})
}

func applyCompletion(file *openDocument, item lspCompletion, version uint64) bool {
	if file.document.Version() != version {
		return false
	}
	view := file.view
	start, end := view.Caret, view.Caret
	insert := item.InsertText
	if insert == "" {
		insert = item.Label
	}
	if item.TextEdit != nil {
		var err error
		editRange := item.TextEdit.Range
		if editRange == nil {
			editRange = item.TextEdit.Insert
		}
		if editRange != nil {
			start, err = file.document.ByteOffset(editor.Position{Line: editRange.Start.Line, Character: editRange.Start.Character})
			if err != nil {
				return false
			}
			end, err = file.document.ByteOffset(editor.Position{Line: editRange.End.Line, Character: editRange.End.Character})
			if err != nil {
				return false
			}
		}
		insert = item.TextEdit.NewText
	}
	type replacement struct {
		start, end int
		text       string
		main       bool
	}
	edits := []replacement{{start, end, insert, true}}
	for _, extra := range item.AdditionalTextEdits {
		if extra.Range == nil {
			return false
		}
		a, err := file.document.ByteOffset(editor.Position{Line: extra.Range.Start.Line, Character: extra.Range.Start.Character})
		if err != nil {
			return false
		}
		b, err := file.document.ByteOffset(editor.Position{Line: extra.Range.End.Line, Character: extra.Range.End.Character})
		if err != nil {
			return false
		}
		edits = append(edits, replacement{a, b, extra.NewText, false})
	}
	sort.SliceStable(edits, func(i, j int) bool { return edits[i].start > edits[j].start })
	caret := start + len(insert)
	for i, edit := range edits {
		if edit.start > edit.end || (i > 0 && edit.end > edits[i-1].start) {
			return false
		}
		if !edit.main && edit.start <= start {
			caret += len(edit.text) - (edit.end - edit.start)
		}
	}
	for _, edit := range edits {
		if !view.ReplaceTextRange(edit.start, edit.end, edit.text) {
			return false
		}
	}
	view.SetSelection(caret, caret)
	return true
}

// uiLogWriter safely forwards process stderr to OutputPanel on the UI goroutine.
type uiLogWriter struct {
	app    *App
	prefix string
}

func (w uiLogWriter) Write(p []byte) (int, error) {
	text := string(p)
	w.app.manager.Post(func() {
		if !w.app.closed {
			line := w.prefix + ": " + strings.TrimRight(text, "\r\n") + "\n"
			w.app.output.AppendText(line)
			if w.prefix == "dlv" && w.app.debugOutput != nil {
				w.app.debugOutput.AppendText(line)
			}
		}
	})
	return len(p), nil
}

func (a *App) logWriter(prefix string) io.Writer {
	return uiLogWriter{app: a, prefix: prefix}
}
