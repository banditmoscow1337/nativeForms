//go:build linux

package main

import (
	ui "github.com/banditmoscow1337/nativeForms"
	"github.com/banditmoscow1337/nativeForms/examples/editor"
	"github.com/banditmoscow1337/nativeForms/platform/linux"
	"log"
)

func main() {
	manager := ui.New()
	var drop func([]string)
	onDrop := func(files []string) {
		if drop != nil {
			drop(files)
		}
	}
	ready := func(host editor.DesktopHost) {
		root, handler := editor.Desktop(manager, host)
		drop = handler
		manager.SetRoot(root)
	}
	backend, err := linux.BackendName()
	if err != nil {
		log.Fatal(err)
	}
	if backend == "wayland" {
		w := linux.NewWayland(manager, linux.WaylandOptions{Title: "nativeForms editor", Width: 1100, Height: 700, OnReady: func(w *linux.WaylandWindow) { ready(w) }, OnFilesDropped: onDrop})
		if err = w.Run(); err != nil {
			log.Fatal(err)
		}
		return
	}
	w := linux.NewX11(manager, linux.X11Options{Title: "nativeForms editor", Width: 1100, Height: 700, OnReady: func(w *linux.X11Window) { ready(w) }, OnFilesDropped: onDrop})
	if err = w.Run(); err != nil {
		log.Fatal(err)
	}
}
