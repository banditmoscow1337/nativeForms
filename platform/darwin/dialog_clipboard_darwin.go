//go:build darwin && (amd64 || arm64)

package darwin

import (
	"fmt"

	"github.com/banditmoscow1337/nativeForms/platform"
	"github.com/ebitengine/purego/objc"
)

func (w *Window) ClipboardText() (string, error) {
	if w == nil || w.window == 0 {
		return "", fmt.Errorf("darwin: clipboard needs a running window")
	}
	board := class("NSPasteboard").Send(selector("generalPasteboard"))
	value := board.Send(selector("stringForType:"), nsString("public.utf8-plain-text"))
	if value == 0 {
		return "", nil
	}
	if objc.Send[uintptr](value, selector("lengthOfBytesUsingEncoding:"), uint64(4)) > platform.ClipboardTextLimit {
		return "", fmt.Errorf("darwin: clipboard text is too large")
	}
	text := stringValue(value)
	if err := platform.ValidateClipboardText(text); err != nil {
		return "", err
	}
	return text, nil
}

func (w *Window) SetClipboardText(value string) error {
	if w == nil || w.window == 0 {
		return fmt.Errorf("darwin: clipboard needs a running window")
	}
	if err := platform.ValidateClipboardText(value); err != nil {
		return err
	}
	board := class("NSPasteboard").Send(selector("generalPasteboard"))
	board.Send(selector("clearContents"))
	if !objc.Send[bool](board, selector("setString:forType:"), nsString(value), nsString("public.utf8-plain-text")) {
		return fmt.Errorf("darwin: clipboard write failed")
	}
	return nil
}


func (w *Window) openModalPanel(chooseFiles, chooseDirs bool, label string) (string, error) {
	if w == nil || w.window == 0 {
		return "", fmt.Errorf("darwin: %s dialog needs a running window", label)
	}
	panel := class("NSOpenPanel").Send(selector("openPanel"))
	if panel == 0 {
		return "", fmt.Errorf("darwin: NSOpenPanel unavailable")
	}
	panel.Send(selector("setCanChooseDirectories:"), chooseDirs)
	panel.Send(selector("setCanChooseFiles:"), chooseFiles)
	panel.Send(selector("setAllowsMultipleSelection:"), false)
	if objc.Send[int64](panel, selector("runModal")) != 1 {
		return "", nil
	}
	return stringValue(panel.Send(selector("URL")).Send(selector("path"))), nil
}

func (w *Window) OpenFile() (string, error) {
	return w.openModalPanel(true, false, "file")
}

// OpenFolder chooses a project directory with a native macOS panel.
func (w *Window) OpenFolder() (string, error) {
	return w.openModalPanel(false, true, "folder")
}

func (w *Window) SaveFile(defaultName string) (string, error) {
	if w == nil || w.window == 0 {
		return "", fmt.Errorf("darwin: file dialog needs a running window")
	}
	panel := class("NSSavePanel").Send(selector("savePanel"))
	if panel == 0 {
		return "", fmt.Errorf("darwin: NSSavePanel unavailable")
	}
	if defaultName != "" {
		panel.Send(selector("setNameFieldStringValue:"), nsString(defaultName))
	}
	if objc.Send[int64](panel, selector("runModal")) != 1 {
		return "", nil
	}
	return stringValue(panel.Send(selector("URL")).Send(selector("path"))), nil
}

