# ADR 0003: Text layout and cache ownership

Status: accepted for v1 (2026-10-14); first implementation (2026-10-15).

Current `TextField` can rewrap/repaint a large document, and repeated
`FontContext.Layout` calls allocate even when a line is unchanged. Avoid a
single global unbounded cache. Cache layout at paragraph/visible-line
granularity, keyed by document version or content hash, font family/size,
style, width, display scale and shaping parameters. Invalidate only edited
paragraphs and downstream wrap offsets; retain a bounded LRU for glyph masks
with generation IDs. Keep memory caps and expose hit/miss and eviction counts
in a future extended trace.

For `CodeEditor`, preserve its visible-line painting and piece-table storage.
For general multiline fields, first separate document storage from viewport
layout and paint, then virtualize wrapping and scrolling. Incremental edit
benchmarks must include beginning/middle/end inserts, IME composition,
selection, 1/10/100k lines and 1x/2x text. Do not cache only the rasterized
frame: selection, caret, popup and scale changes still need correct redraws.

The stage-0 `FrameSample.TextLayout` observes shaping initiated by
`Canvas.Text`/`Canvas.TextStyled`; text measurement in widget layout is part
of `FrameSample.Layout`. Compare those phases and allocations separately
before and after caching; the budgets live in `docs/performance-baseline.md`.

Stage 1 adds a per-TextField wrapped-line index keyed by editor version, width,
font specification, font registration generation and password mode. A normal
replacement rewraps neighboring paragraphs and shifts suffix rune offsets;
scroll, caret lookup and repaint reuse the index. Composition copies the
index once per preedit change and rewraps only neighboring paragraphs; later
frames with unchanged preedit reuse that copy. Each preedit change still copies
the document's rune and line arrays, and the ordinary TextEditor still copies
its string on edits. These paths need further work if large-document typing
or IME misses the target budget. CodeEditor prepares compact Go lexer carry states once per
document version so a cold jump does not lex preceding lines during painting.

FontContext retains glyph runs in a separate 8 MiB LRU (configurable with
`SetLayoutCacheLimit`). Public `Layout` returns independent slices; internal
painting reuses shared runs synchronously. Adding fonts invalidates layouts,
and `SetCacheLimit` clears runs that may retain evicted glyph masks. The frame
trace records layout cache hits and misses. TextField owns its line index;
there is no global cache retaining whole documents after widgets are dropped.

