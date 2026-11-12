//go:build linux

package linux

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/banditmoscow1337/nativeForms/platform"
)

var le = binary.LittleEndian

func x11Display() (string, int, string, error) {
	display := os.Getenv("DISPLAY")
	if !strings.HasPrefix(display, ":") && !strings.HasPrefix(display, "unix:") {
		return "", 0, "", fmt.Errorf("x11: only local Unix DISPLAY is supported: %q", display)
	}
	display = strings.TrimPrefix(display, "unix:")
	display = strings.TrimPrefix(display, ":")
	parts := strings.SplitN(display, ".", 2)
	n, err := strconv.Atoi(parts[0])
	if err != nil || n < 0 || n > 65535 {
		return "", 0, "", fmt.Errorf("x11: invalid DISPLAY")
	}
	screen := 0
	if len(parts) == 2 {
		screen, err = strconv.Atoi(parts[1])
		if err != nil || screen < 0 {
			return "", 0, "", fmt.Errorf("x11: invalid screen")
		}
	}
	return fmt.Sprintf("/tmp/.X11-unix/X%d", n), screen, parts[0], nil
}

func x11Authority(display string) ([]byte, []byte) {
	path := os.Getenv("XAUTHORITY")
	if path == "" {
		if home, err := os.UserHomeDir(); err == nil {
			path = filepath.Join(home, ".Xauthority")
		}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil
	}

	for len(data) >= 2 {
		family := binary.BigEndian.Uint16(data[:2])
		data = data[2:]
		fields := make([][]byte, 4)
		valid := true
		for i := range fields {
			if len(data) < 2 {
				valid = false
				break
			}
			n := int(binary.BigEndian.Uint16(data[:2]))
			data = data[2:]
			if len(data) < n {
				valid = false
				break
			}
			fields[i] = data[:n]
			data = data[n:]
		}
		if !valid {
			break
		}
		if (family == 256 || family == 65535) && string(fields[1]) == display && string(fields[2]) == "MIT-MAGIC-COOKIE-1" {
			return append([]byte(nil), fields[2]...), append([]byte(nil), fields[3]...)
		}
	}
	return nil, nil
}

func padded(n int) int { return (n + 3) &^ 3 }

func (w *X11Window) open(screen int, display string) error {
	name, cookie := x11Authority(display)
	setup := make([]byte, 12+padded(len(name))+padded(len(cookie)))
	setup[0] = 'l'
	le.PutUint16(setup[2:], 11)
	le.PutUint16(setup[6:], uint16(len(name)))
	le.PutUint16(setup[8:], uint16(len(cookie)))
	copy(setup[12:], name)
	copy(setup[12+padded(len(name)):], cookie)
	if _, err := w.conn.Write(setup); err != nil {
		return err
	}
	header := make([]byte, 8)
	if _, err := io.ReadFull(w.conn, header); err != nil {
		return err
	}
	length := int(le.Uint16(header[6:])) * 4
	if length > 16<<20 {
		return fmt.Errorf("x11: setup response too large")
	}
	data := make([]byte, length)
	if _, err := io.ReadFull(w.conn, data); err != nil {
		return err
	}
	if header[0] != 1 {
		return fmt.Errorf("x11: connection rejected: %s", string(data[:min(int(header[1]), len(data))]))
	}
	if len(data) < 32 {
		return fmt.Errorf("x11: short server setup")
	}
	w.base = le.Uint32(data[4:])
	w.mask = le.Uint32(data[8:])
	w.minKey, w.maxKey = data[26], data[27]
	if w.maxKey < w.minKey || w.minKey < 8 {
		return fmt.Errorf("x11: invalid keycode range")
	}
	w.maxRequest = int(le.Uint16(data[18:])) * 4
	if w.maxRequest < 1024 {
		return fmt.Errorf("x11: request size too small")
	}
	if data[22] != 0 {
		return fmt.Errorf("x11: MSB image order is unsupported")
	}
	rootCount, formatCount := int(data[20]), int(data[21])
	if screen >= rootCount {
		return fmt.Errorf("x11: screen %d is unavailable", screen)
	}
	offset := 32 + padded(int(le.Uint16(data[16:])))
	if offset+formatCount*8 > len(data) {
		return fmt.Errorf("x11: truncated pixmap formats")
	}
	formats := data[offset : offset+formatCount*8]
	offset += formatCount * 8
	for s := 0; s < rootCount; s++ {
		if offset+40 > len(data) {
			return fmt.Errorf("x11: truncated screen")
		}
		root := data[offset:]
		depth := root[38]
		visual := le.Uint32(root[32:])
		depths := int(root[39])
		depthOffset := offset + 40
		validVisual := false
		for j := 0; j < depths; j++ {
			if depthOffset+8 > len(data) {
				return fmt.Errorf("x11: truncated depth")
			}
			vcount := int(le.Uint16(data[depthOffset+2:]))
			depthOffset += 8
			if depthOffset+vcount*24 > len(data) {
				return fmt.Errorf("x11: truncated visual")
			}
			for k := 0; k < vcount; k++ {
				v := data[depthOffset+k*24:]
				if le.Uint32(v) == visual && v[4] == 4 && le.Uint32(v[8:]) == 0xff0000 &&
					le.Uint32(v[12:]) == 0x00ff00 && le.Uint32(v[16:]) == 0x0000ff {
					validVisual = true
				}
			}
			depthOffset += vcount * 24
		}
		if s == screen {
			formatOK := false
			for i := 0; i < formatCount; i++ {
				f := formats[i*8:]
				if f[0] == depth && f[1] == 32 && f[2] == 32 {
					formatOK = true
				}
			}
			if !formatOK || !validVisual {
				return fmt.Errorf("x11: a 32 bpp TrueColor RGB visual is required")
			}
			w.root = le.Uint32(root)
			w.depth = depth
			break
		}
		offset = depthOffset
	}
	return nil
}

func (w *X11Window) resource() (uint32, error) {
	step := w.mask & -w.mask
	w.nextID += step
	if w.nextID == 0 || w.nextID&^w.mask != 0 {
		return 0, fmt.Errorf("x11: resource ID space exhausted")
	}
	return w.base | w.nextID, nil
}

func (w *X11Window) send(req []byte) error {
	if len(req) < 4 || len(req)%4 != 0 || len(req) > w.maxRequest {
		return fmt.Errorf("x11: invalid request length")
	}
	le.PutUint16(req[2:], uint16(len(req)/4))
	w.sequence++
	delete(w.peerRequests, w.sequence)
	for len(req) > 0 {
		n, err := w.conn.Write(req)
		if err != nil {
			return err
		}
		if n == 0 {
			return io.ErrShortWrite
		}
		req = req[n:]
	}
	return nil
}

func (w *X11Window) reply() ([]byte, error) {
	for {
		header := make([]byte, 32)
		if _, err := io.ReadFull(w.conn, header); err != nil {
			return nil, err
		}
		if header[0] == 0 {
			return nil, fmt.Errorf("x11: request %d failed with code %d", le.Uint16(header[2:]), header[1])
		}
		if header[0] == 1 {
			extra := int(le.Uint32(header[4:])) * 4
			if extra > 16<<20 {
				return nil, fmt.Errorf("x11: reply too large")
			}
			data := make([]byte, 32+extra)
			copy(data, header)
			_, err := io.ReadFull(w.conn, data[32:])
			return data, err
		}
	}
}

func (w *X11Window) atom(name string) (uint32, error) {
	request := make([]byte, 8+padded(len(name)))
	request[0] = 16
	le.PutUint16(request[4:], uint16(len(name)))
	copy(request[8:], name)
	if err := w.send(request); err != nil {
		return 0, err
	}
	reply, err := w.reply()
	if err != nil {
		return 0, err
	}
	return le.Uint32(reply[8:]), nil
}

func (w *X11Window) createWindow() error {
	var err error
	w.id, err = w.resource()
	if err != nil {
		return err
	}
	w.gc, err = w.resource()
	if err != nil {
		return err
	}
	w.width, w.height = w.options.Width, w.options.Height
	if w.width <= 0 {
		w.width = 800
	}
	if w.height <= 0 {
		w.height = 600
	}
	w.width = platform.PixelSize(w.width, w.scale)
	w.height = platform.PixelSize(w.height, w.scale)
	if w.width > 16384 || w.height > 16384 || int64(w.width)*int64(w.height) > 64*1024*1024 {
		return fmt.Errorf("x11: window exceeds framebuffer limit")
	}
	request := make([]byte, 36)
	request[0] = 1
	le.PutUint32(request[4:], w.id)
	le.PutUint32(request[8:], w.root)
	le.PutUint16(request[16:], uint16(w.width))
	le.PutUint16(request[18:], uint16(w.height))
	le.PutUint16(request[22:], 1)
	le.PutUint32(request[28:], 1<<11)
	le.PutUint32(request[32:], 1|2|4|8|32|64|1<<15|1<<17|1<<21|1<<22)
	if err = w.send(request); err != nil {
		return err
	}
	gc := make([]byte, 16)
	gc[0] = 55
	le.PutUint32(gc[4:], w.gc)
	le.PutUint32(gc[8:], w.id)
	if err = w.send(gc); err != nil {
		return err
	}
	protocols, err := w.atom("WM_PROTOCOLS")
	if err != nil {
		return err
	}
	w.wmProtocols = protocols
	w.wmDelete, err = w.atom("WM_DELETE_WINDOW")
	if err != nil {
		return err
	}
	for _, entry := range []struct {
		name string
		dst  *uint32
	}{
		{"INCR", &w.incrAtom}, {"CLIPBOARD", &w.clipboard}, {"UTF8_STRING", &w.utf8Atom},
		{"TARGETS", &w.targetsAtom}, {"TEXT", &w.textAtom},
		{"_NATIVEFORMS_CLIPBOARD", &w.selectionProperty},
		{"_MOTIF_WM_HINTS", &w.motifHintsAtom},
	} {
		*entry.dst, err = w.atom(entry.name)
		if err != nil {
			return err
		}
	}
	prop := make([]byte, 28)
	prop[0] = 18
	prop[1] = 0
	le.PutUint32(prop[4:], w.id)
	le.PutUint32(prop[8:], protocols)
	le.PutUint32(prop[12:], 4) // ATOM
	prop[16] = 32
	le.PutUint32(prop[20:], 1)
	le.PutUint32(prop[24:], w.wmDelete)
	if err = w.send(prop); err != nil {
		return err
	}
	if w.options.Frameless || w.options.HideWindowControls {
		w.applyMotifHints()
	}
	title := w.options.Title
	if title == "" {
		title = "nativeForms"
	}
	if len(title) > 1024 {
		title = title[:1024]
	}
	name := make([]byte, 24+padded(len(title)))
	name[0] = 18
	le.PutUint32(name[4:], w.id)
	le.PutUint32(name[8:], 39)
	le.PutUint32(name[12:], 31) // WM_NAME, STRING
	name[16] = 8
	le.PutUint32(name[20:], uint32(len(title)))
	copy(name[24:], title)
	if err = w.send(name); err != nil {
		return err
	}
	if err = w.readKeyboardMapping(); err != nil {
		return err
	}
	if err = w.setupDND(); err != nil {
		return err
	}
	mapWindow := make([]byte, 8)
	mapWindow[0] = 8
	le.PutUint32(mapWindow[4:], w.id)
	return w.send(mapWindow)
}

func (w *X11Window) readKeyboardMapping() error {
	request := make([]byte, 8)
	request[0] = 101
	request[4] = w.minKey
	request[5] = w.maxKey - w.minKey + 1
	if err := w.send(request); err != nil {
		return err
	}
	var answer []byte
	var err error
	if w.running {
		answer, err = w.waitReply(w.sequence, time.Now().Add(2*time.Second))
	} else {
		answer, err = w.reply()
	}
	if err != nil {
		return err
	}
	per := int(answer[1])
	count := int(request[5])
	if per == 0 || len(answer) < 32+count*per*4 {
		return fmt.Errorf("x11: invalid keyboard mapping")
	}
	w.keysyms = make(map[byte][]uint32, count)
	for k := 0; k < count; k++ {
		values := make([]uint32, per)
		for j := range values {
			values[j] = le.Uint32(answer[32+(k*per+j)*4:])
		}
		w.keysyms[byte(k)+w.minKey] = values
	}
	return nil
}

func (w *X11Window) readEvent(deadline time.Time) ([]byte, error) {
	_ = w.conn.SetReadDeadline(deadline)
	for w.have < len(w.incoming) {
		n, err := w.conn.Read(w.incoming[w.have:])
		w.have += n
		if err != nil {
			return nil, err
		}
		if n == 0 {
			return nil, io.ErrUnexpectedEOF
		}
	}
	packet := append([]byte(nil), w.incoming[:]...)
	w.have = 0
	return packet, nil
}

func (w *X11Window) refreshOrigin() error {
	w.originDirty = false
	request := make([]byte, 16)
	request[0] = 40 // TranslateCoordinates, from the client window to the root.
	le.PutUint32(request[4:], w.id)
	le.PutUint32(request[8:], w.root)
	if err := w.send(request); err != nil {
		return err
	}
	reply, err := w.waitReply(w.sequence, time.Now().Add(time.Second))
	if err != nil {
		return err
	}
	if reply[1] != 0 {
		w.screenX = int(int16(le.Uint16(reply[12:])))
		w.screenY = int(int16(le.Uint16(reply[14:])))
	}
	return nil
}
