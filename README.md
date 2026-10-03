# nativeForms

nativeForms is a Go UI toolkit with a component tree, layout, input dispatch,
themes, and basic widgets. The core now records backend-independent paint
commands. An offscreen software renderer and an optional Vulkan backend consume
the same frame. Win32, AppKit, local X11 and Wayland adapters present software frames.

The [desktop v1 scope and architecture](docs/desktop-v1.md) records the target.
The [migration notes](docs/migration-notes.md) describe the API changes and
current limitations. [Stages 12–13](docs/stages-12-13.md) describe popup
widgets, virtual models, desktop examples and platform feature availability.

For an offscreen example that writes `frame.png`, see
[`examples/offscreen`](examples/offscreen/main.go). The caller creates a
`nativeforms.Manager`, calls `BeginFrame`, then passes `Frame()` and an
`image.RGBA` to `software.Renderer.Render`.

For the Windows example, run `go run ./examples/windows` on Windows amd64 or
arm64. The [example](examples/windows/main_windows.go) creates a manager and
passes it to `windows.New(manager, windows.Options{...}).Run()`. `Run` owns the
UI thread; use `manager.Post` for updates from other goroutines. Set an
application font with `nativeforms.SetFontData(ttfBytes)` before the first
measurement or frame. Otherwise the toolkit loads a local system TrueType font
and falls back to its embedded ASCII bitmap glyphs if none is available.

The shared editor demo runs with `go run ./examples/editor_windows`,
`go run ./examples/editor_darwin`, or `go run ./examples/editor_linux` on
their respective platforms. Linux selects Wayland when `WAYLAND_DISPLAY` is
set and X11 otherwise. The Wayland adapter uses xdg-shell and `wl_shm` over a
Unix socket. Its keyboard path reads ordinary symbol records from the
compositor's XKB keymap, including common Cyrillic names. It falls back to a
US layout when it cannot read those records. X11 and Wayland exchange UTF-8
clipboard text through their display protocols; compose and IME are not yet
implemented there.

On Windows and macOS, `OpenFile()` and `SaveFile(defaultName)` show native
file panels from the UI goroutine. An empty path means the user cancelled.
On Windows and Wayland, `Options.OnFilesDropped` receives paths dropped from
the shell or a `text/uri-list` offer.
Windows and Linux can run separate windows with separate managers; AppKit
currently supports one active `Run` window per process.

## Dependency policy

The intended first-party implementation uses the Go standard library and
`purego` for platform bindings. `miniVK` is the author's own Vulkan integration.
The software package does not import Vulkan or `miniVK`. Font outlines,
metrics, and glyph masks use first-party TrueType code; `golang.org/x/image`
has been removed.

The Windows adapter handles resize, DPI, pointer, keyboard, Unicode `WM_CHAR`,
IME composition through IMM32, clipboard shortcuts, cursor, and idle wake-ups.
`NewTextArea`, `NewForm`, `NewGrid`, `NewSplitPane`, and two-axis `ScrollPanel`
cover longer forms; [stages 9–11](docs/stages-9-11.md) document their contracts.
The first-party font parser supports quadratic TrueType outlines and
Unicode cmap format 4/12; it does not support CFF, variable fonts, hinting,
or GSUB/GPOS shaping.
