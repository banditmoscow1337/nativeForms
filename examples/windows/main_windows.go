//go:build windows && (amd64 || arm64)

package main

import (
	"log"

	ui "github.com/banditmoscow1337/nativeForms"
	"github.com/banditmoscow1337/nativeForms/platform/windows"
)

func main() {
	root:=ui.NewStack(ui.Vertical,12)
	root.Padding=ui.All(24)
	root.Background=ui.RGBA(20,24,31,255)
	root.Add(ui.NewLabel("nativeForms — desktop"))
	field:=ui.NewTextField("Привет, мир!",nil)
	root.Add(field)
	manager:=ui.New()
	manager.SetRoot(root)
	root.Add(ui.NewButton("Показать текст",func() { manager.Notify(field.Text) }))
	window:=windows.New(manager,windows.Options{Title:"nativeForms",Width:800,Height:520})
	if err:=window.Run();err!=nil { log.Fatal(err) }
}
