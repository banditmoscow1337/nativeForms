package main

import (
	"image"
	"image/png"
	"os"

	ui "github.com/banditmoscow1337/nativeForms"
	"github.com/banditmoscow1337/nativeForms/software"
)

func main() {
	root := ui.NewStack(ui.Vertical, 12)
	root.Padding = ui.All(20)
	root.Background = ui.RGBA(20, 24, 31, 255)
	root.Add(ui.NewLabel("nativeForms software frame"))
	field := ui.NewTextField("Hello, мир!", nil)
	root.Add(field)
	root.Add(ui.NewButton("Button", nil))
	root.Add(ui.NewProgressBar(0.65))

	manager := ui.New()
	manager.SetRoot(root)
	manager.BeginFrame(640, 360, 0)

	dst := image.NewRGBA(image.Rect(0, 0, 640, 360))
	if err := software.New().Render(manager.Frame(), dst); err != nil {
		panic(err)
	}
	file, err := os.Create("frame.png")
	if err != nil { panic(err) }
	defer file.Close()
	if err := png.Encode(file, dst); err != nil { panic(err) }
}
