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
	KeyPageUp
	KeyPageDown
	KeyInsert
	KeyF1
	KeyF2
	KeyF3
	KeyF4
	KeyF7
	KeyF9
	KeyF10
	KeyF11
	KeyF12
	KeyB
	KeyG
	KeyH
	KeyI
	KeyJ
	KeyK
	KeyL
	KeyM
	KeyN
	KeyO
	KeyP
	KeyT
	KeyU
	KeyDigit0
	KeyDigit1
	KeyDigit2
	KeyDigit3
	KeyDigit4
	KeyDigit5
	KeyDigit6
	KeyDigit7
	KeyDigit8
	KeyDigit9
	KeyMinus
	KeyEqual
	KeyLeftBracket
	KeyRightBracket
	KeyBackslash
	KeySemicolon
	KeyQuote
	KeyBackquote
	KeyComma
	KeyPeriod
	KeySlash
)

type MouseButton uint8

const (
	MouseLeft MouseButton = iota
	MouseRight
	MouseMiddle
)

type Event struct {
	Type                                 EventType
	Key                                  Key
	Repeat                               bool
	Button                               MouseButton
	X                                    float32
	Y                                    float32
	Scroll                               Vec2
	Mods                                 int
	Rune                                 rune
	Text                                 string
	Clicks                               int // 1: caret, 2: word, 3: line
	CompositionCursor, CompositionAnchor int
	CompositionSelection                 bool
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
