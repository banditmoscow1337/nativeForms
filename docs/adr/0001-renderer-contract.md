# ADR 0001: Renderer contract and backend choice

Status: renderer contract and macOS Metal adapter implemented (2026-10-15);
target-machine GPU pixel parity and performance pending.

The widget tree produces a `Frame` of ordered `DrawCommand`s in logical
coordinates. Keep `Manager` and `Canvas` backend-independent. A renderer
receives an immutable frame, pixel size and display scale; it reports an error
instead of silently omitting unsupported commands. The command set is quad,
line, alpha mask, image and rounded rectangle. Clip, alpha, image source crop,
filter and ordering must match the software reference. A renderer may batch
adjacent compatible commands, but never reorder across blending/clip changes.

`render.Freeze(frame, previous)` copies commands and glyph masks and returns a
stable display list with a conservative pixel damage rectangle. ImageResource
already owns immutable pixels; ID/version changes invalidate the old and new
command bounds. A backend may keep a frozen list until its submission fence.
`Manager.Frame()` is the legacy synchronous view and is still reused on the
next `BeginFrame`; asynchronous consumers must freeze it first. Lists expose
only detached frames to callers.

`render.Factory` selects a renderer and returns the reason for an automatic
software fallback. `desktop.Options.Graphics` accepts `auto`, `software`, or
`hardware`. macOS chooses a standalone Metal backend after creating the native
view; platforms without a standalone adapter report `ErrUnavailable` for
explicit hardware selection. `auto` falls back during creation and after a
Metal frame fails, and retries that frame on software. `software.RenderAtScale`
remains the correctness oracle and desktop fallback. The engine Vulkan
integration is a separate consumer of the same display list and batches.
Windows/RX 480 still needs a standalone adapter. The public widget API is
independent of the choice.

The Metal adapter owns a CAMetalLayer, native shader pipelines, a three-slot
vertex buffer and dynamic glyph atlas, and a bounded immutable image cache.
It waits for each slot before reusing its buffer or uploading new masks to that
slot's atlas, avoiding whole-device waits. Adjacent draws with the same texture
and filter share a batch; clipping is baked into triangles. Resizing updates
the drawable and the slot's multisample attachment. Window shutdown releases
native resources. `FrameSample` records CPU build/upload/submission, slot wait,
the last completed GPU command duration, batches, uploads and exact fallback
reason. Explicit software/hardware selection supports paired measurements.

The engine Vulkan adapter accepts images, creates textures by ID/version and
groups adjacent commands by texture and sampler without changing paint order.
Image uploads use premultiplied RGBA with a separate blend pipeline and
clamp-to-edge nearest/linear samplers. A bounded texture cache holds CPU
resources, queues uploads, retires evicted handles after a completion serial,
and reuploads after `RecoverDevice`. An engine may report completed
`Backend.SubmittedSerial()` values via `CompletedUISerial()`. Engines without it
wait for device idle only on eviction. The font atlas is currently pinned; the
software-shaped glyph masks remain geometry, not dynamic GPU atlas entries.

The golden scenes exercise software pixel output and the Vulkan adapter's
headless batch/descriptor/draw submission. Actual GPU readback and paired
1x/2x screenshots are still required for visual parity, especially fractional
coverage and cropped bilinear sampling. The repeatable macOS script in
`docs/performance-baseline.md` covers resize, hide/show, repeated workloads
and injected Metal initialization/render failure. Hardware speed and visual
parity remain unverified until the target M1 run is reviewed. Windows/RX 480
also remains on the software desktop path for now.

