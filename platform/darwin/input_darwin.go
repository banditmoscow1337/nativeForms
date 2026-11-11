//go:build darwin && (amd64 || arm64)

package darwin

import (
	"fmt"
	"structs"
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

var (
	nativeViewClass   objc.Class
	nativeWindowClass objc.Class
	trayTargetClass   objc.Class
	trayTargetID      objc.ID
)

func registerView() error {
	protocol := objc.GetProtocol("NSTextInputClient")
	if protocol == nil {
		return fmt.Errorf("darwin: NSTextInputClient unavailable")
	}
	dragAccept := func(self objc.ID, _ objc.SEL, sender objc.ID) uint64 {
		w := viewWindows[self]
		if w != nil && w.options.OnFilesDropped != nil && objc.Send[uint64](sender, selector("draggingSourceOperationMask"))&1 != 0 {
			return 1
		}
		return 0
	}
	invalidateGeom := func(self objc.ID, _ objc.SEL, _ objc.ID) {
		if w := viewWindows[self]; w != nil {
			w.geomValid = false
			w.manager.InvalidateLayout()
		}
	}
	methods := []objc.MethodDef{
		{Cmd: selector("windowDidResize:"), Fn: invalidateGeom},
		{Cmd: selector("windowDidChangeBackingProperties:"), Fn: invalidateGeom},
		{Cmd: selector("windowDidBecomeKey:"), Fn: invalidateGeom},
		{Cmd: selector("windowDidResignKey:"), Fn: invalidateGeom},
		{Cmd: selector("windowDidMiniaturize:"), Fn: invalidateGeom},
		{Cmd: selector("windowDidDeminiaturize:"), Fn: invalidateGeom},
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
		{Cmd: selector("wantsUpdateLayer"), Fn: func(self objc.ID, _ objc.SEL) bool {
			w := viewWindows[self]
			return w != nil && w.representation == 0
		}},
		{Cmd: selector("updateLayer"), Fn: func(objc.ID, objc.SEL) {}},
		{Cmd: selector("drawRect:"), Fn: func(self objc.ID, _ objc.SEL, _ rect) {
			w := viewWindows[self]
			if w == nil || w.representation == 0 {
				return
			}
			bounds := msgSendRect0(self, selBounds)
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
			uiScale := float64(w.uiScale())
			local := rect{Origin: point{X: float64(r.X) * uiScale, Y: float64(r.Y) * uiScale}, Size: size{W: float64(max(float32(1), r.W)) * uiScale, H: float64(r.H) * uiScale}}
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
		{Cmd: selector("draggingEntered:"), Fn: dragAccept},
		{Cmd: selector("draggingUpdated:"), Fn: dragAccept},
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
	if err != nil {
		return err
	}
	// Borderless NSWindow instances return false from canBecomeKeyWindow and
	// canBecomeMainWindow by default; subclass NSWindow so frameless windows
	// continue to receive keyboard focus and IME events.
	winMethods := []objc.MethodDef{
		{Cmd: selector("canBecomeKeyWindow"), Fn: func(objc.ID, objc.SEL) bool { return true }},
		{Cmd: selector("canBecomeMainWindow"), Fn: func(objc.ID, objc.SEL) bool { return true }},
	}
	nativeWindowClass, err = objc.RegisterClass("NativeFormsWindow", objc.GetClass("NSWindow"), nil, nil, winMethods)
	if err != nil {
		return err
	}
	trayMethods := []objc.MethodDef{
		{Cmd: selector("trayItemClicked:"), Fn: func(_ objc.ID, _ objc.SEL, sender objc.ID) {
			if activeApplication == nil || activeApplication.owner == nil {
				return
			}
			tag := int(objc.Send[int64](sender, selector("tag")))
			activeApplication.owner.invokeTrayAction(tag)
		}},
	}
	trayTargetClass, err = objc.RegisterClass("NativeFormsTrayTarget", objc.GetClass("NSObject"), nil, nil, trayMethods)
	if err != nil {
		return err
	}
	trayTargetID = objc.ID(trayTargetClass).Send(selector("new"))
	return nil
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

func utf16RuneUnits(r rune) uint64 {
	if r > 0xffff {
		return 2
	}
	return 1
}

func utf16Length(value string) uint64 {
	n := uint64(0)
	for _, r := range value {
		n += utf16RuneUnits(r)
	}
	return n
}

func utf16RangeBytes(value string, r nsRange) (int, int, bool) {
	if r.Location == nsNotFound || r.Length > ^uint64(0)-r.Location {
		return 0, 0, false
	}
	endTarget := r.Location + r.Length
	a, b := -1, -1
	pos := uint64(0)
	for offset, c := range value {
		if pos == r.Location {
			a = offset
		}
		if pos == endTarget {
			b = offset
			if a >= 0 {
				return a, b, true
			}
		}
		pos += utf16RuneUnits(c)
	}
	if pos == r.Location {
		a = len(value)
	}
	if pos == endTarget {
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
		context := msgSend0(w.view, selInputContext)
		if context != 0 {
			msgSend0(context, selector("discardMarkedText"))
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
	identityChanged := id != w.inputIdentity
	if identityChanged {
		w.cancelMarked()
		w.inputIdentity = id
	}
	if w.view != 0 && ok {
		if identityChanged || w.marked != "" || s.Caret != w.lastInputCaret ||
			s.Cursor != w.lastInputCursor || s.Anchor != w.lastInputAnchor || s.Base != w.lastInputBase {
			w.lastInputCaret = s.Caret
			w.lastInputCursor = s.Cursor
			w.lastInputAnchor = s.Anchor
			w.lastInputBase = s.Base
			context := msgSend0(w.view, selInputContext)
			if context != 0 {
				msgSend0(context, selInvalidateCoords)
			}
		}
	}
}
