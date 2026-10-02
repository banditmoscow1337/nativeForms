package nativeforms

import (
	"fmt"
	"sync"
	"time"
)

type notification struct {
	text      string
	remaining float64
}

// Frame is valid until the next BeginFrame call. Consumers must not modify
// Commands or retain its backing storage beyond that point.
type Frame struct {
	Width, Height int
	Commands      []DrawCommand
}

type Manager struct {
	mu             sync.RWMutex
	root           Component
	visible        bool
	interactive    bool
	unhandled      func(Event) bool
	theme          Theme
	focused        Component
	hovered        Component
	captured       Component
	capturedButton MouseButton
	pointer        Vec2
	consumeUnhandled bool
	gameNavigation bool
	dirtyLayout    bool
	dirtyPaint     bool
	wake           func()
	clipboardRead func() (string,error)
	clipboardWrite func(string) error
	caretDue time.Time
	postMu         sync.Mutex
	pending        []func()

	notificationMu sync.Mutex
	notifications  []notification

	frame          Frame
	clips          []Rect
	layoutWidth    int
	layoutHeight   int
}

// New creates a UI tree without opening a window or initializing Vulkan.
func New() *Manager {
	return &Manager{
		visible: true, theme: DefaultTheme(),
		dirtyLayout: true, dirtyPaint: true,
		frame: Frame{Commands: make([]DrawCommand, 0, 1024)},
		clips: make([]Rect, 0, 16),
	}
}

func (manager *Manager) SetRoot(root Component) {
	manager.mu.Lock()
	blurred := manager.clearInteractionLocked()
	bindTree(manager.root, nil, nil)
	manager.root = root
	bindTree(root, nil, manager)
	applyTheme(root, manager.theme)
	manager.mu.Unlock()
	if blurred != nil { blurred.Handle(Event{Type: FocusLost}) }
	manager.InvalidateLayout()
}

func (manager *Manager) Root() Component {
	manager.mu.RLock()
	defer manager.mu.RUnlock()
	return manager.root
}

func (manager *Manager) SetVisible(visible bool) {
	manager.mu.Lock()
	manager.visible = visible
	var blurred Component
	if !visible {
		blurred = manager.clearInteractionLocked()
	}
	manager.mu.Unlock()
	if blurred != nil { blurred.Handle(Event{Type: FocusLost}) }
	manager.InvalidatePaint()
}

func (manager *Manager) Visible() bool {
	manager.mu.RLock()
	defer manager.mu.RUnlock()
	return manager.visible
}

func (manager *Manager) SetInteractive(interactive bool) {
	manager.mu.Lock()
	manager.interactive = interactive
	var blurred Component
	if !interactive {
		blurred = manager.clearInteractionLocked()
	}
	manager.mu.Unlock()
	if blurred != nil { blurred.Handle(Event{Type: FocusLost}) }
	manager.InvalidatePaint()
}

func (manager *Manager) Interactive() bool {
	manager.mu.RLock()
	defer manager.mu.RUnlock()
	return manager.interactive
}

func (manager *Manager) WantsCursor() bool {
	manager.mu.RLock()
	defer manager.mu.RUnlock()
	return manager.visible && manager.interactive && manager.root != nil
}

func (manager *Manager) SetUnhandledEventHandler(handler func(Event) bool) {
	manager.mu.Lock()
	manager.unhandled = handler
	manager.mu.Unlock()
}

func (manager *Manager) SetTheme(theme Theme) {
	manager.mu.Lock()
	manager.theme = theme
	applyTheme(manager.root, theme)
	manager.mu.Unlock()
	manager.InvalidateLayout()
}

func (manager *Manager) Theme() Theme {
	manager.mu.RLock()
	defer manager.mu.RUnlock()
	return manager.theme
}

func (manager *Manager) Notify(text string, duration ...time.Duration) {
	if text == "" {
		return
	}
	seconds := 2.5
	if len(duration) > 0 {
		seconds = duration[0].Seconds()
	}
	if seconds <= 0 {
		return
	}
	manager.notificationMu.Lock()
	if count := len(manager.notifications); count > 0 && manager.notifications[count-1].text == text {
		manager.notifications[count-1].remaining = seconds
		manager.notificationMu.Unlock()
		manager.InvalidatePaint()
		return
	}
	manager.notifications = append(manager.notifications, notification{text: text, remaining: seconds})
	if len(manager.notifications) > 4 {
		manager.notifications = manager.notifications[len(manager.notifications)-4:]
	}
	manager.notificationMu.Unlock()
	manager.InvalidatePaint()
}

func (manager *Manager) Notifyf(format string, args ...any) {
	manager.Notify(fmt.Sprintf(format, args...))
}

// ShowMessage retains the previous notification helper during migration.
func (manager *Manager) ShowMessage(format string, args ...any) {
	if len(args) == 1 {
		var seconds float64
		switch value := args[0].(type) {
		case float64: seconds = value
		case float32: seconds = float64(value)
		case int: seconds = float64(value)
		case int32: seconds = float64(value)
		default: manager.Notifyf(format, args...); return
		}
		manager.Notify(format, time.Duration(seconds*float64(time.Second)))
		return
	}
	manager.Notifyf(format, args...)
}

func (manager *Manager) ClearNotifications() {
	manager.notificationMu.Lock()
	manager.notifications = manager.notifications[:0]
	manager.notificationMu.Unlock()
	manager.InvalidatePaint()
}

func (manager *Manager) BeginFrame(width, height int, deltaSeconds float64) {
	manager.ProcessPending()
	if !manager.caretDue.IsZero() && !time.Now().Before(manager.caretDue) {
		if field,ok:=manager.focused.(*TextField);ok {
			field.BlinkCaret()
			manager.caretDue=time.Now().Add(500*time.Millisecond)
		} else { manager.caretDue=time.Time{} }
	}

	manager.mu.Lock()
	root := manager.root
	visible := manager.visible
	theme := manager.theme
	layoutNeeded := manager.dirtyLayout || width != manager.layoutWidth || height != manager.layoutHeight
	manager.dirtyLayout, manager.dirtyPaint = false, false
	manager.mu.Unlock()

	manager.frame.Width, manager.frame.Height = width, height
	manager.frame.Commands = manager.frame.Commands[:0]
	if !visible || width <= 0 || height <= 0 {
		return
	}
	viewport := Rect{W: float32(width), H: float32(height)}
	manager.clips = append(manager.clips[:0], viewport)
	canvas := Canvas{
		commands: &manager.frame.Commands,
		clips:    manager.clips,
		theme:    &theme,
	}
	if root != nil && root.UIState().Visible() {
		if layoutNeeded {
			root.Measure(Constraints{Min: Vec2{X: viewport.W, Y: viewport.H}, Max: Vec2{X: viewport.W, Y: viewport.H}})
			root.Arrange(viewport)
		}
		paintComponent(root, &canvas)
	}
	manager.paintNotifications(&canvas, viewport, deltaSeconds)
	manager.clips = canvas.clips
	manager.layoutWidth, manager.layoutHeight = width, height
}

func (manager *Manager) Frame() Frame { return manager.frame }

// InvalidateLayout requests measurement, arrangement, and repaint.
func (manager *Manager) InvalidateLayout() {
	manager.mu.Lock()
	manager.dirtyLayout, manager.dirtyPaint = true, true
	wake := manager.wake
	manager.mu.Unlock()
	if wake != nil { wake() }
}

func (manager *Manager) InvalidatePaint() {
	manager.mu.Lock()
	manager.dirtyPaint = true
	wake := manager.wake
	manager.mu.Unlock()
	if wake != nil { wake() }
}

func (manager *Manager) NeedsFrame() bool {
	manager.mu.RLock()
	dirty := manager.dirtyLayout || manager.dirtyPaint
	manager.mu.RUnlock()
	return dirty
}

// NextFrameAfter reports the next notification or caret deadline. A host can
// use it with its event loop timer; zero means no scheduled frame is pending.
func (manager *Manager) NextFrameAfter() time.Duration {
	manager.notificationMu.Lock()
	defer manager.notificationMu.Unlock()
	var delay time.Duration
	if len(manager.notifications)>0 {
		remaining:=manager.notifications[0].remaining
		for _,item:=range manager.notifications[1:] {
			if item.remaining<remaining { remaining=item.remaining }
		}
		delay=time.Duration(remaining*float64(time.Second))
	}
	if !manager.caretDue.IsZero() {
		caret:=time.Until(manager.caretDue)
		if caret<=0 { caret=time.Millisecond }
		if delay<=0 || caret<delay { delay=caret }
	}
	return delay
}

// SetWakeHandler installs a thread-safe signal to wake the host event loop.
// The handler must only signal; it must not mutate the UI tree.
func (manager *Manager) SetWakeHandler(wake func()) {
	manager.mu.Lock()
	manager.wake = wake
	manager.mu.Unlock()
}

// SetClipboardHandlers connects focused editors to the host clipboard. Both
// functions are called on the UI owner goroutine; pass nil to detach a host.
func (manager *Manager) SetClipboardHandlers(read func()(string,error),write func(string) error) {
	manager.clipboardRead,manager.clipboardWrite=read,write
}

// Post queues a mutation from another goroutine. ProcessPending runs it on
// the owner goroutine before dispatch or painting.
func (manager *Manager) Post(fn func()) {
	if fn == nil { return }
	manager.postMu.Lock()
	manager.pending = append(manager.pending, fn)
	manager.postMu.Unlock()
	manager.InvalidatePaint()
}

func (manager *Manager) ProcessPending() {
	manager.postMu.Lock()
	pending := manager.pending
	manager.pending = nil
	manager.postMu.Unlock()
	for _, fn := range pending { fn() }
}

func paintComponent(component Component, canvas *Canvas) {
	if component == nil {
		return
	}
	state := component.UIState()
	if !state.Visible() || state.rect.Empty() {
		return
	}
	if state.clip {
		canvas.PushClip(state.rect)
	}
	component.Paint(canvas)
	for _, child := range state.children {
		paintComponent(child, canvas)
	}
	if overlay,ok:=component.(interface{ PaintOverlay(*Canvas) });ok { overlay.PaintOverlay(canvas) }
	if state.clip {
		canvas.PopClip()
	}
}

func (manager *Manager) paintNotifications(canvas *Canvas, viewport Rect, deltaSeconds float64) {
	if deltaSeconds < 0 {
		deltaSeconds = 0
	}
	manager.notificationMu.Lock()
	active := manager.notifications[:0]
	for _, item := range manager.notifications {
		item.remaining -= deltaSeconds
		if item.remaining > 0 {
			active = append(active, item)
		}
	}
	manager.notifications = active
	var items [4]notification
	count := copy(items[:], active)
	manager.notificationMu.Unlock()

	y := float32(32)
	for _, item := range items[:count] {
		textSize := MeasureText(item.text, canvas.theme.FontSize)
		width := minFloat32(viewport.W-32, maxFloat32(220, textSize.X+36))
		rect := Rect{X: (viewport.W - width) * 0.5, Y: y, W: width, H: maxFloat32(42, textSize.Y+14)}
		canvas.Rect(rect, canvas.theme.PanelAlt)
		canvas.Border(rect, 1, canvas.theme.Accent)
		canvas.Text(item.text, rect.Inset(Symmetric(12, 4)), canvas.theme.FontSize, TextCenter, canvas.theme.Text)
		y += rect.H + 8
	}
}

func (manager *Manager) HandleEvent(event Event) bool {
	manager.ProcessPending()
	manager.mu.RLock()
	root := manager.root
	visible := manager.visible
	interactive := manager.interactive
	unhandled := manager.unhandled
	consumeUnhandled := manager.consumeUnhandled
	gameNavigation := manager.gameNavigation
	manager.mu.RUnlock()

	if event.IsPointer() {
		manager.pointer = Vec2{X: event.X, Y: event.Y}
	}
	handled := false
	if visible && interactive && root != nil {
		handled = manager.handleTreeEvent(root, event, gameNavigation)
	}
	if !handled && unhandled != nil {
		handled = unhandled(event)
	}
	if visible && interactive && root != nil && (event.Type==PointerDown || event.Type==PointerUp) { manager.InvalidatePaint() }
	return handled || (visible && interactive && root != nil && consumeUnhandled)
}

// SetConsumeUnhandledInput preserves the former game-overlay policy when
// enabled. Desktop hosts normally use the default false value.
func (manager *Manager) SetConsumeUnhandledInput(consume bool) {
	manager.mu.Lock()
	manager.consumeUnhandled = consume
	manager.mu.Unlock()
}

// SetGameNavigation enables arrow/WASD focus movement for game menus.
// Tab and Shift+Tab remain available in either mode.
func (manager *Manager) SetGameNavigation(enabled bool) {
	manager.mu.Lock()
	manager.gameNavigation = enabled
	manager.mu.Unlock()
}

func (manager *Manager) handleTreeEvent(root Component, event Event, gameNavigation bool) bool {
	switch event.Type {
	case PointerMove:
		target := hitTest(root, event.X, event.Y)
		manager.setHovered(target)
		if manager.captured != nil {
			return dispatchEvent(manager.captured, event)
		}
		return false
	case PointerDown:
		if manager.captured != nil && event.Button != manager.capturedButton {
			return dispatchEvent(manager.captured, event)
		}
		target := hitTest(root, event.X, event.Y)
		manager.setHovered(target)
		if target == nil {
			manager.setFocus(nil)
			return false
		}
		if target.UIState().Focusable() {
			manager.setFocus(target)
		} else {
			manager.setFocus(nil)
		}
		manager.captured = target
		manager.capturedButton = event.Button
		target.UIState().pressed = event.Button == MouseLeft
		return dispatchEvent(target, event)
	case PointerUp:
		if manager.captured != nil && event.Button != manager.capturedButton {
			return dispatchEvent(manager.captured, event)
		}
		target := manager.captured
		if target == nil {
			target = hitTest(root, event.X, event.Y)
		}
		handled := dispatchEvent(target, event)
		if manager.captured != nil {
			manager.captured.UIState().pressed = false
		}
		manager.captured = nil
		return handled
	case PointerScroll:
		return dispatchScroll(hitTest(root, event.X, event.Y),event)
	case TextInput,CompositionStart,CompositionUpdate,CompositionEnd:
		return dispatchEvent(manager.focused, event)
	case KeyDown, KeyUp:
		if manager.focused != nil && dispatchEvent(manager.focused, event) {
			return true
		}
		if event.Type == KeyDown {
			switch event.Key {
			case KeyTab:
				if event.Mods&ModShift != 0 {
					return manager.focusNext(root, -1)
				}
				return manager.focusNext(root, 1)
			case KeyDownArrow, KeyS:
				if gameNavigation && event.Mods == 0 { return manager.focusNext(root, 1) }
			case KeyUpArrow, KeyW:
				if gameNavigation && event.Mods == 0 { return manager.focusNext(root, -1) }
			}
		}
		if manager.focused == nil {
			return dispatchEvent(root, event)
		}
	}
	return false
}

func hitTest(component Component, x, y float32) Component {
	if component == nil {
		return nil
	}
	state := component.UIState()
	if !state.Visible() || !state.Enabled() || !state.rect.Contains(x, y) {
		return nil
	}
	for index := len(state.children) - 1; index >= 0; index-- {
		if target := hitTest(state.children[index], x, y); target != nil {
			return target
		}
	}
	return component
}

func dispatchEvent(target Component, event Event) bool {
	for target != nil {
		if target.UIState().Enabled() && target.Handle(event) {
			return true
		}
		target = target.UIState().parent
	}
	return false
}

func dispatchScroll(target Component,event Event) bool {
	remaining:=Vec2{X:-event.Scroll.X*40,Y:-event.Scroll.Y*40}
	used:=false
	for target!=nil {
		if target.UIState().Enabled() {
			if panel,ok:=target.(*ScrollPanel);ok {
				before:=remaining
				remaining=panel.ScrollBy(remaining.X,remaining.Y)
				if remaining!=before { used=true }
				if remaining==(Vec2{}) { return true }
			} else {
				event.Scroll=Vec2{X:-remaining.X/40,Y:-remaining.Y/40}
				if target.Handle(event) { return true }
			}
		}
		target=target.UIState().parent
	}
	return used
}

func (manager *Manager) setHovered(component Component) {
	if sameComponent(manager.hovered, component) {
		return
	}
	if manager.hovered != nil {
		manager.hovered.UIState().hovered = false
	}
	manager.hovered = component
	if component != nil {
		component.UIState().hovered = true
	}
	manager.InvalidatePaint()
}

func (manager *Manager) setFocus(component Component) {
	if sameComponent(manager.focused, component) {
		return
	}
	if manager.focused != nil {
		manager.focused.UIState().focused = false
		manager.focused.Handle(Event{Type: FocusLost})
	}
	manager.focused = component
	if component != nil {
		component.UIState().focused = true
		for ancestor:=component.UIState().parent;ancestor!=nil;ancestor=ancestor.UIState().parent {
			if scroll,ok:=ancestor.(*ScrollPanel);ok { scroll.ScrollIntoView(component) }
		}
	}
	manager.caretDue=time.Time{}
	if field,ok:=component.(*TextField);ok {
		field.caretVisible=true
		manager.caretDue=time.Now().Add(500*time.Millisecond)
	}
	manager.InvalidatePaint()
}

func (manager *Manager) focusNext(root Component, direction int) bool {
	var components []Component
	collectFocusable(root, &components)
	if len(components) == 0 {
		return false
	}
	index := -1
	for candidateIndex, candidate := range components {
		if sameComponent(candidate, manager.focused) {
			index = candidateIndex
			break
		}
	}
	if direction < 0 {
		if index < 0 {
			index = 0
		}
		index = (index - 1 + len(components)) % len(components)
	} else {
		index = (index + 1) % len(components)
	}
	manager.setFocus(components[index])
	return true
}

func sameComponent(left, right Component) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return left.UIState() == right.UIState()
}

func collectFocusable(component Component, result *[]Component) {
	if component == nil {
		return
	}
	state := component.UIState()
	if !state.Visible() || !state.Enabled() {
		return
	}
	if state.Focusable() {
		*result = append(*result, component)
	}
	for _, child := range state.children {
		collectFocusable(child, result)
	}
}

func (manager *Manager) clearInteractionLocked() Component {
	blurred := manager.focused
	if manager.focused != nil {
		manager.focused.UIState().focused = false
	}
	if manager.hovered != nil {
		manager.hovered.UIState().hovered = false
	}
	if manager.captured != nil {
		manager.captured.UIState().pressed = false
	}
	manager.focused = nil
	manager.hovered = nil
	manager.captured = nil
	manager.caretDue=time.Time{}
	return blurred
}

func (manager *Manager) ClearInteraction() {
	manager.mu.Lock()
	blurred := manager.clearInteractionLocked()
	manager.mu.Unlock()
	if blurred != nil { blurred.Handle(Event{Type: FocusLost}) }
	manager.InvalidatePaint()
}

// clearSubtreeInteraction is called by State before a child leaves the tree.
// All tree mutations and input delivery are serialized on the UI owner.
func (manager *Manager) clearSubtreeInteraction(root *State) {
	inSubtree := func(component Component) bool {
		if component == nil { return false }
		for state := component.UIState(); state != nil; {
			if state == root { return true }
			if state.parent == nil { break }
			state = state.parent.UIState()
		}
		return false
	}
	if inSubtree(manager.focused) { manager.setFocus(nil) }
	if inSubtree(manager.hovered) { manager.setHovered(nil) }
	if inSubtree(manager.captured) {
		manager.captured.UIState().pressed = false
		manager.captured = nil
	}
	manager.InvalidatePaint()
}

func (manager *Manager) TextInputFocused() bool {
	_, ok := manager.focused.(*TextField)
	return ok
}

func (manager *Manager) FocusedCaretRect() (Rect,bool) {
	field,ok:=manager.focused.(*TextField)
	if !ok { return Rect{},false }
	return field.CaretRect(),true
}

func (manager *Manager) HoveredTextInput() bool {
	_, ok := manager.hovered.(*TextField)
	return ok
}

func (manager *Manager) CancelPointerCapture() {
	if manager.captured != nil {
		manager.captured.UIState().pressed = false
		manager.captured = nil
		manager.InvalidatePaint()
	}
}

func (manager *Manager) UnhandledEventHandler() func(Event) bool {
	manager.mu.RLock()
	defer manager.mu.RUnlock()
	return manager.unhandled
}
