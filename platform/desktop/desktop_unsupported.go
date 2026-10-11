//go:build (!linux && !windows && !darwin) || (windows && !amd64 && !arm64) || (darwin && !amd64 && !arm64)

package desktop

import (
	"fmt"
	"runtime"

	ui "github.com/banditmoscow1337/nativeForms"
)

func newWindow(_ *ui.Manager, _ Options) (Window, error) {
	return nil, fmt.Errorf("desktop: unsupported target %s/%s", runtime.GOOS, runtime.GOARCH)
}
