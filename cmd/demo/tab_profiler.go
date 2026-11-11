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

	title := ui.NewLabel("Профилирование CPU и Памяти (FrameObserver, runtime/metrics, pprof, FontContext Cache)")
	title.Font = ui.FontSpec{Size: 16, Weight: 700}

	toggles := ui.NewStack(ui.Horizontal, 10)
	d.trackAllocsToggle = ui.NewToggle("Отслеживать аллокации на каждый кадр (FrameObserverOptions.TrackAllocations)", d.flags.trackAllocs, func(v bool) {
		d.flags.trackAllocs = v
		d.setupObserver(v)
		d.logf("TrackAllocations = %v", v)
	})
	d.trackAllocsToggle.UIState().SetFlex(1)

	d.stressToggle = ui.NewToggle("Автоматический стресс-генератор нагрузки (Stress Monkey)", d.flags.stress, func(v bool) {
		d.setStressMode(v)
	})
	d.stressToggle.UIState().SetFlex(1)
	toggles.Add(d.trackAllocsToggle, d.stressToggle)

	actions := ui.NewStack(ui.Horizontal, 8)
	btnGC := newFlexButton("Принудительный GC + FreeOSMemory", func() {
		before := readHeapAlloc()
		runtime.GC()
		debug.FreeOSMemory()
		after := readHeapAlloc()
		d.logf("GC выполнен: HeapAlloc %s -> %s", formatBytes(before), formatBytes(after))
	})

	btnHeap := newFlexButton("Сохранить Heap Profile (heap.pprof)", func() {
		path := nonEmpty(d.flags.memProfile, "heap.pprof")
		if err := writeHeapProfile(path); err != nil {
			d.logf("Ошибка сохранения Heap-профиля: %v", err)
		} else {
			d.manager.Notifyf("Heap-профиль сохранён в %s", path)
			d.logf("Heap-профиль сохранён в %s (go tool pprof -http=:8080 %s)", path, path)
		}
	})

	btnAllocs := newFlexButton("Сохранить Allocs/Goroutine Profile", func() {
		if err := writeNamedProfile("allocs", "allocs.pprof"); err != nil {
			d.logf("Ошибка allocs.pprof: %v", err)
			return
		}
		_ = writeNamedProfile("goroutine", "goroutine.pprof")
		d.manager.Notify("Сохранены allocs.pprof и goroutine.pprof")
		d.logf("Сохранены профили allocs.pprof и goroutine.pprof")
	})

	btnResetStats := newFlexButton("Сбросить пиковые метрики кадра", func() {
		d.mu.Lock()
		d.framesTotal = 0
		d.maxBuild, d.maxPaint, d.maxLayout = 0, 0, 0
		d.sumBuildNS, d.allocBytesSum, d.allocObjSum = 0, 0, 0
		d.card1Cache, d.card2Cache, d.card3Cache = throttledText{}, throttledText{}, throttledText{}
		d.mu.Unlock()
		d.logf("Пиковые метрики кадров сброшены")
	})
	actions.Add(btnGC, btnHeap, btnAllocs, btnResetStats)

	cards := ui.NewGrid(
		ui.Track{Mode: ui.TrackStar, Value: 1},
		ui.Track{Mode: ui.TrackStar, Value: 1},
		ui.Track{Mode: ui.TrackStar, Value: 1},
	)
	cards.ColumnGap = 10
	cards.RowGap = 10

	cardFrame := d.makeMetricCard("Тайминги кадра и рендерера (FrameSample)", func() string {
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
			return fmt.Sprintf(
				"Кадров: %d | Backend: %s\n"+
					"Размер: %dx%d (Damage: %d px)\n"+
					"Build: %v (avg %v, max %v)\n"+
					"Layout: %v (max %v)\n"+
					"Paint: %v (max %v)\n"+
					"TextLayout: %v | Raster: %v\n"+
					"Upload: %v | Present: %v\n"+
					"Commands: %d | Masks: %d | Images: %d\n"+
					"Glyphs: %d | TextRunes: %d\n"+
					"Batches: %d | Vertices: %d | Uploads: %d (%s)",
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

	cardMem := d.makeMetricCard("Память и GC (runtime/metrics — 0 STW)", func() string {
		return d.formatThrottled(&d.card2Cache, func() string {
			d.mu.Lock()
			lastAllocB, lastAllocO := d.lastSample.AllocBytes, d.lastSample.AllocObjects
			sumAllocB, sumAllocO := d.allocBytesSum, d.allocObjSum
			d.mu.Unlock()
			m := readRuntimeMemSnapshot()
			return fmt.Sprintf(
				"HeapAlloc (живая куча): %s\n"+
					"HeapInuse: %s | HeapIdle: %s\n"+
					"HeapSys: %s | TotalSys: %s\n"+
					"HeapObjects: %d\n"+
					"TotalAlloc (за всё время): %s\n"+
					"Mallocs: %d | Frees: %d\n"+
					"Циклов GC (NumGC): %d\n"+
					"Горутин: %d | GOMAXPROCS: %d\n"+
					"Аллокации кадра: %s (%d obj)\n"+
					"Сумма аллокаций кадров: %s (%d obj)",
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

	cardCache := d.makeMetricCard("Кэш шрифтов и Стресс-тест (FontContext)", func() string {
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
			return fmt.Sprintf(
				"Glyph Mask Cache: %s (%d глифов)\n"+
					"Text Layout Cache: %s (%d строк)\n"+
					"Layout Hits: %d | Misses: %d (%.1f%%)\n"+
					"За последний кадр: +%d hits / +%d misses\n"+
					"Стресс-режим активен: %v\n"+
					"Шагов стресс-теста: %d\n"+
					"Масштаб (DisplayScale): %.2fx\n"+
					"Совет по профилированию:\n"+
					"  go tool pprof -http=:8080 cpu.pprof\n"+
					"  go tool pprof -http=:8080 mem.pprof\n"+
					"  python3 tools/summarize_perftrace.py frames.jsonl",
				formatBytes(uint64(glyphBytes)), glyphEntries,
				formatBytes(uint64(layoutBytes)), layoutEntries,
				hits, misses, hitRate,
				fHits, fMisses,
				d.stressRunning.Load(),
				d.stressStep.Load(),
				d.manager.DisplayScale(),
			)
		})
	})

	cards.AddAt(cardFrame, 0, 0, 1, 1)
	cards.AddAt(cardMem, 0, 1, 1, 1)
	cards.AddAt(cardCache, 0, 2, 1, 1)

	root.Add(title, toggles, actions, cards)
	return root
}

func (d *demoApp) makeMetricCard(heading string, textFn func() string) ui.Component {
	card := ui.NewStack(ui.Vertical, 6)
	card.Padding = ui.All(10)
	card.Background = d.darkTheme.PanelAlt
	card.Border = d.darkTheme.Border
	card.BorderWidth = 1

	hdr := ui.NewLabel(heading)
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
