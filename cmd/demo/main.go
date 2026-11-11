package main

import (
	"bufio"
	"flag"
	"fmt"
	"image/color"
	"os"
	"runtime"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"time"

	ui "github.com/banditmoscow1337/nativeForms"
	"github.com/banditmoscow1337/nativeForms/editor"
	"github.com/banditmoscow1337/nativeForms/platform"
	"github.com/banditmoscow1337/nativeForms/platform/desktop"
)

type demoFlags struct {
	backend            string
	graphics           string
	width              int
	height             int
	scale              float64
	frameless          bool
	hideWindowControls bool
	headless           bool
	stress             bool
	duration           time.Duration
	warmup             time.Duration
	interval           time.Duration
	count              int
	trackAllocs        bool
	cpuProfile         string
	memProfile         string
	blockProf          string
	mutexProf          string
	traceFile          string
	jsonlFile          string
	pprofAddr          string
	screenshot         string
}

type demoApp struct {
	flags       demoFlags
	manager     *ui.Manager
	app         *desktop.App
	window      desktop.Window
	secondWin   desktop.Window
	fonts       *ui.FontContext
	darkTheme   ui.Theme
	lightTheme  ui.Theme
	isLight     bool
	icons       []*ui.ImageResource
	bannerImg   *ui.ImageResource
	altBanner   *ui.ImageResource
	usingAltImg bool

	// Main navigation, MenuBar & status
	menuBar     *ui.MenuBar
	header      *ui.Panel
	footer      *ui.Panel
	tabs        *ui.Tabs
	statusLabel *ui.Label
	perfBanner  *ui.Label

	// Tab 1: Controls, ColorPicker, SpinBox, SegmentedControl, Toolbar, Accordion, CornerRadius, AutoSize, SplitPane & Forms
	clickCount        int
	autoTextStep      int
	controlsSplit     *ui.SplitPane
	topToolbar        *ui.Toolbar
	statusBadge       *ui.Badge
	btnCounter        *ui.Button
	toggleSwitch      *ui.Toggle
	framelessCtrlTgl  *ui.Toggle
	radiusSlider      *ui.Slider
	spinBox           *ui.SpinBox
	segmentedCtrl     *ui.SegmentedControl
	labeledSep        *ui.Separator
	accordion         *ui.Accordion
	docHyperlink      *ui.Hyperlink
	indeterminateTgl  *ui.Toggle
	autoSizeToggle    *ui.Toggle
	autoSizeBtn       *ui.Button
	autoSizeLabel     *ui.Label
	autoSizeField     *ui.TextField
	colorPicker       *ui.ColorPicker
	compactPicker     *ui.ColorPicker
	checkBox          *ui.Checkbox
	radioGroup        *ui.RadioGroup
	comboBox          *ui.ComboBox
	sliderStep        *ui.Slider
	sliderCont        *ui.Slider
	progressBar       *ui.ProgressBar
	overflowLabel     *ui.Label
	overflowBtn       *ui.Button
	imageWidget       *ui.Image
	canvasWidget      *customCanvasWidget
	validField        *ui.TextField
	passwordField     *ui.TextField
	readOnlyField     *ui.TextField
	notesArea         *ui.TextField
	scrollForm        *ui.ScrollPanel
	dynamicGrid       *ui.Grid
	overlayBadge      *ui.Label
	suspendedNotif    *ui.NotificationState
	framelessControls []ui.Component
	roundedControls   []ui.Component
	autoSizeControls  []ui.Component

	// Tab 2: CodeEditor & Text
	codeDoc    *editor.Document
	codeEditor *ui.CodeEditor
	editorInfo *ui.Label

	// Tab 3: Virtualized Views & Output
	virtualSplit *ui.SplitPane
	vListModel   *demoListModel
	vList        *ui.VirtualList
	vTableModel  *demoTableModel
	vTable       *ui.VirtualTable
	vTreeModel   *demoTreeModel
	vTree        *ui.VirtualTree
	vStackModel  *demoStackModel
	vStack       *ui.VirtualStack
	vStackScroll *ui.ScrollPanel
	outputPanel  *ui.OutputPanel

	// Tab 4: Popups, System Tray, Window Frame, A11y & Platform
	trayToggle         *ui.Toggle
	trayActive         bool
	trayClicks         int
	winFramelessToggle *ui.Toggle
	winControlsToggle  *ui.Toggle
	closePolicyRadio   *ui.RadioGroup
	closePolicy        platform.CloseAction
	unfocusedPopupID   ui.PopupID
	scopedShortcutID   ui.ShortcutID
	a11ySummary        *ui.Label
	capSummary         *ui.Label
	clipField          *ui.TextField

	// Tab 5: CPU & Memory Profiling
	trackAllocsToggle *ui.Toggle
	stressToggle      *ui.Toggle

	// Profiling & telemetry state (throttled text formatting to keep FontContext cache warm)
	mu                 sync.Mutex
	framesTotal        uint64
	lastSample         ui.FrameSample
	maxBuild           time.Duration
	maxPaint           time.Duration
	maxLayout          time.Duration
	sumBuildNS         uint64
	allocBytesSum      uint64
	allocObjSum        uint64
	bannerCache        throttledText
	card1Cache         throttledText
	card2Cache         throttledText
	card3Cache         throttledText
	stressRunning      atomic.Bool
	stressWarmupDone   atomic.Bool
	stressQueued       atomic.Bool
	lastStressUnixNano atomic.Int64
	uiStressStep       func()
	stressStep         atomic.Int64
	cpuProfFile        *os.File
	traceOutFile       *os.File
	jsonlWriter        *bufio.Writer
	jsonlFile          *os.File
	jsonlBuf           []byte
}

func newDemoApp(flags demoFlags) *demoApp {
	dark := ui.DefaultTheme()
	dark.FontSize = 16
	dark.ControlHeight = 38
	dark.CornerRadius = 6

	mgr := ui.New()
	mgr.SetTheme(dark)
	mgr.SetInteractive(true)
	mgr.SetDisplayScale(float32(flags.scale))

	fonts := ui.NewSystemFontContext()
	fonts.SetCacheLimit(512 << 10)
	fonts.SetLayoutCacheLimit(256 << 10)
	mgr.SetFontContext(fonts)

	icons := []*ui.ImageResource{
		makeProceduralIcon(20, color.NRGBA{R: 235, G: 180, B: 60, A: 255}, color.NRGBA{R: 180, G: 90, B: 20, A: 255}),
		makeProceduralIcon(20, color.NRGBA{R: 70, G: 175, B: 240, A: 255}, color.NRGBA{R: 25, G: 95, B: 170, A: 255}),
		makeProceduralIcon(20, color.NRGBA{R: 90, G: 210, B: 130, A: 255}, color.NRGBA{R: 30, G: 130, B: 65, A: 255}),
		makeProceduralIcon(20, color.NRGBA{R: 230, G: 95, B: 95, A: 255}, color.NRGBA{R: 150, G: 35, B: 35, A: 255}),
	}
	banner := makeCheckerboardResource(144, 72,
		color.NRGBA{R: 50, G: 110, B: 190, A: 255},
		color.NRGBA{R: 225, G: 175, B: 60, A: 255},
	)
	altBanner := makeCheckerboardResource(144, 72,
		color.NRGBA{R: 60, G: 175, B: 115, A: 255},
		color.NRGBA{R: 35, G: 45, B: 65, A: 255},
	)

	d := &demoApp{
		flags:       flags,
		manager:     mgr,
		fonts:       fonts,
		darkTheme:   dark,
		lightTheme:  buildLightTheme(),
		icons:       icons,
		bannerImg:   banner,
		altBanner:   altBanner,
		closePolicy: platform.QuitApp,
		jsonlBuf:    make([]byte, 0, 512),
	}

	var fallbackClip string
	mgr.SetClipboardHandlers(
		func() (string, error) {
			if d.window != nil && d.window.Capabilities().Supports(platform.FeatureClipboard) {
				return d.window.ClipboardText()
			}
			return fallbackClip, nil
		},
		func(s string) error {
			fallbackClip = s
			if d.window != nil && d.window.Capabilities().Supports(platform.FeatureClipboard) {
				return d.window.SetClipboardText(s)
			}
			return nil
		},
	)

	d.setupObserver(flags.trackAllocs)
	d.buildUI()
	d.registerShortcuts()
	// Reclaim transient font-parsing buffers immediately after startup.
	runtime.GC()
	debug.FreeOSMemory()
	return d
}

func (d *demoApp) logf(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	if d.statusLabel != nil {
		d.statusLabel.SetText("Status: " + msg)
	}
	if d.outputPanel != nil {
		ts := time.Now().Format("15:04:05.000")
		d.outputPanel.AppendText("[" + ts + "] " + msg + "\n")
	}
}

func parseFlags() demoFlags {
	var f demoFlags
	flag.StringVar(&f.backend, "backend", "", "Linux window backend: '', 'x11', or 'wayland'")
	flag.StringVar(&f.graphics, "graphics", "auto", "Renderer backend: 'auto', 'hardware', or 'software'")
	flag.IntVar(&f.width, "width", 1280, "Logical window width")
	flag.IntVar(&f.height, "height", 820, "Logical window height")
	flag.Float64Var(&f.scale, "scale", 1.0, "Initial display scale factor (1.0 - 4.0)")
	flag.BoolVar(&f.frameless, "frameless", false, "Start window without standard OS titlebar/frame")
	flag.BoolVar(&f.hideWindowControls, "hide-window-controls", false, "Hide standard OS window control buttons")
	flag.BoolVar(&f.headless, "headless", false, "Run offscreen using software renderer without an OS window")
	flag.BoolVar(&f.stress, "stress", false, "Automatically run stress interactions across all tabs")
	flag.DurationVar(&f.duration, "duration", 15*time.Second, "Duration for headless or automated stress run")
	flag.DurationVar(&f.warmup, "warmup", 1*time.Second, "Warmup delay before automated desktop stress loop")
	flag.DurationVar(&f.interval, "interval", 16*time.Millisecond, "Stress interaction period")
	flag.IntVar(&f.count, "count", 500, "Initial row count for VirtualList, VirtualTable, and VirtualStack")
	flag.BoolVar(&f.trackAllocs, "track-allocs", false, "Sample per-frame runtime allocations via FrameObserverOptions")
	flag.StringVar(&f.cpuProfile, "cpuprofile", "", "Write pprof CPU profile to file")
	flag.StringVar(&f.memProfile, "memprofile", "", "Write pprof heap memory profile to file on exit")
	flag.StringVar(&f.blockProf, "blockprofile", "", "Write goroutine blocking profile to file on exit")
	flag.StringVar(&f.mutexProf, "mutexprofile", "", "Write mutex contention profile to file on exit")
	flag.StringVar(&f.traceFile, "trace", "", "Write runtime/trace execution trace to file")
	flag.StringVar(&f.jsonlFile, "jsonl", "", "Write per-frame FrameSample JSONL log (compatible with tools/summarize_perftrace.py)")
	flag.StringVar(&f.pprofAddr, "pprof-addr", "", "Start net/http/pprof server at address (e.g. localhost:6060)")
	flag.StringVar(&f.screenshot, "screenshot", "", "Save a rendered PNG screenshot in headless mode")
	flag.Parse()
	return f
}

func main() {
	debug.SetMemoryLimit(32 << 20)
	flags := parseFlags()
	demo := newDemoApp(flags)

	if err := demo.startProfilers(); err != nil {
		fmt.Fprintf(os.Stderr, "demo profiler init: %v\n", err)
		os.Exit(1)
	}

	var runErr error
	if flags.headless {
		runErr = demo.runHeadless()
	} else {
		runErr = demo.runDesktop()
	}

	if err := demo.stopProfilers(); err != nil && runErr == nil {
		runErr = err
	}
	if runErr != nil {
		fmt.Fprintf(os.Stderr, "demo: %v\n", runErr)
		os.Exit(1)
	}
}
