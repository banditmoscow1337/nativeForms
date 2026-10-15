package nativeforms

import (
	"strconv"
	"testing"
)

type stackFixture struct{ count, builds, paints int }

func (m *stackFixture) Len() int             { return m.count }
func (m *stackFixture) Key(i int) string     { return strconv.Itoa(i) }
func (m *stackFixture) Height(i int) float32 { return float32(24 + i%5*8) }
func (m *stackFixture) Build(i int) Component {
	m.builds++
	p := &stackProbe{model: m}
	p.owner = p
	return p
}

type stackProbe struct {
	State
	model *stackFixture
}

func (p *stackProbe) Paint(*Canvas) { p.model.paints++ }

func TestVirtualStackVisibleRowsAndVariableHeights(t *testing.T) {
	m := &stackFixture{count: 10000}
	stack := NewVirtualStack(m)
	scroll := NewScrollPanel(stack)
	manager := New()
	manager.SetRoot(scroll)
	manager.BeginFrame(400, 240, 0)
	if m.builds > 20 || m.paints > 20 {
		t.Fatalf("initial viewport built %d and painted %d rows", m.builds, m.paints)
	}
	if got := stack.prefix[10000]; got != 400000 { // five rows sum to 200
		t.Fatalf("variable row height sum = %v", got)
	}
	m.paints = 0
	scroll.Scroll(200000)
	manager.BeginFrame(400, 240, 0)
	if m.builds > 40 || m.paints > 20 {
		t.Fatalf("distant viewport built %d and painted %d rows", m.builds, m.paints)
	}
	old := m.builds
	scroll.Scroll(8)
	manager.BeginFrame(400, 240, 0)
	if m.builds > old+2 {
		t.Fatalf("stable visible keys rebuilt %d rows", m.builds-old)
	}
}

func TestClipTraversalSkipsOffscreenPaint(t *testing.T) {
	visits, visible := 0, 0
	rows := NewPanel(Vertical)
	for i := 0; i < 1000; i++ {
		probe := &paintVisitProbe{visits: &visits, visible: &visible}
		probe.owner = probe
		probe.minimum = Vec2{X: 100, Y: 40}
		rows.Add(probe)
	}
	manager := New()
	manager.SetRoot(NewScrollPanel(rows))
	manager.BeginFrame(1024, 768, 0)
	if visits > 25 || visits != visible {
		t.Fatalf("Paint called %d times, visible %d", visits, visible)
	}
}
