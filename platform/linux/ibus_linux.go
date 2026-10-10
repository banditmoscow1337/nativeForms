//go:build linux

package linux

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unicode/utf8"

	ui "github.com/banditmoscow1337/nativeForms"
)

const ibusName = "org.freedesktop.IBus"
const ibusContext = "org.freedesktop.IBus.InputContext"

type linuxIME struct {
	bus                                 *dbConn
	path                                string
	manager                             *ui.Manager
	state                               ui.TextInputState
	focused, delivered, needSurrounding bool
	failed                              error
	screenX, screenY                    int
	preedit                             string
}

func openLinuxIME(m *ui.Manager) *linuxIME {
	address := ibusAddress()
	b, err := openDBus(address)
	if err != nil {
		return nil
	}
	result, err := b.call(ibusName, "/org/freedesktop/IBus", ibusName, "CreateInputContext", []dbValue{{"s", "nativeForms"}}, nil, time.Second)
	if err != nil || len(result) != 1 {
		b.close()
		return nil
	}
	path, ok := result[0].(string)
	if !ok || !strings.HasPrefix(path, "/") {
		b.close()
		return nil
	}
	i := &linuxIME{bus: b, path: path, manager: m}
	b.signal = i.signal
	if err = b.match("type='signal',sender='org.freedesktop.IBus',interface='org.freedesktop.IBus.InputContext',path='" + path + "'"); err != nil {
		b.close()
		return nil
	}
	_, err = b.call(ibusName, path, ibusContext, "SetCapabilities", []dbValue{{"u", uint32(1 | 8 | 32 | 128)}}, nil, time.Second)
	if err != nil {
		b.close()
		return nil
	}
	return i
}
func (i *linuxIME) close() {
	if i == nil {
		return
	}
	i.focusOut()
	_, _ = i.bus.send(ibusName, i.path, "org.freedesktop.IBus.Service", "Destroy", nil, true)
	i.bus.close()
}
func (i *linuxIME) send(method string, args ...dbValue) error {
	_, err := i.bus.send(ibusName, i.path, ibusContext, method, args, true)
	return err
}
func (i *linuxIME) focusOut() {
	if i.focused {
		_ = i.send("Reset")
		_ = i.send("FocusOut")
		i.manager.HandleEvent(ui.Event{Type: ui.CompositionEnd})
	}
	i.focused = false
	i.state = ui.TextInputState{}
	i.preedit = ""
}
func (i *linuxIME) sync() error {
	if i.failed != nil {
		return i.failed
	}
	if err := i.bus.poll(); err != nil {
		return err
	}
	s, ok := i.manager.TextInputState()
	ok = ok && !s.Password
	if i.focused && (!ok || s.Identity != i.state.Identity) {
		i.focusOut()
	}
	if !ok {
		return nil
	}
	if !i.focused {
		if err := i.send("FocusIn"); err != nil {
			return err
		}
		i.focused = true
	}
	if s != i.state || i.needSurrounding {
		i.state = s
		i.needSurrounding = false
		r := s.Caret
		if err := i.send("SetCursorLocation", dbValue{"i", int32(r.X) + int32(i.screenX)}, dbValue{"i", int32(r.Y) + int32(i.screenY)}, dbValue{"i", int32(max(float32(1), r.W))}, dbValue{"i", int32(max(float32(1), r.H))}); err != nil {
			return err
		}

		text, c, a, valid := surroundingText(s, 16000)
		if valid {
			attrs := dbValue{"(sa{sv}av)", []any{"IBusAttrList", []any{}, []any{}}}
			serialized := dbValue{"(sa{sv}sv)", []any{"IBusText", []any{}, text, attrs}}
			if err := i.send("SetSurroundingText", dbValue{"v", serialized}, dbValue{"u", uint32(utf8.RuneCountInString(text[:c]))}, dbValue{"u", uint32(utf8.RuneCountInString(text[:a]))}); err != nil {
				return err
			}
		}
	}
	return nil
}
func (i *linuxIME) key(sym, code, state uint32, down bool) bool {
	if i.failed != nil {
		return false
	}
	if err := i.sync(); err != nil {
		i.failed = err
		return false
	}
	if !i.focused {
		return false
	}
	if !down {
		state |= 1 << 30
	}
	i.delivered = false
	result, err := i.bus.call(ibusName, i.path, ibusContext, "ProcessKeyEvent", []dbValue{{"u", sym}, {"u", code}, {"u", state}}, nil, time.Second)
	if err != nil {
		i.failed = err
		return i.delivered
	}
	if len(result) != 1 {
		i.failed = fmt.Errorf("ibus: invalid key response")
		return i.delivered
	}
	handled, _ := result[0].(bool)
	return handled || i.delivered
}
func ibusText(v any) string {
	variant, ok := v.(dbValue)
	if !ok {
		return ""
	}
	fields, ok := variant.value.([]any)
	if !ok || len(fields) < 3 || fields[0] != "IBusText" {
		return ""
	}
	text, _ := fields[2].(string)
	return text
}
func (i *linuxIME) signal(m dbMessage) {
	if m.path != i.path || m.iface != ibusContext {
		return
	}
	s, ok := i.manager.TextInputState()
	if !ok || !i.focused || s.Identity != i.state.Identity {
		return
	}
	switch m.member {
	case "CommitText":
		if len(m.args) > 0 {
			i.manager.HandleEvent(ui.Event{Type: ui.CompositionEnd, Text: ibusText(m.args[0])})
			i.preedit = ""
			i.delivered = true
		}
	case "UpdatePreeditText", "UpdatePreeditTextWithMode":
		if len(m.args) >= 3 {
			i.preedit = ibusText(m.args[0])
			visible, _ := m.args[2].(bool)
			if visible {
				cursor, _ := m.args[1].(uint32)
				i.manager.HandleEvent(ui.Event{Type: ui.CompositionUpdate, Text: i.preedit, CompositionSelection: true, CompositionCursor: int(cursor), CompositionAnchor: int(cursor)})
			} else {
				i.manager.HandleEvent(ui.Event{Type: ui.CompositionEnd})
			}
		}
	case "ShowPreeditText":
		i.manager.HandleEvent(ui.Event{Type: ui.CompositionUpdate, Text: i.preedit})
	case "HidePreeditText":
		i.manager.HandleEvent(ui.Event{Type: ui.CompositionEnd})
	case "RequireSurroundingText":
		i.needSurrounding = true
	case "DeleteSurroundingText":
		if len(m.args) != 2 || s.Password {
			return
		}
		offset, ok := m.args[0].(int32)
		if !ok {
			return
		}
		count, ok := m.args[1].(uint32)
		if !ok {
			return
		}
		runes := []rune(s.Text)
		a := utf8.RuneCountInString(s.Text[:s.Cursor]) + int(offset)
		b := int64(a) + int64(count)
		if a >= 0 && b >= int64(a) && b <= int64(len(runes)) {
			i.manager.ReplaceTextRange(len(string(runes[:a])), len(string(runes[:int(b)])), "")
			i.delivered = true
		}
	}
}

func ibusAddress() string {
	if address := os.Getenv("IBUS_ADDRESS"); address != "" {
		return address
	}
	path := os.Getenv("IBUS_ADDRESS_FILE")
	if path == "" {
		var machine string
		for _, name := range []string{"/var/lib/dbus/machine-id", "/etc/machine-id"} {
			if data, err := os.ReadFile(name); err == nil {
				machine = strings.TrimSpace(string(data))
				if machine != "" {
					break
				}
			}
		}
		config, err := os.UserConfigDir()
		if err == nil && machine != "" {
			host, display := "unix", "0"
			value := os.Getenv("DISPLAY")
			if at := strings.LastIndexByte(value, ':'); at >= 0 {
				if at > 0 {
					host = value[:at]
				}
				display = strings.SplitN(value[at+1:], ".", 2)[0]
			}
			if wayland := os.Getenv("WAYLAND_DISPLAY"); wayland != "" {
				host, display = "unix", wayland
			}

			if !strings.ContainsAny(host+display, "/\\") {
				path = filepath.Join(config, "ibus", "bus", machine+"-"+host+"-"+display)
			}
		}
	}
	if path != "" {
		if info, err := os.Stat(path); err == nil && info.Size() <= 64<<10 {
			if data, err := os.ReadFile(path); err == nil {
				address := ""
				pid := 0
				for _, line := range strings.Split(string(data), "\n") {
					if strings.HasPrefix(line, "IBUS_ADDRESS=") {
						address = strings.TrimSpace(strings.TrimPrefix(line, "IBUS_ADDRESS="))
					}
					if strings.HasPrefix(line, "IBUS_DAEMON_PID=") {
						pid, _ = strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(line, "IBUS_DAEMON_PID=")))
					}
				}
				if address != "" && pid > 0 && syscall.Kill(pid, 0) == nil {
					return address
				}
			}
		}
	}
	return sessionBusAddress()
}
