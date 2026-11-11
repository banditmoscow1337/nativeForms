package main

import (
	"math"
	"strconv"

	ui "github.com/banditmoscow1337/nativeForms"
)

func appendFourDigitInt(dst []byte, n int) []byte {
	if n < 0 {
		dst = append(dst, '-')
		n = -n
	}
	if n < 10000 {
		return append(dst,
			byte('0'+(n/1000)%10),
			byte('0'+(n/100)%10),
			byte('0'+(n/10)%10),
			byte('0'+n%10),
		)
	}
	return strconv.AppendInt(dst, int64(n), 10)
}

// customCanvasWidget demonstrates raw Canvas drawing primitives (CommandQuad,
// CommandRoundedRect, RoundedBorder, CommandLine, CommandImage, CommandMask,
// PushClip/PopClip, Tessellate) and the ViewportCuller interface without per-frame allocations.
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
		sampleVerts: make([]ui.Vertex, 0, 64),
		cachedPhase: -1,
	}
	w.UIState().SetPreferredSize(ui.Vec2{X: 360, Y: 130})
	w.UIState().SetMinimumSize(ui.Vec2{X: 220, Y: 110})
	w.UIState().SetTooltip("Custom Canvas widget: Rect, RoundedRect, RoundedBorder, Line, Image, TextStyled, Clip, Tessellate & ViewportCuller")
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
	radius := w.UIState().CornerRadius()
	if radius <= 0 {
		radius = theme.CornerRadius
	}
	if !w.UIState().Frameless() {
		canvas.RoundedRect(r, radius, theme.PanelAlt)
		canvas.RoundedBorder(r, radius, 1, theme.Border)
	}

	inner := r.Inset(ui.All(10))
	canvas.PushClip(inner)
	defer canvas.PopClip()

	// Rounded rectangle + rounded border with dynamic accent colors.
	badgeRect := ui.Rect{X: inner.X, Y: inner.Y, W: min(148, inner.W), H: 36}
	canvas.RoundedRect(badgeRect, 10, theme.Accent.WithAlpha(0.25))
	canvas.RoundedBorder(badgeRect, 10, 1.5, theme.Accent)
	canvas.TextStyled("Canvas Primitives", badgeRect, ui.FontSpec{Size: 14, Weight: 600}, ui.TextCenter, theme.Text)

	// Animated sine wave using Canvas.Line.
	waveLeft := inner.X + 160
	waveWidth := max(40, inner.W-166)
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
	w.sampleCmds[0] = ui.DrawCommand{Kind: ui.CommandRoundedRect, Rect: ui.Rect{W: 32, H: 16}, Clip: ui.Rect{W: 32, H: 16}, Radius: 4, Width: 1, Tint: theme.Accent}
	w.sampleCmds[1] = ui.DrawCommand{Kind: ui.CommandLine, From: ui.Vec2{X: 0, Y: 0}, To: ui.Vec2{X: 32, Y: 16}, Width: 2, Clip: ui.Rect{W: 32, H: 16}, Tint: theme.Text}
	w.sampleVerts = ui.Tessellate(w.sampleCmds[:], w.sampleVerts[:0])
	w.vertices = len(w.sampleVerts)

	if w.cachedText == "" || w.cachedPhase != w.phase || w.cachedCull != w.cullCount {
		w.cachedPhase = w.phase
		w.cachedCull = w.cullCount
		var buf [96]byte
		b := append(buf[:0], "Phase: "...)
		b = strconv.AppendFloat(b, float64(w.phase), 'f', 2, 32)
		b = append(b, " rad | Sample Tessellate: "...)
		b = strconv.AppendInt(b, int64(w.vertices), 10)
		b = append(b, " verts | Culled: "...)
		b = strconv.AppendInt(b, int64(w.cullCount), 10)
		w.cachedText = string(b)
	}
	infoRect := ui.Rect{X: inner.X + 58, Y: inner.Y + 48, W: max(0, inner.W-60), H: 48}
	canvas.Text(w.cachedText, infoRect, 14, ui.TextLeft, theme.MutedText)
}

// --- Virtual Models (lazy/bounded caches so 50k+ rows do not bloat heap) ---

const virtualStringCacheCap = 1024

type cachedRowText struct {
	idx int
	val string
}

type demoListModel struct {
	count  int
	icons  []*ui.ImageResource
	labels [virtualStringCacheCap]cachedRowText
}

func newDemoListModel(count int, icons []*ui.ImageResource) *demoListModel {
	m := &demoListModel{count: count, icons: icons}
	for i := range m.labels {
		m.labels[i].idx = -1
	}
	return m
}

func (m *demoListModel) resize(count int) { m.count = count }
func (m *demoListModel) Len() int         { return m.count }
func (m *demoListModel) ID(i int) string  { return strconv.Itoa(i) }
func (m *demoListModel) Label(i int) string {
	slot := uint(i) & (virtualStringCacheCap - 1)
	if m.labels[slot].idx == i && m.labels[slot].val != "" {
		return m.labels[slot].val
	}
	var buf [64]byte
	b := append(buf[:0], "Item #"...)
	b = appendFourDigitInt(b, i)
	if i%2 == 0 {
		b = append(b, " — VirtualList Row (Icon=true)"...)
	} else {
		b = append(b, " — VirtualList Row (Icon=false)"...)
	}
	s := string(b)
	m.labels[slot] = cachedRowText{idx: i, val: s}
	return s
}
func (m *demoListModel) Icon(i int) *ui.ImageResource {
	if len(m.icons) == 0 || i%2 != 0 {
		return nil
	}
	return m.icons[i%len(m.icons)]
}

type demoTableModel struct {
	rows    int
	icons   []*ui.ImageResource
	rowIDs  [virtualStringCacheCap]cachedRowText
	col0    [virtualStringCacheCap]cachedRowText
	latStrs [250]string
	memStrs [512]string
}

func newDemoTableModel(rows int, icons []*ui.ImageResource) *demoTableModel {
	m := &demoTableModel{rows: rows, icons: icons}
	for i := range m.rowIDs {
		m.rowIDs[i].idx = -1
		m.col0[i].idx = -1
	}
	for i := range m.latStrs {
		m.latStrs[i] = strconv.Itoa(45+i) + " us"
	}
	for i := range m.memStrs {
		m.memStrs[i] = strconv.Itoa(12+i) + " KB"
	}
	return m
}

func (m *demoTableModel) resize(rows int) { m.rows = rows }
func (m *demoTableModel) Len() int        { return m.rows }
func (m *demoTableModel) ID(r int) string {
	slot := uint(r) & (virtualStringCacheCap - 1)
	if m.rowIDs[slot].idx == r && m.rowIDs[slot].val != "" {
		return m.rowIDs[slot].val
	}
	var buf [24]byte
	b := append(buf[:0], "row-"...)
	b = strconv.AppendInt(b, int64(r), 10)
	s := string(b)
	m.rowIDs[slot] = cachedRowText{idx: r, val: s}
	return s
}
func (m *demoTableModel) Columns() int { return 5 }
func (m *demoTableModel) Header(col int) string {
	headers := [...]string{"ID", "Component", "Status", "Latency (us)", "Memory (KB)"}
	if col >= 0 && col < len(headers) {
		return headers[col]
	}
	return ""
}
func (m *demoTableModel) Cell(row, col int) string {
	names := [...]string{"ColorPicker", "SpinBox", "SegmentedControl", "Accordion", "Toolbar", "CodeEditor", "VirtualTable", "Slider"}
	switch col {
	case 0:
		slot := uint(row) & (virtualStringCacheCap - 1)
		if m.col0[slot].idx == row && m.col0[slot].val != "" {
			return m.col0[slot].val
		}
		var buf [24]byte
		b := append(buf[:0], "ROW-"...)
		b = appendFourDigitInt(b, row)
		s := string(b)
		m.col0[slot] = cachedRowText{idx: row, val: s}
		return s
	case 1:
		return names[row%len(names)]
	case 2:
		if row%7 == 0 {
			return "WARN"
		}
		return "OK"
	case 3:
		return m.latStrs[(row*17)%250]
	case 4:
		return m.memStrs[(row*31)%512]
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
			"w-controls":    {"w-colorpicker", "w-spinbox", "w-segmented", "w-toolbar", "w-hyperlink", "w-badge", "w-rounded", "w-autosize", "w-button", "w-toggle", "w-checkbox", "w-radio", "w-combobox", "w-slider", "w-progress", "w-image"},
			"w-layouts":     {"w-panel", "w-stack", "w-separator", "w-accordion", "w-grid", "w-form", "w-scroll", "w-split", "w-tabs", "w-menubar"},
			"w-popups":      {"w-dialog", "w-menu", "w-unfocused", "w-tooltip", "w-notify"},
			"root-virtual":  {"v-list", "v-table", "v-tree", "v-stack", "v-output"},
			"root-editors":  {"e-textfield", "e-textarea", "e-codeeditor", "e-document"},
			"root-platform": {"p-tray", "p-frameless", "p-clipboard", "p-dialogs", "p-ime", "p-a11y", "p-perf"},
		},
		labels: map[string]string{
			"root-widgets":  "Core Widgets & Containers",
			"w-controls":    "Interactive Controls (Rounded, AutoSize & Frameless)",
			"w-colorpicker": "ColorPicker (HSV Square, Hue/Alpha, Presets, Compact Popup)",
			"w-spinbox":     "SpinBox (Numeric Stepper −/+, Wheel & Keyboard)",
			"w-segmented":   "SegmentedControl (Horizontal Pill Button Group)",
			"w-toolbar":     "Toolbar (Compact Action/Toggle Buttons, Separators, Spacers)",
			"w-hyperlink":   "Hyperlink (Accent Link with Hover Underline & Context Menu)",
			"w-badge":       "Badge (Pill Status/Count Indicator)",
			"w-rounded":     "CornerRadius & RoundedBorder (SetCornerRadius / Theme.CornerRadius)",
			"w-autosize":    "AutoSize & SizeToText (Dynamic Text Fitting on Demand)",
			"w-button":      "Button (Icon, FontSpec, CornerRadius, AutoSize, OnClick)",
			"w-toggle":      "Toggle (ON/OFF Switch)",
			"w-checkbox":    "Checkbox (Checked & Indeterminate States)",
			"w-radio":       "RadioGroup (Single Choice)",
			"w-combobox":    "ComboBox (Dropdown Menu)",
			"w-slider":      "Slider (Stepped & Continuous, Clipped Title)",
			"w-progress":    "ProgressBar (Determinate & Indeterminate Marquee)",
			"w-image":       "Image (Contain, Cover, Stretch, Filter)",
			"w-layouts":     "Layout Containers",
			"w-panel":       "Panel (Vertical, Horizontal, Overlay, CornerRadius)",
			"w-stack":       "Stack & Spacer",
			"w-separator":   "Separator & LabeledSeparator (Horizontal/Vertical Divider)",
			"w-accordion":   "Accordion & Collapsible (Expandable Sections)",
			"w-grid":        "Grid (TrackAuto, TrackFixed, TrackStar, Zero-Alloc)",
			"w-form":        "Form (Two-Column Label + Control)",
			"w-scroll":      "ScrollPanel (2D Viewport & ScrollIntoView)",
			"w-split":       "SplitPane (Draggable Resizable Divider)",
			"w-tabs":        "Tabs (Dirty, Closable, Reorderable, Move)",
			"w-menubar":     "MenuBar (Top Application Menu & Shortcuts)",
			"w-popups":      "Overlays & Notifications",
			"w-dialog":      "Modal Dialog (OpenDialog / NewDialog)",
			"w-menu":        "Context & Anchored Menu (OpenContextMenu / OpenMenu)",
			"w-unfocused":   "Unfocused Popup (OpenUnfocusedPopup)",
			"w-tooltip":     "Passive Tooltip (ShowTooltip)",
			"w-notify":      "Toast Notifications (Suspend/Restore)",
			"root-virtual":  "Virtualized Views",
			"v-list":        "VirtualList (ListModel + ListIconModel)",
			"v-table":       "VirtualTable (TableModel + TableIconModel)",
			"v-tree":        "VirtualTree (TreeModel + TreeIconModel)",
			"v-stack":       "VirtualStack (StackModel + StackModelUpdater)",
			"v-output":      "OutputPanel (Streaming Log Buffer)",
			"root-editors":  "Text & Source Editors",
			"e-textfield":   "TextField (Validation, Password, ReadOnly, AutoSize, CornerRadius)",
			"e-textarea":    "TextArea (Multiline + Undo/Redo)",
			"e-codeeditor":  "CodeEditor (Go Lexer, LSP Tokens, Breakpoints)",
			"e-document":    "editor.Document (Piece Tree, UTF-16 Offsets)",
			"root-platform": "Platform, Window & Diagnostics",
			"p-tray":        "System Tray (App.SetTray / RemoveTray / Close-to-Tray)",
			"p-frameless":   "Window Frame & Controls (SetFrameless / SetWindowControls)",
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
	prefix string
	titles [virtualStringCacheCap]cachedRowText
}

func newDemoStackModel(count int) *demoStackModel {
	m := &demoStackModel{
		count:  count,
		values: make(map[int]string),
		prefix: "VirtualRow",
	}
	for i := range m.titles {
		m.titles[i].idx = -1
	}
	return m
}

func (m *demoStackModel) resize(count int, prefix string) {
	m.count = count
	if m.prefix != prefix {
		m.prefix = prefix
		for i := range m.titles {
			m.titles[i] = cachedRowText{idx: -1}
		}
	}
}

func (m *demoStackModel) title(i int) string {
	slot := uint(i) & (virtualStringCacheCap - 1)
	if m.titles[slot].idx == i && m.titles[slot].val != "" {
		return m.titles[slot].val
	}
	var buf [48]byte
	b := append(buf[:0], m.prefix...)
	b = append(b, " #"...)
	b = appendFourDigitInt(b, i)
	s := string(b)
	m.titles[slot] = cachedRowText{idx: i, val: s}
	return s
}

func (m *demoStackModel) Len() int             { return m.count }
func (m *demoStackModel) Key(i int) string     { return strconv.Itoa(i) }
func (m *demoStackModel) Height(i int) float32 { return float32(44 + (i%3)*6) }
func (m *demoStackModel) Build(i int) ui.Component {
	row := ui.NewStack(ui.Horizontal, 8)
	row.Padding = ui.Symmetric(6, 2)
	lbl := ui.NewLabel(m.title(i))
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
	st := panel.UIState()
	if lbl, ok := st.ChildAt(0).(*ui.Label); ok {
		lbl.SetText(m.title(i))
	}
	if field, ok := st.ChildAt(1).(*ui.TextField); ok {
		val, ok := m.values[i]
		if !ok {
			val = "Editable value 0123456789"
		}
		idx := i
		field.OnChange = nil
		field.SetText(val)
		field.OnChange = func(s string) {
			m.values[idx] = s
		}
	}
}
