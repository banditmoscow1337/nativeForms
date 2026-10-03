//go:build windows && (amd64 || arm64)

package windows

import "github.com/banditmoscow1337/nativeForms/platform"

func Capabilities() platform.Capabilities {
	return platform.Capabilities{SoftwareFrame: true, Pointer: true, Keyboard: true, TextInput: true, Clipboard: true, IME: true}
}

func (w *Window) Capabilities() platform.Capabilities { return Capabilities() }
