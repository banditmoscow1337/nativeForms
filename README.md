# nativeForms

nativeForms is a Go UI toolkit with a component tree, layout, input dispatch,
themes, and basic widgets. The core now records backend-independent paint
commands. An offscreen software renderer and an optional Vulkan backend consume
the same frame. A standalone desktop window is not implemented yet.

The [desktop v1 scope and architecture](docs/desktop-v1.md) records the target.
The [migration notes](docs/migration-notes.md) describe the API changes and
current limitations. The first window target is Windows; macOS and Linux are
later platform targets.

For an offscreen example that writes `frame.png`, see
[`examples/offscreen`](examples/offscreen/main.go). The caller creates a
`nativeforms.Manager`, calls `BeginFrame`, then passes `Frame()` and an
`image.RGBA` to `software.Renderer.Render`.

## Dependency policy

The intended first-party implementation uses the Go standard library and
`purego` for platform bindings. `miniVK` is the author's own Vulkan integration.
The software package does not import Vulkan or `miniVK`. The current repository
still imports `golang.org/x/image` for font rasterization; replacing that
dependency is future work described in the design document.

The design document includes future capabilities; it does not claim that
windowing, advanced text input, or cross-platform support is already present.
