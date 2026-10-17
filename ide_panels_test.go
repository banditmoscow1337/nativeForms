package nativeforms

import (
	"reflect"
	"testing"
)

type mutablePanelTree struct {
	roots    []string
	children map[string][]string
}

func (m *mutablePanelTree) Roots() int                    { return len(m.roots) }
func (m *mutablePanelTree) Root(i int) string             { return m.roots[i] }
func (m *mutablePanelTree) Label(id string) string        { return id }
func (m *mutablePanelTree) Children(id string) int        { return len(m.children[id]) }
func (m *mutablePanelTree) Child(id string, i int) string { return m.children[id][i] }

func TestVirtualTreeModelChanged(t *testing.T) {
	model := &mutablePanelTree{roots: []string{"project"},
		children: map[string][]string{"project": {"a.go", "b.go"}}}
	tree := NewVirtualTree(model)
	tree.SetExpanded("project", true)
	tree.SetSelected("project")
	tree.Arrange(Rect{W: 200, H: 32})
	if len(tree.rows) != 3 {
		t.Fatalf("initial nodes: %v", tree.rows)
	}
	tree.Offset = 64
	model.children["project"] = []string{"c.go"}
	tree.ModelChanged()
	if len(tree.rows) != 2 || tree.rows[1].id != "c.go" || tree.Offset != 32 {
		t.Fatalf("changed nodes or offset: rows=%v, offset=%v", tree.rows, tree.Offset)
	}
	if !tree.Expanded["project"] || tree.SelectedID != "project" {
		t.Fatal("model update lost expansion or selection")
	}
	model.roots = nil
	tree.ModelChanged()
	if len(tree.rows) != 0 || tree.Offset != 0 {
		t.Fatalf("removed roots remained visible: rows=%v, offset=%v", tree.rows, tree.Offset)
	}
}

func TestOutputPanelStreamingAndScroll(t *testing.T) {
	panel := NewOutputPanel()
	panel.Arrange(Rect{W: 180, H: 64})
	panel.SetMaxLines(3)
	panel.AppendText("a\r")
	panel.AppendText("\nb\nc")
	if got := panel.Lines(); !reflect.DeepEqual(got, []string{"a", "b", "c"}) {
		t.Fatalf("chunked lines: %q", got)
	}
	panel.AppendText("\nd\n")
	if got := panel.Lines(); !reflect.DeepEqual(got, []string{"b", "c", "d"}) {
		t.Fatalf("bounded lines: %q", got)
	}
	if panel.List.Offset != 32 || panel.List.Model.ID(0) != "1" {
		t.Fatalf("tail follow and stable IDs: offset=%v, first=%s", panel.List.Offset, panel.List.Model.ID(0))
	}
	panel.List.Offset = 0 // Reader scrolled back.
	panel.AppendText("e\n")
	if panel.List.Offset != 0 {
		t.Fatalf("append disturbed reader's scroll: %v", panel.List.Offset)
	}
	if got := panel.Lines(); !reflect.DeepEqual(got, []string{"c", "d", "e"}) {
		t.Fatalf("trimmed lines: %q", got)
	}
	panel.ScrollToEnd()
	if panel.List.Offset != 32 {
		t.Fatalf("explicit tail follow: %v", panel.List.Offset)
	}
	panel.Clear()
	if len(panel.Lines()) != 0 || panel.List.Offset != 0 {
		t.Fatal("clear retained rows or scroll")
	}
}

func TestTabsWorkspaceOperations(t *testing.T) {
	var changes []string
	tabs := NewTabs([]Tab{{ID: "a", Title: "a", Content: NewPanel()},
		{ID: "b", Title: "b", Content: NewPanel()}}, "a",
		func(id string) { changes = append(changes, id) })
	tabs.Arrange(Rect{W: 140, H: 100})
	if !tabs.SetDirty("a", true) || !tabs.Tabs[0].Dirty || tabs.SetDirty("a", true) {
		t.Fatal("dirty indicator did not update")
	}
	if !tabs.Open(Tab{ID: "c", Title: "c", Content: NewPanel()}) ||
		tabs.Open(Tab{ID: "c", Title: "duplicate"}) || tabs.SelectedID != "c" || tabs.Offset <= 0 {
		t.Fatal("opening a tab did not select and scroll to it")
	}
	if !tabs.Move("a", 2) || tabs.Tabs[2].ID != "a" || !tabs.Tabs[2].Dirty {
		t.Fatal("reorder lost tab identity or dirty state")
	}
	tabs.OnCloseRequest = func(id string) bool { return id != "c" }
	if tabs.Close("c") || tabs.SelectedID != "c" {
		t.Fatal("close guard was ignored")
	}
	tabs.OnCloseRequest = nil
	if !tabs.Close("c") || tabs.SelectedID != "a" {
		t.Fatalf("closing selected tab chose %q", tabs.SelectedID)
	}
	tabs.Close("a")
	tabs.Close("b")
	if tabs.SelectedID != "" || !reflect.DeepEqual(changes, []string{"c", "a", "b", ""}) {
		t.Fatalf("selection notifications after close: %q", changes)
	}
}

func TestTabsSwitchTransfersEditorFocus(t *testing.T) {
	first, second := NewTextField("one", nil), NewTextField("two", nil)
	tabs := NewTabs([]Tab{{ID: "one", Title: "one", Content: first},
		{ID: "two", Title: "two", Content: second}}, "one", nil)
	manager := New()
	manager.SetInteractive(true)
	manager.SetRoot(tabs)
	manager.BeginFrame(300, 200, 0)
	manager.setFocus(first)
	tabs.Select("two")
	if manager.focused != second || first.Visible() || !second.Visible() {
		t.Fatal("selection left focus in a hidden editor")
	}
}
