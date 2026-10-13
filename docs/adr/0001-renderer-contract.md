# ADR 0001: Renderer contract and backend choice

Status: proposed for v1; stage 0 records the contract, not a GPU backend.

The widget tree produces a `Frame` of ordered `DrawCommand`s in logical
coordinates. Keep `Manager` and `Canvas` backend-independent. A renderer
receives an immutable frame, pixel size and display scale; it reports an error
instead of silently omitting unsupported commands. The command set is quad,
line, alpha mask, image and rounded rectangle. Clip, alpha, image source crop,
filter and ordering must match the software reference. A renderer may batch
adjacent compatible commands, but never reorder across blending/clip changes.

`software.RenderAtScale` remains the correctness oracle and supported
fallback. The existing engine Vulkan integration is a separate consumer of
the frame; it is not automatically a desktop surface backend. The desktop
GPU selection should be a later spike on Windows/RX 480 and macOS/M1; decide
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

Before adopting a GPU adapter, require the shared parity corpus, 1x/2x
screenshot comparisons, lifecycle tests (resize, minimize, device loss), and
the two-machine performance baseline in `docs/performance-baseline.md`. Avoid
uploading all glyph masks and images every frame: use a bounded atlas/cache
with generation IDs and deterministic eviction. Software can always rebuild
from the immutable CPU resources.
