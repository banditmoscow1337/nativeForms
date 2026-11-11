//go:build linux

package linux

import (
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"strings"
	"unicode/utf8"
)

type dbWriter struct {
	data []byte
	err  error
}

func (w *dbWriter) align(n int) {
	for len(w.data)%n != 0 {
		w.data = append(w.data, 0)
	}
}
func (w *dbWriter) word(v uint32) { w.align(4); w.data = append(w.data, wlWord(v)...) }
func dbAlignment(sig string) int {
	switch sig[0] {
	case 'y', 'g', 'v':
		return 1
	case 'n', 'q':
		return 2
	case 'x', 't', 'd', '(', '{':
		return 8
	default:
		return 4
	}
}
func (w *dbWriter) value(sig string, v any) {
	if w.err != nil {
		return
	}
	if len(sig) == 0 {
		w.err = fmt.Errorf("dbus: empty type")
		return
	}
	w.align(dbAlignment(sig))
	switch sig[0] {
	case 's', 'o':
		s := v.(string)
		if strings.ContainsRune(s, 0) || !utf8.ValidString(s) {
			w.err = fmt.Errorf("dbus: invalid string")
			return
		}
		w.word(uint32(len(s)))
		w.data = append(w.data, s...)
		w.data = append(w.data, 0)
	case 'g':
		s := v.(string)
		if len(s) > 255 {
			w.err = fmt.Errorf("dbus: signature too long")
			return
		}
		w.data = append(w.data, byte(len(s)))
		w.data = append(w.data, s...)
		w.data = append(w.data, 0)
	case 'u':
		w.word(v.(uint32))
	case 'i':
		w.word(uint32(v.(int32)))
	case 'b':
		n := uint32(0)
		if v.(bool) {
			n = 1
		}
		w.word(n)
	case 'y':
		w.data = append(w.data, v.(byte))
	case 'v':
		a := v.(dbValue)
		w.value("g", a.sig)
		w.value(a.sig, a.value)
	case 'a':
		w.word(0)
		at := len(w.data) - 4
		w.align(dbAlignment(sig[1:]))
		start := len(w.data)
		for _, item := range v.([]any) {
			w.value(sig[1:], item)
		}
		le.PutUint32(w.data[at:], uint32(len(w.data)-start))
	case '(', '{':
		types, err := dbTypes(sig[1:len(sig)-1], 0)
		if err != nil {
			w.err = err
			return
		}
		values := v.([]any)
		if len(types) != len(values) {
			w.err = fmt.Errorf("dbus: invalid struct")
			return
		}
		for i, t := range types {
			w.value(t, values[i])
		}
	default:
		w.err = fmt.Errorf("dbus: unsupported outgoing type %q", sig)
	}
}
func dbTypes(sig string, depth int) ([]string, error) {
	if depth > 32 {
		return nil, fmt.Errorf("dbus: signature nesting exceeds 32")
	}
	var types []string
	for len(sig) > 0 {
		n, err := dbTypeLength(sig, depth)
		if err != nil {
			return nil, err
		}
		types = append(types, sig[:n])
		sig = sig[n:]
	}
	return types, nil
}
func dbTypeLength(s string, depth int) (int, error) {
	if len(s) == 0 || depth > 32 {
		return 0, fmt.Errorf("dbus: invalid signature")
	}
	switch s[0] {
	case 'a':
		n, e := dbTypeLength(s[1:], depth+1)
		return n + 1, e
	case '(', '{':
		close := byte(')')
		if s[0] == '{' {
			close = '}'
		}
		i := 1
		for i < len(s) && s[i] != close {
			n, e := dbTypeLength(s[i:], depth+1)
			if e != nil {
				return 0, e
			}
			i += n
		}
		if i >= len(s) {
			return 0, fmt.Errorf("dbus: unterminated signature")
		}
		return i + 1, nil
	default:
		if strings.ContainsRune("ybnqiuxtdhsogv", rune(s[0])) {
			return 1, nil
		}
	}
	return 0, fmt.Errorf("dbus: invalid type")
}

type dbReader struct {
	data  []byte
	pos   int
	order binary.ByteOrder
	err   error
}

func (r *dbReader) align(n int) { r.pos = (r.pos + n - 1) &^ (n - 1) }
func (r *dbReader) take(n int) []byte {
	if r.err != nil {
		return nil
	}
	if n < 0 || r.pos > len(r.data) || n > len(r.data)-r.pos {
		r.err = io.ErrUnexpectedEOF
		return nil
	}
	v := r.data[r.pos : r.pos+n]
	r.pos += n
	return v
}
func (r *dbReader) value(sig string, depth int) any {
	if r.err != nil {
		return nil
	}
	if depth > 32 || len(sig) == 0 {
		r.err = fmt.Errorf("dbus: invalid nested value")
		return nil
	}
	r.align(dbAlignment(sig))
	switch sig[0] {
	case 'y':
		b := r.take(1)
		if b != nil {
			return b[0]
		}
	case 'u', 'i', 'b', 'h':
		b := r.take(4)
		if b != nil {
			v := r.order.Uint32(b)
			if sig[0] == 'i' {
				return int32(v)
			}
			if sig[0] == 'b' {
				if v > 1 {
					r.err = fmt.Errorf("dbus: invalid boolean")
				}
				return v != 0
			}
			return v
		}
	case 'n', 'q':
		b := r.take(2)
		if b != nil {
			return r.order.Uint16(b)
		}
	case 'x', 't', 'd':
		b := r.take(8)
		if b != nil {
			v := r.order.Uint64(b)
			if sig[0] == 'd' {
				return math.Float64frombits(v)
			}
			return v
		}
	case 's', 'o', 'g':
		n := uint32(0)
		if sig[0] == 'g' {
			b := r.take(1)
			if b == nil {
				return nil
			}
			n = uint32(b[0])
		} else {
			b := r.take(4)
			if b == nil {
				return nil
			}
			n = r.order.Uint32(b)
		}
		if n > 16<<20 {
			r.err = fmt.Errorf("dbus: oversized string")
			return nil
		}
		b := r.take(int(n) + 1)
		if b != nil {
			if b[n] != 0 || !utf8.Valid(b[:n]) || strings.ContainsRune(string(b[:n]), 0) {
				r.err = fmt.Errorf("dbus: invalid string")
				return nil
			}
			return string(b[:n])
		}
	case 'v':
		v := r.value("g", depth+1)
		s, ok := v.(string)
		if !ok {
			return nil
		}
		types, e := dbTypes(s, depth+1)
		if e != nil || len(types) != 1 {
			r.err = fmt.Errorf("dbus: invalid variant signature")
			return nil
		}
		return dbValue{s, r.value(s, depth+1)}
	case 'a':
		b := r.take(4)
		if b == nil {
			return nil
		}
		n := uint64(r.order.Uint32(b))
		r.align(dbAlignment(sig[1:]))
		end := uint64(r.pos) + n
		if end > uint64(len(r.data)) {
			r.err = io.ErrUnexpectedEOF
			return nil
		}
		var values []any
		for uint64(r.pos) < end && r.err == nil {
			before := r.pos
			values = append(values, r.value(sig[1:], depth+1))
			if r.pos <= before || len(values) > 1<<20 {
				r.err = fmt.Errorf("dbus: invalid array")
				break
			}
		}
		if uint64(r.pos) != end {
			r.err = fmt.Errorf("dbus: invalid array length")
		}
		return values
	case '(', '{':
		types, e := dbTypes(sig[1:len(sig)-1], depth+1)
		if e != nil {
			r.err = e
			return nil
		}
		var values []any
		for _, t := range types {
			values = append(values, r.value(t, depth+1))
		}
		return values
	}
	if r.err == nil {
		r.err = fmt.Errorf("dbus: invalid value")
	}
	return nil
}
func readDBMessage(reader io.Reader) (dbMessage, error) {
	var m dbMessage
	h := make([]byte, 16)
	if _, err := io.ReadFull(reader, h); err != nil {
		return m, err
	}
	var order binary.ByteOrder = le
	if h[0] == 'B' {
		order = binary.BigEndian
	} else if h[0] != 'l' {
		return m, fmt.Errorf("dbus: invalid byte order")
	}
	if h[3] != 1 || h[1] < 1 || h[1] > 4 {
		return m, fmt.Errorf("dbus: unsupported message")
	}
	n, hn := uint64(order.Uint32(h[4:])), uint64(order.Uint32(h[12:]))
	if n > 16<<20 || hn > 64<<10 {
		return m, fmt.Errorf("dbus: message too large")
	}
	headerSize := (16 + int(hn) + 7) &^ 7
	data := make([]byte, headerSize+int(n))
	copy(data, h)
	if _, err := io.ReadFull(reader, data[16:]); err != nil {
		return m, err
	}
	m.kind, m.serial = h[1], order.Uint32(h[8:])
	r := dbReader{data: data[:16+int(hn)], pos: 16, order: order}
	sig := ""
	for r.pos < len(r.data) && r.err == nil {
		r.align(8)
		key, ok := r.value("y", 0).(byte)
		if !ok {
			break
		}
		v, ok := r.value("v", 0).(dbValue)
		if !ok {
			break
		}
		switch key {
		case 1:
			m.path, _ = v.value.(string)
		case 2:
			m.iface, _ = v.value.(string)
		case 3:
			m.member, _ = v.value.(string)
		case 4:
			m.errName, _ = v.value.(string)
		case 5:
			m.reply, _ = v.value.(uint32)
		case 7:
			m.sender, _ = v.value.(string)
		case 8:
			sig, _ = v.value.(string)
		}
	}
	if r.err != nil {
		return m, r.err
	}
	types, err := dbTypes(sig, 0)
	if err != nil {
		return m, err
	}
	r = dbReader{data: data[headerSize:], order: order}
	for _, t := range types {
		m.args = append(m.args, r.value(t, 0))
	}
	if r.err != nil {
		return m, r.err
	}
	if r.pos != len(r.data) {
		return m, fmt.Errorf("dbus: trailing body data")
	}
	return m, nil
}
