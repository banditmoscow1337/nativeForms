//go:build linux

package linux

import (
	"encoding/binary"
	"fmt"
	"image"
	"io"
	"net"
	"os"
	"sync/atomic"
	"syscall"
	"time"
	"unicode"

	ui "github.com/banditmoscow1337/nativeForms"
	"github.com/banditmoscow1337/nativeForms/platform"
	"github.com/banditmoscow1337/nativeForms/software"
)

type WaylandOptions struct {
	Title         string
	Width, Height int
	OnReady       func(*WaylandWindow)
}

type waylandWire struct {
	conn    *net.UnixConn
	next    uint32
	pending []byte
	fds     []int
}

func (wire *waylandWire) id() uint32 { wire.next++; return wire.next }

func wlWord(value uint32) []byte {
	b := make([]byte, 4)
	binary.LittleEndian.PutUint32(b, value)
	return b
}

func wlString(value string) []byte {
	n := len(value) + 1
	b := make([]byte, 4+padded(n))
	binary.LittleEndian.PutUint32(b, uint32(n))
	copy(b[4:], value)
	return b
}

func (wire *waylandWire) send(object uint32, opcode uint16, args []byte) error {
	if len(args)%4 != 0 || len(args) > 65535-8 {
		return fmt.Errorf("wayland: invalid request length")
	}
	b := make([]byte, 8+len(args))
	binary.LittleEndian.PutUint32(b, object)
	binary.LittleEndian.PutUint32(b[4:], uint32(len(b))<<16|uint32(opcode))
	copy(b[8:], args)
	for len(b) > 0 {
		n, err := wire.conn.Write(b)
		if err != nil {
			return err
		}
		if n == 0 {
			return io.ErrShortWrite
		}
		b = b[n:]
	}
	return nil
}

func (wire *waylandWire) sendFD(object uint32, opcode uint16, args []byte, fd int) error {
	if len(args)%4 != 0 {
		return fmt.Errorf("wayland: invalid fd request")
	}
	b := make([]byte, 8+len(args))
	binary.LittleEndian.PutUint32(b, object)
	binary.LittleEndian.PutUint32(b[4:], uint32(len(b))<<16|uint32(opcode))
	copy(b[8:], args)
	n, _, err := wire.conn.WriteMsgUnix(b, syscall.UnixRights(fd), nil)
	if err != nil {
		return err
	}
	if n == 0 {
		return io.ErrShortWrite
	}
	if n < len(b) {
		for tail := b[n:]; len(tail) > 0; {
			written, writeErr := wire.conn.Write(tail)
			if writeErr != nil {
				return writeErr
			}
			if written == 0 {
				return io.ErrShortWrite
			}
			tail = tail[written:]
		}
	}
	return nil
}

func (wire *waylandWire) read(deadline time.Time) (uint32, uint16, []byte, error) {
	for {
		if len(wire.pending) >= 8 {
			word := binary.LittleEndian.Uint32(wire.pending[4:])
			size := int(word >> 16)
			if size < 8 || size > 65535 || size%4 != 0 {
				return 0, 0, nil, fmt.Errorf("wayland: invalid event size %d", size)
			}
			if len(wire.pending) >= size {
				object := binary.LittleEndian.Uint32(wire.pending)
				payload := append([]byte(nil), wire.pending[8:size]...)
				wire.pending = wire.pending[size:]
				return object, uint16(word), payload, nil
			}
		}
		_ = wire.conn.SetReadDeadline(deadline)
		var data [16384]byte
		var control [256]byte
		n, oob, _, _, err := wire.conn.ReadMsgUnix(data[:], control[:])
		if n > 0 {
			wire.pending = append(wire.pending, data[:n]...)
		}
		if oob > 0 {
			messages, parseErr := syscall.ParseSocketControlMessage(control[:oob])
			if parseErr != nil {
				return 0, 0, nil, parseErr
			}
			for _, message := range messages {
				fds, rightsErr := syscall.ParseUnixRights(&message)
				if rightsErr == nil {
					wire.fds = append(wire.fds, fds...)
				}
			}
		}
		if err != nil {
			return 0, 0, nil, err
		}
		if n == 0 {
			return 0, 0, nil, io.ErrUnexpectedEOF
		}
	}
}

func (wire *waylandWire) takeFD() (int, error) {
	if len(wire.fds) == 0 {
		return -1, fmt.Errorf("wayland: keymap fd missing")
	}
	fd := wire.fds[0]
	wire.fds = wire.fds[1:]
	return fd, nil
}

func (wire *waylandWire) bind(global WaylandGlobal, version uint32) (uint32, error) {
	id := wire.id()
	args := append(wlWord(global.Name), wlString(global.Interface)...)
	args = append(args, wlWord(min(global.Version, version))...)
	args = append(args, wlWord(id)...)
	return id, wire.send(2, 0, args)
}

type wlBuffer struct {
	id            uint32
	pixels        []byte
	busy, retired bool
}

// WaylandWindow uses xdg-shell and wl_shm directly over the compositor's Unix
// socket. Keyboard text uses the US evdev layout until XKB parsing is added.
type WaylandWindow struct {
	Manager                                                              *ui.Manager
	Options                                                              WaylandOptions
	wire                                                                 *waylandWire
	compositor, shm, wm, surface, xdg, toplevel, seat, pointer, keyboard uint32
	width, height                                                        int
	framebuffer                                                          *image.RGBA
	renderer                                                             *software.Renderer
	buffers                                                              map[uint32]*wlBuffer
	current                                                              [2]*wlBuffer
	ready                                                                bool
	closing                                                              atomic.Bool
	lastFrame                                                            time.Time
	pointerPos                                                           ui.Vec2
	pressed                                                              map[uint32]bool
	globals                                                              map[uint32]string
}

func NewWayland(manager *ui.Manager, options WaylandOptions) *WaylandWindow {
	return &WaylandWindow{Manager: manager, Options: options}
}

func (w *WaylandWindow) Capabilities() platform.Capabilities { return WaylandCapabilities() }

func (w *WaylandWindow) Close() {
	if w == nil {
		return
	}
	w.closing.Store(true)
}

func (w *WaylandWindow) Run() error {
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
	defer func() {
		for _, buffer := range w.buffers {
			_ = syscall.Munmap(buffer.pixels)
		}
		for _, fd := range w.wire.fds {
			_ = syscall.Close(fd)
		}
		_ = unix.Close()
	}()
	if err = w.setup(); err != nil {
		return err
	}
	w.renderer = software.New()
	w.buffers = make(map[uint32]*wlBuffer)
	w.pressed = make(map[uint32]bool)
	w.lastFrame = time.Now()
	w.Manager.SetInteractive(true)
	w.Manager.SetWakeHandler(func() { _ = unix.SetReadDeadline(time.Now()) })
	defer func() { w.Manager.SetWakeHandler(nil); w.Manager.SetInteractive(false) }()
	if w.Options.OnReady != nil {
		w.Options.OnReady(w)
	}
	for !w.closing.Load() {
		if w.ready && (w.Manager.NeedsFrame() || w.framebuffer == nil) {
			if err = w.paint(); err != nil {
				return err
			}
		}
		deadline := time.Now().Add(50 * time.Millisecond)
		if d := w.Manager.NextFrameAfter(); d > 0 && d < 50*time.Millisecond {
			deadline = time.Now().Add(d)
		}
		object, opcode, payload, readErr := w.wire.read(deadline)
		if timeout, ok := readErr.(net.Error); ok && timeout.Timeout() {
			if w.ready && w.Manager.NextFrameAfter() <= time.Millisecond {
				w.Manager.InvalidatePaint()
			}
			continue
		}
		if readErr != nil {
			return readErr
		}
		if err = w.handle(object, opcode, payload); err != nil {
			return err
		}
	}
	return nil
}

func (w *WaylandWindow) setup() error {
	// wl_display.get_registry(2), wl_display.sync(3).
	if err := w.wire.send(1, 1, wlWord(2)); err != nil {
		return err
	}
	if err := w.wire.send(1, 0, wlWord(3)); err != nil {
		return err
	}
	var globals []WaylandGlobal
	for {
		object, opcode, payload, err := w.wire.read(time.Now().Add(3 * time.Second))
		if err != nil {
			return err
		}
		if object == 1 && opcode == 0 {
			return fmt.Errorf("wayland: compositor rejected registry")
		}
		if object == 3 && opcode == 0 {
			break
		}
		if object == 2 && opcode == 0 && len(payload) >= 12 {
			n := int(binary.LittleEndian.Uint32(payload[4:]))
			offset := 8 + padded(n)
			if n < 1 || offset+4 > len(payload) {
				return fmt.Errorf("wayland: invalid global")
			}
			globals = append(globals, WaylandGlobal{Name: binary.LittleEndian.Uint32(payload),
				Interface: string(payload[8 : 8+n-1]), Version: binary.LittleEndian.Uint32(payload[offset:])})
		}
	}
	for _, global := range globals {
		var id uint32
		var err error
		switch global.Interface {
		case "wl_compositor":
			if w.compositor != 0 {
				continue
			}
			id, err = w.wire.bind(global, 1)
			w.compositor = id
		case "wl_shm":
			if w.shm != 0 {
				continue
			}
			id, err = w.wire.bind(global, 1)
			w.shm = id
		case "xdg_wm_base":
			if w.wm != 0 {
				continue
			}
			id, err = w.wire.bind(global, 1)
			w.wm = id
		case "wl_seat":
			if w.seat != 0 {
				continue
			}
			id, err = w.wire.bind(global, 1)
			w.seat = id
		}
		if err != nil {
			return err
		}
		if id != 0 {
			if w.globals == nil {
				w.globals = make(map[uint32]string)
			}
			w.globals[global.Name] = global.Interface
		}
	}
	if w.compositor == 0 || w.shm == 0 || w.wm == 0 {
		return fmt.Errorf("wayland: compositor lacks wl_compositor, wl_shm or xdg_wm_base")
	}
	w.surface = w.wire.id()
	if err := w.wire.send(w.compositor, 0, wlWord(w.surface)); err != nil {
		return err
	}
	w.xdg = w.wire.id()
	if err := w.wire.send(w.wm, 2, append(wlWord(w.xdg), wlWord(w.surface)...)); err != nil {
		return err
	}
	w.toplevel = w.wire.id()
	if err := w.wire.send(w.xdg, 1, wlWord(w.toplevel)); err != nil {
		return err
	}
	title := w.Options.Title
	if title == "" {
		title = "nativeForms"
	}
	if err := w.wire.send(w.toplevel, 2, wlString(title)); err != nil {
		return err
	}
	w.width, w.height = w.Options.Width, w.Options.Height
	if w.width <= 0 {
		w.width = 800
	}
	if w.height <= 0 {
		w.height = 600
	}
	return w.wire.send(w.surface, 6, nil) // Empty initial commit; wait for configure.
}

func (w *WaylandWindow) createBuffer() (*wlBuffer, error) {
	count := int64(w.width) * int64(w.height) * 4
	if count <= 0 || count > 256<<20 {
		return nil, fmt.Errorf("wayland: framebuffer exceeds 64 megapixels")
	}
	path := os.Getenv("XDG_RUNTIME_DIR")
	if path == "" {
		path = os.TempDir()
	}
	file, err := os.CreateTemp(path, "nativeforms-shm-")
	if err != nil {
		return nil, err
	}
	_ = os.Remove(file.Name())
	defer file.Close()
	if err = file.Truncate(count); err != nil {
		return nil, err
	}
	pixels, err := syscall.Mmap(int(file.Fd()), 0, int(count), syscall.PROT_READ|syscall.PROT_WRITE, syscall.MAP_SHARED)
	if err != nil {
		return nil, err
	}
	pool := w.wire.id()
	args := append(wlWord(pool), wlWord(uint32(count))...)
	if err = w.wire.sendFD(w.shm, 0, args, int(file.Fd())); err != nil {
		_ = syscall.Munmap(pixels)
		return nil, err
	}
	id := w.wire.id()
	args = append(wlWord(id), wlWord(0)...)
	for _, value := range []int{w.width, w.height, w.width * 4} {
		args = append(args, wlWord(uint32(value))...)
	}
	args = append(args, wlWord(0)...) // WL_SHM_FORMAT_ARGB8888.
	if err = w.wire.send(pool, 0, args); err != nil {
		_ = syscall.Munmap(pixels)
		return nil, err
	}
	if err = w.wire.send(pool, 1, nil); err != nil {
		_ = syscall.Munmap(pixels)
		return nil, err
	}
	buffer := &wlBuffer{id: id, pixels: pixels}
	w.buffers[id] = buffer
	return buffer, nil
}

func (w *WaylandWindow) retireBuffers() {
	for i, buffer := range w.current {
		if buffer == nil {
			continue
		}
		buffer.retired = true
		if !buffer.busy {
			w.destroyBuffer(buffer)
		}
		w.current[i] = nil
	}
}

func (w *WaylandWindow) destroyBuffer(buffer *wlBuffer) {
	_ = w.wire.send(buffer.id, 0, nil)
	_ = syscall.Munmap(buffer.pixels)
	delete(w.buffers, buffer.id)
}

func (w *WaylandWindow) paint() error {
	if w.width < 1 || w.height < 1 || w.width > 16384 || w.height > 16384 || int64(w.width)*int64(w.height) > 64*1024*1024 {
		return fmt.Errorf("wayland: invalid window dimensions")
	}
	if w.framebuffer == nil || w.framebuffer.Bounds().Dx() != w.width || w.framebuffer.Bounds().Dy() != w.height {
		w.retireBuffers()
		w.framebuffer = image.NewRGBA(image.Rect(0, 0, w.width, w.height))
		w.Manager.InvalidateLayout()
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
	now := time.Now()
	w.Manager.BeginFrame(w.width, w.height, now.Sub(w.lastFrame).Seconds())
	w.lastFrame = now
	if err := w.renderer.Render(w.Manager.Frame(), w.framebuffer); err != nil {
		return err
	}
	for y := 0; y < w.height; y++ {
		for x := 0; x < w.width; x++ {
			i := w.framebuffer.PixOffset(x, y)
			j := (y*w.width + x) * 4
			buffer.pixels[j], buffer.pixels[j+1], buffer.pixels[j+2], buffer.pixels[j+3] =
				w.framebuffer.Pix[i+2], w.framebuffer.Pix[i+1], w.framebuffer.Pix[i], w.framebuffer.Pix[i+3]
		}
	}
	args := append(wlWord(buffer.id), wlWord(0)...)
	args = append(args, wlWord(0)...)
	if err := w.wire.send(w.surface, 1, args); err != nil {
		return err
	}
	damage := make([]byte, 16)
	binary.LittleEndian.PutUint32(damage[8:], uint32(w.width))
	binary.LittleEndian.PutUint32(damage[12:], uint32(w.height))
	if err := w.wire.send(w.surface, 2, damage); err != nil {
		return err
	}
	if err := w.wire.send(w.surface, 6, nil); err != nil {
		return err
	}
	buffer.busy = true
	return nil
}

func (w *WaylandWindow) handle(object uint32, opcode uint16, payload []byte) error {
	u32 := func(offset int) uint32 {
		if offset+4 > len(payload) {
			return 0
		}
		return binary.LittleEndian.Uint32(payload[offset:])
	}
	switch object {
	case 1:
		if opcode == 0 {
			return fmt.Errorf("wayland: compositor protocol error (%d): %s", u32(4), string(payload[min(12, len(payload)):]))
		}
	case 2:
		if opcode == 1 {
			switch w.globals[u32(0)] {
			case "wl_compositor", "wl_shm", "xdg_wm_base":
				return fmt.Errorf("wayland: required global was removed")
			}
		}
	case w.wm:
		if opcode == 0 {
			return w.wire.send(w.wm, 3, wlWord(u32(0)))
		}
	case w.toplevel:
		if opcode == 1 {
			w.Close()
			return nil
		}
		if opcode == 0 && len(payload) >= 8 {
			width, height := int(int32(u32(0))), int(int32(u32(4)))
			if width > 0 && height > 0 && (width != w.width || height != w.height) {
				w.width, w.height = width, height
				w.Manager.InvalidateLayout()
			}
		}
	case w.xdg:
		if opcode == 0 && len(payload) >= 4 {
			if err := w.wire.send(w.xdg, 4, wlWord(u32(0))); err != nil {
				return err
			}
			w.ready = true
			w.Manager.InvalidatePaint()
		}
	case w.seat:
		if opcode == 0 && len(payload) >= 4 {
			capabilities := u32(0)
			if capabilities&1 != 0 && w.pointer == 0 {
				w.pointer = w.wire.id()
				if err := w.wire.send(w.seat, 0, wlWord(w.pointer)); err != nil {
					return err
				}
			}
			if capabilities&2 != 0 && w.keyboard == 0 {
				w.keyboard = w.wire.id()
				if err := w.wire.send(w.seat, 1, wlWord(w.keyboard)); err != nil {
					return err
				}
			}
		}
	case w.pointer:
		return w.pointerEvent(opcode, payload)
	case w.keyboard:
		return w.keyboardEvent(opcode, payload)
	default:
		if buffer := w.buffers[object]; buffer != nil && opcode == 0 {
			buffer.busy = false
			if buffer.retired {
				w.destroyBuffer(buffer)
			}
		}
	}
	return nil
}

func (w *WaylandWindow) pointerEvent(opcode uint16, p []byte) error {
	u := func(offset int) uint32 {
		if offset+4 > len(p) {
			return 0
		}
		return binary.LittleEndian.Uint32(p[offset:])
	}
	switch opcode {
	case 0, 2:
		offset := 4
		if opcode == 0 {
			offset = 8
		}
		if len(p) < offset+8 {
			return fmt.Errorf("wayland: short pointer event")
		}
		w.pointerPos = ui.Vec2{X: float32(int32(u(offset))) / 256, Y: float32(int32(u(offset+4))) / 256}
		w.Manager.HandleEvent(ui.Event{Type: ui.PointerMove, X: w.pointerPos.X, Y: w.pointerPos.Y})
	case 1:
		w.Manager.HandleEvent(ui.Event{Type: ui.PointerMove, X: -1e6, Y: -1e6})
	case 3:
		if len(p) < 16 {
			return fmt.Errorf("wayland: short pointer button")
		}
		button := ui.MouseLeft
		switch u(8) {
		case 0x110:
			button = ui.MouseLeft
		case 0x111:
			button = ui.MouseRight
		case 0x112:
			button = ui.MouseMiddle
		default:
			return nil
		}
		kind := ui.PointerDown
		if u(12) == 0 {
			kind = ui.PointerUp
		}
		w.Manager.HandleEvent(ui.Event{Type: kind, Button: button, X: w.pointerPos.X, Y: w.pointerPos.Y})
	case 4:
		if len(p) < 12 {
			return fmt.Errorf("wayland: short pointer axis")
		}
		value := -float32(int32(u(8))) / 256 / 40
		scroll := ui.Vec2{}
		if u(4) == 0 {
			scroll.Y = value
		} else {
			scroll.X = value
		}
		w.Manager.HandleEvent(ui.Event{Type: ui.PointerScroll, X: w.pointerPos.X, Y: w.pointerPos.Y, Scroll: scroll})
	}
	return nil
}

func (w *WaylandWindow) keyboardEvent(opcode uint16, p []byte) error {
	u := func(offset int) uint32 {
		if offset+4 > len(p) {
			return 0
		}
		return binary.LittleEndian.Uint32(p[offset:])
	}
	switch opcode {
	case 0:
		fd, err := w.wire.takeFD()
		if err != nil {
			return err
		}
		_ = syscall.Close(fd) // XKB parsing and compose still need implementation.
	case 2:
		w.pressed = make(map[uint32]bool)
		w.Manager.ClearInteraction()
		if len(p) >= 12 {
			size := int(u(8))
			if size <= len(p)-12 {
				for i := 12; i+4 <= 12+size; i += 4 {
					w.pressed[binary.LittleEndian.Uint32(p[i:])] = true
				}
			}
		}
	case 3:
		if len(p) < 16 {
			return fmt.Errorf("wayland: short keyboard key")
		}
		code, down := u(8), u(12) != 0
		w.pressed[code] = down
		mods := 0
		if w.pressed[42] || w.pressed[54] {
			mods |= ui.ModShift
		}
		if w.pressed[29] || w.pressed[97] {
			mods |= ui.ModControl
		}
		if w.pressed[56] || w.pressed[100] {
			mods |= ui.ModAlt
		}
		if w.pressed[125] || w.pressed[126] {
			mods |= ui.ModSuper
		}
		key, character := evdevUS(code, mods&ui.ModShift != 0)
		kind := ui.KeyDown
		if !down {
			kind = ui.KeyUp
		}
		w.Manager.HandleEvent(ui.Event{Type: kind, Key: key, Mods: mods})
		if down && character >= 32 && mods&(ui.ModControl|ui.ModAlt|ui.ModSuper) == 0 && unicode.IsPrint(character) {
			w.Manager.HandleEvent(ui.Event{Type: ui.TextInput, Rune: character})
		}
	}
	return nil
}

func evdevUS(code uint32, shift bool) (ui.Key, rune) {
	switch code {
	case 1:
		return ui.KeyEscape, 0
	case 14:
		return ui.KeyBackspace, 0
	case 15:
		return ui.KeyTab, 0
	case 28:
		return ui.KeyEnter, 0
	case 57:
		return ui.KeySpace, ' '
	case 102:
		return ui.KeyHome, 0
	case 107:
		return ui.KeyEnd, 0
	case 111:
		return ui.KeyDelete, 0
	case 103:
		return ui.KeyUpArrow, 0
	case 105:
		return ui.KeyLeftArrow, 0
	case 106:
		return ui.KeyRightArrow, 0
	case 108:
		return ui.KeyDownArrow, 0
	case 63:
		return ui.KeyF5, 0
	case 64:
		return ui.KeyF6, 0
	case 66:
		return ui.KeyF8, 0
	}
	if letter := waylandUSLetters[code]; letter != 0 {
		key := ui.KeyUnknown
		switch letter {
		case 'a':
			key = ui.KeyA
		case 'c':
			key = ui.KeyC
		case 'd':
			key = ui.KeyD
		case 'e':
			key = ui.KeyE
		case 'f':
			key = ui.KeyF
		case 'q':
			key = ui.KeyQ
		case 'r':
			key = ui.KeyR
		case 's':
			key = ui.KeyS
		case 'v':
			key = ui.KeyV
		case 'w':
			key = ui.KeyW
		case 'x':
			key = ui.KeyX
		case 'y':
			key = ui.KeyY
		case 'z':
			key = ui.KeyZ
		}
		if shift {
			letter = unicode.ToUpper(letter)
		}
		return key, letter
	}
	if pair, ok := waylandUSPunctuation[code]; ok {
		if shift {
			return ui.KeyUnknown, pair[1]
		}
		return ui.KeyUnknown, pair[0]
	}
	return ui.KeyUnknown, 0
}

var waylandUSLetters = map[uint32]rune{16: 'q', 17: 'w', 18: 'e', 19: 'r', 20: 't', 21: 'y', 22: 'u', 23: 'i', 24: 'o', 25: 'p', 30: 'a', 31: 's', 32: 'd', 33: 'f', 34: 'g', 35: 'h', 36: 'j', 37: 'k', 38: 'l', 44: 'z', 45: 'x', 46: 'c', 47: 'v', 48: 'b', 49: 'n', 50: 'm'}

var waylandUSPunctuation = map[uint32][2]rune{
	2: {'1', '!'}, 3: {'2', '@'}, 4: {'3', '#'}, 5: {'4', '$'}, 6: {'5', '%'},
	7: {'6', '^'}, 8: {'7', '&'}, 9: {'8', '*'}, 10: {'9', '('}, 11: {'0', ')'},
	12: {'-', '_'}, 13: {'=', '+'}, 26: {'[', '{'}, 27: {']', '}'}, 39: {';', ':'},
	40: {'\'', '"'}, 41: {'`', '~'}, 43: {'\\', '|'}, 51: {',', '<'}, 52: {'.', '>'}, 53: {'/', '?'},
}
