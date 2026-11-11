//go:build linux

package linux

import (
	"encoding/binary"
	"fmt"
	"image"
	"net"
	"syscall"
	"time"

	ui "github.com/banditmoscow1337/nativeForms"
	"github.com/banditmoscow1337/nativeForms/platform"
	"github.com/banditmoscow1337/nativeForms/software"
)

func (w *WaylandWindow) appStart() error {
	if w == nil || w.Manager == nil {
		return fmt.Errorf("wayland: nil window or manager")
	}
	path, err := waylandPath()
	if err != nil {
		return err
	}
	conn, err := net.DialTimeout("unix", path, 2*time.Second)
	if err != nil {
		return err
	}
	unix, ok := conn.(*net.UnixConn)
	if !ok {
		conn.Close()
		return fmt.Errorf("wayland: Unix transport unavailable")
	}
	w.wire = &waylandWire{conn: unix, next: 3}
	w.scale = 1
	w.outputs = make(map[uint32]*wlOutput)
	if err = w.setup(); err != nil {
		return err
	}
	w.portal = portalAvailable()
	w.repeatRate, w.repeatDelay = 25, 600
	selection, err := software.Open(w.Options.Graphics, nil)
	if err != nil {
		return err
	}
	w.renderer = selection.Backend
	w.fallbackReason = selection.FallbackReason()
	w.buffers = make(map[uint32]*wlBuffer)
	w.pressed = make(map[uint32]bool)
	w.dataOffers = make(map[uint32]map[string]bool)
	w.dataSources = make(map[uint32]string)
	w.lastFrame = time.Now()
	if w.Options.Scale > 0 && w.Options.Scale <= 8 {
		w.Manager.SetDisplayScale(w.Options.Scale)
	}
	w.Manager.SetInteractive(true)
	w.Manager.SetWakeHandler(w.app.wake)
	if w.dataDevice != 0 {
		w.Manager.SetClipboardHandlers(w.ClipboardText, w.SetClipboardText)
	}
	if w.Options.OnReady != nil {
		w.Options.OnReady(w)
	}
	if w.hidden.Load() {
		w.setVisible(false)
	}
	return nil
}

func (w *WaylandWindow) appStep(read bool) (time.Time, error) {
	if !w.closing.Load() {
		w.Manager.ProcessPending()
		// Drain all queued Wayland events before rendering so scroll/pointer bursts
		// coalesce into a single frame.
		if read || w.wire.hasEvent() {
			for i := 0; i < 64 && !w.closing.Load(); i++ {
				object, opcode, payload, readErr := w.wire.read(time.Now())
				if timeout, ok := readErr.(net.Error); ok && timeout.Timeout() {
					break
				}
				if readErr != nil {
					return time.Time{}, readErr
				}
				if err := w.handle(object, opcode, payload); err != nil {
					return time.Time{}, err
				}
				if !w.wire.hasEvent() && !read {
					break
				}
			}
		}
		w.Manager.ProcessPending()
		if err := w.syncTextInput(false); err != nil {
			return time.Time{}, err
		}
		if !w.modal && w.repeatCode != 0 && !w.repeatAt.IsZero() && !time.Now().Before(w.repeatAt) {
			w.dispatchKey(w.repeatCode, true, true)
			w.repeatAt = time.Now().Add(time.Second / time.Duration(max(1, w.repeatRate)))
		}
		if !w.hidden.Load() && w.paintAvailable() && (w.Manager.NeedsFrame() || w.framebuffer == nil) {
			if err := w.paint(); err != nil {
				return time.Time{}, err
			}
		}
		deadline := time.Now().Add(50 * time.Millisecond)
		if !w.hidden.Load() && w.paintAvailable() {
			if next, scheduled := w.Manager.NextFrameDeadline(); scheduled && next.Before(deadline) {
				deadline = next
			}
		}
		if w.repeatCode != 0 && !w.repeatAt.IsZero() {
			deadline = minTime(deadline, w.repeatAt)
		}
		w.Manager.ProcessPending()
		if w.wire.hasEvent() || (!w.hidden.Load() && w.paintAvailable() && w.Manager.NeedsFrame()) {
			return time.Now(), nil
		}
		return deadline, nil
	}
	return time.Time{}, nil
}

func (w *WaylandWindow) appStop() {
	w.Manager.SetWakeHandler(nil)
	w.Manager.SetClipboardHandlers(nil, nil)
	w.Manager.SetInteractive(false)
	if w.wire != nil {
		for _, buffer := range w.buffers {
			_ = syscall.Munmap(buffer.pixels)
		}
		for _, fd := range w.wire.fds {
			_ = syscall.Close(fd)
		}
		_ = w.wire.conn.Close()
		w.wire = nil
	}
	if w.renderer != nil {
		_ = w.renderer.Close()
	}
	w.framebuffer, w.renderer = nil, nil
}

func (w *WaylandWindow) appClosed() bool         { return w.closing.Load() }
func (w *WaylandWindow) appManager() *ui.Manager { return w.Manager }
func (w *WaylandWindow) appOwner() *App          { return w.app }
func (w *WaylandWindow) appSetApp(a *App)        { w.app = a }
func (w *WaylandWindow) appFD() (int, error)     { return unixFD(w.wire.conn) }

func (w *WaylandWindow) paintAvailable() bool {
	if !w.ready || w.width < 1 || w.height < 1 {
		return false
	}
	for _, buffer := range w.current {
		if buffer == nil || !buffer.busy {
			return true
		}
	}
	return false
}

func (w *WaylandWindow) paint() error {
	pw, ph := platform.PixelSize(w.width, w.scale), platform.PixelSize(w.height, w.scale)
	if pw < 1 || ph < 1 || pw > 16384 || ph > 16384 || int64(pw)*int64(ph) > 64*1024*1024 {
		return fmt.Errorf("wayland: invalid window dimensions")
	}
	if w.framebuffer == nil || w.framebuffer.Bounds().Dx() != pw || w.framebuffer.Bounds().Dy() != ph {
		w.retireBuffers()
		w.framebuffer = image.NewRGBA(image.Rect(0, 0, pw, ph))
	}
	var buffer *wlBuffer
	for i := range w.current {
		if w.current[i] == nil {
			newBuffer, err := w.createBuffer()
			if err != nil {
				return err
			}
			w.current[i] = newBuffer
		}
		if !w.current[i].busy && buffer == nil {
			buffer = w.current[i]
		}
	}
	if buffer == nil {
		return nil
	} // Wait for wl_buffer.release before reusing memory.
	uiScale := w.uiScale()
	if w.lastUIScale != uiScale {
		w.lastUIScale = uiScale
		for _, b := range w.current {
			if b != nil {
				b.valid = false
			}
		}
	}
	logicalW := platform.LogicalSize(w.width, uiScale)
	logicalH := platform.LogicalSize(w.height, uiScale)
	renderScale := min(w.scale*uiScale, 8)
	now := time.Now()
	w.Manager.BeginFrame(logicalW, logicalH, now.Sub(w.lastFrame).Seconds())
	w.lastFrame = now
	trace := w.Manager.FrameTrace()
	rasterStart := trace.Start()
	err := w.renderer.RenderAtScale(w.Manager.Frame(), w.framebuffer, renderScale)
	trace.Record(ui.PhaseRaster, rasterStart)
	if err != nil {
		return err
	}
	uploadStart := trace.Start()
	dirty := software.UpdateBGRA(buffer.pixels, w.framebuffer, buffer.valid)
	trace.RecordDamage(dirty.Dx(), dirty.Dy())
	buffer.valid = true
	trace.Record(ui.PhaseUpload, uploadStart)
	if dirty.Empty() {
		trace.Finish("software/wayland", w.fallbackReason)
		return nil
	}
	presentStart := trace.Start()
	args := append(wlWord(buffer.id), wlWord(0)...)
	args = append(args, wlWord(0)...)
	if w.viewport != 0 {
		destination := append(wlWord(uint32(w.width)), wlWord(uint32(w.height))...)
		if err := w.wire.send(w.viewport, 2, destination); err != nil {
			return err
		}
	}
	if err := w.wire.send(w.surface, 1, args); err != nil {
		return err
	}
	// wl_surface.damage uses logical surface coordinates. Round outward so
	// fractional display scales never leave a changed pixel undamaged.
	x0 := max(0, int(float32(dirty.Min.X)/w.scale))
	y0 := max(0, int(float32(dirty.Min.Y)/w.scale))
	x1 := min(w.width, int(float32(dirty.Max.X)/w.scale+1))
	y1 := min(w.height, int(float32(dirty.Max.Y)/w.scale+1))
	damage := make([]byte, 16)
	binary.LittleEndian.PutUint32(damage[0:], uint32(x0))
	binary.LittleEndian.PutUint32(damage[4:], uint32(y0))
	binary.LittleEndian.PutUint32(damage[8:], uint32(x1-x0))
	binary.LittleEndian.PutUint32(damage[12:], uint32(y1-y0))
	if err := w.wire.send(w.surface, 2, damage); err != nil {
		return err
	}
	if err := w.wire.send(w.surface, 6, nil); err != nil {
		return err
	}
	buffer.busy = true
	trace.Record(ui.PhasePresent, presentStart)
	trace.Finish("software/wayland", w.fallbackReason)
	return nil
}

func (w *WaylandWindow) updateScale() error {
	scale := w.desiredScale()
	if scale == w.scale {
		return nil
	}
	if w.viewport == 0 {
		if err := w.wire.send(w.surface, 8, wlWord(uint32(scale))); err != nil {
			return err
		}
	}
	w.scale = scale
	w.Manager.SetBackingScale(scale)
	return nil
}

func (w *WaylandWindow) uiScale() float32 {
	if w != nil && w.Manager != nil {
		if s := w.Manager.DisplayScale(); s > 0 && s <= 8 {
			return s
		}
	}
	return 1
}

func (w *WaylandWindow) desiredScale() float32 {
	if w.viewport != 0 && w.preferredScale > 0 {
		return w.preferredScale
	}
	scale := float32(1)
	if w.compositorVersion >= 3 || w.viewport != 0 {
		for _, output := range w.outputs {
			if output.entered {
				scale = max(scale, float32(output.scale))
			}
		}
	}
	return scale
}
