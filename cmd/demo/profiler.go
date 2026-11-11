package main

import (
	"runtime/metrics"
	"strconv"
	"time"

	ui "github.com/banditmoscow1337/nativeForms"
)

type throttledText struct {
	updated time.Time
	value   string
}

func (d *demoApp) formatThrottled(slot *throttledText, compute func() string) string {
	now := time.Now()
	d.mu.Lock()
	if slot.value != "" && now.Sub(slot.updated) < 250*time.Millisecond {
		txt := slot.value
		d.mu.Unlock()
		return txt
	}
	d.mu.Unlock()
	txt := compute()
	d.mu.Lock()
	slot.value = txt
	slot.updated = now
	d.mu.Unlock()
	return txt
}

func appendJSONIntField(dst []byte, key string, val int64, comma bool) []byte {
	dst = append(dst, '"')
	dst = append(dst, key...)
	dst = append(dst, '"', ':')
	dst = strconv.AppendInt(dst, val, 10)
	if comma {
		dst = append(dst, ',')
	}
	return dst
}

func appendJSONUintField(dst []byte, key string, val uint64, comma bool) []byte {
	dst = append(dst, '"')
	dst = append(dst, key...)
	dst = append(dst, '"', ':')
	dst = strconv.AppendUint(dst, val, 10)
	if comma {
		dst = append(dst, ',')
	}
	return dst
}

func appendJSONStringField(dst []byte, key, val string, comma bool) []byte {
	dst = append(dst, '"')
	dst = append(dst, key...)
	dst = append(dst, '"', ':')
	dst = strconv.AppendQuote(dst, val)
	if comma {
		dst = append(dst, ',')
	}
	return dst
}

// appendFrameRecordJSON serializes one jsonlFrameRecord directly into dst with
// zero reflection and zero heap allocations (avoiding encoding/json/v2 init).
func appendFrameRecordJSON(dst []byte, at time.Time, s ui.FrameSample) []byte {
	dst = append(dst, `{"Kind":"frame","Scenario":"full-showcase","At":"`...)
	dst = at.AppendFormat(dst, time.RFC3339Nano)
	dst = append(dst, `","Sample":{`...)
	dst = appendJSONIntField(dst, "Width", int64(s.Width), true)
	dst = appendJSONIntField(dst, "Height", int64(s.Height), true)
	dst = appendJSONIntField(dst, "Build", int64(s.Build), true)
	dst = appendJSONIntField(dst, "Layout", int64(s.Layout), true)
	dst = appendJSONIntField(dst, "Paint", int64(s.Paint), true)
	dst = appendJSONIntField(dst, "TextLayout", int64(s.TextLayout), true)
	dst = appendJSONIntField(dst, "Raster", int64(s.Raster), true)
	dst = appendJSONIntField(dst, "Upload", int64(s.Upload), true)
	dst = appendJSONIntField(dst, "Present", int64(s.Present), true)
	dst = appendJSONIntField(dst, "GPUWait", int64(s.GPUWait), true)
	dst = appendJSONIntField(dst, "CompletedGPU", int64(s.CompletedGPU), true)
	dst = appendJSONIntField(dst, "Commands", int64(s.Commands), true)
	dst = appendJSONIntField(dst, "Masks", int64(s.Masks), true)
	dst = appendJSONIntField(dst, "Images", int64(s.Images), true)
	dst = appendJSONIntField(dst, "TextRunes", int64(s.TextRunes), true)
	dst = appendJSONIntField(dst, "Batches", int64(s.Batches), true)
	dst = appendJSONIntField(dst, "Vertices", int64(s.Vertices), true)
	dst = appendJSONIntField(dst, "TextureUploads", int64(s.TextureUploads), true)
	dst = appendJSONIntField(dst, "MaskAtlasFallbacks", int64(s.MaskAtlasFallbacks), true)
	dst = appendJSONIntField(dst, "UploadBytes", s.UploadBytes, true)
	dst = appendJSONIntField(dst, "Glyphs", int64(s.Glyphs), true)
	dst = appendJSONIntField(dst, "DamagePixels", int64(s.DamagePixels), true)
	dst = appendJSONUintField(dst, "TextCacheHits", s.TextCacheHits, true)
	dst = appendJSONUintField(dst, "TextCacheMisses", s.TextCacheMisses, true)
	dst = appendJSONUintField(dst, "AllocBytes", s.AllocBytes, true)
	dst = appendJSONUintField(dst, "AllocObjects", s.AllocObjects, true)
	dst = appendJSONStringField(dst, "Backend", s.Backend, true)
	dst = appendJSONStringField(dst, "FallbackReason", s.FallbackReason, false)
	dst = append(dst, "}}\n"...)
	return dst
}

func (d *demoApp) setupObserver(trackAllocs bool) {
	if d.uiStressStep == nil {
		d.uiStressStep = func() {
			d.stressQueued.Store(false)
			if !d.stressRunning.Load() {
				return
			}
			cur := int(d.stressStep.Add(1))
			d.stepStressWorkload(cur)
		}
	}
	d.manager.SetFrameObserver(func(sample ui.FrameSample) {
		now := time.Now()
		d.mu.Lock()
		d.framesTotal++
		d.lastSample = sample
		d.sumBuildNS += uint64(sample.Build.Nanoseconds())
		if sample.Build > d.maxBuild {
			d.maxBuild = sample.Build
		}
		if sample.Paint > d.maxPaint {
			d.maxPaint = sample.Paint
		}
		if sample.Layout > d.maxLayout {
			d.maxLayout = sample.Layout
		}
		d.allocBytesSum += sample.AllocBytes
		d.allocObjSum += sample.AllocObjects
		if d.jsonlWriter != nil {
			d.jsonlBuf = appendFrameRecordJSON(d.jsonlBuf[:0], now.UTC(), sample)
			_, _ = d.jsonlWriter.Write(d.jsonlBuf)
		}
		d.mu.Unlock()

		// Drive fast-interval stress steps directly on the UI frame loop to avoid
		// cross-thread ticker condvar wakeups and Objective-C postEvent calls.
		if !d.flags.headless && d.stressRunning.Load() && d.stressWarmupDone.Load() &&
			d.flags.interval <= 20*time.Millisecond {
			d.lastStressUnixNano.Store(now.UnixNano())
			if d.stressQueued.CompareAndSwap(false, true) {
				d.manager.Post(d.uiStressStep)
			}
		}
	}, ui.FrameObserverOptions{TrackAllocations: trackAllocs})
}

type runtimeMemSnapshot struct {
	heapAlloc   uint64
	heapInuse   uint64
	heapIdle    uint64
	heapSys     uint64
	totalSys    uint64
	heapObjects uint64
	totalAlloc  uint64
	mallocs     uint64
	frees       uint64
	numGC       uint64
}

func readRuntimeMemSnapshot() runtimeMemSnapshot {
	samples := [...]metrics.Sample{
		{Name: "/memory/classes/heap/objects:bytes"},
		{Name: "/memory/classes/heap/unused:bytes"},
		{Name: "/memory/classes/heap/free:bytes"},
		{Name: "/memory/classes/heap/released:bytes"},
		{Name: "/memory/classes/total:bytes"},
		{Name: "/gc/heap/objects:objects"},
		{Name: "/gc/heap/allocs:bytes"},
		{Name: "/gc/heap/allocs:objects"},
		{Name: "/gc/heap/frees:objects"},
		{Name: "/gc/cycles/total:gc-cycles"},
	}
	metrics.Read(samples[:])
	val := func(i int) uint64 {
		if samples[i].Value.Kind() == metrics.KindUint64 {
			return samples[i].Value.Uint64()
		}
		return 0
	}
	heapObj := val(0)
	heapUnused := val(1)
	heapFree := val(2)
	heapReleased := val(3)
	return runtimeMemSnapshot{
		heapAlloc:   heapObj,
		heapInuse:   heapObj + heapUnused,
		heapIdle:    heapFree + heapReleased,
		heapSys:     heapObj + heapUnused + heapFree + heapReleased,
		totalSys:    val(4),
		heapObjects: val(5),
		totalAlloc:  val(6),
		mallocs:     val(7),
		frees:       val(8),
		numGC:       val(9),
	}
}
