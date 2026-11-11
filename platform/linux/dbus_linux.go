//go:build linux

package linux

import (
	"bufio"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

type dbValue struct {
	sig   string
	value any
}
type dbMessage struct {
	kind                                 byte
	serial, reply                        uint32
	path, iface, member, sender, errName string
	args                                 []any
}
type dbIncoming struct {
	message dbMessage
	err     error
}
type dbConn struct {
	conn     net.Conn
	incoming chan dbIncoming
	done     chan struct{}
	serial   uint32
	name     string
	signal   func(dbMessage)
}

func sessionBusAddress() string {
	if a := os.Getenv("DBUS_SESSION_BUS_ADDRESS"); a != "" {
		return a
	}
	if d := os.Getenv("XDG_RUNTIME_DIR"); d != "" {
		return "unix:path=" + d + "/bus"
	}
	return "unix:path=/run/user/" + strconv.Itoa(os.Getuid()) + "/bus"
}
func openDBus(address string) (*dbConn, error) {
	var conn net.Conn
	var err error
	for _, candidate := range strings.Split(address, ";") {
		if !strings.HasPrefix(candidate, "unix:") {
			continue
		}
		for _, option := range strings.Split(strings.TrimPrefix(candidate, "unix:"), ",") {
			pair := strings.SplitN(option, "=", 2)
			if len(pair) != 2 {
				continue
			}
			if pair[0] != "path" && pair[0] != "abstract" {
				continue
			}
			path, e := url.PathUnescape(pair[1])
			if e != nil {
				continue
			}
			if pair[0] == "abstract" {
				path = "\x00" + path
			}
			conn, err = net.DialTimeout("unix", path, time.Second)
			if err == nil {
				break
			}
		}
		if conn != nil {
			break
		}
	}
	if conn == nil {
		return nil, fmt.Errorf("dbus: no accessible local bus: %v", err)
	}
	ok := false
	defer func() {
		if !ok {
			conn.Close()
		}
	}()
	_ = conn.SetDeadline(time.Now().Add(2 * time.Second))
	auth := "\x00AUTH EXTERNAL " + hex.EncodeToString([]byte(strconv.Itoa(os.Getuid()))) + "\r\n"
	if _, err = io.WriteString(conn, auth); err != nil {
		return nil, err
	}
	reader := bufio.NewReaderSize(conn, 4096)
	line, err := reader.ReadSlice('\n')
	if err != nil || !strings.HasPrefix(string(line), "OK ") {
		return nil, fmt.Errorf("dbus: EXTERNAL authentication failed")
	}
	if _, err = io.WriteString(conn, "BEGIN\r\n"); err != nil {
		return nil, err
	}
	_ = conn.SetDeadline(time.Time{})
	b := &dbConn{conn: conn, incoming: make(chan dbIncoming, 64), done: make(chan struct{})}
	ok = true
	go func() {
		for {
			m, e := readDBMessage(reader)
			select {
			case b.incoming <- dbIncoming{m, e}:
			case <-b.done:
				return
			}
			if e != nil {
				return
			}
		}
	}()
	result, err := b.call("org.freedesktop.DBus", "/org/freedesktop/DBus", "org.freedesktop.DBus", "Hello", nil, nil, 2*time.Second)
	if err != nil {
		b.close()
		return nil, err
	}
	if len(result) != 1 {
		b.close()
		return nil, fmt.Errorf("dbus: invalid Hello response")
	}
	b.name, _ = result[0].(string)
	if b.name == "" {
		b.close()
		return nil, fmt.Errorf("dbus: missing unique name")
	}
	return b, nil
}
func (b *dbConn) close() {
	if b != nil {
		select {
		case <-b.done:
			return
		default:
			close(b.done)
			b.conn.Close()
		}
	}
}
func (b *dbConn) send(dest, path, iface, method string, args []dbValue, noReply bool) (uint32, error) {
	body := dbWriter{}
	sig := ""
	for _, a := range args {
		sig += a.sig
		body.value(a.sig, a.value)
	}
	if body.err != nil {
		return 0, body.err
	}
	header := dbWriter{data: make([]byte, 16)}
	fields := []struct {
		key   byte
		value dbValue
	}{{1, dbValue{"o", path}}, {2, dbValue{"s", iface}}, {3, dbValue{"s", method}}, {6, dbValue{"s", dest}}}
	if sig != "" {
		fields = append(fields, struct {
			key   byte
			value dbValue
		}{8, dbValue{"g", sig}})
	}
	for _, f := range fields {
		header.align(8)
		header.data = append(header.data, f.key)
		header.value("v", f.value)
	}
	if header.err != nil {
		return 0, header.err
	}
	fieldsLen := len(header.data) - 16
	header.align(8)
	b.serial++
	if b.serial == 0 {
		b.serial++
	}
	h := header.data
	h[0], h[1], h[3] = 'l', 1, 1
	if noReply {
		h[2] = 1
	}
	le.PutUint32(h[4:], uint32(len(body.data)))
	le.PutUint32(h[8:], b.serial)
	le.PutUint32(h[12:], uint32(fieldsLen))
	packet := append(h, body.data...)
	_ = b.conn.SetWriteDeadline(time.Now().Add(2 * time.Second))
	for len(packet) > 0 {
		n, err := b.conn.Write(packet)
		if err != nil {
			return 0, err
		}
		if n == 0 {
			return 0, io.ErrShortWrite
		}
		packet = packet[n:]
	}
	return b.serial, nil
}
func (b *dbConn) call(dest, path, iface, method string, args []dbValue, pump func() error, timeout time.Duration) ([]any, error) {
	serial, err := b.send(dest, path, iface, method, args, false)
	if err != nil {
		return nil, err
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	tick := time.NewTicker(10 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case in := <-b.incoming:
			if in.err != nil {
				return nil, in.err
			}
			m := in.message
			if m.reply == serial {
				if m.kind == 3 {
					return nil, fmt.Errorf("dbus: %s: %v", m.errName, m.args)
				}
				if m.kind != 2 {
					return nil, fmt.Errorf("dbus: invalid method reply")
				}
				return m.args, nil
			}
			if m.kind == 4 && b.signal != nil {
				b.signal(m)
			}
		case <-tick.C:
			if pump != nil {
				if err = pump(); err != nil {
					return nil, err
				}
			}
		case <-timer.C:
			return nil, fmt.Errorf("dbus: %s timed out", method)
		case <-b.done:
			return nil, fmt.Errorf("dbus: closed")
		}
	}
}
func (b *dbConn) poll() error {
	for i := 0; i < 64; i++ {
		select {
		case in := <-b.incoming:
			if in.err != nil {
				return in.err
			}
			if in.message.kind == 4 && b.signal != nil {
				b.signal(in.message)
			}
		default:
			return nil
		}
	}
	return nil
}
func (b *dbConn) match(rule string) error {
	_, err := b.call("org.freedesktop.DBus", "/org/freedesktop/DBus", "org.freedesktop.DBus", "AddMatch", []dbValue{{"s", rule}}, nil, 2*time.Second)
	return err
}


func dbDict(values map[string]dbValue) dbValue {
	entries := make([]any, 0, len(values))
	for k, v := range values {
		entries = append(entries, []any{k, v})
	}
	return dbValue{"a{sv}", entries}
}
func dbLookup(value any, key string) (dbValue, bool) {
	entries, ok := value.([]any)
	if !ok {
		return dbValue{}, false
	}
	for _, e := range entries {
		pair, ok := e.([]any)
		if ok && len(pair) == 2 && pair[0] == key {
			v, ok := pair[1].(dbValue)
			return v, ok
		}
	}
	return dbValue{}, false
}
