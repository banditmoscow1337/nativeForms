# Software renderer benchmark: Apple M1 (user supplied, 2026-10-13)

These numbers were supplied after stage 0 was merged into `main`. The Go
version, OS version, power profile, benchmark command and display conditions
were not included, so this is a reference snapshot rather than the full
target-machine acceptance baseline. Units are Go benchmark `ns/op`.

| Benchmark | Iterations | ns/op | B/op | allocs/op |
| --- | ---: | ---: | ---: | ---: |
| RenderCommands/1k | 3,255 | 368,258 | 0 | 0 |
| RenderCommands/10k | 355 | 3,364,284 | 0 | 0 |
| RenderTextMasks | 98 | 12,577,339 | 0 | 0 |
| RenderImageHiDPI | 21 | 53,866,968 | 6 | 0 |

The reported run passed with 49.2% statement coverage. Coverage sampling can
change timing, so compare future runs only with the same flags. The image at
2x takes roughly 53.9 ms per render in this isolated benchmark. This does
not include widget layout, staging upload or display submission. Record those
with `FrameSample` in an integrated app before assigning a whole-frame budget.
