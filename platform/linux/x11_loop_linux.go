//go:build linux

package linux

import (
	"fmt"
	"image"
	"net"
	"time"

	ui "github.com/banditmoscow1337/nativeForms"
	"github.com/banditmoscow1337/nativeForms/platform"
	"github.com/banditmoscow1337/nativeForms/software"
)

func (w *X11Window) Run() error {
	a := NewApp()
	a.autoQuit = true
	if err := a.Add(w); err != nil {
		return err
	}
	return a.Run()
}

func (w *X11Window) appStart() error {
	if w == nil || w.manager == nil {
		return fmt.Errorf("x11: nil window or manager")
	}
	path, screen, display, err := x11Display()
	if err != nil {
		return err
	}
	w.conn, err = net.Dial("unix", path)
	if err != nil {
		return err
	}
	if err = w.open(screen, display); err != nil {
		return err
	}
	w.scale = w.resourceScale()
	if !(w.scale > 0 && w.scale <= 8) {
		w.scale = 1
	}
	w.manager.SetBackingScale(w.scale)
	if w.options.Scale > 0 && w.options.Scale <= 8 {
		w.manager.SetDisplayScale(w.options.Scale)
	}
	if err = w.createWindow(); err != nil {
		return err
	}
	w.originDirty = true
	w.running = true
	w.portal = portalAvailable()
	w.ime = openLinuxIME(w.manager)
	if w.ime != nil {
		w.ime.scale = w.effectiveScale()
	}
	selection, err := software.Open(w.options.Graphics, nil)
	if err != nil {
		return err
	}
	w.renderer = selection.Backend
	w.fallbackReason = selection.FallbackReason()
	w.lastFrame = time.Now()
	w.manager.SetInteractive(true)
	w.manager.SetClipboardHandlers(w.ClipboardText, w.SetClipboardText)
	w.manager.SetWakeHandler(w.app.wake)
	if w.options.OnReady != nil {
		w.options.OnReady(w)
	}
	if w.hidden.Load() {
		w.setVisible(false)
	}
	return nil
}

func (w *X11Window) appStep(read bool) (time.Time, error) {
	if !w.closeRequested.Load() {
		w.manager.ProcessPending()
		// Drain all queued input packets first so high-frequency wheel/motion bursts
		// coalesce into a single frame render instead of painting per packet.
		if read {
			for i := 0; i < 64 && !w.closeRequested.Load(); i++ {
				message, err := w.clipboardPacket(time.Now())
				if timeout, ok := err.(net.Error); ok && timeout.Timeout() {
					break
				}
				if err != nil {
					return time.Time{}, err
				}
				if err := w.clipboardDispatch(message); err != nil {
					return time.Time{}, err
				}
			}
		}
		w.manager.ProcessPending()
		if w.ime != nil {
			w.ime.scale = w.effectiveScale()
			if w.originDirty && w.manager.TextInputFocused() {
				_ = w.refreshOrigin()
			}
			w.ime.screenX, w.ime.screenY = w.screenX, w.screenY
			if err := w.ime.sync(); err != nil {
				w.ime.close()
				w.ime = nil
			}
		}
		for k, t := range w.transfers {
			if time.Now().After(t.expires) {
				delete(w.transfers, k)
			}
		}
		if !w.hidden.Load() && (w.framebuffer == nil || w.manager.NeedsFrame()) {
			if err := w.paint(); err != nil {
				return time.Time{}, err
			}
		}
		deadline := time.Now().Add(50 * time.Millisecond)
		if next, scheduled := w.manager.NextFrameDeadline(); !w.hidden.Load() && w.width > 0 && w.height > 0 && scheduled && next.Before(deadline) {
			deadline = next
		}
		w.manager.ProcessPending()
		if !w.hidden.Load() && w.width > 0 && w.height > 0 && w.manager.NeedsFrame() {
			return time.Now(), nil
		}
		return deadline, nil
	}
	return time.Time{}, nil
}

func (w *X11Window) appStop() {
	w.running = false
	w.manager.SetWakeHandler(nil)
	w.manager.SetClipboardHandlers(nil, nil)
	w.manager.SetInteractive(false)
	if w.ime != nil {
		w.ime.close()
		w.ime = nil
	}
	if w.conn != nil {
		_ = w.conn.Close()
		w.conn = nil
	}
	if w.renderer != nil {
		_ = w.renderer.Close()
	}
	w.framebuffer, w.renderer = nil, nil
}

func (w *X11Window) appClosed() bool         { return w.closeRequested.Load() }
func (w *X11Window) appManager() *ui.Manager { return w.manager }
func (w *X11Window) appOwner() *App          { return w.app }
func (w *X11Window) appSetApp(a *App)        { w.app = a }
func (w *X11Window) appFD() (int, error)     { return unixFD(w.conn.(*net.UnixConn)) }

func (w *X11Window) effectiveScale() float32 {
	base := w.scale
	if !(base > 0 && base <= 8) {
		base = 1
	}
	uiScale := float32(1)
	if w.manager != nil {
		if s := w.manager.DisplayScale(); s > 0 && s <= 8 {
			uiScale = s
		}
	}
	return min(base*uiScale, 8)
}

func (w *X11Window) paint() error {
	if w.width <= 0 || w.height <= 0 {
		return nil
	}
	if int64(w.width)*int64(w.height) > 64*1024*1024 {
		return fmt.Errorf("x11: framebuffer exceeds 64 megapixels")
	}
	renderScale := w.effectiveScale()
	if w.framebuffer == nil || w.framebuffer.Bounds().Dx() != w.width || w.framebuffer.Bounds().Dy() != w.height || w.lastRenderScale != renderScale {
		if w.framebuffer == nil || w.framebuffer.Bounds().Dx() != w.width || w.framebuffer.Bounds().Dy() != w.height {
			w.framebuffer = image.NewRGBA(image.Rect(0, 0, w.width, w.height))
			w.staging = make([]byte, w.width*w.height*4)
		}
		w.lastRenderScale = renderScale
		w.stagingValid = false
	}
	now := time.Now()
	w.manager.BeginFrame(platform.LogicalSize(w.width, renderScale), platform.LogicalSize(w.height, renderScale), now.Sub(w.lastFrame).Seconds())
	w.lastFrame = now
	trace := w.manager.FrameTrace()
	rasterStart := trace.Start()
	err := w.renderer.RenderAtScale(w.manager.Frame(), w.framebuffer, renderScale)
	trace.Record(ui.PhaseRaster, rasterStart)
	if err != nil {
		return err
	}
	uploadStart := trace.Start()
	dirty := software.UpdateBGRA(w.staging, w.framebuffer, w.stagingValid)
	trace.RecordDamage(dirty.Dx(), dirty.Dy())
	w.stagingValid = true
	trace.Record(ui.PhaseUpload, uploadStart)
	if dirty.Empty() {
		trace.Finish("software/x11", w.fallbackReason)
		return nil
	}
	rowBytes := dirty.Dx() * 4
	rowsPerRequest := (w.maxRequest - 24) / rowBytes
	if rowsPerRequest < 1 {
		return fmt.Errorf("x11: scanline exceeds maximum request size")
	}
	for y := dirty.Min.Y; y < dirty.Max.Y; {
		uploadStart := trace.Start()
		n := min(rowsPerRequest, dirty.Max.Y-y)
		length := 24 + n*rowBytes
		if cap(w.request) < length {
			w.request = make([]byte, length)
		}
		request := w.request[:length]
		clear(request[:24])
		request[0] = 72
		request[1] = 2 // ZPixmap
		le.PutUint32(request[4:], w.id)
		le.PutUint32(request[8:], w.gc)
		le.PutUint16(request[12:], uint16(dirty.Dx()))
		le.PutUint16(request[14:], uint16(n))
		le.PutUint16(request[16:], uint16(dirty.Min.X))
		le.PutUint16(request[18:], uint16(y))
		request[21] = w.depth
		for row := 0; row < n; row++ {
			start := ((y+row)*w.width + dirty.Min.X) * 4
			output := request[24+row*rowBytes : 24+(row+1)*rowBytes]
			copy(output, w.staging[start:start+rowBytes])
			if w.depth == 24 {
				for x := 3; x < len(output); x += 4 {
					output[x] = 0 // preserve unused pad bits in 24-bit ZPixmap
				}
			}
		}
		trace.Record(ui.PhaseUpload, uploadStart)
		presentStart := trace.Start()
		if err := w.send(request); err != nil {
			return err
		}
		trace.Record(ui.PhasePresent, presentStart)
		y += n
	}
	trace.Finish("software/x11", w.fallbackReason)
	return nil
}
