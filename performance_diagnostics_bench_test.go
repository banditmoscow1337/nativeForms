package nativeforms

import (
	"strings"
	"testing"

	"github.com/banditmoscow1337/nativeForms/editor"
)

type paintVisitProbe struct {
	State
	visits, visible *int
}

func (p *paintVisitProbe) Paint(*Canvas) {
	*p.visits++
	if p.rect.Y < 768 && p.rect.Y+p.rect.H > 0 {
		*p.visible++
	}
}

func BenchmarkOffscreenPaintVisits(b *testing.B) {
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
	visits, visible = 0, 0
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		manager.BeginFrame(1024, 768, 0)
	}
	b.ReportMetric(float64(visits)/float64(b.N), "visits/op")
	b.ReportMetric(float64(visible)/float64(b.N), "visible/op")
}

// These targeted cases isolate text-input snapshots and a cold distant editor
// jump. They complement the paint/layout and raster benchmarks, not FPS.
func BenchmarkTextInputSnapshot(b *testing.B) {
	for _, sample := range []struct {
		name, text string
	}{
		{"1k-lines", strings.Repeat("alpha beta gamma\n", 1000)},
		{"100k-lines", strings.Repeat("alpha beta gamma\n", 100000)},
	} {
		b.Run(sample.name, func(b *testing.B) {
			field := NewTextArea(sample.text, nil)
			field.Cursor = len([]rune(sample.text))
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				state, ok := field.TextInputSnapshot(4096)
				if !ok || state.Cursor < 0 {
					b.Fatal("invalid text input state")
				}
			}
		})
	}
}

func BenchmarkFocusedTextInputState100k(b *testing.B) {
	field := NewTextArea(strings.Repeat("alpha beta gamma\n", 100000), nil)
	manager := New()
	manager.SetRoot(field)
	manager.BeginFrame(1024, 768, 0)
	manager.focused = field
	b.ReportAllocs()

	for b.Loop() {
		state, ok := manager.TextInputState()
		if !ok || state.Identity != field.UIState() {
			b.Fatal("missing focused input")
		}
	}
}

func BenchmarkCodeEditorColdJump100k(b *testing.B) {
	document, err := editor.NewDocument(strings.Repeat("const x = 12345; // source line\n", 100000))
	if err != nil {
		b.Fatal(err)
	}
	manager := New()
	b.ReportAllocs()

	for b.Loop() {
		b.StopTimer()
		widget := NewCodeEditor(document)
		manager.SetRoot(widget)
		manager.BeginFrame(1024, 768, 0)
		b.StartTimer()
		widget.ScrollToLine(90000)
		manager.BeginFrame(1024, 768, 0)
	}
	if manager.Frame().Width != 1024 {
		b.Fatal("missing frame")
	}
}

func BenchmarkCodeEditorWarmJump100k(b *testing.B) {
	document, err := editor.NewDocument(strings.Repeat("const x = 12345; // source line\n", 100000))
	if err != nil {
		b.Fatal(err)
	}
	manager := New()
	widget := NewCodeEditor(document)
	manager.SetRoot(widget)
	manager.BeginFrame(1024, 768, 0)
	widget.ScrollToLine(90000)
	manager.BeginFrame(1024, 768, 0)
	b.ReportAllocs()

	for i := 0; b.Loop(); i++ {
		widget.ScrollToLine(90001 + i%10)
		manager.BeginFrame(1024, 768, 0)
	}
}

func BenchmarkFrameBuildLongParagraph(b *testing.B) {
	benchmarkManagerFrame(b, NewTextArea(strings.Repeat("wide text with spaces ", 1000), nil), false, 1)
}
