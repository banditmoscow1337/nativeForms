# Large forms with VirtualStack

Use `VirtualStack` as the child of `ScrollPanel` when a vertical form has many
independent rows. A regular `Panel` still measures and arranges every child
on layout; clip-aware painting alone avoids only offscreen `Paint` calls.

```go
type Record struct { ID, Value string; RowHeight float32 }
type formRows struct { records []Record }

func (m *formRows) Len() int { return len(m.records) }
func (m *formRows) Key(i int) string { return m.records[i].ID }
func (m *formRows) Height(i int) float32 { return m.records[i].RowHeight }
func (m *formRows) Build(i int) nativeforms.Component {
    return nativeforms.NewTextField(m.records[i].Value, nil)
}

rows := nativeforms.NewVirtualStack(&formRows{records: records})
scroll := nativeforms.NewScrollPanel(rows)
manager.SetRoot(scroll)
```

Keys must be unique and stable across reordering. The model supplies a height
for each row and `VirtualStack` indexes these heights in one pass; `Arrange`
then finds the visible rows with binary search. A row is retained while its
key remains visible and released after it scrolls out. Implement the optional
`StackModelUpdater.Update(index, row)` to refresh retained content when the
record changes or moves. After changing the number, order or height of rows,
call `rows.ModelChanged()` on the UI thread. This rebuilds the height index.

The view constructs one row above and below the viewport to avoid visible
gaps during fractional scrolling. A focused row that leaves the viewport is
removed and loses focus. Applications that must preserve editing state across
large scrolls should store it in the model under the stable key and restore it
in `Build`.
