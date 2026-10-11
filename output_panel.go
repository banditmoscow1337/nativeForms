package nativeforms

import (
	"strconv"
	"strings"
)

// OutputPanel displays a bounded, scrollable stream of build or debug output.
// AppendText accepts arbitrary chunks; a line may be completed in a later call.
type OutputPanel struct {
	State
	List     *VirtualList
	lines    []string
	partial  bool
	firstID  uint64
	maxLines int
}

type outputLines struct{ panel *OutputPanel }

func (m outputLines) Len() int { return len(m.panel.lines) }
func (m outputLines) ID(i int) string {
	return strconv.FormatUint(m.panel.firstID+uint64(i), 10)
}
func (m outputLines) Label(i int) string { return m.panel.lines[i] }

func NewOutputPanel() *OutputPanel {
	p := &OutputPanel{maxLines: 5000}
	p.owner, p.clip = p, true
	p.List = NewVirtualList(outputLines{panel: p})
	p.Add(p.List)
	return p
}

// Lines returns a snapshot of the visible log, including an unfinished line.
func (p *OutputPanel) Lines() []string {
	return append([]string(nil), p.lines...)
}

func (p *OutputPanel) Measure(c Constraints) Vec2 {
	return measureWithState(&p.State, p.List.Measure(c), c)
}

func (p *OutputPanel) Arrange(r Rect) {
	p.rect = r
	p.List.Arrange(r)
}

// Paint is handled by the virtual list child.
func (p *OutputPanel) Paint(*Canvas) {}

// SetMaxLines limits retained lines; values below one use the default limit.
func (p *OutputPanel) SetMaxLines(limit int) {
	if limit < 1 {
		limit = 5000
	}
	p.maxLines = limit
	p.trim()
	p.List.ModelChanged()
}

func (p *OutputPanel) trim() {
	if excess := len(p.lines) - p.maxLines; excess > 0 {
		copy(p.lines, p.lines[excess:])
		p.lines = p.lines[:len(p.lines)-excess]
		p.firstID += uint64(excess)
	}
}

// AppendText updates the last unfinished line, then adds any completed lines.
// A trailing newline does not create an empty extra row.
func (p *OutputPanel) AppendText(chunk string) {
	if chunk == "" {
		return
	}
	follow := p.atBottom()
	if p.partial && len(p.lines) > 0 {
		chunk = p.lines[len(p.lines)-1] + chunk
		p.lines = p.lines[:len(p.lines)-1]
	}
	parts := strings.Split(chunk, "\n")
	for _, part := range parts[:len(parts)-1] {
		p.lines = append(p.lines, strings.TrimSuffix(part, "\r"))
	}
	p.partial = !strings.HasSuffix(chunk, "\n")
	if p.partial {
		p.lines = append(p.lines, parts[len(parts)-1])
	}
	p.trim()
	p.List.ModelChanged()
	if follow {
		p.ScrollToEnd()
	}
}

func (p *OutputPanel) atBottom() bool {
	height := p.List.rowHeight()
	end := maxFloat32(0, float32(len(p.lines))*height-p.List.rect.H)
	return p.List.Offset >= end-1
}

// ScrollToEnd follows the most recent line of output.
func (p *OutputPanel) ScrollToEnd() {
	if n := len(p.lines); n != 0 {
		p.List.ScrollTo(n - 1)
	}
}

func (p *OutputPanel) Clear() {
	p.firstID += uint64(len(p.lines))
	p.lines = nil
	p.partial = false
	p.List.SelectedID = ""
	p.List.Offset = 0
	p.List.ModelChanged()
}
