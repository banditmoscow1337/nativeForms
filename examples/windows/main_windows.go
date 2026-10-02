//go:build windows && (amd64 || arm64)

package main

import (
	"log"

	ui "github.com/banditmoscow1337/nativeForms"
	"github.com/banditmoscow1337/nativeForms/platform/windows"
)

func main() {
	root:=ui.NewStack(ui.Vertical,12)
	root.Padding=ui.All(20)
	root.Background=ui.RGBA(20,24,31,255)
	root.Add(ui.NewLabel("nativeForms — редактор и формы"))
	form:=ui.NewForm(150)
	form.ColumnGap,form.RowGap=12,12
	field:=ui.NewTextField("Привет, мир!",nil)
	form.AddRow(ui.NewLabel("Имя"),field)
	form.AddRow(ui.NewLabel("Заметка"),ui.NewTextArea("Текст можно выделять, переносить, отменять и вставлять.",nil))
	for i:=0;i<16;i++ {
		label:=ui.NewLabel("Дополнительное поле")
		label.Wrap=true
		form.AddRow(label,ui.NewTextField("",nil))
	}
	left:=ui.NewScrollPanel(form)
	right:=ui.NewStack(ui.Vertical,10)
	right.Padding=ui.All(16)
	right.Background=ui.RGBA(30,36,46,255)
	right.Add(ui.NewLabel("Ctrl+A/C/X/V, Ctrl+Z/Y, Shift+стрелки"))
	manager:=ui.New()
	right.Add(ui.NewButton("Показать текст",func() { manager.Notify(field.Text) }))
	split:=ui.NewSplitPane(ui.Horizontal,left,right)
	split.Ratio=0.65
	split.SetFlex(1)
	root.Add(split)
	manager.SetRoot(root)
	window:=windows.New(manager,windows.Options{Title:"nativeForms",Width:800,Height:520})
	if err:=window.Run();err!=nil { log.Fatal(err) }
}
