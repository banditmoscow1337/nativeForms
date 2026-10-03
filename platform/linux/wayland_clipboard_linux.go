//go:build linux

package linux

import (
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	waylandClipboardMIME  = "text/plain;charset=utf-8"
	waylandClipboardLimit = 16 << 20
)

func wlReadString(payload []byte) (string, error) {
	if len(payload) < 4 {
		return "", fmt.Errorf("wayland: short string")
	}
	n := int(binary.LittleEndian.Uint32(payload))
	if n < 1 || n > len(payload)-4 || payload[4+n-1] != 0 {
		return "", fmt.Errorf("wayland: invalid string length")
	}
	return string(payload[4 : 4+n-1]), nil
}

func (w *WaylandWindow) dataDeviceEvent(opcode uint16, payload []byte) error {
	switch opcode {
	case 0: // wl_data_device.data_offer
		if len(payload) < 4 {
			return fmt.Errorf("wayland: short data offer")
		}
		id := binary.LittleEndian.Uint32(payload)
		w.dataOffers[id] = make(map[string]bool)
	case 1: // Drag entered this surface; track the offer for disposal.
		if len(payload) >= 20 {
			w.dragOffer = binary.LittleEndian.Uint32(payload[16:])
			if w.dragOffer != 0 && w.Options.OnFilesDropped != nil && w.dataOffers[w.dragOffer]["text/uri-list"] {
				args := append(wlWord(binary.LittleEndian.Uint32(payload)), wlString("text/uri-list")...)
				if err := w.wire.send(w.dragOffer, 0, args); err != nil {
					return err
				}
			}
		}
	case 2: // Drag left.
		if w.receivingDrag {
			return nil
		}
		return w.releaseDragOffer()
	case 4: // Drop.
		if w.dragOffer != 0 && w.Options.OnFilesDropped != nil && w.dataOffers[w.dragOffer]["text/uri-list"] {
			w.receivingDrag = true
			data, err := w.receiveOffer(w.dragOffer, "text/uri-list")
			w.receivingDrag = false
			if err != nil {
				_ = w.releaseDragOffer()
				return err
			}
			paths := waylandDroppedFiles(data)
			if len(paths) != 0 {
				w.Options.OnFilesDropped(paths)
			}
		}
		return w.releaseDragOffer()
	case 5: // wl_data_device.selection
		if len(payload) < 4 {
			return fmt.Errorf("wayland: short selection")
		}
		next := binary.LittleEndian.Uint32(payload)
		if w.selection != 0 && w.selection != next {
			if err := w.wire.send(w.selection, 2, nil); err != nil {
				return err
			}
			delete(w.dataOffers, w.selection)
		}
		w.selection = next
	}
	return nil
}

func (w *WaylandWindow) releaseDragOffer() error {
	if w.dragOffer != 0 && w.dragOffer != w.selection {
		if err := w.wire.send(w.dragOffer, 2, nil); err != nil {
			return err
		}
		delete(w.dataOffers, w.dragOffer)
	}
	w.dragOffer = 0
	return nil
}

func waylandDroppedFiles(data []byte) []string {
	if !utf8.Valid(data) {
		return nil
	}
	var paths []string
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSuffix(line, "\r")
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		u, err := url.Parse(line)
		if err != nil || u.Scheme != "file" || (u.Host != "" && u.Host != "localhost") || u.RawQuery != "" || u.Fragment != "" || !strings.HasPrefix(u.Path, "/") || strings.ContainsRune(u.Path, 0) {
			continue
		}
		paths = append(paths, u.Path)
		if len(paths) >= 4096 {
			break
		}
	}
	return paths
}

func (w *WaylandWindow) dataOfferEvent(id uint32, opcode uint16, payload []byte) error {
	if opcode == 0 { // wl_data_offer.offer(mime_type)
		mime, err := wlReadString(payload)
		if err != nil {
			return err
		}
		w.dataOffers[id][mime] = true
	}
	return nil
}

func (w *WaylandWindow) dataSourceEvent(id uint32, opcode uint16, payload []byte) error {
	switch opcode {
	case 1: // wl_data_source.send(mime_type, fd)
		fd, err := w.wire.takeFD()
		if err != nil {
			return err
		}
		mime, err := wlReadString(payload)
		if err != nil {
			_ = os.NewFile(uintptr(fd), "wayland-clipboard").Close()
			return err
		}
		value := w.dataSources[id]
		go func() {
			file := os.NewFile(uintptr(fd), "wayland-clipboard")
			defer file.Close()
			if mime == waylandClipboardMIME || mime == "text/plain" {
				_, _ = io.Copy(file, strings.NewReader(value))
			}
		}()
	case 2: // Ownership was lost. Keep older sources until they are cancelled.
		if w.ownSource == id {
			w.ownSource = 0
			w.ownedText = ""
		}
		delete(w.dataSources, id)
		return w.wire.send(id, 1, nil)
	}
	return nil
}

// SetClipboardText claims the current seat selection. Call it on the Run
// goroutine in response to keyboard or pointer input, which provides the
// serial required by wl_data_device.set_selection.
func (w *WaylandWindow) SetClipboardText(value string) error {
	if w == nil || w.dataDevice == 0 || w.wire == nil {
		return fmt.Errorf("wayland: clipboard data device unavailable")
	}
	if w.serial == 0 {
		return fmt.Errorf("wayland: clipboard selection needs an input serial")
	}
	if len(value) > waylandClipboardLimit || !utf8.ValidString(value) {
		return fmt.Errorf("wayland: clipboard text is invalid or exceeds 16 MiB")
	}
	id := w.wire.id()
	if err := w.wire.send(w.dataManager, 0, wlWord(id)); err != nil {
		return err
	}
	for _, mime := range []string{waylandClipboardMIME, "text/plain"} {
		if err := w.wire.send(id, 0, wlString(mime)); err != nil {
			return err
		}
	}
	if err := w.wire.send(w.dataDevice, 1, append(wlWord(id), wlWord(w.serial)...)); err != nil {
		return err
	}
	w.dataSources[id] = value
	w.ownSource, w.ownedText = id, value
	return nil
}

// ClipboardText reads UTF-8 from the current compositor selection. An empty
// selection returns an empty string. Call it on the Run goroutine.
func (w *WaylandWindow) ClipboardText() (string, error) {
	if w == nil || w.dataDevice == 0 || w.wire == nil {
		return "", fmt.Errorf("wayland: clipboard data device unavailable")
	}
	if w.ownSource != 0 {
		return w.ownedText, nil
	}
	offer := w.dataOffers[w.selection]
	if offer == nil {
		return "", nil
	}
	mime := waylandClipboardMIME
	if !offer[mime] {
		mime = "text/plain"
		if !offer[mime] {
			return "", nil
		}
	}
	data, err := w.receiveOffer(w.selection, mime)
	if err != nil {
		return "", err
	}
	if !utf8.Valid(data) {
		return "", fmt.Errorf("wayland: clipboard text is not UTF-8")
	}
	return string(data), nil
}

func (w *WaylandWindow) receiveOffer(offer uint32, mime string) ([]byte, error) {
	reader, writer, err := os.Pipe()
	if err != nil {
		return nil, err
	}
	if err = w.wire.sendFD(offer, 1, wlString(mime), int(writer.Fd())); err != nil {
		reader.Close()
		writer.Close()
		return nil, err
	}
	_ = writer.Close()
	type received struct {
		data []byte
		err  error
	}
	done := make(chan received, 1)
	go func() {
		data, readErr := io.ReadAll(io.LimitReader(reader, waylandClipboardLimit+1))
		done <- received{data, readErr}
	}()
	defer reader.Close()
	deadline := time.Now().Add(2 * time.Second)
	for !w.closing.Load() && time.Now().Before(deadline) {
		select {
		case result := <-done:
			if result.err != nil {
				return nil, result.err
			}
			if len(result.data) > waylandClipboardLimit {
				return nil, fmt.Errorf("wayland: transfer exceeds 16 MiB")
			}
			return result.data, nil
		default:
		}
		object, opcode, payload, readErr := w.wire.read(time.Now().Add(20 * time.Millisecond))
		if timeout, ok := readErr.(net.Error); ok && timeout.Timeout() {
			continue
		}
		if readErr != nil {
			return nil, readErr
		}
		if err = w.handle(object, opcode, payload); err != nil {
			return nil, err
		}
	}
	return nil, fmt.Errorf("wayland: transfer timed out or window closed")
}
