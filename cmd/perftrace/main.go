// Command perftrace records real native window frames as JSONL. See
// docs/performance-baseline.md for comparable target-machine runs.
package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"runtime/debug"
	"strings"
	"sync/atomic"
	"time"

	ui "github.com/banditmoscow1337/nativeForms"
	"github.com/banditmoscow1337/nativeForms/editor"
	"github.com/banditmoscow1337/nativeForms/platform/desktop"
)

type manifest struct {
	Kind, Scenario, Pattern, Revision, GoVersion, OS, Arch string
	OSVersion, CPU, GPU, PowerProfile, Display             string
	Count, Width, Height                                   int
	Duration, Interval                                     string
	Started                                                time.Time
}

type frame struct {
	Kind, Scenario string
	At             time.Time
	Sample         ui.FrameSample
}

type action struct {
	Kind, Scenario string
	At             time.Time
	Step           int
	DurationNS     int64
}

type workload struct {
	root   ui.Component
	scroll *ui.ScrollPanel
	field  *ui.TextField
	code   *ui.CodeEditor
	count  int
}

func build(name string, count int) (workload, error) {
	w := workload{count: count}
	switch name {
	case "widgets", "form":
		rows := ui.NewPanel(ui.Vertical)
		for i := 0; i < count; i++ {
			if name == "form" {
				rows.Add(ui.NewTextField("Field value 0123456789", nil))
			} else {
				rows.Add(ui.NewLabel("Record 12345: sample text"))
			}
		}
		w.scroll = ui.NewScrollPanel(rows)
		w.root = w.scroll
	case "textarea":
		w.field = ui.NewTextArea(strings.Repeat("A long line of editable text 0123456789\n", count), nil)
		w.root = w.field
	case "editor":
		doc, err := editor.NewDocument(strings.Repeat("const x = 12345; // source line\n", count))
		if err != nil {
			return w, err
		}
		w.code = ui.NewCodeEditor(doc)
		w.root = w.code
	case "paragraph":
		w.field = ui.NewTextArea(strings.Repeat("wide text with spaces ", count), nil)
		w.root = w.field
	default:
		return w, fmt.Errorf("unknown scenario %q", name)
	}
	return w, nil
}

func (w workload) tick(manager *ui.Manager, pattern string, step int) {
	if w.scroll != nil {
		w.scroll.Scroll(72)
		if step%40 == 0 {
			w.scroll.Scroll(-1800)
		}
	}
	if w.code != nil {
		line := 90000 + step%12
		if pattern == "cold" {
			line = step * 7919 % w.count
		}
		w.code.ScrollToLine(min(w.count-1, line))
	}
	if w.field != nil {
		if step == 1 {
			manager.HandleEvent(ui.Event{Type: ui.PointerDown, Button: ui.MouseLeft, X: 40, Y: 40})
			manager.HandleEvent(ui.Event{Type: ui.PointerUp, Button: ui.MouseLeft, X: 40, Y: 40})
		}
		// This captures platform surrounding-text costs as well as paint.
		_, _ = manager.TextInputState()
		if step%20 == 0 {
			manager.HandleEvent(ui.Event{Type: ui.TextInput, Text: "x"})
		}
	}
}

func revision() string {
	if info, ok := debug.ReadBuildInfo(); ok {
		for _, setting := range info.Settings {
			if setting.Key == "vcs.revision" {
				return setting.Value
			}
		}
	}
	return "unknown"
}

func run() error {
	scenario := flag.String("scenario", "widgets", "widgets, form, textarea, editor, paragraph")
	pattern := flag.String("pattern", "warm", "editor jumps: warm or cold")
	count := flag.Int("count", 1000, "number of widgets, fields, lines or paragraph fragments")
	duration := flag.Duration("duration", 60*time.Second, "measurement time")
	interval := flag.Duration("interval", 100*time.Millisecond, "scripted interaction period")
	output := flag.String("output", "frames.jsonl", "JSONL output")
	backend := flag.String("backend", "", "Linux: x11 or wayland (empty: session default)")
	osVersion := flag.String("os-version", "", "target OS version")
	cpu := flag.String("cpu", "", "target CPU")
	gpu := flag.String("gpu", "", "target GPU and driver")
	power := flag.String("power", "", "power profile")
	display := flag.String("display", "", "resolution, scale, font and theme")
	allocations := flag.Bool("allocations", false, "sample runtime allocations (separate run from latency)")
	flag.Parse()
	if *count < 1 || *interval <= 0 || *duration <= 0 || (*pattern != "warm" && *pattern != "cold") {
		return fmt.Errorf("count, duration, interval and pattern must be valid")
	}
	w, err := build(*scenario, *count)
	if err != nil {
		return err
	}
	file, err := os.Create(*output)
	if err != nil {
		return err
	}
	defer file.Close()
	manager := ui.New()
	manager.SetRoot(w.root)
	app := desktop.NewApp()
	window, err := app.NewWindow(manager, desktop.Options{Title: "nativeForms perftrace", Width: 1024, Height: 768, Backend: *backend})
	if err != nil {
		return err
	}
	lines := make(chan any, 8192)
	writerDone := make(chan error, 1)
	meta := manifest{Kind: "manifest", Scenario: *scenario, Pattern: *pattern, Revision: revision(),
		GoVersion: runtime.Version(), OS: runtime.GOOS, Arch: runtime.GOARCH, OSVersion: *osVersion,
		CPU: *cpu, GPU: *gpu, PowerProfile: *power, Display: *display,
		Count: *count, Width: 1024, Height: 768, Duration: duration.String(), Interval: interval.String(), Started: time.Now().UTC()}
	go func() {
		writer := bufio.NewWriterSize(file, 256*1024)
		encoder := json.NewEncoder(writer)
		writeErr := encoder.Encode(meta)
		for line := range lines {
			if writeErr == nil {
				writeErr = encoder.Encode(line)
			}
		}
		if err := writer.Flush(); writeErr == nil {
			writeErr = err
		}
		writerDone <- writeErr
	}()
	var dropped atomic.Uint64
	var droppedActions atomic.Uint64
	var skippedActions atomic.Uint64
	var pending atomic.Bool
	manager.SetFrameObserver(func(sample ui.FrameSample) {
		select {
		case lines <- frame{Kind: "frame", Scenario: *scenario, At: time.Now().UTC(), Sample: sample}:
		default:
			dropped.Add(1)
		}
	}, ui.FrameObserverOptions{TrackAllocations: *allocations})
	stop := make(chan struct{})
	go func() {
		ticker := time.NewTicker(*interval)
		defer ticker.Stop()
		timer := time.NewTimer(*duration)
		defer timer.Stop()
		interrupt := make(chan os.Signal, 1)
		signal.Notify(interrupt, os.Interrupt)
		defer signal.Stop(interrupt)
		step := 0
		for {
			select {
			case <-stop:
				return
			case <-timer.C:
				app.Quit()
				return
			case <-interrupt:
				app.Quit()
				return
			case <-ticker.C:
				if !pending.CompareAndSwap(false, true) {
					skippedActions.Add(1)
					continue
				}
				step++
				current := step
				app.Post(func() {
					defer pending.Store(false)
					started := time.Now()
					w.tick(manager, *pattern, current)
					select {
					case lines <- action{Kind: "action", Scenario: *scenario, At: time.Now().UTC(), Step: current, DurationNS: time.Since(started).Nanoseconds()}:
					default:
						droppedActions.Add(1)
					}
					if current%30 == 0 {
						window.Hide()
					} else if current%30 == 1 {
						window.Show()
					}
					if current%50 == 0 {
						_ = window.SetSize(800, 600)
					} else if current%50 == 1 {
						_ = window.SetSize(1024, 768)
					}
				})
			}
		}
	}()
	runErr := app.Run()
	close(stop)
	lines <- map[string]any{"kind": "summary", "dropped_frames": dropped.Load(), "dropped_actions": droppedActions.Load(), "skipped_actions": skippedActions.Load(), "ended": time.Now().UTC()}
	close(lines)
	if err := <-writerDone; err != nil {
		return err
	}
	return runErr
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "perftrace:", err)
		os.Exit(1)
	}
}
