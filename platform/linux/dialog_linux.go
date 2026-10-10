//go:build linux

package linux

import (
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/banditmoscow1337/nativeForms/platform"
)

const portalName = "org.freedesktop.portal.Desktop"
const portalPath = "/org/freedesktop/portal/desktop"

func portalAvailable() bool {
	b, err := openDBus(sessionBusAddress())
	if err != nil {
		return false
	}
	defer b.close()
	_, err = b.call(portalName, portalPath, "org.freedesktop.DBus.Properties", "Get", []dbValue{{"s", "org.freedesktop.portal.FileChooser"}, {"s", "version"}}, nil, 2*time.Second)
	return err == nil
}
func portalFile(parent, name string, save bool, pump func() error) (string, error) {
	b, err := openDBus(sessionBusAddress())
	if err != nil {
		return "", fmt.Errorf("linux: desktop portal unavailable: %w (%v)", platform.ErrUnsupported, err)
	}
	defer b.close()

	if err = b.match("type='signal',sender='org.freedesktop.portal.Desktop',interface='org.freedesktop.portal.Request',member='Response'"); err != nil {
		return "", err
	}
	token := fmt.Sprintf("nativeforms_%d", time.Now().UnixNano())
	handle := "/org/freedesktop/portal/desktop/request/" + strings.ReplaceAll(strings.TrimPrefix(b.name, ":"), ".", "_") + "/" + token
	responses := make(map[string]dbMessage)
	b.signal = func(m dbMessage) {
		if m.iface == "org.freedesktop.portal.Request" && m.member == "Response" && len(responses) < 16 {
			responses[m.path] = m
		}
	}
	options := map[string]dbValue{"handle_token": {"s", token}, "modal": {"b", true}}
	method, title := "OpenFile", "Open file"
	if save {
		method, title = "SaveFile", "Save file"
		options["current_name"] = dbValue{"s", name}
	}

	completed := false
	defer func() {
		if !completed {
			_, _ = b.send(portalName, handle, "org.freedesktop.portal.Request", "Close", nil, true)
		}
	}()
	values, err := b.call(portalName, portalPath, "org.freedesktop.portal.FileChooser", method, []dbValue{{"s", parent}, {"s", title}, dbDict(options)}, pump, 10*time.Second)
	if err != nil {
		return "", err
	}
	if len(values) != 1 {
		return "", fmt.Errorf("portal: missing request handle")
	}
	h, ok := values[0].(string)
	if !ok || !strings.HasPrefix(h, "/org/freedesktop/portal/desktop/request/") {
		return "", fmt.Errorf("portal: invalid request handle")
	}
	handle = h
	for {
		if m, ok := responses[handle]; ok {
			completed = true
			if len(m.args) != 2 {
				return "", fmt.Errorf("portal: invalid response")
			}
			code, ok := m.args[0].(uint32)
			if !ok {
				return "", fmt.Errorf("portal: invalid response code")
			}
			if code == 1 {
				return "", nil
			}
			if code != 0 {
				return "", fmt.Errorf("portal: file chooser failed (%d)", code)
			}
			value, ok := dbLookup(m.args[1], "uris")
			if !ok || value.sig != "as" {
				return "", fmt.Errorf("portal: missing file URI")
			}
			uris, ok := value.value.([]any)
			if !ok || len(uris) != 1 {
				return "", fmt.Errorf("portal: expected one file")
			}
			uri, ok := uris[0].(string)
			if !ok {
				return "", fmt.Errorf("portal: invalid URI")
			}
			files := waylandDroppedFiles([]byte(uri))
			if len(files) != 1 {
				return "", fmt.Errorf("portal: selected URI is not a local file")
			}
			return files[0], nil
		}
		if err = b.poll(); err != nil {
			return "", err
		}
		if err = pump(); err != nil {
			return "", err
		}
	}
}
func (w *X11Window) OpenFile() (string, error)            { return w.fileDialog("", false) }
func (w *X11Window) SaveFile(name string) (string, error) { return w.fileDialog(name, true) }
func (w *X11Window) fileDialog(name string, save bool) (string, error) {
	if w == nil || w.conn == nil || w.modal {
		return "", fmt.Errorf("x11: file dialog needs an available running window")
	}
	w.modal = true
	defer func() { w.modal = false }()
	if w.ime != nil {
		w.ime.focusOut()
	}
	w.compose.reset(w.manager)
	return portalFile(fmt.Sprintf("x11:%x", w.id), name, save, func() error {
		if w.closeRequested.Load() {
			return fmt.Errorf("x11: window closed")
		}
		if w.manager.NeedsFrame() {
			if err := w.paint(); err != nil {
				return err
			}
		}
		packet, err := w.clipboardPacket(time.Now().Add(10 * time.Millisecond))
		if e, ok := err.(net.Error); ok && e.Timeout() {
			return nil
		}
		if err != nil {
			return err
		}
		return w.clipboardDispatch(packet)
	})
}
func (w *WaylandWindow) OpenFile() (string, error)            { return w.fileDialog("", false) }
func (w *WaylandWindow) SaveFile(name string) (string, error) { return w.fileDialog(name, true) }
func (w *WaylandWindow) fileDialog(name string, save bool) (string, error) {
	if w == nil || w.wire == nil || w.modal {
		return "", fmt.Errorf("wayland: file dialog needs an available running window")
	}
	w.modal = true
	defer func() { w.modal = false }()

	w.repeatCode = 0
	if err := w.syncTextInput(false); err != nil {
		return "", err
	}
	return portalFile("", name, save, func() error {
		if w.closing.Load() {
			return fmt.Errorf("wayland: window closed")
		}
		if w.ready && w.Manager.NeedsFrame() {
			if err := w.paint(); err != nil {
				return err
			}
		}
		object, opcode, payload, err := w.wire.read(time.Now().Add(10 * time.Millisecond))
		if e, ok := err.(net.Error); ok && e.Timeout() {
			return nil
		}
		if err != nil {
			return err
		}
		return w.handle(object, opcode, payload)
	})
}
