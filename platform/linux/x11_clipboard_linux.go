//go:build linux

package linux

import (
	"fmt"
	"io"
	"time"
	"unicode/utf8"
)

const x11ClipboardLimit = 16 << 20

// SetClipboardText claims CLIPBOARD on this window. The window must keep
// running to answer conversion requests from other applications.
func (w *X11Window) SetClipboardText(value string) error {
	if w == nil || w.conn == nil || w.clipboard == 0 {
		return fmt.Errorf("x11: clipboard needs a running window")
	}
	if !utf8.ValidString(value) || len(value) > w.maxRequest-24 {
		return fmt.Errorf("x11: clipboard text is invalid or exceeds one server request")
	}
	request := make([]byte, 16)
	request[0] = 22 // SetSelectionOwner
	le.PutUint32(request[4:], w.id)
	le.PutUint32(request[8:], w.clipboard)
	if err := w.send(request); err != nil {
		return err
	}
	w.clipboardOwned, w.clipboardText = true, value
	return nil
}

// ClipboardText requests the current CLIPBOARD as UTF8_STRING. The transfer
// is synchronous on the UI goroutine and pumps selection requests while it
// waits; an empty clipboard returns an empty string.
func (w *X11Window) ClipboardText() (string, error) {
	if w == nil || w.conn == nil || w.clipboard == 0 {
		return "", fmt.Errorf("x11: clipboard needs a running window")
	}
	if w.clipboardOwned {
		return w.clipboardText, nil
	}
	request := make([]byte, 24)
	request[0] = 24 // ConvertSelection
	le.PutUint32(request[4:], w.id)
	le.PutUint32(request[8:], w.clipboard)
	le.PutUint32(request[12:], w.utf8Atom)
	le.PutUint32(request[16:], w.selectionProperty)
	if err := w.send(request); err != nil {
		return "", err
	}
	deadline := time.Now().Add(2 * time.Second)
	for !w.closeRequested.Load() && time.Now().Before(deadline) {
		packet, err := w.clipboardPacket(deadline)
		if err != nil {
			return "", err
		}
		if packet[0]&0x7f == 31 && le.Uint32(packet[8:]) == w.id &&
			le.Uint32(packet[12:]) == w.clipboard {
			if le.Uint32(packet[20:]) == 0 {
				return "", nil
			}
			if le.Uint32(packet[20:]) != w.selectionProperty {
				return "", fmt.Errorf("x11: unexpected clipboard property")
			}
			return w.readClipboardProperty(deadline)
		}
		if err := w.clipboardDispatch(packet); err != nil {
			return "", err
		}
	}
	return "", fmt.Errorf("x11: clipboard transfer timed out or window closed")
}

func (w *X11Window) clipboardPacket(deadline time.Time) ([]byte, error) {
	packet, err := w.readEvent(deadline)
	if err != nil {
		return nil, err
	}
	if packet[0] != 1 {
		return packet, nil
	}
	extra := uint64(le.Uint32(packet[4:])) * 4
	if extra > x11ClipboardLimit+32 {
		w.clipboardError = fmt.Errorf("x11: clipboard reply exceeds 16 MiB")
		return nil, w.clipboardError
	}
	data := make([]byte, 32+int(extra))
	copy(data, packet)
	_ = w.conn.SetReadDeadline(deadline)
	if _, err := io.ReadFull(w.conn, data[32:]); err != nil {
		// A partial reply cannot be resynchronized with the event stream.
		w.clipboardError = fmt.Errorf("x11: incomplete clipboard reply: %w", err)
		return nil, w.clipboardError
	}
	return data, nil
}

func (w *X11Window) clipboardDispatch(packet []byte) error {
	if packet[0] == 0 {
		return fmt.Errorf("x11: clipboard protocol error %d", packet[1])
	}
	if packet[0] == 1 {
		return fmt.Errorf("x11: unexpected clipboard reply")
	}
	if w.handle(packet) {
		w.manager.InvalidatePaint()
	}
	return w.clipboardError
}

func (w *X11Window) readClipboardProperty(deadline time.Time) (string, error) {
	request := make([]byte, 24)
	request[0], request[1] = 20, 1 // GetProperty, delete after reading.
	le.PutUint32(request[4:], w.id)
	le.PutUint32(request[8:], w.selectionProperty)
	le.PutUint32(request[20:], x11ClipboardLimit/4)
	if err := w.send(request); err != nil {
		return "", err
	}
	sequence := w.sequence
	for !w.closeRequested.Load() && time.Now().Before(deadline) {
		packet, err := w.clipboardPacket(deadline)
		if err != nil {
			return "", err
		}
		if packet[0] != 1 {
			if err := w.clipboardDispatch(packet); err != nil {
				return "", err
			}
			continue
		}
		if le.Uint16(packet[2:]) != sequence {
			return "", fmt.Errorf("x11: unexpected GetProperty reply sequence")
		}
		if packet[1] != 8 || le.Uint32(packet[8:]) != w.utf8Atom || le.Uint32(packet[12:]) != 0 {
			return "", fmt.Errorf("x11: unsupported clipboard property format or INCR transfer")
		}
		n := uint64(le.Uint32(packet[16:]))
		if n > x11ClipboardLimit || n > uint64(len(packet)-32) {
			return "", fmt.Errorf("x11: invalid clipboard property size")
		}
		value := packet[32 : 32+int(n)]
		if !utf8.Valid(value) {
			return "", fmt.Errorf("x11: clipboard is not UTF-8")
		}
		return string(value), nil
	}
	return "", fmt.Errorf("x11: clipboard property timed out or window closed")
}

func (w *X11Window) selectionRequest(event []byte) error {
	requestor := le.Uint32(event[12:])
	target := le.Uint32(event[20:])
	property := le.Uint32(event[24:])
	if property == 0 {
		property = target
	}
	resultProperty := uint32(0)
	if le.Uint32(event[16:]) == w.clipboard && w.clipboardOwned {
		var value []byte
		typ, format := target, byte(8)
		switch target {
		case w.targetsAtom:
			format, typ = 32, 4 // ATOM
			value = make([]byte, 16)
			for i, atom := range []uint32{w.targetsAtom, w.utf8Atom, w.textAtom, 31} {
				le.PutUint32(value[i*4:], atom)
			}
		case w.utf8Atom, w.textAtom:
			typ = w.utf8Atom
			value = []byte(w.clipboardText)
		case 31: // STRING, Latin-1 only.
			value = make([]byte, 0, len(w.clipboardText))
			for _, r := range w.clipboardText {
				if r > 255 {
					value = nil
					break
				}
				value = append(value, byte(r))
			}
		default:
			value = nil
		}
		if value != nil && len(value) <= w.maxRequest-24 {
			change := make([]byte, 24+padded(len(value)))
			change[0], change[16] = 18, format // ChangeProperty, Replace.
			le.PutUint32(change[4:], requestor)
			le.PutUint32(change[8:], property)
			le.PutUint32(change[12:], typ)
			le.PutUint32(change[20:], uint32(len(value))*8/uint32(format))
			copy(change[24:], value)
			if err := w.send(change); err != nil {
				return err
			}
			resultProperty = property
		}
	}
	notify := make([]byte, 44)
	notify[0] = 25 // SendEvent, no propagation, no event mask.
	le.PutUint32(notify[4:], requestor)
	notify[12] = 31                 // SelectionNotify
	copy(notify[16:20], event[4:8]) // Time from SelectionRequest.
	le.PutUint32(notify[20:], requestor)
	copy(notify[24:28], event[16:20]) // Selection from SelectionRequest.
	le.PutUint32(notify[28:], target)
	le.PutUint32(notify[32:], resultProperty)
	return w.send(notify)
}
