//go:build linux

package linux

import (
	"os"
	"path/filepath"
	"testing"
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
