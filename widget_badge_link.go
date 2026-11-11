package nativeforms

// Hyperlink is a focusable clickable link widget that renders accent-colored
// text with hover/focus underline and a default "Copy Link" context menu.
type Hyperlink struct {
	State
	Text      string
	URL       string
	Color     Color
	Underline bool
	OnClick   func(url string)
}

func NewHyperlink(text, url string, onClick func(url string)) *Hyperlink {
	link := &Hyperlink{
		Text:      text,
		URL:       url,
		Underline: true,
		OnClick:   onClick,
	}
	link.owner = link
	link.focusable = true
	link.SetContextMenuBuilder(func(_, _ float32) []MenuItem {
		target := link.URL
		if target == "" {
			target = link.Text
		}
		return []MenuItem{
			{
				ID:    "open_link",
				Label: "Open Link",
				Action: func() {
					link.Activate()
				},
			},
			{
				ID:       "copy_link",
				Label:    "Copy Link Address (" + target + ")",
				Disabled: target == "",
				Action: func() {
					if link.manager != nil && link.manager.clipboardWrite != nil {
						_ = link.manager.clipboardWrite(target)
					}
				},
			},
		}
	})
	return link
}

func (link *Hyperlink) SetText(text string) {
	if link.Text == text {
		return
	}
	link.Text = text
	link.invalidateLayout()
}

func (link *Hyperlink) SetURL(url string) {
	link.URL = url
}

func (link *Hyperlink) Activate() {
	if !link.Enabled() {
		return
	}
	if link.OnClick != nil {
		link.OnClick(link.URL)
	}
}

func (link *Hyperlink) measureTextSize() Vec2 {
	theme := link.State.Theme()
	sz := link.measureText(link.Text, FontSpec{Size: theme.FontSize})
	return Vec2{X: sz.X + 8, Y: maxFloat32(sz.Y+8, theme.FontSize*1.4)}
}

func (link *Hyperlink) Measure(constraints Constraints) Vec2 {
	return measureWithState(&link.State, link.measureTextSize(), constraints)
}

func (link *Hyperlink) Paint(canvas *Canvas) {
	if link.rect.Empty() {
		return
	}
	color := link.Color
	if color.A <= 0 {
		color = canvas.theme.Accent
		if link.Hovered() || link.Focused() {
			color = canvas.theme.AccentHover
		}
	}
	if !link.Enabled() {
		color = canvas.theme.MutedText
	}
	inner := link.rect.Inset(Symmetric(4, 2))
	canvas.Text(link.Text, inner, canvas.theme.FontSize, TextLeft, color)
	if (link.Underline || link.Hovered() || link.Focused()) && link.Text != "" {
		tw := minFloat32(inner.W, link.textWidth(link.Text, FontSpec{Size: canvas.theme.FontSize}))
		lineY := link.rect.Y + link.rect.H - 3
		canvas.Line(Vec2{X: inner.X, Y: lineY}, Vec2{X: inner.X + tw, Y: lineY}, 1, color)
	}
	if link.Focused() && !link.Frameless() {
		canvas.RoundedBorder(link.rect, link.effectiveCornerRadius(canvas.theme), 1, canvas.theme.Accent)
	}
}

func (link *Hyperlink) Handle(event Event) bool {
	return handleControlActivation(&link.State, event, link.Activate)
}

// Badge renders a compact pill-shaped status or count indicator fitted to its text.
type Badge struct {
	State
	Text       string
	Color      Color
	Background Color
	Border     Color
	FontSize   float32
}

func NewBadge(text string, background ...Color) *Badge {
	b := &Badge{Text: text}
	if len(background) > 0 {
		b.Background = background[0]
	}
	b.owner = b
	b.autoSize = true
	return b
}

func (b *Badge) SetText(text string) {
	if b.Text == text {
		return
	}
	b.Text = text
	b.invalidateLayout()
}

func (b *Badge) SetBackground(color Color) {
	if b.Background == color {
		return
	}
	b.Background = color
	b.InvalidatePaint()
}

func (b *Badge) fontSpec() FontSpec {
	size := b.FontSize
	if size <= 0 {
		size = b.Theme().FontSize * 0.8
	}
	return FontSpec{Size: size, Weight: 700}
}

func (b *Badge) measureTextSize() Vec2 {
	spec := b.fontSpec()
	sz := b.measureText(b.Text, spec)
	h := maxFloat32(22, sz.Y+8)
	return Vec2{X: maxFloat32(h, sz.X+16), Y: h}
}

func (b *Badge) Measure(constraints Constraints) Vec2 {
	return measureWithState(&b.State, b.measureTextSize(), constraints)
}

func (b *Badge) Paint(canvas *Canvas) {
	if b.rect.Empty() {
		return
	}
	bg := b.Background
	if bg.A <= 0 {
		bg = canvas.theme.Accent.WithAlpha(0.25)
	}
	border := b.Border
	if border.A <= 0 {
		border = canvas.theme.Accent
	}
	fg := b.Color
	if fg.A <= 0 {
		fg = canvas.theme.Text
	}
	radius := b.rect.H * 0.5
	if b.cornerRadius > 0 {
		radius = b.cornerRadius
	}
	if !b.Frameless() {
		canvas.RoundedRect(b.rect, radius, bg)
		canvas.RoundedBorder(b.rect, radius, 1, border)
	}
	canvas.TextStyled(b.Text, b.rect.Inset(Symmetric(6, 2)), b.fontSpec(), TextCenter, fg)
}
