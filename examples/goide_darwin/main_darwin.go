//go:build darwin && (amd64 || arm64)

package main

import (
	"log"
	"os"
	"runtime"

	ui "github.com/banditmoscow1337/nativeForms"
	"github.com/banditmoscow1337/nativeForms/examples/goide"
	"github.com/banditmoscow1337/nativeForms/platform/darwin"
)

func init(){runtime.LockOSThread()}
func main(){
	manager:=ui.New()
	var app *goide.App
	window:=darwin.New(manager,darwin.Options{
		Title:"Go IDE · nativeForms",Width:1300,Height:820,
		OnReady:func(w *darwin.Window){
			app=goide.New(manager,w)
			manager.SetRoot(app.Root)
			if len(os.Args)>1 { if err:=app.OpenProject(os.Args[1]);err!=nil { log.Print(err) } }
		},
		OnFilesDropped:func(paths []string){ if app!=nil { app.OpenDropped(paths) } },
	})
	err:=window.Run()
	if app!=nil { app.Close() }
	if err!=nil { log.Fatal(err) }
}
