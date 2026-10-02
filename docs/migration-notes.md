# Migration notes: core, Vulkan, and offscreen software

This branch implements stages 2-6 of the migration plan: detach Vulkan from
the UI core, correct selected lifecycle/layout issues, establish a serialized
UI mutation path, record drawing commands, and rasterize those commands into
an offscreen image. It does not create or present an OS window.

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
to a future OS window adapter. `Frame().Commands` is borrowed storage; render
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
pass elapsed seconds to `BeginFrame`. Caret blinking needs a later scheduler.

Removing, hiding, or disabling a component clears focus, hover, and pointer
capture in its subtree. Removing focus emits `FocusLost`. Flex distribution
now accounts for each child's min/max on the main axis. `ScrollPanel` has
its own measurement and lets an unconsumed wheel event reach its parent.

`HandleEvent` now reports whether a handler consumed the event. An embedded
game that formerly relied on consuming all input while the UI was active
should call `SetConsumeUnhandledInput(true)`.
Arrow/WASD focus navigation is now opt-in with `SetGameNavigation(true)`;
Tab and Shift+Tab remain available by default.

## Deliberate limits

- The CPU renderer supports the current Canvas primitives: solid and
  atlas-backed rectangles, text, borders, and lines. It draws at 1:1 logical
  to physical pixels; DPI conversion belongs to a future platform adapter.
- The CPU renderer uses nearest atlas sampling and limited stroke
  antialiasing. Renderer visual parity requires further visual review.
- The current font atlas still uses `golang.org/x/image`. The dependency
  policy is not fully met until the font replacement stage.
- There is no platform event loop, clipboard, IME bridge, or native window.
- Public fields in `Panel` and widgets do not automatically invalidate.
- The Vulkan renderer's host must synchronize resource destruction with
  GPU work, as it did before this separation.

No builds or tests were run for this change at the user's request.
