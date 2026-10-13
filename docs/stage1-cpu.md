# Stage 1: CPU path, usage and limits

## Text

`TextField` keeps rune, paragraph and wrapped-line indexes across paint,
scroll, hit-testing and measure. On edit it rescans the flat text once to
identify paragraphs, rewraps changed paragraphs and shifts positions of
unchanged suffix lines. Paint shapes only rows intersecting the viewport.
The shared bounded `FontContext` layout LRU accelerates repeated line shapes;
the public `Layout` returns independent slices. `SetLayoutCacheLimit` controls
its budget separately from glyph masks. Adding a face invalidates the cache.

The current `TextEditor.Text` remains a flat string. Large edits still copy
text and rescan paragraph boundaries, so this stage guarantees work
independent of total document size for steady scroll/repaint, not constant
time for every edit. A piece-table-backed generic TextField is a later API
choice; CodeEditor already uses `editor.Document`.

## Forms

Painting skips leaves and clipped subtrees outside the active Canvas clip.
For a large form with variable row heights, use `VirtualStack` instead of
materializing thousands of `Panel` children:

```go
type Rows struct { IDs []string }
func (m *Rows) Len() int { return len(m.IDs) }
func (m *Rows) ID(i int) string { return m.IDs[i] }
func (m *Rows) Height(i int, width float32) float32 {
    if i%2 == 0 { return 28 }
    return 40
}
func (m *Rows) Build(i int) nativeforms.Component {
    return nativeforms.NewTextField(m.IDs[i], nil)
}

stack := nativeforms.NewVirtualStack(rows)
manager.SetRoot(stack)
```

Call `stack.ModelChanged()` after a change in row count, order, content or
heights. For component content changes, implement the optional
`VirtualStackUpdater.Update(index, component)` method. `ScrollBy`, `ScrollTo`
and `VisibleRange` work with variable heights; the first visible stable ID
anchors scroll position across model insertions. Offscreen components are
detached, so their transient UI state should live in the model. Unique,
nonempty IDs are required; rows with invalid IDs cannot be recycled.

## Software renderer

`Frame.Damage` is the logical union of invalidated component regions.
Layout, resize and explicit `Manager.InvalidatePaint()` mark the full frame.
`RenderAtScaleDamage` and `RenderBGRAAtScaleDamage` preserve pixels outside
the pixel damage rectangle, so callers must retain the previous full target;
`platform.PixelDamage` rounds out logical bounds with a 1-pixel fringe.
The original `Render`/`RenderAtScale` always render the full frame.

Windows renders directly into DIB memory; Wayland renders directly into the
chosen shared-memory buffer and tracks dirtiness per buffer; X11 renders BGRA
before copying rows into protocol requests; macOS renders RGBA into the
bitmap representation. X11 exposure and freshly allocated targets force a
full render. Windows and Wayland still submit full presentation regions for
safe exposure/compositor behavior. These are CPU raster and staging changes,
not the standalone GPU backend from later roadmap stages.

## Measurement

Re-run the stage-0 benchmarks with matching flags and target hardware.
The October 13 sandbox snapshots in `performance-snapshot-2026-10-13.md`
and `performance-snapshot-stage1-2026-10-13.md` compare five warmed
single-iteration measurements on the same sandbox CPU. The 100k-line
TextArea median drops from ~708 ms / ~72 MB to ~0.287 ms / ~18 KB per
steady paint; the scrolled form drops from ~5.15 ms to ~0.081 ms. These
are not p95 acceptance results or target-device measurements. The new
`BenchmarkVirtualStackScroll10k` and `BenchmarkRenderImageHiDPIDamage`
exercise virtual form scrolling and 2x partial image raster respectively.
