//go:build linux

package linux

import (
	"fmt"
	"net"
	"time"
	"unicode/utf8"

	"github.com/banditmoscow1337/nativeForms/platform"
)

const x11ClipboardLimit = platform.ClipboardTextLimit

type selectionKey struct{ window, property uint32 }
type selectionTransfer struct {
	typ     uint32
	format  byte
	data    []byte
	offset  int
	expires time.Time
}
type xProperty struct {
	typ    uint32
	format byte
	data   []byte
}

func (w *X11Window) SetClipboardText(value string) error {
	if w == nil || w.conn == nil || w.clipboard == 0 {
		return fmt.Errorf("x11: clipboard needs a running window")
	}
	if err := platform.ValidateClipboardText(value); err != nil {
		return err
	}
	request := make([]byte, 16)
	request[0] = 22
	le.PutUint32(request[4:], w.id)
	le.PutUint32(request[8:], w.clipboard)
	if err := w.send(request); err != nil {
		return err
	}
	w.clipboardOwned, w.clipboardText = true, value
	return nil
}

func (w *X11Window) ClipboardText() (string, error) {
	if w == nil || w.conn == nil || w.clipboard == 0 {
		return "", fmt.Errorf("x11: clipboard needs a running window")
	}
	if w.clipboardOwned {
		return w.clipboardText, nil
	}
	data, err := w.convertSelection(w.clipboard, w.utf8Atom, 0)
	if err != nil {
		return "", err
	}
	if !utf8.Valid(data) {
		return "", fmt.Errorf("x11: clipboard is not UTF-8")
	}
	value := string(data)
	if err := platform.ValidateClipboardText(value); err != nil {
		return "", err
	}
	return value, nil
}

func (w *X11Window) convertSelection(selection, target, timestamp uint32) ([]byte, error) {
	if w.readingSelection {
		return nil, fmt.Errorf("x11: a selection transfer is already pending")
	}
	w.readingSelection = true
	defer func() { w.readingSelection = false }()
	request := make([]byte, 24)
	request[0] = 24
	le.PutUint32(request[4:], w.id)
	le.PutUint32(request[8:], selection)
	le.PutUint32(request[12:], target)
	le.PutUint32(request[16:], w.selectionProperty)
	le.PutUint32(request[20:], timestamp)
	if err := w.send(request); err != nil {
		return nil, err
	}
	defer w.deleteProperty(w.id, w.selectionProperty)
	deadline := time.Now().Add(10 * time.Second)
	for {
		packet, err := w.selectionPacket(deadline)
		if err != nil {
			return nil, err
		}
		if packet[0]&127 == 31 && le.Uint32(packet[8:]) == w.id && le.Uint32(packet[12:]) == selection && le.Uint32(packet[16:]) == target {
			property := le.Uint32(packet[20:])
			if property == 0 {
				return nil, nil
			}
			if property != w.selectionProperty {
				return nil, fmt.Errorf("x11: unexpected selection property")
			}
			break
		}
		if err = w.clipboardDispatch(packet); err != nil {
			return nil, err
		}
	}
	p, err := w.readProperty(w.id, w.selectionProperty, true, deadline)
	if err != nil {
		return nil, err
	}
	if p.typ != w.incrAtom {
		if p.typ != target || p.format != 8 {
			return nil, fmt.Errorf("x11: unsupported selection format")
		}
		return p.data, nil
	}
	if p.format != 32 || len(p.data) != 4 || le.Uint32(p.data) > x11ClipboardLimit {
		return nil, fmt.Errorf("x11: invalid INCR header")
	}
	var result []byte
	for {
		packet, err := w.selectionPacket(deadline)
		if err != nil {
			return nil, err
		}
		if packet[0]&127 == 28 && le.Uint32(packet[4:]) == w.id && le.Uint32(packet[8:]) == w.selectionProperty && packet[16] == 0 {
			p, err = w.readProperty(w.id, w.selectionProperty, true, deadline)
			if err != nil {
				return nil, err
			}
			if p.typ != target || p.format != 8 {
				return nil, fmt.Errorf("x11: invalid INCR chunk")
			}
			if len(p.data) == 0 {
				return result, nil
			}
			if len(result)+len(p.data) > x11ClipboardLimit {
				return nil, fmt.Errorf("x11: INCR transfer exceeds 16 MiB")
			}
			result = append(result, p.data...)
		} else if err = w.clipboardDispatch(packet); err != nil {
			return nil, err
		}
	}
}

func (w *X11Window) selectionPacket(deadline time.Time) ([]byte, error) {
	for !w.closeRequested.Load() && time.Now().Before(deadline) {
		p, err := w.clipboardPacket(minTime(deadline, time.Now().Add(50*time.Millisecond)))
		if e, ok := err.(net.Error); ok && e.Timeout() {
			continue
		}
		return p, err
	}
	return nil, fmt.Errorf("x11: selection timed out or window closed")
}
func minTime(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}

func (w *X11Window) clipboardPacket(deadline time.Time) ([]byte, error) {
	if w.packet == nil {
		p, err := w.readEvent(deadline)
		if err != nil {
			return nil, err
		}
		extra := uint64(0)
		if p[0] == 1 || p[0]&127 == 35 {
			extra = uint64(le.Uint32(p[4:])) * 4
		}
		if extra > x11ClipboardLimit+32 {
			return nil, fmt.Errorf("x11: packet exceeds 16 MiB")
		}
		w.packet = make([]byte, 32+int(extra))
		copy(w.packet, p)
		w.packetHave = 32
	}
	_ = w.conn.SetReadDeadline(deadline)
	for w.packetHave < len(w.packet) {
		n, err := w.conn.Read(w.packet[w.packetHave:])
		w.packetHave += n
		if err != nil {
			return nil, err
		}
		if n == 0 {
			return nil, fmt.Errorf("x11: incomplete reply")
		}
	}
	p := w.packet
	w.packet = nil
	w.packetHave = 0
	return p, nil
}

func (w *X11Window) clipboardDispatch(packet []byte) error {
	if packet[0] == 0 {
		if packet[1] == 3 && w.peerRequests[le.Uint16(packet[2:])] {
			delete(w.peerRequests, le.Uint16(packet[2:]))
			return nil
		}
		return fmt.Errorf("x11: protocol error %d, request %d", packet[1], packet[10])
	}
	if packet[0] == 1 {
		seq := le.Uint16(packet[2:])
		if w.pendingReplies[seq] {
			w.replies[seq] = packet
			return nil
		}
		return fmt.Errorf("x11: unexpected reply")
	}
	if w.handle(packet) {
		w.manager.InvalidatePaint()
	}
	return w.clipboardError
}

func (w *X11Window) readProperty(window, property uint32, remove bool, deadline time.Time) (xProperty, error) {
	request := make([]byte, 24)
	request[0] = 20
	if remove {
		request[1] = 1
	}
	le.PutUint32(request[4:], window)
	le.PutUint32(request[8:], property)
	le.PutUint32(request[20:], x11ClipboardLimit/4)
	if err := w.send(request); err != nil {
		return xProperty{}, err
	}
	packet, err := w.waitReply(w.sequence, deadline)
	if err != nil {
		return xProperty{}, err
	}

	format := packet[1]
	n := uint64(le.Uint32(packet[16:])) * uint64(format) / 8
	if (format != 0 && format != 8 && format != 16 && format != 32) || le.Uint32(packet[12:]) != 0 || n > x11ClipboardLimit || n > uint64(len(packet)-32) {
		return xProperty{}, fmt.Errorf("x11: invalid or oversized property")
	}
	return xProperty{le.Uint32(packet[8:]), format, packet[32 : 32+int(n)]}, nil
}

func (w *X11Window) changeProperty(window, property, typ uint32, format byte, data []byte) error {
	request := make([]byte, 24+padded(len(data)))
	request[0], request[16] = 18, format
	le.PutUint32(request[4:], window)
	le.PutUint32(request[8:], property)
	le.PutUint32(request[12:], typ)
	le.PutUint32(request[20:], uint32(len(data))*8/uint32(format))
	copy(request[24:], data)
	return w.sendPeer(request, window)
}
func (w *X11Window) deleteProperty(window, property uint32) error {
	r := make([]byte, 12)
	r[0] = 19
	le.PutUint32(r[4:], window)
	le.PutUint32(r[8:], property)
	return w.sendPeer(r, window)
}
func (w *X11Window) sendPeer(request []byte, window uint32) error {
	err := w.send(request)
	if err == nil && window != w.id {
		if w.peerRequests == nil {
			w.peerRequests = make(map[uint16]bool)
		}
		w.peerRequests[w.sequence] = true
	}
	return err
}

func (w *X11Window) selectionRequest(event []byte) error {
	requestor, target, property := le.Uint32(event[12:]), le.Uint32(event[20:]), le.Uint32(event[24:])
	if property == 0 {
		property = target
	}
	result := uint32(0)
	if le.Uint32(event[16:]) == w.clipboard && w.clipboardOwned {
		var value []byte
		typ, format := target, byte(8)
		switch target {
		case w.targetsAtom:
			format, typ = 32, 4
			value = make([]byte, 16)
			for i, a := range []uint32{w.targetsAtom, w.utf8Atom, w.textAtom, 31} {
				le.PutUint32(value[i*4:], a)
			}
		case w.utf8Atom, w.textAtom:
			typ = w.utf8Atom
			value = []byte(w.clipboardText)
		case 31:
			value = make([]byte, 0, len(w.clipboardText))
			for _, r := range w.clipboardText {
				if r > 255 {
					value = nil
					break
				}
				value = append(value, byte(r))
			}
		}
		if value != nil {
			key := selectionKey{requestor, property}
			if len(value) > w.selectionChunkSize() {
				if w.transfers == nil {
					w.transfers = make(map[selectionKey]*selectionTransfer)
				}
				if len(w.transfers) < 32 {
					selectEvents := make([]byte, 16)
					selectEvents[0] = 2
					le.PutUint32(selectEvents[4:], requestor)
					le.PutUint32(selectEvents[8:], 1<<11)
					le.PutUint32(selectEvents[12:], 1<<22)
					if err := w.sendPeer(selectEvents, requestor); err != nil {
						return err
					}
					n := make([]byte, 4)
					le.PutUint32(n, uint32(len(value)))
					if err := w.changeProperty(requestor, property, w.incrAtom, 32, n); err != nil {
						return err
					}
					w.transfers[key] = &selectionTransfer{typ: typ, format: format, data: value, expires: time.Now().Add(10 * time.Second)}
					result = property
				}
			} else {
				if err := w.changeProperty(requestor, property, typ, format, value); err != nil {
					return err
				}
				result = property
			}
		}
	}
	notify := make([]byte, 44)
	notify[0] = 25
	le.PutUint32(notify[4:], requestor)
	notify[12] = 31
	copy(notify[16:20], event[4:8])
	le.PutUint32(notify[20:], requestor)
	copy(notify[24:28], event[16:20])
	le.PutUint32(notify[28:], target)
	le.PutUint32(notify[32:], result)
	return w.sendPeer(notify, requestor)
}
func (w *X11Window) selectionChunkSize() int { return min((w.maxRequest-24)&^3, 64<<10) }
func (w *X11Window) selectionProgress(event []byte) error {
	if event[16] != 1 {
		return nil
	}
	key := selectionKey{le.Uint32(event[4:]), le.Uint32(event[8:])}
	t := w.transfers[key]
	if t == nil {
		return nil
	}
	end := min(len(t.data), t.offset+w.selectionChunkSize())
	if err := w.changeProperty(key.window, key.property, t.typ, t.format, t.data[t.offset:end]); err != nil {
		return err
	}
	if t.offset == len(t.data) {
		delete(w.transfers, key)
	} else {
		t.offset = end
		t.expires = time.Now().Add(10 * time.Second)
	}
	return nil
}

func (w *X11Window) waitReply(seq uint16, deadline time.Time) ([]byte, error) {
	if w.pendingReplies == nil {
		w.pendingReplies = make(map[uint16]bool)
		w.replies = make(map[uint16][]byte)
	}
	w.pendingReplies[seq] = true
	defer func() { delete(w.pendingReplies, seq); delete(w.replies, seq) }()
	for {
		if p := w.replies[seq]; p != nil {
			return p, nil
		}
		p, err := w.selectionPacket(deadline)
		if err != nil {
			return nil, err
		}
		if le.Uint16(p[2:]) == seq {
			if p[0] == 1 {
				return p, nil
			}
			if p[0] == 0 {
				return nil, fmt.Errorf("x11: request failed (%d)", p[1])
			}
		}
		if err = w.clipboardDispatch(p); err != nil {
			return nil, err
		}
	}
}
