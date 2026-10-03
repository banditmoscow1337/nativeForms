//go:build darwin && (amd64 || arm64)

package main

import (
	"log"

	ui "github.com/banditmoscow1337/nativeForms"
	"github.com/banditmoscow1337/nativeForms/examples/editor"
	"github.com/banditmoscow1337/nativeForms/platform/darwin"
)

func main() {
	manager := ui.New()
	manager.SetRoot(editor.New(manager))
	window := darwin.New(manager, darwin.Options{Title: "nativeForms editor", Width: 1100, Height: 700})
	if err := window.Run(); err != nil {
		log.Fatal(err)
	}
}
