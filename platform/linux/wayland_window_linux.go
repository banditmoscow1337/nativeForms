//go:build linux

package linux

import (
	"fmt"
	"image"
	"sync/atomic"
	"time"

	ui "github.com/banditmoscow1337/nativeForms"
	"github.com/banditmoscow1337/nativeForms/platform"
	"github.com/banditmoscow1337/nativeForms/render"
)

type WaylandOptions struct {
	Title              string
	Width, Height      int
	Scale              float32
	Frameless          bool
	HideWindowControls bool
	OnReady            func(*WaylandWindow)
	OnFilesDropped     func([]string)
	OnCloseRequest     func(*WaylandWindow) platform.CloseAction
	Graphics           render.Kind
}

type WaylandWindow struct {
	Manager                                                              *ui.Manager
	app                                                                  *App
	Options                                                              WaylandOptions
	wire                                                                 *waylandWire
	compositor, shm, wm, surface, xdg, toplevel, seat, pointer, keyboard uint32
	width, height                                                        int
	compositorVersion                                                    int
	scale, preferredScale, lastUIScale                                   float32
	viewporter, viewport, fractionalManager, fractional                  uint32
	framebuffer                                                          *image.RGBA
	renderer                                                             render.Backend
	fallbackReason                                                       string
	buffers                                                              map[uint32]*wlBuffer
	current                                                              [2]*wlBuffer
	ready                                                                bool
	closing                                                              atomic.Bool
	hidden                                                               atomic.Bool
	lastFrame                                                            time.Time
	pointerPos                                                           ui.Vec2
	pressed                                                              map[uint32]bool
	globals                                                              map[uint32]string
	outputs                                                              map[uint32]*wlOutput
	keymap                                                               waylandSymbols
	keyGroup                                                             uint32
	keyMods                                                              uint32
	compose                                                              composeState
	repeatCode                                                           uint32
	repeatAt                                                             time.Time
	repeatRate, repeatDelay                                              int
	modal, portal                                                        bool
	textManager, textInput                                               uint32
	input                                                                wlTextInput
	dataManager, dataDevice, ownSource, serial, dragOffer                uint32
	dataOffers                                                           map[uint32]map[string]bool
	dataSources                                                          map[uint32]string
	selection                                                            uint32
	ownedText                                                            string
	receivingDrag                                                        bool
}

func NewWayland(manager *ui.Manager, options WaylandOptions) *WaylandWindow {
	return &WaylandWindow{Manager: manager, Options: options}
}

func (w *WaylandWindow) SetFrameless(frameless bool) {
	if w == nil {
		return
	}
	w.Options.Frameless = frameless
	if w.Manager != nil {
		w.Manager.InvalidateLayout()
	}
}

func (w *WaylandWindow) Frameless() bool {
	if w == nil {
		return false
	}
	return w.Options.Frameless
}

func (w *WaylandWindow) SetWindowControls(visible bool) {
	if w == nil {
		return
	}
	w.Options.HideWindowControls = !visible
	if w.Manager != nil {
		w.Manager.InvalidatePaint()
	}
}

func (w *WaylandWindow) WindowControlsVisible() bool {
	if w == nil {
		return false
	}
	return !w.Options.HideWindowControls && !w.Options.Frameless
}

func (w *WaylandWindow) Minimize() {
	if w == nil || w.app == nil {
		return
	}
	w.app.Post(func() {
		if w.wire != nil && w.toplevel != 0 && !w.closing.Load() {
			_ = w.wire.send(w.toplevel, 12, nil) // xdg_toplevel.set_minimized
		}
	})
}

func (w *WaylandWindow) Maximize() {
	if w == nil || w.app == nil {
		return
	}
	w.app.Post(func() {
		if w.wire != nil && w.toplevel != 0 && !w.closing.Load() {
			_ = w.wire.send(w.toplevel, 9, nil) // xdg_toplevel.set_maximized
		}
	})
}

// SetSize requests a new logical surface size. Wayland compositors may override
// this request with a later configure event. Call it on the window's UI goroutine.
func (w *WaylandWindow) SetSize(width, height int) error {
	if w == nil || w.Manager == nil {
		return fmt.Errorf("wayland: nil window or manager")
	}
	if err := platform.ValidateSize(width, height, w.scale); err != nil {
		return err
	}
	w.Options.Width, w.Options.Height = width, height
	if w.wire != nil {
		w.width, w.height = width, height
		w.Manager.InvalidateLayout()
	}
	return nil
}

func (w *WaylandWindow) Capabilities() platform.Capabilities {
	c := WaylandCapabilities()
	c.Clipboard = w.dataDevice != 0
	c.DragDrop = w.dataDevice != 0
	c.IME = w.textInput != 0
	c.FileDialog = w.portal
	return c
}

func (w *WaylandWindow) Close() {
	if w == nil {
		return
	}
	w.closing.Store(true)
	if w.app != nil {
		w.app.wake()
	}
}

func (w *WaylandWindow) Show() { w.setVisible(true) }
func (w *WaylandWindow) Hide() { w.setVisible(false) }
func (w *WaylandWindow) setVisible(visible bool) {
	if w == nil {
		return
	}
	w.hidden.Store(!visible)
	if w.app != nil {
		w.app.Post(func() {
			if w.wire == nil || w.closing.Load() {
				return
			}
			if visible {
				// An empty commit requests a new xdg_surface configure after unmap.
				for _, buffer := range w.current {
					if buffer != nil {
						buffer.valid = false
					}
				}
				w.Manager.InvalidatePaint()
				if err := w.wire.send(w.surface, 6, nil); err != nil {
					w.app.fail(err)
				}
			} else {
				w.Manager.ClearInteraction()
				w.ready = false
				// A null buffer unmaps the xdg_surface without dropping its role.
				args := append(wlWord(0), wlWord(0)...)
				args = append(args, wlWord(0)...)
				if err := w.wire.send(w.surface, 1, args); err != nil {
					w.app.fail(err)
					return
				}
				if err := w.wire.send(w.surface, 6, nil); err != nil {
					w.app.fail(err)
				}
			}
		})
	}
}

func (w *WaylandWindow) closeAction() {
	decision := platform.CloseWindow
	if w.Options.OnCloseRequest != nil {
		decision = w.Options.OnCloseRequest(w)
	}
	applyCloseDecision(decision, w.app, w.Hide, w.Close)
}

func (w *WaylandWindow) Run() error {
	a := NewApp()
	a.autoQuit = true
	if err := a.Add(w); err != nil {
		return err
	}
	return a.Run()
}
