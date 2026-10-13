package nativeforms

import (
	"image"
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

func BenchmarkFrameBuildTextArea(b *testing.B) {
	for _, scenario := range []struct {
		name  string
		lines int
	}{{"1k", 1000}, {"10k", 10000}, {"100k", 100000}} {
		b.Run(scenario.name, func(b *testing.B) {
			benchmarkManagerFrame(b, NewTextArea(strings.Repeat("A long line of editable text 0123456789\n", scenario.lines), nil), false, 1)
		})
	}
}

func BenchmarkFrameBuildCodeEditor100k(b *testing.B) {
	document, err := editor.NewDocument(strings.Repeat("const x = 12345; // an editor line\n", 100000))
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
