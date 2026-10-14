# ADR 0003: Text layout and cache ownership

Status: accepted for v1 (2026-10-14). Cache implementation belongs to later stages.

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
