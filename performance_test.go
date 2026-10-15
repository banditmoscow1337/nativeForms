package nativeforms

import (
	"testing"
	"time"
)

func TestFrameTraceOptInAndSingleDelivery(t *testing.T) {
	manager := New()
	if manager.FrameTrace() != nil {
		t.Fatal("tracing should be disabled by default")
	}
	var samples []FrameSample
	manager.SetRoot(NewLabel("hello"))
	manager.SetFrameObserver(func(sample FrameSample) { samples = append(samples, sample) })
	manager.BeginFrame(320, 160, 0)
	trace := manager.FrameTrace()
	if trace == nil {
		t.Fatal("missing opt-in trace")
	}
	start := trace.Start()
	trace.Record(PhaseRaster, start)
	trace.Finish("test/software", "")
	trace.Finish("test/software", "duplicate")
	if len(samples) != 1 {
		t.Fatalf("got %d samples, want one", len(samples))
	}
	sample := samples[0]
	if sample.Width != 320 || sample.Height != 160 || sample.Backend != "test/software" || sample.FallbackReason != "" {
		t.Fatalf("unexpected sample: %+v", sample)
	}
	if sample.Build < 0 || sample.Layout < 0 || sample.Paint < 0 || sample.TextLayout < 0 || sample.Raster < 0 {
		t.Fatalf("negative duration: %+v", sample)
	}
	if sample.TextRunes < 5 || sample.Commands == 0 {
		t.Fatalf("missing text or command counts: %+v", sample)
	}
	manager.SetFrameObserver(nil)
	manager.BeginFrame(320, 160, time.Second.Seconds())
	if manager.FrameTrace() != nil {
		t.Fatal("observer removal did not disable tracing")
	}
	var disabled *FrameTrace
	disabled.Record(PhasePaint, disabled.Start())
	disabled.Finish("ignored", "")
}

func TestFrameTraceCountsCommandKinds(t *testing.T) {
	var sample FrameSample
	trace := newFrameTrace(func(got FrameSample) { sample = got }, FrameObserverOptions{}, 4, 4)
	trace.countCommands(Frame{Commands: []DrawCommand{{Kind: CommandQuad}, {Kind: CommandMask}, {Kind: CommandImage}}})
	trace.Finish("software", "no-device")
	if sample.Commands != 3 || sample.Masks != 1 || sample.Images != 1 || sample.FallbackReason != "no-device" {
		t.Fatalf("unexpected command count: %+v", sample)
	}
}

func TestFrameTraceGPUStatsAreSeparateFromCurrentSubmission(t *testing.T) {
	var sample FrameSample
	trace := newFrameTrace(func(got FrameSample) { sample = got }, FrameObserverOptions{}, 12, 8)
	stats := RenderStats{Build: 2 * time.Millisecond, Upload: 3 * time.Millisecond,
		Submit: time.Millisecond, GPUWait: 4 * time.Millisecond, CompletedGPU: 5 * time.Millisecond,
		Batches: 2, Vertices: 12, TextureUploads: 1, UploadBytes: 1024}
	trace.RecordRenderStats(stats)
	trace.Finish("hardware/metal", "")
	if sample.Raster != stats.Build || sample.Upload != stats.Upload || sample.Present != stats.Submit ||
		sample.GPUWait != stats.GPUWait || sample.CompletedGPU != stats.CompletedGPU ||
		sample.Batches != 2 || sample.Vertices != 12 || sample.TextureUploads != 1 || sample.UploadBytes != 1024 {
		t.Fatalf("lost GPU statistics: %+v", sample)
	}
}
