//go:build linux

package linux

import (
	"strconv"
	"strings"
	"time"
)

func x11ResourceScale(resources string) float32 {
	for _, line := range strings.Split(resources, "\n") {
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 || !strings.HasSuffix(strings.TrimSpace(parts[0]), "Xft.dpi") {
			continue
		}
		dpi, err := strconv.ParseFloat(strings.TrimSpace(parts[1]), 32)
		if err == nil && dpi >= 48 && dpi <= 768 {
			return float32(dpi / 96)
		}
	}
	return 1
}

func (w *X11Window) resourceScale() float32 {
	property, err := w.atom("RESOURCE_MANAGER")
	if err != nil {
		return 1
	}
	data, err := w.readProperty(w.root, property, false, time.Now().Add(time.Second))
	if err != nil || data.format != 8 {
		return 1
	}
	return x11ResourceScale(string(data.data))
}
