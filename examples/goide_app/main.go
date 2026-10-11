package main

import (
	"log"
	"os"

	ui "github.com/banditmoscow1337/nativeForms"
	"github.com/banditmoscow1337/nativeForms/examples/goide"
	"github.com/banditmoscow1337/nativeForms/platform/desktop"
)

func main() {
	manager := ui.New()
	var app *goide.App
	window, err := desktop.New(manager, desktop.Options{
		Title: "Go IDE · nativeForms", Width: 1300, Height: 820,
		OnReady: func(host desktop.Window) {
			app = goide.New(manager, host)
			manager.SetRoot(app.Root)
			if len(os.Args) > 1 {
				if err := app.OpenProject(os.Args[1]); err != nil {
					log.Print(err)
				}
			}
		},
		OnFilesDropped: func(paths []string) {
			if app != nil {
				app.OpenDropped(paths)
			}
		},
	})
	if err != nil {
		log.Fatal(err)
	}
	err = window.Run()
	if app != nil {
		app.Close()
	}
	if err != nil {
		log.Fatal(err)
	}
}
