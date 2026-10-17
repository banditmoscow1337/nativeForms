package nativeforms

import (
	"image"
	"testing"
)

func testIcon(t *testing.T) *ImageResource {
	t.Helper()
	icon, err := NewImageResource(image.NewRGBA(image.Rect(0, 0, 4, 2)))
	if err != nil {
		t.Fatal(err)
	}
	return icon
}

func imageCommands(commands []DrawCommand) []DrawCommand {
	var images []DrawCommand
	for _, command := range commands {
		if command.Kind == CommandImage {
			images = append(images, command)
		}
	}
	return images
}

func TestButtonIconLayoutAndDisabledState(t *testing.T) {
	icon := testIcon(t)
	button := NewButton("Open", nil)
	button.SetMinimumSize(Vec2{})
	plainWidth := button.Measure(Constraints{}).X
	manager := New()
	manager.SetRoot(button)
	manager.BeginFrame(160, 44, 0)
	button.SetIcon(icon)
	if !manager.NeedsFrame() || button.Measure(Constraints{}).X <= plainWidth {
		t.Fatal("adding an icon did not expand or invalidate the button")
	}
	manager.BeginFrame(160, 44, 0)
	images := imageCommands(manager.Frame().Commands)
	if len(images) != 1 || images[0].Image != icon || images[0].Filter != FilterBilinear ||
		images[0].Rect.W != 20 || images[0].Rect.H != 10 {
		t.Fatalf("icon did not preserve its aspect ratio: %+v", images)
	}
	button.SetEnabled(false)
	manager.BeginFrame(160, 44, 0)
	images = imageCommands(manager.Frame().Commands)
	if len(images) != 1 || images[0].Tint.A != 0.5 {
		t.Fatalf("disabled icon opacity: %+v", images)
	}
}

func TestMenuComboAndTabsUseOptionIcons(t *testing.T) {
	icon := testIcon(t)
	options := []RadioOption{{ID: "first", Label: "First", Icon: icon}, {ID: "second", Label: "Second"}}
	combo := NewComboBox(options, "first", nil)
	manager := New()
	manager.SetInteractive(true)
	manager.SetRoot(combo)
	manager.BeginFrame(200, 44, 0)
	if len(imageCommands(manager.Frame().Commands)) != 1 {
		t.Fatal("selected combo option has no icon")
	}
	if !combo.open() || len(manager.popups) != 1 {
		t.Fatal("combo did not open its menu")
	}
	menu := manager.popups[0].content.(*Menu)
	if menu.Items[0].Icon != icon || menu.Items[1].Icon != nil {
		t.Fatal("combo lost icons while building menu items")
	}
	manager.BeginFrame(200, 140, 0)
	if len(imageCommands(manager.Frame().Commands)) != 2 {
		t.Fatal("combo and its menu did not both paint the selected icon")
	}

	tabs := NewTabs([]Tab{{ID: "one", Title: "One", Icon: icon}, {ID: "two", Title: "Two"}}, "one", nil)
	firstWidth := tabs.tabWidth(tabs.Tabs[0])
	manager.SetRoot(tabs)
	manager.BeginFrame(300, 80, 0)
	if len(imageCommands(manager.Frame().Commands)) != 1 || tabs.Measure(Constraints{}).X < firstWidth+tabs.tabWidth(tabs.Tabs[1]) {
		t.Fatal("tab strip did not measure or paint its icon")
	}
	tabs.Handle(Event{Type: PointerUp, Button: MouseLeft, X: firstWidth + 1, Y: 10})
	if tabs.SelectedID != "two" {
		t.Fatal("tab hit testing disagrees with its icon-aware width")
	}
}

type iconListModel struct {
	icon  *ImageResource
	calls int
}

func (m *iconListModel) Len() int                  { return 1000 }
func (m *iconListModel) ID(i int) string           { return "row" }
func (m *iconListModel) Label(i int) string        { return "Row" }
func (m *iconListModel) Icon(i int) *ImageResource { m.calls++; return m.icon }

type iconTreeModelWithImage struct{ icon *ImageResource }

func (m iconTreeModelWithImage) Roots() int             { return 1 }
func (m iconTreeModelWithImage) Root(int) string        { return "folder" }
func (m iconTreeModelWithImage) Label(id string) string { return id }
func (m iconTreeModelWithImage) Children(id string) int {
	if id == "folder" {
		return 1
	}
	return 0
}
func (m iconTreeModelWithImage) Child(string, int) string   { return "leaf" }
func (m iconTreeModelWithImage) Icon(string) *ImageResource { return m.icon }

type iconTableModel struct {
	icon  *ImageResource
	calls int
}

func (m *iconTableModel) Len() int             { return 1000 }
func (m *iconTableModel) ID(int) string        { return "row" }
func (m *iconTableModel) Columns() int         { return 2 }
func (m *iconTableModel) Header(int) string    { return "Column" }
func (m *iconTableModel) Cell(int, int) string { return "Cell" }
func (m *iconTableModel) CellIcon(_, col int) *ImageResource {
	m.calls++
	if col == 0 {
		return m.icon
	}
	return nil
}

func TestVirtualRowsPaintVisibleIcons(t *testing.T) {
	icon := testIcon(t)
	model := &iconListModel{icon: icon}
	list := NewVirtualList(model)
	manager := New()
	manager.SetRoot(list)
	manager.BeginFrame(180, 64, 0)
	if model.calls != 2 || len(imageCommands(manager.Frame().Commands)) != 2 {
		t.Fatalf("virtual list painted offscreen icons: calls=%d", model.calls)
	}
	model.calls = 0
	list.Offset = 320
	manager.BeginFrame(180, 64, 0)
	if model.calls != 2 || len(imageCommands(manager.Frame().Commands)) != 2 {
		t.Fatal("scrolled virtual list did not keep painting only visible icons")
	}

	tree := NewVirtualTree(iconTreeModelWithImage{icon: icon})
	manager.SetRoot(tree)
	manager.BeginFrame(180, 64, 0)
	images := imageCommands(manager.Frame().Commands)
	if len(images) != 1 || images[0].Rect.X < 26 || images[0].Rect.X >= 54 {
		t.Fatalf("tree icon overlaps the expander or indent: %+v", images)
	}

	tableModel := &iconTableModel{icon: icon}
	table := NewVirtualTable(tableModel)
	manager.SetRoot(table)
	manager.BeginFrame(300, 100, 0)
	if tableModel.calls != 4 || len(imageCommands(manager.Frame().Commands)) != 2 {
		t.Fatalf("table should query two visible rows and paint the first column's icons: calls=%d", tableModel.calls)
	}
}

func TestCheckboxToggleAndRadioIcons(t *testing.T) {
	icon := testIcon(t)
	checkbox := NewCheckbox("Check", false, nil)
	checkbox.SetIcon(icon)
	toggle := NewToggle("Toggle", true, nil)
	toggle.SetIcon(icon)
	radio := NewRadioGroup([]RadioOption{{ID: "one", Label: "One", Icon: icon}, {ID: "two", Label: "Two"}}, "one", nil)
	root := NewStack(Vertical, 0)
	root.Add(checkbox, toggle, radio)
	manager := New()
	manager.SetRoot(root)
	manager.BeginFrame(320, 176, 0)
	if len(imageCommands(manager.Frame().Commands)) != 3 {
		t.Fatal("checkbox, toggle or radio option did not paint its icon")
	}
}
