package nativeforms

import (
	"fmt"
	"image"
	"sync"
	"time"
	"unsafe"
)

const maxVertices = 64 * 1024

type notification struct {
	text      string
	remaining float64
}

type Texture struct {
}

type Renderer interface {
	Device() uintptr
	QuadPipelineLayout() uint64
	FramesInFlight() uint32
	CreateBufferHelper(size uint64, usage, props uint32) (uint64, uint64)
	MapBufferHelper(buffer, offset, size uint64) unsafe.Pointer
	CreateTexture(name string, img image.Image) int
	SwapchainFormat() uint32
	SwapRenderPass() uint64
	QuadSetLayout() uint64
	DefaultSampler() uint64
	TextureImageViewByID(id uint32) (uint64, bool)
	DestroyTexture(uint32)
	TextureReady(id uint32) bool
	CurrentFrameIndex() uint32
	DestroyBufferHelper(uint64)
	DescriptorPool() uint64
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

	notificationMu sync.Mutex
	notifications  []notification

	renderer       Renderer
	device         uintptr
	pipeline       uint64
	pipelineFormat uint32
	pipelineLayout uint64
	descriptorSet  uint64
	buffer         uint64
	mapped         unsafe.Pointer
	frameStride    uint64
	atlasTextureID uint32
	vertices       []Vertex
	clips          []Rect
}

func New(renderer Renderer) *Manager {
	manager := &Manager{
		renderer: renderer,
		visible:  true,
		theme:    DefaultTheme(),
		vertices: make([]Vertex, 0, maxVertices),
		clips:    make([]Rect, 0, 16),
	}
	manager.initSurface()
	return manager
}

func (manager *Manager) SetRoot(root Component) {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	manager.clearInteractionLocked()
	manager.root = root
	bindTree(root, nil)
	applyTheme(root, manager.theme)
}

func (manager *Manager) Root() Component {
	manager.mu.RLock()
	defer manager.mu.RUnlock()
	return manager.root
}

func (manager *Manager) SetVisible(visible bool) {
	manager.mu.Lock()
	manager.visible = visible
	if !visible {
		manager.clearInteractionLocked()
	}
	manager.mu.Unlock()
}

func (manager *Manager) Visible() bool {
	manager.mu.RLock()
	defer manager.mu.RUnlock()
	return manager.visible
}

func (manager *Manager) SetInteractive(interactive bool) {
	manager.mu.Lock()
	manager.interactive = interactive
	if !interactive {
		manager.clearInteractionLocked()
	}
	manager.mu.Unlock()
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
		return
	}
	manager.notifications = append(manager.notifications, notification{text: text, remaining: seconds})
	if len(manager.notifications) > 4 {
		manager.notifications = manager.notifications[len(manager.notifications)-4:]
	}
	manager.notificationMu.Unlock()
}

func (manager *Manager) Notifyf(format string, args ...any) {
	manager.Notify(fmt.Sprintf(format, args...))
}

func (manager *Manager) ClearNotifications() {
	manager.notificationMu.Lock()
	manager.notifications = manager.notifications[:0]
	manager.notificationMu.Unlock()
}

func (manager *Manager) BeginFrame(width, height int, deltaSeconds float64) {
	manager.ensurePipeline()

	manager.mu.RLock()
	root := manager.root
	visible := manager.visible
	theme := manager.theme
	manager.mu.RUnlock()

	manager.vertices = manager.vertices[:0]
	if !visible || width <= 0 || height <= 0 {
		return
	}
	viewport := Rect{W: float32(width), H: float32(height)}
	manager.clips = append(manager.clips[:0], viewport)
	canvas := Canvas{
		vertices: &manager.vertices,
		limit:    maxVertices,
		clips:    manager.clips,
		theme:    &theme,
	}
	if root != nil && root.UIState().Visible() {
		root.Measure(Constraints{Min: Vec2{X: viewport.W, Y: viewport.H}, Max: Vec2{X: viewport.W, Y: viewport.H}})
		root.Arrange(viewport)
		paintComponent(root, &canvas)
	}
	manager.paintNotifications(&canvas, viewport, deltaSeconds)
	manager.clips = canvas.clips
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
	if state.clip {
		canvas.PopClip()
	}
}

func (manager *Manager) paintNotifications(canvas *Canvas, viewport Rect, deltaSeconds float64) {
	if deltaSeconds < 0 || deltaSeconds > 0.5 {
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
	manager.mu.RLock()
	root := manager.root
	visible := manager.visible
	interactive := manager.interactive
	unhandled := manager.unhandled
	manager.mu.RUnlock()

	if event.IsPointer() {
		manager.pointer = Vec2{X: event.X, Y: event.Y}
	}
	handled := false
	if visible && interactive && root != nil {
		handled = manager.handleTreeEvent(root, event)
	}
	if !handled && unhandled != nil {
		handled = unhandled(event)
	}
	return handled || (visible && interactive && root != nil)
}

func (manager *Manager) handleTreeEvent(root Component, event Event) bool {
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
		return dispatchEvent(hitTest(root, event.X, event.Y), event)
	case TextInput:
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
				return manager.focusNext(root, 1)
			case KeyUpArrow, KeyW:
				return manager.focusNext(root, -1)
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
	}
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

func (manager *Manager) clearInteractionLocked() {
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
}

func (manager *Manager) ClearInteraction() { manager.clearInteractionLocked() }

func (manager *Manager) TextInputFocused() bool {
	_, ok := manager.focused.(*TextField)
	return ok
}

func (manager *Manager) UnhandledEventHandler() func(Event) bool {
	manager.mu.RLock()
	defer manager.mu.RUnlock()
	return manager.unhandled
}
