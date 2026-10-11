package nativeforms

import (
	"math"
	"runtime"
	"strings"
	"testing"

	"github.com/banditmoscow1337/nativeForms/editor"
)

func largeGoSource() string { return strings.Repeat("var value = 1\n", 100000) }

func TestCodeEditor100kRowsRemainVirtual(t *testing.T) {
	document,err:=editor.NewDocument(largeGoSource())
	if err!=nil { t.Fatal(err) }
	view:=NewCodeEditor(document)
	view.Arrange(Rect{W:900,H:560})
	view.ScrollToLine(90000)
	first,last:=view.VisibleLines()
	if document.LineCount()!=100001 || first<89999 || last-first>50 {
		t.Fatalf("100k line viewport: total=%d visible=%d..%d",document.LineCount(),first,last)
	}
	start,_,err:=document.LineRange(90000)
	if err!=nil || !view.ReplaceTextRange(start,start,"x") { t.Fatal("edit near the end failed") }
	if !view.Handle(Event{Type:KeyDown,Key:KeyZ,Mods:ModControl}) || document.LineCount()!=100001 {
		t.Fatal("large-document undo failed")
	}
}

func TestCodeEditorCyrillicEmojiIMEAndScaledHighlight(t *testing.T) {
	source:="package main\nvar имя = \"🙂\"\n"
	document,err:=editor.NewDocument(source)
	if err!=nil { t.Fatal(err) }
	view:=NewCodeEditor(document)
	manager:=New()
	manager.SetRoot(view)
	manager.SetDisplayScale(1)
	manager.BeginFrame(600,160,0)
	start:=strings.Index(source,"имя")
	end:=start+len("имя")
	if err:=view.SetSemanticTokens(document.Version(),[]string{"variable"},[]uint32{1,4,3,0,0});err!=nil {
		t.Fatalf("UTF-16 semantic highlight: %v",err)
	}
	if len(view.semantic)!=1 || view.semantic[0]!=(StyledRange{Start:start,End:end,Style:CodeVariable}) {
		t.Fatalf("Cyrillic byte coordinates: %+v",view.semantic)
	}
	if !view.SetSelection(start,end) { t.Fatal("Cyrillic selection failed") }
	manager.BeginFrame(600,160,0)
	highlight:=func() (Rect,bool) {
		for _,command:=range manager.Frame().Commands {
			if command.Kind==CommandQuad && command.Tint==manager.Theme().Selection && command.Rect.W>0 {
				return command.Rect,true
			}
		}
		return Rect{},false
	}
	normal,ok:=highlight()
	if !ok { t.Fatal("selected syntax range did not paint") }
	manager.SetDisplayScale(2)
	manager.BeginFrame(600,160,0)
	scaled,ok:=highlight()
	if !ok || math.Abs(float64(normal.X-scaled.X))>1 || math.Abs(float64(normal.W-scaled.W))>1 {
		t.Fatalf("logical highlight moved at 2x scale: before=%+v after=%+v",normal,scaled)
	}
	view.Handle(Event{Type:CompositionStart})
	view.Handle(Event{Type:CompositionUpdate,Text:"界"})
	if document.Text()!=source || !view.TextInputComposing() { t.Fatal("IME preedit mutated the document") }
	view.Handle(Event{Type:CompositionEnd,Text:"界"})
	if !strings.Contains(document.Text(),"var 界 = \"🙂\"") || view.TextInputComposing() {
		t.Fatalf("IME commit or emoji preservation: %q",document.Text())
	}
}

func BenchmarkCodeEditor100kEditUndo(b *testing.B) {
	document,err:=editor.NewDocument(largeGoSource())
	if err!=nil { b.Fatal(err) }
	view:=NewCodeEditor(document)
	view.Arrange(Rect{W:900,H:560})
	start,_,_:=document.LineRange(50000)
	b.ReportAllocs()
	b.ResetTimer()
	for i:=0;i<b.N;i++ {
		if !view.ReplaceTextRange(start,start,"x") { b.Fatal("edit") }
		if !view.Handle(Event{Type:KeyDown,Key:KeyZ,Mods:ModControl}) { b.Fatal("undo") }
	}
}

func BenchmarkCodeEditor100kScrollPaint(b *testing.B) {
	document,err:=editor.NewDocument(largeGoSource())
	if err!=nil { b.Fatal(err) }
	view:=NewCodeEditor(document)
	manager:=New()
	manager.SetRoot(view)
	manager.BeginFrame(900,560,0)
	b.ReportAllocs()
	b.ResetTimer()
	for i:=0;i<b.N;i++ {
		view.ScrollToLine((i*997)%100000)
		manager.BeginFrame(900,560,0)
	}
}

func BenchmarkCodeEditor100kUndoFootprint(b *testing.B) {
	document,err:=editor.NewDocument(largeGoSource())
	if err!=nil { b.Fatal(err) }
	start,_,_:=document.LineRange(50000)
	runtime.GC()
	var before,after runtime.MemStats
	runtime.ReadMemStats(&before)
	b.ReportAllocs()
	b.ResetTimer()
	for i:=0;i<b.N;i++ {
		value:="x"
		if i%2!=0 { value="y" }
		if _,err:=document.Apply([]editor.Edit{{Start:start,End:start+1,Text:value}});err!=nil { b.Fatal(err) }
	}
	b.StopTimer()
	runtime.GC()
	runtime.ReadMemStats(&after)
	if after.Alloc>before.Alloc {
		b.ReportMetric(float64(after.Alloc-before.Alloc)/float64(min(b.N,document.HistoryLimit)),"retained-B/undo")
	}
	runtime.KeepAlive(document)
}
