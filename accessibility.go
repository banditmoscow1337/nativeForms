package nativeforms

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"sync/atomic"
	"unicode/utf8"
)

type AccessibleID uint64

var nextAccessibleID atomic.Uint64

type AccessibleRole string

const (
	RoleGroup       AccessibleRole = "group"
	RoleText        AccessibleRole = "text"
	RoleButton      AccessibleRole = "button"
	RoleCheckbox    AccessibleRole = "checkbox"
	RoleSwitch      AccessibleRole = "switch"
	RoleSlider      AccessibleRole = "slider"
	RoleProgressBar AccessibleRole = "progressbar"
	RoleTextField   AccessibleRole = "textfield"
	RoleDialog      AccessibleRole = "dialog"
)

type AccessibilityInfo struct {
	Role                        AccessibleRole
	Name, Description, Language string
	LabelledBy                  Component
	Hidden                      bool
}

func (s *State) SetAccessibility(info AccessibilityInfo) {
	s.accessibility = info
	s.invalidatePaint()
}
func (s *State) Accessibility() AccessibilityInfo { return s.accessibility }

func (s *State) AccessibilityID() AccessibleID {
	if s.accessibleID == 0 {
		s.accessibleID = AccessibleID(nextAccessibleID.Add(1))
	}
	return s.accessibleID
}

type AccessibleState uint32

const (
	AccessibleDisabled AccessibleState = 1 << iota
	AccessibleFocusable
	AccessibleFocused
	AccessibleChecked
	AccessibleReadOnly
	AccessibleMultiline
	AccessibleProtected
	AccessibleOffscreen
	AccessibleModal
)

type AccessibilityAction uint16

const (
	AccessibilityFocus AccessibilityAction = 1 << iota
	AccessibilityActivate
	AccessibilityIncrement
	AccessibilityDecrement
	AccessibilitySetValue
)

type AccessibleRange struct{ Min, Max, Value, Step float32 }
type AccessibleSelection struct{ Anchor, Cursor int }

type AccessibilityNode struct {
	ID, Parent                         AccessibleID
	Children                           []AccessibleID
	Role                               AccessibleRole
	Name, Description, Language, Value string
	LabelledBy                         AccessibleID
	Bounds, VisibleBounds              Rect
	State                              AccessibleState
	Actions                            AccessibilityAction
	Range                              *AccessibleRange
	Selection                          *AccessibleSelection
}

type AccessibilityTree struct {
	Roots   []AccessibleID
	Focused AccessibleID
	Nodes   []AccessibilityNode
}

func (t AccessibilityTree) Node(id AccessibleID) (AccessibilityNode, bool) {
	for _, node := range t.Nodes {
		if node.ID == id {
			return node, true
		}
	}
	return AccessibilityNode{}, false
}

func (m *Manager) AccessibilitySnapshot() AccessibilityTree {
	tree, _ := m.accessibilitySnapshot()
	return tree
}

func (m *Manager) accessibilitySnapshot() (AccessibilityTree, map[AccessibleID]Component) {
	var tree AccessibilityTree
	components := make(map[AccessibleID]Component)
	if !m.Visible() {
		return tree, components
	}
	interactive := m.Interactive()
	viewport := Rect{W: float32(m.layoutWidth), H: float32(m.layoutHeight)}
	var walk func(Component, AccessibleID, Rect, bool, string) AccessibleID
	walk = func(c Component, parent AccessibleID, clip Rect, disabled bool, language string) AccessibleID {
		if c == nil {
			return 0
		}
		s := c.UIState()
		if !s.Visible() || s.accessibility.Hidden || s.manager != m {
			return 0
		}
		id := s.AccessibilityID()
		if _, duplicate := components[id]; duplicate {
			return 0
		}
		components[id] = c
		if s.accessibility.Language != "" {
			language = s.accessibility.Language
		}
		node := describeAccessible(c)
		node.ID, node.Parent, node.Language = id, parent, language
		node.Bounds = s.rect
		node.VisibleBounds = intersect(s.rect, clip)
		if node.VisibleBounds.Empty() {
			node.State |= AccessibleOffscreen
		}
		disabled = disabled || !s.Enabled()
		if disabled {
			node.State |= AccessibleDisabled
			node.Actions = 0
		}
		if !interactive {
			node.Actions = 0
		}
		if s.Focused() && sameComponent(m.focused, c) {
			node.State |= AccessibleFocused
			tree.Focused = id
		}
		index := len(tree.Nodes)
		tree.Nodes = append(tree.Nodes, node)
		if s.clip || s.rect.Empty() {
			clip = intersect(clip, s.rect)
		}
		for _, child := range s.children {
			if childID := walk(child, id, clip, disabled, language); childID != 0 {
				tree.Nodes[index].Children = append(tree.Nodes[index].Children, childID)
			}
		}
		return id
	}
	language := "en"
	if m.root != nil && m.root.UIState().accessibility.Language != "" {
		language = m.root.UIState().accessibility.Language
	}
	if modal := m.topModal(); modal != nil {
		if id := walk(modal.content, 0, viewport, false, language); id != 0 {
			tree.Roots = append(tree.Roots, id)
			tree.Nodes[0].State |= AccessibleModal
		}
	} else {
		if id := walk(m.root, 0, viewport, false, language); id != 0 {
			tree.Roots = append(tree.Roots, id)
		}
		for _, popup := range m.popups {
			if !popup.passive {
				if id := walk(popup.content, 0, viewport, false, language); id != 0 {
					tree.Roots = append(tree.Roots, id)
				}
			}
		}
	}

	indices := make(map[AccessibleID]int, len(tree.Nodes))
	for i := range tree.Nodes {
		indices[tree.Nodes[i].ID] = i
	}

	for i := range tree.Nodes {
		node := &tree.Nodes[i]
		info := components[node.ID].UIState().accessibility
		if info.LabelledBy != nil {
			labelID := info.LabelledBy.UIState().accessibleID
			if label, ok := indices[labelID]; ok && labelID != node.ID {
				node.LabelledBy = labelID
				if info.Name == "" && tree.Nodes[label].Name != "" {
					node.Name = tree.Nodes[label].Name
				}
			}
		}
	}
	return tree, components
}

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
	case *Button:
		n.Role, n.Name = RoleButton, w.Text
		n.Actions |= AccessibilityActivate
	case *Checkbox:
		n.Role, n.Name = RoleCheckbox, w.Text
		if w.Checked {
			n.State |= AccessibleChecked
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
	case *ProgressBar:
		value := clampFloat32(w.Value, 0, 1)
		n.Role, n.Value = RoleProgressBar, fmt.Sprintf("%d%%", int(value*100+0.5))
		n.Range = &AccessibleRange{Min: 0, Max: 1, Value: value}
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
	if info.Name != "" {
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
	tree, components := m.accessibilitySnapshot()
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
	case *Checkbox:
		if action == AccessibilityActivate {
			w.SetChecked(!w.Checked)
			return true
		}
	case *Toggle:
		if action == AccessibilityActivate {
			w.SetValue(!w.Value)
			return true
		}
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
