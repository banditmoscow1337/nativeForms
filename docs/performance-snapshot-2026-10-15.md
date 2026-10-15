# Stage 1 diagnostic snapshot: 2026-10-15

Linux/amd64 sandbox, AMD EPYC 9V74, Go 1.27.2, `GOMAXPROCS=2`. The source is
the stage 0/1 working branch based on upstream `625979a5ac2ddae7fcb9062b142694d2a3b72979`.
Each benchmark has 20 timed operations and three independent repetitions;
initial widget construction and index building are outside the timed loop.
[Scroll/repaint output](perf/stage1-linux-2026-10-15.txt) and
[initialization output](perf/stage1-initial-linux-2026-10-15.txt) record every
repetition. Initialization uses one timed operation per run.

| Scenario | Median ms/op | B/op | allocs/op |
| --- | ---: | ---: | ---: |
| Virtual form, 10k variable-height rows, scroll | 0.092 | 4,892 | 125 |
| TextArea, 100k lines, distant steady scroll | 0.109 | 3,251 | 147 |
| Offscreen traversal, 1k rows | 0.013 | 352–356 | 2 |
| Focused input state, 100k lines | 3.623 | 3 | 1 |
| CodeEditor, 100k lines, cold jump near 90k | 2.762 | 89,704 | 2,080 |
| CodeEditor, 100k lines, nearby warm jump | 2.652 | 89,696 | 2,080 |
| TextArea, initial 100k-line frame (warm atlas) | 129.7 | 21,021,840 | 182 |
| CodeEditor, initial 100k-line frame | 23.0 | 4,455,656 | 7,171 |

The offscreen probe painted 20 intersecting rows out of 1,000, versus 1,000
visits in the earlier diagnostic snapshot. A prior single-operation run on the
same class of sandbox reported 277 ms for focused input geometry and 1,410 ms
for a cold editor jump; run length and source revision differ, so those figures
are directional comparisons, not controlled speedup factors. The remaining
~3.6 ms focused input path includes the surrounding-text byte/UTF-16 offset
query, which still scans from the beginning of TextField's string.
The first TextArea run also initialized the fallback font atlas and took
798 ms; later runs were about 128–130 ms. Initial 100k-line layout remains
linear and is distinct from steady scrolling.

No native window present, upload, p95 frame distribution or target-device
budgets are measured here. Run `cmd/perftrace` on Windows/RX 480 and macOS/M1
and attach raw JSONL before accepting the stage-0/1 hardware exit criteria.
