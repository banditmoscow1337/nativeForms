//go:build windows && (amd64 || arm64)

package main

import (
	ui "github.com/banditmoscow1337/nativeForms"
	"github.com/banditmoscow1337/nativeForms/examples/editor"
	"github.com/banditmoscow1337/nativeForms/platform/windows"
	"log"
)

func main() {
	manager := ui.New()
	var drop func([]string)
	w := windows.New(manager, windows.Options{Title: "nativeForms editor", Width: 1100, Height: 700,
		OnReady: func(w *windows.Window) {
			root, handler := editor.Desktop(manager, w)
			drop = handler
			manager.SetRoot(root)
		},
		OnFilesDropped: func(files []string) {
			if drop != nil {
				drop(files)
			}
		},
	})
	if err := w.Run(); err != nil {
		log.Fatal(err)
	}
}
