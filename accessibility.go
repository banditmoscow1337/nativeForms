package nativeforms

import "sync/atomic"

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
	RoleSpinBox     AccessibleRole = "spinbox"
	RoleProgressBar AccessibleRole = "progressbar"
	RoleTextField   AccessibleRole = "textfield"
	RoleDialog      AccessibleRole = "dialog"
	RoleMenu        AccessibleRole = "menu"
	RoleColorPicker AccessibleRole = "colorpicker"
	RoleLink        AccessibleRole = "link"
	RoleTabList     AccessibleRole = "tablist"
	RoleSeparator   AccessibleRole = "separator"
	RoleToolbar     AccessibleRole = "toolbar"
	RoleStatus      AccessibleRole = "status"
	RoleCollapsible AccessibleRole = "collapsible"
	RoleCanvas      AccessibleRole = "canvas"
	RoleList        AccessibleRole = "list"
)

type AccessibilityInfo struct {
	Role                        AccessibleRole
	Name, Description, Language string
	LabelledBy                  Component
	Hidden                      bool
}

func (s *State) SetAccessibility(info AccessibilityInfo) {
	s.accessibility = info
	s.InvalidatePaint()
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
	tree, _ := m.accessibilitySnapshot(false)
	return tree
}

func (m *Manager) accessibilitySnapshot(needMap bool) (AccessibilityTree, map[AccessibleID]Component) {
	var tree AccessibilityTree
	if !m.Visible() {
		return tree, nil
	}
	var components map[AccessibleID]Component
	if needMap {
		components = make(map[AccessibleID]Component, 64)
	}
	tree.Nodes = make([]AccessibilityNode, 0, 64)
	var nodeCompsBuf [128]Component
	nodeComps := nodeCompsBuf[:0]
	hasLabelledBy := false
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
		if components != nil {
			if _, duplicate := components[id]; duplicate {
				return 0
			}
			components[id] = c
		}
		if s.accessibility.LabelledBy != nil {
			hasLabelledBy = true
		}
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
		nodeComps = append(nodeComps, c)
		if s.clip || s.rect.Empty() {
			clip = intersect(clip, s.rect)
		}
		if len(s.children) > 0 {
			for _, child := range s.children {
				if childID := walk(child, id, clip, disabled, language); childID != 0 {
					if tree.Nodes[index].Children == nil {
						tree.Nodes[index].Children = make([]AccessibleID, 0, len(s.children))
					}
					tree.Nodes[index].Children = append(tree.Nodes[index].Children, childID)
				}
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

	if hasLabelledBy {
		for i := range tree.Nodes {
			node := &tree.Nodes[i]
			info := nodeComps[i].UIState().accessibility
			if info.LabelledBy != nil {
				labelID := info.LabelledBy.UIState().accessibleID
				if labelID != 0 && labelID != node.ID {
					for j := range tree.Nodes {
						if tree.Nodes[j].ID == labelID {
							node.LabelledBy = labelID
							if info.Name == "" && tree.Nodes[j].Name != "" {
								node.Name = tree.Nodes[j].Name
							}
							break
						}
					}
				}
			}
		}
	}
	return tree, components
}
