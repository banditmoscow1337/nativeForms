# Stage 5: font context and text layout

`FontContext` is an opt-in, manager-owned portable text path. It uses the same
`Layout` result for measuring, painting, selection and hit testing. The old
global `SetFontData`, `SetFallbackFontData` and `SetTextShaper` path continues to
work for managers without a context, including the existing macOS Core Text
integration.

```go
fonts := nativeforms.NewFontContext()
if err := fonts.AddFont("interface", 400, false, regularTTF); err != nil { return err }
if err := fonts.AddFont("interface", 700, false, boldTTF); err != nil { return err }
fonts.AddSystemFallbacks() // optional; append after application fonts

manager := nativeforms.New()
manager.SetFontContext(fonts)
label := nativeforms.NewLabel("Привет, world!")
label.Font = nativeforms.FontSpec{Family: "interface", Weight: 700, Size: 18, Scale: 2}
manager.SetRoot(label)
```

The `Font` field is available on `Label`, `Button` and `TextField`. Legacy
`FontSize` on a label or button overrides `Font.Size` when set. Other widgets
use the context's first family at the theme size. `Canvas.TextStyled` allows
custom components to use the same specification. `FontContext.Measure`,
`Layout` and `CaretX` support standalone text measurement. Set `Scale` to the
ratio of physical to logical pixels; the UI frame and caret coordinates remain
logical. The application/window scale propagation is a later adapter stage.
`FontContext.Ellipsize` and `Label.Ellipsis` fit a single display line without
cutting a grapheme cluster.

Register faces before using a context in a manager. If you register more
faces or change widget font properties afterward, call
`manager.InvalidateLayout()`. Fallback checks the entire grapheme cluster;
when only the mark is missing, it draws the base from its selected face and
tries the mark from another face. Common decomposed Russian letters use a
precomposed glyph when available. The portable path supports ordinary RU/EN
combining accents, horizontal `kern` and synthetic bold/italic when no matching
face exists. `Language` is retained as a hint for future native shaping.

Masks are rasterized for the requested physical size, with a quarter-pixel
size cache key. The cache defaults to 8 MiB per context, can be changed with
`SetCacheLimit`, and evicts least recently used masks. Already recorded frames
retain immutable mask pointers. `TextEditor.UndoByteLimit` separately bounds
the total text retained in undo or redo snapshots (8 MiB by default); edits
larger than the budget are not undoable with this snapshot model.

Current limits: left-to-right RU/EN text only; no complete bidi, GSUB, GPOS,
hinting, CFF, variable/color fonts, or full Unicode mark positioning. The
portable renderer uses a simple 2×2 coverage rasterizer. Core Text remains
available through the existing global shaper for macOS managers that do not
select a portable context. DirectWrite integration is still future work.

The in-memory TrueType test face exercises fallback, decomposed clusters,
physical scaling, style masks, cache eviction, canvas placement, editor
hit testing and undo limits without installed fonts. The project owner runs
the automated and manual checks on the six supported desktop targets.
