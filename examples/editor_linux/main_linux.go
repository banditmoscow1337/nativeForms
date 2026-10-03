//go:build linux

package main

import (
	"log"

	ui "github.com/banditmoscow1337/nativeForms"
	"github.com/banditmoscow1337/nativeForms/examples/editor"
	"github.com/banditmoscow1337/nativeForms/platform/linux"
)

func main() {
	manager := ui.New()
	manager.SetRoot(editor.New(manager))
	backend, err := linux.BackendName()
	if err != nil {
		log.Fatal(err)
	}
	if backend == "wayland" {
		log.Fatal((&linux.WaylandWindow{Manager: manager}).Run())
	}
	window := linux.NewX11(manager, linux.X11Options{Title: "nativeForms editor", Width: 1100, Height: 700})
	if err := window.Run(); err != nil {
		log.Fatal(err)
	}
}
