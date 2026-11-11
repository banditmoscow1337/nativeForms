//go:build darwin && (amd64 || arm64)

package darwin

import (
	"structs"
	"sync"
	"unsafe"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"
)

type point struct {
	_    structs.HostLayout
	X, Y float64
}
type size struct {
	_    structs.HostLayout
	W, H float64
}
type rect struct {
	_      structs.HostLayout
	Origin point
	Size   size
}

func makeRect(w, h float64) rect { return rect{Size: size{W: w, H: h}} }

var (
	appMu              sync.Mutex
	selectorMu         sync.RWMutex
	selectorCache      = make(map[string]objc.SEL, 128)
	classMu            sync.RWMutex
	classCache         = make(map[string]objc.ID, 32)
	objcOnce           sync.Once
	objcMsgSend        uintptr
	objcMsgSendFloat0  func(objc.ID, objc.SEL) float64
	objcMsgSendFloat1  func(objc.ID, objc.SEL, float64) objc.ID
	objcMsgSendRect0   func(objc.ID, objc.SEL) rect
	objcMsgSendRect1   func(objc.ID, objc.SEL, rect) rect
	objcMsgSendPoint0  func(objc.ID, objc.SEL) point
	objcMsgSendRegion  func(objc.ID, objc.SEL, metalRegion, uint64, unsafe.Pointer, uint64)

	// Pre-cached selectors and classes for zero-lock per-frame execution.
	clsNSAutoreleasePool objc.ID
	clsNSDate            objc.ID
	selNew               objc.SEL
	selDrain             objc.SEL
	selRelease           objc.SEL
	selRetain            objc.SEL
	selIsVisible         objc.SEL
	selIsKeyWindow       objc.SEL
	selBounds            objc.SEL
	selConvertToBacking  objc.SEL
	selBackingScale      objc.SEL
	selDateWithInterval  objc.SEL
	selNextEvent         objc.SEL
	selPostEventAtStart  objc.SEL
	selSendEvent         objc.SEL
	selEventType         objc.SEL
	selEventWindow       objc.SEL
	selLocationInWindow  objc.SEL
	selInputContext      objc.SEL
	selInvalidateCoords  objc.SEL
	selReplaceRegion     objc.SEL
)

func ensureObjcMsgSend() uintptr {
	if objcMsgSend != 0 {
		return objcMsgSend
	}
	objcOnce.Do(func() {
		if lib, err := purego.Dlopen("/usr/lib/libobjc.A.dylib", purego.RTLD_GLOBAL|purego.RTLD_LAZY); err == nil {
			objcMsgSend, _ = purego.Dlsym(lib, "objc_msgSend")
			if objcMsgSend != 0 {
				purego.RegisterFunc(&objcMsgSendFloat0, objcMsgSend)
				purego.RegisterFunc(&objcMsgSendFloat1, objcMsgSend)
				purego.RegisterFunc(&objcMsgSendRect0, objcMsgSend)
				purego.RegisterFunc(&objcMsgSendRect1, objcMsgSend)
				purego.RegisterFunc(&objcMsgSendPoint0, objcMsgSend)
				purego.RegisterFunc(&objcMsgSendRegion, objcMsgSend)
			}
		}
	})
	return objcMsgSend
}

func msgSend0(id objc.ID, sel objc.SEL) objc.ID {
	if id == 0 {
		return 0
	}
	r, _, _ := purego.SyscallN(ensureObjcMsgSend(), uintptr(id), uintptr(sel))
	return objc.ID(r)
}

func msgSend1(id objc.ID, sel objc.SEL, a0 uintptr) objc.ID {
	if id == 0 {
		return 0
	}
	r, _, _ := purego.SyscallN(ensureObjcMsgSend(), uintptr(id), uintptr(sel), a0)
	return objc.ID(r)
}

func msgSend2(id objc.ID, sel objc.SEL, a0, a1 uintptr) objc.ID {
	if id == 0 {
		return 0
	}
	r, _, _ := purego.SyscallN(ensureObjcMsgSend(), uintptr(id), uintptr(sel), a0, a1)
	return objc.ID(r)
}

func msgSend3(id objc.ID, sel objc.SEL, a0, a1, a2 uintptr) objc.ID {
	if id == 0 {
		return 0
	}
	r, _, _ := purego.SyscallN(ensureObjcMsgSend(), uintptr(id), uintptr(sel), a0, a1, a2)
	return objc.ID(r)
}

func msgSend4(id objc.ID, sel objc.SEL, a0, a1, a2, a3 uintptr) objc.ID {
	if id == 0 {
		return 0
	}
	r, _, _ := purego.SyscallN(ensureObjcMsgSend(), uintptr(id), uintptr(sel), a0, a1, a2, a3)
	return objc.ID(r)
}

func msgSendBool0(id objc.ID, sel objc.SEL) bool {
	if id == 0 {
		return false
	}
	r, _, _ := purego.SyscallN(ensureObjcMsgSend(), uintptr(id), uintptr(sel))
	return uint8(r) != 0
}

func msgSendUint0(id objc.ID, sel objc.SEL) uint64 {
	if id == 0 {
		return 0
	}
	r, _, _ := purego.SyscallN(ensureObjcMsgSend(), uintptr(id), uintptr(sel))
	return uint64(r)
}

func msgSendFloat0(id objc.ID, sel objc.SEL) float64 {
	if id == 0 {
		return 0
	}
	ensureObjcMsgSend()
	if objcMsgSendFloat0 != nil {
		return objcMsgSendFloat0(id, sel)
	}
	return objc.Send[float64](id, sel)
}

func msgSendFloat1(id objc.ID, sel objc.SEL, a0 float64) objc.ID {
	if id == 0 {
		return 0
	}
	ensureObjcMsgSend()
	if objcMsgSendFloat1 != nil {
		return objcMsgSendFloat1(id, sel, a0)
	}
	return id.Send(sel, a0)
}

func msgSendRect0(id objc.ID, sel objc.SEL) rect {
	if id == 0 {
		return rect{}
	}
	ensureObjcMsgSend()
	if objcMsgSendRect0 != nil {
		return objcMsgSendRect0(id, sel)
	}
	return objc.Send[rect](id, sel)
}

func msgSendRect1(id objc.ID, sel objc.SEL, a0 rect) rect {
	if id == 0 {
		return rect{}
	}
	ensureObjcMsgSend()
	if objcMsgSendRect1 != nil {
		return objcMsgSendRect1(id, sel, a0)
	}
	return objc.Send[rect](id, sel, a0)
}

func msgSendPoint0(id objc.ID, sel objc.SEL) point {
	if id == 0 {
		return point{}
	}
	ensureObjcMsgSend()
	if objcMsgSendPoint0 != nil {
		return objcMsgSendPoint0(id, sel)
	}
	return objc.Send[point](id, sel)
}

func selector(name string) objc.SEL {
	selectorMu.RLock()
	sel, ok := selectorCache[name]
	selectorMu.RUnlock()
	if ok {
		return sel
	}
	sel = objc.RegisterName(name)
	selectorMu.Lock()
	selectorCache[name] = sel
	selectorMu.Unlock()
	return sel
}

func class(name string) objc.ID {
	classMu.RLock()
	cls, ok := classCache[name]
	classMu.RUnlock()
	if ok {
		return cls
	}
	cls = objc.ID(objc.GetClass(name))
	classMu.Lock()
	classCache[name] = cls
	classMu.Unlock()
	return cls
}

func nsString(value string) objc.ID {
	return class("NSString").Send(selector("stringWithUTF8String:"), value)
}

var cocoaOnce sync.Once
var cocoaErr error
var onMainThread func() int32

func loadAppKit() error {
	cocoaOnce.Do(func() {
		var lib uintptr
		lib, cocoaErr = purego.Dlopen("/System/Library/Frameworks/Cocoa.framework/Cocoa", purego.RTLD_GLOBAL|purego.RTLD_LAZY)
		if cocoaErr != nil {
			return
		}
		purego.RegisterLibFunc(&onMainThread, lib, "pthread_main_np")
		ensureObjcMsgSend()
		clsNSAutoreleasePool = class("NSAutoreleasePool")
		clsNSDate = class("NSDate")
		selNew = selector("new")
		selDrain = selector("drain")
		selRelease = selector("release")
		selRetain = selector("retain")
		selIsVisible = selector("isVisible")
		selIsKeyWindow = selector("isKeyWindow")
		selBounds = selector("bounds")
		selConvertToBacking = selector("convertRectToBacking:")
		selBackingScale = selector("backingScaleFactor")
		selDateWithInterval = selector("dateWithTimeIntervalSinceNow:")
		selNextEvent = selector("nextEventMatchingMask:untilDate:inMode:dequeue:")
		selPostEventAtStart = selector("postEvent:atStart:")
		selSendEvent = selector("sendEvent:")
		selEventType = selector("type")
		selEventWindow = selector("window")
		selLocationInWindow = selector("locationInWindow")
		selInputContext = selector("inputContext")
		selInvalidateCoords = selector("invalidateCharacterCoordinates")
		selReplaceRegion = selector("replaceRegion:mipmapLevel:withBytes:bytesPerRow:")
		cocoaErr = registerView()
	})
	return cocoaErr
}

func stringValue(value objc.ID) string {
	length := objc.Send[uintptr](value, selector("lengthOfBytesUsingEncoding:"), uint64(4))
	pointer := objc.Send[uintptr](value, selector("UTF8String"))
	if pointer == 0 || length == 0 || length > 16<<20 {
		return ""
	}
	return string(unsafe.Slice((*byte)(unsafe.Pointer(pointer)), length))
}
