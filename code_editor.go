package nativeforms

import (
	"sort"
	"sync"
	"time"

	"github.com/banditmoscow1337/nativeForms/editor"
)

const codeEditorScrollbarWidth float32 = 12

// CodeDiagnostic describes an error, warning, or informational diagnostic on a
// specific 0-based line of a CodeEditor document.
type CodeDiagnostic struct {
	Line      int    // 0-based line index
	Column    int    // 0-based column index
	EndLine   int    // 0-based end line index
	EndColumn int    // 0-based end column index
	Severity  int    // 1 = Error, 2 = Warning, 3 = Info/Hint
	Message   string // human-readable diagnostic message
	Source    string // e.g. "gopls", "go/parser", "go build", "go vet"
}

type codeLineCacheEntry struct {
	valid       bool
	line        int
	version     uint64
	start, end  int
	spec        FontSpec
	fontGen     uint64
	tabWidth    int
	stylesLen   int
	semanticLen int
	hasTabs     bool
	display     []rune
	offsets     []int
	columns     []int
	carets      []float32
	styles      []CodeStyle
}

type codeEditorScratch struct {
	display   [256]rune
	offsets   [256]int
	columns   [256]int
	carets    [256]float32
	styles    [256]CodeStyle
	spans     [32]StyledRange
	lineBytes [256]byte
	lineCache [128]codeLineCacheEntry
}

const codeEditorPoolSize = 64

var (
	sharedNormalLexStates = make([]goLexState, 131072)
	codeEditorPoolMu      sync.Mutex
	codeEditorStorage     [codeEditorPoolSize]CodeEditor
	codeEditorScratchPool [codeEditorPoolSize]codeEditorScratch
	freeCodeEditors       []*CodeEditor
	codeEditorPoolOnce    sync.Once
)

func initCodeEditorPool() {
	freeCodeEditors = make([]*CodeEditor, 0, codeEditorPoolSize)
	for i := codeEditorPoolSize - 1; i >= 0; i-- {
		e := &codeEditorStorage[i]
		e.scratch = &codeEditorScratchPool[i]
		e.pooled = true
		e.inPool = true
		freeCodeEditors = append(freeCodeEditors, e)
	}
}

func recycleCodeEditor(e *CodeEditor) {
	if e == nil || e.parent != nil || e.manager != nil {
		return
	}
	if e.Document != nil {
		e.Document.ReleaseAuto()
		e.Document = nil
	}
	codeEditorPoolMu.Lock()
	codeEditorPoolOnce.Do(initCodeEditorPool)
	if e.inPool {
		codeEditorPoolMu.Unlock()
		return
	}
	sc := e.scratch
	wasPooled := e.pooled
	if sc != nil {
		for i := range sc.lineCache {
			sc.lineCache[i].valid = false
		}
	}
	*e = CodeEditor{scratch: sc, pooled: wasPooled, inPool: true}
	if len(freeCodeEditors) < cap(freeCodeEditors) {
		freeCodeEditors = append(freeCodeEditors, e)
	}
	codeEditorPoolMu.Unlock()
}

// Close releases a detached CodeEditor back to the internal widget pool.
func (e *CodeEditor) Close() {
	recycleCodeEditor(e)
}

// CodeEditor is a focusable, unwrapped source editor backed by editor.Document.
// It paints only visible lines. Byte offsets, breakpoint line numbers and
// callbacks are zero based; displayed line numbers start at one.
type CodeEditor struct {
	State
	Document           *editor.Document
	Font               FontSpec
	TabWidth           int
	ReadOnly           bool
	Caret, Anchor      int
	Palette            map[CodeStyle]Color
	Lexer              SyntaxLexer
	OnChange           func(editor.Change)
	OnBreakpointChange func(line int, enabled bool)
	// OnGoToDefinition is invoked from the context menu or F12 shortcut with the
	// identifier under the caret and its zero-based byte offset in the Document.
	// When nil, CodeEditor searches the current document for a matching declaration.
	OnGoToDefinition func(symbol string, offset int)
	// ContextMenuItems optionally customizes or extends the default context menu.
	// It receives the default items (Go to Definition, Cut, Copy, Paste, Select All,
	// Undo/Redo, Toggle Breakpoint) and returns the items to display.
	ContextMenuItems func(defaultItems []MenuItem) []MenuItem

	breakpoints                  map[int]bool
	diagnostics                  []CodeDiagnostic
	diagByLine                   map[int]CodeDiagnostic
	debugLine                    int
	pendingCenterLine            int
	styles, semantic             []StyledRange
	lexStates                    []goLexState
	lexStatesShared              bool
	lastVersion                  uint64
	scrollX, scrollY             float32
	scrollTargetX, scrollTargetY float32
	smoothScrolling              bool
	scrollDrag                   bool
	scrollDragOffset             float32
	scrollHover                  bool
	visibleMaxLineW              float32
	desiredX                     float32
	retainX                      bool
	composing                    bool
	preedit                      string
	preeditCaret                 int
	caretVisible                 bool
	lastClick                    time.Time
	lastPoint                    Vec2
	clicks                       int
	completionPopup              PopupID

	scratchDisplay   []rune
	scratchOffsets   []int
	scratchColumns   []int
	scratchCarets    []float32
	scratchStyles    []CodeStyle
	scratchSpans     []StyledRange
	scratchLineBytes []byte
	lineCache        []codeLineCacheEntry
	numBuf           [20]byte
	crossPieceLines  map[int]string
	lineNumberCache  []string
	gutterDigits     int
	gutterSpec       FontSpec
	gutterFontGen    uint64
	gutterCachedW    float32

	snapValid                      bool
	snapVersion                    uint64
	snapCaret, snapAnchor, snapMax int
	snapState                      TextInputState

	caretRectValid        bool
	caretRectVersion      uint64
	caretRectCaret        int
	caretRectAnchor       int
	caretRectComposing    bool
	caretRectPreedit      string
	caretRectPreeditCaret int
	caretRectScrollX      float32
	caretY                float32
	caretRectBounds       Rect
	caretRectFontGen      uint64
	caretRectSpec         FontSpec
	cachedCaretRect       Rect
	scratch               *codeEditorScratch
	pooled                bool
	inPool                bool
}

var _ Component = (*CodeEditor)(nil)
var _ TextInputClient = (*CodeEditor)(nil)

func NewCodeEditor(document *editor.Document) *CodeEditor {
	if document == nil {
		document, _ = editor.NewDocument("")
	}
	document.Acquire()
	lineCount := document.LineCount()
	var e *CodeEditor
	codeEditorPoolMu.Lock()
	codeEditorPoolOnce.Do(initCodeEditorPool)
	if n := len(freeCodeEditors); n > 0 {
		e = freeCodeEditors[n-1]
		freeCodeEditors[n-1] = nil
		freeCodeEditors = freeCodeEditors[:n-1]
		e.inPool = false
	}
	codeEditorPoolMu.Unlock()
	if e == nil {
		e = &CodeEditor{scratch: new(codeEditorScratch)}
	}
	sc := e.scratch
	if sc == nil {
		sc = new(codeEditorScratch)
		e.scratch = sc
	}
	for i := range sc.lineCache {
		sc.lineCache[i].valid = false
	}
	var lexStates []goLexState
	lexShared := false
	if !document.MayContainMultilineTokens() && lineCount+1 <= len(sharedNormalLexStates) {
		lexStates = sharedNormalLexStates[:lineCount+1 : lineCount+1]
		lexShared = true
	} else {
		lexStates = make([]goLexState, lineCount+1, max(16, lineCount+1))
	}
	wasPooled := e.pooled
	*e = CodeEditor{
		Document:          document,
		Font:              FontSpec{Family: "monospace", Size: 16},
		TabWidth:          4,
		debugLine:         -1,
		pendingCenterLine: -1,
		lexStates:         lexStates,
		lexStatesShared:   lexShared,
		lastVersion:       document.Version(),
		caretVisible:      true,
		scratchDisplay:    sc.display[:0],
		scratchOffsets:    sc.offsets[:0],
		scratchColumns:    sc.columns[:0],
		scratchCarets:     sc.carets[:0],
		scratchStyles:     sc.styles[:0],
		scratchSpans:      sc.spans[:0],
		scratchLineBytes:  sc.lineBytes[:0],
		lineCache:         sc.lineCache[:],
		scratch:           sc,
		pooled:            wasPooled,
		inPool:            false,
	}
	e.owner = e
	e.focusable = true
	e.clip = true
	e.minimum = Vec2{X: 300, Y: 180}
	if !lexShared && document.MayContainMultilineTokens() {
		e.rebuildLexStates(0)
	}
	return e
}

// SetLexer configures a custom SyntaxLexer (or nil for the default Go lexer)
// and refreshes syntax states and line caches.
func (e *CodeEditor) SetLexer(lexer SyntaxLexer) {
	if e.Lexer == lexer {
		return
	}
	e.Lexer = lexer
	e.invalidateLineCache()
	e.rebuildLexStates(0)
	e.InvalidatePaint()
}

func (e *CodeEditor) invalidateLineCache() {
	for i := range e.lineCache {
		e.lineCache[i].valid = false
	}
}

func (e *CodeEditor) SetDocument(document *editor.Document) {
	e.HideCompletions()
	if document == nil {
		document, _ = editor.NewDocument("")
	}
	if e.Document != document {
		if e.Document != nil {
			e.Document.ReleaseAuto()
		}
		document.Acquire()
	}
	e.Document = document
	e.Caret, e.Anchor = 0, 0
	e.scrollX, e.scrollY = 0, 0
	e.scrollTargetX, e.scrollTargetY = 0, 0
	e.smoothScrolling = false
	e.scrollDrag = false
	e.visibleMaxLineW = 0
	e.debugLine = -1
	e.pendingCenterLine = -1
	e.diagnostics = nil
	clear(e.diagByLine)
	e.styles, e.semantic = nil, nil
	clear(e.crossPieceLines)
	e.invalidateLineCache()
	e.rebuildLexStates(0)
	e.lastVersion = document.Version()
	e.snapValid = false
	e.caretRectValid = false
	clear(e.breakpoints)
	e.composing, e.preedit = false, ""
	e.invalidateLayout()
}

func (e *CodeEditor) Text() string { e.syncDocument(); return e.Document.Text() }

// SetDiagnostics sets per-line error/warning markers to render inside the editor
// (gutter badges, red line numbers, wavy underlines, inline Error Lens messages,
// and scrollbar overview ticks).
func (e *CodeEditor) SetDiagnostics(diags []CodeDiagnostic) {
	e.syncDocument()
	e.diagnostics = append(e.diagnostics[:0], diags...)
	if e.diagByLine == nil {
		e.diagByLine = make(map[int]CodeDiagnostic, len(diags))
	} else {
		clear(e.diagByLine)
	}
	for _, d := range diags {
		if d.Line < 0 {
			continue
		}
		if d.Severity <= 0 {
			d.Severity = 1
		}
		existing, has := e.diagByLine[d.Line]
		if !has || d.Severity < existing.Severity {
			e.diagByLine[d.Line] = d
		} else if existing.Message != "" && d.Message != "" && existing.Message != d.Message {
			existing.Message = existing.Message + " | " + d.Message
			e.diagByLine[d.Line] = existing
		}
	}
	e.InvalidatePaint()
}

// Diagnostics returns a copy of the active per-line diagnostics in the editor.
func (e *CodeEditor) Diagnostics() []CodeDiagnostic {
	return append([]CodeDiagnostic(nil), e.diagnostics...)
}

// DiagnosticAtLine returns the primary diagnostic for the given 0-based line.
func (e *CodeEditor) DiagnosticAtLine(line int) (CodeDiagnostic, bool) {
	if len(e.diagByLine) == 0 {
		return CodeDiagnostic{}, false
	}
	d, ok := e.diagByLine[line]
	return d, ok
}

// SetDebugLine sets the 0-based active execution line during debugging (-1 clears it).
func (e *CodeEditor) SetDebugLine(line int) {
	if e.debugLine == line {
		return
	}
	e.debugLine = line
	e.InvalidatePaint()
}

// DebugLine returns the 0-based active execution line during debugging (-1 if none).
func (e *CodeEditor) DebugLine() int { return e.debugLine }

func (e *CodeEditor) SetBreakpoint(line int, enabled bool) bool {
	e.syncDocument()
	if line < 0 || line >= e.Document.LineCount() || e.breakpoints[line] == enabled {
		return false
	}
	if e.breakpoints == nil {
		e.breakpoints = make(map[int]bool)
	}
	e.breakpoints[line] = enabled
	if !enabled {
		delete(e.breakpoints, line)
	}
	if e.OnBreakpointChange != nil {
		e.OnBreakpointChange(line, enabled)
	}
	e.InvalidatePaint()
	return true
}

func (e *CodeEditor) Breakpoint(line int) bool { return e.breakpoints[line] }

func (e *CodeEditor) BreakpointLines() []int {
	lines := make([]int, 0, len(e.breakpoints))
	for line := range e.breakpoints {
		lines = append(lines, line)
	}
	sort.Ints(lines)
	return lines
}

func (e *CodeEditor) syncDocument() {
	if e.Document == nil {
		e.Document, _ = editor.NewDocument("")
	}
	if e.Document.Version() == e.lastVersion {
		return
	}
	e.lastVersion = e.Document.Version()
	e.snapValid = false
	e.caretRectValid = false
	clear(e.crossPieceLines)
	e.invalidateLineCache()
	e.rebuildLexStates(0)
	e.styles, e.semantic = nil, nil
	e.Caret = e.validCaret(e.Caret)
	e.Anchor = e.validCaret(e.Anchor)
	maxY := e.maxScrollY()
	e.scrollY = clampFloat32(e.scrollY, 0, maxY)
	e.scrollTargetY = clampFloat32(e.scrollTargetY, 0, maxY)
	maxX := e.maxScrollX()
	e.scrollX = clampFloat32(e.scrollX, 0, maxX)
	e.scrollTargetX = clampFloat32(e.scrollTargetX, 0, maxX)
	e.InvalidatePaint()
}

func (e *CodeEditor) validCaret(at int) int {
	return e.Document.ValidCaretOffset(at)
}
