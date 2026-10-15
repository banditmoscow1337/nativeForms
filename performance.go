package nativeforms

import (
	"runtime"
	"time"
)

// FramePhase names measured work in the renderer pipeline. Build includes
// Layout and Paint, and TextLayout is a subset of Paint.
type FramePhase uint8

const (
	PhaseBuild FramePhase = iota
	PhaseLayout
	PhasePaint
	PhaseTextLayout
	PhaseRaster
	PhaseUpload
	PhasePresent
)

// FrameSample reports one completed frame. Build includes Layout and Paint;
// TextLayout is a subset of Paint. Present is the time spent submitting the
// frame to the window system, not the time until it appears on the display.
type FrameSample struct {
	Width, Height                                         int
	Build, Layout, Paint, TextLayout                      time.Duration
	Raster, Upload, Present                               time.Duration
	GPUWait, CompletedGPU                                 time.Duration // GPU timing belongs to an earlier completed frame.
	Commands, Masks, Images, TextRunes                    int
	Batches, Vertices, TextureUploads, MaskAtlasFallbacks int
	UploadBytes                                           int64
	Glyphs, DamagePixels                                  int
	TextCacheHits, TextCacheMisses                        uint64
	AllocBytes, AllocObjects                              uint64
	Backend, FallbackReason                               string
}

// RenderStats describes a native GPU submission. CompletedGPU is the last
// frame that finished before the current slot was reused, never scan-out time.
type RenderStats struct {
	Build, Upload, Submit, GPUWait, CompletedGPU          time.Duration
	Batches, Vertices, TextureUploads, MaskAtlasFallbacks int
	UploadBytes                                           int64
}

func (trace *FrameTrace) RecordRenderStats(stats RenderStats) {
	if trace == nil || trace.finished {
		return
	}
	trace.sample.Raster += stats.Build
	trace.sample.Upload += stats.Upload
	trace.sample.Present += stats.Submit
	trace.sample.GPUWait += stats.GPUWait
	trace.sample.CompletedGPU = stats.CompletedGPU
	trace.sample.Batches += stats.Batches
	trace.sample.Vertices += stats.Vertices
	trace.sample.TextureUploads += stats.TextureUploads
	trace.sample.MaskAtlasFallbacks += stats.MaskAtlasFallbacks
	trace.sample.UploadBytes += stats.UploadBytes
}

// FrameObserverOptions controls potentially intrusive profiling features.
// Allocation sampling calls runtime.ReadMemStats twice per frame and should
// only be enabled for diagnosis, not when measuring render latency.
type FrameObserverOptions struct {
	TrackAllocations bool
}

// FrameTrace is only used from the window's frame/render goroutine. A nil
// trace is safe: adapters can call its methods without separate branches.
type FrameTrace struct {
	sample           FrameSample
	observer         func(FrameSample)
	trackAllocations bool
	startAlloc       uint64
	startObjects     uint64
	startHits        uint64
	startMisses      uint64
	fonts            *FontContext
	finished         bool
}

// SetFrameObserver enables or disables opt-in per-frame measurements. The
// callback runs synchronously after FrameTrace.Finish, on the render goroutine.
// It must not retain DrawCommand data or block the UI loop.
func (manager *Manager) SetFrameObserver(observer func(FrameSample), options ...FrameObserverOptions) {
	manager.mu.Lock()
	manager.frameObserver = observer
	manager.frameObserverOptions = FrameObserverOptions{}
	if len(options) > 0 {
		manager.frameObserverOptions = options[0]
	}
	manager.mu.Unlock()
}

// FrameTrace returns the trace created by the most recent BeginFrame. The
// caller must finish it after presentation, before the next BeginFrame.
// It is nil when no observer is installed.
func (manager *Manager) FrameTrace() *FrameTrace { return manager.frameTrace }

func newFrameTrace(observer func(FrameSample), options FrameObserverOptions, width, height int) *FrameTrace {
	if observer == nil {
		return nil
	}
	trace := &FrameTrace{observer: observer, trackAllocations: options.TrackAllocations}
	trace.sample.Width, trace.sample.Height = width, height
	if trace.trackAllocations {
		var mem runtime.MemStats
		runtime.ReadMemStats(&mem)
		trace.startAlloc = mem.TotalAlloc
		trace.startObjects = mem.Mallocs
	}
	return trace
}

// RecordDamage reports the conservative pixel region uploaded to the OS.
func (trace *FrameTrace) RecordDamage(width, height int) {
	if trace != nil && !trace.finished && width > 0 && height > 0 {
		trace.sample.DamagePixels = width * height
	}
}

// Start returns a timestamp only when tracing is enabled.
func (trace *FrameTrace) Start() time.Time {
	if trace == nil {
		return time.Time{}
	}
	return time.Now()
}

// Record adds the elapsed time to one phase. Adapters may use Raster, Upload,
// and Present to instrument the same logical frame across the pipeline.
func (trace *FrameTrace) Record(phase FramePhase, start time.Time) {
	if trace == nil || trace.finished || start.IsZero() {
		return
	}
	d := time.Since(start)
	switch phase {
	case PhaseBuild:
		trace.sample.Build += d
	case PhaseLayout:
		trace.sample.Layout += d
	case PhasePaint:
		trace.sample.Paint += d
	case PhaseTextLayout:
		trace.sample.TextLayout += d
	case PhaseRaster:
		trace.sample.Raster += d
	case PhaseUpload:
		trace.sample.Upload += d
	case PhasePresent:
		trace.sample.Present += d
	}
}

func (trace *FrameTrace) countCommands(frame Frame) {
	if trace == nil {
		return
	}
	trace.sample.Commands = len(frame.Commands)
	for _, command := range frame.Commands {
		switch command.Kind {
		case CommandMask:
			trace.sample.Masks++
		case CommandImage:
			trace.sample.Images++
		}
	}
}

// Finish publishes exactly one sample; FallbackReason is empty when the
// selected backend rendered normally. Finish is also safe on a nil trace.
func (trace *FrameTrace) Finish(backend, fallbackReason string) {
	if trace == nil || trace.finished {
		return
	}
	trace.finished = true
	trace.sample.Backend = backend
	trace.sample.FallbackReason = fallbackReason
	if trace.fonts != nil {
		_, _, hits, misses := trace.fonts.LayoutCacheStats()
		trace.sample.TextCacheHits = hits - trace.startHits
		trace.sample.TextCacheMisses = misses - trace.startMisses
	}
	if trace.trackAllocations {
		var mem runtime.MemStats
		runtime.ReadMemStats(&mem)
		trace.sample.AllocBytes = mem.TotalAlloc - trace.startAlloc
		trace.sample.AllocObjects = mem.Mallocs - trace.startObjects
	}
	trace.observer(trace.sample)
}
