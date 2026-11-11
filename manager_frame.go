package nativeforms

import "time"

func (manager *Manager) clearPinnedCommandPointers(from, to int) {
	cmds := manager.frame.Commands[:cap(manager.frame.Commands)]
	to = min(to, len(cmds))
	for i := from; i < to; i++ {
		if cmds[i].Image != nil {
			cmds[i].Image = nil
		}
		if cmds[i].Mask != nil {
			cmds[i].Mask = nil
		}
	}
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
	manager.frameTheme = manager.theme
	fonts := manager.fonts
	displayScale := manager.effectiveScaleLocked()
	layoutNeeded := manager.dirtyLayout || width != manager.layoutWidth || height != manager.layoutHeight
	if layoutNeeded && !manager.dirtyLayout {
		manager.layoutGen++
	}
	manager.dirtyLayout, manager.dirtyPaint = false, false
	manager.framesRendered++
	manager.mu.Unlock()
	manager.frameTrace = manager.initFrameTrace(observer, observerOptions, width, height)
	trace := manager.frameTrace
	if trace != nil && fonts != nil {
		trace.fonts = fonts
		_, _, trace.startHits, trace.startMisses = fonts.LayoutCacheStats()
	}
	buildStart := trace.Start()

	oldCount := len(manager.frame.Commands)
	// Clear any Image pointers up front so even commands overwritten by non-image
	// draws or beyond the new frame length never pin discarded ImageResources.
	for i := 0; i < oldCount; i++ {
		if manager.frame.Commands[i].Image != nil {
			manager.frame.Commands[i].Image = nil
		}
	}
	manager.frame.Width, manager.frame.Height = width, height
	manager.frame.Commands = manager.frame.Commands[:0]
	if !visible || width <= 0 || height <= 0 {
		manager.clearPinnedCommandPointers(0, max(oldCount, manager.prevCmdCount))
		manager.prevCmdCount = 0
		trace.countCommands(manager.frame)
		trace.Record(PhaseBuild, buildStart)
		return
	}
	viewport := Rect{W: float32(width), H: float32(height)}
	manager.clips = append(manager.clips[:0], viewport)
	manager.canvas = Canvas{
		commands:     &manager.frame.Commands,
		clips:        manager.clips,
		theme:        &manager.frameTheme,
		fonts:        fonts,
		displayScale: displayScale,
		trace:        trace,
	}
	canvas := &manager.canvas
	animating := false
	if root != nil && root.UIState().Visible() {
		if layoutNeeded {
			layoutStart := trace.Start()
			root.Measure(Constraints{Min: Vec2{X: viewport.W, Y: viewport.H}, Max: Vec2{X: viewport.W, Y: viewport.H}})
			root.Arrange(viewport)
			trace.Record(PhaseLayout, layoutStart)
		}
		if stepSmoothTree(root, deltaSeconds) {
			animating = true
		}
		paintStart := trace.Start()
		paintComponent(root, canvas)
		trace.Record(PhasePaint, paintStart)
	}
	popupStart := trace.Start()
	manager.layoutPopups(viewport)
	trace.Record(PhaseLayout, popupStart)
	paintStart := trace.Start()
	for _, entry := range manager.popups {
		if stepSmoothTree(entry.content, deltaSeconds) {
			animating = true
		}
		if entry.modal {
			canvas.Rect(viewport, manager.frameTheme.Scrim)
		}
		paintComponent(entry.content, canvas)
	}
	manager.paintNotifications(canvas, viewport, now)
	trace.Record(PhasePaint, paintStart)
	manager.mu.Lock()
	manager.smoothActive = animating
	if animating {
		manager.smoothDue = now.Add(8 * time.Millisecond)
		// Clear dirtyPaint caused by stepSmoothTree so stationary checks remain clean,
		// while NextFrameDeadline drives the 120Hz smooth animation frames.
		manager.dirtyPaint = false
	} else {
		manager.smoothDue = time.Time{}
	}
	manager.mu.Unlock()
	manager.clips = canvas.clips
	manager.layoutWidth, manager.layoutHeight = width, height
	newCount := len(manager.frame.Commands)
	if oldCount > newCount {
		manager.clearPinnedCommandPointers(newCount, oldCount)
	}
	manager.prevCmdCount = newCount
	trace.countCommands(manager.frame)
	trace.Record(PhaseBuild, buildStart)
}

func (manager *Manager) Frame() Frame { return manager.frame }

func (manager *Manager) InvalidateLayout() {
	manager.mu.Lock()
	wasDirty := manager.dirtyLayout && manager.dirtyPaint
	manager.dirtyLayout, manager.dirtyPaint = true, true
	manager.layoutGen++
	wake := manager.wake
	manager.mu.Unlock()
	if !wasDirty && wake != nil {
		wake()
	}
}

func (manager *Manager) InvalidatePaint() {
	manager.mu.Lock()
	wasDirty := manager.dirtyPaint
	manager.dirtyPaint = true
	wake := manager.wake
	manager.mu.Unlock()
	if !wasDirty && wake != nil {
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
	smoothActive := manager.smoothActive
	smoothDue := manager.smoothDue
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
	if smoothActive && !smoothDue.IsZero() && (deadline.IsZero() || smoothDue.Before(deadline)) {
		deadline = smoothDue
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
