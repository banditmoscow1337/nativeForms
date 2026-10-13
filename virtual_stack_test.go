package nativeforms

import (
	"strconv"
	"testing"
)

type virtualFormModel struct {
	ids    []string
	built  int
	height float32
}

func (m *virtualFormModel) Len() int        { return len(m.ids) }
func (m *virtualFormModel) ID(i int) string { return m.ids[i] }
func (m *virtualFormModel) Height(i int, _ float32) float32 {
	if i%2 == 0 {
		return m.height
	}
	return m.height * 2
}
func (m *virtualFormModel) Build(i int) Component {
	m.built++
	return NewTextField(m.ids[i], nil)
}

func newVirtualForm(count int) *virtualFormModel {
	m := &virtualFormModel{ids: make([]string, count), height: 20}
	for i := range m.ids {
		m.ids[i] = strconv.Itoa(i)
	}
	return m
}

func TestVirtualStackVariableRowsAndRecycling(t *testing.T) {
	model := newVirtualForm(10000)
	stack := NewVirtualStack(model)
	manager := New()
	manager.SetRoot(stack)
	manager.BeginFrame(400, 300, 0)
	if model.built == 0 || model.built > 30 {
		t.Fatalf("initial realization built %d of 10k rows", model.built)
	}
	first, end := stack.VisibleRange()
	if first != 0 || end < 9 || end > 12 {
		t.Fatalf("unexpected variable-height visible range: %d:%d", first, end)
	}
	old := stack.realized["2"]
	stack.ScrollBy(10)
	if stack.realized["2"] != old {
		t.Fatal("stable ID did not preserve a visible component")
	}
	stack.ScrollTo(9000)
	manager.BeginFrame(400, 300, 0)
	first, end = stack.VisibleRange()
	if first < 8980 || end > 9020 || model.built > 60 {
		t.Fatalf("scroll materialized too many rows: %d:%d, built=%d", first, end, model.built)
	}
	model.ids = append([]string{"new"}, model.ids...)
	anchor := stack.anchorID
	stack.ModelChanged()
	manager.BeginFrame(400, 300, 0)
	if len(stack.children) > 30 || stack.contentHeight() <= 300 {
		t.Fatalf("model update lost virtualization: %d children", len(stack.children))
	}
	if stack.anchorID != anchor {
		t.Fatalf("model insertion moved scroll anchor from %q to %q", anchor, stack.anchorID)
	}
}

func TestClipAwarePaintSkipsOffscreenFormRows(t *testing.T) {
	form := NewPanel(Vertical)
	for i := 0; i < 1000; i++ {
		field := NewTextField("value", nil)
		field.SetPreferredSize(Vec2{Y: 30})
		form.Add(field)
	}
	manager := New()
	manager.SetRoot(NewScrollPanel(form))
	manager.BeginFrame(400, 300, 0)
	if got := len(manager.Frame().Commands); got > 250 {
		t.Fatalf("offscreen widgets emitted %d commands", got)
	}
}
