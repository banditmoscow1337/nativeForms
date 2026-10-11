package main

import (
	"log"

	ui "github.com/banditmoscow1337/nativeForms"
	"github.com/banditmoscow1337/nativeForms/examples/editor"
	"github.com/banditmoscow1337/nativeForms/platform/desktop"
)

func main() {
	manager := ui.New()
	var drop func([]string)
	window, err := desktop.New(manager, desktop.Options{
		Title: "nativeForms editor", Width: 1100, Height: 700,
		OnReady: func(host desktop.Window) {
			root, handler := editor.Desktop(manager, host)
			drop = handler
			manager.SetRoot(root)
		},
		OnFilesDropped: func(files []string) {
			if drop != nil {
				drop(files)
			}
		},
	})
	if err != nil {
		log.Fatal(err)
	}
	if err := window.Run(); err != nil {
		log.Fatal(err)
	}
}
