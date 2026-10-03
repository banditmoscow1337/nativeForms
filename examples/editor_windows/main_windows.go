//go:build windows && (amd64 || arm64)

package main

import (
	"log"

	ui "github.com/banditmoscow1337/nativeForms"
	"github.com/banditmoscow1337/nativeForms/examples/editor"
	"github.com/banditmoscow1337/nativeForms/platform/windows"
)

func main() {
	manager := ui.New()
	manager.SetRoot(editor.New(manager))
	window := windows.New(manager, windows.Options{Title: "nativeForms editor", Width: 1100, Height: 700})
	if err := window.Run(); err != nil {
		log.Fatal(err)
	}
}
