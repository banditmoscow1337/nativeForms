//go:build linux

package linux

import (
	"errors"
	"net"
	"os"
	"path/filepath"
	"testing"

	ui "github.com/banditmoscow1337/nativeForms"
	"github.com/banditmoscow1337/nativeForms/platform"
)

func TestPortalChooserOptionsForProjectFolder(t *testing.T) {
	method, title, options := portalChooserOptions("request", "ignored", false, true)
	if method != "OpenFile" || title != "Open project folder" {
		t.Fatalf("folder request: method=%q title=%q", method, title)
	}
	if directory := options["directory"]; directory.sig != "b" || directory.value != true {
		t.Fatalf("portal directory option: %#v", directory)
	}
	if _, exists := options["current_name"]; exists {
		t.Fatal("folder chooser included a save-file name")
	}
	if token := options["handle_token"]; token.value != "request" {
		t.Fatalf("request token: %#v", token)
	}
	method, _, options = portalChooserOptions("save", "main.go", true, false)
	if method != "SaveFile" || options["current_name"].value != "main.go" {
		t.Fatal("save dialog was changed by folder support")
	}
}

func TestSelectedProjectFolderValidation(t *testing.T) {
	dir := t.TempDir()
	if err := validatePortalFolder(dir); err != nil {
		t.Fatalf("existing directory rejected: %v", err)
	}
	file := filepath.Join(dir, "main.go")
	if err := os.WriteFile(file, []byte("package main"), 0600); err != nil {
		t.Fatal(err)
	}
	if validatePortalFolder(file) == nil || validatePortalFolder(filepath.Join(dir, "missing")) == nil {
		t.Fatal("file or missing path accepted as project folder")
	}
}

func TestUnavailablePortalMatchesCapabilities(t *testing.T) {
	conn, peer := net.Pipe()
	defer conn.Close()
	defer peer.Close()
	x11 := NewX11(ui.New(), X11Options{})
	x11.conn = conn
	wayland := NewWayland(ui.New(), WaylandOptions{})
	wayland.wire = &waylandWire{}
	for _, window := range []interface {
		platform.FileDialogs
		Capabilities() platform.Capabilities
	}{x11, wayland} {
		if window.Capabilities().FileDialog {
			t.Fatalf("%T advertises an unavailable portal", window)
		}
		_, err := window.OpenFile()
		if !errors.Is(err, platform.ErrUnsupported) {
			t.Fatalf("%T: expected ErrUnsupported, got %v", window, err)
		}
	}
}
