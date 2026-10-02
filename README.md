# nativeForms

nativeForms is a Go UI toolkit that currently runs inside a Vulkan-based game.
It has a component tree, layout, input dispatch, themes, and basic widgets.
Its current `Manager` and renderer are coupled to Vulkan; a standalone desktop
application and software renderer are design goals, not implemented features.

The [desktop v1 scope and architecture](docs/desktop-v1.md) records the intended
migration. The first target is a single-window Windows application while keeping
the existing in-game Vulkan use case. macOS and Linux are later platform targets.

## Dependency policy

The intended first-party implementation uses the Go standard library and
`purego` for platform bindings. `miniVK` is the author's own Vulkan integration.
The software path must not depend on Vulkan or `miniVK`. The current repository
still imports `golang.org/x/image` for font rasterization; replacing that
dependency is future work described in the design document.

The design document describes proposed behavior and package boundaries. It
does not claim that the current API already provides those capabilities.
