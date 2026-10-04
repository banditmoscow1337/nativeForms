//go:build darwin && (amd64 || arm64)

package main

import (
	ui "github.com/banditmoscow1337/nativeForms"
	"github.com/banditmoscow1337/nativeForms/platform/darwin"
	"log"
	"runtime"
)

func init() { runtime.LockOSThread() }
func window(title string) *darwin.Window {
	manager := ui.New()
	root := ui.NewStack(ui.Vertical, 12)
	root.Padding = ui.All(16)
	root.Add(ui.NewLabel(title), ui.NewTextArea("Hello, мир!\n日本語 · العربية", nil))
	root.Add(ui.NewButton("Новое окно", func() {
		if err := window("Ещё одно окно").Open(); err != nil {
			log.Print(err)
		}
	}))
	manager.SetRoot(root)
	return darwin.New(manager, darwin.Options{Title: title, Width: 520, Height: 320})
}
func main() {
	if err := darwin.Run(window("Первое окно"), window("Второе окно")); err != nil {
		log.Fatal(err)
	}
}
