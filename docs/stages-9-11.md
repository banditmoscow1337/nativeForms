# Stages 9–11: editing, layout and repaint

## Editing

`TextEditor` owns committed text, a caret and selection anchor in rune offsets,
bounded undo/redo snapshots, and separate IME preedit text. `NewTextField` and
`NewTextArea` share it. Use `SetText` for external updates, or edit through
`Editor()` and call `CommitEditor()` to publish the change. Exported `Text` and
`Cursor` remain for older code; direct writes need explicit invalidation.

The widget supports Shift selection, pointer drag, double-click word selection,
triple-click line selection, word navigation/deletion, Ctrl+A/C/X/V/Z/Y,
read-only and password modes, and `Validate` for proposed edits. A multiline
field wraps and scrolls its content and inserts a newline with Enter. Ctrl+Enter
submits it. The Windows host attaches the Unicode clipboard and IMM32 preedit
and result messages. `CaretRect` is expressed in logical window coordinates.

Grapheme navigation implements the common extended-cluster rules for combining
marks, Hangul, flag pairs, variation selectors, and ZWJ emoji. It is not a
complete implementation of the current UAX #29 property data: uncommon Indic
conjuncts and prepend cases can still split. Font shaping/ligatures and
password protection beyond visual masking are not provided.

## Layout and scrolling

`Constraints.BoundedX` and `BoundedY` distinguish an explicit zero maximum
from an unbounded axis. Existing positive `Max` values continue to be bounded.
Panels use intrinsic flex bases, optional `SetFlexBasis`, `SetFlex` growth, and
`SetFlexShrink` with min/max freezing. Horizontal panels also accept
`AlignBaseline`. `Label.Wrap` measures at the available width and is measured
again after grid column allocation.

`NewGrid` accepts `TrackAuto`, `TrackFixed`, and `TrackStar`; `AddAt` places
children, including spans. `NewForm(labelWidth)` creates a two-column grid
with `AddRow`. `NewSplitPane` creates a draggable divider. `ScrollPanel` retains
vertical `Offset` and `Scroll(pixels)` and adds `OffsetX`, `Horizontal`,
`ScrollBy`, `ScrollIntoView`, scrollbars, and residual wheel dispatch to outer
scroll panels. Its content is measured without a maximum along scroll axes.

## Repaint

Setters request layout or paint independently. `Manager.Post` wakes the native
message loop; caret and notification deadlines come from `NextFrameAfter`.
Windows repaints the full framebuffer for a changed frame. On an expose event
without a changed frame it presents the cached DIB. This keeps repaint correct
for overlapping layers; damaged-region drawing and measurement caches are
deliberately postponed until profiling shows a benefit. An embedded game can
still call `BeginFrame` from its own frame loop.

No builds or tests were run for this change at the project owner's request.
