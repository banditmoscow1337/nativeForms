# Go IDE example (stage 6)

This example composes nativeForms widgets into a small desktop Go IDE. It keeps
project files, LSP/DAP clients, and subprocesses in the example package. The
framework itself remains independent of `gopls` and Delve.

Install `gopls` and `dlv` on your `PATH`, then run the entry point for your
desktop:

```sh
go run ./examples/goide_linux /absolute/path/to/module
go run ./examples/goide_darwin /absolute/path/to/module
go run ./examples/goide_windows C:\path\to\module
```

Run only the command matching your operating system. You may omit the module
argument and use **Open folder**. The directory must contain `go.mod`.
Folders in the left tree expand from the arrow; clicking a Go file opens it.
Open at least two files to exercise tabs. Ctrl+S (Cmd+S on macOS) saves the
active file. A dirty file cannot be closed until saved.

After `gopls` initializes, diagnostics appear in **Problems**; select one to
jump to its location. **Complete** or Ctrl+Space in the editor asks `gopls`
for completions and applies the selected text edit, including import edits.
Each document is synchronized using UTF-16 positions, with changes sent in
the order required for multi-edit transactions. The example requests plain
completion text rather than snippets.

Click the gutter or press F9 to set a breakpoint, save, then choose **Debug**
for a main package or **Test** for package tests. These launch a local `dlv
dap` process. Once it stops, **Call stack** lists frames and **Variables**
lists values from their scopes. **Continue**, **Next**, **Step in**, **Step out**,
and **Stop** control the local session. Build/debugger output appears in
**Output**. The example starts Delve through a loopback connection initiated
by its `--client-addr` option; it does not expose a remote debug listener.

The module's source files are saved before launching Delve. The example keeps
breakpoints per file while the module is open. Nested variable expansion,
remote attach, terminal emulation, project file watching, and a save conflict
dialog are outside this demonstration.

## Checks to run

```sh
go test ./...                          # core, example, and platform tests
go test -run 'TestCodeEditor100kRowsRemainVirtual|TestCodeEditorCyrillicEmojiIMEAndScaledHighlight' .
go test -run '^$' -bench 'BenchmarkCodeEditor100k' -benchmem .
```

The benchmarks report edit/undo time, scroll/paint time, allocations, and
retained bytes per undo transaction. Compare the numbers across releases on
the same machine; they intentionally do not impose a machine-specific timing
limit. The UI path also needs a manual run with an installed `gopls` and
`dlv`: open two files, make a diagnostic, request completion, place a
breakpoint, and inspect a stopped frame and its variables.
