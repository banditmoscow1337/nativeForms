package main

import (
	"bufio"
	"fmt"
	"image"
	"image/color"
	"math"
	"net/http"
	"net/http/pprof"
	"os"
	"runtime"
	rpprof "runtime/pprof"
	"runtime/trace"
	"strconv"
	"time"

	ui "github.com/banditmoscow1337/nativeForms"
)

func makeProceduralIcon(size int, primary, secondary color.NRGBA) *ui.ImageResource {
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	center := float64(size) / 2
	radius := center - 2
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			dx := float64(x) + 0.5 - center
			dy := float64(y) + 0.5 - center
			dist := math.Hypot(dx, dy)
			if dist <= radius {
				t := dist / radius
				r := uint8(float64(primary.R)*(1-t) + float64(secondary.R)*t)
				g := uint8(float64(primary.G)*(1-t) + float64(secondary.G)*t)
				b := uint8(float64(primary.B)*(1-t) + float64(secondary.B)*t)
				img.SetRGBA(x, y, color.RGBA{R: r, G: g, B: b, A: 255})
			}
		}
	}
	res, err := ui.NewImageResource(img)
	if err != nil {
		panic(err)
	}
	return res
}

func makeCheckerboardResource(w, h int, c1, c2 color.NRGBA) *ui.ImageResource {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	r1 := color.RGBA{R: c1.R, G: c1.G, B: c1.B, A: c1.A}
	r2 := color.RGBA{R: c2.R, G: c2.G, B: c2.B, A: c2.A}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			cell := ((x / 12) + (y / 12)) % 2
			if cell == 0 {
				img.SetRGBA(x, y, r1)
			} else {
				img.SetRGBA(x, y, r2)
			}
		}
	}
	res, err := ui.NewImageResource(img)
	if err != nil {
		panic(err)
	}
	return res
}

func buildLightTheme() ui.Theme {
	t := ui.LightTheme()
	t.FontSize = 16
	t.ControlHeight = 38
	t.CornerRadius = 6
	return t
}

func darkSyntaxPalette() map[ui.CodeStyle]ui.Color {
	return map[ui.CodeStyle]ui.Color{
		ui.CodeKeyword:  ui.RGBA(240, 180, 70, 255),
		ui.CodeFunction: ui.RGBA(110, 210, 245, 255),
		ui.CodeType:     ui.RGBA(135, 225, 165, 255),
		ui.CodeString:   ui.RGBA(180, 230, 140, 255),
		ui.CodeComment:  ui.RGBA(140, 155, 175, 255),
		ui.CodeNumber:   ui.RGBA(215, 165, 240, 255),
	}
}

func lightSyntaxPalette() map[ui.CodeStyle]ui.Color {
	return map[ui.CodeStyle]ui.Color{
		ui.CodeKeyword:  ui.RGBA(175, 75, 0, 255),
		ui.CodeFunction: ui.RGBA(0, 95, 175, 255),
		ui.CodeType:     ui.RGBA(15, 120, 65, 255),
		ui.CodeString:   ui.RGBA(35, 115, 30, 255),
		ui.CodeComment:  ui.RGBA(105, 115, 130, 255),
		ui.CodeNumber:   ui.RGBA(135, 40, 170, 255),
	}
}

func (d *demoApp) startProfilers() error {
	if d.flags.pprofAddr != "" {
		mux := http.NewServeMux()
		mux.HandleFunc("/debug/pprof/", pprof.Index)
		mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
		mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
		mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
		mux.HandleFunc("/debug/pprof/trace", pprof.Trace)
		go func() {
			d.logf("Запущен HTTP pprof сервер на http://%s/debug/pprof/", d.flags.pprofAddr)
			if err := http.ListenAndServe(d.flags.pprofAddr, mux); err != nil {
				fmt.Fprintf(os.Stderr, "pprof http error: %v\n", err)
			}
		}()
	}
	if d.flags.blockProf != "" {
		runtime.SetBlockProfileRate(1)
	}
	if d.flags.mutexProf != "" {
		runtime.SetMutexProfileFraction(1)
	}
	if d.flags.cpuProfile != "" {
		f, err := os.Create(d.flags.cpuProfile)
		if err != nil {
			return fmt.Errorf("create cpuprofile: %w", err)
		}
		if err := rpprof.StartCPUProfile(f); err != nil {
			f.Close()
			return fmt.Errorf("start cpuprofile: %w", err)
		}
		d.cpuProfFile = f
		d.logf("Запись CPU-профиля в %s", d.flags.cpuProfile)
	}
	if d.flags.traceFile != "" {
		f, err := os.Create(d.flags.traceFile)
		if err != nil {
			return fmt.Errorf("create trace: %w", err)
		}
		if err := trace.Start(f); err != nil {
			f.Close()
			return fmt.Errorf("start trace: %w", err)
		}
		d.traceOutFile = f
		d.logf("Запись runtime/trace в %s", d.flags.traceFile)
	}
	if d.flags.jsonlFile != "" {
		f, err := os.Create(d.flags.jsonlFile)
		if err != nil {
			return fmt.Errorf("create jsonl: %w", err)
		}
		d.jsonlFile = f
		d.jsonlWriter = bufio.NewWriterSize(f, 32*1024)
		buf := d.jsonlBuf[:0]
		buf = append(buf, '{')
		buf = appendJSONStringField(buf, "Kind", "manifest", true)
		buf = appendJSONStringField(buf, "Scenario", "full-showcase", true)
		buf = appendJSONStringField(buf, "Pattern", "warm", true)
		buf = appendJSONStringField(buf, "Graphics", d.flags.graphics, true)
		buf = appendJSONStringField(buf, "GoVersion", runtime.Version(), true)
		buf = appendJSONStringField(buf, "OS", runtime.GOOS, true)
		buf = appendJSONStringField(buf, "Arch", runtime.GOARCH, true)
		buf = appendJSONIntField(buf, "Width", int64(d.flags.width), true)
		buf = appendJSONIntField(buf, "Height", int64(d.flags.height), true)
		buf = appendJSONIntField(buf, "Count", int64(d.flags.count), true)
		buf = append(buf, `"Started":"`...)
		buf = time.Now().UTC().AppendFormat(buf, time.RFC3339Nano)
		buf = append(buf, "\"}\n"...)
		d.jsonlBuf = buf
		_, _ = d.jsonlWriter.Write(buf)
		_ = d.jsonlWriter.Flush()
	}
	return nil
}

func (d *demoApp) stopProfilers() error {
	if d.cpuProfFile != nil {
		rpprof.StopCPUProfile()
		_ = d.cpuProfFile.Close()
		d.cpuProfFile = nil
	}
	if d.traceOutFile != nil {
		trace.Stop()
		_ = d.traceOutFile.Close()
		d.traceOutFile = nil
	}
	if d.flags.memProfile != "" {
		if err := writeHeapProfile(d.flags.memProfile); err != nil {
			return fmt.Errorf("write memprofile: %w", err)
		}
	}
	if d.flags.blockProf != "" {
		if err := writeNamedProfile("block", d.flags.blockProf); err != nil {
			return fmt.Errorf("write blockprofile: %w", err)
		}
	}
	if d.flags.mutexProf != "" {
		if err := writeNamedProfile("mutex", d.flags.mutexProf); err != nil {
			return fmt.Errorf("write mutexprofile: %w", err)
		}
	}
	if d.jsonlWriter != nil {
		buf := d.jsonlBuf[:0]
		buf = append(buf, `{"kind":"summary","dropped_frames":0,"frames":`...)
		buf = strconv.AppendUint(buf, d.framesTotal, 10)
		buf = append(buf, `,"ended":"`...)
		buf = time.Now().UTC().AppendFormat(buf, time.RFC3339Nano)
		buf = append(buf, "\"}\n"...)
		d.jsonlBuf = buf
		_, _ = d.jsonlWriter.Write(buf)
		_ = d.jsonlWriter.Flush()
		_ = d.jsonlFile.Close()
	}
	return nil
}
