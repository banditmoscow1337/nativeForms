package main

import (
	"fmt"
	"image"
	"image/png"
	"os"
	"os/signal"
	"runtime"
	"runtime/debug"
	"sync/atomic"
	"time"

	ui "github.com/banditmoscow1337/nativeForms"
	"github.com/banditmoscow1337/nativeForms/platform"
	"github.com/banditmoscow1337/nativeForms/platform/desktop"
	"github.com/banditmoscow1337/nativeForms/render"
	"github.com/banditmoscow1337/nativeForms/software"
)

func (d *demoApp) runHeadless() error {
	renderer := software.New()
	defer renderer.Close()

	scale := float32(d.flags.scale)
	if scale <= 0 {
		scale = 1
	}
	target := image.NewRGBA(image.Rect(0, 0, int(float32(d.flags.width)*scale), int(float32(d.flags.height)*scale)))

	renderOneFrame := func() error {
		curScale := d.manager.DisplayScale()
		if !(curScale > 0 && curScale <= 8) {
			curScale = scale
		}
		logicalW := platform.LogicalSize(target.Bounds().Dx(), curScale)
		logicalH := platform.LogicalSize(target.Bounds().Dy(), curScale)
		d.manager.BeginFrame(logicalW, logicalH, 0.016)
		tr := d.manager.FrameTrace()
		rasterStart := tr.Start()
		if err := renderer.RenderAtScale(d.manager.Frame(), target, curScale); err != nil {
			return err
		}
		tr.Record(ui.PhaseRaster, rasterStart)
		tr.RecordDamage(target.Bounds().Dx(), target.Bounds().Dy())
		tr.Finish(string(render.Software), "")
		return nil
	}

	if err := renderOneFrame(); err != nil {
		return err
	}
	d.runSelfTests()
	if err := renderOneFrame(); err != nil {
		return err
	}

	deadline := time.Now().Add(d.flags.duration)
	step := 0
	for time.Now().Before(deadline) {
		step++
		if d.flags.stress {
			d.stepStressWorkload(step)
		}
		if err := renderOneFrame(); err != nil {
			return err
		}
		if d.flags.interval > 0 && !d.flags.stress {
			time.Sleep(d.flags.interval)
		}
	}

	if d.flags.screenshot != "" {
		d.tabs.Select("controls")
		if err := renderOneFrame(); err != nil {
			return err
		}
		f, err := os.Create(d.flags.screenshot)
		if err != nil {
			return err
		}
		defer f.Close()
		if err := png.Encode(f, target); err != nil {
			return err
		}
		fmt.Printf("Saved headless screenshot to %s (%dx%d)\n", d.flags.screenshot, target.Bounds().Dx(), target.Bounds().Dy())
	}

	fmt.Printf("Headless run complete: %s\n", d.shortPerfSummary())
	return nil
}

func (d *demoApp) runDesktop() error {
	d.app = desktop.NewApp()
	ready := make(chan struct{})

	win, err := d.app.NewWindow(d.manager, desktop.Options{
		Title:              "nativeForms — Widget Showcase & CPU/Memory Profiler",
		Width:              d.flags.width,
		Height:             d.flags.height,
		Scale:              float32(d.flags.scale),
		Frameless:          d.flags.frameless,
		HideWindowControls: d.flags.hideWindowControls,
		Backend:            d.flags.backend,
		Graphics:           render.Kind(d.flags.graphics),
		OnReady: func(w desktop.Window) {
			d.setTrayEnabled(true)
			runtime.GC()
			debug.FreeOSMemory()
			close(ready)
		},
		OnFilesDropped: func(paths []string) {
			d.manager.Notifyf("Перетащено файлов: %d", len(paths))
			d.logf("OnFilesDropped: %v", paths)
		},
		OnCloseRequest: func(w desktop.Window) platform.CloseAction {
			switch d.closePolicy {
			case platform.HideWindow:
				d.manager.Notify("Окно скрыто в системный трей (OnCloseRequest: HideWindow)")
				d.logf("Окно скрыто в системный трей; откройте его через иконку NF в трее")
				return platform.HideWindow
			case platform.CancelClose:
				d.manager.Notify("Закрытие отменено политикой CancelClose (переключите во вкладке 4)")
				d.logf("Закрытие окна заблокировано (OnCloseRequest: CancelClose)")
				return platform.CancelClose
			default:
				return platform.QuitApp
			}
		},
	})
	if err != nil {
		return err
	}
	d.window = win

	if d.flags.stress {
		d.setStressMode(true)
	}

	stop := make(chan struct{})
	go func() {
		select {
		case <-ready:
		case <-stop:
			return
		}
		if d.flags.warmup > 0 {
			select {
			case <-time.After(d.flags.warmup):
			case <-stop:
				return
			}
		}
		d.stressWarmupDone.Store(true)
		var endTimer <-chan time.Time
		if d.flags.stress && d.flags.duration > 0 {
			t := time.NewTimer(d.flags.duration)
			defer t.Stop()
			endTimer = t.C
		}
		// When interval <= 20ms, frames are self-paced on the UI thread via
		// setupObserver -> manager.Post, so the background ticker only acts as a
		// coarse idle watchdog (avoiding 60Hz cross-thread condvar wakeups).
		tickPeriod := d.flags.interval
		if tickPeriod <= 20*time.Millisecond {
			tickPeriod = 100 * time.Millisecond
		}
		ticker := time.NewTicker(tickPeriod)
		defer ticker.Stop()

		sig := make(chan os.Signal, 1)
		signal.Notify(sig, os.Interrupt)
		defer signal.Stop(sig)

		var pending atomic.Bool
		postStep := func() {
			defer pending.Store(false)
			d.stressQueued.Store(false)
			d.lastStressUnixNano.Store(time.Now().UnixNano())
			cur := int(d.stressStep.Add(1))
			d.stepStressWorkload(cur)
		}
		if d.stressRunning.Load() {
			pending.Store(true)
			d.app.Post(postStep)
		}
		for {
			select {
			case <-stop:
				return
			case <-sig:
				d.app.Quit()
				return
			case <-endTimer:
				d.app.Quit()
				return
			case <-ticker.C:
				if !d.stressRunning.Load() {
					continue
				}
				if d.flags.interval <= 20*time.Millisecond {
					last := d.lastStressUnixNano.Load()
					if last != 0 && time.Since(time.Unix(0, last)) < 80*time.Millisecond {
						continue
					}
				}
				if !pending.CompareAndSwap(false, true) {
					continue
				}
				d.app.Post(postStep)
			}
		}
	}()

	runErr := d.app.Run()
	close(stop)
	return runErr
}
