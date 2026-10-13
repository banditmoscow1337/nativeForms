# Headless snapshot: 2026-10-13

This is an initial **sandbox CPU measurement**, not the Windows/RX 480 or
macOS/M1 acceptance baseline, and not a window FPS result. Linux/amd64,
AMD EPYC 9V74, Go 1.27.0, `CGO_ENABLED=0`, `-p 1`, 1x display scale unless
named HiDPI. Source revision: stage-0 PR #30 based on `136080e6`.

Command (five single-iteration samples per case):

```sh
go test -p 1 -run '^$' -bench 'Benchmark(FrameBuild|FrameResize|Render)' -benchmem -benchtime=1x -count=5 . ./software
```

Times below are medians of five `ns/op` values, with min–max in parentheses.
They are **not p95 frame times**. The benchmark fixtures are built outside
the timed loop; allocation figures are per measured iteration.

| Case | Median ms (min–max) | B/op | allocs/op |
| --- | ---: | ---: | ---: |
| 1k widgets, paint | 4.45 (4.33–4.91) | 88,352 | 3,002 |
| 1k widgets, relayout | 8.82 (8.27–9.89) | 301,472 | 11,004 |
| 10k widgets, paint | 44.32 (43.23–51.58) | 880,352 | 30,002 |
| 10k widgets, relayout | 99.04 (91.93–102.74) | 3,011,552 | 110,004 |
| 1k-field scrolled form | 5.15 (4.94–5.60) | 280,352 | 9,002 |
| TextArea, 1k lines | 6.84 (6.59–6.99) | 676,064 | 4,234 |
| TextArea, 10k lines | 65.90 (64.09–70.75) | 6,894,048 | 40,241 |
| TextArea, 100k lines | 708.09 (652.33–757.58) | 71,703,520 | 400,251 |
| CodeEditor, 100k lines | 0.59 (0.54–0.77) | 156,072 | 2,204 |
| Image widget, paint | 0.001 (0.0009–0.0010) | 352 | 2 |
| HiDPI resize, alternating sizes | 9.69 (9.41–13.28) | 15,164,816 | 11,039 |
| Software, 1k quads | 0.68 (0.51–1.40) | 0 | 0 |
| Software, 10k quads | 5.25 (4.83–6.84) | 0 | 0 |
| Software, 10k glyph masks | 21.06 (19.83–53.41) | 0 | 0 |
| Software, bilinear image at 2x | 77.94 (76.20–85.74) | 128 | 1 |

The dominant finding is multiline `TextArea`: 100k-line paint is ~708 ms
and allocates ~72 MB, while visible-line `CodeEditor` is ~0.6 ms on this
fixture. A 10k-widget full layout is ~99 ms. Those are good regression
anchors, not proof that CodeEditor covers the same editing semantics or that
GPU rasterization alone solves layout and text costs. Investigate separately
with pprof on the target machines; record integrated `FrameSample` traces
and OS upload/present time before choosing the next optimization.
