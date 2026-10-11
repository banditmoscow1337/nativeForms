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

type FileDialogs interface {
	OpenFile() (string, error)
	OpenFolder() (string, error)
	SaveFile(defaultName string) (string, error)
}

// ValidateSize checks a requested logical client size against the renderer limit.
func ValidateSize(width, height int, scale float32) error {
	if width < 1 || height < 1 || width > 16384 || height > 16384 {
		return fmt.Errorf("nativeForms: invalid client size %dx%d at scale %g", width, height, scale)
	}
	pw, ph := PixelSize(width, scale), PixelSize(height, scale)
	if pw < 1 || ph < 1 || pw > 16384 || ph > 16384 || int64(pw)*int64(ph) > 64*1024*1024 {
		return fmt.Errorf("nativeForms: invalid client size %dx%d at scale %g", width, height, scale)
	}
	return nil
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
