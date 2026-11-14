package main

import (
	"fmt"
	"os"
	"runtime"
	"runtime/debug"
	"runtime/pprof"
	"strconv"
	"time"

	ui "github.com/banditmoscow1337/nativeForms"
)

func (d *demoApp) buildProfilerTab() ui.Component {
	root := ui.NewStack(ui.Vertical, 10)
	root.Padding = ui.All(8)

	title := ui.NewLabelKey("prof.title")
	title.Font = ui.FontSpec{Size: 16, Weight: 700}

	toggles := ui.NewStack(ui.Horizontal, 10)
	d.trackAllocsToggle = ui.NewToggleKey("prof.tgl_allocs", d.flags.trackAllocs, func(v bool) {
		d.flags.trackAllocs = v
		d.setupObserver(v)
		d.logf("TrackAllocations = %v", v)
	})
	d.trackAllocsToggle.UIState().SetFlex(1)

	d.stressToggle = ui.NewToggleKey("prof.tgl_stress", d.flags.stress, func(v bool) {
		d.setStressMode(v)
	})
	d.stressToggle.UIState().SetFlex(1)
	toggles.Add(d.trackAllocsToggle, d.stressToggle)

	actions := ui.NewStack(ui.Horizontal, 8)
	btnGC := newFlexButtonKey("prof.btn_gc", func() {
		before := readHeapAlloc()
		runtime.GC()
		debug.FreeOSMemory()
		after := readHeapAlloc()
		d.logf("GC complete: HeapAlloc %s -> %s", formatBytes(before), formatBytes(after))
	})

	btnHeap := newFlexButtonKey("prof.btn_heap", func() {
		path := nonEmpty(d.flags.memProfile, "heap.pprof")
		if err := writeHeapProfile(path); err != nil {
			d.logf("Error saving heap profile: %v", err)
		} else {
			d.manager.Notifyf("Heap profile saved to %s", path)
			d.logf("Heap profile saved to %s (go tool pprof -http=:8080 %s)", path, path)
		}
	})

	btnAllocs := newFlexButtonKey("prof.btn_allocs", func() {
		if err := writeNamedProfile("allocs", "allocs.pprof"); err != nil {
			d.logf("Error saving allocs.pprof: %v", err)
			return
		}
		_ = writeNamedProfile("goroutine", "goroutine.pprof")
		d.manager.Notify("Saved allocs.pprof and goroutine.pprof")
		d.logf("Saved profiles allocs.pprof and goroutine.pprof")
	})

	btnResetStats := newFlexButtonKey("prof.btn_reset", func() {
		d.mu.Lock()
		d.framesTotal = 0
		d.maxBuild, d.maxPaint, d.maxLayout = 0, 0, 0
		d.sumBuildNS, d.allocBytesSum, d.allocObjSum = 0, 0, 0
		d.card1Cache, d.card2Cache, d.card3Cache = throttledText{}, throttledText{}, throttledText{}
		d.mu.Unlock()
		d.logf("Peak frame metrics reset")
	})
	actions.Add(btnGC, btnHeap, btnAllocs, btnResetStats)

	cards := ui.NewGrid(
		ui.Track{Mode: ui.TrackStar, Value: 1},
		ui.Track{Mode: ui.TrackStar, Value: 1},
		ui.Track{Mode: ui.TrackStar, Value: 1},
	)
	cards.ColumnGap = 10
	cards.RowGap = 10

	cardFrame := d.makeMetricCardKey("prof.card1_title", func() string {
		return d.formatThrottled(&d.card1Cache, func() string {
			d.mu.Lock()
			s := d.lastSample
			total := d.framesTotal
			maxB, maxL, maxP := d.maxBuild, d.maxLayout, d.maxPaint
			sumB := d.sumBuildNS
			d.mu.Unlock()
			avgB := time.Duration(0)
			if total > 0 {
				avgB = time.Duration(sumB / total)
			}
			return d.trf(
				"prof.card1_fmt",
				total, nonEmpty(s.Backend, "n/a"),
				s.Width, s.Height, s.DamagePixels,
				s.Build.Round(time.Microsecond), avgB.Round(time.Microsecond), maxB.Round(time.Microsecond),
				s.Layout.Round(time.Microsecond), maxL.Round(time.Microsecond),
				s.Paint.Round(time.Microsecond), maxP.Round(time.Microsecond),
				s.TextLayout.Round(time.Microsecond), s.Raster.Round(time.Microsecond),
				s.Upload.Round(time.Microsecond), s.Present.Round(time.Microsecond),
				s.Commands, s.Masks, s.Images,
				s.Glyphs, s.TextRunes,
				s.Batches, s.Vertices, s.TextureUploads, formatBytes(uint64(max(int64(0), s.UploadBytes))),
			)
		})
	})

	cardMem := d.makeMetricCardKey("prof.card2_title", func() string {
		return d.formatThrottled(&d.card2Cache, func() string {
			d.mu.Lock()
			lastAllocB, lastAllocO := d.lastSample.AllocBytes, d.lastSample.AllocObjects
			sumAllocB, sumAllocO := d.allocBytesSum, d.allocObjSum
			d.mu.Unlock()
			m := readRuntimeMemSnapshot()
			return d.trf(
				"prof.card2_fmt",
				formatBytes(m.heapAlloc),
				formatBytes(m.heapInuse), formatBytes(m.heapIdle),
				formatBytes(m.heapSys), formatBytes(m.totalSys),
				m.heapObjects,
				formatBytes(m.totalAlloc),
				m.mallocs, m.frees,
				m.numGC,
				runtime.NumGoroutine(), runtime.GOMAXPROCS(0),
				formatBytes(lastAllocB), lastAllocO,
				formatBytes(sumAllocB), sumAllocO,
			)
		})
	})

	cardCache := d.makeMetricCardKey("prof.card3_title", func() string {
		return d.formatThrottled(&d.card3Cache, func() string {
			d.mu.Lock()
			fHits, fMisses := d.lastSample.TextCacheHits, d.lastSample.TextCacheMisses
			d.mu.Unlock()
			glyphBytes, glyphEntries := d.fonts.CacheUsage()
			layoutBytes, layoutEntries, hits, misses := d.fonts.LayoutCacheStats()
			hitRate := 0.0
			if hits+misses > 0 {
				hitRate = float64(hits) * 100.0 / float64(hits+misses)
			}
			return d.trf(
				"prof.card3_fmt",
				formatBytes(uint64(glyphBytes)), glyphEntries,
				formatBytes(uint64(layoutBytes)), layoutEntries,
				hits, misses, hitRate,
				fHits, fMisses,
				d.stressRunning.Load(),
				d.stressStep.Load(),
				d.manager.DisplayScale(),
				d.manager.Locale(),
			)
		})
	})

	cards.AddAt(cardFrame, 0, 0, 1, 1)
	cards.AddAt(cardMem, 0, 1, 1, 1)
	cards.AddAt(cardCache, 0, 2, 1, 1)

	root.Add(title, toggles, actions, cards)
	return root
}

func (d *demoApp) makeMetricCardKey(headingKey string, textFn func() string) ui.Component {
	card := ui.NewStack(ui.Vertical, 6)
	card.Padding = ui.All(10)
	card.Background = d.darkTheme.PanelAlt
	card.Border = d.darkTheme.Border
	card.BorderWidth = 1

	hdr := ui.NewLabelKey(headingKey)
	hdr.Font = ui.FontSpec{Size: 14, Weight: 700}

	body := ui.NewLabel("")
	body.Font = ui.FontSpec{Family: "monospace", Size: 15}
	body.TextFunc = textFn

	card.Add(hdr, body)
	return card
}

func (d *demoApp) shortPerfSummary() string {
	return d.formatThrottled(&d.bannerCache, func() string {
		d.mu.Lock()
		s := d.lastSample
		total := d.framesTotal
		d.mu.Unlock()
		m := readRuntimeMemSnapshot()
		return fmt.Sprintf(
			"Frames: %d | Build: %v | Cmds: %d | Glyphs: %d | Allocs: %d (%s) | Heap: %s | GC: %d",
			total, s.Build.Round(time.Microsecond), s.Commands, s.Glyphs,
			s.AllocObjects, formatBytes(s.AllocBytes), formatBytes(m.heapAlloc), m.numGC,
		)
	})
}

func writeHeapProfile(path string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	runtime.GC()
	return pprof.WriteHeapProfile(f)
}

func writeNamedProfile(name, path string) error {
	prof := pprof.Lookup(name)
	if prof == nil {
		return fmt.Errorf("profile %q not found", name)
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return prof.WriteTo(f, 0)
}

func readHeapAlloc() uint64 {
	return readRuntimeMemSnapshot().heapAlloc
}

func formatBytes(b uint64) string {
	const unit = 1024
	if b < unit {
		return strconv.FormatUint(b, 10) + " B"
	}
	div, exp := uint64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return strconv.FormatFloat(float64(b)/float64(div), 'f', 2, 64) + " " + string("KMGTPE"[exp]) + "iB"
}

func nonEmpty(s, fallback string) string {
	if s == "" {
		return fallback
	}
	return s
}
