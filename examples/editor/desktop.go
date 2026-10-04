package editor

import (
	"fmt"
	"strings"

	ui "github.com/banditmoscow1337/nativeForms"
	"github.com/banditmoscow1337/nativeForms/platform"
)

type DesktopHost interface {
	platform.FileDialogs
	Capabilities() platform.Capabilities
}

// Desktop adds native file pickers and a visible file-drop result to the editor.
// Construct it from OnReady, once the host has discovered its capabilities.
func Desktop(manager *ui.Manager, host DesktopHost) (ui.Component, func([]string)) {
	root := ui.NewStack(ui.Vertical, 4)
	tools := ui.NewStack(ui.Horizontal, 8)
	tools.Padding = ui.Symmetric(12, 6)
	status := ui.NewLabel("Перетащите файлы в окно")
	status.Wrap = true
	show := func(path string, err error) {
		if err != nil {
			status.SetText(fmt.Sprint(err))
		} else if path != "" {
			status.SetText(path)
		} else {
			status.SetText("Выбор отменён")
		}
	}
	open := ui.NewButton("Открыть", func() { path, err := host.OpenFile(); show(path, err) })
	save := ui.NewButton("Сохранить как", func() { path, err := host.SaveFile("scene.json"); show(path, err) })
	open.SetEnabled(host.Capabilities().FileDialog)
	save.SetEnabled(host.Capabilities().FileDialog)
	tools.Add(open, save)
	content := New(manager)
	content.UIState().SetFlex(1)
	root.Add(tools, content, status)
	return root, func(files []string) { status.SetText(strings.Join(files, "\n")) }
}
