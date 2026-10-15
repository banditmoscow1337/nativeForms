package nativeforms

import (
	"image"
	"strconv"
	"strings"
	"testing"

	"github.com/banditmoscow1337/nativeForms/editor"
)

func benchRows(count int) *Panel {
	rows := NewPanel(Vertical)
	for i := 0; i < count; i++ {
		rows.Add(NewLabel("Record 12345: sample text"))
	}
	return rows
}

func benchmarkManagerFrame(b *testing.B, component Component, relayout bool, scale float32) {
	b.Helper()
	manager := New()
	manager.SetRoot(component)
	manager.SetDisplayScale(scale)
	manager.BeginFrame(1024, 768, 0)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if relayout {
			manager.InvalidateLayout()
		}
		manager.BeginFrame(1024, 768, 0)
	}
	b.StopTimer()
	if manager.Frame().Width != 1024 {
		b.Fatal("invalid frame")
	}
}

func BenchmarkFrameBuildWidgets(b *testing.B) {
	for _, scenario := range []struct {
		name  string
		count int
	}{{"1k", 1000}, {"10k", 10000}} {
		b.Run(scenario.name+"/paint", func(b *testing.B) {
			benchmarkManagerFrame(b, benchRows(scenario.count), false, 1)
		})
		b.Run(scenario.name+"/layout", func(b *testing.B) {
			benchmarkManagerFrame(b, benchRows(scenario.count), true, 1)
		})
	}
}

func BenchmarkFrameBuildScrolledForm(b *testing.B) {
	form := NewPanel(Vertical)
	for i := 0; i < 1000; i++ {
		form.Add(NewTextField("Value 123456789", nil))
	}
	benchmarkManagerFrame(b, NewScrollPanel(form), false, 1)
}

type benchmarkStackModel int

func (m benchmarkStackModel) Len() int              { return int(m) }
func (m benchmarkStackModel) Key(i int) string      { return strconv.Itoa(i) }
func (m benchmarkStackModel) Height(i int) float32  { return float32(32 + i%3*8) }
func (m benchmarkStackModel) Build(i int) Component { return NewTextField("Value 123456789", nil) }

func BenchmarkVirtualFormScroll10k(b *testing.B) {
	scroll := NewScrollPanel(NewVirtualStack(benchmarkStackModel(10000)))
	manager := New()
	manager.SetRoot(scroll)
	manager.BeginFrame(1024, 768, 0)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		scroll.Scroll(float32(32 + i%3*8))
		manager.BeginFrame(1024, 768, 0)
	}
}

func BenchmarkTextAreaScroll100k(b *testing.B) {
	field := NewTextArea(strings.Repeat("alpha beta gamma\n", 100000), nil)
	manager := New()
	manager.SetRoot(field)
	manager.BeginFrame(1024, 768, 0)
	h := field.lineHeight(field.fontSpec())
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		field.scrollY = float32(90000+i%20) * h
		field.invalidatePaint()
		manager.BeginFrame(1024, 768, 0)
	}
}

func BenchmarkFrameBuildTextArea(b *testing.B) {
	for _, scenario := range []struct {
		name  string
		lines int
	}{{"1k", 1000}, {"10k", 10000}, {"100k", 100000}} {
		b.Run(scenario.name, func(b *testing.B) {
			text := strings.TrimSuffix(strings.Repeat("A long line of editable text 0123456789\n", scenario.lines), "\n")
			benchmarkManagerFrame(b, NewTextArea(text, nil), false, 1)
		})
	}
}

func BenchmarkTextAreaInitial100k(b *testing.B) {
	text := strings.Repeat("alpha beta gamma\n", 100000)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		manager := New()
		manager.SetRoot(NewTextArea(text, nil))
		manager.BeginFrame(1024, 768, 0)
	}
}

func BenchmarkCodeEditorInitial100k(b *testing.B) {
	text := strings.Repeat("const x = 12345; // source line\n", 100000)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		document, err := editor.NewDocument(text)
		if err != nil {
			b.Fatal(err)
		}
		manager := New()
		manager.SetRoot(NewCodeEditor(document))
		manager.BeginFrame(1024, 768, 0)
	}
}

func BenchmarkFrameBuildCodeEditor100k(b *testing.B) {
	text := strings.TrimSuffix(strings.Repeat("const x = 12345; // an editor line\n", 100000), "\n")
	document, err := editor.NewDocument(text)
	if err != nil {
		b.Fatal(err)
	}
	benchmarkManagerFrame(b, NewCodeEditor(document), false, 1)
}

func BenchmarkFrameBuildImage(b *testing.B) {
	resource, err := NewImageResource(image.NewNRGBA(image.Rect(0, 0, 512, 512)))
	if err != nil {
		b.Fatal(err)
	}
	benchmarkManagerFrame(b, NewImage(resource), false, 1)
}

func BenchmarkFrameResizeHiDPI(b *testing.B) {
	manager := New()
	manager.SetRoot(benchRows(1000))
	manager.SetDisplayScale(2)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if i%2 == 0 {
			manager.BeginFrame(1024, 768, 0)
		} else {
			manager.BeginFrame(800, 600, 0)
		}
	}
}
