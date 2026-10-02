# Migration notes: core, Vulkan, and offscreen software

This branch implements stages 2-11 of the migration plan: detach Vulkan from
the UI core, correct selected lifecycle/layout issues, establish a serialized
UI mutation path, record drawing commands, and rasterize those commands into
an offscreen image, replace the font dependency, and present a software frame
in one Windows window, and add text editing, form layout, scrolling and
event-driven repaint. See [stages 9–11](stages-9-11.md) for the new APIs.

## API changes

The old `nativeforms.New(renderer)` initialized Vulkan. Construct the UI
without a renderer, then create the optional Vulkan backend separately:

```go
manager := nativeforms.New()
manager.SetRoot(root)
manager.SetInteractive(true)
manager.SetConsumeUnhandledInput(true) // game overlay policy
manager.SetGameNavigation(true)        // arrow/WASD menu navigation
manager.BeginFrame(width, height, deltaSeconds)

backend, err := vulkan.New(gameRenderer, vulkan.ShaderPaths{
    Vertex: "shaders/hud.vert.spv",
    Fragment: "shaders/hud.frag.spv",
})
if err != nil { return err }
defer backend.Destroy() // after in-flight GPU work has completed

if err := backend.Render(commandBuffer, manager.Frame()); err != nil {
    return err
}
```

The game host supplies its shader paths. Backend creation and rendering return
errors. The Vulkan backend reports a frame over its current 64K-vertex capacity
instead of silently dropping geometry. The host can choose a smaller UI or
handle the error; growing/splitting the GPU buffer remains future work.

Offscreen software painting uses the same frame:

```go
manager := nativeforms.New()
manager.SetRoot(root)
manager.BeginFrame(640, 360, 0)
dst := image.NewRGBA(image.Rect(0, 0, 640, 360))
err := software.New().Render(manager.Frame(), dst)
```

`Render` clears the destination and requires a zero-origin RGBA buffer whose
size matches the frame. The result can be encoded with `image/png` or passed
to the Windows adapter. `RenderScaled` maps logical commands to a physical
pixel buffer. `Frame().Commands` is borrowed storage; render
or copy it before the next `BeginFrame`, and do not mutate it.

## UI ownership and invalidation

Tree changes, callbacks, `HandleEvent`, and `BeginFrame` run on one UI
owner goroutine. For work originating on another goroutine, call
`manager.Post(func() { ... })`. The host may install a thread-safe wake
signal via `SetWakeHandler`. `ProcessPending` is invoked automatically
before events and frames; the wake handler itself must not touch the tree.

State setters and the new text/value setters invalidate layout or paint.
Direct writes to existing exported fields remain possible for compatibility;
after such writes, call `InvalidateLayout` if measurements/positions change,
otherwise `InvalidatePaint`. `NeedsFrame` tells a host when it should paint.
The host can use `NextFrameAfter` to schedule notification expiration and
caret blinking, and pass elapsed seconds to `BeginFrame`.

Removing, hiding, or disabling a component clears focus, hover, and pointer
capture in its subtree. Removing focus emits `FocusLost`. Flex distribution
now accounts for each child's min/max on the main axis. `ScrollPanel` has
its own measurement and lets an unconsumed wheel event reach its parent.

`HandleEvent` now reports whether a handler consumed the event. An embedded
game that formerly relied on consuming all input while the UI was active
should call `SetConsumeUnhandledInput(true)`.
Arrow/WASD focus navigation is now opt-in with `SetGameNavigation(true)`;
Tab and Shift+Tab remain available by default.

## Fonts and Windows

`SetFontData(ttfBytes)` installs a TrueType font before the first text
measurement or frame. Without it, nativeForms tries an OS-installed font and
then an embedded ASCII fallback. The shared 2048² atlas is generated in Go;
no font asset is redistributed. Unicode cmap 4/12 and quadratic glyph outlines
cover basic Latin, Greek, and Cyrillic in a suitable font. Complex shaping,
variable outlines, CFF, advanced composite placement, and glyphs outside the
fixed atlas repertoire require further work. Rasterization does not apply
hinting, and the renderer samples the atlas with nearest pixels.

The Windows package is available on Windows amd64/arm64 and expects Windows 10
with GetDpiForWindow. Run the [example](../examples/windows/main_windows.go)
with `go run ./examples/windows`. `Window.Run` locks the calling OS thread,
creates one resizable Win32 window and handles events, DPI, repaint, and
shutdown. `Window.Close` may signal it from another goroutine. The adapter
converts logical input coordinates and scales paint commands to physical
pixels. `SetClipboardText` and `ClipboardText` are available on the UI thread;
focused text widgets use them for Ctrl+C/X/V. Win32 IMM32 sends composition
updates and committed text to the editor and positions the candidate window
near the caret. The window repaints on changes, resize, and scheduled
notification/caret timers; an expose event can reuse its existing DIB.

## Deliberate limits

- The CPU renderer supports the current Canvas primitives: solid and
  atlas-backed rectangles, text, borders, and lines. `RenderScaled` maps
  logical positions to a physical pixel buffer.
- The CPU renderer uses nearest atlas sampling and limited stroke
  antialiasing. Renderer visual parity requires further visual review.
- The grapheme implementation covers common UAX #29 rules but lacks complete
  Unicode property tables for rare Indic and prepend cases. Complex shaping
  and other OS window adapters are future work.
- Public fields in `Panel` and widgets do not automatically invalidate.
- The Vulkan renderer's host must synchronize resource destruction with
  GPU work, as it did before this separation.

No builds or tests were run for this change at the user's request.
