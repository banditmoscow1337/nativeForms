// Package editor demonstrates the same application tree on every host.
package editor

import (
	"fmt"

	ui "github.com/banditmoscow1337/nativeForms"
)

type object struct{ id, name, kind string }

type objects []object

func (m objects) Len() int              { return len(m) }
func (m objects) ID(i int) string       { return m[i].id }
func (m objects) Columns() int          { return 3 }
func (m objects) Header(col int) string { return []string{"Object", "Type", "ID"}[col] }
func (m objects) Cell(row, col int) string {
	switch col {
	case 0:
		return m[row].name
	case 1:
		return m[row].kind
	default:
		return m[row].id
	}
}

type hierarchy struct {
	labels   map[string]string
	children map[string][]string
}

func (h hierarchy) Roots() int                        { return 1 }
func (h hierarchy) Root(int) string                   { return "scene" }
func (h hierarchy) Label(id string) string            { return h.labels[id] }
func (h hierarchy) Children(id string) int            { return len(h.children[id]) }
func (h hierarchy) Child(id string, index int) string { return h.children[id][index] }

func New(manager *ui.Manager) ui.Component {
	data := objects{
		{id: "camera", name: "Camera", kind: "View"},
		{id: "light", name: "Sun", kind: "Light"},
		{id: "mesh-a", name: "Cube", kind: "Mesh"},
		{id: "mesh-b", name: "Sphere", kind: "Mesh"},
	}
	scene := hierarchy{
		labels:   map[string]string{"scene": "Scene", "camera": "Camera", "light": "Sun", "geometry": "Geometry", "mesh-a": "Cube", "mesh-b": "Sphere"},
		children: map[string][]string{"scene": {"camera", "light", "geometry"}, "geometry": {"mesh-a", "mesh-b"}},
	}
	tree := ui.NewVirtualTree(scene)
	tree.SetExpanded("scene", true)
	tree.SetExpanded("geometry", true)
	tree.SetMinimumSize(ui.Vec2{X: 150})
	table := ui.NewVirtualTable(data)
	table.SetMinimumSize(ui.Vec2{X: 250})
	selected := ui.NewLabel("Выберите объект")
	selected.Wrap = true
	selectedID := ""
	nameField := ui.NewTextField("Имя объекта", func(name string) {
		if selectedID == "" {
			return
		}
		for i := range data {
			if data[i].id == selectedID {
				data[i].name = name
				break
			}
		}
		scene.labels[selectedID] = name
		manager.InvalidatePaint()
	})
	show := func(id string) {
		selectedID = id
		selected.SetText("Выбран: " + id)
		name := scene.Label(id)
		for _, object := range data {
			if object.id == id {
				name = object.name
				break
			}
		}
		nameField.SetText(name)
	}
	tree.OnSelect = show
	table.OnSelect = show
	properties := ui.NewStack(ui.Vertical, 10)
	properties.Padding = ui.All(12)
	properties.Background = ui.DefaultTheme().PanelAlt
	properties.SetMinimumSize(ui.Vec2{X: 170})
	properties.Add(ui.NewLabel("Свойства"), selected)
	properties.Add(nameField)
	properties.Add(ui.NewCheckbox("Виден", true, nil))
	properties.Add(ui.NewRadioGroup([]ui.RadioOption{{ID: "local", Label: "Local"}, {ID: "world", Label: "World"}}, "local", nil))
	properties.Add(ui.NewComboBox([]ui.RadioOption{{ID: "solid", Label: "Solid"}, {ID: "wire", Label: "Wireframe"}}, "solid", nil))
	details := ui.NewStack(ui.Vertical, 8)
	details.Padding = ui.All(12)
	details.Add(ui.NewLabel("Таблица объектов"), table)
	table.SetFlex(1)
	tabs := ui.NewTabs([]ui.Tab{{ID: "objects", Title: "Объекты", Content: details}, {ID: "notes", Title: "Заметки", Content: ui.NewTextArea("Сцена", nil)}}, "objects", nil)
	center := ui.NewSplitPane(ui.Horizontal, tabs, properties)
	center.Ratio = 0.72
	workspace := ui.NewSplitPane(ui.Horizontal, tree, center)
	workspace.Ratio = 0.22
	workspace.SetFlex(1)
	header := ui.NewStack(ui.Horizontal, 8)
	header.Padding = ui.Symmetric(12, 8)
	header.Background = ui.DefaultTheme().Header
	header.Add(ui.NewLabel("Редактор nativeForms"))
	menu := ui.NewButton("Меню", func() {
		manager.OpenMenu([]ui.MenuItem{{ID: "refresh", Label: "Обновить", Action: func() { manager.InvalidateLayout() }},
			{ID: "help", Label: "Подсказка", Action: func() { manager.Notify("Выбор сохраняется по ID") }}}, ui.Rect{X: 210, Y: 52, W: 180})
	})
	menu.SetMinimumSize(ui.Vec2{X: 100})
	menu.SetTooltip("Открыть меню редактора")
	header.Add(menu)
	var dialogID ui.PopupID
	header.Add(ui.NewButton("Диалог", func() {
		closeButton := ui.NewButton("Закрыть", func() { manager.ClosePopup(dialogID) })
		dialog := ui.NewDialog("Пример диалога", ui.NewLabel(fmt.Sprintf("Объектов: %d", len(data))), closeButton)
		dialogID = manager.OpenDialog(dialog, 340, 180)
	}))
	root := ui.NewStack(ui.Vertical, 0)
	root.Background = ui.DefaultTheme().Panel
	root.Add(header, workspace)
	return root
}
