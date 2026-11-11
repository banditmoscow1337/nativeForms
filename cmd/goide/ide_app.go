package goide

import (
	"context"
	"io"
	"sync"
	"time"

	ui "github.com/banditmoscow1337/nativeForms"
	"github.com/banditmoscow1337/nativeForms/editor"
	"github.com/banditmoscow1337/nativeForms/platform"
)

// DesktopHost abstracts native OS file dialogs and platform capabilities.
type DesktopHost interface {
	platform.FileDialogs
	Capabilities() platform.Capabilities
}

type openDocument struct {
	path       string
	document   *editor.Document
	view       *ui.CodeEditor
	views      [3]*ui.CodeEditor
	lastText   string
	lspVersion int
	dirty      bool
	shortcuts  []ui.ShortcutID
}

func (d *openDocument) allViews() []*ui.CodeEditor {
	if d == nil {
		return nil
	}
	var out []*ui.CodeEditor
	for _, v := range d.views {
		if v != nil {
			out = append(out, v)
		}
	}
	if len(out) == 0 && d.view != nil {
		out = append(out, d.view)
	}
	return out
}

// App owns the VS Code-like Go IDE state, UI workbench, and background tool clients.
type App struct {
	manager        *ui.Manager
	host           DesktopHost
	Root           ui.Component
	icons          *Icons
	fonts          *ui.FontContext
	darkTheme      ui.Theme
	lightTheme     ui.Theme
	isLight        bool
	editorFontSize float32
	editorTabWidth int
	FormatOnSave   bool

	// Go SDK & Toolchain management
	selectedGoSDK        string
	selectedToolPaths    map[string]string
	selectedToolVersions map[string]string
	AutoInstallTools     bool
	sdkCombo             *ui.ComboBox
	goplsVerCombo        *ui.ComboBox
	dlvVerCombo          *ui.ComboBox

	// System shell terminal state
	terminalShell       string
	terminalCwd         string
	terminalPromptBadge *ui.Badge
	shellCombo          *ui.ComboBox
	termMu              sync.Mutex
	termStdin           io.WriteCloser
	termRunning         bool

	project           *projectTree
	fileTree          *ui.VirtualTree
	openEditors       *openEditorsModel
	openEditorsList   *ui.VirtualList
	outline           *outlineModel
	outlineView       *ui.VirtualList
	searchInput       *ui.TextField
	searchResults     *searchModel
	searchResultsView *ui.VirtualTable
	testResults       *testResultsModel
	testResultsView   *ui.VirtualTable
	breakpointsModel  *breakpointsModel
	breakpointsView   *ui.VirtualList

	menuBar           *ui.MenuBar
	topToolbar        *ui.Toolbar
	activityBar       *ui.Panel
	activityButtons   map[string]*ui.Button
	sidebarContainer  *ui.Panel
	sidebarViews      map[string]ui.Component
	activeSidebarID   string
	sidebarVisible    bool
	savedSidebarRatio float32

	breadcrumbBar     *ui.Panel
	breadcrumbLabel   *ui.Label
	activeSymbolBadge *ui.Badge
	splitBadge        *ui.Badge
	findBar           *ui.Panel
	findInput         *ui.TextField
	replaceInput      *ui.TextField

	// Side-by-side Editor Splits (1 / 2 / 3 slices)
	editorTabs       *ui.Tabs
	editorGroups     [3]*ui.Tabs
	editorMainSplit  *ui.SplitPane
	editorRightSplit *ui.SplitPane
	activeGroup      int
	splitCount       int

	bottomTabs       *ui.Tabs
	bottomVisible    bool
	savedBottomRatio float32
	centerSplit      *ui.SplitPane
	mainSplit        *ui.SplitPane

	output         *ui.OutputPanel
	terminalOutput *ui.OutputPanel
	terminalInput  *ui.TextField
	debugOutput    *ui.OutputPanel
	debugEvalInput *ui.TextField

	statusBar         *ui.Panel
	branchBadge       *ui.Badge
	lspBadge          *ui.Badge
	problemsBadge     *ui.Badge
	debugBadge        *ui.Badge
	taskProgress      *ui.ProgressBar
	status            *ui.Label
	caretLabel        *ui.Label
	goVersionBadge    *ui.Badge
	telemetryLabel    *ui.Label
	toolsSummaryLabel *ui.Label

	diagnostics       *diagnosticsModel
	diagnosticsView   *ui.VirtualTable
	diagnosticByURI   map[string][]lspDiagnostic
	syntaxDiagsByPath map[string][]diagnosticRow
	toolDiagsBySource map[string][]diagnosticRow

	stack         *stackModel
	stackView     *ui.VirtualTable
	variables     *variablesModel
	variablesView *ui.VirtualTable

	files        map[string]*openDocument
	breakpoints  map[string]map[int]bool
	lsp          *lspClient
	debug        *debugSession
	taskCancel   context.CancelFunc
	closingForce bool
	closed       bool

	perfMu       sync.Mutex
	framesTotal  uint64
	lastBuildDur time.Duration
	lastTotalDur time.Duration
}

// New constructs and wires the Go IDE workbench on the provided ui.Manager.
func New(manager *ui.Manager, host DesktopHost) *App {
	icons := DefaultIcons()
	dark := VSCodeDarkTheme()
	light := VSCodeLightTheme()

	if manager.FontContext() == nil {
		fonts := ui.NewSystemFontContext()
		fonts.SetCacheLimit(512 << 10)
		fonts.SetLayoutCacheLimit(256 << 10)
		manager.SetFontContext(fonts)
	}
	manager.SetTheme(dark)
	manager.SetInteractive(true)

	a := &App{
		manager:           manager,
		host:              host,
		icons:             icons,
		fonts:             manager.FontContext(),
		darkTheme:         dark,
		lightTheme:        light,
		editorFontSize:    16,
		editorTabWidth:    4,
		FormatOnSave:      false,
		AutoInstallTools:  true,
		selectedToolPaths: make(map[string]string),
		selectedToolVersions: map[string]string{
			"gopls":       "latest",
			"dlv":         "latest",
			"goimports":   "latest",
			"staticcheck": "latest",
		},
		terminalShell:     DetectSystemShell(),
		splitCount:        1,
		activeGroup:       0,
		project:           newProjectTree(""),
		files:             make(map[string]*openDocument),
		breakpoints:       make(map[string]map[int]bool),
		diagnosticByURI:   make(map[string][]lspDiagnostic),
		syntaxDiagsByPath: make(map[string][]diagnosticRow),
		toolDiagsBySource: make(map[string][]diagnosticRow),
		diagnostics:       &diagnosticsModel{icons: icons},
		stack:             &stackModel{icons: icons},
		variables:         &variablesModel{icons: icons},
		sidebarVisible:    true,
		savedSidebarRatio: 0.22,
		bottomVisible:     true,
		savedBottomRatio:  0.70,
	}

	manager.SetFrameObserver(func(sample ui.FrameSample) {
		a.perfMu.Lock()
		a.framesTotal++
		a.lastBuildDur = sample.Build
		a.perfMu.Unlock()
	})

	a.buildWorkbench()
	a.registerGlobalShortcuts()
	a.AutoDiscoverToolchain(false)
	return a
}
