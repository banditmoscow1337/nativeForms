package nativeforms

import (
	"fmt"
	"slices"
	"sync"
)

// Horizontal returns the sum of left and right insets.
func (in Insets) Horizontal() float32 { return in.Left + in.Right }

// Vertical returns the sum of top and bottom insets.
func (in Insets) Vertical() float32 { return in.Top + in.Bottom }

// LocaleSizePolicy controls how a component's geometry adapts when the active
// language changes or when a component is measured across multiple locales.
type LocaleSizePolicy uint8

const (
	// LocaleSizeNone keeps the widget's existing sizing rules (PreferredSize,
	// MinimumSize, Flex, or explicit AutoSize) without locale-specific adjustments.
	LocaleSizeNone LocaleSizePolicy = iota

	// LocaleSizeCurrent resizes the widget to fit the active locale's translated
	// text whenever the locale, font, or display scale changes (via SizeToText).
	LocaleSizeCurrent

	// LocaleSizeMaxEnvelope measures the widget's translated text across all
	// locales registered in LanguageManager using FontContext and sets MinimumSize.X
	// to the widest translation (plus widget chrome/icon padding). This prevents
	// buttons, toolbars, segmented controls, and form label columns from jumping
	// or clipping when switching between compact (e.g. "en") and verbose (e.g. "ru", "de") locales.
	LocaleSizeMaxEnvelope

	// LocaleSizeWrapHeight keeps horizontal constraints intact while enabling
	// multiline wrapping and re-evaluating vertical height for longer translations.
	LocaleSizeWrapHeight
)

// LanguageManager stores translation catalogs for multiple locales, resolves
// localized strings with fallback support, and coordinates automatic widget
// text and size updates across a Manager's component tree.
type LanguageManager struct {
	mu        sync.RWMutex
	active    string
	fallback  string
	order     []string
	catalogs  map[string]map[string]string
	listeners []func(locale string)
	revision  uint64
}

// NewLanguageManager creates a LanguageManager with the given default/fallback
// locale (for example, "en").
func NewLanguageManager(defaultLocale string) *LanguageManager {
	if defaultLocale == "" {
		defaultLocale = "en"
	}
	return &LanguageManager{
		active:   defaultLocale,
		fallback: defaultLocale,
		order:    []string{defaultLocale},
		catalogs: make(map[string]map[string]string, 4),
		revision: 1,
	}
}

// Register adds or merges a key->translation dictionary for the specified locale.
func (lm *LanguageManager) Register(locale string, entries map[string]string) {
	if lm == nil || locale == "" || len(entries) == 0 {
		return
	}
	lm.mu.Lock()
	cat := lm.catalogs[locale]
	if cat == nil {
		cat = make(map[string]string, len(entries))
		lm.catalogs[locale] = cat
	}
	for k, v := range entries {
		cat[k] = v
	}
	if !slices.Contains(lm.order, locale) {
		lm.order = append(lm.order, locale)
	}
	lm.revision++
	lm.mu.Unlock()
}

// SetFallback sets the fallback locale used when a key is missing in the active locale.
func (lm *LanguageManager) SetFallback(locale string) {
	if lm == nil || locale == "" {
		return
	}
	lm.mu.Lock()
	lm.fallback = locale
	lm.revision++
	lm.mu.Unlock()
}

// Locale returns the currently active locale code (e.g. "en", "ru").
func (lm *LanguageManager) Locale() string {
	if lm == nil {
		return ""
	}
	lm.mu.RLock()
	defer lm.mu.RUnlock()
	return lm.active
}

// Locales returns a copy of all registered locale codes in registration order.
func (lm *LanguageManager) Locales() []string {
	if lm == nil {
		return nil
	}
	lm.mu.RLock()
	defer lm.mu.RUnlock()
	return append([]string(nil), lm.order...)
}

// Revision returns a monotonically increasing counter bumped whenever catalogs
// or the active locale change.
func (lm *LanguageManager) Revision() uint64 {
	if lm == nil {
		return 0
	}
	lm.mu.RLock()
	defer lm.mu.RUnlock()
	return lm.revision
}

// SetLocale switches the active locale and notifies registered listeners.
// Returns true if the active locale changed.
func (lm *LanguageManager) SetLocale(locale string) bool {
	if lm == nil || locale == "" {
		return false
	}
	lm.mu.Lock()
	if lm.active == locale {
		lm.mu.Unlock()
		return false
	}
	lm.active = locale
	if !slices.Contains(lm.order, locale) {
		lm.order = append(lm.order, locale)
	}
	lm.revision++
	listeners := append([]func(string)(){}, lm.listeners...)
	lm.mu.Unlock()

	for _, fn := range listeners {
		if fn != nil {
			fn(locale)
		}
	}
	return true
}

// OnLocaleChanged registers a callback invoked whenever SetLocale changes the
// active locale (useful for updating desktop window titles, system tray menus,
// or external state).
func (lm *LanguageManager) OnLocaleChanged(fn func(locale string)) {
	if lm == nil || fn == nil {
		return
	}
	lm.mu.Lock()
	lm.listeners = append(lm.listeners, fn)
	lm.mu.Unlock()
}

// Tr resolves key for the active locale, falling back to the fallback locale,
// and finally returning the key itself if no translation is registered.
// Zero heap allocations on the lookup path.
func (lm *LanguageManager) Tr(key string) string {
	if lm == nil || key == "" {
		return key
	}
	lm.mu.RLock()
	defer lm.mu.RUnlock()
	if cat := lm.catalogs[lm.active]; cat != nil {
		if val, ok := cat[key]; ok {
			return val
		}
	}
	if lm.fallback != "" && lm.fallback != lm.active {
		if cat := lm.catalogs[lm.fallback]; cat != nil {
			if val, ok := cat[key]; ok {
				return val
			}
		}
	}
	return key
}

// TrLocale resolves key for an explicit locale code, falling back to the
// fallback locale and then key.
func (lm *LanguageManager) TrLocale(locale, key string) string {
	if lm == nil || key == "" {
		return key
	}
	lm.mu.RLock()
	defer lm.mu.RUnlock()
	if cat := lm.catalogs[locale]; cat != nil {
		if val, ok := cat[key]; ok {
			return val
		}
	}
	if lm.fallback != "" && lm.fallback != locale {
		if cat := lm.catalogs[lm.fallback]; cat != nil {
			if val, ok := cat[key]; ok {
				return val
			}
		}
	}
	return key
}

// Trf formats a localized template resolved via Tr(key) with fmt.Sprintf.
// Call this on event/state transitions or inside throttled formatters rather
// than on every Paint frame.
func (lm *LanguageManager) Trf(key string, args ...any) string {
	return fmt.Sprintf(lm.Tr(key), args...)
}

// VariantsInto appends all distinct translations of key across all registered
// locales into dst without allocating if cap(dst) is sufficient.
func (lm *LanguageManager) VariantsInto(key string, dst []string) []string {
	dst = dst[:0]
	if lm == nil || key == "" {
		return dst
	}
	lm.mu.RLock()
	defer lm.mu.RUnlock()
	for _, loc := range lm.order {
		if cat := lm.catalogs[loc]; cat != nil {
			if val, ok := cat[key]; ok && val != "" {
				if !slices.Contains(dst, val) {
					dst = append(dst, val)
				}
			}
		}
	}
	if len(dst) == 0 {
		dst = append(dst, key)
	}
	return dst
}

// MeasureKeyMax measures the maximum bounding Vec2 (width X and height Y) of
// all registered locale variants of key using the provided State's FontContext
// and effective FontSpec.
func (lm *LanguageManager) MeasureKeyMax(state *State, key string, spec FontSpec) Vec2 {
	if state == nil || key == "" {
		return Vec2{}
	}
	if lm == nil {
		return state.measureText(key, spec)
	}
	var stackBuf [8]string
	variants := lm.VariantsInto(key, stackBuf[:0])
	var maxSz Vec2
	for _, txt := range variants {
		sz := state.measureText(txt, spec)
		maxSz.X = maxFloat32(maxSz.X, sz.X)
		maxSz.Y = maxFloat32(maxSz.Y, sz.Y)
	}
	return maxSz
}

// ---------------------------------------------------------------------------
// State Localization & Sizing Methods
// ---------------------------------------------------------------------------

func (state *State) TextKey() string                    { return state.textKey }
func (state *State) TooltipKey() string                 { return state.tooltipKey }
func (state *State) PlaceholderKey() string             { return state.placeholderKey }
func (state *State) LocaleSizePolicy() LocaleSizePolicy { return state.localeSizePolicy }

// SetTextKey binds a primary localization key to the component and immediately
// resolves it if the component is attached to a Manager with a LanguageManager.
func (state *State) SetTextKey(key string) {
	if state.textKey == key {
		return
	}
	state.textKey = key
	if state.manager != nil && state.manager.lang != nil && state.owner != nil {
		applyComponentLocale(state.owner, state.manager.lang)
	}
}

// SetTooltipKey binds a localization key for the component's hover tooltip.
func (state *State) SetTooltipKey(key string) {
	if state.tooltipKey == key {
		return
	}
	state.tooltipKey = key
	if state.manager != nil && state.manager.lang != nil {
		state.tooltip = state.manager.lang.Tr(key)
	}
}

// SetPlaceholderKey binds a localization key for a TextField's placeholder.
func (state *State) SetPlaceholderKey(key string) {
	if state.placeholderKey == key {
		return
	}
	state.placeholderKey = key
	if f, ok := state.owner.(*TextField); ok {
		if state.manager != nil && state.manager.lang != nil {
			f.Placeholder = state.manager.lang.Tr(key)
			f.InvalidatePaint()
		}
	}
}

// SetLocaleSizePolicy configures how the component adjusts its bounds when
// translations are applied or measured.
func (state *State) SetLocaleSizePolicy(policy LocaleSizePolicy) {
	if state.localeSizePolicy == policy {
		return
	}
	if !state.baseMinCaptured {
		state.baseMinSize = state.minimum
		state.baseMinCaptured = true
	}
	state.localeSizePolicy = policy
	switch policy {
	case LocaleSizeCurrent:
		state.autoSize = true
	case LocaleSizeMaxEnvelope:
		// Keep autoSize false so Measure respects state.minimum.X computed across all locales.
		state.autoSize = false
	case LocaleSizeWrapHeight:
		if lbl, ok := state.owner.(*Label); ok {
			lbl.Wrap = true
		}
	}
	if state.manager != nil && state.manager.lang != nil && state.owner != nil {
		applyComponentLocale(state.owner, state.manager.lang)
	} else {
		state.invalidateLayout()
	}
}

// SetOnLocaleChange attaches a custom localization hook invoked whenever the
// Manager's active locale changes (ideal for parameterized counters, status
// banners, or custom canvas widgets).
func (state *State) SetOnLocaleChange(fn func(*LanguageManager)) {
	state.onLocaleChange = fn
	if fn != nil && state.manager != nil && state.manager.lang != nil {
		fn(state.manager.lang)
	}
}

// ---------------------------------------------------------------------------
// Manager Integration
// ---------------------------------------------------------------------------

// SetLanguageManager attaches a LanguageManager to the UI Manager and applies
// translations and sizing policies across the root tree and active popups.
func (manager *Manager) SetLanguageManager(lm *LanguageManager) {
	if manager == nil {
		return
	}
	manager.mu.Lock()
	manager.lang = lm
	root := manager.root
	popups := append([]popupEntry(nil), manager.popups...)
	manager.mu.Unlock()
	if lm != nil {
		applyLocale(root, lm)
		for _, popup := range popups {
			applyLocale(popup.content, lm)
		}
	}
	manager.InvalidateLayout()
}

// LanguageManager returns the Manager's bound LanguageManager, if any.
func (manager *Manager) LanguageManager() *LanguageManager {
	if manager == nil {
		return nil
	}
	manager.mu.RLock()
	defer manager.mu.RUnlock()
	return manager.lang
}

// SetLocale switches the active locale on the Manager's LanguageManager and
// updates all localized components, popups, and layout envelopes in a single pass.
func (manager *Manager) SetLocale(locale string) {
	if manager == nil {
		return
	}
	manager.mu.RLock()
	lm := manager.lang
	manager.mu.RUnlock()
	if lm == nil {
		return
	}
	_ = lm.SetLocale(locale)
	manager.RefreshLocale()
}

// Locale returns the active locale code from the Manager's LanguageManager.
func (manager *Manager) Locale() string {
	if manager == nil {
		return ""
	}
	manager.mu.RLock()
	lm := manager.lang
	manager.mu.RUnlock()
	if lm == nil {
		return ""
	}
	return lm.Locale()
}

// Tr translates key using the Manager's LanguageManager, or returns key if none is set.
func (manager *Manager) Tr(key string) string {
	if manager == nil {
		return key
	}
	manager.mu.RLock()
	lm := manager.lang
	manager.mu.RUnlock()
	if lm == nil {
		return key
	}
	return lm.Tr(key)
}

// Trf translates and formats key using the Manager's LanguageManager.
func (manager *Manager) Trf(key string, args ...any) string {
	return fmt.Sprintf(manager.Tr(key), args...)
}

// RefreshLocale re-evaluates all localized strings and LocaleSizePolicy bounds
// across the component tree and open popups (called on SetLocale, SetFontContext,
// SetDisplayScale, and SetTheme).
func (manager *Manager) RefreshLocale() {
	if manager == nil {
		return
	}
	manager.mu.RLock()
	lm := manager.lang
	root := manager.root
	popups := append([]popupEntry(nil), manager.popups...)
	manager.mu.RUnlock()
	if lm != nil {
		applyLocale(root, lm)
		for _, popup := range popups {
			applyLocale(popup.content, lm)
		}
		manager.InvalidateLayout()
	}
}

// ---------------------------------------------------------------------------
// Component Tree Locale Traversal & Multi-Locale Sizing
// ---------------------------------------------------------------------------

func applyLocale(component Component, lm *LanguageManager) {
	if component == nil || lm == nil {
		return
	}
	applyComponentLocale(component, lm)
	state := component.UIState()
	for _, child := range state.children {
		applyLocale(child, lm)
	}
}

func applyComponentLocale(component Component, lm *LanguageManager) {
	if component == nil || lm == nil {
		return
	}
	state := component.UIState()
	if !state.baseMinCaptured {
		state.baseMinSize = state.minimum
		state.baseMinCaptured = true
	}

	if state.tooltipKey != "" {
		state.tooltip = lm.Tr(state.tooltipKey)
	}

	// Translate primary text and composite items by concrete widget type.
	switch w := component.(type) {
	case *Label:
		if state.textKey != "" {
			w.SetText(lm.Tr(state.textKey))
		}
	case *Button:
		if state.textKey != "" {
			w.SetText(lm.Tr(state.textKey))
		}
	case *Toggle:
		if state.textKey != "" {
			w.SetText(lm.Tr(state.textKey))
		}
	case *Checkbox:
		if state.textKey != "" {
			w.SetText(lm.Tr(state.textKey))
		}
	case *Slider:
		if state.textKey != "" {
			w.SetText(lm.Tr(state.textKey))
		}
	case *SpinBox:
		if state.textKey != "" {
			w.SetText(lm.Tr(state.textKey))
		}
	case *ProgressBar:
		if state.textKey != "" {
			w.Text = lm.Tr(state.textKey)
		}
	case *ColorPicker:
		if state.textKey != "" {
			w.SetText(lm.Tr(state.textKey))
		}
	case *Separator:
		if state.textKey != "" {
			w.SetText(lm.Tr(state.textKey))
		}
	case *Hyperlink:
		if state.textKey != "" {
			w.SetText(lm.Tr(state.textKey))
		}
	case *Badge:
		if state.textKey != "" {
			w.SetText(lm.Tr(state.textKey))
		}
	case *Collapsible:
		if state.textKey != "" {
			w.SetTitle(lm.Tr(state.textKey))
		}
	case *TextField:
		if state.textKey != "" {
			w.SetText(lm.Tr(state.textKey))
		}
		if state.placeholderKey != "" {
			w.Placeholder = lm.Tr(state.placeholderKey)
		}
	case *RadioGroup:
		translateRadioOptions(w.Options, lm)
	case *SegmentedControl:
		translateRadioOptions(w.Options, lm)
	case *ComboBox:
		translateRadioOptions(w.Options, lm)
	case *MenuBar:
		for i := range w.Menus {
			if w.Menus[i].LabelKey != "" {
				w.Menus[i].Label = lm.Tr(w.Menus[i].LabelKey)
			}
			translateMenuItems(w.Menus[i].Items, lm)
		}
	case *Menu:
		translateMenuItems(w.Items, lm)
	case *Toolbar:
		for i := range w.Items {
			if w.Items[i].LabelKey != "" {
				w.Items[i].Label = lm.Tr(w.Items[i].LabelKey)
			}
			if w.Items[i].TooltipKey != "" {
				w.Items[i].Tooltip = lm.Tr(w.Items[i].TooltipKey)
			}
		}
	case *Tabs:
		for i := range w.Tabs {
			if w.Tabs[i].TitleKey != "" {
				w.Tabs[i].Title = lm.Tr(w.Tabs[i].TitleKey)
			}
		}
	}

	if len(state.contextMenu) > 0 {
		translateMenuItems(state.contextMenu, lm)
	}

	if state.onLocaleChange != nil {
		state.onLocaleChange(lm)
	}

	// Apply locale-driven geometry sizing policy.
	switch state.localeSizePolicy {
	case LocaleSizeCurrent:
		state.SizeToText()
	case LocaleSizeMaxEnvelope:
		env := measureMaxLocaleEnvelope(component, lm)
		if env.X > 0 {
			state.minimum.X = maxFloat32(state.baseMinSize.X, env.X)
		}
		if env.Y > 0 {
			state.minimum.Y = maxFloat32(state.baseMinSize.Y, env.Y)
		}
	case LocaleSizeWrapHeight:
		if lbl, ok := component.(*Label); ok {
			lbl.Wrap = true
			lbl.cachedMode = 0
		}
	}
}

func translateRadioOptions(options []RadioOption, lm *LanguageManager) {
	for i := range options {
		if options[i].LabelKey != "" {
			options[i].Label = lm.Tr(options[i].LabelKey)
		}
	}
}

func translateMenuItems(items []MenuItem, lm *LanguageManager) {
	for i := range items {
		if items[i].LabelKey != "" {
			items[i].Label = lm.Tr(items[i].LabelKey)
		}
	}
}

// measureMaxLocaleEnvelope computes the bounding Vec2 required to fit a widget
// across all locales registered in lm without clipping or layout jitter.
func measureMaxLocaleEnvelope(component Component, lm *LanguageManager) Vec2 {
	if component == nil || lm == nil {
		return Vec2{}
	}
	state := component.UIState()
	theme := state.Theme()
	defaultSpec := FontSpec{Size: theme.FontSize}

	switch w := component.(type) {
	case *Label:
		key := state.textKey
		if key == "" {
			return w.measureTextSize()
		}
		spec := w.fontSpec()
		sz := lm.MeasureKeyMax(state, key, spec)
		return Vec2{
			X: sz.X + w.Padding.Left + w.Padding.Right,
			Y: sz.Y + w.Padding.Top + w.Padding.Bottom,
		}

	case *Button:
		key := state.textKey
		if key == "" {
			return w.measureTextSize()
		}
		spec := w.fontSpec()
		sz := lm.MeasureKeyMax(state, key, spec)
		desired := Vec2{
			X: sz.X + 36 + iconWidth(w.Icon),
			Y: maxFloat32(sz.Y+16, theme.ControlHeight),
		}
		if w.Icon != nil {
			desired.Y = maxFloat32(desired.Y, iconSize+12)
		}
		return desired

	case *Toggle:
		key := state.textKey
		if key == "" {
			return w.measureTextSize()
		}
		sz := lm.MeasureKeyMax(state, key, defaultSpec)
		return Vec2{
			X: sz.X + 86 + iconWidth(w.Icon),
			Y: maxFloat32(sz.Y+16, theme.ControlHeight),
		}

	case *Checkbox:
		key := state.textKey
		if key == "" {
			return w.measureTextSize()
		}
		sz := lm.MeasureKeyMax(state, key, defaultSpec)
		return Vec2{
			X: sz.X + 34 + iconWidth(w.Icon),
			Y: maxFloat32(sz.Y+12, theme.ControlHeight),
		}

	case *Badge:
		key := state.textKey
		if key == "" {
			return w.measureTextSize()
		}
		spec := w.fontSpec()
		sz := lm.MeasureKeyMax(state, key, spec)
		h := maxFloat32(22, sz.Y+8)
		return Vec2{
			X: maxFloat32(h, sz.X+16),
			Y: h,
		}

	case *Hyperlink:
		key := state.textKey
		if key == "" {
			return w.measureTextSize()
		}
		sz := lm.MeasureKeyMax(state, key, defaultSpec)
		return Vec2{X: sz.X + 8, Y: maxFloat32(sz.Y+8, theme.FontSize*1.4)}

	case *SpinBox:
		key := state.textKey
		if key == "" {
			return w.measureTextSize()
		}
		sz := lm.MeasureKeyMax(state, key, defaultSpec)
		valW := state.textWidth(w.FormattedValue(), defaultSpec)
		height := maxFloat32(theme.ControlHeight, theme.FontSize*1.2+14)
		btnW := minFloat32(32, maxFloat32(22, height*0.8))
		return Vec2{
			X: maxFloat32(120, sz.X+16+valW+btnW*2+28),
			Y: height,
		}

	case *Slider:
		key := state.textKey
		if key == "" {
			return w.measureTextSize()
		}
		sz := lm.MeasureKeyMax(state, key, defaultSpec)
		valW := state.textWidth(w.formattedValue(), defaultSpec)
		return Vec2{
			X: maxFloat32(120, sz.X+valW+44),
			Y: maxFloat32(58, maxFloat32(theme.ControlHeight+14, theme.FontSize*1.2+30)),
		}

	case *ColorPicker:
		key := state.textKey
		if key == "" {
			return w.measureTextSize()
		}
		sz := lm.MeasureKeyMax(state, key, defaultSpec)
		base := w.measureTextSize()
		curTitleW := state.textWidth(w.Text, defaultSpec)
		deltaX := maxFloat32(0, sz.X-curTitleW)
		return Vec2{X: base.X + deltaX, Y: base.Y}

	case *TextField:
		var maxW float32
		spec := w.fontSpec()
		if state.textKey != "" {
			maxW = maxFloat32(maxW, lm.MeasureKeyMax(state, state.textKey, spec).X)
		}
		if state.placeholderKey != "" {
			maxW = maxFloat32(maxW, lm.MeasureKeyMax(state, state.placeholderKey, spec).X)
		}
		if maxW <= 0 {
			return w.measureTextSize()
		}
		return Vec2{
			X: maxFloat32(48, maxW+24),
			Y: maxFloat32(theme.ControlHeight, w.lineHeight(spec)+12),
		}

	case *SegmentedControl:
		maxSegW := float32(48)
		for _, opt := range w.Options {
			optW := measureRadioOptionMax(state, opt, defaultSpec, lm) + 24 + iconWidth(opt.Icon)
			maxSegW = maxFloat32(maxSegW, optW)
		}
		totalW := float32(8) + maxSegW*float32(len(w.Options))
		return Vec2{X: totalW, Y: maxFloat32(34, theme.ControlHeight)}

	case *RadioGroup:
		maxOptW := float32(0)
		iconSpace := float32(0)
		if hasRadioOptionIcons(w.Options) {
			iconSpace = iconSize + iconGap
		}
		for _, opt := range w.Options {
			optW := measureRadioOptionMax(state, opt, defaultSpec, lm) + 38 + iconSpace
			maxOptW = maxFloat32(maxOptW, optW)
		}
		return Vec2{X: maxOptW, Y: float32(len(w.Options)) * theme.ControlHeight}

	case *ComboBox:
		maxOptW := float32(0)
		iconSpace := float32(0)
		if hasRadioOptionIcons(w.Options) {
			iconSpace = iconSize + iconGap
		}
		for _, opt := range w.Options {
			optW := measureRadioOptionMax(state, opt, defaultSpec, lm) + 54 + iconSpace
			maxOptW = maxFloat32(maxOptW, optW)
		}
		return Vec2{X: maxOptW, Y: theme.ControlHeight}
	}

	if m, ok := component.(textSizeMeasurer); ok {
		return m.measureTextSize()
	}
	return Vec2{}
}

func measureRadioOptionMax(state *State, opt RadioOption, spec FontSpec, lm *LanguageManager) float32 {
	if opt.LabelKey != "" && lm != nil {
		return lm.MeasureKeyMax(state, opt.LabelKey, spec).X
	}
	return state.measureText(opt.Label, spec).X
}

// ---------------------------------------------------------------------------
// Localized Widget Constructors & Form Helpers
// ---------------------------------------------------------------------------

// NewLabelKey creates a Label bound to a localization key.
func NewLabelKey(key string) *Label {
	lbl := NewLabel(key)
	lbl.textKey = key
	return lbl
}

// NewButtonKey creates a compact Button bound to a localization key with
// LocaleSizeMaxEnvelope enabled by default so its width fits all locales.
func NewButtonKey(key string, onClick func()) *Button {
	btn := NewCompactButton(key, onClick)
	btn.textKey = key
	btn.localeSizePolicy = LocaleSizeMaxEnvelope
	btn.baseMinCaptured = true
	return btn
}

// NewToggleKey creates a Toggle bound to a localization key.
func NewToggleKey(key string, value bool, onChange func(bool)) *Toggle {
	t := NewToggle(key, value, onChange)
	t.textKey = key
	return t
}

// NewCheckboxKey creates a Checkbox bound to a localization key.
func NewCheckboxKey(key string, checked bool, onChange func(bool)) *Checkbox {
	c := NewCheckbox(key, checked, onChange)
	c.textKey = key
	return c
}

// NewLocalizedForm creates a 2-column form Grid whose label column uses
// TrackAuto and automatically sizes each row's label using LocaleSizeMaxEnvelope
// so form controls stay aligned across language switches.
func NewLocalizedForm() *Grid {
	form := NewForm(0) // Column 0 = TrackAuto, Column 1 = TrackStar(1)
	form.ColumnGap = 8
	form.RowGap = 6
	return form
}

// AddLocalizedRow adds a localized label (by key) and control to a Grid/Form,
// configuring the label with LocaleSizeMaxEnvelope so column 0 reserves space
// for the widest registered translation.
func (g *Grid) AddLocalizedRow(labelKey string, control Component) *Label {
	lbl := NewLabelKey(labelKey)
	lbl.SetLocaleSizePolicy(LocaleSizeMaxEnvelope)
	g.AddRow(lbl, control)
	return lbl
}