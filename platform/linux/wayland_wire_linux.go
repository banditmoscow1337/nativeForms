//go:build linux

package linux

import (
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"os"
	"syscall"
	"time"
)

type waylandWire struct {
	conn    *net.UnixConn
	next    uint32
	pending []byte
	fds     []int
}

func (wire *waylandWire) hasEvent() bool {
	if len(wire.pending) < 8 {
		return false
	}
	size := int(binary.LittleEndian.Uint32(wire.pending[4:]) >> 16)
	return size >= 8 && len(wire.pending) >= size
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
	id                   uint32
	pixels               []byte
	busy, retired, valid bool
}

type wlOutput struct {
	global  uint32
	scale   int
	entered bool
}

func (w *WaylandWindow) createBuffer() (*wlBuffer, error) {
	pw, ph := w.framebuffer.Bounds().Dx(), w.framebuffer.Bounds().Dy()
	count := int64(pw) * int64(ph) * 4
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
	for _, value := range []int{pw, ph, pw * 4} {
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
			w.compositorVersion = int(min(global.Version, 4))
			id, err = w.wire.bind(global, uint32(w.compositorVersion))
			w.compositor = id
		case "wl_output":
			if global.Version < 2 {
				continue
			}
			id, err = w.wire.bind(global, 2)
			w.outputs[id] = &wlOutput{global: global.Name, scale: 1}
		case "wp_viewporter":
			if w.viewporter != 0 {
				continue
			}
			id, err = w.wire.bind(global, 1)
			w.viewporter = id
		case "wp_fractional_scale_manager_v1":
			if w.fractionalManager != 0 {
				continue
			}
			id, err = w.wire.bind(global, 1)
			w.fractionalManager = id
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
			id, err = w.wire.bind(global, 4)
			w.seat = id
		case "zwp_text_input_manager_v3":
			if w.textManager != 0 {
				continue
			}
			id, err = w.wire.bind(global, 1)
			w.textManager = id
		case "wl_data_device_manager":
			if w.dataManager != 0 {
				continue
			}
			id, err = w.wire.bind(global, 1)
			w.dataManager = id
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
	if w.dataManager != 0 && w.seat != 0 {
		w.dataDevice = w.wire.id()
		if err := w.wire.send(w.dataManager, 1, append(wlWord(w.dataDevice), wlWord(w.seat)...)); err != nil {
			return err
		}
	}
	if w.textManager != 0 && w.seat != 0 {
		w.textInput = w.wire.id()
		if err := w.wire.send(w.textManager, 1, append(wlWord(w.textInput), wlWord(w.seat)...)); err != nil {
			return err
		}
	}
	w.surface = w.wire.id()
	if err := w.wire.send(w.compositor, 0, wlWord(w.surface)); err != nil {
		return err
	}
	if w.viewporter != 0 && w.fractionalManager != 0 {
		w.viewport = w.wire.id()
		if err := w.wire.send(w.viewporter, 1, append(wlWord(w.viewport), wlWord(w.surface)...)); err != nil {
			return err
		}
		w.fractional = w.wire.id()
		if err := w.wire.send(w.fractionalManager, 1, append(wlWord(w.fractional), wlWord(w.surface)...)); err != nil {
			return err
		}
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
