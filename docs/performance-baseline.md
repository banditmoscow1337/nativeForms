# Performance baseline (v1, stage 0)

The benchmark suite and optional frame samples are measurement tools, not an
assertion that the current renderer meets the v1 budgets. Record actual numbers
on both target machines before declaring this stage complete.
An initial headless sandbox run is recorded in
`docs/performance-snapshot-2026-10-13.md`; it is not a target-device baseline.

## Repeatable run

Use a release build with the same commit, Go version, power profile, window
size, theme, and font configuration. Record OS version, CPU, GPU/driver,
display scale, resolution and whether the app is visible. Close unrelated
heavy processes. Warm each benchmark once. Save raw output from at least five
runs; compare median and p95 across runs, not a single best number.

```sh
go version
go test ./... -count=1
go test ./... -run '^$' -bench 'Benchmark(FrameBuild|FrameResize|Render)' -benchmem -benchtime=1x -count=5
go test . -run '^$' -bench 'Benchmark(TextInputSnapshot|FocusedTextInputState100k|CodeEditorColdJump100k|CodeEditorWarmJump100k|FrameBuildLongParagraph)' -benchmem -benchtime=1x -count=5
go test ./... -run '^$' -bench 'Benchmark(FrameBuildWidgets|RenderCommands)' -benchmem -benchtime=100x -count=5
go test . -run '^$' -bench 'BenchmarkOffscreenPaintVisits' -benchmem -benchtime=100x -count=5
go test . -run '^$' -bench 'Benchmark(TextAreaScroll100k|VirtualFormScroll10k|FocusedTextInputState100k|CodeEditorColdJump100k)' -benchmem -benchtime=20x -count=5
go test . -run '^$' -bench 'Benchmark(TextAreaInitial100k|CodeEditorInitial100k)' -benchmem -benchtime=1x -count=5
```

The 100k-line scenarios are intentionally isolated with `-benchtime=1x`.
`TextInputSnapshot` measures the surrounding-text window alone; the focused
`TextInputState` includes caret geometry and full-document wrapping. The
CodeEditor cold jump rebuilds lexical state up to line 90k for each attempt,
while the warm jump revisits nearby lines. The 1k-fragment long paragraph is
separate from a 1k-line TextArea, since wrapping work differs sharply.
`BenchmarkOffscreenPaintVisits` reports `visits/op` and `visible/op` for a
clipped 1k-row viewport; its no-op Paint deliberately isolates traversal.
The stage-1 scroll cases warm the text index and the virtual row-height index
before timing. Compare them to the cold construction benchmarks separately.
`BenchmarkFrameBuildWidgets` covers 1k/10k widgets, both paint-only and forced
layout; `BenchmarkFrameBuildScrolledForm` has 1k fields; text area has
1k/10k/100k lines; CodeEditor has 100k lines; image and 2x HiDPI include
resizing. Software raster benchmarks distinguish 1k/10k commands, 10k glyph
masks and a bilinear 2x image. Setup is excluded from each benchmark's timed
loop. The headless suite exercises `BeginFrame` and `software.RenderAtScale`,
but not OS upload or display submission.

For an integrated window run, use the included driver. Run the same commit on
each target machine and retain one file per scenario:

```sh
go run ./cmd/perftrace -scenario form -count 1000 -duration 60s -output form.jsonl -os-version "..." -cpu "..." -gpu "..." -power "..." -display "resolution, scale, font, theme"
go run ./cmd/perftrace -scenario virtual-form -count 10000 -duration 60s -output virtual-form.jsonl
go run ./cmd/perftrace -scenario textarea -count 100000 -duration 60s -output textarea.jsonl
go run ./cmd/perftrace -scenario editor -count 100000 -pattern cold -duration 60s -output editor-cold.jsonl
go run ./cmd/perftrace -scenario editor -count 100000 -pattern warm -duration 60s -output editor-warm.jsonl
```

Repeat for `widgets` (`-count 1000` and `10000`), `form` and `paragraph`.
The driver records a JSONL manifest, frame samples, action durations (which
include IME state queries), and a summary with dropped and skipped counts. Actions
scroll, type, resize and briefly hide/show the window on the UI thread.
Use `-backend x11` or `-backend wayland` when probing a particular Linux
session. Run `-allocations` separately: allocation sampling distorts latency.
Do not interpret no frames during a hidden interval as missing data. The
headless sandbox cannot run native integrated traces on the two target hosts.
Summarize recorded phases and action latency with
`python3 tools/summarize_perftrace.py form.jsonl editor-cold.jsonl`.

Alternatively, install an observer before `Run`:

```go
manager.SetFrameObserver(func(s nativeforms.FrameSample) {
    // Append s as JSONL to a buffered writer on the UI thread, or copy it
    // to a bounded channel and serialize asynchronously. Never block here.
})
```

Samples have `Build`, `Layout`, `Paint`, `TextLayout`, `Raster`, `Upload`,
`Present`, command/mask/image/rune/glyph counts, text layout cache hits/misses,
the conservative changed-tile bounding area (`DamagePixels`), backend and
fallback reason. Windows still submits the full DIB on `WM_PAINT`, so
`DamagePixels` is not a Windows present byte count.
`Build` contains `Layout` and `Paint`; `TextLayout` is the font-layout portion
of `Paint` only. Text measurement inside widget `Measure` is counted in
`Layout`, not `TextLayout`. `Present` measures submission, not scan-out:
`StretchDIBits` on Windows, `setNeedsDisplay` on macOS, X11 request writes,
and Wayland attach/damage/commit writes. It is not a GPU time measurement.
`AllocBytes` and `AllocObjects` are opt-in with `FrameObserverOptions{TrackAllocations: true}`;
`runtime.ReadMemStats` is intrusive, so use separate runs for allocations and
latency. A headless `BeginFrame` must call `FrameTrace().Finish("headless", "")`
after rendering to deliver a sample. In `Graphics: "auto"`, desktop reports
`render: backend unavailable` while no standalone hardware renderer is
installed; explicit `Graphics: "software"` has an empty fallback reason.

Capture at least 60 seconds of interactive scrolling, typing, resize and
repeated hide/show, with cold and warm text-cache runs distinguished. Retain
raw JSONL and environment metadata for Windows/Radeon RX 480 and macOS/M1.
The Go benchmarks alone do not satisfy the integrated-frame exit criterion.

## Provisional v1 budgets (to validate on both target machines)

| Scenario | Proposed p95 frame budget | Other target |
| --- | ---: | --- |
| 1k widgets, scrolled form, active typing | 16.7 ms | No sustained UI-thread stalls > 50 ms |
| CodeEditor 100k lines, visible-region repaint | 16.7 ms | Work proportional to visible lines, not document length |
| 10k widgets, full relayout or HiDPI resize | 33.3 ms | No unbounded allocation growth |
| 100k-line TextArea, scrolling/editing | 33.3 ms | Flag as known limitation if not achievable before v1 |

These are acceptance targets, **not measurements**. Revisit after real
baselines: if 100k-line TextArea cannot meet its target, document a supported
size limit or make virtualized text a release blocker. Track p50, p95, max,
allocations/frame, counts, and any fallback reason. Do not compare only FPS:
the pipeline is event driven and idle windows may legitimately render zero
frames.

The [2026-10-15 headless snapshot](performance-snapshot-2026-10-15.md)
isolates the stage-1 changes on Linux. It does not replace native p95 runs on
the Radeon RX 480 and Apple M1 target machines.

## Render parity

`internal/renderparity.Scenes()` is the shared, OS-independent golden corpus;
`software/TestRenderParityGoldenScenes` checks exact premultiplied RGBA
pixels. It covers all five command kinds plus clips, rounded corners,
compositing order, nearest/cropped images, bilinear filtering and alpha.
The existing tests additionally check scaled scratch reuse and fractional
pixels. Future GPU readback tests should use the same scenes, document sRGB
versus linear blending, and allow at most two 8-bit levels per channel where
GPU coverage rounding differs; no missing command, clipping, or order changes.
Supplement this tiny deterministic corpus with OS screenshot goldens at
1x/2x once the backend runs on target hardware.

