//go:build windows && (amd64 || arm64)

package windows

import (
	"fmt"
	"runtime"
	"syscall"
	"unicode/utf16"
	"unsafe"

	"github.com/banditmoscow1337/nativeForms/platform"
)

type openFilename struct {
	Size                                                   uint32
	Owner, Instance, Filter, CustomFilter                  uintptr
	MaxCustomFilter, FilterIndex                           uint32
	File                                                   uintptr
	MaxFile                                                uint32
	FileTitle                                              uintptr
	MaxFileTitle                                           uint32
	InitialDir, Title                                      uintptr
	Flags                                                  uint32
	FileOffset, FileExtension                              uint16
	DefaultExtension, CustomData, Hook, Template, Reserved uintptr
	ReservedFlags, FlagsEx                                 uint32
}

var _ platform.FileDialogs = (*Window)(nil)

type browseInfo struct {
	Owner, Root, DisplayName, Title uintptr
	Flags                           uint32
	Callback, Param                 uintptr
	Image                           int32
}

// OpenFolder selects a filesystem directory for a project.
func (w *Window) OpenFolder() (string, error) {
	if w == nil || w.hwnd.Load() == 0 {
		return "", fmt.Errorf("windows: folder dialog needs a running window")
	}
	shell, err := syscall.LoadLibrary("shell32.dll")
	if err != nil {
		return "", err
	}
	defer syscall.FreeLibrary(shell)
	ole, err := syscall.LoadLibrary("ole32.dll")
	if err != nil {
		return "", err
	}
	defer syscall.FreeLibrary(ole)
	browse, err := syscall.GetProcAddress(shell, "SHBrowseForFolderW")
	if err != nil {
		return "", err
	}
	pathForID, err := syscall.GetProcAddress(shell, "SHGetPathFromIDListW")
	if err != nil {
		return "", err
	}
	freeID, err := syscall.GetProcAddress(ole, "CoTaskMemFree")
	if err != nil {
		return "", err
	}
	display := make([]uint16, 260)
	title := utf16.Encode([]rune("Select project folder\x00"))
	info := browseInfo{Owner: w.hwnd.Load(), DisplayName: ptr16(display),
		Title: ptr16(title), Flags: 0x0001} // BIF_RETURNONLYFSDIRS.
	id := call(uintptr(browse), pointer(&info))
	runtime.KeepAlive(display)
	runtime.KeepAlive(title)
	runtime.KeepAlive(&info)
	if id == 0 {
		return "", nil
	}
	defer call(uintptr(freeID), id)
	path := make([]uint16, 32768)
	// Windows 8.1 and newer expose the long-path variant.
	if longPath, err := syscall.GetProcAddress(shell, "SHGetPathFromIDListEx"); err == nil {
		if call(uintptr(longPath), id, ptr16(path), uintptr(len(path)), 0) == 0 {
			return "", fmt.Errorf("windows: selected folder has no filesystem path")
		}
	} else if call(uintptr(pathForID), id, ptr16(path)) == 0 {
		return "", fmt.Errorf("windows: selected folder has no filesystem path")
	}
	runtime.KeepAlive(path)
	return syscall.UTF16ToString(path), nil
}

func (w *Window) OpenFile() (string, error) { return w.fileDialog(false, "") }

func (w *Window) SaveFile(defaultName string) (string, error) {
	return w.fileDialog(true, defaultName)
}

func (w *Window) fileDialog(save bool, defaultName string) (string, error) {
	if w == nil || w.hwnd.Load() == 0 {
		return "", fmt.Errorf("windows: file dialog needs a running window")
	}
	lib, err := syscall.LoadLibrary("comdlg32.dll")
	if err != nil {
		return "", err
	}
	defer syscall.FreeLibrary(lib)
	name := "GetOpenFileNameW"
	if save {
		name = "GetSaveFileNameW"
	}
	fn, err := syscall.GetProcAddress(lib, name)
	if err != nil {
		return "", err
	}
	extendedError, err := syscall.GetProcAddress(lib, "CommDlgExtendedError")
	if err != nil {
		return "", err
	}
	file := make([]uint16, 32768)
	initial := utf16.Encode([]rune(defaultName))
	if len(initial) >= len(file) {
		return "", fmt.Errorf("windows: initial file name is too long")
	}
	copy(file, initial)
	filter := utf16.Encode([]rune("All files\x00*.*\x00\x00"))
	flags := uint32(0x00080000 | 0x00000800) // OFN_EXPLORER | OFN_PATHMUSTEXIST.
	if save {
		flags |= 0x00000002
	} else {
		flags |= 0x00001000
	}
	data := openFilename{Size: uint32(unsafe.Sizeof(openFilename{})), Owner: w.hwnd.Load(),
		Filter: ptr16(filter), FilterIndex: 1, File: ptr16(file), MaxFile: uint32(len(file)), Flags: flags}
	ok := call(uintptr(fn), pointer(&data))
	runtime.KeepAlive(file)
	runtime.KeepAlive(filter)
	runtime.KeepAlive(&data)
	if ok == 0 {
		if code := call(uintptr(extendedError)); code != 0 {
			return "", fmt.Errorf("windows: %s failed (CommDlgExtendedError %#x)", name, code)
		}
		return "", nil
	}
	return syscall.UTF16ToString(file), nil
}

func (w *Window) filesDropped(handle uintptr) {
	defer call(win.dragFinish, handle)
	count := call(win.dragQueryFile, handle, ^uintptr(0), 0, 0)
	if count > 4096 {
		return
	}
	files := make([]string, 0, count)
	for i := uintptr(0); i < count; i++ {
		length := call(win.dragQueryFile, handle, i, 0, 0)
		if length == 0 || length > 32767 {
			continue
		}
		buffer := make([]uint16, length+1)
		if call(win.dragQueryFile, handle, i, ptr16(buffer), uintptr(len(buffer))) != 0 {
			files = append(files, syscall.UTF16ToString(buffer))
		}
		runtime.KeepAlive(buffer)
	}
	if len(files) != 0 && w.options.OnFilesDropped != nil {
		w.options.OnFilesDropped(files)
	}
}
