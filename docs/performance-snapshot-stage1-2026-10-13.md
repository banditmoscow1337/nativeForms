# Stage-1 headless snapshot: 2026-10-13

Linux/amd64, AMD EPYC 9V74, Go 1.27.0, `CGO_ENABLED=0`, `-p 1`.
This sandbox result is not an Apple M1 or Windows/RX 480 measurement and
does not include desktop upload, present or user input latency. The fixtures
are built and warmed outside the timed loop. Each row reports the median of
five independent single-iteration measurements with min–max in parentheses;
these values are not p95 frame times.

```sh
go test -p 1 -run '^$' -bench 'Benchmark(FrameBuildScrolledForm|FrameBuildTextArea|VirtualStackScroll10k|RenderImageHiDPI)' -benchmem -benchtime=1x -count=5 . ./software
```

| Case | Stage 0 median ms | Stage 1 median ms (min–max) | Stage 1 B/op | Stage 1 allocs/op |
| --- | ---: | ---: | ---: | ---: |
| Scrolled 1k-field form, paint | 5.15 | 0.081 (0.079–0.104) | 3,376 | 110 |
| TextArea, 1k lines, warm paint | 6.84 | 0.253 (0.241–0.258) | 18,448 | 222 |
| TextArea, 10k lines, warm paint | 65.90 | 0.271 (0.243–0.334) | 18,448 | 222 |
| TextArea, 100k lines, warm paint | 708.09 | 0.287 (0.245–0.465) | 18,448 | 222 |
| VirtualStack, 10k variable-height rows, scroll | — | 0.026 (0.023–0.038) | 6,240 | 144 |
| Bilinear image at 2x, full render | 77.94 | 86.89 (76.68–95.19) | 128 | 1 |
| Bilinear image at 2x, 100×100 pixel damage | — | 0.414 (0.385–0.457) | 0 | 0 |

The partial image fixture uses the same gradient/image command as the full
render. It measures a retained target where only a 100×100 physical pixel
region changed; it does not imply a full-frame image render became faster.
The full-render samples overlap the stage-0 range and give no evidence of a
full-frame raster speedup. The text and form numbers reflect the current
warm viewport and visible children; initial text indexing, full relayout,
large edits and OS presentation have separate costs.

The M1 numbers in `performance-snapshot-m1-2026-10-13.md` belong to the
merged stage-0 software renderer. Re-run both commits with the same flags
and scenario on the M1 before claiming a device-specific speedup. Record
integrated frame traces and p95 scroll/typing latency for release gates.
