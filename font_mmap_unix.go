//go:build darwin || linux

package nativeforms

import (
	"os"
	"syscall"
)

func readSystemFontFile(path string) (data []byte, mapped bool, err error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, false, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, false, err
	}
	size := info.Size()
	if size <= 0 || size > 64<<20 {
		data, err = os.ReadFile(path)
		return data, false, err
	}
	b, err := syscall.Mmap(int(f.Fd()), 0, int(size), syscall.PROT_READ, syscall.MAP_SHARED)
	if err == nil && len(b) > 0 {
		return b, true, nil
	}
	data, err = os.ReadFile(path)
	return data, false, err
}
