# Desktop v1: scope and architecture

Status: core/backend separation, offscreen CPU painting, first-party TrueType
rendering, the Windows software window, editing, form layout, and event-driven
painting are implemented on the migration branches. Other v1 features below
remain future work.

## Goal

Make the existing nativeForms component model usable in both an embedded game
UI and a standalone desktop application. The first standalone application has
one Windows window, a CPU renderer, and a usable set of form controls. The game
continues to use the Vulkan renderer.

The first end-to-end milestone is the same example component tree displayed
through the existing Vulkan integration and through an offscreen software
renderer. The next milestone presents the software frame in a Windows window.

## Dependency rule

- The Go standard library and `github.com/ebitengine/purego` are the only
  third-party Go code permitted in the portable core, software renderer, and
  platform bindings. Standard-library `image` packages are permitted.
- `miniVK` is an author-owned integration and belongs only to the optional
  Vulkan backend. A software-only consumer must not import or initialize it.
  The package boundary now preserves that property for software imports. The
  Go module still lists the author-owned `miniVK` dependency for its Vulkan
  package; separating modules is an optional later packaging decision.
- SDL, GLFW, Cairo, FreeType, HarfBuzz, `golang.org/x/image`, and other external
  UI, drawing, font, or window packages are outside this design.
- Calls into OS-provided APIs and frameworks are permitted through first-party
  bindings. On Linux, direct X11/Wayland and keyboard protocol work is a separate
  platform milestone if external native client libraries are also disallowed.
- The module retains the author's `miniVK`; `golang.org/x/image` is removed.

## Desktop v1 scope

The first standalone target is Windows on a supported desktop architecture,
with one resizable window and a software framebuffer. It should support pointer,
wheel, keyboard, text entry, focus, clipboard, cursor changes, DPI changes,
resize, and a clean close. UI state can be updated from background goroutines
through a queue delivered to the UI owner.

The usable UI surface includes labels, buttons, toggles, sliders, progress
bars, single-line and multiline text entry, scrollable containers, menus,
popups/dialogs, tabs, and a virtualized list or table. Form and grid layout
should obey min/max constraints. CPU rendering must handle clipping, text,
images, alpha blending, and the primitives used by those widgets.

Desktop v1 is complete when a standalone example built with the software
path can edit and paste text, resize and rescale correctly, display a long
scrolling list, and sit idle without a continuous render loop. The same
component tree must still work in the game through the Vulkan path.

## Deferred scope

macOS and Linux window backends, multi-window support, full language coverage,
all OpenType outline formats and features, platform accessibility adapters,
system file dialogs, and broad drag-and-drop support are later milestones.
APIs should leave room for these features without promising them in desktop v1.

The initial text implementation reads quadratic TrueType outlines in Go,
using installed system fonts or caller-supplied bytes. OS text shaping and
cross-platform fallback remain separate work. Text capabilities should be
documented according to the backend that actually implements them.

## Ownership boundaries

| Layer | Owns | Does not own |
| --- | --- | --- |
| UI core | Component tree, measure/arrange, focus, event routing, widget state, invalidation | Native window handles, GPU handles, OS event loop |
| Text | Font selection, shaping/measurement, glyph positions, caret mapping, glyph masks | Window creation, widget state, frame presentation |
| Paint | Backend-independent ordered commands, clipping, images and glyph references | GPU buffers, native windows, focus |
| Software renderer | Paint commands to a caller-owned or renderer-owned pixel buffer | Window events or widget tree |
| Vulkan renderer | Paint commands to Vulkan resources using the author's miniVK integration | Layout or platform input translation |
| Platform adapter | Window lifetime, event translation, DPI, clipboard, cursor, presentation, wake-up | Component layout or drawing semantics |

Package names and exact public interfaces can change during implementation.
The ownership rules above should remain stable. The Vulkan-specific `Renderer`
interface now lives in the `vulkan` package rather than in the UI core.

## Data flow and contracts

1. A host or platform adapter supplies logical window size, current scale,
   timestamp, and normalized input events.
2. The UI owner processes pending mutations, input, and scheduled work. A
   mutation requests layout, paint, or both.
3. The core measures and arranges the tree when needed and records ordered
   drawing commands. Text measurement and drawing share one shaping result.
4. One selected renderer consumes the commands. The software renderer writes
   pixels; the Vulkan renderer submits GPU work.
5. The platform adapter presents the result when a frame is due.

Logical UI coordinates are device-independent units. Platform adapters convert
pointer coordinates and framebuffer size at the boundary. Renderers use the
physical target scale; DPI changes invalidate relevant layout and font caches.
The paint contract defines clip intersection, ordered alpha composition, color
space/pixel format, and resource lifetime before either renderer is finalized.

Input routing belongs to the core. The platform adapter converts native key,
text, pointer, focus, and IME events. A text-editing component consumes ordinary
typing and selection keys before optional game-navigation shortcuts. Pointer
capture and focused targets are cleared when their components are removed,
hidden, disabled, or their window loses focus.

One UI owner serializes tree changes and callbacks. `Manager.Post(func())` is
the cross-goroutine entry point. Native window work runs on the OS-required thread. Renderer
resources have an explicit creation/close lifetime. Library initialization
returns errors to callers instead of terminating the process.

Desktop presentation is demand-driven: an event, invalidation, caret blink,
notification, or animation schedules the next frame. The embedded game host
may still request a frame from its own loop. Full-frame redraw is acceptable
initially; dirty-region rendering is an optimization after profiling.

## Migration constraints

- Keep the existing `Component` shape and the useful layout/widget behavior
  where possible. The `Paint(*Canvas)` call may remain while Canvas records
  commands internally.
- Existing consumers may need an explicit migration from `New(renderer)`,
  `BeginFrame`, and `Render`. Avoid pretending these signatures are already
  backend-neutral; document any breaking change and offer a small conversion
  example.
- Vulkan initialization and destruction live in the Vulkan backend. Shader
  file paths must be supplied by the game host until embedded resources exist.
  Creating a UI tree does not initialize a graphics device.
- The first-party font path measures and draws Latin and Cyrillic when the
  installed or caller-supplied TrueType font includes those glyphs. The
  embedded emergency bitmap fallback is ASCII only.
- Overflow of a drawing buffer must grow, batch, or return a visible error;
  dropping widgets or text silently is not permitted.
- Each backend renders the same command stream, so behavior and clipping can
  be compared using an identical example tree.

## Delivery order and review points

1. Separate Vulkan ownership from the core while preserving an in-game
   example. Review: creating and measuring a tree needs no renderer.
2. Record commands and move the old Vulkan path behind a command consumer.
   Review: the existing widgets still display through Vulkan.
3. Add an offscreen CPU renderer. Review: deterministic example frames,
   alpha/clipping behavior, and no Vulkan import in the software path.
4. Add the Windows adapter. Review: independent window, resizing, input,
   scale changes, clipboard, idle behavior, and clean shutdown.
5. Improve text editing, layout, scrolling, and compound widgets until the
   desktop-v1 example is usable. Review: keyboard and pointer workflows on a
   long form and a virtualized data view.
6. Remove the remaining font dependency with a verified replacement. Review:
   supported Latin and Cyrillic text has consistent measurement and painting.

Later platform adapters and the broader text/accessibility milestones should
follow their own implementation plans. Builds and tests for stages 7-8 are
left to the project owner as requested.
