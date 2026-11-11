package nativeforms

type Tab struct {
	ID, Title string
	Icon      *ImageResource
	Content   Component
	Dirty     bool
	HasError  bool
	// NonClosable hides the close button and prevents Close from removing this tab.
	// Tabs are closable by default for compatibility with existing applications.
	NonClosable bool
}

type Tabs struct {
	State
	Tabs             []Tab
	SelectedID       string
	OnChange         func(string)
	OnCloseRequest   func(string) bool
	OnReorder        func(id string, index int)
	OnTabContextMenu func(id string, x, y float32) []MenuItem
	OnFocus          func()
	Offset           float32

	headerHeight float32
	downID       string
	downX        float32
	downClose    bool
	dragged      bool
}

func NewTabs(tabs []Tab, selected string, onChange func(string)) *Tabs {
	t := &Tabs{Tabs: append([]Tab(nil), tabs...), SelectedID: selected, OnChange: onChange, headerHeight: 38}
	t.owner, t.focusable = t, true
	t.minimum = Vec2{X: 120, Y: 38}
	for _, tab := range t.Tabs {
		if tab.Content != nil {
			t.Add(tab.Content)
		}
	}
	t.selectTab(selected, false)
	return t
}

func (t *Tabs) selectedContent() Component {
	for _, tab := range t.Tabs {
		if tab.ID == t.SelectedID {
			return tab.Content
		}
	}
	return nil
}

func (t *Tabs) ownsFocus(content Component) bool {
	return t.manager != nil && t.manager.focused != nil &&
		componentContainsState(content, t.manager.focused.UIState())
}

func (t *Tabs) focusSelected() {
	if t.manager == nil {
		return
	}
	var focusables []Component
	collectFocusable(t.selectedContent(), &focusables)
	if len(focusables) > 0 {
		t.manager.setFocus(focusables[0])
	}
}

func (t *Tabs) selectTab(id string, notify bool) {
	found := false
	for _, tab := range t.Tabs {
		if tab.ID == id {
			found = true
			break
		}
	}
	if !found {
		id = ""
		if len(t.Tabs) > 0 {
			id = t.Tabs[0].ID
		}
	}
	if id == t.SelectedID && notify {
		return
	}
	focusContent := t.manager != nil && (sameComponent(t.manager.focused, t) ||
		t.ownsFocus(t.selectedContent()))
	changed := id != t.SelectedID
	t.SelectedID = id
	for _, tab := range t.Tabs {
		if tab.Content != nil {
			tab.Content.UIState().SetVisible(tab.ID == id)
		}
	}
	t.ScrollTo(id)
	t.invalidateLayout()
	if focusContent {
		t.focusSelected()
	}
	if notify && changed && t.OnChange != nil {
		t.OnChange(id)
	}
}

func (t *Tabs) Select(id string) { t.selectTab(id, true) }

// Open adds a tab and selects it. IDs must be unique and nonempty.
func (t *Tabs) Open(tab Tab) bool {
	if tab.ID == "" {
		return false
	}
	for _, existing := range t.Tabs {
		if existing.ID == tab.ID {
			return false
		}
	}
	t.Tabs = append(t.Tabs, tab)
	if tab.Content != nil {
		t.Add(tab.Content)
	}
	t.selectTab(tab.ID, true)
	return true
}

// Close removes a tab after OnCloseRequest, if set, allows it.
func (t *Tabs) Close(id string) bool {
	for index, tab := range t.Tabs {
		if tab.ID != id {
			continue
		}
		if tab.NonClosable {
			return false
		}
		if t.OnCloseRequest != nil && !t.OnCloseRequest(id) {
			return false
		}
		selected := id == t.SelectedID
		focused := selected && t.manager != nil && t.ownsFocus(tab.Content)
		if tab.Content != nil {
			t.Remove(tab.Content)
			ReleaseComponent(tab.Content)
		}
		t.Tabs = append(t.Tabs[:index], t.Tabs[index+1:]...)
		if selected {
			t.SelectedID = ""
			next := ""
			if len(t.Tabs) > 0 {
				next = t.Tabs[min(index, len(t.Tabs)-1)].ID
			}
			if next == "" {
				t.Offset = 0
				t.invalidateLayout()
				if t.OnChange != nil {
					t.OnChange("")
				}
			} else {
				t.selectTab(next, true)
			}
			if focused {
				t.focusSelected()
			}
		} else {
			t.Offset = clampFloat32(t.Offset, 0, t.maxOffset())
			t.invalidateLayout()
		}
		return true
	}
	return false
}

// Move places an existing tab at index; it is also used by header dragging.
func (t *Tabs) Move(id string, index int) bool {
	if index < 0 || index >= len(t.Tabs) {
		return false
	}
	from := -1
	for i := range t.Tabs {
		if t.Tabs[i].ID == id {
			from = i
			break
		}
	}
	if from < 0 || from == index {
		return false
	}
	tab := t.Tabs[from]
	if from < index {
		copy(t.Tabs[from:index], t.Tabs[from+1:index+1])
	} else {
		copy(t.Tabs[index+1:from+1], t.Tabs[index:from])
	}
	t.Tabs[index] = tab
	t.ScrollTo(id)
	t.InvalidatePaint()
	if t.OnReorder != nil {
		t.OnReorder(id, index)
	}
	return true
}

func (t *Tabs) SetDirty(id string, dirty bool) bool {
	for i := range t.Tabs {
		if t.Tabs[i].ID == id {
			if t.Tabs[i].Dirty == dirty {
				return false
			}
			t.Tabs[i].Dirty = dirty
			t.InvalidatePaint()
			return true
		}
	}
	return false
}

func (t *Tabs) SetTabError(id string, hasError bool) bool {
	for i := range t.Tabs {
		if t.Tabs[i].ID == id {
			if t.Tabs[i].HasError == hasError {
				return false
			}
			t.Tabs[i].HasError = hasError
			t.InvalidatePaint()
			return true
		}
	}
	return false
}

func (t *Tabs) SetNonClosable(id string, nonClosable bool) bool {
	for i := range t.Tabs {
		if t.Tabs[i].ID == id {
			if t.Tabs[i].NonClosable == nonClosable {
				return false
			}
			t.Tabs[i].NonClosable = nonClosable
			t.invalidateLayout()
			return true
		}
	}
	return false
}
