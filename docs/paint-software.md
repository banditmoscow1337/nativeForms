# Image and software paint contract

The supported desktop path for images and rounded rectangles is
`Manager.BeginFrame` → `software.Renderer.Render` (or `RenderScaled`) →
the Windows, macOS, X11 or Wayland software presenter. Vulkan currently reports
an explicit unsupported-command error for these new operations. Existing
atlas quads, lines and text masks remain supported there.

## Resources and ownership

- `NewImageResource(image.Image)` copies the pixels into a zero-origin
  premultiplied RGBA snapshot; image bounds must be nonempty and at most
  16,384 pixels per side and 64 megapixels in total. The source can be
  discarded immediately. `ID()` is unique, and the first `Version()` is 1.
- `resource.Replace(image)` creates a second immutable snapshot with the
  same ID and incremented version. Call `widget.SetImage(replacement)` to
  invalidate layout. A command referencing the old version keeps its pixels
  alive until that command/frame is discarded. Go's GC releases unreferenced
  snapshots; there is no explicit destroy call.
- `Manager.Frame().Commands` is borrowed only until the next `BeginFrame`.
  Consume it synchronously. Copy the command slice if it must outlive the
  frame; the immutable image resources can be shared across copied commands.
  Shaped text masks have their own immutability contract.

## Pixels and coordinates

- `Color` and image tint use straight RGB components in the range 0–1.
  `ImageResource` stores premultiplied RGBA. The software target is a
  zero-origin `image.RGBA`, also premultiplied. Source-over composition runs
  in sRGB component space without color-profile conversion or linear-light
  blending. `Render` clears the target to transparent before every frame.
- `Canvas.Image(resource, dest, source, tint, filter)` uses logical coordinates
  for `dest` and pixel-edge coordinates for `source`. A zero source selects
  the whole resource. Source pixels outside the image are clipped without
  stretching the visible remainder. The canvas clip applies to the destination.
  Nearest or bilinear filtering samples premultiplied channels, avoiding
  colored fringes around transparent pixels. Tint multiplies RGB and alpha;
  its alpha can be used as opacity.
- `NewImage(resource)` measures to the pixel size unless preferred size or
  constraints override it. `ImageContain` preserves the full source with
  letterboxing, `ImageCover` crops centrally, and `ImageStretch` fills the
  bounds. The widget clips its own drawing.
- `Canvas.RoundedRect` clips corners at a radius up to half the shorter side.
  Software rasterization uses four samples per axis at boundary pixels.
  Clip rectangles, images, rounded rectangles, text and popups follow command
  order.
- `RenderScaled` maps logical coordinates to the physical target dimensions,
  scales a stroke or corner radius by the smaller axis scale, and reuses its
  command scratch on subsequent calls. `Render` writes into the caller's
  framebuffer; desktop adapters reuse that framebuffer until the window size
  changes. The Windows/X11/Wayland presenters copy premultiplied RGBA into
  their native BGRA buffers. Draw an opaque root background when the window
  must have a predictable opaque result; transparent offscreen pixels are
  otherwise composed by the host window system.

## Review

`go test . ./software` includes small exact-pixel goldens for source-over,
clipping, image source crop, bilinear alpha, fractional coverage, glyph masks,
rounded corners and scaled frames. The synthetic pixels avoid dependence on
installed system fonts. `go run ./examples/offscreen` writes `frame.png`
using `examples/paintdemo`; the same component is part of the shared native
editor example. Inspect it at native scale and after resize/DPI changes on
Windows, macOS, X11 and Wayland. Owner-run commands and the target matrix
are in [desktop-baseline.md](desktop-baseline.md).
