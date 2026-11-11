package nativeforms

import (
	"math"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

const outputPanelScrollbarWidth float32 = 10

// OutputTextPos represents a 0-based (Line, Col) rune coordinate in an OutputPanel.
type OutputTextPos struct {
	Line int
	Col  int
}

func compareOutputPos(a, b OutputTextPos) int {
	if a.Line != b.Line {
		if a.Line < b.Line {
			return -1
		}
		return 1
	}
	if a.Col < b.Col {
		return -1
	}
	if a.Col > b.Col {
		return 1
	}
	return 0
}

// OutputPanel displays a bounded, continuous, selectable stream of build, log, or
// terminal output. When TerminalMode is enabled, it also renders an interactive
// inline command prompt on the last line and accepts direct keyboard/IME input.
type OutputPanel struct {
	State
	List         *VirtualList
	Font         FontSpec
	LineHeight   float32
	TerminalMode bool
	Prompt       string
	InputText    string
	InputCaret   int
	OnCommand    func(string)
	OnInterrupt  func()

	lines           []string
	partial         bool
	firstID         uint64
	maxLines        int
	scrollX         float32
	scrollY         float32
	targetScrollY   float32
	smoothScrolling bool
	scrollDrag      bool
	scrollDragOff   float32
	scrollHover     bool

	selAnchor    OutputTextPos
	selCaret     OutputTextPos
	selecting    bool
	lastClick    time.Time
	lastPoint    Vec2
	clicks       int
	caretVisible bool
	composing    bool
	preedit      string
	history      []string
	historyIdx   int
}

var _ Component = (*OutputPanel)(nil)
var _ TextInputClient = (*OutputPanel)(nil)

type outputLines struct{ panel *OutputPanel }

func (m outputLines) Len() int { return len(m.panel.lines) }
func (m outputLines) ID(i int) string {
	return strconv.FormatUint(m.panel.firstID+uint64(i), 10)
}
func (m outputLines) Label(i int) string { return m.panel.lines[i] }

func NewOutputPanel() *OutputPanel {
	p := &OutputPanel{
		maxLines:     5000,
		Font:         FontSpec{Family: "monospace", Size: 14},
		Prompt:       "$ ",
		caretVisible: true,
	}
	p.owner, p.focusable, p.clip = p, true, true
	// Keep List initialized and synchronized for backward compatibility with callers
	// that inspect p.List.Offset or p.List.Model, but do NOT add it as a child so
	// OutputPanel renders continuous sequential text instead of discrete list rows.
	p.List = NewVirtualList(outputLines{panel: p})
	return p
}

func (p *OutputPanel) fontSpec() FontSpec {
	spec := p.Font
	if spec.Size <= 0 {
		spec.Size = 14
	}
	if spec.Family == "" {
		spec.Family = "monospace"
	}
	return spec
}

func (p *OutputPanel) visualLineHeight() float32 {
	if p.LineHeight > 0 {
		return p.LineHeight
	}
	if fonts := p.fontContext(); fonts != nil {
		h := fonts.Measure("", p.effectiveFontSpec(p.fontSpec())).Y
		return maxFloat32(16, float32(math.Ceil(float64(h))))
	}
	return maxFloat32(18, float32(math.Ceil(float64(p.fontSpec().Size*1.25))))
}

func (p *OutputPanel) displayLineCount() int {
	if !p.TerminalMode {
		return len(p.lines)
	}
	if len(p.lines) == 0 {
		return 1
	}
	if p.partial {
		return len(p.lines)
	}
	return len(p.lines) + 1
}

func (p *OutputPanel) promptLineIndex() int {
	if !p.TerminalMode {
		return -1
	}
	return max(0, p.displayLineCount()-1)
}

func (p *OutputPanel) promptPrefixForLastLine() string {
	if !p.TerminalMode {
		return ""
	}
	if p.partial && len(p.lines) > 0 {
		return p.lines[len(p.lines)-1]
	}
	return p.Prompt
}

func (p *OutputPanel) displayLine(i int) string {
	if i < 0 || i >= p.displayLineCount() {
		return ""
	}
	if !p.TerminalMode {
		return p.lines[i]
	}
	if i == p.promptLineIndex() {
		return p.promptPrefixForLastLine() + p.InputText + p.preedit
	}
	return p.lines[i]
}

// Lines returns a snapshot of the visible log, including an unfinished line.
func (p *OutputPanel) Lines() []string {
	return append([]string(nil), p.lines...)
}

// AllText returns all retained lines joined by newline characters.
func (p *OutputPanel) AllText() string {
	return strings.Join(p.lines, "\n")
}

// HasSelection reports whether a non-empty character range is currently selected.
func (p *OutputPanel) HasSelection() bool {
	return p.selAnchor != p.selCaret
}

func (p *OutputPanel) normalizedSelection() (OutputTextPos, OutputTextPos) {
	if compareOutputPos(p.selAnchor, p.selCaret) <= 0 {
		return p.selAnchor, p.selCaret
	}
	return p.selCaret, p.selAnchor
}

func sliceRunes(s string, startCol, endCol int) string {
	r := []rune(s)
	startCol = min(max(0, startCol), len(r))
	endCol = min(max(startCol, endCol), len(r))
	return string(r[startCol:endCol])
}

// SelectedText returns the currently selected text across one or more lines.
func (p *OutputPanel) SelectedText() string {
	if !p.HasSelection() {
		return ""
	}
	total := p.displayLineCount()
	if total == 0 {
		return ""
	}
	start, end := p.normalizedSelection()
	start.Line = min(max(0, start.Line), total-1)
	end.Line = min(max(0, end.Line), total-1)
	if start.Line == end.Line {
		return sliceRunes(p.displayLine(start.Line), start.Col, end.Col)
	}
	var b strings.Builder
	firstLine := p.displayLine(start.Line)
	b.WriteString(sliceRunes(firstLine, start.Col, utf8.RuneCountInString(firstLine)))
	for line := start.Line + 1; line < end.Line; line++ {
		b.WriteByte('\n')
		b.WriteString(p.displayLine(line))
	}
	b.WriteByte('\n')
	b.WriteString(sliceRunes(p.displayLine(end.Line), 0, end.Col))
	return b.String()
}

// SelectAll selects all text in the OutputPanel.
func (p *OutputPanel) SelectAll() {
	total := p.displayLineCount()
	if total == 0 {
		p.selAnchor, p.selCaret = OutputTextPos{}, OutputTextPos{}
		return
	}
	lastIdx := total - 1
	lastLen := utf8.RuneCountInString(p.displayLine(lastIdx))
	p.selAnchor = OutputTextPos{Line: 0, Col: 0}
	p.selCaret = OutputTextPos{Line: lastIdx, Col: lastLen}
	p.InvalidatePaint()
}

// Copy copies the current selection (or all text if nothing is selected) to the clipboard.
func (p *OutputPanel) Copy() bool {
	if p.manager == nil || p.manager.clipboardWrite == nil {
		return false
	}
	text := p.SelectedText()
	if text == "" {
		return false
	}
	return p.manager.clipboardWrite(text) == nil
}

// CopyAll copies the entire buffer of the OutputPanel to the clipboard.
func (p *OutputPanel) CopyAll() bool {
	if p.manager == nil || p.manager.clipboardWrite == nil {
		return false
	}
	text := p.AllText()
	if text == "" {
		return false
	}
	return p.manager.clipboardWrite(text) == nil
}

// Paste inserts clipboard text into the interactive terminal prompt.
func (p *OutputPanel) Paste() bool {
	if !p.TerminalMode || p.manager == nil || p.manager.clipboardRead == nil {
		return false
	}
	val, err := p.manager.clipboardRead()
	if err != nil || val == "" {
		return false
	}
	val = strings.ReplaceAll(val, "\r\n", "\n")
	parts := strings.Split(val, "\n")
	for i, part := range parts {
		p.insertAtPrompt(part)
		if i < len(parts)-1 {
			p.submitPrompt()
		}
	}
	return true
}
