# IDE panels (stage 5)

The UI building blocks now cover project navigation and the usual IDE side and bottom panes. An application supplies the filesystem, diagnostics, stack, and variable models; the widgets do not launch `gopls` or `dlv`.

```go
files := nativeforms.NewVirtualTree(fileModel)
diagnostics := nativeforms.NewVirtualTable(diagnosticModel)
stack := nativeforms.NewVirtualTable(stackModel)
variables := nativeforms.NewVirtualTree(variableModel)

editorTabs := nativeforms.NewTabs(nil, "", func(id string) {
    // Activate the document identified by id.
})
editorTabs.OnCloseRequest = func(id string) bool {
    // Prompt to save a dirty document when needed.
    return true
}
editorTabs.Open(nativeforms.Tab{
    ID: "main.go", Title: "main.go", Content: codeEditor,
})

debugTabs := nativeforms.NewTabs([]nativeforms.Tab{
    {ID: "problems", Title: "Problems", Content: diagnostics},
    {ID: "stack", Title: "Call stack", Content: stack},
    {ID: "variables", Title: "Variables", Content: variables},
}, "problems", nil)

output := nativeforms.NewOutputPanel()
bottom := nativeforms.NewTabs([]nativeforms.Tab{
    {ID: "debug", Title: "Debug", Content: debugTabs},
    {ID: "output", Title: "Output", Content: output},
}, "output", nil)

workArea := nativeforms.NewSplitPane(nativeforms.Vertical, editorTabs, bottom)
root := nativeforms.NewSplitPane(nativeforms.Horizontal, files, workArea)
_ = root
```

When the backing model changes, call `files.ModelChanged()`, `diagnostics.ModelChanged()`, `stack.ModelChanged()`, or `variables.ModelChanged()`. The tree keeps expanded IDs and selection and clamps its scroll position. The list and table also clamp after shrinking.

`Tabs.Open`, `Close`, `Move`, `Select`, `SetDirty`, and `ScrollTo` manage documents. `OnCloseRequest` can veto a close. The header has a close target, supports drag reordering, and scrolls horizontally when tabs exceed its width. `OnReorder` reports the new index. Keep document state keyed by the stable tab ID.

`OutputPanel.AppendText` accepts streaming chunks, including partial lines and CRLF across chunks. It follows the bottom while the reader is at the bottom and keeps the reader's position when scrolled back. `SetMaxLines` bounds retained output (default 5,000); `Clear` empties it. As with other widget changes, call these methods on the UI thread.

Desktop windows implement `platform.FileDialogs.OpenFolder() (string, error)` for selecting a project directory. An empty path with a nil error means the user cancelled. Linux uses the desktop portal directory option and checks that the returned local path is a directory.

A terminal emulator, filesystem watchers, `gopls`/Delve adapters, and a complete IDE application belong to later stages.
