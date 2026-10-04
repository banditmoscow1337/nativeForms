//go:build darwin && (amd64 || arm64)

package main

import (
	ui "github.com/banditmoscow1337/nativeForms"
	"github.com/banditmoscow1337/nativeForms/examples/editor"
	"github.com/banditmoscow1337/nativeForms/platform/darwin"
	"log"
	"runtime"
)

func init() { runtime.LockOSThread() }
func main() {
	manager := ui.New()
	var drop func([]string)
	window := darwin.New(manager, darwin.Options{Title: "nativeForms editor", Width: 1100, Height: 700,
		OnReady: func(w *darwin.Window) {
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
	if err := window.Run(); err != nil {
		log.Fatal(err)
	}
}
