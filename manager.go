package nativeforms

import (
	"fmt"
	"sync"
	"time"
)

type notification struct {
	text     string
	deadline time.Time
}

type Frame struct {
	Width, Height int
	Commands      []DrawCommand
}

type Manager struct {
	mu                   sync.RWMutex
	root                 Component
	visible              bool
	interactive          bool
	unhandled            func(Event) bool
	theme                Theme
	fonts                *FontContext
	displayScale         float32
	focused              Component
	hovered              Component
	captured             Component
	capturedButton       MouseButton
	pointer              Vec2
	consumeUnhandled     bool
	gameNavigation       bool
	dirtyLayout          bool
	dirtyPaint           bool
	wake                 func()
	clipboardRead        func() (string, error)
	clipboardWrite       func(string) error
	clock                func() time.Time
	frameObserver        func(FrameSample)
	frameObserverOptions FrameObserverOptions
	frameTrace           *FrameTrace
	caretDue             time.Time
	popups               []popupEntry
	nextPopup            PopupID
	tooltipID            PopupID
	shortcuts            map[ShortcutID]Shortcut
	nextShortcut         ShortcutID
	postMu               sync.Mutex
	pending              []func()

	notificationMu sync.Mutex
	notifications  []notification

	frame        Frame
	clips        []Rect
	layoutWidth  int
	layoutHeight int
}

func New() *Manager {
	return &Manager{
		visible: true, theme: DefaultTheme(),
		displayScale: 1,
		dirtyLayout:  true, dirtyPaint: true,
		clock: time.Now,
		frame: Frame{Commands: make([]DrawCommand, 0, 1024)},
		clips: make([]Rect, 0, 16),
	}
}

func (manager *Manager) now() time.Time {
	if manager.clock != nil {
		return manager.clock()
	}
	return time.Now()
}

func (manager *Manager) SetRoot(root Component) {
	for len(manager.popups) > 0 {
		manager.CloseTopPopup()
	}
	manager.mu.Lock()
	blurred := manager.clearInteractionLocked()
	bindTree(manager.root, nil, nil)
	manager.root = root
	bindTree(root, nil, manager)
	applyTheme(root, manager.theme)
	manager.mu.Unlock()
	if blurred != nil {
		blurred.Handle(Event{Type: FocusLost})
	}
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
	if blurred != nil {
		blurred.Handle(Event{Type: FocusLost})
		manager.closeUnfocusedPopupsOutside(nil)
	}
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
	if blurred != nil {
		blurred.Handle(Event{Type: FocusLost})
		manager.closeUnfocusedPopupsOutside(nil)
	}
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
	for _, popup := range manager.popups {
		applyTheme(popup.content, theme)
	}
	manager.mu.Unlock()
	manager.InvalidateLayout()
}

func (manager *Manager) Theme() Theme {
	manager.mu.RLock()
	defer manager.mu.RUnlock()
	return manager.theme
}

func (manager *Manager) SetFontContext(ctx *FontContext) {
	manager.mu.Lock()
	manager.fonts = ctx
	manager.mu.Unlock()
	manager.InvalidateLayout()
}

func (manager *Manager) FontContext() *FontContext {
	manager.mu.RLock()
	defer manager.mu.RUnlock()
	return manager.fonts
}

// SetDisplayScale sets the number of framebuffer pixels per UI coordinate.
// Desktop adapters update it when the window moves between displays. It affects
// glyph rasterization, but leaves layout and input coordinates in logical units.
func (manager *Manager) SetDisplayScale(scale float32) {
	if !(scale > 0 && scale <= 8) {
		scale = 1
	}
	manager.mu.Lock()
	changed := manager.displayScale != scale
	manager.displayScale = scale
	manager.mu.Unlock()
	if changed {
		manager.InvalidatePaint()
	}
}

func (manager *Manager) DisplayScale() float32 {
	manager.mu.RLock()
	defer manager.mu.RUnlock()
	return manager.displayScale
}

func (manager *Manager) Notify(text string, duration ...time.Duration) {
	if text == "" {
		return
	}
	lifetime := 2500 * time.Millisecond
	if len(duration) > 0 {
		lifetime = duration[0]
	}
	if lifetime <= 0 {
		return
	}
	deadline := manager.now().Add(lifetime)
	manager.notificationMu.Lock()
	if count := len(manager.notifications); count > 0 && manager.notifications[count-1].text == text {
		manager.notifications[count-1].deadline = deadline
		manager.notificationMu.Unlock()
		manager.InvalidatePaint()
		return
	}
	manager.notifications = append(manager.notifications, notification{text: text, deadline: deadline})
	if len(manager.notifications) > 4 {
		manager.notifications = manager.notifications[len(manager.notifications)-4:]
	}
	manager.notificationMu.Unlock()
	manager.InvalidatePaint()
}

func (manager *Manager) Notifyf(format string, args ...any) {
	manager.Notify(fmt.Sprintf(format, args...))
}

func (manager *Manager) ShowMessage(format string, args ...any) {
	if len(args) == 1 {
		var seconds float64
		switch value := args[0].(type) {
		case float64:
			seconds = value
		case float32:
			seconds = float64(value)
		case int:
			seconds = float64(value)
		case int32:
			seconds = float64(value)
		default:
			manager.Notifyf(format, args...)
			return
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
	now := manager.now()
	if !manager.caretDue.IsZero() && !now.Before(manager.caretDue) {
		if client, ok := manager.textInputClient(); ok {
			client.BlinkCaret()
			manager.caretDue = now.Add(500 * time.Millisecond)
		} else {
			manager.caretDue = time.Time{}
		}
	}

	manager.mu.Lock()
	observer := manager.frameObserver
	observerOptions := manager.frameObserverOptions
	root := manager.root
	visible := manager.visible
	theme := manager.theme
	fonts := manager.fonts
	displayScale := manager.displayScale
	layoutNeeded := manager.dirtyLayout || width != manager.layoutWidth || height != manager.layoutHeight
	manager.dirtyLayout, manager.dirtyPaint = false, false
	manager.mu.Unlock()
	manager.frameTrace = newFrameTrace(observer, observerOptions, width, height)
	trace := manager.frameTrace
	if trace != nil && fonts != nil {
		trace.fonts = fonts
		_, _, trace.startHits, trace.startMisses = fonts.LayoutCacheStats()
	}
	buildStart := trace.Start()

	manager.frame.Width, manager.frame.Height = width, height
	clear(manager.frame.Commands)
	manager.frame.Commands = manager.frame.Commands[:0]
	if !visible || width <= 0 || height <= 0 {
		trace.countCommands(manager.frame)
		trace.Record(PhaseBuild, buildStart)
		return
	}
	viewport := Rect{W: float32(width), H: float32(height)}
	manager.clips = append(manager.clips[:0], viewport)
	canvas := Canvas{
		commands:     &manager.frame.Commands,
		clips:        manager.clips,
		theme:        &theme,
		fonts:        fonts,
		displayScale: displayScale,
		trace:        trace,
	}
	if root != nil && root.UIState().Visible() {
		if layoutNeeded {
			layoutStart := trace.Start()
			root.Measure(Constraints{Min: Vec2{X: viewport.W, Y: viewport.H}, Max: Vec2{X: viewport.W, Y: viewport.H}})
			root.Arrange(viewport)
			trace.Record(PhaseLayout, layoutStart)
		}
		paintStart := trace.Start()
		paintComponent(root, &canvas)
		trace.Record(PhasePaint, paintStart)
	}
	popupStart := trace.Start()
	manager.layoutPopups(viewport)
	trace.Record(PhaseLayout, popupStart)
	paintStart := trace.Start()
	for _, entry := range manager.popups {
		if entry.modal {
			canvas.Rect(viewport, theme.Scrim)
		}
		paintComponent(entry.content, &canvas)
	}
	manager.paintNotifications(&canvas, viewport, now)
	trace.Record(PhasePaint, paintStart)
	manager.clips = canvas.clips
	manager.layoutWidth, manager.layoutHeight = width, height
	trace.countCommands(manager.frame)
	trace.Record(PhaseBuild, buildStart)
}

func (manager *Manager) Frame() Frame { return manager.frame }

func (manager *Manager) InvalidateLayout() {
	manager.mu.Lock()
	manager.dirtyLayout, manager.dirtyPaint = true, true
	wake := manager.wake
	manager.mu.Unlock()
	if wake != nil {
		wake()
	}
}

func (manager *Manager) InvalidatePaint() {
	manager.mu.Lock()
	manager.dirtyPaint = true
	wake := manager.wake
	manager.mu.Unlock()
	if wake != nil {
		wake()
	}
}

func (manager *Manager) NeedsFrame() bool {
	manager.mu.RLock()
	dirty := manager.dirtyLayout || manager.dirtyPaint
	manager.mu.RUnlock()
	if dirty {
		return true
	}
	deadline, scheduled := manager.NextFrameDeadline()
	return scheduled && !manager.now().Before(deadline)
}

func (manager *Manager) NextFrameDeadline() (time.Time, bool) {
	manager.mu.RLock()
	visible := manager.visible
	manager.mu.RUnlock()
	if !visible {
		return time.Time{}, false
	}
	manager.notificationMu.Lock()
	defer manager.notificationMu.Unlock()
	var deadline time.Time
	if len(manager.notifications) > 0 {
		deadline = manager.notifications[0].deadline
		for _, item := range manager.notifications[1:] {
			if item.deadline.Before(deadline) {
				deadline = item.deadline
			}
		}
	}
	if !manager.caretDue.IsZero() && (deadline.IsZero() || manager.caretDue.Before(deadline)) {
		deadline = manager.caretDue
	}
	return deadline, !deadline.IsZero()
}

func (manager *Manager) NextFrameAfter() time.Duration {
	deadline, scheduled := manager.NextFrameDeadline()
	if !scheduled {
		return 0
	}
	return max(time.Duration(0), deadline.Sub(manager.now()))
}

func (manager *Manager) SetWakeHandler(wake func()) {
	manager.mu.Lock()
	manager.wake = wake
	manager.mu.Unlock()
}

func (manager *Manager) SetClipboardHandlers(read func() (string, error), write func(string) error) {
	manager.clipboardRead, manager.clipboardWrite = read, write
}

func (manager *Manager) Post(fn func()) {
	if fn == nil {
		return
	}
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
	for _, fn := range pending {
		fn()
	}
}

func paintComponent(component Component, canvas *Canvas) {
	if component == nil {
		return
	}
	state := component.UIState()
	if !state.Visible() || state.rect.Empty() || len(canvas.clips) == 0 {
		return
	}
	viewport := canvas.clips[len(canvas.clips)-1]
	if intersect(state.rect, viewport).Empty() {
		return
	}
	// Components with expensive, internally virtualized painting can narrow
	// their own visible range further. Cull must not change the clip stack.
	if culler, ok := component.(ViewportCuller); ok && culler.Cull(viewport) {
		return
	}
	if state.clip {
		canvas.PushClip(state.rect)
	}
	component.Paint(canvas)
	for _, child := range state.children {
		paintComponent(child, canvas)
	}
	if overlay, ok := component.(interface{ PaintOverlay(*Canvas) }); ok {
		overlay.PaintOverlay(canvas)
	}
	if state.clip {
		canvas.PopClip()
	}
}

func (manager *Manager) paintNotifications(canvas *Canvas, viewport Rect, now time.Time) {
	manager.notificationMu.Lock()
	active := manager.notifications[:0]
	for _, item := range manager.notifications {
		if now.Before(item.deadline) {
			active = append(active, item)
		}
	}
	manager.notifications = active
	var items [4]notification
	count := copy(items[:], active)
	manager.notificationMu.Unlock()

	y := float32(32)
	for _, item := range items[:count] {
		textSize := canvas.MeasureText(item.text, FontSpec{Size: canvas.theme.FontSize})
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
	if visible && interactive && root != nil && (event.Type == PointerDown || event.Type == PointerUp) {
		manager.InvalidatePaint()
	}
	return handled || (visible && interactive && root != nil && consumeUnhandled)
}

func (manager *Manager) SetConsumeUnhandledInput(consume bool) {
	manager.mu.Lock()
	manager.consumeUnhandled = consume
	manager.mu.Unlock()
}

func (manager *Manager) SetGameNavigation(enabled bool) {
	manager.mu.Lock()
	manager.gameNavigation = enabled
	manager.mu.Unlock()
}

func (manager *Manager) handleTreeEvent(root Component, event Event, gameNavigation bool) bool {
	switch event.Type {
	case PointerMove:
		target, blocked := manager.popupHit(event.X, event.Y)
		manager.setHovered(target)
		if manager.captured != nil {
			target = manager.captured
		}
		return dispatchEvent(target, event) || blocked
	case PointerDown:
		if manager.captured != nil && event.Button != manager.capturedButton {
			return dispatchEvent(manager.captured, event)
		}
		target, blocked := manager.popupHit(event.X, event.Y)
		if top := manager.topInteractivePopup(); !blocked && top != nil && !top.modal && !containsComponent(top.content, target) {
			manager.ClosePopup(top.id)
			target, blocked = manager.popupHit(event.X, event.Y)
		}
		manager.setHovered(target)
		if target == nil {
			if !blocked {
				manager.setFocus(nil)
			}
			return blocked
		}
		if manager.unfocusedPopupFor(target) == nil {
			if target.UIState().Focusable() {
				manager.setFocus(target)
			} else {
				manager.setFocus(nil)
			}
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
		blocked := false
		if target == nil {
			target, blocked = manager.popupHit(event.X, event.Y)
		}
		handled := dispatchEvent(target, event)
		if manager.captured != nil {
			manager.captured.UIState().pressed = false
		}
		manager.captured = nil
		return handled || blocked
	case PointerScroll:
		target, blocked := manager.popupHit(event.X, event.Y)
		if target == nil {
			return blocked
		}
		return dispatchScroll(target, event)
	case TextInput, CompositionStart, CompositionUpdate, CompositionEnd:
		return dispatchEvent(manager.focused, event)
	case KeyDown, KeyUp:
		if event.Type == KeyDown && event.Key == KeyEscape {
			if client, ok := manager.textInputClient(); ok && client.TextInputComposing() && dispatchEvent(manager.focused, event) {
				return true
			}
		}
		if event.Type == KeyDown && manager.popupKey(event) {
			return true
		}
		if event.Type == KeyDown && event.Key == KeyEscape {
			for i := len(manager.popups) - 1; i >= 0; i-- {
				if !manager.popups[i].passive {
					return manager.ClosePopup(manager.popups[i].id)
				}
			}
		}
		for target := manager.focused; target != nil; target = target.UIState().parent {
			if !target.UIState().Enabled() {
				continue
			}
			if target.Handle(event) {
				return true
			}
			if event.Type == KeyDown && manager.handleShortcut(event, target) {
				return true
			}
		}
		if event.Type == KeyDown {
			if manager.handleShortcut(event, nil) {
				return true
			}
			if modal := manager.topModal(); modal != nil {
				root = modal.content
			} else {
				for i := len(manager.popups) - 1; i >= 0; i-- {
					if !manager.popups[i].passive {
						root = manager.popups[i].content
						break
					}
				}
			}
			switch event.Key {
			case KeyTab:
				if event.Mods&ModShift != 0 {
					return manager.focusNext(root, -1)
				}
				return manager.focusNext(root, 1)
			case KeyDownArrow, KeyS:
				if gameNavigation && event.Mods == 0 {
					return manager.focusNext(root, 1)
				}
			case KeyUpArrow, KeyW:
				if gameNavigation && event.Mods == 0 {
					return manager.focusNext(root, -1)
				}
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

func dispatchScroll(target Component, event Event) bool {
	remaining := Vec2{X: -event.Scroll.X * 40, Y: -event.Scroll.Y * 40}
	used := false
	for target != nil {
		if target.UIState().Enabled() {
			if panel, ok := target.(*ScrollPanel); ok {
				before := remaining
				remaining = panel.ScrollBy(remaining.X, remaining.Y)
				if remaining != before {
					used = true
				}
				if remaining == (Vec2{}) {
					return true
				}
			} else {
				event.Scroll = Vec2{X: -remaining.X / 40, Y: -remaining.Y / 40}
				if target.Handle(event) {
					return true
				}
			}
		}
		target = target.UIState().parent
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
	if manager.tooltipID != 0 {
		manager.ClosePopup(manager.tooltipID)
		manager.tooltipID = 0
	}
	manager.hovered = component
	if component != nil {
		component.UIState().hovered = true
		if value := component.UIState().tooltip; value != "" && manager.topModal() == nil {
			manager.tooltipID = manager.ShowTooltip(value, Rect{X: manager.pointer.X + 14, Y: manager.pointer.Y + 18})
		}
	}
	manager.InvalidatePaint()
}

func (manager *Manager) setFocus(component Component) {
	if modal := manager.topModal(); modal != nil && component != nil && !containsComponent(modal.content, component) {
		return
	}
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
		for ancestor := component.UIState().parent; ancestor != nil; ancestor = ancestor.UIState().parent {
			if scroll, ok := ancestor.(*ScrollPanel); ok {
				scroll.ScrollIntoView(component)
			}
		}
	}
	manager.closeUnfocusedPopupsOutside(component)
	manager.caretDue = time.Time{}
	if client, ok := manager.textInputClient(); ok {
		client.ResetCaretBlink()
		manager.caretDue = manager.now().Add(500 * time.Millisecond)
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
	manager.caretDue = time.Time{}
	return blurred
}

func (manager *Manager) ClearInteraction() {
	manager.mu.Lock()
	blurred := manager.clearInteractionLocked()
	manager.mu.Unlock()
	if blurred != nil {
		blurred.Handle(Event{Type: FocusLost})
		manager.closeUnfocusedPopupsOutside(nil)
	}
	manager.InvalidatePaint()
}

func (manager *Manager) clearSubtreeInteraction(root *State) {
	inSubtree := func(component Component) bool {
		if component == nil {
			return false
		}
		for state := component.UIState(); state != nil; {
			if state == root {
				return true
			}
			if state.parent == nil {
				break
			}
			state = state.parent.UIState()
		}
		return false
	}
	if inSubtree(manager.focused) {
		manager.setFocus(nil)
	}
	if inSubtree(manager.hovered) {
		manager.setHovered(nil)
	}
	if inSubtree(manager.captured) {
		manager.captured.UIState().pressed = false
		manager.captured = nil
	}
	manager.InvalidatePaint()
}

func (manager *Manager) TextInputFocused() bool {
	_, ok := manager.textInputClient()
	return ok
}

func (manager *Manager) FocusedCaretRect() (Rect, bool) {
	client, ok := manager.textInputClient()
	if !ok {
		return Rect{}, false
	}
	return client.CaretRect(), true
}

func (manager *Manager) HoveredTextInput() bool {
	_, ok := manager.hovered.(TextInputClient)
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
