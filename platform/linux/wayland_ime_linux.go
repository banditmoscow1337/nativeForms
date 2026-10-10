//go:build linux

package linux

import (
	"fmt"
	"unicode/utf8"

	ui "github.com/banditmoscow1337/nativeForms"
)

type wlTextInput struct {
	entered, enabled, hold bool
	state                  ui.TextInputState
	serial                 uint32
	preedit, commit        string
	before, after          uint32
	cursor, anchor         int32
}

func (w *WaylandWindow) textCommit() error {
	if err := w.wire.send(w.textInput, 7, nil); err != nil {
		return err
	}
	w.input.serial++
	return nil
}
func (w *WaylandWindow) syncTextInput(fromIME bool) error {
	if w.textInput == 0 || !w.input.entered || w.input.hold {
		return nil
	}
	state, focused := w.Manager.TextInputState()
	focused = focused && !w.modal
	i := &w.input
	if i.enabled && (!focused || state.Identity != i.state.Identity) {
		if err := w.wire.send(w.textInput, 2, nil); err != nil {
			return err
		}
		if err := w.textCommit(); err != nil {
			return err
		}
		i.enabled = false
		i.state = ui.TextInputState{}
	}
	if !focused {
		return nil
	}
	changed := !i.enabled || state != i.state
	if !changed {
		return nil
	}
	if !i.enabled {
		if err := w.wire.send(w.textInput, 1, nil); err != nil {
			return err
		}
		i.enabled = true
	}
	hints, purpose := uint32(0), uint32(0)
	if state.Multiline {
		hints |= 0x200
	}
	if state.Password {
		hints |= 0xc0
		purpose = 8
	}
	if err := w.wire.send(w.textInput, 5, append(wlWord(hints), wlWord(purpose)...)); err != nil {
		return err
	}
	if !state.Password {
		text, cursor, anchor, ok := surroundingText(state, 4000)
		if ok {
			args := append(wlString(text), wlWord(uint32(cursor))...)
			args = append(args, wlWord(uint32(anchor))...)
			if err := w.wire.send(w.textInput, 3, args); err != nil {
				return err
			}
		}
	}
	cause := uint32(1)
	if fromIME {
		cause = 0
	}
	if err := w.wire.send(w.textInput, 4, wlWord(cause)); err != nil {
		return err
	}
	r := state.Caret
	args := make([]byte, 16)
	for n, v := range []int{int(r.X), int(r.Y), max(1, int(r.W)), max(1, int(r.H))} {
		le.PutUint32(args[n*4:], uint32(int32(v)))
	}
	if err := w.wire.send(w.textInput, 6, args); err != nil {
		return err
	}
	i.state = state
	return w.textCommit()
}
func surroundingText(s ui.TextInputState, limit int) (string, int, int, bool) {
	a, b := min(s.Cursor, s.Anchor), max(s.Cursor, s.Anchor)
	if b-a > limit {
		return "", 0, 0, false
	}
	start := max(0, a-(limit-(b-a))/2)
	end := min(len(s.Text), start+limit)
	for start < a && !utf8.RuneStart(s.Text[start]) {
		start++
	}
	for end > b && end < len(s.Text) && !utf8.RuneStart(s.Text[end]) {
		end--
	}
	return s.Text[start:end], s.Cursor - start, s.Anchor - start, true
}
func wlReadTextInputString(p []byte) (string, int, error) {
	if len(p) < 4 {
		return "", 0, fmt.Errorf("wayland: short string")
	}
	n := uint64(le.Uint32(p))
	if n == 0 {
		return "", 4, nil
	}
	if n > uint64(len(p)-4) || 4+padded(int(n)) > len(p) || p[4+int(n)-1] != 0 {
		return "", 0, fmt.Errorf("wayland: invalid string")
	}
	s := string(p[4 : 4+int(n)-1])
	if !utf8.ValidString(s) {
		return "", 0, fmt.Errorf("wayland: non-UTF-8 string")
	}
	return s, 4 + padded(int(n)), nil
}
func (w *WaylandWindow) textInputEvent(op uint16, p []byte) error {
	i := &w.input
	switch op {
	case 0:
		if len(p) < 4 {
			return fmt.Errorf("wayland: short text enter")
		}
		i.entered = le.Uint32(p) == w.surface
		i.enabled = false
		i.state = ui.TextInputState{}
		i.hold = false
		return w.syncTextInput(false)
	case 1:
		w.Manager.HandleEvent(ui.Event{Type: ui.CompositionEnd})
		i.entered, i.enabled, i.hold = false, false, false
		i.preedit, i.commit = "", ""
		i.before, i.after = 0, 0
	case 2, 3:
		text, n, err := wlReadTextInputString(p)
		if err != nil {
			return err
		}
		if op == 2 {
			if len(p)-n < 8 {
				return fmt.Errorf("wayland: short preedit")
			}
			i.preedit = text
			i.cursor, i.anchor = int32(le.Uint32(p[n:])), int32(le.Uint32(p[n+4:]))
		} else {
			i.commit = text
		}
	case 4:
		if len(p) < 8 {
			return fmt.Errorf("wayland: short surrounding deletion")
		}
		i.before, i.after = le.Uint32(p), le.Uint32(p[4:])
	case 5:
		if len(p) < 4 {
			return fmt.Errorf("wayland: short text done")
		}
		state, ok := w.Manager.TextInputState()
		if i.enabled && ok && state.Identity == i.state.Identity {
			w.Manager.HandleEvent(ui.Event{Type: ui.CompositionEnd})
			if i.before != 0 || i.after != 0 || i.commit != "" {
				w.Manager.CommitTextInput(int(i.before), int(i.after), i.commit)
			}

			if i.preedit != "" {
				w.Manager.HandleEvent(ui.Event{Type: ui.CompositionStart})
				w.Manager.HandleEvent(ui.Event{Type: ui.CompositionUpdate, Text: i.preedit, CompositionSelection: true, CompositionCursor: preeditRuneOffset(i.preedit, i.cursor), CompositionAnchor: preeditRuneOffset(i.preedit, i.anchor)})
			}
		}
		i.preedit, i.commit = "", ""
		i.before, i.after = 0, 0
		i.hold = le.Uint32(p) != i.serial
		return w.syncTextInput(true)
	}
	return nil
}

func preeditRuneOffset(s string, at int32) int {
	if at < 0 {
		return -1
	}
	n := min(int(at), len(s))
	for n > 0 && !utf8.ValidString(s[:n]) {
		n--
	}
	return utf8.RuneCountInString(s[:n])
}
