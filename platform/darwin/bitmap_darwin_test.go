//go:build darwin && (amd64 || arm64)

package darwin

import (
	"image/color"
	"runtime"
	"testing"
	"unsafe"

	ui "github.com/banditmoscow1337/nativeForms"
	"github.com/banditmoscow1337/nativeForms/software"
	"github.com/ebitengine/purego/objc"
)

func TestBitmapFramebufferSharesNativeBacking(t *testing.T) {
	runtime.LockOSThread() // AppKit autorelease pools belong to their OS thread.
	defer runtime.UnlockOSThread()
	if err := loadAppKit(); err != nil {
		t.Fatal(err)
	}
	pool := class("NSAutoreleasePool").Send(selector("new"))
	defer pool.Send(selector("drain"))
	rep, framebuffer, err := newBitmapFramebuffer(4, 2)
	if err != nil {
		t.Fatal(err)
	}
	defer rep.Send(selector("release"))
	pointer := objc.Send[uintptr](rep, selector("bitmapData"))
	if pointer != uintptr(unsafe.Pointer(&framebuffer.Pix[0])) {
		t.Fatal("software framebuffer has a separate pixel allocation")
	}
	runtime.GC() // A native-owned pixel slice must remain usable across GC.
	frame := ui.Frame{Width: 2, Height: 1, Commands: []ui.DrawCommand{{
		Kind: ui.CommandRoundedRect, Rect: ui.Rect{W: 2, H: 1},
		Clip: ui.Rect{W: 2, H: 1}, Tint: ui.RGBA(20, 40, 80, 255),
	}}}
	if err := software.New().RenderAtScale(frame, framebuffer, 2); err != nil {
		t.Fatal(err)
	}
	want := color.RGBA{R: 20, G: 40, B: 80, A: 255}
	if got := framebuffer.RGBAAt(3, 1); got != want {
		t.Fatalf("rendered pixel = %v, want %v", got, want)
	}
	native := unsafe.Slice((*byte)(unsafe.Pointer(pointer)), len(framebuffer.Pix))
	if offset := framebuffer.PixOffset(3, 1); native[offset] != want.R || native[offset+3] != want.A {
		t.Fatal("rendering did not update the bitmap representation")
	}
}
