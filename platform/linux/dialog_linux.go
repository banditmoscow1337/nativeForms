//go:build linux

package linux

import "github.com/banditmoscow1337/nativeForms/platform"

func (*X11Window) OpenFile() (string, error) {
	return "", platform.Unsupported("x11", platform.FeatureFileDialog)
}

func (*X11Window) SaveFile(string) (string, error) {
	return "", platform.Unsupported("x11", platform.FeatureFileDialog)
}

func (*WaylandWindow) OpenFile() (string, error) {
	return "", platform.Unsupported("wayland", platform.FeatureFileDialog)
}

func (*WaylandWindow) SaveFile(string) (string, error) {
	return "", platform.Unsupported("wayland", platform.FeatureFileDialog)
}
