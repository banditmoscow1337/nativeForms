# ADR 0001: Renderer contract and backend choice

Status: renderer contract implemented (2026-10-15); GPU pixel parity pending.

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
`hardware`; hardware selection currently reports `ErrUnavailable` because no
standalone desktop GPU backend is installed. `software.RenderAtScale` remains
the correctness oracle and desktop fallback. The engine Vulkan integration
is a separate consumer of the same display list and batches. The desktop GPU
selection should be a later spike on Windows/RX 480 and macOS/M1; decide
between native D3D/Metal and a maintained portable layer after measuring
dependency, packaging and fallback costs. Do not make the v1 public widget
API depend on that choice.

Proposed adapter boundary: initialize backend after the native window and
surface exist; resize/recreate swapchain on scale change; render the same
`Frame`; release device resources on window shutdown. Choose GPU if surface
initialization succeeds, otherwise software; on device loss, release the GPU
path and retry software for that frame. Publish backend and exact fallback
reason in `FrameSample`. Explicit user selection should be available for
debugging and parity tests. The trace already separates CPU build, raster,
upload and submission; GPU timestamp queries may add a distinct GPU metric.

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
headless batch/descriptor/draw submission. Actual Vulkan pixel readback on a
GPU is still required to establish raster parity, especially fractional
coverage and cropped bilinear sampling. Before adopting a desktop GPU adapter,
require the shared parity corpus, 1x/2x
screenshot comparisons, lifecycle tests (resize, minimize, device loss), and
the two-machine performance baseline in `docs/performance-baseline.md`. Avoid
uploading all glyph masks and images every frame: use a bounded atlas/cache
with generation IDs and deterministic eviction. Software can always rebuild
from the immutable CPU resources.

