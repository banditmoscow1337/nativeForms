//go:build linux

package linux

import (
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	ui "github.com/banditmoscow1337/nativeForms"
	"github.com/banditmoscow1337/nativeForms/platform"
)

type WaylandGlobal struct {
	Name      uint32
	Interface string
	Version   uint32
}

// ProbeWayland exchanges wl_display.get_registry and sync messages directly
// with the compositor. It never loads libwayland-client.
func ProbeWayland() ([]WaylandGlobal, error) {
	name := os.Getenv("WAYLAND_DISPLAY")
	if name == "" {
		return nil, fmt.Errorf("wayland: WAYLAND_DISPLAY is unset")
	}
	path := name
	if !filepath.IsAbs(path) {
		runtimeDir := os.Getenv("XDG_RUNTIME_DIR")
		if runtimeDir == "" {
			return nil, fmt.Errorf("wayland: XDG_RUNTIME_DIR is unset")
		}
		if strings.ContainsRune(name, '/') {
			return nil, fmt.Errorf("wayland: invalid display name")
		}
		path = filepath.Join(runtimeDir, name)
	}
	conn, err := net.DialTimeout("unix", path, 2*time.Second)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(3 * time.Second))
	// wl_display@1.get_registry(new_id=2), then sync(new_id=3).
	request := make([]byte, 24)
	binary.LittleEndian.PutUint32(request[0:], 1)
	binary.LittleEndian.PutUint32(request[4:], (12<<16)|1)
	binary.LittleEndian.PutUint32(request[8:], 2)
	binary.LittleEndian.PutUint32(request[12:], 1)
	binary.LittleEndian.PutUint32(request[16:], 12<<16)
	binary.LittleEndian.PutUint32(request[20:], 3)
	if _, err = conn.Write(request); err != nil {
		return nil, err
	}
	var globals []WaylandGlobal
	for {
		header := make([]byte, 8)
		if _, err = io.ReadFull(conn, header); err != nil {
			return nil, err
		}
		object := binary.LittleEndian.Uint32(header)
		word := binary.LittleEndian.Uint32(header[4:])
		opcode, size := uint16(word), int(word>>16)
		if size < 8 || size > 1<<20 || size%4 != 0 {
			return nil, fmt.Errorf("wayland: invalid event size")
		}
		payload := make([]byte, size-8)
		if _, err = io.ReadFull(conn, payload); err != nil {
			return nil, err
		}
		if object == 1 && opcode == 0 {
			return nil, fmt.Errorf("wayland: compositor protocol error")
		}
		if object == 3 && opcode == 0 {
			return globals, nil
		}
		if object != 2 || opcode != 0 {
			continue
		}
		if len(payload) < 12 {
			return nil, fmt.Errorf("wayland: short registry global")
		}
		id := binary.LittleEndian.Uint32(payload)
		n := int(binary.LittleEndian.Uint32(payload[4:]))
		if n < 1 || n > len(payload)-12 {
			return nil, fmt.Errorf("wayland: invalid interface name")
		}
		iface := string(payload[8 : 8+n-1])
		offset := 8 + padded(n)
		if offset+4 > len(payload) {
			return nil, fmt.Errorf("wayland: invalid global version")
		}
		globals = append(globals, WaylandGlobal{Name: id, Interface: iface, Version: binary.LittleEndian.Uint32(payload[offset:])})
	}
}

type WaylandWindow struct{ Manager *ui.Manager }

func (w *WaylandWindow) Capabilities() platform.Capabilities { return WaylandCapabilities() }

func (w *WaylandWindow) Run() error {
	if _, err := ProbeWayland(); err != nil {
		return err
	}
	return platform.Unsupported("wayland", platform.FeatureWayland)
}
