//go:build darwin && (amd64 || arm64)

package darwin

import (
	"fmt"
	"unsafe"

	ui "github.com/banditmoscow1337/nativeForms"
	"github.com/ebitengine/purego/objc"
)

type TrayOptions struct {
	Title   string
	Tooltip string
	Icon    *ui.ImageResource
	Menu    []ui.MenuItem
	OnClick func()
}

func (a *App) SetTray(options TrayOptions) error {
	if a == nil {
		return fmt.Errorf("darwin: nil app")
	}
	copied := options
	copied.Menu = append([]ui.MenuItem(nil), options.Menu...)
	a.trayMu.Lock()
	a.pendingTray = &copied
	a.trayMu.Unlock()
	a.runOnMain(func() { a.applyTrayOnMain(copied) })
	return nil
}

func (a *App) RemoveTray() {
	if a == nil {
		return
	}
	a.trayMu.Lock()
	a.pendingTray = nil
	a.trayMu.Unlock()
	a.runOnMain(a.removeTrayOnMain)
}

func (a *App) invokeTrayAction(tag int) {
	a.trayMu.Lock()
	fn := a.trayActions[tag]
	a.trayMu.Unlock()
	if fn != nil {
		fn()
	}
}

func makeNSImage(res *ui.ImageResource) objc.ID {
	if res == nil {
		return 0
	}
	b := res.Bounds()
	w, h := b.Dx(), b.Dy()
	if w <= 0 || h <= 0 {
		return 0
	}
	rep := class("NSBitmapImageRep").Send(selector("alloc")).Send(
		selector("initWithBitmapDataPlanes:pixelsWide:pixelsHigh:bitsPerSample:samplesPerPixel:hasAlpha:isPlanar:colorSpaceName:bytesPerRow:bitsPerPixel:"),
		uintptr(0), w, h, 8, 4, true, false, nsString("NSDeviceRGBColorSpace"), w*4, 32,
	)
	if rep == 0 {
		return 0
	}
	defer rep.Send(selRelease)
	ptr := objc.Send[uintptr](rep, selector("bitmapData"))
	if ptr == 0 {
		return 0
	}
	dst := unsafe.Slice((*byte)(unsafe.Pointer(ptr)), w*h*4)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			c := res.RGBAAt(b.Min.X+x, b.Min.Y+y)
			i := (y*w + x) * 4
			dst[i], dst[i+1], dst[i+2], dst[i+3] = c.R, c.G, c.B, c.A
		}
	}
	img := class("NSImage").Send(selector("alloc")).Send(selector("initWithSize:"), size{W: 18, H: 18})
	if img == 0 {
		return 0
	}
	img.Send(selector("addRepresentation:"), rep)
	return img
}

func (a *App) applyTrayOnMain(options TrayOptions) {
	statusBar := class("NSStatusBar").Send(selector("systemStatusBar"))
	if statusBar == 0 {
		return
	}
	if a.statusItem == 0 {
		a.statusItem = objc.Send[objc.ID](statusBar, selector("statusItemWithLength:"), float64(-1.0))
		if a.statusItem != 0 {
			a.statusItem.Send(selRetain)
		}
	}
	if a.statusItem == 0 {
		return
	}
	title := options.Title
	if title == "" && options.Icon == nil {
		title = "nativeForms"
	}
	button := a.statusItem.Send(selector("button"))
	if button != 0 {
		button.Send(selector("setTitle:"), nsString(title))
		if options.Tooltip != "" {
			button.Send(selector("setToolTip:"), nsString(options.Tooltip))
		}
		if options.Icon != nil {
			if img := makeNSImage(options.Icon); img != 0 {
				button.Send(selector("setImage:"), img)
				img.Send(selRelease)
			}
		}
	} else {
		a.statusItem.Send(selector("setTitle:"), nsString(title))
		if options.Tooltip != "" {
			a.statusItem.Send(selector("setToolTip:"), nsString(options.Tooltip))
		}
	}

	actions := make(map[int]func(), len(options.Menu)+1)
	if options.OnClick != nil {
		actions[-1] = options.OnClick
	}
	if a.trayMenu != 0 {
		a.trayMenu.Send(selRelease)
		a.trayMenu = 0
	}
	if len(options.Menu) > 0 {
		menu := class("NSMenu").Send(selector("alloc")).Send(selector("initWithTitle:"), nsString(title))
		menu.Send(selector("setAutoenablesItems:"), false)
		for i, item := range options.Menu {
			if item.Separator || item.Label == "-" {
				sep := class("NSMenuItem").Send(selector("separatorItem"))
				menu.Send(selector("addItem:"), sep)
				continue
			}
			label := item.Label
			if item.Shortcut != "" {
				label = label + " (" + item.Shortcut + ")"
			}
			mi := class("NSMenuItem").Send(selector("alloc")).Send(
				selector("initWithTitle:action:keyEquivalent:"),
				nsString(label), selector("trayItemClicked:"), nsString(""),
			)
			mi.Send(selector("setTarget:"), trayTargetID)
			mi.Send(selector("setTag:"), int64(i))
			mi.Send(selector("setEnabled:"), !item.Disabled)
			if item.Checked {
				mi.Send(selector("setState:"), int64(1))
			}
			if item.Action != nil {
				actions[i] = item.Action
			}
			menu.Send(selector("addItem:"), mi)
			mi.Send(selRelease)
		}
		a.trayMenu = menu
		a.statusItem.Send(selector("setMenu:"), menu)
	} else {
		a.statusItem.Send(selector("setMenu:"), objc.ID(0))
		if button != 0 && options.OnClick != nil {
			button.Send(selector("setTarget:"), trayTargetID)
			button.Send(selector("setAction:"), selector("trayItemClicked:"))
			button.Send(selector("setTag:"), int64(-1))
		}
	}
	a.trayMu.Lock()
	a.trayActions = actions
	a.trayMu.Unlock()
}

func (a *App) removeTrayOnMain() {
	if a.statusItem != 0 {
		statusBar := class("NSStatusBar").Send(selector("systemStatusBar"))
		if statusBar != 0 {
			statusBar.Send(selector("removeStatusItem:"), a.statusItem)
		}
		a.statusItem.Send(selRelease)
		a.statusItem = 0
	}
	if a.trayMenu != 0 {
		a.trayMenu.Send(selRelease)
		a.trayMenu = 0
	}
	a.trayMu.Lock()
	a.trayActions = nil
	a.trayMu.Unlock()
}
