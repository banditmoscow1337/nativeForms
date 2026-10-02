package nativeforms

type EventType uint8

const (
	KeyDown EventType = iota
	KeyUp
	PointerMove
	PointerDown
	PointerUp
	PointerScroll
	TextInput
	FocusLost
	CompositionStart
	CompositionUpdate
	CompositionEnd
)

type Key uint16

const (
	KeyUnknown Key = iota
	KeySpace
	KeyEnter
	KeyEscape
	KeyTab
	KeyUpArrow
	KeyDownArrow
	KeyLeftArrow
	KeyRightArrow
	KeyW
	KeyA
	KeyS
	KeyD
	KeyQ
	KeyE
	KeyControl
	KeyBackspace
	KeyDelete
	KeyHome
	KeyEnd
	KeyR
	KeyF
	KeyZ
	KeyY
	KeyF5
	KeyF6
	KeyF8
	KeyC
	KeyV
	KeyX
)

type MouseButton uint8

const (
	MouseLeft MouseButton = iota
	MouseRight
	MouseMiddle
)

type Event struct {
	Type   EventType
	Key    Key
	Repeat bool
	Button MouseButton
	X      float32
	Y      float32
	Scroll Vec2
	Mods   int
	Rune   rune
	Text   string // IME preedit or committed result
	Clicks int    // 1: caret, 2: word, 3: line
}

func (event Event) IsPointer() bool {
	return event.Type == PointerMove || event.Type == PointerDown ||
		event.Type == PointerUp || event.Type == PointerScroll
}

const (
	ModShift   = 1
	ModControl = 2
	ModAlt     = 4
	ModSuper   = 8
)
