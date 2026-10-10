# nativeForms

The current desktop target is Windows, Linux and macOS on amd64 and arm64.
See the [desktop baseline and owner-run verification](docs/desktop-baseline.md)
for the six-target matrix, required Go version and manual checks. The owner
reported that all six CI builds passed for the baseline PR; native runtime
behavior still needs verification on the target systems.

nativeForms is a Go UI toolkit with a component tree, layout, input dispatch,
themes, and basic widgets. The core now records backend-independent paint
commands. An offscreen software renderer and an optional Vulkan backend consume
the same frame. Win32, AppKit, local X11 and Wayland adapters present software frames.

The [desktop v1 scope and architecture](docs/desktop-v1.md) records the target.
The [migration notes](docs/migration-notes.md) describe the API changes and
current limitations. [Stages 12–13](docs/stages-12-13.md) describe popup
widgets, virtual models, desktop examples and platform feature availability.
[Stage 14.1](docs/stage-14.md) starts Russian/English text improvements and
the platform-independent accessibility tree; native screen-reader bridges
remain future work.

For an offscreen example that writes `frame.png`, see
[`examples/offscreen`](examples/offscreen/main.go). The caller creates a
`nativeforms.Manager`, calls `BeginFrame`, then passes `Frame()` and an
`image.RGBA` to `software.Renderer.Render`.
The [image and software paint contract](docs/paint-software.md) covers image
resource lifetime, clipping, alpha, filtering, scaling and current GPU limits.

For the Windows example, run `go run ./examples/windows` on Windows amd64 or
arm64. The [example](examples/windows/main_windows.go) creates a manager and
passes it to `windows.New(manager, windows.Options{...}).Run()`. `Run` owns the
UI thread; use `manager.Post` for updates from other goroutines. Set an
application font with `nativeforms.SetFontData(ttfBytes)` before the first
measurement or frame. Otherwise the toolkit loads a local system TrueType font
and falls back to its embedded ASCII bitmap glyphs if none is available.
`SetFallbackFontData(ttfBytes, ...)` configures an ordered portable fallback
chain. Supply a font with Cyrillic coverage for predictable Russian text.

The shared editor demo runs with `go run ./examples/editor_windows`,
`go run ./examples/editor_darwin`, or `go run ./examples/editor_linux` on their
respective platforms. It exposes native Open/Save pickers and displays dropped
file paths. Linux selects native Wayland when `WAYLAND_DISPLAY` is set and X11
otherwise. Both implement clipboard and compose; X11 supports INCR and Xdnd,
and Wayland consumes resolved XKB types, groups, modifiers and repeat settings.

Linux IME uses IBus on X11 or text-input-v3 on Wayland. File pickers use the
session's desktop portal over a first-party D-Bus connection. Query the window's
`Capabilities()` from `OnReady`: service-dependent features can be unavailable.
No GTK/Qt/Xlib/xkbcommon bindings or helper executables are required.

AppKit supports several windows through `darwin.Run(w1, w2)` and `Window.Open()`
from an existing UI callback. Lock the main goroutine with
`func init() { runtime.LockOSThread() }`. The adapter provides NSTextInputClient,
file drops, native pickers and Core Text shaping/system-font fallback. See
`go run ./examples/multiwindow_darwin` and the [stage 13 contracts and manual
checks](docs/stages-12-13.md). Windows and Linux use separate managers and UI
loops for independent windows.

## Dependency policy

The intended first-party implementation uses the Go standard library and
`purego` for platform bindings. `miniVK` is the author's own Vulkan integration.
The software package does not import Vulkan or `miniVK`. Font outlines,
metrics, and glyph masks in the portable path use first-party TrueType code; `golang.org/x/image`
has been removed.

The Windows adapter handles resize, DPI, pointer, keyboard, Unicode `WM_CHAR`,
IME composition through IMM32, clipboard shortcuts, cursor, and idle wake-ups.
`NewTextArea`, `NewForm`, `NewGrid`, `NewSplitPane`, and two-axis `ScrollPanel`
cover longer forms; [stages 9–11](docs/stages-9-11.md) document their contracts.
The first-party font parser supports quadratic TrueType outlines and
Unicode cmap format 4/12; it does not support CFF, variable fonts, hinting,
or GSUB/GPOS shaping.
The portable path supports horizontal `kern` format 0, shared glyph/caret
positions, and common Russian/English wrapping rules. See
[`examples/text_ru_en`](examples/text_ru_en/main.go) for a PNG and semantic
snapshot specimen you can run locally.
