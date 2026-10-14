# Diagnostic headless snapshot: 2026-10-14

This is a Linux/amd64 sandbox measurement on AMD EPYC 9V74 with Go 1.27.2,
`CGO_ENABLED=0`, `-p 1`, 1x layout scale. It uses the source based on GitHub
main `5d20d18d7f15b020e5c042a325012ad6776d259f` plus the stage 0/1
changes. It is not a Windows/RX 480 or macOS/M1 baseline. Raw single-iteration
output from five runs is in [diagnostics-linux-2026-10-14.txt](perf/diagnostics-linux-2026-10-14.txt).
The traversal fixture has its own [raw output](perf/offscreen-linux-2026-10-14.txt).

| Scenario | Median (ms) | B/op | allocs/op |
| --- | ---: | ---: | ---: |
| 100k-line surrounding text snapshot | 3.59 | 0 | 0 |
| 100k-line focused input state including caret geometry | 277.29 | 25,685,472 | 200,030 |
| CodeEditor 100k, cold jump to line 90k | 1410.30 | 20,633,320 | 182,099 |
| CodeEditor 100k, nearby warm jump | 1.60 | 89,928 | 2,082 |
| TextArea, one paragraph with 1k repeated fragments | 387.59 | 24,504,176 | 6,237 |

The offscreen traversal fixture invoked `Paint` on all 1,000 rows despite
only 20 intersecting its viewport (five runs of 100 iterations). Its Paint
method draws nothing, so the ~9 µs iteration is **not** a cost estimate for
real labels. It isolates a culling opportunity for a later optimization.

The focused IME path is far more expensive than its surrounding-text slice
because `Manager.TextInputState` also calculates caret geometry, which wraps
the document. The cold CodeEditor jump also reconstructs lexical state across
preceding lines. These measurements add separate optimization targets for
later stages. They do not measure native upload, presentation, or p95 frame
latency. Run `cmd/perftrace` on both target machines before treating any v1
performance budget as met.
