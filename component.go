package nativeforms

type Component interface {
	UIState() *State
	Measure(Constraints) Vec2
	Arrange(Rect)
	Paint(*Canvas)
	Handle(Event) bool
}

// ViewportCuller may skip painting its entire subtree for an effective clip.
// Cull returns true only when neither the component nor its children or
// overlay can produce pixels inside viewport.
type ViewportCuller interface {
	Cull(viewport Rect) bool
}

// textSizeMeasurer is implemented by text-bearing widgets to report their
// natural size fitted tightly to their current text content.
type textSizeMeasurer interface {
	measureTextSize() Vec2
}

type State struct {
	owner    Component
	parent   Component
	manager  *Manager
	children []Component
	rect     Rect
	theme    Theme
	themed   bool

	preferred    Vec2
	minimum      Vec2
	maximum      Vec2
	margin       Insets
	offset       Vec2
	anchor       Anchor
	flex         float32
	basis        float32
	shrink       float32
	shrinkSet    bool
	cornerRadius float32
	autoSize     bool

	hidden    bool
	disabled  bool
	focusable bool
	clip      bool
	frameless bool
	hovered   bool
	pressed   bool
	focused   bool

	onEvent         func(Event) bool
	contextMenu     []MenuItem
	contextMenuFunc func(x, y float32) []MenuItem
	tooltip         string
	accessibleID    AccessibleID
	accessibility   AccessibilityInfo
}

func (state *State) UIState() *State { return state }

func (state *State) fontContext() *FontContext {
	if state.manager == nil {
		return nil
	}
	return state.manager.FontContext()
}

func (state *State) effectiveFontSpec(spec FontSpec) FontSpec {
	if spec.Scale <= 0 && state.manager != nil {
		if s := state.manager.effectiveScaleLocked(); s > 0 {
			spec.Scale = s
		}
	}
	return spec
}

func (state *State) textWidth(text string, spec FontSpec) float32 {
	if fonts := state.fontContext(); fonts != nil {
		return fonts.Advance(text, state.effectiveFontSpec(spec))
	}
	return measureTextLine(text, spec.Size)
}

func (state *State) measureText(text string, spec FontSpec) Vec2 {
	if fonts := state.fontContext(); fonts != nil {
		return fonts.Measure(text, state.effectiveFontSpec(spec))
	}
	return MeasureText(text, spec.Size)
}

func (state *State) caretX(text string, spec FontSpec, index int) float32 {
	if fonts := state.fontContext(); fonts != nil {
		return fonts.CaretX(text, state.effectiveFontSpec(spec), index)
	}
	return TextCaretX(text, spec.Size, index)
}

func (state *State) caretPositions(text string, spec FontSpec) []float32 {
	if fonts := state.fontContext(); fonts != nil {
		return fonts.layoutSharedCarets(text, state.effectiveFontSpec(spec)).Carets
	}
	return textCaretPositions(text, spec.Size)
}

func (state *State) caretPositionsInto(text string, spec FontSpec, dst []float32) []float32 {
	if fonts := state.fontContext(); fonts != nil {
		return fonts.CaretPositionsInto(text, state.effectiveFontSpec(spec), dst)
	}
	return textCaretPositionsInto(text, spec.Size, dst)
}

func (state *State) wrapText(text string, width float32, spec FontSpec) string {
	if state.fontContext() != nil {
		return wrapTextWith(text, width, spec.Size, func(s string, _ float32) float32 { return state.textWidth(s, spec) })
	}
	return WrapText(text, width, spec.Size)
}

func (state *State) ellipsizeText(text string, width float32, spec FontSpec) string {
	if fonts := state.fontContext(); fonts != nil {
		return fonts.Ellipsize(text, width, state.effectiveFontSpec(spec))
	}
	return ellipsizeTextWith(text, width, func(s string) float32 { return measureTextLine(s, spec.Size) }, "…")
}

func (state *State) Measure(constraints Constraints) Vec2 {
	return measureWithState(state, state.preferred, constraints)
}

func (state *State) Arrange(rect Rect)    { state.rect = rect }
func (state *State) Paint(canvas *Canvas) {}

func (state *State) Handle(event Event) bool {
	if state.onEvent != nil && state.onEvent(event) {
		return true
	}
	if event.Type == PointerDown && event.Button == MouseRight {
		return state.OpenContextMenuAt(event.X, event.Y)
	}
	return false
}

// SetContextMenu attaches a static right-click context menu to the component.
func (state *State) SetContextMenu(items []MenuItem) {
	if len(items) == 0 {
		state.contextMenu = nil
		return
	}
	state.contextMenu = append(state.contextMenu[:0], items...)
}

// ContextMenu returns a copy of the component's static context menu items.
func (state *State) ContextMenu() []MenuItem {
	return append([]MenuItem(nil), state.contextMenu...)
}

// SetContextMenuBuilder attaches a dynamic builder invoked on right-click (or
// programmatic context menu activation) with the pointer coordinates.
func (state *State) SetContextMenuBuilder(fn func(x, y float32) []MenuItem) {
	state.contextMenuFunc = fn
}

// ContextMenuBuilder returns the dynamic context menu builder, if any.
func (state *State) ContextMenuBuilder() func(x, y float32) []MenuItem {
	return state.contextMenuFunc
}

func (state *State) buildContextMenuItems(x, y float32) []MenuItem {
	if state.contextMenuFunc != nil {
		return state.contextMenuFunc(x, y)
	}
	return state.contextMenu
}

// OpenContextMenuAt opens the component's configured context menu at (x, y).
func (state *State) OpenContextMenuAt(x, y float32) bool {
	if state == nil || state.manager == nil || !state.Enabled() {
		return false
	}
	items := state.buildContextMenuItems(x, y)
	if len(items) == 0 {
		return false
	}
	return state.manager.OpenContextMenu(items, x, y) != 0
}

func (state *State) Add(children ...Component) {
	for _, child := range children {
		if child == nil || child.UIState() == state || componentContainsState(child, state) {
			continue
		}
		childState := child.UIState()
		if childState.parent != nil {
			childState.parent.UIState().Remove(child)
		}
		childState.parent = state.owner
		state.children = append(state.children, child)
		bindTree(child, state.owner, state.manager)
		if state.themed {
			applyTheme(child, state.theme)
		}
	}
	if state.manager != nil {
		if panel, ok := state.owner.(*Panel); ok {
			panel.ensureLayoutCapacity(len(panel.children))
		}
	}
	state.invalidateLayout()
}

func componentContainsState(component Component, target *State) bool {
	if component == nil {
		return false
	}
	state := component.UIState()
	if state == target {
		return true
	}
	for _, child := range state.children {
		if componentContainsState(child, target) {
			return true
		}
	}
	return false
}

func (state *State) Remove(child Component) bool {
	for index, candidate := range state.children {
		if child == nil || candidate.UIState() != child.UIState() {
			continue
		}
		copy(state.children[index:], state.children[index+1:])
		state.children[len(state.children)-1] = nil
		state.children = state.children[:len(state.children)-1]
		bindTree(child, nil, nil)
		if state.manager != nil {
			state.manager.clearSubtreeInteraction(child.UIState())
		}
		state.invalidateLayout()
		return true
	}
	return false
}

func (state *State) ClearChildren() {
	children := state.children
	state.children = nil
	for _, child := range children {
		if child != nil {
			bindTree(child, nil, nil)
			if state.manager != nil {
				state.manager.clearSubtreeInteraction(child.UIState())
			}
		}
	}
	state.invalidateLayout()
}

func (state *State) Children() []Component {
	return append([]Component(nil), state.children...)
}

// ChildCount returns the number of direct child components without allocating.
func (state *State) ChildCount() int {
	return len(state.children)
}

// ChildAt returns the direct child component at index, or nil if out of range.
func (state *State) ChildAt(index int) Component {
	if index < 0 || index >= len(state.children) {
		return nil
	}
	return state.children[index]
}

// ReleaseComponent detaches and recycles a component subtree that is no longer
// mounted in a parent or attached to an active Manager, returning pooled
// widgets (such as TextField and CodeEditor) and unreferenced initial documents
// back to their internal pools.
func ReleaseComponent(component Component) {
	if component == nil {
		return
	}
	state := component.UIState()
	if state == nil || state.parent != nil || state.manager != nil {
		return
	}
	releaseDetachedSubtree(component)
}

func releaseDetachedSubtree(component Component) {
	if component == nil {
		return
	}
	state := component.UIState()
	if state == nil || state.manager != nil {
		return
	}
	children := state.children
	state.children = nil
	for _, child := range children {
		if child != nil {
			cs := child.UIState()
			if cs != nil {
				cs.parent = nil
				cs.manager = nil
			}
			releaseDetachedSubtree(child)
		}
	}
	switch w := component.(type) {
	case *TextField:
		recycleTextField(w)
	case *CodeEditor:
		recycleCodeEditor(w)
	}
}

func canAutoRecycleRoot(component Component) bool {
	if component == nil {
		return false
	}
	state := component.UIState()
	if state == nil || state.parent != nil || state.manager != nil || state.onEvent != nil || len(state.children) > 0 {
		return false
	}
	switch w := component.(type) {
	case *TextField:
		return w.OnChange == nil && w.OnSubmit == nil && w.Validate == nil &&
			(w.editor == nil || w.editor.version == 0)
	case *CodeEditor:
		return w.OnChange == nil && w.OnBreakpointChange == nil &&
			w.OnGoToDefinition == nil && w.ContextMenuItems == nil &&
			len(w.breakpoints) == 0 && len(w.diagnostics) == 0 &&
			(w.Document == nil || w.Document.Version() <= 1)
	default:
		return false
	}
}
