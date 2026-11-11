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

var asciiLetterKeys = [26]Key{
	KeyA, KeyB, KeyC, KeyD, KeyE, KeyF, KeyG, KeyH, KeyI, KeyJ, KeyK, KeyL, KeyM,
	KeyN, KeyO, KeyP, KeyQ, KeyR, KeyS, KeyT, KeyU, KeyV, KeyW, KeyX, KeyY, KeyZ,
}

var functionKeys = [12]Key{
	KeyF1, KeyF2, KeyF3, KeyF4, KeyF5, KeyF6,
	KeyF7, KeyF8, KeyF9, KeyF10, KeyF11, KeyF12,
}

// LetterKey maps an ASCII letter to its Key constant.
func LetterKey(r rune) Key {
	if r >= 'A' && r <= 'Z' {
		return asciiLetterKeys[r-'A']
	}
	if r >= 'a' && r <= 'z' {
		return asciiLetterKeys[r-'a']
	}
	return KeyUnknown
}

// FunctionKey returns KeyF1..KeyF12 for 1-based index n in 1..12.
func FunctionKey(n int) Key {
	if n >= 1 && n <= len(functionKeys) {
		return functionKeys[n-1]
	}
	return KeyUnknown
}
