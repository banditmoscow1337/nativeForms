//go:build linux

package main

import (
	"log"
	"os"

	ui "github.com/banditmoscow1337/nativeForms"
	"github.com/banditmoscow1337/nativeForms/examples/goide"
	"github.com/banditmoscow1337/nativeForms/platform/linux"
)

func main() {
	manager:=ui.New()
	var app *goide.App
	ready:=func(host goide.DesktopHost) {
		app=goide.New(manager,host)
		manager.SetRoot(app.Root)
		if len(os.Args)>1 { if err:=app.OpenProject(os.Args[1]); err!=nil { log.Print(err) } }
	}
	dropped:=func(paths []string){ if app!=nil { app.OpenDropped(paths) } }
	backend,err:=linux.BackendName()
	if err!=nil { log.Fatal(err) }
	if backend=="wayland" {
		window:=linux.NewWayland(manager,linux.WaylandOptions{
			Title:"Go IDE · nativeForms",Width:1300,Height:820,
			OnReady:func(w *linux.WaylandWindow){ready(w)},OnFilesDropped:dropped,
		})
		err=window.Run()
	} else {
		window:=linux.NewX11(manager,linux.X11Options{
			Title:"Go IDE · nativeForms",Width:1300,Height:820,
			OnReady:func(w *linux.X11Window){ready(w)},OnFilesDropped:dropped,
		})
		err=window.Run()
	}
	if app!=nil { app.Close() }
	if err!=nil { log.Fatal(err) }
}
