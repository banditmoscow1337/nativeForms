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

## Editor overlays and commands

`ShowCompletions` opens a menu beside the caret. Pass `MenuItem` values whose
`Action` applies the selected edit; the editor keeps keyboard and native IME
focus. The menu consumes Up/Down/Home/End and Enter. Escape dismisses it,
typing continues in the editor, and starting IME composition closes it.
Calling `ShowCompletions` again replaces the list; `HideCompletions` closes it.
Long lists show eight rows and scroll the active row into view.

```go
view.ShowCompletions([]nativeforms.MenuItem{
    {ID: "println", Label: "Println", Action: func() {
        view.ReplaceTextRange(start, end, "Println")
    }},
})
```

For signature help, hover text, and quick fixes, use
`manager.OpenUnfocusedPopup(content, nativeforms.PopupOptions{...})`.
`Owner` is the focused editor, `Anchor` returns its current caret rectangle,
`Passive` lets pointer events pass through, and `OnKeyDown` can handle keys
before the editor. Popup size comes from `Bounds.W/H` or content measurement.
An anchored popup follows its owner during layout, flips above the caret near
the bottom of the viewport, and closes when focus leaves its owner.

```go
manager.OpenUnfocusedPopup(signaturePanel, nativeforms.PopupOptions{
    Owner: view,
    Anchor: view.CaretRect,
    Passive: true,
})
```

Use `Shortcut.Scope` to register an editor or panel command. With focus in a
component, keys are handled by that component, its scoped shortcuts, each
ancestor and its scoped shortcuts, then application shortcuts with no scope.
The existing `AllowInModal` flag still applies.
