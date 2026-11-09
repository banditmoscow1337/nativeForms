// Command demo showcases all widgets, layouts, editors, virtualized views,
// popups, accessibility APIs, platform integrations, and CPU/memory profiling
// capabilities of github.com/banditmoscow1337/nativeForms.
//
// Usage examples:
//
//	# Interactive desktop showcase window:
//	go run ./cmd/demo
//
//	# Interactive run with CPU, Heap, Execution Trace, and pprof HTTP server:
//	go run ./cmd/demo -cpuprofile cpu.pprof -memprofile mem.pprof -trace trace.out -pprof-addr localhost:6060 -track-allocs
//
//	# Automated stress-test in a desktop window for 30s with JSONL frame log:
//	go run ./cmd/demo -stress -duration 30s -interval 16ms -jsonl frames.jsonl -cpuprofile cpu.pprof -memprofile mem.pprof
//
//	# Headless verification & profiling (no X11/Wayland/macOS/Windows display needed):
//	go run ./cmd/demo -headless -stress -duration 10s -cpuprofile cpu.pprof -memprofile mem.pprof -screenshot demo_frame.png
package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"runtime"
	"runtime/debug"
	"runtime/pprof"
	"runtime/trace"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	ui "github.com/banditmoscow1337/nativeForms"
	"github.com/banditmoscow1337/nativeForms/editor"
	"github.com/banditmoscow1337/nativeForms/platform"
	"github.com/banditmoscow1337/nativeForms/platform/desktop"
	"github.com/banditmoscow1337/nativeForms/render"
	"github.com/banditmoscow1337/nativeForms/software"
)

// demoFlags holds command-line configuration for display, stress testing, and profiling.
type demoFlags struct {
	backend     string
	graphics    string
	width       int
	height      int
	scale       float64
	headless    bool
	stress      bool
	duration    time.Duration
	warmup      time.Duration
	interval    time.Duration
	count       int
	trackAllocs bool
	cpuProfile  string
	memProfile  string
	blockProf   string
	mutexProf   string
	traceFile   string
	jsonlFile   string
	pprofAddr   string
	screenshot  string
}

// customCanvasWidget demonstrates raw Canvas drawing primitives (CommandQuad,
// CommandRoundedRect, CommandLine, CommandImage, CommandMask, PushClip/PopClip,
// Tessellate) and the ViewportCuller interface without per-frame allocations.
type customCanvasWidget struct {
	ui.State
	icon        *ui.ImageResource
	phase       float32
	vertices    int
	cullCount   int
	sampleCmds  [2]ui.DrawCommand
	sampleVerts []ui.Vertex
	cachedPhase float32
	cachedCull  int
	cachedText  string
}

func newCustomCanvasWidget(icon *ui.ImageResource) *customCanvasWidget {
	w := &customCanvasWidget{
		icon:        icon,
		sampleVerts: make([]ui.Vertex, 0, 128),
		cachedPhase: -1,
	}
	w.UIState().SetPreferredSize(ui.Vec2{X: 360, Y: 130})
	w.UIState().SetMinimumSize(ui.Vec2{X: 220, Y: 110})
	w.UIState().SetTooltip("Custom Canvas widget: Rect, RoundedRect, Border, Line, Image, TextStyled, Clip, Tessellate & ViewportCuller")
	return w
}

func (w *customCanvasWidget) Cull(viewport ui.Rect) bool {
	b := w.Bounds()
	outside := b.X+b.W <= viewport.X || b.X >= viewport.X+viewport.W ||
		b.Y+b.H <= viewport.Y || b.Y >= viewport.Y+viewport.H
	if outside {
		w.cullCount++
	}
	return outside
}

func (w *customCanvasWidget) Paint(canvas *ui.Canvas) {
	r := w.Bounds()
	if r.Empty() {
		return
	}
	theme := canvas.Theme()
	canvas.Rect(r, theme.PanelAlt)
	canvas.Border(r, 1, theme.Border)

	inner := r.Inset(ui.All(10))
	canvas.PushClip(inner)
	defer canvas.PopClip()

	// Rounded rectangles with dynamic accent colors.
	badgeRect := ui.Rect{X: inner.X, Y: inner.Y, W: 140, H: 36}
	canvas.RoundedRect(badgeRect, 10, theme.Accent.WithAlpha(0.25))
	canvas.TextStyled("Canvas Primitives", badgeRect, ui.FontSpec{Size: 14, Weight: 600}, ui.TextCenter, theme.Text)

	// Animated sine wave using Canvas.Line.
	waveLeft := inner.X + 154
	waveWidth := maxFloat32(40, inner.W-160)
	midY := inner.Y + 24
	prev := ui.Vec2{X: waveLeft, Y: midY}
	segments := 24
	for i := 1; i <= segments; i++ {
		t := float32(i) / float32(segments)
		x := waveLeft + t*waveWidth
		y := midY + float32(math.Sin(float64(t*6.28318+w.phase)))*14
		next := ui.Vec2{X: x, Y: y}
		canvas.Line(prev, next, 2.0, theme.AccentHover)
		prev = next
	}

	// Render ImageResource with source crop and tint.
	if w.icon != nil {
		imgDest := ui.Rect{X: inner.X, Y: inner.Y + 46, W: 48, H: 48}
		canvas.Image(w.icon, imgDest, ui.Rect{}, ui.RGBA(255, 255, 255, 255), ui.FilterBilinear)
	}

	// Verify Tessellate reusing pre-allocated vertex buffer.
	w.sampleCmds[0] = ui.DrawCommand{Kind: ui.CommandRoundedRect, Rect: ui.Rect{W: 32, H: 16}, Clip: ui.Rect{W: 32, H: 16}, Radius: 4, Tint: theme.Accent}
	w.sampleCmds[1] = ui.DrawCommand{Kind: ui.CommandLine, From: ui.Vec2{X: 0, Y: 0}, To: ui.Vec2{X: 32, Y: 16}, Width: 2, Clip: ui.Rect{W: 32, H: 16}, Tint: theme.Text}
	w.sampleVerts = ui.Tessellate(w.sampleCmds[:], w.sampleVerts[:0])
	w.vertices = len(w.sampleVerts)

	if w.cachedText == "" || w.cachedPhase != w.phase || w.cachedCull != w.cullCount {
		w.cachedPhase = w.phase
		w.cachedCull = w.cullCount
		w.cachedText = fmt.Sprintf("Phase: %.2f rad | Sample Tessellate: %d verts | Culled: %d", w.phase, w.vertices, w.cullCount)
	}
	infoRect := ui.Rect{X: inner.X + 58, Y: inner.Y + 48, W: maxFloat32(0, inner.W-60), H: 48}
	canvas.Text(w.cachedText, infoRect, 14, ui.TextLeft, theme.MutedText)
}

// --- Virtual Models (ListModel, ListIconModel, TableModel, TableIconModel, TreeModel, TreeIconModel, StackModel, StackModelUpdater) ---

type demoListModel struct {
	count  int
	icons  []*ui.ImageResource
	ids    []string
	labels []string
}

func newDemoListModel(count int, icons []*ui.ImageResource) *demoListModel {
	m := &demoListModel{icons: icons}
	m.resize(count)
	return m
}

func (m *demoListModel) resize(count int) {
	m.count = count
	for len(m.ids) < count {
		i := len(m.ids)
		m.ids = append(m.ids, strconv.Itoa(i))
		m.labels = append(m.labels, fmt.Sprintf("Item #%04d — VirtualList Row (Icon=%v)", i, i%2 == 0))
	}
}

func (m *demoListModel) Len() int          { return m.count }
func (m *demoListModel) ID(i int) string   { return m.ids[i] }
func (m *demoListModel) Label(i int) string { return m.labels[i] }
func (m *demoListModel) Icon(i int) *ui.ImageResource {
	if len(m.icons) == 0 || i%2 != 0 {
		return nil
	}
	return m.icons[i%len(m.icons)]
}

type demoTableModel struct {
	rows  int
	icons []*ui.ImageResource
	ids   []string
	cells [][5]string
}

func newDemoTableModel(rows int, icons []*ui.ImageResource) *demoTableModel {
	m := &demoTableModel{icons: icons}
	m.resize(rows)
	return m
}

func (m *demoTableModel) resize(rows int) {
	m.rows = rows
	names := [...]string{"Label", "Button", "TextField", "CodeEditor", "VirtualTable", "VirtualTree", "VirtualStack", "Slider"}
	for len(m.ids) < rows {
		r := len(m.ids)
		m.ids = append(m.ids, fmt.Sprintf("row-%d", r))
		status := "OK"
		if r%7 == 0 {
			status = "WARN"
		}
		m.cells = append(m.cells, [5]string{
			fmt.Sprintf("ROW-%04d", r),
			names[r%len(names)],
			status,
			fmt.Sprintf("%d us", 45+(r*17)%250),
			fmt.Sprintf("%d KB", 12+(r*31)%512),
		})
	}
}

func (m *demoTableModel) Len() int        { return m.rows }
func (m *demoTableModel) ID(r int) string { return m.ids[r] }
func (m *demoTableModel) Columns() int    { return 5 }
func (m *demoTableModel) Header(col int) string {
	headers := [...]string{"ID", "Component", "Status", "Latency (us)", "Memory (KB)"}
	if col >= 0 && col < len(headers) {
		return headers[col]
	}
	return ""
}
func (m *demoTableModel) Cell(row, col int) string {
	if row >= 0 && row < len(m.cells) && col >= 0 && col < 5 {
		return m.cells[row][col]
	}
	return ""
}
func (m *demoTableModel) CellIcon(row, col int) *ui.ImageResource {
	if col == 0 && len(m.icons) > 0 {
		return m.icons[row%len(m.icons)]
	}
	return nil
}

type demoTreeModel struct {
	roots    []string
	children map[string][]string
	labels   map[string]string
	icons    []*ui.ImageResource
}

func newDemoTreeModel(icons []*ui.ImageResource) *demoTreeModel {
	return &demoTreeModel{
		roots: []string{"root-widgets", "root-virtual", "root-editors", "root-platform"},
		children: map[string][]string{
			"root-widgets":  {"w-controls", "w-layouts", "w-popups"},
			"w-controls":    {"w-button", "w-toggle", "w-checkbox", "w-radio", "w-combobox", "w-slider", "w-progress", "w-image"},
			"w-layouts":     {"w-panel", "w-stack", "w-grid", "w-form", "w-scroll", "w-tabs"},
			"w-popups":      {"w-dialog", "w-menu", "w-tooltip", "w-notify"},
			"root-virtual":  {"v-list", "v-table", "v-tree", "v-stack", "v-output"},
			"root-editors":  {"e-textfield", "e-textarea", "e-codeeditor", "e-document"},
			"root-platform": {"p-clipboard", "p-dialogs", "p-ime", "p-a11y", "p-perf"},
		},
		labels: map[string]string{
			"root-widgets":  "Core Widgets & Containers",
			"w-controls":    "Interactive Controls",
			"w-button":      "Button (Icon, FontSpec, OnClick)",
			"w-toggle":      "Toggle (ON/OFF Switch)",
			"w-checkbox":    "Checkbox (Boolean State)",
			"w-radio":       "RadioGroup (Single Choice)",
			"w-combobox":    "ComboBox (Dropdown Menu)",
			"w-slider":      "Slider (Stepped & Continuous)",
			"w-progress":    "ProgressBar (Custom Fill & Value)",
			"w-image":       "Image (Contain, Cover, Stretch, Filter)",
			"w-layouts":     "Layout Containers",
			"w-panel":       "Panel (Vertical, Horizontal, Overlay)",
			"w-stack":       "Stack & Spacer",
			"w-grid":        "Grid (TrackAuto, TrackFixed, TrackStar)",
			"w-form":        "Form (Two-Column Label + Control)",
			"w-scroll":      "ScrollPanel (2D Viewport & ScrollIntoView)",
			"w-tabs":        "Tabs (Dirty, Closable, Reorderable)",
			"w-popups":      "Overlays & Notifications",
			"w-dialog":      "Modal Dialog",
			"w-menu":        "Context & Dropdown Menu",
			"w-tooltip":     "Passive Tooltip",
			"w-notify":      "Toast Notifications (Suspend/Restore)",
			"root-virtual":  "Virtualized Views",
			"v-list":        "VirtualList (ListModel + ListIconModel)",
			"v-table":       "VirtualTable (TableModel + TableIconModel)",
			"v-tree":        "VirtualTree (TreeModel + TreeIconModel)",
			"v-stack":       "VirtualStack (StackModel + StackModelUpdater)",
			"v-output":      "OutputPanel (Streaming Log Buffer)",
			"root-editors":  "Text & Source Editors",
			"e-textfield":   "TextField (Validation, Password, ReadOnly)",
			"e-textarea":    "TextArea (Multiline + Undo/Redo)",
			"e-codeeditor":  "CodeEditor (Go Lexer, LSP Tokens, Breakpoints)",
			"e-document":    "editor.Document (Piece Tree, UTF-16 Offsets)",
			"root-platform": "Platform & Diagnostics",
			"p-clipboard":   "TextClipboard (Read/Write)",
			"p-dialogs":     "FileDialogs (OpenFile, OpenFolder, SaveFile)",
			"p-ime":         "TextInputClient (Surrounding Text, Composition)",
			"p-a11y":        "AccessibilityTree & PerformAccessibilityAction",
			"p-perf":        "FrameTrace, pprof CPU/Heap & Runtime Metrics",
		},
		icons: icons,
	}
}

func (m *demoTreeModel) Roots() int        { return len(m.roots) }
func (m *demoTreeModel) Root(i int) string { return m.roots[i] }
func (m *demoTreeModel) Label(id string) string {
	if lbl, ok := m.labels[id]; ok {
		return lbl
	}
	return id
}
func (m *demoTreeModel) Children(id string) int        { return len(m.children[id]) }
func (m *demoTreeModel) Child(id string, i int) string { return m.children[id][i] }
func (m *demoTreeModel) Icon(id string) *ui.ImageResource {
	if len(m.icons) == 0 {
		return nil
	}
	if len(m.children[id]) > 0 {
		return m.icons[0]
	}
	return m.icons[1%len(m.icons)]
}

type demoStackModel struct {
	count  int
	values map[int]string
	keys   []string
	titles []string
	prefix string
}

func newDemoStackModel(count int) *demoStackModel {
	m := &demoStackModel{
		values: make(map[int]string),
		prefix: "VirtualRow",
	}
	m.resize(count, "VirtualRow")
	return m
}

func (m *demoStackModel) resize(count int, prefix string) {
	if m.prefix != prefix {
		m.prefix = prefix
		m.titles = m.titles[:0]
	}
	m.count = count
	for len(m.keys) < count {
		i := len(m.keys)
		m.keys = append(m.keys, strconv.Itoa(i))
	}
	for len(m.titles) < count {
		i := len(m.titles)
		m.titles = append(m.titles, fmt.Sprintf("%s #%04d", m.prefix, i))
	}
}

func (m *demoStackModel) Len() int             { return m.count }
func (m *demoStackModel) Key(i int) string     { return m.keys[i] }
func (m *demoStackModel) Height(i int) float32 { return float32(44 + (i%3)*6) }
func (m *demoStackModel) Build(i int) ui.Component {
	row := ui.NewStack(ui.Horizontal, 8)
	row.Padding = ui.Symmetric(6, 2)
	lbl := ui.NewLabel(m.titles[i])
	lbl.UIState().SetPreferredSize(ui.Vec2{X: 160, Y: 36})
	val, ok := m.values[i]
	if !ok {
		val = "Editable value 0123456789"
	}
	idx := i
	field := ui.NewTextField(val, func(s string) {
		m.values[idx] = s
	})
	field.UIState().SetFlex(1)
	row.Add(lbl, field)
	return row
}
func (m *demoStackModel) Update(i int, comp ui.Component) {
	panel, ok := comp.(*ui.Panel)
	if !ok {
		return
	}
	children := panel.UIState().Children()
	if len(children) > 0 {
		if lbl, ok := children[0].(*ui.Label); ok {
			lbl.SetText(m.titles[i])
		}
	}
}

type jsonlFrameRecord struct {
	Kind, Scenario string
	At             time.Time
	Sample         ui.FrameSample
}

// --- Demo Application State ---

type demoApp struct {
	flags       demoFlags
	manager     *ui.Manager
	app         *desktop.App
	window      desktop.Window
	fonts       *ui.FontContext
	darkTheme   ui.Theme
	lightTheme  ui.Theme
	isLight     bool
	icons       []*ui.ImageResource
	bannerImg   *ui.ImageResource
	altBanner   *ui.ImageResource
	usingAltImg bool

	// Main navigation & status
	tabs        *ui.Tabs
	statusLabel *ui.Label
	perfBanner  *ui.Label

	// Tab 1: Controls & Forms
	clickCount     int
	btnCounter     *ui.Button
	toggleSwitch   *ui.Toggle
	checkBox       *ui.Checkbox
	radioGroup     *ui.RadioGroup
	comboBox       *ui.ComboBox
	sliderStep     *ui.Slider
	sliderCont     *ui.Slider
	progressBar    *ui.ProgressBar
	imageWidget    *ui.Image
	canvasWidget   *customCanvasWidget
	validField     *ui.TextField
	passwordField  *ui.TextField
	readOnlyField  *ui.TextField
	notesArea      *ui.TextField
	scrollForm     *ui.ScrollPanel
	dynamicGrid    *ui.Grid
	overlayBadge   *ui.Label
	suspendedNotif *ui.NotificationState

	// Tab 2: CodeEditor & Text
	codeDoc    *editor.Document
	codeEditor *ui.CodeEditor
	editorInfo *ui.Label

	// Tab 3: Virtualized Views & Output
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

	// Tab 4: Popups, A11y & Platform
	a11ySummary *ui.Label
	capSummary  *ui.Label
	clipField   *ui.TextField

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
	lastBannerUpdate   time.Time
	cachedBannerText   string
	lastCard1Update    time.Time
	cachedCard1Text    string
	lastCard2Update    time.Time
	cachedCard2Text    string
	lastCard3Update    time.Time
	cachedCard3Text    string
	stressRunning      atomic.Bool
	stressStep         atomic.Int64
	cpuProfFile        *os.File
	traceOutFile       *os.File
	jsonlWriter        *bufio.Writer
	jsonlFile          *os.File
	jsonlEnc           *json.Encoder
}

func makeProceduralIcon(size int, primary, secondary color.NRGBA) *ui.ImageResource {
	img := image.NewNRGBA(image.Rect(0, 0, size, size))
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
				img.SetNRGBA(x, y, color.NRGBA{R: r, G: g, B: b, A: 255})
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
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			cell := ((x / 12) + (y / 12)) % 2
			if cell == 0 {
				img.SetNRGBA(x, y, c1)
			} else {
				img.SetNRGBA(x, y, c2)
			}
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err == nil {
		if decoded, err := ui.DecodeImageResource(&buf); err == nil {
			return decoded
		}
	}
	res, err := ui.NewImageResource(img)
	if err != nil {
		panic(err)
	}
	return res
}

func buildLightTheme() ui.Theme {
	return ui.Theme{
		Text:           ui.RGBA(24, 28, 36, 255),
		MutedText:      ui.RGBA(92, 102, 118, 255),
		Panel:          ui.RGBA(244, 246, 250, 255),
		PanelAlt:       ui.RGBA(230, 234, 242, 255),
		Border:         ui.RGBA(175, 184, 198, 255),
		Accent:         ui.RGBA(34, 108, 214, 255),
		AccentHover:    ui.RGBA(22, 88, 186, 255),
		Control:        ui.RGBA(255, 255, 255, 255),
		ControlHover:   ui.RGBA(235, 241, 252, 255),
		ControlPressed: ui.RGBA(216, 226, 244, 255),
		Disabled:       ui.RGBA(200, 205, 214, 210),
		Danger:         ui.RGBA(205, 52, 52, 255),
		Selection:      ui.RGBA(188, 214, 252, 255),
		Tooltip:        ui.RGBA(255, 253, 235, 255),
		Scrim:          ui.RGBA(15, 20, 30, 110),
		Header:         ui.RGBA(218, 224, 235, 255),
		FontSize:       16,
		ControlHeight:  38,
	}
}

func newDemoApp(flags demoFlags) *demoApp {
	dark := ui.DefaultTheme()
	dark.FontSize = 16
	dark.ControlHeight = 38

	mgr := ui.New()
	mgr.SetTheme(dark)
	mgr.SetInteractive(true)
	mgr.SetDisplayScale(float32(flags.scale))

	fonts := ui.NewSystemFontContext()
	fonts.SetCacheLimit(8 << 20)
	fonts.SetLayoutCacheLimit(8 << 20)
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
		flags:      flags,
		manager:    mgr,
		fonts:      fonts,
		darkTheme:  dark,
		lightTheme: buildLightTheme(),
		icons:      icons,
		bannerImg:  banner,
		altBanner:  altBanner,
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
	return d
}

func (d *demoApp) logf(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	if d.statusLabel != nil {
		d.statusLabel.SetText("Status: " + msg)
	}
	if d.outputPanel != nil {
		ts := time.Now().Format("15:04:05.000")
		d.outputPanel.AppendText(fmt.Sprintf("[%s] %s\n", ts, msg))
	}
}

func (d *demoApp) setupObserver(trackAllocs bool) {
	d.manager.SetFrameObserver(func(sample ui.FrameSample) {
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
		enc := d.jsonlEnc
		d.mu.Unlock()

		if enc != nil {
			_ = enc.Encode(jsonlFrameRecord{
				Kind:     "frame",
				Scenario: "full-showcase",
				At:       time.Now().UTC(),
				Sample:   sample,
			})
		}
	}, ui.FrameObserverOptions{TrackAllocations: trackAllocs})
}

func (d *demoApp) buildUI() {
	root := ui.NewStack(ui.Vertical, 6)
	root.Padding = ui.All(8)
	root.Background = d.darkTheme.Panel
	root.UIState().SetAccessibility(ui.AccessibilityInfo{
		Role:        ui.RoleGroup,
		Name:        "nativeForms Demo & Profiler Root",
		Description: "Root container hosting all widget tabs and live telemetry",
		Language:    "ru",
	})

	header := ui.NewStack(ui.Horizontal, 8)
	header.Padding = ui.Symmetric(10, 6)
	header.Background = d.darkTheme.Header
	header.Border = d.darkTheme.Border
	header.BorderWidth = 1
	header.Align = ui.AlignCenter

	title := ui.NewLabel("nativeForms — Полное демо виджетов и профилировщик (CPU / Memory)")
	title.Font = ui.FontSpec{Size: 17, Weight: 700}
	title.UIState().SetFlex(1)

	btnTheme := ui.NewButton("Тема: Dark/Light (F2)", func() {
		d.toggleTheme()
	})
	btnTheme.SetIcon(d.icons[0])
	btnTheme.UIState().SetMinimumSize(ui.Vec2{X: 180})
	btnTheme.UIState().SetTooltip("Переключить цветовую тему (SetTheme)")

	btnSelfTest := ui.NewButton("Авто-тест API (F5)", func() {
		d.runSelfTests()
	})
	btnSelfTest.SetIcon(d.icons[2])
	btnSelfTest.UIState().SetMinimumSize(ui.Vec2{X: 170})
	btnSelfTest.UIState().SetTooltip("Проверить корректность всех виджетов, редактора, виртуализации и Accessibility")

	btnStress := ui.NewButton("Стресс-тест (F6)", func() {
		next := !d.stressRunning.Load()
		d.setStressMode(next)
	})
	btnStress.SetIcon(d.icons[3])
	btnStress.UIState().SetMinimumSize(ui.Vec2{X: 165})
	btnStress.UIState().SetTooltip("Запустить/остановить автоматическую нагрузку на все вкладки для профилирования")

	header.Add(title, btnTheme, btnSelfTest, btnStress)

	d.tabs = ui.NewTabs([]ui.Tab{
		{
			ID:          "controls",
			Title:       "1. Виджеты и Формы",
			Icon:        d.icons[0],
			NonClosable: true,
			Content:     d.buildControlsTab(),
		},
		{
			ID:          "editor",
			Title:       "2. CodeEditor и Текст",
			Icon:        d.icons[1],
			NonClosable: true,
			Content:     d.buildEditorTab(),
		},
		{
			ID:          "virtual",
			Title:       "3. Виртуализация и Логи",
			Icon:        d.icons[2],
			NonClosable: true,
			Content:     d.buildVirtualTab(),
		},
		{
			ID:          "popups",
			Title:       "4. Попапы, A11y и ОС",
			Icon:        d.icons[3],
			NonClosable: true,
			Content:     d.buildPopupsAndPlatformTab(),
		},
		{
			ID:          "profiler",
			Title:       "5. Профилирование CPU/RAM",
			Icon:        d.icons[1],
			NonClosable: true,
			Content:     d.buildProfilerTab(),
		},
	}, "controls", func(selected string) {
		d.logf("Выбрана вкладка: %s", selected)
	})
	d.tabs.UIState().SetFlex(1)
	d.tabs.OnReorder = func(id string, index int) {
		d.logf("Вкладка %q перемещена на позицию %d", id, index)
	}
	d.tabs.OnCloseRequest = func(id string) bool {
		d.logf("Закрытие динамической вкладки %q", id)
		return true
	}

	footer := ui.NewStack(ui.Horizontal, 12)
	footer.Padding = ui.Symmetric(10, 4)
	footer.Background = d.darkTheme.Header
	footer.Border = d.darkTheme.Border
	footer.BorderWidth = 1

	d.statusLabel = ui.NewLabel("Status: Готово. Нажмите F5 для самопроверки или F6 для стресс-профилирования.")
	d.statusLabel.Ellipsis = true
	d.statusLabel.UIState().SetFlex(1)

	d.perfBanner = ui.NewLabel("")
	d.perfBanner.TextFunc = d.shortPerfSummary
	d.perfBanner.Align = ui.TextRight
	d.perfBanner.UIState().SetPreferredSize(ui.Vec2{X: 480, Y: 26})

	footer.Add(d.statusLabel, d.perfBanner)

	root.Add(header, d.tabs, footer)
	d.manager.SetRoot(root)
}

func (d *demoApp) buildControlsTab() ui.Component {
	columns := ui.NewStack(ui.Horizontal, 10)
	columns.Padding = ui.All(6)

	leftStack := ui.NewStack(ui.Vertical, 8)
	leftStack.Padding = ui.All(8)

	secTitle := ui.NewLabel("Базовые контролы (Button, Toggle, Checkbox, Radio, ComboBox, Slider, ProgressBar)")
	secTitle.Font = ui.FontSpec{Size: 15, Weight: 700}
	secTitle.Wrap = true

	d.btnCounter = ui.NewButton("Счётчик кликов: 0", func() {
		d.clickCount++
		d.btnCounter.SetText(fmt.Sprintf("Счётчик кликов: %d", d.clickCount))
		d.progressBar.SetValue(float32(d.clickCount%20) / 20.0)
		d.tabs.SetDirty("controls", d.clickCount%2 == 1)
		d.logf("Клик по кнопке #%d", d.clickCount)
	})
	d.btnCounter.SetIcon(d.icons[0])
	d.btnCounter.UIState().SetTooltip("Кнопка с иконкой и отслеживанием состояния Dirty на вкладке")

	d.toggleSwitch = ui.NewToggle("Режим геймпад/клавиатурной навигации (SetGameNavigation)", false, func(v bool) {
		d.manager.SetGameNavigation(v)
		d.logf("SetGameNavigation = %v", v)
	})
	d.toggleSwitch.SetIcon(d.icons[1])

	d.checkBox = ui.NewCheckbox("Активировать контролы формы (SetEnabled)", true, func(checked bool) {
		d.validField.UIState().SetEnabled(checked)
		d.sliderStep.UIState().SetEnabled(checked)
		d.comboBox.UIState().SetEnabled(checked)
		d.logf("Состояние активности контролов: %v", checked)
	})
	d.checkBox.SetIcon(d.icons[2])

	d.radioGroup = ui.NewRadioGroup([]ui.RadioOption{
		{ID: "contain", Label: "ImageFit: ImageContain", Icon: d.icons[0]},
		{ID: "cover", Label: "ImageFit: ImageCover", Icon: d.icons[1]},
		{ID: "stretch", Label: "ImageFit: ImageStretch", Icon: d.icons[2]},
	}, "contain", func(id string) {
		switch id {
		case "contain":
			d.imageWidget.SetFit(ui.ImageContain)
		case "cover":
			d.imageWidget.SetFit(ui.ImageCover)
		case "stretch":
			d.imageWidget.SetFit(ui.ImageStretch)
		}
		d.logf("Image.SetFit(%s)", id)
	})

	comboOpts := make([]ui.RadioOption, 16)
	for i := range comboOpts {
		scale := 1.0 + float64(i)*0.125
		comboOpts[i] = ui.RadioOption{
			ID:    fmt.Sprintf("%.3f", scale),
			Label: fmt.Sprintf("Опция #%02d — DisplayScale %.3fx", i+1, scale),
			Icon:  d.icons[i%len(d.icons)],
		}
	}
	d.comboBox = ui.NewComboBox(comboOpts, "1.000", func(id string) {
		if val, err := strconv.ParseFloat(id, 32); err == nil {
			d.manager.SetDisplayScale(float32(val))
			d.logf("SetDisplayScale(%.3f)", val)
		}
	})
	d.comboBox.UIState().SetTooltip("ComboBox с прокруткой (16 элементов, MaxVisibleItems=8)")

	var pctLabels [21]string
	for i := 0; i <= 20; i++ {
		pctLabels[i] = fmt.Sprintf("%d%%", i*5)
	}
	d.sliderStep = ui.NewSlider("Прогресс (шаг 5%)", 0, 100, 35, 5, func(val float32) {
		d.progressBar.SetValue(val / 100.0)
	})
	d.sliderStep.FormatValue = func(v float32) string {
		idx := min(20, max(0, int(v/5+0.5)))
		return pctLabels[idx]
	}

	var radLabels [64]string
	for i := range radLabels {
		radLabels[i] = fmt.Sprintf("%.2f rad", float64(i)*0.1)
	}
	d.sliderCont = ui.NewSlider("Фаза волны Canvas (непрерывный)", 0, 6.28318, 0.8, 0.1, func(val float32) {
		if d.canvasWidget != nil {
			d.canvasWidget.phase = val
			d.manager.InvalidatePaint()
		}
	})
	d.sliderCont.FormatValue = func(v float32) string {
		idx := min(len(radLabels)-1, max(0, int(v*10+0.5)))
		return radLabels[idx]
	}

	d.progressBar = ui.NewProgressBar(0.35)
	d.progressBar.ShowValue = true

	leftStack.Add(
		secTitle,
		d.btnCounter,
		d.toggleSwitch,
		d.checkBox,
		d.radioGroup,
		d.comboBox,
		d.sliderStep,
		d.sliderCont,
		d.progressBar,
		ui.NewSpacer(1, 8),
	)
	d.scrollForm = ui.NewScrollPanel(leftStack)
	d.scrollForm.UIState().SetFlex(1)

	rightStack := ui.NewStack(ui.Vertical, 8)
	rightStack.Padding = ui.All(8)

	formTitle := ui.NewLabel("Формы (NewForm / NewGrid), Текстовые поля и Графика")
	formTitle.Font = ui.FontSpec{Size: 15, Weight: 700}

	form := ui.NewForm(150)
	form.ColumnGap = 8
	form.RowGap = 6

	lblDigits := ui.NewLabel("Только цифры:")
	d.validField = ui.NewTextField("12345", func(s string) {
		d.logf("Цифровое поле: %q", s)
	})
	d.validField.Placeholder = "Введите число..."
	d.validField.Validate = func(next string) bool {
		for _, r := range next {
			if r < '0' || r > '9' {
				return false
			}
		}
		return true
	}
	d.validField.OnSubmit = func(s string) {
		d.manager.Notifyf("Отправлено значение: %s", s)
	}
	d.validField.UIState().SetAccessibility(ui.AccessibilityInfo{
		Role:        ui.RoleTextField,
		LabelledBy:  lblDigits,
		Description: "Принимает только цифры 0-9",
	})

	lblPass := ui.NewLabel("Пароль (Password):")
	d.passwordField = ui.NewTextField("secret2026", nil)
	d.passwordField.Password = true
	d.passwordField.Placeholder = "Пароль..."

	lblRO := ui.NewLabel("Только чтение:")
	d.readOnlyField = ui.NewTextField("ReadOnly TextField (нельзя изменить)", nil)
	d.readOnlyField.ReadOnly = true

	form.AddRow(lblDigits, d.validField)
	form.AddRow(lblPass, d.passwordField)
	form.AddRow(lblRO, d.readOnlyField)

	d.notesArea = ui.NewTextArea("Многострочное поле TextArea (Undo/Redo, выделение слов, автоперенос).\nВторая строка с кириллицей и UTF-8 символами: № € ₽ — «тест».", func(s string) {
		d.tabs.SetDirty("controls", true)
	})
	d.notesArea.Editor().Limit = 64
	d.notesArea.Editor().UndoByteLimit = 2 << 20
	d.notesArea.UIState().SetPreferredSize(ui.Vec2{X: 360, Y: 90})

	d.dynamicGrid = ui.NewGrid(
		ui.Track{Mode: ui.TrackFixed, Value: 150},
		ui.Track{Mode: ui.TrackStar, Value: 1},
	)
	d.dynamicGrid.ColumnGap = 8
	d.dynamicGrid.RowGap = 6

	overlayContainer := ui.NewPanel(ui.Overlay)
	overlayContainer.UIState().SetPreferredSize(ui.Vec2{X: 150, Y: 84})
	overlayContainer.Border = d.darkTheme.Border
	overlayContainer.BorderWidth = 1

	d.imageWidget = ui.NewImage(d.bannerImg)
	d.imageWidget.UIState().SetAnchor(ui.AnchorFill)

	d.overlayBadge = ui.NewLabel("OVERLAY")
	d.overlayBadge.Font = ui.FontSpec{Size: 12, Weight: 700}
	d.overlayBadge.UIState().SetAnchor(ui.AnchorBottomRight)
	d.overlayBadge.UIState().SetOffset(ui.Vec2{X: -4, Y: -4})
	overlayContainer.Add(d.imageWidget, d.overlayBadge)

	imgActions := ui.NewStack(ui.Vertical, 4)
	btnSwapImg := ui.NewButton("ImageResource.Replace / SetFilter", func() {
		d.usingAltImg = !d.usingAltImg
		if d.usingAltImg {
			if replaced, err := d.bannerImg.Replace(d.altBanner); err == nil {
				d.bannerImg = replaced
				d.imageWidget.SetImage(replaced)
			}
			d.imageWidget.SetFilter(ui.FilterNearest)
			d.imageWidget.SetTint(ui.RGBA(230, 255, 230, 255))
		} else {
			d.imageWidget.SetFilter(ui.FilterBilinear)
			d.imageWidget.SetTint(ui.RGBA(255, 255, 255, 255))
		}
		d.logf("Обновлён ресурс изображения (Version=%d)", d.bannerImg.Version())
	})
	btnScrollTop := ui.NewButton("ScrollIntoView (к счётчику)", func() {
		d.scrollForm.ScrollIntoView(d.btnCounter)
	})
	imgActions.Add(btnSwapImg, btnScrollTop)

	d.dynamicGrid.AddAt(overlayContainer, 0, 0, 1, 1)
	d.dynamicGrid.AddAt(imgActions, 0, 1, 1, 1)

	d.canvasWidget = newCustomCanvasWidget(d.icons[1])

	rightStack.Add(formTitle, form, d.notesArea, d.dynamicGrid, d.canvasWidget)
	rightScroll := ui.NewScrollPanel(rightStack)
	rightScroll.UIState().SetFlex(1)

	columns.Add(d.scrollForm, rightScroll)
	return columns
}

const sampleGoSource = `package showcase

import (
	"fmt"
	"time"
)

// WidgetMetrics stores per-frame CPU and memory diagnostics.
type WidgetMetrics struct {
	FrameID     uint64
	Duration    time.Duration
	AllocBytes  uint64
	Description string
}

// ComputeScore evaluates synthetic layout throughput.
func ComputeScore(items int, scale float64) (float64, error) {
	if items <= 0 {
		return 0, fmt.Errorf("items must be positive: %d", items)
	}
	total := 0.0
	for i := 0; i < items; i++ {
		total += float64(i) * scale
	}
	return total / float64(items), nil
}
`

func (d *demoApp) buildEditorTab() ui.Component {
	panel := ui.NewStack(ui.Vertical, 8)
	panel.Padding = ui.All(6)

	toolbar := ui.NewStack(ui.Horizontal, 6)
	btnUndo := ui.NewButton("Undo", func() {
		if _, ok := d.codeDoc.Undo(); ok {
			d.manager.InvalidatePaint()
			d.updateEditorInfo()
		}
	})
	btnUndo.UIState().SetMinimumSize(ui.Vec2{X: 90})

	btnRedo := ui.NewButton("Redo", func() {
		if _, ok := d.codeDoc.Redo(); ok {
			d.manager.InvalidatePaint()
			d.updateEditorInfo()
		}
	})
	btnRedo.UIState().SetMinimumSize(ui.Vec2{X: 90})

	btnCompl := ui.NewButton("Автодополнение (ShowCompletions)", func() {
		d.manager.PerformAccessibilityAction(d.codeEditor.UIState().AccessibilityID(), ui.AccessibilityFocus, "")
		d.showEditorCompletions()
	})
	btnCompl.UIState().SetMinimumSize(ui.Vec2{X: 240})

	btnBp := ui.NewButton("Точка останова (Line 15)", func() {
		cur := d.codeEditor.Breakpoint(14)
		d.codeEditor.SetBreakpoint(14, !cur)
		d.updateEditorInfo()
	})
	btnBp.UIState().SetMinimumSize(ui.Vec2{X: 190})

	btnTokens := ui.NewButton("LSP SemanticTokens / Styles", func() {
		d.applySampleSemanticTokens()
	})
	btnTokens.UIState().SetMinimumSize(ui.Vec2{X: 210})

	btnLargeDoc := ui.NewButton("Сгенерировать 5 000 строк", func() {
		var b strings.Builder
		b.WriteString(sampleGoSource)
		for i := 0; i < 5000; i++ {
			fmt.Fprintf(&b, "func generatedHelper_%04d(x int) int { return x + %d } // line %d\n", i, i*3, i+30)
		}
		doc, err := editor.NewDocument(b.String())
		if err == nil {
			d.codeDoc = doc
			d.codeEditor.SetDocument(doc)
			d.codeEditor.SetBreakpoint(2, true)
			d.codeEditor.ScrollToLine(2500)
			d.updateEditorInfo()
			d.logf("Загружен документ на %d строк (%d байт)", doc.LineCount(), doc.Len())
		}
	})
	btnLargeDoc.UIState().SetMinimumSize(ui.Vec2{X: 200})

	toolbar.Add(btnUndo, btnRedo, btnCompl, btnBp, btnTokens, btnLargeDoc)

	doc, _ := editor.NewDocument(sampleGoSource)
	d.codeDoc = doc
	d.codeEditor = ui.NewCodeEditor(doc)
	d.codeEditor.UIState().SetFlex(1)
	d.codeEditor.SetBreakpoint(7, true)
	d.codeEditor.SetBreakpoint(15, true)
	d.codeEditor.Palette = map[ui.CodeStyle]ui.Color{
		ui.CodeKeyword:  ui.RGBA(240, 180, 70, 255),
		ui.CodeFunction: ui.RGBA(110, 210, 245, 255),
		ui.CodeType:     ui.RGBA(135, 225, 165, 255),
		ui.CodeString:   ui.RGBA(180, 230, 140, 255),
		ui.CodeComment:  ui.RGBA(140, 155, 175, 255),
		ui.CodeNumber:   ui.RGBA(215, 165, 240, 255),
	}
	d.codeEditor.OnChange = func(ch editor.Change) {
		d.updateEditorInfo()
	}
	d.codeEditor.OnBreakpointChange = func(line int, enabled bool) {
		d.updateEditorInfo()
		d.logf("Breakpoint на строке %d: %v", line+1, enabled)
	}

	d.editorInfo = ui.NewLabel("")
	d.updateEditorInfo()

	panel.Add(toolbar, d.codeEditor, d.editorInfo)
	return panel
}

func (d *demoApp) updateEditorInfo() {
	if d.codeEditor == nil || d.editorInfo == nil {
		return
	}
	first, last := d.codeEditor.VisibleLines()
	bps := d.codeEditor.BreakpointLines()
	pos, _ := d.codeDoc.UTF16Position(d.codeEditor.Caret)
	d.editorInfo.SetText(fmt.Sprintf(
		"Документ v%d | Строк: %d | Байт: %d | Каретка: L%d:C%d (offset %d) | Видимые строки: [%d..%d) | Breakpoints (0-based): %v",
		d.codeDoc.Version(), d.codeDoc.LineCount(), d.codeDoc.Len(),
		pos.Line+1, pos.Character+1, d.codeEditor.Caret, first, last, bps,
	))
}

func (d *demoApp) showEditorCompletions() {
	insertSnippet := func(snippet string) {
		at := d.codeEditor.Caret
		d.codeEditor.ReplaceTextRange(at, at, snippet)
		d.updateEditorInfo()
	}
	items := []ui.MenuItem{
		{ID: "fmt", Label: "fmt.Println(\"debug metric\")", Icon: d.icons[0], Action: func() { insertSnippet("fmt.Println(\"debug metric\")") }},
		{ID: "for", Label: "for i := 0; i < count; i++ { ... }", Icon: d.icons[1], Action: func() { insertSnippet("for i := 0; i < count; i++ {\n\t\n}") }},
		{ID: "err", Label: "if err != nil { return err }", Icon: d.icons[2], Action: func() { insertSnippet("if err != nil {\n\treturn err\n}") }},
		{ID: "disabled", Label: "Недоступное предложение (Disabled)", Icon: d.icons[3], Disabled: true},
	}
	d.codeEditor.ShowCompletions(items)
}

func (d *demoApp) applySampleSemanticTokens() {
	ver := d.codeDoc.Version()
	legend := []string{"namespace", "struct", "function", "variable"}
	data := []uint32{
		0, 8, 8, 0, 0,
	}
	if err := d.codeEditor.SetSemanticTokens(ver, legend, data); err != nil {
		d.logf("SetSemanticTokens error: %v", err)
		return
	}
	_ = d.codeEditor.SetStyleRanges(ver, []ui.StyledRange{
		{Start: 0, End: 7, Style: ui.CodeKeyword},
	})
	d.logf("Применены SetSemanticTokens и SetStyleRanges для документа v%d", ver)
}

func (d *demoApp) buildVirtualTab() ui.Component {
	root := ui.NewStack(ui.Vertical, 8)
	root.Padding = ui.All(6)

	bar := ui.NewStack(ui.Horizontal, 8)
	btn1k := ui.NewButton("1 000 записей", func() { d.resizeVirtualModels(1000) })
	btn10k := ui.NewButton("10 000 записей", func() { d.resizeVirtualModels(10000) })
	btn50k := ui.NewButton("50 000 записей", func() { d.resizeVirtualModels(50000) })
	btnExpand := ui.NewButton("Развернуть/Свернуть дерево", func() {
		exp := !d.vTree.Expanded["w-controls"]
		for _, k := range []string{"root-widgets", "w-controls", "w-layouts", "w-popups", "root-virtual", "root-editors", "root-platform"} {
			d.vTree.SetExpanded(k, exp)
		}
	})
	btnLog := ui.NewButton("Добавить 100 строк в OutputPanel", func() {
		var chunk strings.Builder
		for i := 0; i < 100; i++ {
			fmt.Fprintf(&chunk, "[BUILD %04d] Compiling widget module... OK (%.2fms)\n", i, 0.4+float64(i%9)*0.15)
		}
		d.outputPanel.AppendText(chunk.String())
	})
	bar.Add(btn1k, btn10k, btn50k, btnExpand, btnLog)

	upper := ui.NewStack(ui.Horizontal, 8)
	upper.UIState().SetFlex(1)

	treeBox := ui.NewStack(ui.Vertical, 4)
	treeBox.UIState().SetFlex(1)
	treeBox.Add(ui.NewLabel("VirtualTree (TreeModel + TreeIconModel):"))
	d.vTreeModel = newDemoTreeModel(d.icons)
	d.vTree = ui.NewVirtualTree(d.vTreeModel)
	d.vTree.UIState().SetFlex(1)
	for _, id := range []string{"root-widgets", "w-controls", "root-virtual", "root-editors", "root-platform"} {
		d.vTree.SetExpanded(id, true)
	}
	d.vTree.OnSelect = func(id string) {
		d.logf("VirtualTree выбран узел: %s (%s)", id, d.vTreeModel.Label(id))
	}
	treeBox.Add(d.vTree)

	listBox := ui.NewStack(ui.Vertical, 4)
	listBox.UIState().SetFlex(1)
	listBox.Add(ui.NewLabel("VirtualList (ListModel + ListIconModel):"))
	d.vListModel = newDemoListModel(d.flags.count, d.icons)
	d.vList = ui.NewVirtualList(d.vListModel)
	d.vList.UIState().SetFlex(1)
	d.vList.OnSelect = func(id string) {
		d.logf("VirtualList выбрана строка ID=%s", id)
	}
	listBox.Add(d.vList)

	stackBox := ui.NewStack(ui.Vertical, 4)
	stackBox.UIState().SetFlex(1)
	stackBox.Add(ui.NewLabel("VirtualStack (StackModel + StackModelUpdater):"))
	d.vStackModel = newDemoStackModel(d.flags.count)
	d.vStack = ui.NewVirtualStack(d.vStackModel)
	d.vStackScroll = ui.NewScrollPanel(d.vStack)
	d.vStackScroll.UIState().SetFlex(1)
	stackBox.Add(d.vStackScroll)

	upper.Add(treeBox, listBox, stackBox)

	lower := ui.NewStack(ui.Horizontal, 8)
	lower.UIState().SetFlex(1)

	tableBox := ui.NewStack(ui.Vertical, 4)
	tableBox.UIState().SetFlex(1)
	tableBox.Add(ui.NewLabel("VirtualTable (2D Scroll + TableIconModel):"))
	d.vTableModel = newDemoTableModel(d.flags.count, d.icons)
	d.vTable = ui.NewVirtualTable(d.vTableModel)
	d.vTable.ColumnWidth = 140
	d.vTable.UIState().SetFlex(1)
	d.vTable.OnSelect = func(id string) {
		d.logf("VirtualTable выбрана строка: %s", id)
	}
	tableBox.Add(d.vTable)

	outBox := ui.NewStack(ui.Vertical, 4)
	outBox.UIState().SetFlex(1)
	outHeader := ui.NewStack(ui.Horizontal, 6)
	outHeader.Add(ui.NewLabel("OutputPanel (потоковый лог с автопрокруткой):"))
	btnClearOut := ui.NewButton("Очистить (Clear)", func() {
		d.outputPanel.Clear()
	})
	btnClearOut.UIState().SetMinimumSize(ui.Vec2{X: 140})
	outHeader.Add(btnClearOut)

	d.outputPanel = ui.NewOutputPanel()
	d.outputPanel.SetMaxLines(2000)
	d.outputPanel.UIState().SetFlex(1)
	d.outputPanel.AppendText("=== Инициализация OutputPanel завершена ===\n")
	outBox.Add(outHeader, d.outputPanel)

	lower.Add(tableBox, outBox)
	root.Add(bar, upper, lower)
	return root
}

func (d *demoApp) resizeVirtualModels(count int) {
	d.vListModel.resize(count)
	d.vList.ModelChanged()

	d.vTableModel.resize(count)
	d.vTable.ModelChanged()

	d.vStackModel.resize(count, fmt.Sprintf("Row[%dk]", count/1000))
	d.vStack.ModelChanged()

	d.logf("Размер виртуальных моделей обновлён до %d элементов", count)
}

func (d *demoApp) buildPopupsAndPlatformTab() ui.Component {
	columns := ui.NewStack(ui.Horizontal, 12)
	columns.Padding = ui.All(8)

	left := ui.NewStack(ui.Vertical, 8)
	left.UIState().SetFlex(1)

	lblPopup := ui.NewLabel("Оверлеи: Dialog, ContextMenu, UnfocusedPopup, Tooltip, Notifications")
	lblPopup.Font = ui.FontSpec{Size: 15, Weight: 700}

	btnModal := ui.NewButton("Открыть модальный диалог (OpenDialog / NewDialog)", func() {
		var popupID ui.PopupID
		body := ui.NewLabel("Модальное окно блокирует клики снаружи и фокусирует вложенные кнопки. Нажмите Escape или кнопку ниже для закрытия.")
		body.Wrap = true
		okBtn := ui.NewButton("Подтвердить", func() {
			d.manager.ClosePopup(popupID)
			d.manager.Notify("Модальный диалог подтверждён")
		})
		okBtn.SetIcon(d.icons[2])
		cancelBtn := ui.NewButton("Отмена", func() {
			d.manager.ClosePopup(popupID)
		})
		dlg := ui.NewDialog("Проверка модального диалога (RoleDialog)", body, okBtn, cancelBtn)
		popupID = d.manager.OpenDialog(dlg, 460, 220)
	})

	btnMenu := ui.NewButton("Открыть контекстное меню (OpenContextMenu)", func() {
		b := d.tabs.Bounds()
		d.manager.OpenContextMenu([]ui.MenuItem{
			{ID: "notify", Label: "Показать уведомление", Icon: d.icons[0], Action: func() { d.manager.Notify("Выбрано действие из меню!") }},
			{ID: "newtab", Label: "Открыть новую вкладку", Icon: d.icons[1], Action: d.openDynamicTab},
			{ID: "disabled", Label: "Неактивный пункт (Disabled)", Icon: d.icons[3], Disabled: true},
			{ID: "clear", Label: "Очистить уведомления", Icon: d.icons[2], Action: func() { d.manager.ClearNotifications() }},
		}, b.X+120, b.Y+120)
	})

	btnTooltip := ui.NewButton("Показать пассивный тултип (ShowTooltip)", func() {
		d.manager.ShowTooltip("Пассивный тултип (пропускает клики сквозь себя)", ui.Rect{X: 280, Y: 180, W: 360, H: 40})
	})

	notifRow := ui.NewStack(ui.Horizontal, 6)
	btnNotify := ui.NewButton("Notify / ShowMessage", func() {
		d.manager.ShowMessage("Тост-уведомление (3 сек)", 3.0)
	})
	btnNotify.UIState().SetFlex(1)

	btnSuspend := ui.NewButton("Suspend / Restore Notifications", func() {
		if d.suspendedNotif == nil {
			state := d.manager.SuspendNotifications()
			d.suspendedNotif = &state
			d.logf("Уведомления приостановлены (SuspendNotifications)")
		} else {
			d.manager.RestoreNotifications(*d.suspendedNotif)
			d.suspendedNotif = nil
			d.logf("Уведомления восстановлены (RestoreNotifications)")
		}
	})
	btnSuspend.UIState().SetFlex(1)
	notifRow.Add(btnNotify, btnSuspend)

	btnDynTab := ui.NewButton("Добавить закрываемую вкладку (Tabs.Open)", d.openDynamicTab)

	left.Add(lblPopup, btnModal, btnMenu, btnTooltip, notifRow, btnDynTab)

	right := ui.NewStack(ui.Vertical, 8)
	right.UIState().SetFlex(1)

	lblPlat := ui.NewLabel("Accessibility Tree и интеграция с ОС (Clipboard, FileDialogs, Capabilities)")
	lblPlat.Font = ui.FontSpec{Size: 15, Weight: 700}

	d.a11ySummary = ui.NewLabel("Нажмите «Снимок AccessibilitySnapshot», чтобы проинспектировать дерево доступности.")
	d.a11ySummary.Wrap = true

	btnA11y := ui.NewButton("Снимок AccessibilitySnapshot + PerformAccessibilityAction", func() {
		tree := d.manager.AccessibilitySnapshot()
		roleCounts := make(map[ui.AccessibleRole]int)
		for _, node := range tree.Nodes {
			roleCounts[node.Role]++
		}
		sliderID := d.sliderStep.UIState().AccessibilityID()
		acted := d.manager.PerformAccessibilityAction(sliderID, ui.AccessibilityIncrement, "")
		summary := fmt.Sprintf(
			"AccessibilityTree: корней=%d, узлов=%d, фокус ID=%d | Button=%d, TextField=%d, Slider=%d | PerformAction(Slider++): %v",
			len(tree.Roots), len(tree.Nodes), tree.Focused,
			roleCounts[ui.RoleButton], roleCounts[ui.RoleTextField], roleCounts[ui.RoleSlider], acted,
		)
		d.a11ySummary.SetText(summary)
		d.logf("%s", summary)
	})

	d.clipField = ui.NewTextField("Текст для проверки буфера обмена UTF-8", nil)
	clipRow := ui.NewStack(ui.Horizontal, 6)
	btnCopy := ui.NewButton("В буфер (SetClipboardText)", func() {
		if d.window != nil && d.window.Capabilities().Supports(platform.FeatureClipboard) {
			err := d.window.SetClipboardText(d.clipField.Text)
			d.logf("SetClipboardText: err=%v", err)
		} else {
			d.logf("Платформенный буфер обмена недоступен в текущем режиме")
		}
	})
	btnCopy.UIState().SetFlex(1)

	btnPaste := ui.NewButton("Из буфера (ClipboardText)", func() {
		if d.window != nil && d.window.Capabilities().Supports(platform.FeatureClipboard) {
			txt, err := d.window.ClipboardText()
			if err == nil {
				d.clipField.SetText(txt)
			}
			d.logf("ClipboardText: %q (err=%v)", txt, err)
		} else {
			d.logf("Платформенный буфер обмена недоступен в текущем режиме")
		}
	})
	btnPaste.UIState().SetFlex(1)
	clipRow.Add(btnCopy, btnPaste)

	dialogRow := ui.NewStack(ui.Horizontal, 6)
	btnOpenFile := ui.NewButton("OpenFile()", func() {
		if d.window == nil {
			d.logf("OpenFile пропущен (headless режим)")
			return
		}
		path, err := d.window.OpenFile()
		d.logf("OpenFile: path=%q err=%v", path, err)
	})
	btnOpenFile.UIState().SetMinimumSize(ui.Vec2{X: 120})
	btnOpenFile.UIState().SetFlex(1)

	btnOpenFolder := ui.NewButton("OpenFolder()", func() {
		if d.window == nil {
			d.logf("OpenFolder пропущен (headless режим)")
			return
		}
		path, err := d.window.OpenFolder()
		d.logf("OpenFolder: path=%q err=%v", path, err)
	})
	btnOpenFolder.UIState().SetMinimumSize(ui.Vec2{X: 120})
	btnOpenFolder.UIState().SetFlex(1)

	btnSaveFile := ui.NewButton("SaveFile()", func() {
		if d.window == nil {
			d.logf("SaveFile пропущен (headless режим)")
			return
		}
		path, err := d.window.SaveFile("report.jsonl")
		d.logf("SaveFile: path=%q err=%v", path, err)
	})
	btnSaveFile.UIState().SetMinimumSize(ui.Vec2{X: 120})
	btnSaveFile.UIState().SetFlex(1)
	dialogRow.Add(btnOpenFile, btnOpenFolder, btnSaveFile)

	var cachedCapText string
	d.capSummary = ui.NewLabel("")
	d.capSummary.Wrap = true
	d.capSummary.TextFunc = func() string {
		if d.window == nil {
			return "Capabilities: Headless Software Renderer (без нативного окна ОС)"
		}
		if cachedCapText == "" {
			c := d.window.Capabilities()
			cachedCapText = fmt.Sprintf(
				"Capabilities: SoftwareFrame=%v | Pointer=%v | Keyboard=%v | TextInput=%v | Clipboard=%v | IME=%v | MultiWin=%v | DragDrop=%v | FileDialog=%v | Wayland=%v",
				c.SoftwareFrame, c.Pointer, c.Keyboard, c.TextInput,
				c.Clipboard, c.IME, c.MultipleWindows, c.DragDrop, c.FileDialog, c.Wayland,
			)
		}
		return cachedCapText
	}

	right.Add(lblPlat, btnA11y, d.a11ySummary, d.clipField, clipRow, dialogRow, d.capSummary)
	columns.Add(left, right)
	return columns
}

func (d *demoApp) openDynamicTab() {
	id := fmt.Sprintf("dyn-%d", time.Now().UnixNano()%100000)
	content := ui.NewStack(ui.Vertical, 8)
	content.Padding = ui.All(16)
	content.Add(
		ui.NewLabel(fmt.Sprintf("Динамическая вкладка %s (можно перетаскивать заголовок или закрыть крестиком ×)", id)),
		ui.NewButton("Пометить вкладку изменённой (SetDirty)", func() {
			d.tabs.SetDirty(id, true)
		}),
	)
	d.tabs.Open(ui.Tab{
		ID:      id,
		Title:   "Вкладка " + id,
		Icon:    d.icons[2],
		Content: content,
		Dirty:   true,
	})
}

func (d *demoApp) buildProfilerTab() ui.Component {
	root := ui.NewStack(ui.Vertical, 10)
	root.Padding = ui.All(8)

	title := ui.NewLabel("Профилирование CPU и Памяти (FrameObserver, runtime.MemStats, pprof, FontContext Cache)")
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
	btnGC := ui.NewButton("Принудительный GC + FreeOSMemory", func() {
		before := readHeapAlloc()
		runtime.GC()
		debug.FreeOSMemory()
		after := readHeapAlloc()
		d.logf("GC выполнен: HeapAlloc %s -> %s", formatBytes(before), formatBytes(after))
	})
	btnGC.UIState().SetFlex(1)

	btnHeap := ui.NewButton("Сохранить Heap Profile (heap.pprof)", func() {
		path := d.flags.memProfile
		if path == "" {
			path = "heap.pprof"
		}
		if err := writeHeapProfile(path); err != nil {
			d.logf("Ошибка сохранения Heap-профиля: %v", err)
		} else {
			d.manager.Notifyf("Heap-профиль сохранён в %s", path)
			d.logf("Heap-профиль сохранён в %s (go tool pprof -http=:8080 %s)", path, path)
		}
	})
	btnHeap.UIState().SetFlex(1)

	btnAllocs := ui.NewButton("Сохранить Allocs/Goroutine Profile", func() {
		if err := writeNamedProfile("allocs", "allocs.pprof"); err != nil {
			d.logf("Ошибка allocs.pprof: %v", err)
			return
		}
		_ = writeNamedProfile("goroutine", "goroutine.pprof")
		d.manager.Notify("Сохранены allocs.pprof и goroutine.pprof")
		d.logf("Сохранены профили allocs.pprof и goroutine.pprof")
	})
	btnAllocs.UIState().SetFlex(1)

	btnResetStats := ui.NewButton("Сбросить пиковые метрики кадра", func() {
		d.mu.Lock()
		d.framesTotal = 0
		d.maxBuild = 0
		d.maxPaint = 0
		d.maxLayout = 0
		d.sumBuildNS = 0
		d.allocBytesSum = 0
		d.allocObjSum = 0
		d.lastCard1Update = time.Time{}
		d.lastCard2Update = time.Time{}
		d.lastCard3Update = time.Time{}
		d.mu.Unlock()
		d.logf("Пиковые метрики кадров сброшены")
	})
	btnResetStats.UIState().SetFlex(1)
	actions.Add(btnGC, btnHeap, btnAllocs, btnResetStats)

	cards := ui.NewGrid(
		ui.Track{Mode: ui.TrackStar, Value: 1},
		ui.Track{Mode: ui.TrackStar, Value: 1},
		ui.Track{Mode: ui.TrackStar, Value: 1},
	)
	cards.ColumnGap = 10
	cards.RowGap = 10

	cardFrame := d.makeMetricCard("Тайминги кадра и рендерера (FrameSample)", func() string {
		now := time.Now()
		d.mu.Lock()
		if d.cachedCard1Text != "" && now.Sub(d.lastCard1Update) < 250*time.Millisecond {
			txt := d.cachedCard1Text
			d.mu.Unlock()
			return txt
		}
		s := d.lastSample
		total := d.framesTotal
		maxB, maxL, maxP := d.maxBuild, d.maxLayout, d.maxPaint
		avgB := time.Duration(0)
		if total > 0 {
			avgB = time.Duration(d.sumBuildNS / total)
		}
		txt := fmt.Sprintf(
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
			s.Batches, s.Vertices, s.TextureUploads, formatBytes(uint64(maxInt64(0, s.UploadBytes))),
		)
		d.cachedCard1Text = txt
		d.lastCard1Update = now
		d.mu.Unlock()
		return txt
	})

	cardMem := d.makeMetricCard("Память и GC (runtime.MemStats)", func() string {
		now := time.Now()
		d.mu.Lock()
		if d.cachedCard2Text != "" && now.Sub(d.lastCard2Update) < 250*time.Millisecond {
			txt := d.cachedCard2Text
			d.mu.Unlock()
			return txt
		}
		lastAllocB := d.lastSample.AllocBytes
		lastAllocO := d.lastSample.AllocObjects
		sumAllocB := d.allocBytesSum
		sumAllocO := d.allocObjSum
		d.mu.Unlock()

		var m runtime.MemStats
		runtime.ReadMemStats(&m)
		txt := fmt.Sprintf(
			"HeapAlloc (живая куча): %s\n"+
				"HeapInuse: %s | HeapIdle: %s\n"+
				"HeapSys: %s | TotalSys: %s\n"+
				"HeapObjects: %d\n"+
				"TotalAlloc (за всё время): %s\n"+
				"Mallocs: %d | Frees: %d\n"+
				"Циклов GC (NumGC): %d\n"+
				"Паузы GC (PauseTotal): %v\n"+
				"Горутин: %d | GOMAXPROCS: %d\n"+
				"Аллокации кадра: %s (%d obj)\n"+
				"Сумма аллокаций кадров: %s (%d obj)",
			formatBytes(m.HeapAlloc),
			formatBytes(m.HeapInuse), formatBytes(m.HeapIdle),
			formatBytes(m.HeapSys), formatBytes(m.Sys),
			m.HeapObjects,
			formatBytes(m.TotalAlloc),
			m.Mallocs, m.Frees,
			m.NumGC,
			time.Duration(m.PauseTotalNs).Round(time.Microsecond),
			runtime.NumGoroutine(), runtime.GOMAXPROCS(0),
			formatBytes(lastAllocB), lastAllocO,
			formatBytes(sumAllocB), sumAllocO,
		)
		d.mu.Lock()
		d.cachedCard2Text = txt
		d.lastCard2Update = now
		d.mu.Unlock()
		return txt
	})

	cardCache := d.makeMetricCard("Кэш шрифтов и Стресс-тест (FontContext)", func() string {
		now := time.Now()
		d.mu.Lock()
		if d.cachedCard3Text != "" && now.Sub(d.lastCard3Update) < 250*time.Millisecond {
			txt := d.cachedCard3Text
			d.mu.Unlock()
			return txt
		}
		fHits := d.lastSample.TextCacheHits
		fMisses := d.lastSample.TextCacheMisses
		d.mu.Unlock()

		glyphBytes, glyphEntries := d.fonts.CacheUsage()
		layoutBytes, layoutEntries, hits, misses := d.fonts.LayoutCacheStats()
		hitRate := 0.0
		if hits+misses > 0 {
			hitRate = float64(hits) * 100.0 / float64(hits+misses)
		}
		txt := fmt.Sprintf(
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
		d.mu.Lock()
		d.cachedCard3Text = txt
		d.lastCard3Update = now
		d.mu.Unlock()
		return txt
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
	body.Font = ui.FontSpec{Family: "monospace", Size: 13}
	body.TextFunc = textFn

	card.Add(hdr, body)
	return card
}

func (d *demoApp) shortPerfSummary() string {
	now := time.Now()
	d.mu.Lock()
	if d.cachedBannerText != "" && now.Sub(d.lastBannerUpdate) < 250*time.Millisecond {
		txt := d.cachedBannerText
		d.mu.Unlock()
		return txt
	}
	s := d.lastSample
	total := d.framesTotal
	d.mu.Unlock()

	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	txt := fmt.Sprintf(
		"Frames: %d | Build: %v | Cmds: %d | Glyphs: %d | Allocs: %d (%s) | Heap: %s | GC: %d",
		total, s.Build.Round(time.Microsecond), s.Commands, s.Glyphs,
		s.AllocObjects, formatBytes(s.AllocBytes), formatBytes(m.HeapAlloc), m.NumGC,
	)
	d.mu.Lock()
	d.cachedBannerText = txt
	d.lastBannerUpdate = now
	d.mu.Unlock()
	return txt
}

func (d *demoApp) registerShortcuts() {
	d.manager.AddShortcut(ui.Shortcut{
		Key:          ui.KeyF2,
		AllowInModal: true,
		Action:       d.toggleTheme,
	})
	d.manager.AddShortcut(ui.Shortcut{
		Key:          ui.KeyF5,
		AllowInModal: true,
		Action:       d.runSelfTests,
	})
	d.manager.AddShortcut(ui.Shortcut{
		Key:          ui.KeyF6,
		AllowInModal: true,
		Action: func() {
			d.setStressMode(!d.stressRunning.Load())
		},
	})
	d.manager.AddShortcut(ui.Shortcut{
		Key:  ui.KeyG,
		Mods: ui.ModControl,
		Action: func() {
			runtime.GC()
			d.manager.Notify("Выполнен runtime.GC()")
		},
	})
}

func (d *demoApp) toggleTheme() {
	d.isLight = !d.isLight
	if d.isLight {
		d.manager.SetTheme(d.lightTheme)
		d.logf("Установлена светлая тема (Light Theme)")
	} else {
		d.manager.SetTheme(d.darkTheme)
		d.logf("Установлена тёмная тема (Dark Theme)")
	}
}

func (d *demoApp) setStressMode(enabled bool) {
	d.stressRunning.Store(enabled)
	if d.stressToggle != nil {
		d.stressToggle.SetValue(enabled)
	}
	if enabled {
		d.logf("Стресс-тест ЗАПУЩЕН (интервал %v)", d.flags.interval)
	} else {
		d.logf("Стресс-тест ОСТАНОВЛЕН")
	}
}

// stepStressWorkload exercises layout, painting, text shaping, scrolling,
// virtualized containers, and code editing on a single UI tick without
// allocating temporary strings on every frame.
func (d *demoApp) stepStressWorkload(step int) {
	tabIDs := [...]string{"controls", "editor", "virtual", "popups", "profiler"}
	targetTab := tabIDs[(step/25)%len(tabIDs)]
	if d.tabs.SelectedID != targetTab {
		d.tabs.Select(targetTab)
	}

	switch targetTab {
	case "controls":
		d.sliderStep.SetValue(float32((step * 5) % 105))
		d.sliderCont.SetValue(float32((step % 62)) * 0.1)
		d.progressBar.SetValue(float32(step%100) / 100.0)
		d.scrollForm.Scroll(48)
		if step%12 == 0 {
			d.scrollForm.Scroll(-400)
		}
	case "editor":
		lineCount := d.codeDoc.LineCount()
		targetLine := (step * 97) % max(1, lineCount)
		d.codeEditor.ScrollToLine(targetLine)
		if step%10 == 0 {
			at := d.codeEditor.Caret
			d.codeEditor.ReplaceTextRange(at, at, "// perf ")
			_, _ = d.codeDoc.Undo()
		}
		_, _ = d.manager.TextInputState()
	case "virtual":
		d.vList.ScrollTo((step * 31) % max(1, d.vListModel.Len()))
		d.vTable.Offset = float32((step * 36) % max(1, d.vTableModel.Len()*32))
		d.vTable.OffsetX = float32((step * 15) % 220)
		d.vStackScroll.Scroll(64)
		if step%15 == 0 {
			d.vStackScroll.Scroll(-600)
		}
		if step%16 == 0 {
			d.outputPanel.AppendText("[STRESS] Virtualized stream tick OK\n")
		}
	case "popups":
		if step%10 == 0 {
			_ = d.manager.AccessibilitySnapshot()
		}
	case "profiler":
		d.manager.InvalidatePaint()
	}
}

func (d *demoApp) runSelfTests() {
	passed := 0
	failed := 0
	check := func(name string, cond bool) {
		if cond {
			passed++
		} else {
			failed++
			d.logf("[FAIL] %s", name)
		}
	}

	if d.flags.headless {
		d.manager.BeginFrame(d.flags.width, d.flags.height, 0.016)
	}

	okValid := d.validField.ReplaceTextRange(0, len(d.validField.Text), "98765")
	okInvalid := d.validField.ReplaceTextRange(0, len(d.validField.Text), "98a65")
	check("TextField.Validate accepts digits and rejects letters", okValid && !okInvalid && d.validField.Text == "98765")

	d.sliderStep.SetValue(43.2)
	check("Slider step snapping (43.2 -> 45)", d.sliderStep.Value == 45)

	prevToggle := d.toggleSwitch.Value
	d.toggleSwitch.SetValue(!prevToggle)
	check("Toggle.SetValue updates value", d.toggleSwitch.Value == !prevToggle)
	d.toggleSwitch.SetValue(prevToggle)

	d.radioGroup.SetSelected("cover")
	check("RadioGroup.SetSelected updates selection & Image.Fit", d.radioGroup.SelectedID == "cover" && d.imageWidget.Fit == ui.ImageCover)
	d.radioGroup.SetSelected("contain")

	vBefore := d.codeDoc.Version()
	ch, err := d.codeDoc.Apply([]editor.Edit{{Start: 0, End: 0, Text: "// header\n"}})
	check("editor.Document.Apply increments version", err == nil && ch.Version == vBefore+1)
	_, undone := d.codeDoc.Undo()
	check("editor.Document.Undo restores content", undone && d.codeDoc.Version() == vBefore+2)
	_, redone := d.codeDoc.Redo()
	check("editor.Document.Redo re-applies edit", redone)
	_, _ = d.codeDoc.Undo()

	d.vList.SetSelected("42")
	d.vList.ScrollTo(42)
	check("VirtualList selection & scroll", d.vList.SelectedID == "42")

	d.vTable.SetSelected("row-10")
	check("VirtualTable selection", d.vTable.SelectedID == "row-10")

	d.vTree.SetExpanded("w-layouts", true)
	d.vTree.SetSelected("w-grid")
	check("VirtualTree expand & select", d.vTree.Expanded["w-layouts"] && d.vTree.SelectedID == "w-grid")

	beforeLines := len(d.outputPanel.Lines())
	d.outputPanel.AppendText("Self-test log line 1\nSelf-test partial ")
	d.outputPanel.AppendText("completed\n")
	check("OutputPanel chunked streaming", len(d.outputPanel.Lines()) == beforeLines+2)

	pid := d.manager.OpenDialog(ui.NewDialog("SelfTest Dialog", ui.NewLabel("Testing modal")), 320, 160)
	check("OpenDialog returns non-zero PopupID", pid != 0)
	closed := d.manager.ClosePopup(pid)
	check("ClosePopup closes dialog", closed)

	d.tabs.Select("controls")
	if d.flags.headless {
		d.manager.BeginFrame(d.flags.width, d.flags.height, 0.016)
	}
	tree := d.manager.AccessibilitySnapshot()
	check("AccessibilitySnapshot produces non-empty tree", len(tree.Nodes) > 5)
	btnID := d.btnCounter.UIState().AccessibilityID()
	beforeClicks := d.clickCount
	activated := d.manager.PerformAccessibilityAction(btnID, ui.AccessibilityActivate, "")
	check("PerformAccessibilityAction activates Button", activated && d.clickCount == beforeClicks+1)

	msg := fmt.Sprintf("Самопроверка завершена: %d PASS, %d FAIL", passed, failed)
	d.manager.Notify(msg)
	d.logf("%s", msg)
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
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return m.HeapAlloc
}

func formatBytes(b uint64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := uint64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.2f %ciB", float64(b)/float64(div), "KMGTPE"[exp])
}

func maxFloat32(a, b float32) float32 {
	if a > b {
		return a
	}
	return b
}

func maxInt64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

func nonEmpty(s, fallback string) string {
	if s == "" {
		return fallback
	}
	return s
}

func (d *demoApp) startProfilers() error {
	if d.flags.pprofAddr != "" {
		go func() {
			d.logf("Запущен HTTP pprof сервер на http://%s/debug/pprof/", d.flags.pprofAddr)
			if err := http.ListenAndServe(d.flags.pprofAddr, nil); err != nil {
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
		if err := pprof.StartCPUProfile(f); err != nil {
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
		d.jsonlWriter = bufio.NewWriterSize(f, 256*1024)
		d.jsonlEnc = json.NewEncoder(d.jsonlWriter)
		_ = d.jsonlEnc.Encode(map[string]any{
			"Kind":      "manifest",
			"Scenario":  "full-showcase",
			"Pattern":   "warm",
			"Graphics":  d.flags.graphics,
			"GoVersion": runtime.Version(),
			"OS":        runtime.GOOS,
			"Arch":      runtime.GOARCH,
			"Width":     d.flags.width,
			"Height":    d.flags.height,
			"Count":     d.flags.count,
			"Started":   time.Now().UTC(),
		})
		_ = d.jsonlWriter.Flush()
	}
	return nil
}

func (d *demoApp) stopProfilers() error {
	if d.cpuProfFile != nil {
		pprof.StopCPUProfile()
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
		_ = d.jsonlEnc.Encode(map[string]any{
			"kind":           "summary",
			"dropped_frames": 0,
			"frames":         d.framesTotal,
			"ended":          time.Now().UTC(),
		})
		_ = d.jsonlWriter.Flush()
		_ = d.jsonlFile.Close()
	}
	return nil
}

func (d *demoApp) runHeadless() error {
	renderer := software.New()
	defer renderer.Close()

	scale := float32(d.flags.scale)
	if scale <= 0 {
		scale = 1
	}
	target := image.NewRGBA(image.Rect(0, 0, int(float32(d.flags.width)*scale), int(float32(d.flags.height)*scale)))

	renderOneFrame := func() error {
		d.manager.BeginFrame(d.flags.width, d.flags.height, 0.016)
		tr := d.manager.FrameTrace()
		rasterStart := tr.Start()
		if err := renderer.RenderAtScale(d.manager.Frame(), target, scale); err != nil {
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
		Title:    "nativeForms — Widget Showcase & CPU/Memory Profiler",
		Width:    d.flags.width,
		Height:   d.flags.height,
		Backend:  d.flags.backend,
		Graphics: render.Kind(d.flags.graphics),
		OnReady: func(w desktop.Window) {
			close(ready)
		},
		OnFilesDropped: func(paths []string) {
			d.manager.Notifyf("Перетащено файлов: %d", len(paths))
			d.logf("OnFilesDropped: %v", paths)
		},
		OnCloseRequest: func(w desktop.Window) platform.CloseAction {
			return platform.QuitApp
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
		var endTimer <-chan time.Time
		if d.flags.stress && d.flags.duration > 0 {
			t := time.NewTimer(d.flags.duration)
			defer t.Stop()
			endTimer = t.C
		}
		ticker := time.NewTicker(d.flags.interval)
		defer ticker.Stop()

		sig := make(chan os.Signal, 1)
		signal.Notify(sig, os.Interrupt)
		defer signal.Stop(sig)

		var pending atomic.Bool
		// Allocate the Post callback once outside the loop so ticks do 0 closure allocations.
		postStep := func() {
			defer pending.Store(false)
			cur := int(d.stressStep.Add(1))
			d.stepStressWorkload(cur)
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

func parseFlags() demoFlags {
	var f demoFlags
	flag.StringVar(&f.backend, "backend", "", "Linux window backend: '', 'x11', or 'wayland'")
	flag.StringVar(&f.graphics, "graphics", "auto", "Renderer backend: 'auto', 'hardware', or 'software'")
	flag.IntVar(&f.width, "width", 1280, "Logical window width")
	flag.IntVar(&f.height, "height", 820, "Logical window height")
	flag.Float64Var(&f.scale, "scale", 1.0, "Initial display scale factor (1.0 - 4.0)")
	flag.BoolVar(&f.headless, "headless", false, "Run offscreen using software renderer without an OS window")
	flag.BoolVar(&f.stress, "stress", false, "Automatically run stress interactions across all tabs")
	flag.DurationVar(&f.duration, "duration", 15*time.Second, "Duration for headless or automated stress run")
	flag.DurationVar(&f.warmup, "warmup", 1*time.Second, "Warmup delay before automated desktop stress loop")
	flag.DurationVar(&f.interval, "interval", 16*time.Millisecond, "Stress interaction period")
	flag.IntVar(&f.count, "count", 2000, "Initial row count for VirtualList, VirtualTable, and VirtualStack")
	flag.BoolVar(&f.trackAllocs, "track-allocs", true, "Sample per-frame runtime allocations via FrameObserverOptions")
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