//go:build windows

package nativeforms

import (
	"os"
	"syscall"
	"unsafe"
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
	hMap, err := syscall.CreateFileMapping(syscall.Handle(f.Fd()), nil, syscall.PAGE_READONLY, 0, 0, nil)
	if err != nil || hMap == 0 {
		data, err = os.ReadFile(path)
		return data, false, err
	}
	defer syscall.CloseHandle(hMap)
	addr, err := syscall.MapViewOfFile(hMap, syscall.FILE_MAP_READ, 0, 0, uintptr(size))
	if err != nil || addr == 0 {
		data, err = os.ReadFile(path)
		return data, false, err
	}
	b := unsafe.Slice((*byte)(unsafe.Add(nil, addr)), int(size))
	if len(b) > 0 {
		return b, true, nil
	}
	data, err = os.ReadFile(path)
	return data, false, err
}
