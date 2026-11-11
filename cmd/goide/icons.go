package goide

import (
	"image"
	"image/color"
	"math"
	"sync"

	ui "github.com/banditmoscow1337/nativeForms"
)

// Icons holds procedural 20x20 RGBA icons styled after VS Code's Dark+/Light+ icon set.
type Icons struct {
	Explorer   *ui.ImageResource
	Search     *ui.ImageResource
	Outline    *ui.ImageResource
	Debug      *ui.ImageResource
	GoTools    *ui.ImageResource
	Settings   *ui.ImageResource
	Folder     *ui.ImageResource
	GoFile     *ui.ImageResource
	GoTestFile *ui.ImageResource
	GoMod      *ui.ImageResource
	File       *ui.ImageResource
	Run        *ui.ImageResource
	Test       *ui.ImageResource
	Build      *ui.ImageResource
	Format     *ui.ImageResource
	Vet        *ui.ImageResource
	Stop       *ui.ImageResource
	Continue   *ui.ImageResource
	StepOver   *ui.ImageResource
	StepInto   *ui.ImageResource
	StepOut    *ui.ImageResource
	Error      *ui.ImageResource
	Warning    *ui.ImageResource
	Info       *ui.ImageResource
	Pass       *ui.ImageResource
	Breakpoint *ui.ImageResource
	SymFunc    *ui.ImageResource
	SymType    *ui.ImageResource
	SymVar     *ui.ImageResource
	SymConst   *ui.ImageResource
	SymPkg     *ui.ImageResource
	Theme      *ui.ImageResource
	Command    *ui.ImageResource
}

var (
	sharedIconsOnce sync.Once
	sharedIcons     *Icons
)

// DefaultIcons returns the shared procedural icon set for the IDE.
func DefaultIcons() *Icons {
	sharedIconsOnce.Do(func() {
		sharedIcons = &Icons{
			Explorer:   makeBoxIcon(color.NRGBA{R: 55, G: 148, B: 255, A: 255}, color.NRGBA{R: 25, G: 95, B: 185, A: 255}, 'E'),
			Search:     makeCircleIcon(color.NRGBA{R: 78, G: 201, B: 176, A: 255}, color.NRGBA{R: 32, G: 130, B: 112, A: 255}, 'S'),
			Outline:    makeBoxIcon(color.NRGBA{R: 197, G: 134, B: 192, A: 255}, color.NRGBA{R: 132, G: 72, B: 138, A: 255}, 'O'),
			Debug:      makeCircleIcon(color.NRGBA{R: 241, G: 76, B: 76, A: 255}, color.NRGBA{R: 168, G: 38, B: 38, A: 255}, 'D'),
			GoTools:    makeBoxIcon(color.NRGBA{R: 0, G: 173, B: 216, A: 255}, color.NRGBA{R: 0, G: 118, B: 156, A: 255}, 'G'),
			Settings:   makeCircleIcon(color.NRGBA{R: 156, G: 168, B: 182, A: 255}, color.NRGBA{R: 88, G: 98, B: 112, A: 255}, '*'),
			Folder:     makeFolderIcon(color.NRGBA{R: 226, G: 174, B: 58, A: 255}, color.NRGBA{R: 178, G: 126, B: 28, A: 255}),
			GoFile:     makeBoxIcon(color.NRGBA{R: 0, G: 173, B: 216, A: 255}, color.NRGBA{R: 0, G: 112, B: 148, A: 255}, 'G'),
			GoTestFile: makeBoxIcon(color.NRGBA{R: 78, G: 201, B: 176, A: 255}, color.NRGBA{R: 34, G: 132, B: 112, A: 255}, 'T'),
			GoMod:      makeBoxIcon(color.NRGBA{R: 206, G: 112, B: 214, A: 255}, color.NRGBA{R: 138, G: 58, B: 148, A: 255}, 'M'),
			File:       makeBoxIcon(color.NRGBA{R: 148, G: 158, B: 172, A: 255}, color.NRGBA{R: 92, G: 102, B: 116, A: 255}, 'F'),
			Run:        makeTriangleIcon(color.NRGBA{R: 72, G: 204, B: 116, A: 255}),
			Test:       makeCircleIcon(color.NRGBA{R: 72, G: 196, B: 142, A: 255}, color.NRGBA{R: 28, G: 128, B: 82, A: 255}, 'T'),
			Build:      makeBoxIcon(color.NRGBA{R: 86, G: 156, B: 214, A: 255}, color.NRGBA{R: 42, G: 98, B: 156, A: 255}, 'B'),
			Format:     makeBoxIcon(color.NRGBA{R: 220, G: 220, B: 170, A: 255}, color.NRGBA{R: 148, G: 148, B: 96, A: 255}, '='),
			Vet:        makeCircleIcon(color.NRGBA{R: 226, G: 174, B: 58, A: 255}, color.NRGBA{R: 162, G: 112, B: 22, A: 255}, 'V'),
			Stop:       makeStopIcon(color.NRGBA{R: 241, G: 76, B: 76, A: 255}),
			Continue:   makeTriangleIcon(color.NRGBA{R: 55, G: 188, B: 255, A: 255}),
			StepOver:   makeCircleIcon(color.NRGBA{R: 86, G: 156, B: 214, A: 255}, color.NRGBA{R: 38, G: 96, B: 148, A: 255}, '>'),
			StepInto:   makeCircleIcon(color.NRGBA{R: 86, G: 156, B: 214, A: 255}, color.NRGBA{R: 38, G: 96, B: 148, A: 255}, 'v'),
			StepOut:    makeCircleIcon(color.NRGBA{R: 86, G: 156, B: 214, A: 255}, color.NRGBA{R: 38, G: 96, B: 148, A: 255}, '^'),
			Error:      makeCircleIcon(color.NRGBA{R: 241, G: 76, B: 76, A: 255}, color.NRGBA{R: 175, G: 32, B: 32, A: 255}, '!'),
			Warning:    makeCircleIcon(color.NRGBA{R: 235, G: 175, B: 52, A: 255}, color.NRGBA{R: 172, G: 116, B: 18, A: 255}, '!'),
			Info:       makeCircleIcon(color.NRGBA{R: 55, G: 148, B: 255, A: 255}, color.NRGBA{R: 24, G: 92, B: 180, A: 255}, 'i'),
			Pass:       makeCircleIcon(color.NRGBA{R: 72, G: 204, B: 116, A: 255}, color.NRGBA{R: 28, G: 136, B: 68, A: 255}, '+'),
			Breakpoint: makeDotIcon(color.NRGBA{R: 229, G: 48, B: 48, A: 255}),
			SymFunc:    makeBoxIcon(color.NRGBA{R: 177, G: 128, B: 215, A: 255}, color.NRGBA{R: 118, G: 72, B: 158, A: 255}, 'f'),
			SymType:    makeBoxIcon(color.NRGBA{R: 78, G: 201, B: 176, A: 255}, color.NRGBA{R: 34, G: 132, B: 112, A: 255}, 'T'),
			SymVar:     makeBoxIcon(color.NRGBA{R: 117, G: 190, B: 255, A: 255}, color.NRGBA{R: 52, G: 118, B: 186, A: 255}, 'v'),
			SymConst:   makeBoxIcon(color.NRGBA{R: 226, G: 174, B: 58, A: 255}, color.NRGBA{R: 164, G: 116, B: 24, A: 255}, 'c'),
			SymPkg:     makeBoxIcon(color.NRGBA{R: 206, G: 145, B: 120, A: 255}, color.NRGBA{R: 142, G: 86, B: 64, A: 255}, 'p'),
			Theme:      makeCircleIcon(color.NRGBA{R: 226, G: 174, B: 58, A: 255}, color.NRGBA{R: 55, G: 148, B: 255, A: 255}, 'C'),
			Command:    makeBoxIcon(color.NRGBA{R: 0, G: 122, B: 204, A: 255}, color.NRGBA{R: 0, G: 82, B: 148, A: 255}, '>'),
		}
	})
	return sharedIcons
}

func makeCircleIcon(primary, secondary color.NRGBA, glyph byte) *ui.ImageResource {
	const size = 20
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	center := float64(size) / 2
	radius := center - 1.5
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			dx := float64(x) + 0.5 - center
			dy := float64(y) + 0.5 - center
			dist := math.Hypot(dx, dy)
			if dist <= radius {
				t := dist / radius
				r := uint8(float64(primary.R)*(1-t) + float64(secondary.R)*t)
				g := uint8(float64(primary.G)*(1-t) + float64(secondary.G)*t)
				b := uint8(float64(primary.B)*(1-t) + float64(secondary.B)*t)
				img.SetRGBA(x, y, color.RGBA{R: r, G: g, B: b, A: 255})
			}
		}
	}
	drawMiniGlyph(img, glyph)
	res, err := ui.NewImageResource(img)
	if err != nil {
		panic(err)
	}
	return res
}

func makeBoxIcon(primary, secondary color.NRGBA, glyph byte) *ui.ImageResource {
	const size = 20
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	for y := 2; y < size-2; y++ {
		t := float64(y-2) / float64(size-5)
		r := uint8(float64(primary.R)*(1-t) + float64(secondary.R)*t)
		g := uint8(float64(primary.G)*(1-t) + float64(secondary.G)*t)
		b := uint8(float64(primary.B)*(1-t) + float64(secondary.B)*t)
		for x := 2; x < size-2; x++ {
			if (x == 2 || x == size-3) && (y == 2 || y == size-3) {
				continue
			}
			img.SetRGBA(x, y, color.RGBA{R: r, G: g, B: b, A: 255})
		}
	}
	drawMiniGlyph(img, glyph)
	res, err := ui.NewImageResource(img)
	if err != nil {
		panic(err)
	}
	return res
}

func makeFolderIcon(primary, secondary color.NRGBA) *ui.ImageResource {
	const size = 20
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	for y := 3; y < 17; y++ {
		for x := 2; x < 18; x++ {
			if y < 6 && x > 9 {
				continue
			}
			c := primary
			if y < 6 {
				c = secondary
			}
			img.SetRGBA(x, y, color.RGBA{R: c.R, G: c.G, B: c.B, A: 255})
		}
	}
	res, err := ui.NewImageResource(img)
	if err != nil {
		panic(err)
	}
	return res
}

func makeTriangleIcon(fill color.NRGBA) *ui.ImageResource {
	const size = 20
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	for y := 3; y < 17; y++ {
		half := 7 - int(math.Abs(float64(y-10)))
		maxX := 5 + half*2
		for x := 5; x <= maxX && x < 17; x++ {
			img.SetRGBA(x, y, color.RGBA{R: fill.R, G: fill.G, B: fill.B, A: 255})
		}
	}
	res, err := ui.NewImageResource(img)
	if err != nil {
		panic(err)
	}
	return res
}

func makeStopIcon(fill color.NRGBA) *ui.ImageResource {
	const size = 20
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	for y := 4; y < 16; y++ {
		for x := 4; x < 16; x++ {
			img.SetRGBA(x, y, color.RGBA{R: fill.R, G: fill.G, B: fill.B, A: 255})
		}
	}
	res, err := ui.NewImageResource(img)
	if err != nil {
		panic(err)
	}
	return res
}

func makeDotIcon(fill color.NRGBA) *ui.ImageResource {
	const size = 20
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	center := float64(size) / 2
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			if math.Hypot(float64(x)+0.5-center, float64(y)+0.5-center) <= 5.5 {
				img.SetRGBA(x, y, color.RGBA{R: fill.R, G: fill.G, B: fill.B, A: 255})
			}
		}
	}
	res, err := ui.NewImageResource(img)
	if err != nil {
		panic(err)
	}
	return res
}

func drawMiniGlyph(img *image.RGBA, glyph byte) {
	white := color.RGBA{R: 250, G: 252, B: 255, A: 255}
	switch glyph {
	case '!':
		for y := 5; y <= 11; y++ {
			img.SetRGBA(9, y, white)
			img.SetRGBA(10, y, white)
		}
		img.SetRGBA(9, 13, white)
		img.SetRGBA(10, 13, white)
		img.SetRGBA(9, 14, white)
		img.SetRGBA(10, 14, white)
	case '+':
		for i := 6; i <= 13; i++ {
			img.SetRGBA(i, 9, white)
			img.SetRGBA(i, 10, white)
			img.SetRGBA(9, i, white)
			img.SetRGBA(10, i, white)
		}
	case '=':
		for x := 5; x <= 14; x++ {
			img.SetRGBA(x, 7, white)
			img.SetRGBA(x, 10, white)
			img.SetRGBA(x, 13, white)
		}
	default:
		// Crisp center highlight dot/bar so every badge icon has clear contrast.
		for y := 7; y <= 12; y++ {
			for x := 7; x <= 12; x++ {
				if (x == 7 || x == 12 || y == 7 || y == 12) && glyph != 'i' {
					img.SetRGBA(x, y, white)
				} else if glyph == 'i' && (x == 9 || x == 10) {
					img.SetRGBA(x, y, white)
				}
			}
		}
	}
}
