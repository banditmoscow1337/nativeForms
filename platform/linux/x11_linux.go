//go:build linux

package linux

import (
	"fmt"
	"image"
	"net"
	"sync/atomic"
	"time"

	ui "github.com/banditmoscow1337/nativeForms"
	"github.com/banditmoscow1337/nativeForms/platform"
	"github.com/banditmoscow1337/nativeForms/render"
)

type X11Options struct {
	Title              string
	Width, Height      int
	Scale              float32 // Zero uses the Xft.dpi setting, falling back to 96 DPI.
	Frameless          bool
	HideWindowControls bool
	OnReady            func(*X11Window)
	OnFilesDropped     func([]string)
	OnCloseRequest     func(*X11Window) platform.CloseAction
	Graphics           render.Kind
}

type X11Window struct {
	manager                                                       *ui.Manager
	app                                                           *App
	options                                                       X11Options
	conn                                                          net.Conn
	id, gc, root                                                  uint32
	base, mask, nextID                                            uint32
	depth                                                         byte
	minKey, maxKey                                                byte
	maxRequest                                                    int
	width, height                                                 int
	scale, lastRenderScale                                        float32
	framebuffer                                                   *image.RGBA
	staging                                                       []byte
	stagingValid                                                  bool
	request                                                       []byte
	renderer                                                      render.Backend
	fallbackReason                                                string
	lastFrame                                                     time.Time
	closeRequested                                                atomic.Bool
	hidden                                                        atomic.Bool
	keysyms                                                       map[byte][]uint32
	lastReleaseCode                                               byte
	lastReleaseTime                                               uint32
	hadRelease                                                    bool
	wmDelete, wmProtocols, motifHintsAtom                         uint32
	incrAtom                                                      uint32
	transfers                                                     map[selectionKey]*selectionTransfer
	peerRequests                                                  map[uint16]bool
	pendingReplies                                                map[uint16]bool
	replies                                                       map[uint16][]byte
	running                                                       bool
	readingSelection                                              bool
	packet                                                        []byte
	packetHave                                                    int
	dnd                                                           x11DND
	modal, portal                                                 bool
	compose                                                       composeState
	ime                                                           *linuxIME
	screenX, screenY                                              int
	originDirty                                                   bool
	clipboard, utf8Atom, targetsAtom, textAtom, selectionProperty uint32
	clipboardText                                                 string
	clipboardOwned                                                bool
	clipboardError                                                error
	sequence                                                      uint16
	incoming                                                      [32]byte
	have                                                          int
}

func NewX11(manager *ui.Manager, options X11Options) *X11Window {
	return &X11Window{manager: manager, options: options}
}

func (w *X11Window) applyMotifHints() {
	if w.conn == nil || w.id == 0 || w.motifHintsAtom == 0 {
		return
	}
	prop := make([]byte, 44) // 24-byte header + 5 uint32 _MOTIF_WM_HINTS fields
	prop[0] = 18             // ChangeProperty
	prop[1] = 0              // PropModeReplace
	le.PutUint32(prop[4:], w.id)
	le.PutUint32(prop[8:], w.motifHintsAtom)
	le.PutUint32(prop[12:], w.motifHintsAtom)
	prop[16] = 32
	le.PutUint32(prop[20:], 5)
	flags := uint32(2) // MWM_HINTS_DECORATIONS
	decorations := uint32(1)
	if w.options.Frameless {
		decorations = 0
	} else if w.options.HideWindowControls {
		// MWM_DECOR_BORDER (2) | MWM_DECOR_RESIZEH (4) | MWM_DECOR_TITLE (8)
		decorations = 2 | 4 | 8
	}
	le.PutUint32(prop[24:], flags)
	le.PutUint32(prop[32:], decorations)
	_ = w.send(prop)
}

func (w *X11Window) SetFrameless(frameless bool) {
	if w == nil {
		return
	}
	w.options.Frameless = frameless
	if w.app != nil && w.running {
		w.app.Post(func() {
			w.applyMotifHints()
			w.manager.InvalidateLayout()
		})
	}
}

func (w *X11Window) Frameless() bool {
	if w == nil {
		return false
	}
	return w.options.Frameless
}

func (w *X11Window) SetWindowControls(visible bool) {
	if w == nil {
		return
	}
	w.options.HideWindowControls = !visible
	if w.app != nil && w.running {
		w.app.Post(func() {
			w.applyMotifHints()
			w.manager.InvalidatePaint()
		})
	}
}

func (w *X11Window) WindowControlsVisible() bool {
	if w == nil {
		return false
	}
	return !w.options.HideWindowControls && !w.options.Frameless
}

func (w *X11Window) Minimize() {
	if w == nil || w.app == nil {
		return
	}
	w.app.Post(func() {
		if w.conn != nil && w.id != 0 {
			w.setVisible(false)
		}
	})
}

func (w *X11Window) Maximize() {
	if w == nil || w.app == nil {
		return
	}
	w.app.Post(func() {
		if w.conn != nil && w.id != 0 {
			w.manager.InvalidateLayout()
		}
	})
}

// SetSize requests a new logical client size. Call it on the window's UI
// goroutine after Run starts (for example from OnReady or Manager.Post).
func (w *X11Window) SetSize(width, height int) error {
	if w == nil || w.manager == nil {
		return fmt.Errorf("x11: nil window or manager")
	}
	scale := w.scale
	if scale == 0 {
		scale = w.options.Scale
	}
	if err := platform.ValidateSize(width, height, scale); err != nil {
		return err
	}
	if w.conn != nil && w.id != 0 {
		request := make([]byte, 20) // ConfigureWindow: width and height value-list entries.
		request[0] = 12
		le.PutUint32(request[4:], w.id)
		le.PutUint16(request[8:], 1<<2|1<<3)
		le.PutUint32(request[12:], uint32(platform.PixelSize(width, w.scale)))
		le.PutUint32(request[16:], uint32(platform.PixelSize(height, w.scale)))
		if err := w.send(request); err != nil {
			return err
		}
	}
	w.options.Width, w.options.Height = width, height
	return nil
}

func (w *X11Window) Capabilities() platform.Capabilities {
	c := X11Capabilities()
	c.FileDialog = w.portal
	c.IME = w.ime != nil
	return c
}

func (w *X11Window) Close() {
	if w == nil {
		return
	}
	w.closeRequested.Store(true)
	if w.app != nil {
		w.app.wake()
	}
}

func (w *X11Window) Show() { w.setVisible(true) }
func (w *X11Window) Hide() { w.setVisible(false) }
func (w *X11Window) setVisible(visible bool) {
	if w == nil {
		return
	}
	w.hidden.Store(!visible)
	if w.app != nil {
		w.app.Post(func() {
			if w.conn == nil || w.closeRequested.Load() {
				return
			}
			request := make([]byte, 8)
			if visible {
				request[0] = 8         // MapWindow
				w.stagingValid = false // the server may have discarded the pixmap
				w.manager.InvalidatePaint()
			} else {
				request[0] = 10 // UnmapWindow
				w.manager.ClearInteraction()
			}
			le.PutUint32(request[4:], w.id)
			if err := w.send(request); err != nil {
				w.app.fail(err)
			}
		})
	}
}

func applyCloseDecision(decision platform.CloseAction, app *App, hide, closeFn func()) {
	switch decision {
	case platform.HideWindow:
		hide()
	case platform.QuitApp:
		app.Quit()
	case platform.CancelClose:
	default:
		closeFn()
	}
}

func (w *X11Window) closeAction() {
	decision := platform.CloseWindow
	if w.options.OnCloseRequest != nil {
		decision = w.options.OnCloseRequest(w)
	}
	applyCloseDecision(decision, w.app, w.Hide, w.Close)
}
