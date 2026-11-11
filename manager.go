package nativeforms

import (
	"sync"
	"sync/atomic"
	"time"
)

type notification struct {
	text     string
	deadline time.Time
}

type smoothScroller interface {
	stepSmoothScroll(dt float64) bool
}

func stepSmoothTree(component Component, dt float64) bool {
	if component == nil {
		return false
	}
	state := component.UIState()
	if !state.Visible() {
		return false
	}
	active := false
	if scroller, ok := component.(smoothScroller); ok {
		if scroller.stepSmoothScroll(dt) {
			active = true
		}
	}
	for _, child := range state.children {
		if stepSmoothTree(child, dt) {
			active = true
		}
	}
	return active
}

type Frame struct {
	Width, Height int
	Commands      []DrawCommand
}

const (
	managerPoolSize        = 128
	ephemeralManagerWindow = 16
)

var (
	managerPoolMu         sync.Mutex
	managerStorage        [managerPoolSize]Manager
	managerCommandStorage [managerPoolSize][2048]DrawCommand
	freeManagers          []*Manager
	managerPoolOnce       sync.Once
	managerRingIdx        atomic.Uint32
)

func initManagerPool() {
	freeManagers = make([]*Manager, 0, managerPoolSize)
	for i := managerPoolSize - 1; i >= 0; i-- {
		m := &managerStorage[i]
		m.slotIdx = i
		m.pooled = true
		m.inPool = true
		freeManagers = append(freeManagers, m)
	}
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
	backingScale         float32
	focused              Component
	hovered              Component
	captured             Component
	capturedButton       MouseButton
	pointer              Vec2
	consumeUnhandled     bool
	gameNavigation       bool
	dirtyLayout          bool
	dirtyPaint           bool
	smoothActive         bool
	smoothDue            time.Time
	layoutGen            uint64
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

	frame          Frame
	prevCmdCount   int
	clips          []Rect
	clipStorage    [16]Rect
	frameTheme     Theme
	canvas         Canvas
	traceStorage   FrameTrace
	layoutWidth    int
	layoutHeight   int
	framesRendered uint64
	slotIdx        int
	pooled         bool
	inPool         bool
	pinned         bool
}

func (manager *Manager) isEphemeralReclaimable() bool {
	if manager == nil || manager.inPool || manager.pinned || manager.framesRendered == 0 {
		return false
	}
	if manager.interactive || manager.wake != nil ||
		manager.focused != nil || manager.hovered != nil || manager.captured != nil ||
		manager.clipboardRead != nil || manager.clipboardWrite != nil ||
		manager.unhandled != nil || manager.frameObserver != nil ||
		len(manager.popups) > 0 || len(manager.shortcuts) > 0 || len(manager.notifications) > 0 {
		return false
	}
	return manager.root == nil || canAutoRecycleRoot(manager.root)
}

func resetManagerSlot(m *Manager) {
	slotIdx := m.slotIdx
	wasPooled := m.pooled
	var cmds []DrawCommand
	if wasPooled && slotIdx >= 0 && slotIdx < managerPoolSize {
		cmds = managerCommandStorage[slotIdx][:0]
	} else if cap(m.frame.Commands) > 0 {
		cmds = m.frame.Commands[:0]
	}
	*m = Manager{
		visible:      true,
		theme:        DefaultTheme(),
		displayScale: 1,
		backingScale: 1,
		dirtyLayout:  true,
		dirtyPaint:   true,
		layoutGen:    1,
		clock:        time.Now,
		slotIdx:      slotIdx,
		pooled:       wasPooled,
		inPool:       false,
	}
	m.frame.Commands = cmds
	m.clips = m.clipStorage[:0]
}

func New() *Manager {
	var m *Manager
	var oldRoot Component
	managerPoolMu.Lock()
	managerPoolOnce.Do(initManagerPool)
	if len(freeManagers) > 0 {
		n := len(freeManagers)
		m = freeManagers[n-1]
		freeManagers[n-1] = nil
		freeManagers = freeManagers[:n-1]
		resetManagerSlot(m)
	} else {
		for probe := 0; probe < ephemeralManagerWindow; probe++ {
			idx := (managerRingIdx.Add(1) - 1) & (ephemeralManagerWindow - 1)
			cand := &managerStorage[idx]
			if cand.inPool {
				m = cand
				resetManagerSlot(m)
				break
			}
			if cand.isEphemeralReclaimable() {
				oldRoot = cand.root
				cand.root = nil
				m = cand
				resetManagerSlot(m)
				break
			}
		}
	}
	managerPoolMu.Unlock()
	if oldRoot != nil {
		bindTree(oldRoot, nil, nil)
		ReleaseComponent(oldRoot)
	}
	if m != nil {
		return m
	}
	m = &Manager{slotIdx: -1}
	resetManagerSlot(m)
	return m
}

// Pin marks a Manager instance so its preallocated pool slot is never
// automatically reclaimed until Close is called.
func (manager *Manager) Pin() {
	if manager == nil {
		return
	}
	manager.mu.Lock()
	manager.pinned = true
	manager.mu.Unlock()
}

// Close detaches and recycles the root component tree, closes all popups, and
// returns the Manager to the internal pool.
func (manager *Manager) Close() {
	if manager == nil {
		return
	}
	for len(manager.popups) > 0 {
		manager.CloseTopPopup()
	}
	manager.mu.Lock()
	_ = manager.clearInteractionLocked()
	oldRoot := manager.root
	manager.root = nil
	manager.wake = nil
	manager.clipboardRead, manager.clipboardWrite = nil, nil
	manager.unhandled = nil
	manager.frameObserver = nil
	clear(manager.shortcuts)
	manager.mu.Unlock()

	manager.ClearNotifications()
	manager.postMu.Lock()
	manager.pending = nil
	manager.postMu.Unlock()

	if oldRoot != nil {
		bindTree(oldRoot, nil, nil)
		ReleaseComponent(oldRoot)
	}
	manager.clearPinnedCommandPointers(0, max(len(manager.frame.Commands), manager.prevCmdCount))

	managerPoolMu.Lock()
	managerPoolOnce.Do(initManagerPool)
	if manager.inPool {
		managerPoolMu.Unlock()
		return
	}
	manager.inPool = true
	manager.pinned = false
	if manager.pooled && len(freeManagers) < cap(freeManagers) {
		freeManagers = append(freeManagers, manager)
	}
	managerPoolMu.Unlock()
}

func (manager *Manager) now() time.Time {
	if manager.clock != nil {
		return manager.clock()
	}
	return time.Now()
}

func estimateTreeCommands(component Component) int {
	if component == nil {
		return 0
	}
	if _, ok := component.(*ScrollPanel); ok {
		return 512
	}
	count := 2
	if label, ok := component.(*Label); ok {
		count += len(label.Text)
	}
	for _, child := range component.UIState().children {
		count += estimateTreeCommands(child)
	}
	return count
}

func (manager *Manager) SetRoot(root Component) {
	for len(manager.popups) > 0 {
		manager.CloseTopPopup()
	}
	manager.mu.Lock()
	blurred := manager.clearInteractionLocked()
	oldRoot := manager.root
	if oldRoot != nil && !sameComponent(oldRoot, root) {
		bindTree(oldRoot, nil, nil)
	} else {
		oldRoot = nil
	}
	manager.root = root
	bindTree(root, nil, manager)
	applyTheme(root, manager.theme)
	if est := estimateTreeCommands(root); est > cap(manager.frame.Commands) {
		manager.frame.Commands = make([]DrawCommand, 0, est+64)
		manager.prevCmdCount = 0
	}
	manager.mu.Unlock()
	if oldRoot != nil && canAutoRecycleRoot(oldRoot) {
		ReleaseComponent(oldRoot)
	}
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

func (manager *Manager) finishClearInteraction(blurred Component) {
	if blurred != nil {
		blurred.Handle(Event{Type: FocusLost})
		manager.closeUnfocusedPopupsOutside(nil)
	}
	manager.InvalidatePaint()
}

func (manager *Manager) updateActiveFlag(field *bool, value bool) {
	manager.mu.Lock()
	*field = value
	var blurred Component
	if !value {
		blurred = manager.clearInteractionLocked()
	}
	manager.mu.Unlock()
	manager.finishClearInteraction(blurred)
}

func (manager *Manager) SetVisible(visible bool) {
	manager.updateActiveFlag(&manager.visible, visible)
}

func (manager *Manager) Visible() bool {
	manager.mu.RLock()
	defer manager.mu.RUnlock()
	return manager.visible
}

func (manager *Manager) SetInteractive(interactive bool) {
	manager.updateActiveFlag(&manager.interactive, interactive)
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

func (manager *Manager) UnhandledEventHandler() func(Event) bool {
	manager.mu.RLock()
	defer manager.mu.RUnlock()
	return manager.unhandled
}

func (manager *Manager) SetTheme(theme Theme) {
	manager.mu.Lock()
	prev := manager.theme
	manager.theme = theme
	applyTheme(manager.root, theme)
	for _, popup := range manager.popups {
		applyTheme(popup.content, theme)
	}
	manager.mu.Unlock()
	if prev.FontSize != theme.FontSize || prev.ControlHeight != theme.ControlHeight {
		manager.InvalidateLayout()
	} else {
		manager.InvalidatePaint()
	}
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

// SetDisplayScale sets the UI/DPI scale factor (1.0 = 100%, 1.5 = 150%, etc.).
// Desktop backends scale the logical viewport and glyph rasterization accordingly.
func (manager *Manager) SetDisplayScale(scale float32) {
	if !(scale > 0 && scale <= 8) {
		scale = 1
	}
	manager.mu.Lock()
	changed := manager.displayScale != scale
	manager.displayScale = scale
	manager.mu.Unlock()
	if changed {
		manager.InvalidateLayout()
	}
}

func (manager *Manager) DisplayScale() float32 {
	manager.mu.RLock()
	defer manager.mu.RUnlock()
	if !(manager.displayScale > 0 && manager.displayScale <= 8) {
		return 1
	}
	return manager.displayScale
}

// SetBackingScale records the OS monitor pixel-per-point backing scale (such as
// 2.0 on Retina displays) without overwriting the user/application DisplayScale.
func (manager *Manager) SetBackingScale(scale float32) {
	if !(scale > 0 && scale <= 8) {
		scale = 1
	}
	manager.mu.Lock()
	changed := manager.backingScale != scale
	manager.backingScale = scale
	manager.mu.Unlock()
	if changed {
		manager.InvalidateLayout()
	}
}

func (manager *Manager) BackingScale() float32 {
	manager.mu.RLock()
	defer manager.mu.RUnlock()
	if !(manager.backingScale > 0 && manager.backingScale <= 8) {
		return 1
	}
	return manager.backingScale
}

func (manager *Manager) effectiveScaleLocked() float32 {
	ds := manager.displayScale
	if !(ds > 0 && ds <= 8) {
		ds = 1
	}
	bs := manager.backingScale
	if !(bs > 0 && bs <= 8) {
		bs = 1
	}
	return minFloat32(ds*bs, 8)
}

// EffectiveScale returns the combined physical pixel scale (DisplayScale * BackingScale).
func (manager *Manager) EffectiveScale() float32 {
	manager.mu.RLock()
	defer manager.mu.RUnlock()
	return manager.effectiveScaleLocked()
}
