package nativeforms

import (
	"math"
	"strconv"
	"strings"
	"unicode/utf8"
)

func describeAccessible(c Component) AccessibilityNode {
	s := c.UIState()
	n := AccessibilityNode{Role: RoleGroup, Description: s.tooltip}
	if s.Focusable() {
		n.State |= AccessibleFocusable
		n.Actions |= AccessibilityFocus
	}
	switch w := c.(type) {
	case *Label:
		n.Role, n.Name = RoleText, w.currentText()
	case *Badge:
		n.Role, n.Name, n.Value = RoleStatus, w.Text, w.Text
		n.State |= AccessibleReadOnly
	case *Separator:
		n.Role, n.Name = RoleSeparator, w.Text
		n.State |= AccessibleReadOnly
	case *Hyperlink:
		n.Role, n.Name, n.Value = RoleLink, w.Text, w.URL
		n.Actions |= AccessibilityActivate
	case *Button:
		n.Role, n.Name = RoleButton, w.Text
		if w.Active {
			n.State |= AccessibleChecked
		}
		n.Actions |= AccessibilityActivate
	case *Checkbox:
		n.Role, n.Name = RoleCheckbox, w.Text
		if w.Checked {
			n.State |= AccessibleChecked
		}
		if w.Indeterminate {
			n.Value = "mixed"
		}
		n.Actions |= AccessibilityActivate
	case *Toggle:
		n.Role, n.Name = RoleSwitch, w.Text
		if w.Value {
			n.State |= AccessibleChecked
		}
		n.Actions |= AccessibilityActivate
	case *Slider:
		minimum, maximum := w.bounds()
		n.Role, n.Name, n.Value = RoleSlider, w.Text, w.formattedValue()
		n.Range = &AccessibleRange{Min: minimum, Max: maximum, Value: w.Value, Step: w.Step}
		n.Actions |= AccessibilityIncrement | AccessibilityDecrement | AccessibilitySetValue
	case *SpinBox:
		minimum, maximum := w.bounds()
		n.Role, n.Name, n.Value = RoleSpinBox, w.Text, w.FormattedValue()
		n.Range = &AccessibleRange{Min: minimum, Max: maximum, Value: w.Value, Step: w.Step}
		n.Actions |= AccessibilityIncrement | AccessibilityDecrement | AccessibilitySetValue
	case *SegmentedControl:
		n.Role, n.Value = RoleTabList, w.SelectedID
		n.Actions |= AccessibilityActivate | AccessibilityIncrement | AccessibilityDecrement | AccessibilitySetValue
	case *Collapsible:
		n.Role, n.Name = RoleCollapsible, w.Title
		if w.Expanded {
			n.State |= AccessibleChecked
		}
		n.Actions |= AccessibilityActivate
	case *Toolbar:
		n.Role = RoleToolbar
		n.Actions |= AccessibilityActivate
	case *ColorPicker:
		n.Role, n.Name, n.Value = RoleColorPicker, w.Text, w.Hex()
		n.Actions |= AccessibilityActivate | AccessibilityIncrement | AccessibilityDecrement | AccessibilitySetValue
	case *InteractiveCanvas:
		n.Role = RoleCanvas
		n.Value = w.OverlayBadge
	case *SwatchGrid:
		n.Role, n.Value = RoleColorPicker, w.Selected.Hex()
		n.Actions |= AccessibilityActivate | AccessibilityIncrement | AccessibilityDecrement | AccessibilitySetValue
	case *LayerList:
		n.Role = RoleList
		if w.SelectedIndex >= 0 && w.SelectedIndex < len(w.Items) {
			n.Value = w.Items[w.SelectedIndex].Name
		}
		n.Actions |= AccessibilityIncrement | AccessibilityDecrement
	case *FrameStrip:
		n.Role = RoleTabList
		if w.SelectedIndex >= 0 && w.SelectedIndex < len(w.Items) {
			n.Value = w.Items[w.SelectedIndex].Title
		}
		n.Actions |= AccessibilityIncrement | AccessibilityDecrement
	case *ProgressBar:
		value := clampFloat32(w.Value, 0, 1)
		n.Role, n.Value = RoleProgressBar, w.percentString()
		if !w.Indeterminate {
			n.Range = &AccessibleRange{Min: 0, Max: 1, Value: value}
		}
		n.State |= AccessibleReadOnly
	case *TextField:
		e := w.Editor()
		n.Role, n.Name = RoleTextField, w.Placeholder
		if w.ReadOnly {
			n.State |= AccessibleReadOnly
		} else {
			n.Actions |= AccessibilitySetValue
		}
		if w.Multiline {
			n.State |= AccessibleMultiline
		}
		if w.Password {
			n.State |= AccessibleProtected
		} else {
			n.Value = e.Text
			n.Selection = &AccessibleSelection{
				Anchor: textByteOffset(e.Text, e.Anchor),
				Cursor: textByteOffset(e.Text, e.Caret),
			}
		}
	}
	info := s.accessibility
	if info.Role != "" {
		n.Role = info.Role
	}
	if info.Name == "" && n.Name == "" {
		// keep empty
	} else if info.Name != "" {
		n.Name = info.Name
	}
	if info.Description != "" {
		n.Description = info.Description
	}
	return n
}

func (m *Manager) PerformAccessibilityAction(id AccessibleID, action AccessibilityAction, value string) bool {
	if action == 0 || action&(action-1) != 0 || !m.Visible() || !m.Interactive() {
		return false
	}
	tree, components := m.accessibilitySnapshot(true)
	node, ok := tree.Node(id)
	if !ok || node.Actions&action == 0 {
		return false
	}
	c := components[id]
	if action == AccessibilityFocus {
		m.setFocus(c)
		current := m.AccessibilitySnapshot()
		now, exists := current.Node(id)
		if !exists || now.Actions&AccessibilityFocus == 0 {
			if sameComponent(m.focused, c) {
				m.setFocus(nil)
			}
			return false
		}
		return sameComponent(m.focused, c)
	}
	switch w := c.(type) {
	case *Button:
		if action == AccessibilityActivate {
			if w.OnClick != nil {
				w.OnClick()
			}
			m.InvalidatePaint()
			return true
		}
	case *Hyperlink:
		if action == AccessibilityActivate {
			w.Activate()
			m.InvalidatePaint()
			return true
		}
	case *Checkbox:
		if action == AccessibilityActivate {
			if w.Indeterminate {
				w.SetChecked(true)
			} else {
				w.SetChecked(!w.Checked)
			}
			return true
		}
	case *Toggle:
		if action == AccessibilityActivate {
			w.SetValue(!w.Value)
			return true
		}
	case *Collapsible:
		if action == AccessibilityActivate {
			w.Toggle()
			return true
		}
	case *SegmentedControl:
		if len(w.Options) == 0 {
			return false
		}
		switch action {
		case AccessibilitySetValue:
			w.SetSelected(value)
			return w.SelectedID == value
		case AccessibilityActivate, AccessibilityIncrement:
			cur := max(0, w.SelectedIndex())
			w.SetSelected(w.Options[(cur+1)%len(w.Options)].ID)
			return true
		case AccessibilityDecrement:
			cur := max(0, w.SelectedIndex())
			w.SetSelected(w.Options[(cur-1+len(w.Options))%len(w.Options)].ID)
			return true
		}
	case *SpinBox:
		next := w.Value
		switch action {
		case AccessibilitySetValue:
			v, err := strconv.ParseFloat(value, 32)
			if err != nil || math.IsNaN(v) || math.IsInf(v, 0) {
				return false
			}
			next = float32(v)
		case AccessibilityIncrement:
			next += w.stepSize()
		case AccessibilityDecrement:
			next -= w.stepSize()
		default:
			return false
		}
		w.SetValue(next)
		return true
	case *Slider:
		next := w.Value
		switch action {
		case AccessibilitySetValue:
			v, err := strconv.ParseFloat(value, 32)
			if err != nil || math.IsNaN(v) || math.IsInf(v, 0) {
				return false
			}
			next = float32(v)
		case AccessibilityIncrement, AccessibilityDecrement:
			step := w.stepSize()
			if action == AccessibilityDecrement {
				step = -step
			}
			next += step
		default:
			return false
		}
		w.SetValue(next)
		return true
	case *ColorPicker:
		switch action {
		case AccessibilitySetValue:
			return w.SetHex(value)
		case AccessibilityIncrement:
			h, s, v, a := w.HSVA()
			w.SetHSVA(h+15, s, v, a)
			return true
		case AccessibilityDecrement:
			h, s, v, a := w.HSVA()
			w.SetHSVA(h-15, s, v, a)
			return true
		case AccessibilityActivate:
			if w.Compact {
				return w.openPopup()
			}
			if len(w.Presets) > 0 {
				w.SetValue(w.Presets[0])
				return true
			}
		}
	case *SwatchGrid:
		if len(w.Colors) == 0 {
			return false
		}
		switch action {
		case AccessibilitySetValue:
			col, ok := ParseHexColor(value)
			if !ok {
				return false
			}
			w.SetSelected(col)
			return true
		case AccessibilityActivate, AccessibilityIncrement:
			cur := max(0, w.SelectedIndex())
			return w.selectIndex((cur + 1) % len(w.Colors))
		case AccessibilityDecrement:
			cur := max(0, w.SelectedIndex())
			return w.selectIndex((cur - 1 + len(w.Colors)) % len(w.Colors))
		}
	case *LayerList:
		if len(w.Items) == 0 {
			return false
		}
		switch action {
		case AccessibilityIncrement:
			if w.SelectedIndex+1 < len(w.Items) {
				w.SelectedIndex++
				if w.OnSelect != nil {
					w.OnSelect(w.SelectedIndex)
				}
				return true
			}
		case AccessibilityDecrement:
			if w.SelectedIndex > 0 {
				w.SelectedIndex--
				if w.OnSelect != nil {
					w.OnSelect(w.SelectedIndex)
				}
				return true
			}
		}
	case *FrameStrip:
		if len(w.Items) == 0 {
			return false
		}
		switch action {
		case AccessibilityIncrement:
			if w.SelectedIndex+1 < len(w.Items) {
				w.SelectedIndex++
				if w.OnSelect != nil {
					w.OnSelect(w.SelectedIndex)
				}
				return true
			}
		case AccessibilityDecrement:
			if w.SelectedIndex > 0 {
				w.SelectedIndex--
				if w.OnSelect != nil {
					w.OnSelect(w.SelectedIndex)
				}
				return true
			}
		}
	case *TextField:
		if action == AccessibilitySetValue && !w.ReadOnly && utf8.ValidString(value) {
			if !w.Multiline {
				value = strings.ReplaceAll(normalizeTextBreaks(value), "\n", "")
			}
			e := w.Editor()
			if w.Validate != nil && !w.Validate(value) {
				return false
			}
			if e.Text == value {
				return true
			}
			e.Anchor, e.Caret, e.Preedit = 0, utf8.RuneCountInString(e.Text), ""
			if e.Replace(value) {
				w.changed()
				return true
			}
		}
	}
	return false
}
