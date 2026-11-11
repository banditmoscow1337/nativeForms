// Command goide_app launches the VS Code-inspired Go IDE built on nativeForms.
//
// Usage:
//
//	go run ./cmd/goide_app [/path/to/go/module]
//	go run ./cmd/goide_app -project . -light -scale 1.25
//	go run ./cmd/goide_app -headless -project . -screenshot goide_preview.png
package main

import (
	"flag"
	"fmt"
	"image"
	"image/png"
	"log"
	"os"
	"path/filepath"

	ui "github.com/banditmoscow1337/nativeForms"
	"github.com/banditmoscow1337/nativeForms/cmd/goide"
	"github.com/banditmoscow1337/nativeForms/platform"
	"github.com/banditmoscow1337/nativeForms/platform/desktop"
	"github.com/banditmoscow1337/nativeForms/render"
	"github.com/banditmoscow1337/nativeForms/software"
)

type headlessHost struct{}

func (headlessHost) Capabilities() platform.Capabilities {
	return platform.Capabilities{SoftwareFrame: true, Keyboard: true, Pointer: true, TextInput: true, Clipboard: true, FileDialog: true}
}
func (headlessHost) OpenFolder() (string, error)          { return "", nil }
func (headlessHost) OpenFile() (string, error)            { return "", nil }
func (headlessHost) SaveFile(name string) (string, error) { return name, nil }

func resolveInitialProject(flagPath string, args []string) string {
	if flagPath != "" {
		return flagPath
	}
	if len(args) > 0 && args[0] != "" {
		return args[0]
	}
	if info, err := os.Stat("go.mod"); err == nil && !info.IsDir() {
		if wd, err := os.Getwd(); err == nil {
			return wd
		}
	}
	return ""
}

func main() {
	var (
		projectPath        string
		backend            string
		graphics           string
		width              int
		height             int
		scale              float64
		lightTheme         bool
		frameless          bool
		hideWindowControls bool
		headless           bool
		screenshot         string
	)
	flag.StringVar(&projectPath, "project", "", "Path to a Go module directory containing go.mod")
	flag.StringVar(&backend, "backend", "", "Linux window backend: '', 'x11', or 'wayland'")
	flag.StringVar(&graphics, "graphics", "auto", "Renderer backend: 'auto', 'hardware', or 'software'")
	flag.IntVar(&width, "width", 1440, "Logical window width")
	flag.IntVar(&height, "height", 900, "Logical window height")
	flag.Float64Var(&scale, "scale", 1.0, "Initial display scale factor (1.0 - 4.0)")
	flag.BoolVar(&lightTheme, "light", false, "Start in VS Code Light+ theme")
	flag.BoolVar(&frameless, "frameless", false, "Start window without standard OS titlebar/frame")
	flag.BoolVar(&hideWindowControls, "hide-window-controls", false, "Hide standard OS window control buttons")
	flag.BoolVar(&headless, "headless", false, "Run offscreen using software renderer without an OS window")
	flag.StringVar(&screenshot, "screenshot", "", "Save a rendered PNG screenshot in headless mode")
	flag.Parse()

	initialProject := resolveInitialProject(projectPath, flag.Args())

	manager := ui.New()
	fonts := ui.NewSystemFontContext()
	fonts.SetCacheLimit(512 << 10)
	fonts.SetLayoutCacheLimit(256 << 10)
	manager.SetFontContext(fonts)
	manager.SetDisplayScale(float32(scale))
	manager.SetInteractive(true)

	if headless {
		if err := runHeadlessIDE(manager, initialProject, width, height, float32(scale), lightTheme, screenshot); err != nil {
			log.Fatal(err)
		}
		return
	}

	desktopApp := desktop.NewApp()
	var (
		ideApp     *goide.App
		mainWin    desktop.Window
		forceQuit  bool
		memoryClip string
	)

	manager.SetClipboardHandlers(
		func() (string, error) {
			if mainWin != nil && mainWin.Capabilities().Supports(platform.FeatureClipboard) {
				return mainWin.ClipboardText()
			}
			return memoryClip, nil
		},
		func(s string) error {
			memoryClip = s
			if mainWin != nil && mainWin.Capabilities().Supports(platform.FeatureClipboard) {
				return mainWin.SetClipboardText(s)
			}
			return nil
		},
	)

	win, err := desktopApp.NewWindow(manager, desktop.Options{
		Title:              "Go IDE (VS Code Edition) · nativeForms",
		Width:              width,
		Height:             height,
		Scale:              float32(scale),
		Frameless:          frameless,
		HideWindowControls: hideWindowControls,
		Backend:            backend,
		Graphics:           render.Kind(graphics),
		OnReady: func(host desktop.Window) {
			mainWin = host
			ideApp = goide.New(manager, host)
			if lightTheme {
				ideApp.ToggleTheme()
			}
			manager.SetRoot(ideApp.Root)
			if initialProject != "" {
				if err := ideApp.OpenProject(initialProject); err != nil {
					log.Print(err)
				}
			}
		},
		OnFilesDropped: func(paths []string) {
			if ideApp != nil {
				ideApp.OpenDropped(paths)
			}
		},
		OnCloseRequest: func(w desktop.Window) platform.CloseAction {
			if forceQuit || ideApp == nil || !ideApp.HasDirtyFiles() {
				return platform.QuitApp
			}
			var popupID ui.PopupID
			lbl := ui.NewLabel("В проекте есть несохранённые изменения. Сохранить все файлы перед выходом?")
			lbl.Wrap = true
			btnSaveQuit := ui.NewButton("Сохранить и выйти", func() {
				manager.ClosePopup(popupID)
				_ = ideApp.SaveActive()
				forceQuit = true
				desktopApp.Quit()
			})
			btnQuitNoSave := ui.NewButton("Выйти без сохранения", func() {
				manager.ClosePopup(popupID)
				forceQuit = true
				desktopApp.Quit()
			})
			btnCancel := ui.NewButton("Отмена", func() {
				manager.ClosePopup(popupID)
			})
			dlg := ui.NewDialog("Выход из Go IDE", lbl, btnSaveQuit, btnQuitNoSave, btnCancel)
			popupID = manager.OpenDialog(dlg, 520, 210)
			return platform.CancelClose
		},
	})
	if err != nil {
		log.Fatal(err)
	}
	mainWin = win

	runErr := desktopApp.Run()
	if ideApp != nil {
		ideApp.Close()
	}
	if runErr != nil {
		log.Fatal(runErr)
	}
}

func runHeadlessIDE(manager *ui.Manager, projectPath string, width, height int, scale float32, light bool, screenshot string) error {
	if !(scale > 0 && scale <= 8) {
		scale = 1
	}
	ideApp := goide.New(manager, headlessHost{})
	defer ideApp.Close()
	if light {
		ideApp.ToggleTheme()
	}
	manager.SetRoot(ideApp.Root)

	if projectPath != "" {
		if err := ideApp.OpenProject(projectPath); err != nil {
			return err
		}
		// Automatically open up to two .go files to render a realistic IDE workspace.
		var goFiles []string
		_ = filepath.WalkDir(projectPath, func(p string, d os.DirEntry, err error) error {
			if err != nil || len(goFiles) >= 2 {
				return nil
			}
			if !d.IsDir() && filepath.Ext(d.Name()) == ".go" {
				goFiles = append(goFiles, p)
			}
			return nil
		})
		if len(goFiles) > 0 {
			ideApp.OpenDropped(goFiles)
		}
	}

	renderer := software.New()
	defer renderer.Close()
	target := image.NewRGBA(image.Rect(0, 0, int(float32(width)*scale), int(float32(height)*scale)))
	logicalW := platform.LogicalSize(target.Bounds().Dx(), scale)
	logicalH := platform.LogicalSize(target.Bounds().Dy(), scale)
	manager.BeginFrame(logicalW, logicalH, 0.016)
	if err := renderer.RenderAtScale(manager.Frame(), target, scale); err != nil {
		return err
	}
	if screenshot != "" {
		f, err := os.Create(screenshot)
		if err != nil {
			return err
		}
		defer f.Close()
		if err := png.Encode(f, target); err != nil {
			return err
		}
		fmt.Printf("Saved headless Go IDE screenshot to %s (%dx%d)\n", screenshot, target.Bounds().Dx(), target.Bounds().Dy())
	}
	return nil
}
