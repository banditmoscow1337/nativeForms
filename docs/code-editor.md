# CodeEditor

`CodeEditor` is a source widget backed by `editor.Document`. It renders the
visible rows, keeps its caret and selection as UTF-8 byte offsets, and uses the
framework's `TextInputClient` path for native IME input.

```go
doc, err := editor.NewDocument("package main\n")
if err != nil {
    return err
}
view := nativeforms.NewCodeEditor(doc)
view.OnChange = func(change editor.Change) {
    // Send the edits and new document version to the language server.
}
view.OnBreakpointChange = func(line int, enabled bool) {
    // The line is zero based; update the debug session's breakpoints.
}
manager := nativeforms.New()
manager.SetRoot(view)
```

Go lexical colors are available immediately. For custom spans, call
`SetStyleRanges(doc.Version(), ranges)` with absolute byte offsets. To apply
gopls semantic tokens, call `SetSemanticTokens(version, legend, data)` using
the LSP `semanticTokens` delta array and the matching legend. The method
converts UTF-16 positions to document byte offsets and rejects stale versions.
Apply language-server responses on the UI goroutine. A document edit clears
the old semantic and custom ranges while retaining local Go highlighting.

`SetBreakpoint`, `BreakpointLines`, and F9 operate on zero-based line numbers;
the gutter displays numbers starting at one. The editor supports selection,
copy/paste, undo/redo, tab indent/outdent, page movement, mouse selection and
scrolling. Its text is not wrapped; `TabWidth` controls visual tab stops.
