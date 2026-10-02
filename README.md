# nativeForms

nativeForms is a Go UI toolkit with a component tree, layout, input dispatch,
themes, and basic widgets. The core now records backend-independent paint
commands. An offscreen software renderer and an optional Vulkan backend consume
the same frame. A Win32 adapter presents software frames in one resizable window.

The [desktop v1 scope and architecture](docs/desktop-v1.md) records the target.
The [migration notes](docs/migration-notes.md) describe the API changes and
current limitations. The first window target is Windows; macOS and Linux are
later platform targets.

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

## Dependency policy

The intended first-party implementation uses the Go standard library and
`purego` for platform bindings. `miniVK` is the author's own Vulkan integration.
The software package does not import Vulkan or `miniVK`. Font outlines,
metrics, and glyph masks use first-party TrueType code; `golang.org/x/image`
has been removed.

The Windows adapter handles resize, DPI, pointer, keyboard, Unicode `WM_CHAR`,
clipboard methods, cursor, and idle wake-ups. Clipboard shortcuts in widgets,
IME composition, richer font shaping, and other desktop controls remain future
work. The first-party font parser supports quadratic TrueType outlines and
Unicode cmap format 4/12; it does not support CFF, variable fonts, hinting,
or GSUB/GPOS shaping.
