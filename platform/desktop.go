// Package platform describes the features of a native desktop adapter.
package platform

import (
	"errors"
	"fmt"
)

var ErrUnsupported = errors.New("nativeForms: platform feature is unsupported")

type Feature string

const (
	FeatureClipboard       Feature = "clipboard"
	FeatureIME             Feature = "ime"
	FeatureMultipleWindows Feature = "multiple-windows"
	FeatureDragDrop        Feature = "drag-drop"
	FeatureFileDialog      Feature = "file-dialog"
	FeatureWayland         Feature = "wayland"
)

type Capabilities struct {
	SoftwareFrame, Pointer, Keyboard, TextInput bool
	Clipboard, IME, MultipleWindows             bool
	DragDrop, FileDialog, Wayland               bool
}

// FileDialogs opens a native file picker on the adapter's UI goroutine.
// The empty path with a nil error means the user cancelled the picker.
type FileDialogs interface {
	OpenFile() (string, error)
	SaveFile(defaultName string) (string, error)
}

func (c Capabilities) Supports(feature Feature) bool {
	switch feature {
	case FeatureClipboard:
		return c.Clipboard
	case FeatureIME:
		return c.IME
	case FeatureMultipleWindows:
		return c.MultipleWindows
	case FeatureDragDrop:
		return c.DragDrop
	case FeatureFileDialog:
		return c.FileDialog
	case FeatureWayland:
		return c.Wayland
	}
	return false
}

func Unsupported(backend string, feature Feature) error {
	return fmt.Errorf("%s: %s: %w", backend, feature, ErrUnsupported)
}
