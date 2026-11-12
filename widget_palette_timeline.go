package nativeforms

import "math"

// SwatchGrid renders a scrollable multi-column grid of color swatches with
// hover outlines, active selection ring, optional center indicator dot, and
// keyboard navigation.
type SwatchGrid struct {
	State
	Colors        []Color
	Selected      Color
	Columns       int
	CellSize      float32
	Gap           float32
	ScrollY       float32
	ShowActiveDot bool
	OnSelect      func(index int, color Color)

	hoveredIdx int
}

func NewSwatchGrid(colors []Color, selected Color, onSelect func(index int, color Color)) *SwatchGrid {
	sg := &SwatchGrid{
		Colors:        append([]Color(nil), colors...),
		Selected:      selected,
		Columns:       8,
		CellSize:      24,
		Gap:           5,
		ShowActiveDot: true,
		OnSelect:      onSelect,
		hoveredIdx:    -1,
	}
	sg.owner = sg
	sg.focusable = true
	sg.clip = true
	return sg
}

func (sg *SwatchGrid) SetColors(colors []Color) {
	sg.Colors = append(sg.Colors[:0], colors...)
	sg.hoveredIdx = -1
	sg.invalidateLayout()
}

func (sg *SwatchGrid) SetSelected(color Color) {
	if sameRGB(sg.Selected, color) && sg.Selected.A == color.A {
		return
	}
	sg.Selected = color
	sg.InvalidatePaint()
}

func (sg *SwatchGrid) SelectedIndex() int {
	for i, c := range sg.Colors {
		if sameRGB(c, sg.Selected) {
			return i
		}
	}
	return -1
}

func (sg *SwatchGrid) cols() int {
	if sg.Columns > 0 {
		return sg.Columns
	}
	return 8
}

func (sg *SwatchGrid) cellMetrics(width float32) (cols int, cell float32, gap float32, rows int, contentH float32) {
	cols = sg.cols()
	gap = sg.Gap
	if gap < 0 {
		gap = 4
	}
	cell = sg.CellSize
	if cell <= 0 {
		cell = 24
	}
	if width > 0 {
		avail := maxFloat32(0, width-float32(cols-1)*gap-4)
		if avail > 0 {
			cell = maxFloat32(14, minFloat32(32, avail/float32(cols)))
		}
	}
	n := len(sg.Colors)
	rows = (n + cols - 1) / cols
	if rows < 1 {
		rows = 1
	}
	contentH = float32(rows)*cell + float32(max(0, rows-1))*gap + 4
	return cols, cell, gap, rows, contentH
}

func (sg *SwatchGrid) Measure(constraints Constraints) Vec2 {
	w := constraints.Max.X
	if !constraints.HasMaxX() || w <= 0 {
		w = float32(sg.cols())*24 + float32(sg.cols()-1)*5 + 4
	}
	_, _, _, _, contentH := sg.cellMetrics(w)
	desiredH := minFloat32(175, maxFloat32(32, contentH))
	return measureWithState(&sg.State, Vec2{X: w, Y: desiredH}, constraints)
}

func (sg *SwatchGrid) maxScrollY() float32 {
	_, _, _, _, contentH := sg.cellMetrics(sg.rect.W)
	return maxFloat32(0, contentH-sg.rect.H)
}

func (sg *SwatchGrid) indexAt(x, y float32) int {
	if !sg.rect.Contains(x, y) || len(sg.Colors) == 0 {
		return -1
	}
	cols, cell, gap, _, _ := sg.cellMetrics(sg.rect.W)
	stride := cell + gap
	relX := x - (sg.rect.X + 2)
	relY := y - (sg.rect.Y + 2) + sg.ScrollY
	if relX < 0 || relY < 0 {
		return -1
	}
	col := int(relX / stride)
	row := int(relY / stride)
	if col < 0 || col >= cols || relX-float32(col)*stride > cell || relY-float32(row)*stride > cell {
		return -1
	}
	idx := row*cols + col
	if idx < 0 || idx >= len(sg.Colors) {
		return -1
	}
	return idx
}

func (sg *SwatchGrid) Paint(canvas *Canvas) {
	if sg.rect.Empty() {
		return
	}
	sg.ScrollY = clampFloat32(sg.ScrollY, 0, sg.maxScrollY())
	cols, cell, gap, _, _ := sg.cellMetrics(sg.rect.W)
	stride := cell + gap
	radius := minFloat32(4, sg.effectiveCornerRadius(canvas.theme))

	canvas.PushClip(sg.rect)
	defer canvas.PopClip()

	for i, c := range sg.Colors {
		col := i % cols
		row := i / cols
		x := sg.rect.X + 2 + float32(col)*stride
		y := sg.rect.Y + 2 + float32(row)*stride - sg.ScrollY
		if y+cell < sg.rect.Y || y > sg.rect.Y+sg.rect.H {
			continue
		}
		r := Rect{X: x, Y: y, W: cell, H: cell}
		canvas.RoundedRect(r, radius, c)
		chosen := sameRGB(c, sg.Selected)
		if chosen {
			canvas.RoundedBorder(r.Inset(All(-1)), radius+1, 2, RGBA(233, 243, 246, 255))
			if sg.ShowActiveDot {
				dot := Rect{X: r.X + r.W*0.5 - 2.5, Y: r.Y + r.H*0.5 - 2.5, W: 5, H: 5}
				canvas.RoundedRect(dot.Inset(All(-1)), 3.5, RGBA(0, 0, 0, 140))
				canvas.RoundedRect(dot, 2.5, RGBA(255, 255, 255, 255))
			}
		} else if i == sg.hoveredIdx && sg.Hovered() && sg.Enabled() {
			canvas.RoundedBorder(r, radius, 2, RGBA(255, 255, 255, 255))
		} else {
			canvas.RoundedBorder(r, radius, 1, RGBA(255, 255, 255, 28))
		}
	}
}

func (sg *SwatchGrid) selectIndex(idx int) bool {
	if idx < 0 || idx >= len(sg.Colors) {
		return false
	}
	c := sg.Colors[idx]
	sg.Selected = c
	sg.InvalidatePaint()
	if sg.OnSelect != nil {
		sg.OnSelect(idx, c)
	}
	return true
}

func (sg *SwatchGrid) Handle(event Event) bool {
	if !sg.Enabled() || len(sg.Colors) == 0 {
		return false
	}
	switch event.Type {
	case PointerMove:
		idx := sg.indexAt(event.X, event.Y)
		if idx != sg.hoveredIdx {
			sg.hoveredIdx = idx
			sg.InvalidatePaint()
		}
		return idx >= 0
	case PointerDown:
		if event.Button == MouseLeft && sg.rect.Contains(event.X, event.Y) {
			if idx := sg.indexAt(event.X, event.Y); idx >= 0 {
				return sg.selectIndex(idx)
			}
			return true
		}
	case PointerScroll:
		maxY := sg.maxScrollY()
		if maxY <= 0 {
			return false
		}
		next := clampFloat32(sg.ScrollY-event.Scroll.Y*28, 0, maxY)
		if next != sg.ScrollY {
			sg.ScrollY = next
			sg.InvalidatePaint()
			return true
		}
	case KeyDown:
		cur := max(0, sg.SelectedIndex())
		cols := sg.cols()
		switch event.Key {
		case KeyRightArrow:
			return sg.selectIndex(min(len(sg.Colors)-1, cur+1))
		case KeyLeftArrow:
			return sg.selectIndex(max(0, cur-1))
		case KeyDownArrow:
			return sg.selectIndex(min(len(sg.Colors)-1, cur+cols))
		case KeyUpArrow:
			return sg.selectIndex(max(0, cur-cols))
		}
	}
	return sg.State.Handle(event)
}

// LayerItem describes a single slice/layer row inside a LayerList widget.
type LayerItem struct {
	ID        string
	Name      string
	Meta      string
	Visible   bool
	Locked    bool
	Opacity   float32
	Thumbnail *ImageResource
}

// LayerList renders a scrollable stack of sprite/document layers from top to
// bottom (or in custom order) with per-row visibility toggle, 32x32 pixel
// thumbnail over a checkerboard background, two-line name/meta info, and lock toggle.
type LayerList struct {
	State
	Items          []LayerItem
	SelectedIndex  int
	RowHeight      float32
	ScrollY        float32
	OnSelect       func(index int)
	OnToggleVis    func(index int)
	OnToggleLock   func(index int)
	hoveredRow     int
	hoveredSubZone uint8 // 0=row, 1=eye, 2=lock
}

func NewLayerList(onSelect func(index int)) *LayerList {
	ll := &LayerList{
		RowHeight:  48,
		OnSelect:   onSelect,
		hoveredRow: -1,
	}
	ll.owner = ll
	ll.focusable = true
	ll.clip = true
	ll.minimum = Vec2{X: 160, Y: 80}
	return ll
}

func (ll *LayerList) rowH() float32 {
	if ll.RowHeight > 0 {
		return ll.RowHeight
	}
	return 48
}

func (ll *LayerList) SetItems(items []LayerItem, selectedIndex int) {
	ll.Items = append(ll.Items[:0], items...)
	if len(ll.Items) > 0 {
		ll.SelectedIndex = min(max(0, selectedIndex), len(ll.Items)-1)
	} else {
		ll.SelectedIndex = 0
	}
	ll.InvalidatePaint()
}

func (ll *LayerList) ScrollIntoView(index int) {
	if index < 0 || index >= len(ll.Items) || ll.rect.Empty() {
		return
	}
	rh := ll.rowH()
	top := float32(index) * rh
	bottom := top + rh
	if top < ll.ScrollY {
		ll.ScrollY = top
		ll.InvalidatePaint()
	} else if bottom > ll.ScrollY+ll.rect.H {
		ll.ScrollY = maxFloat32(0, bottom-ll.rect.H)
		ll.InvalidatePaint()
	}
}

func (ll *LayerList) maxScrollY() float32 {
	return maxFloat32(0, float32(len(ll.Items))*ll.rowH()-ll.rect.H)
}

func (ll *LayerList) Measure(constraints Constraints) Vec2 {
	h := minFloat32(360, maxFloat32(80, float32(len(ll.Items))*ll.rowH()))
	return measureWithState(&ll.State, Vec2{X: 220, Y: h}, constraints)
}

func (ll *LayerList) hitRow(x, y float32) (row int, zone uint8) {
	if !ll.rect.Contains(x, y) || len(ll.Items) == 0 {
		return -1, 0
	}
	rh := ll.rowH()
	row = int((y - ll.rect.Y + ll.ScrollY) / rh)
	if row < 0 || row >= len(ll.Items) {
		return -1, 0
	}
	relX := x - ll.rect.X
	if relX >= 4 && relX <= 30 {
		return row, 1 // visibility toggle
	}
	if relX >= ll.rect.W-32 && relX <= ll.rect.W-4 {
		return row, 2 // lock toggle
	}
	return row, 0
}

func (ll *LayerList) Paint(canvas *Canvas) {
	if ll.rect.Empty() {
		return
	}
	ll.ScrollY = clampFloat32(ll.ScrollY, 0, ll.maxScrollY())
	rh := ll.rowH()
	canvas.PushClip(ll.rect)
	defer canvas.PopClip()

	chk := SharedCheckerboardResource()
	nameSpec := FontSpec{Size: maxFloat32(11, canvas.theme.FontSize*0.78), Weight: 550}
	metaSpec := FontSpec{Size: maxFloat32(9.5, canvas.theme.FontSize*0.66)}
	iconSpec := FontSpec{Size: maxFloat32(11, canvas.theme.FontSize*0.76), Weight: 700}

	for i, item := range ll.Items {
		y := ll.rect.Y + float32(i)*rh - ll.ScrollY
		if y+rh < ll.rect.Y || y > ll.rect.Y+ll.rect.H {
			continue
		}
		rowRect := Rect{X: ll.rect.X + 4, Y: y + 2, W: maxFloat32(0, ll.rect.W-8), H: rh - 4}
		selected := i == ll.SelectedIndex
		if selected {
			canvas.RoundedRect(rowRect, 7, canvas.theme.Accent.WithAlpha(0.1))
			canvas.RoundedBorder(rowRect, 7, 1, canvas.theme.Accent.WithAlpha(0.45))
		} else if i == ll.hoveredRow && ll.Hovered() {
			canvas.RoundedRect(rowRect, 7, canvas.theme.ControlHover)
		}

		// 1. Visibility eye button
		eyeRect := Rect{X: rowRect.X + 4, Y: rowRect.Y + (rowRect.H-24)*0.5, W: 22, H: 24}
		if i == ll.hoveredRow && ll.hoveredSubZone == 1 {
			canvas.RoundedRect(eyeRect, 4, RGBA(255, 255, 255, 20))
		}
		eyeSymbol := "◉"
		eyeColor := canvas.theme.Text
		if !item.Visible {
			eyeSymbol = "○"
			eyeColor = canvas.theme.MutedText.WithAlpha(0.55)
		}
		canvas.TextStyled(eyeSymbol, eyeRect, iconSpec, TextCenter, eyeColor)

		// 2. 32x32 Thumbnail over checkerboard
		thumbRect := Rect{X: eyeRect.X + eyeRect.W + 5, Y: rowRect.Y + (rowRect.H-32)*0.5, W: 32, H: 32}
		if chk != nil {
			canvas.Image(chk, thumbRect, Rect{}, RGBA(255, 255, 255, 255), FilterNearest)
		} else {
			canvas.Rect(thumbRect, RGBA(16, 21, 29, 255))
		}
		if item.Thumbnail != nil && item.Thumbnail.width > 0 && item.Thumbnail.height > 0 {
			scale := minFloat32(30.0/float32(item.Thumbnail.width), 30.0/float32(item.Thumbnail.height))
			tw := float32(item.Thumbnail.width) * scale
			th := float32(item.Thumbnail.height) * scale
			dst := Rect{
				X: thumbRect.X + (thumbRect.W-tw)*0.5,
				Y: thumbRect.Y + (thumbRect.H-th)*0.5,
				W: tw,
				H: th,
			}
			alpha := clampFloat32(item.Opacity, 0, 1)
			if !item.Visible {
				alpha *= 0.35
			}
			canvas.Image(item.Thumbnail, dst, Rect{}, RGBA(255, 255, 255, 255).WithAlpha(alpha), FilterNearest)
		}
		canvas.Border(thumbRect, 1, RGBA(255, 255, 255, 22))

		// 3. Lock button on the right
		lockRect := Rect{X: rowRect.X + rowRect.W - 26, Y: rowRect.Y + (rowRect.H-24)*0.5, W: 22, H: 24}
		if i == ll.hoveredRow && ll.hoveredSubZone == 2 {
			canvas.RoundedRect(lockRect, 4, RGBA(255, 255, 255, 20))
		}
		lockSymbol := "·"
		lockColor := canvas.theme.MutedText.WithAlpha(0.4)
		if item.Locked {
			lockSymbol = "🔒"
			lockColor = canvas.theme.AccentHover
		} else if i == ll.hoveredRow {
			lockSymbol = "🔓"
			lockColor = canvas.theme.MutedText
		}
		canvas.TextStyled(lockSymbol, lockRect, iconSpec, TextCenter, lockColor)

		// 4. Info text (Name + Meta)
		infoX := thumbRect.X + thumbRect.W + 7
		infoW := maxFloat32(0, lockRect.X-infoX-4)
		if infoW > 4 {
			nameColor := canvas.theme.Text
			if !item.Visible {
				nameColor = canvas.theme.MutedText.WithAlpha(0.45)
			}
			nameBounds := Rect{X: infoX, Y: rowRect.Y + 5, W: infoW, H: 18}
			metaBounds := Rect{X: infoX, Y: rowRect.Y + 23, W: infoW, H: 15}
			canvas.TextStyled(ll.ellipsizeText(item.Name, infoW, nameSpec), nameBounds, nameSpec, TextLeft, nameColor)
			canvas.TextStyled(ll.ellipsizeText(item.Meta, infoW, metaSpec), metaBounds, metaSpec, TextLeft, canvas.theme.MutedText)
		}
	}

	// Scrollbar indicator
	maxY := ll.maxScrollY()
	if maxY > 0 && ll.rect.H > 0 {
		totalH := float32(len(ll.Items)) * rh
		barH := maxFloat32(18, ll.rect.H*(ll.rect.H/totalH))
		barY := ll.rect.Y + (ll.rect.H-barH)*(ll.ScrollY/maxY)
		canvas.RoundedRect(Rect{X: ll.rect.X + ll.rect.W - 4, Y: barY, W: 3, H: barH}, 1.5, canvas.theme.Border)
	}
}

func (ll *LayerList) Handle(event Event) bool {
	if !ll.Enabled() || len(ll.Items) == 0 {
		return false
	}
	switch event.Type {
	case PointerMove:
		row, zone := ll.hitRow(event.X, event.Y)
		if row != ll.hoveredRow || zone != ll.hoveredSubZone {
			ll.hoveredRow = row
			ll.hoveredSubZone = zone
			ll.InvalidatePaint()
		}
		return row >= 0
	case PointerDown:
		if event.Button == MouseLeft && ll.rect.Contains(event.X, event.Y) {
			row, zone := ll.hitRow(event.X, event.Y)
			if row >= 0 {
				switch zone {
				case 1:
					if ll.OnToggleVis != nil {
						ll.OnToggleVis(row)
					}
				case 2:
					if ll.OnToggleLock != nil {
						ll.OnToggleLock(row)
					}
				default:
					ll.SelectedIndex = row
					ll.InvalidatePaint()
					if ll.OnSelect != nil {
						ll.OnSelect(row)
					}
				}
				return true
			}
		}
	case PointerScroll:
		maxY := ll.maxScrollY()
		if maxY <= 0 {
			return false
		}
		next := clampFloat32(ll.ScrollY-event.Scroll.Y*36, 0, maxY)
		if next != ll.ScrollY {
			ll.ScrollY = next
			ll.InvalidatePaint()
			return true
		}
	case KeyDown:
		switch event.Key {
		case KeyUpArrow:
			if ll.SelectedIndex > 0 {
				ll.SelectedIndex--
				ll.ScrollIntoView(ll.SelectedIndex)
				if ll.OnSelect != nil {
					ll.OnSelect(ll.SelectedIndex)
				}
				return true
			}
		case KeyDownArrow:
			if ll.SelectedIndex+1 < len(ll.Items) {
				ll.SelectedIndex++
				ll.ScrollIntoView(ll.SelectedIndex)
				if ll.OnSelect != nil {
					ll.OnSelect(ll.SelectedIndex)
				}
				return true
			}
		}
	}
	return ll.State.Handle(event)
}

// FrameStripItem describes one animation frame card in a horizontal FrameStrip.
type FrameStripItem struct {
	ID        string
	Header    string
	Title     string
	Thumbnail *ImageResource
}

// FrameStrip renders a horizontal scrollable timeline of animation frame cards
// with thumbnails, active frame highlight, and playback indicator.
type FrameStrip struct {
	State
	Items         []FrameStripItem
	SelectedIndex int
	PlayingIndex  int
	CardWidth     float32
	CardHeight    float32
	Gap           float32
	ScrollX       float32
	OnSelect      func(index int)
	hoveredIdx    int
}

func NewFrameStrip(onSelect func(index int)) *FrameStrip {
	fs := &FrameStrip{
		PlayingIndex: -1,
		CardWidth:    88,
		CardHeight:   76,
		Gap:          7,
		OnSelect:     onSelect,
		hoveredIdx:   -1,
	}
	fs.owner = fs
	fs.focusable = true
	fs.clip = true
	fs.minimum = Vec2{X: 180, Y: 84}
	return fs
}

func (fs *FrameStrip) SetItems(items []FrameStripItem, selectedIndex, playingIndex int) {
	fs.Items = append(fs.Items[:0], items...)
	if len(fs.Items) > 0 {
		fs.SelectedIndex = min(max(0, selectedIndex), len(fs.Items)-1)
	} else {
		fs.SelectedIndex = 0
	}
	fs.PlayingIndex = playingIndex
	fs.InvalidatePaint()
}

func (fs *FrameStrip) cardW() float32 {
	if fs.CardWidth > 0 {
		return fs.CardWidth
	}
	return 88
}

func (fs *FrameStrip) cardH() float32 {
	if fs.CardHeight > 0 {
		return fs.CardHeight
	}
	return 76
}

func (fs *FrameStrip) maxScrollX() float32 {
	n := len(fs.Items)
	if n == 0 {
		return 0
	}
	totalW := float32(n)*fs.cardW() + float32(max(0, n-1))*fs.Gap + 16
	return maxFloat32(0, totalW-fs.rect.W)
}

func (fs *FrameStrip) ScrollIntoView(index int) {
	if index < 0 || index >= len(fs.Items) || fs.rect.Empty() {
		return
	}
	stride := fs.cardW() + fs.Gap
	left := 8 + float32(index)*stride
	right := left + fs.cardW() + 8
	if left < fs.ScrollX {
		fs.ScrollX = maxFloat32(0, left-8)
		fs.InvalidatePaint()
	} else if right > fs.ScrollX+fs.rect.W {
		fs.ScrollX = minFloat32(fs.maxScrollX(), right-fs.rect.W)
		fs.InvalidatePaint()
	}
}

func (fs *FrameStrip) Measure(constraints Constraints) Vec2 {
	return measureWithState(&fs.State, Vec2{X: 320, Y: fs.cardH() + 10}, constraints)
}

func (fs *FrameStrip) indexAt(x, y float32) int {
	if !fs.rect.Contains(x, y) || len(fs.Items) == 0 {
		return -1
	}
	cw := fs.cardW()
	stride := cw + fs.Gap
	relX := x - (fs.rect.X + 8) + fs.ScrollX
	if relX < 0 {
		return -1
	}
	idx := int(math.Floor(float64(relX / stride)))
	if idx < 0 || idx >= len(fs.Items) || relX-float32(idx)*stride > cw {
		return -1
	}
	return idx
}

func (fs *FrameStrip) Paint(canvas *Canvas) {
	if fs.rect.Empty() {
		return
	}
	fs.ScrollX = clampFloat32(fs.ScrollX, 0, fs.maxScrollX())
	cw := fs.cardW()
	ch := minFloat32(fs.cardH(), maxFloat32(56, fs.rect.H-8))
	stride := cw + fs.Gap
	hdrSpec := FontSpec{Size: maxFloat32(9, canvas.theme.FontSize*0.64)}
	titleSpec := FontSpec{Size: maxFloat32(9.5, canvas.theme.FontSize*0.68), Weight: 550}
	cyan := RGBA(129, 203, 210, 255)

	canvas.PushClip(fs.rect)
	defer canvas.PopClip()

	for i, item := range fs.Items {
		x := fs.rect.X + 8 + float32(i)*stride - fs.ScrollX
		if x+cw < fs.rect.X || x > fs.rect.X+fs.rect.W {
			continue
		}
		card := Rect{X: x, Y: fs.rect.Y + 4, W: cw, H: ch}
		selected := i == fs.SelectedIndex
		playing := i == fs.PlayingIndex
		bg := RGBA(17, 24, 33, 255)
		border := canvas.theme.Border
		if selected {
			bg = canvas.theme.Accent.WithAlpha(0.1)
			border = canvas.theme.Accent
		} else if i == fs.hoveredIdx && fs.Hovered() {
			bg = canvas.theme.ControlHover
		}
		canvas.RoundedRect(card, 6, bg)
		canvas.RoundedBorder(card, 6, 1, border)
		if playing {
			canvas.RoundedBorder(card.Inset(All(2)), 4, 1, cyan)
			canvas.TextStyled("▶", Rect{X: card.X + card.W - 16, Y: card.Y + 2, W: 12, H: 12}, hdrSpec, TextRight, cyan)
		}

		// Header (01 · Z 23)
		hdrRect := Rect{X: card.X + 5, Y: card.Y + 3, W: card.W - 18, H: 13}
		canvas.TextStyled(item.Header, hdrRect, hdrSpec, TextLeft, canvas.theme.MutedText)

		// 64x38 Thumbnail
		thumbH := maxFloat32(28, ch-34)
		thumbRect := Rect{X: card.X + (card.W-64)*0.5, Y: card.Y + 16, W: 64, H: thumbH}
		if item.Thumbnail != nil && item.Thumbnail.width > 0 && item.Thumbnail.height > 0 {
			scale := minFloat32(thumbRect.W/float32(item.Thumbnail.width), thumbRect.H/float32(item.Thumbnail.height))
			tw := float32(item.Thumbnail.width) * scale
			th := float32(item.Thumbnail.height) * scale
			dst := Rect{
				X: thumbRect.X + (thumbRect.W-tw)*0.5,
				Y: thumbRect.Y + (thumbRect.H-th)*0.5,
				W: tw,
				H: th,
			}
			canvas.Image(item.Thumbnail, dst, Rect{}, RGBA(255, 255, 255, 255), FilterNearest)
		}

		// Bottom title
		titleRect := Rect{X: card.X + 5, Y: card.Y + card.H - 16, W: card.W - 10, H: 14}
		titleColor := canvas.theme.Text
		if selected {
			titleColor = canvas.theme.Accent
		}
		canvas.TextStyled(fs.ellipsizeText(item.Title, titleRect.W, titleSpec), titleRect, titleSpec, TextCenter, titleColor)
	}

	// Horizontal scrollbar indicator
	maxX := fs.maxScrollX()
	if maxX > 0 && fs.rect.W > 0 {
		totalW := float32(len(fs.Items))*stride + 16
		barW := maxFloat32(24, fs.rect.W*(fs.rect.W/totalW))
		barX := fs.rect.X + (fs.rect.W-barW)*(fs.ScrollX/maxX)
		canvas.RoundedRect(Rect{X: barX, Y: fs.rect.Y + fs.rect.H - 3, W: barW, H: 2.5}, 1.2, canvas.theme.Border)
	}
}

func (fs *FrameStrip) Handle(event Event) bool {
	if !fs.Enabled() || len(fs.Items) == 0 {
		return false
	}
	switch event.Type {
	case PointerMove:
		idx := fs.indexAt(event.X, event.Y)
		if idx != fs.hoveredIdx {
			fs.hoveredIdx = idx
			fs.InvalidatePaint()
		}
		return idx >= 0
	case PointerDown:
		if event.Button == MouseLeft && fs.rect.Contains(event.X, event.Y) {
			if idx := fs.indexAt(event.X, event.Y); idx >= 0 {
				fs.SelectedIndex = idx
				fs.InvalidatePaint()
				if fs.OnSelect != nil {
					fs.OnSelect(idx)
				}
				return true
			}
		}
	case PointerScroll:
		maxX := fs.maxScrollX()
		if maxX <= 0 {
			return false
		}
		delta := event.Scroll.X
		if delta == 0 {
			delta = event.Scroll.Y
		}
		next := clampFloat32(fs.ScrollX-delta*36, 0, maxX)
		if next != fs.ScrollX {
			fs.ScrollX = next
			fs.InvalidatePaint()
			return true
		}
	case KeyDown:
		switch event.Key {
		case KeyLeftArrow:
			if fs.SelectedIndex > 0 {
				fs.SelectedIndex--
				fs.ScrollIntoView(fs.SelectedIndex)
				if fs.OnSelect != nil {
					fs.OnSelect(fs.SelectedIndex)
				}
				return true
			}
		case KeyRightArrow:
			if fs.SelectedIndex+1 < len(fs.Items) {
				fs.SelectedIndex++
				fs.ScrollIntoView(fs.SelectedIndex)
				if fs.OnSelect != nil {
					fs.OnSelect(fs.SelectedIndex)
				}
				return true
			}
		}
	}
	return fs.State.Handle(event)
}
