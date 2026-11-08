//go:build darwin && (amd64 || arm64)

package darwin

import (
	"fmt"
	"structs"
	"unicode/utf16"
	"unicode/utf8"

	ui "github.com/banditmoscow1337/nativeForms"
	"github.com/banditmoscow1337/nativeForms/platform"
	"github.com/ebitengine/purego/objc"
)

type nsRange struct {
	_                structs.HostLayout
	Location, Length uint64
}

const nsNotFound = uint64(0x7fffffffffffffff)

var nativeViewClass objc.Class

func registerView() error {
	protocol := objc.GetProtocol("NSTextInputClient")
	if protocol == nil {
		return fmt.Errorf("darwin: NSTextInputClient unavailable")
	}
	methods := []objc.MethodDef{
		{Cmd: selector("windowShouldClose:"), Fn: func(self objc.ID, _ objc.SEL, _ objc.ID) bool {
			w := viewWindows[self]
			if w != nil {
				switch w.closeAction() {
				case platform.HideWindow:
					w.Hide()
				case platform.QuitApp:
					w.owner.Quit()
				case platform.CloseWindow:
					w.Close()
				}
			}
			return false // the App destroys the window after the callback returns
		}},
		{Cmd: selector("isFlipped"), Fn: func(objc.ID, objc.SEL) bool { return true }},
		{Cmd: selector("drawRect:"), Fn: func(self objc.ID, _ objc.SEL, _ rect) {
			w := viewWindows[self]
			if w == nil || w.representation == 0 {
				return
			}
			bounds := objc.Send[rect](self, selector("bounds"))
			objc.Send[bool](w.representation, selector("drawInRect:fromRect:operation:fraction:respectFlipped:hints:"),
				bounds, rect{}, uint64(1), float64(1), true, objc.ID(0))
		}},
		{Cmd: selector("acceptsFirstResponder"), Fn: func(objc.ID, objc.SEL) bool { return true }},
		{Cmd: selector("acceptsFirstMouse:"), Fn: func(objc.ID, objc.SEL, objc.ID) bool { return true }},
		{Cmd: selector("hasMarkedText"), Fn: func(self objc.ID, _ objc.SEL) bool { w := viewWindows[self]; return w != nil && w.marked != "" }},
		{Cmd: selector("markedRange"), Fn: func(self objc.ID, _ objc.SEL) nsRange {
			w := viewWindows[self]
			if w == nil || w.marked == "" {
				return nsRange{Location: nsNotFound}
			}
			r := w.committedSelection()
			r.Length = utf16Length(w.marked)
			return r
		}},
		{Cmd: selector("selectedRange"), Fn: func(self objc.ID, _ objc.SEL) nsRange {
			w := viewWindows[self]
			if w == nil {
				return nsRange{Location: nsNotFound}
			}
			r := w.committedSelection()
			if w.marked != "" {
				r.Location += w.markedSelection.Location
				r.Length = w.markedSelection.Length
			}
			return r
		}},
		{Cmd: selector("setMarkedText:selectedRange:replacementRange:"), Fn: func(self objc.ID, _ objc.SEL, value objc.ID, selected, replacement nsRange) {
			w := viewWindows[self]
			if w == nil {
				return
			}
			w.nativeReplacement(replacement)
			w.marked = plainString(value)
			n := utf16Length(w.marked)
			selected.Location = min(selected.Location, n)
			selected.Length = min(selected.Length, n-selected.Location)
			w.markedSelection = selected
			a, b, ok := utf16RangeBytes(w.marked, selected)
			if !ok {
				a, b = len(w.marked), len(w.marked)
			}
			w.manager.HandleEvent(ui.Event{Type: ui.CompositionUpdate, Text: w.marked, CompositionSelection: true, CompositionCursor: utf8.RuneCountInString(w.marked[:b]), CompositionAnchor: utf8.RuneCountInString(w.marked[:a])})
			w.inputHandled = true
		}},
		{Cmd: selector("insertText:replacementRange:"), Fn: func(self objc.ID, _ objc.SEL, value objc.ID, replacement nsRange) {
			w := viewWindows[self]
			if w == nil {
				return
			}
			w.nativeReplacement(replacement)
			w.marked = ""
			w.manager.HandleEvent(ui.Event{Type: ui.CompositionEnd, Text: plainString(value)})
			w.inputHandled = true
		}},
		{Cmd: selector("unmarkText"), Fn: func(self objc.ID, _ objc.SEL) {
			w := viewWindows[self]
			if w == nil {
				return
			}
			value := w.marked
			w.marked = ""
			w.manager.HandleEvent(ui.Event{Type: ui.CompositionEnd, Text: value})
		}},
		{Cmd: selector("validAttributesForMarkedText"), Fn: func(objc.ID, objc.SEL) objc.ID { return class("NSArray").Send(selector("array")) }},
		{Cmd: selector("attributedSubstringForProposedRange:actualRange:"), Fn: func(self objc.ID, _ objc.SEL, requested nsRange, actual *nsRange) objc.ID {
			if actual != nil {
				*actual = nsRange{Location: nsNotFound}
			}
			w := viewWindows[self]
			if w == nil {
				return 0
			}
			state, ok := w.manager.TextInputState()
			if !ok || state.Password {
				return 0
			}
			value := state.Text
			if w.marked != "" {
				a, b := min(state.Cursor, state.Anchor), max(state.Cursor, state.Anchor)
				value = value[:a] + w.marked + value[b:]
			}
			n := uint64(state.UTF16Base) + utf16Length(value)
			if requested.Location < uint64(state.UTF16Base) || requested.Location > n {
				return 0
			}
			requested.Length = min(requested.Length, n-requested.Location)
			local := requested
			local.Location -= uint64(state.UTF16Base)
			start, end, ok := utf16RangeBytes(value, local)
			if !ok {
				return 0
			}
			if actual != nil {
				*actual = requested
			}
			return class("NSAttributedString").Send(selector("alloc")).Send(selector("initWithString:"), nsString(value[start:end])).Send(selector("autorelease"))
		}},
		{Cmd: selector("characterIndexForPoint:"), Fn: func(objc.ID, objc.SEL, point) uint64 { return nsNotFound }},
		{Cmd: selector("firstRectForCharacterRange:actualRange:"), Fn: func(self objc.ID, _ objc.SEL, _ nsRange, actual *nsRange) rect {
			w := viewWindows[self]
			if w == nil {
				return rect{}
			}
			if actual != nil {
				*actual = w.committedSelection()
			}
			r, ok := w.manager.FocusedCaretRect()
			if !ok {
				return rect{}
			}
			local := rect{Origin: point{X: float64(r.X), Y: float64(r.Y)}, Size: size{W: float64(max(float32(1), r.W)), H: float64(r.H)}}
			inWindow := objc.Send[rect](self, selector("convertRect:toView:"), local, objc.ID(0))
			return objc.Send[rect](w.window, selector("convertRectToScreen:"), inWindow)
		}},
		{Cmd: selector("doCommandBySelector:"), Fn: func(self objc.ID, _ objc.SEL, command objc.SEL) {
			w := viewWindows[self]
			if w == nil {
				return
			}
			e := w.currentKey
			e.Type = ui.KeyDown
			for _, m := range []struct {
				name   string
				key    ui.Key
				extend bool
			}{
				{"insertNewline:", ui.KeyEnter, false}, {"insertTab:", ui.KeyTab, false}, {"insertBacktab:", ui.KeyTab, true},
				{"deleteBackward:", ui.KeyBackspace, false}, {"deleteForward:", ui.KeyDelete, false}, {"cancelOperation:", ui.KeyEscape, false},
				{"moveLeft:", ui.KeyLeftArrow, false}, {"moveRight:", ui.KeyRightArrow, false}, {"moveUp:", ui.KeyUpArrow, false}, {"moveDown:", ui.KeyDownArrow, false},
				{"moveLeftAndModifySelection:", ui.KeyLeftArrow, true}, {"moveRightAndModifySelection:", ui.KeyRightArrow, true}, {"moveUpAndModifySelection:", ui.KeyUpArrow, true}, {"moveDownAndModifySelection:", ui.KeyDownArrow, true},
				{"moveToBeginningOfLine:", ui.KeyHome, false}, {"moveToEndOfLine:", ui.KeyEnd, false},
				{"pageUp:", ui.KeyPageUp, false}, {"pageDown:", ui.KeyPageDown, false},
				{"movePageUp:", ui.KeyPageUp, false}, {"movePageDown:", ui.KeyPageDown, false},
				{"movePageUpAndModifySelection:", ui.KeyPageUp, true}, {"movePageDownAndModifySelection:", ui.KeyPageDown, true},
			} {
				if command == selector(m.name) {
					e.Key = m.key
					e.Mods = 0
					if m.extend {
						e.Mods |= ui.ModShift
					}
					break
				}
			}
			if !w.inputHandled {
				w.manager.HandleEvent(e)
				w.inputHandled = true
			}
		}},
		{Cmd: selector("draggingEntered:"), Fn: func(self objc.ID, _ objc.SEL, sender objc.ID) uint64 {
			w := viewWindows[self]
			if w != nil && w.options.OnFilesDropped != nil && objc.Send[uint64](sender, selector("draggingSourceOperationMask"))&1 != 0 {
				return 1
			}
			return 0
		}},
		{Cmd: selector("draggingUpdated:"), Fn: func(self objc.ID, _ objc.SEL, sender objc.ID) uint64 {
			w := viewWindows[self]
			if w != nil && w.options.OnFilesDropped != nil && objc.Send[uint64](sender, selector("draggingSourceOperationMask"))&1 != 0 {
				return 1
			}
			return 0
		}},
		{Cmd: selector("prepareForDragOperation:"), Fn: func(self objc.ID, _ objc.SEL, _ objc.ID) bool {
			w := viewWindows[self]
			return w != nil && w.options.OnFilesDropped != nil
		}},
		{Cmd: selector("performDragOperation:"), Fn: func(self objc.ID, _ objc.SEL, sender objc.ID) bool {
			w := viewWindows[self]
			if w == nil || w.options.OnFilesDropped == nil {
				return false
			}
			board := sender.Send(selector("draggingPasteboard"))
			classes := class("NSArray").Send(selector("arrayWithObject:"), class("NSURL"))
			urls := board.Send(selector("readObjectsForClasses:options:"), classes, objc.ID(0))
			if urls != 0 {
				n := objc.Send[uint64](urls, selector("count"))
				if n > 0 && n <= 4096 {
					files := make([]string, 0, n)
					for j := uint64(0); j < n; j++ {
						u := urls.Send(selector("objectAtIndex:"), j)
						if objc.Send[bool](u, selector("isFileURL")) {
							if path := stringValue(u.Send(selector("path"))); path != "" {
								files = append(files, path)
							}
						}
					}
					if len(files) > 0 {
						w.options.OnFilesDropped(files)
						return true
					}
				}
			}
			values := board.Send(selector("propertyListForType:"), nsString("NSFilenamesPboardType"))
			if values == 0 || !objc.Send[bool](values, selector("isKindOfClass:"), class("NSArray")) {
				return false
			}
			n := objc.Send[uint64](values, selector("count"))
			if n == 0 || n > 4096 {
				return false
			}
			files := make([]string, 0, n)
			for j := uint64(0); j < n; j++ {
				v := values.Send(selector("objectAtIndex:"), j)
				if !objc.Send[bool](v, selector("isKindOfClass:"), class("NSString")) {
					return false
				}
				path := stringValue(v)
				if path != "" {
					files = append(files, path)
				}
			}
			if len(files) == 0 {
				return false
			}
			w.options.OnFilesDropped(files)
			return true
		}},
	}
	var err error
	nativeViewClass, err = objc.RegisterClass("NativeFormsInputView", objc.GetClass("NSView"), []*objc.Protocol{protocol}, nil, methods)
	return err
}
func plainString(value objc.ID) string {
	if value == 0 {
		return ""
	}
	if objc.Send[bool](value, selector("isKindOfClass:"), class("NSAttributedString")) {
		value = value.Send(selector("string"))
	}
	return stringValue(value)
}
func utf16Length(value string) uint64 {
	n := uint64(0)
	for _, r := range value {
		if r > 0xffff {
			n += 2
		} else {
			n++
		}
	}
	return n
}
func utf16RangeBytes(value string, r nsRange) (int, int, bool) {
	if r.Location == nsNotFound || r.Length > ^uint64(0)-r.Location {
		return 0, 0, false
	}
	a, b := -1, -1
	pos := uint64(0)
	for offset, c := range value {
		if pos == r.Location {
			a = offset
		}
		if pos == r.Location+r.Length {
			b = offset
		}
		pos += uint64(len(utf16.Encode([]rune{c})))
	}
	if pos == r.Location {
		a = len(value)
	}
	if pos == r.Location+r.Length {
		b = len(value)
	}
	return a, b, a >= 0 && b >= a
}
func (w *Window) committedSelection() nsRange {
	s, ok := w.manager.TextInputState()
	if !ok {
		return nsRange{Location: nsNotFound}
	}
	a, b := min(s.Cursor, s.Anchor), max(s.Cursor, s.Anchor)
	return nsRange{Location: uint64(s.UTF16Base) + utf16Length(s.Text[:a]), Length: utf16Length(s.Text[a:b])}
}
func (w *Window) nativeReplacement(r nsRange) {
	if r.Location == nsNotFound {
		return
	}
	s, ok := w.manager.TextInputState()
	if !ok || s.Password {
		return
	}
	base := w.committedSelection()
	if w.marked != "" && r.Location >= base.Location && r.Location <= base.Location+utf16Length(w.marked) {
		return
	}
	if r.Location >= uint64(s.UTF16Base) {
		local := r
		local.Location -= uint64(s.UTF16Base)
		if a, b, ok := utf16RangeBytes(s.Text, local); ok {
			w.manager.SelectTextRange(s.Base+a, s.Base+b)
		}
	}
}
func (w *Window) cancelMarked() {
	if w.marked != "" {
		w.marked = ""
		w.manager.HandleEvent(ui.Event{Type: ui.CompositionEnd})
	}
	if w.view != 0 {
		context := w.view.Send(selector("inputContext"))
		if context != 0 {
			context.Send(selector("discardMarkedText"))
		}
	}
}
func (w *Window) syncInputFocus() {
	if !w.manager.TextInputFocused() && w.inputIdentity == nil {
		return
	}
	s, ok := w.manager.TextInputState()
	var id *ui.State
	if ok {
		id = s.Identity
	}
	if id != w.inputIdentity {
		w.cancelMarked()
		w.inputIdentity = id
	}
	if w.view != 0 && ok {
		context := msgSend0(w.view, selector("inputContext"))
		if context != 0 {
			msgSend0(context, selector("invalidateCharacterCoordinates"))
		}
	}
}
