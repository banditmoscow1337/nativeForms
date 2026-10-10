package nativeforms

// ScrollPanel clips one child and can scroll in either axis. Offset retains
// the previous vertical API; OffsetX is the horizontal position.
type ScrollPanel struct {
	State
	Content Component
	Offset,OffsetX float32
	Horizontal,Vertical bool
	width,height float32
	dragAxis Direction
	dragStart,dragOffset float32
}

func NewScrollPanel(content Component) *ScrollPanel {
	p:=&ScrollPanel{Content:content,Vertical:true}
	p.owner=p;p.clip=true
	if content!=nil { p.Add(content) }
	return p
}

func (p *ScrollPanel) contentSize(view Rect) Vec2 {
	if p.Content==nil { return Vec2{} }
	limits:=Constraints{Max:Vec2{X:view.W,Y:view.H}}
	if p.Horizontal { limits.Max.X=0 }
	if p.Vertical { limits.Max.Y=0 }
	if !p.Horizontal && view.W>0 { limits.BoundedX=true;limits.Min.X=view.W }
	if !p.Vertical && view.H>0 { limits.BoundedY=true;limits.Min.Y=view.H }
	size:=p.Content.Measure(limits)
	return Vec2{X:maxFloat32(view.W,size.X),Y:maxFloat32(view.H,size.Y)}
}

func (p *ScrollPanel) Measure(c Constraints) Vec2 {
	if p.Content==nil { return p.State.Measure(c) }
	view:=Rect{W:c.Max.X,H:c.Max.Y}
	size:=p.contentSize(view)
	return measureWithState(&p.State,size,c)
}

func (p *ScrollPanel) Arrange(r Rect) {
	p.rect=r
	if p.Content==nil { return }
	size:=p.contentSize(r)
	p.width,p.height=size.X,size.Y
	p.OffsetX=clampFloat32(p.OffsetX,0,maxFloat32(0,size.X-r.W))
	p.Offset=clampFloat32(p.Offset,0,maxFloat32(0,size.Y-r.H))
	p.Content.Arrange(Rect{X:r.X-p.OffsetX,Y:r.Y-p.Offset,W:size.X,H:size.Y})
}

// ScrollBy returns the unused delta, allowing a nested scroll panel's parent
// to consume movement after this viewport reaches its edge.
func (p *ScrollPanel) ScrollBy(dx,dy float32) Vec2 {
	beforeX,beforeY:=p.OffsetX,p.Offset
	p.OffsetX=clampFloat32(p.OffsetX+dx,0,maxFloat32(0,p.width-p.rect.W))
	p.Offset=clampFloat32(p.Offset+dy,0,maxFloat32(0,p.height-p.rect.H))
	if beforeX!=p.OffsetX || beforeY!=p.Offset {
		p.Arrange(p.rect);p.invalidatePaint()
	}
	return Vec2{X:dx-(p.OffsetX-beforeX),Y:dy-(p.Offset-beforeY)}
}

func (p *ScrollPanel) Scroll(pixels float32) { p.ScrollBy(0,pixels) }

func (p *ScrollPanel) ScrollIntoView(target Component) bool {
	if target==nil { return false }
	found:=false
	for parent:=target.UIState().parent;parent!=nil;parent=parent.UIState().parent {
		if sameComponent(parent,p) { found=true;break }
	}
	if !found { return false }
	r:=target.UIState().rect
	dx,dy:=float32(0),float32(0)
	if r.X<p.rect.X { dx=r.X-p.rect.X } else if r.X+r.W>p.rect.X+p.rect.W { dx=r.X+r.W-p.rect.X-p.rect.W }
	if r.Y<p.rect.Y { dy=r.Y-p.rect.Y } else if r.Y+r.H>p.rect.Y+p.rect.H { dy=r.Y+r.H-p.rect.Y-p.rect.H }
	beforeX,beforeY:=p.OffsetX,p.Offset
	p.ScrollBy(dx,dy)
	return beforeX!=p.OffsetX || beforeY!=p.Offset
}

// PaintOverlay runs after its content so the scrollbars remain visible.
func (p *ScrollPanel) PaintOverlay(c *Canvas) {
	const thickness float32=8
	if p.height>p.rect.H && p.rect.H>0 {
		fraction:=p.rect.H/p.height
		h:=minFloat32(p.rect.H,maxFloat32(20,p.rect.H*fraction))
		y:=p.rect.Y+(p.rect.H-h)*p.Offset/(p.height-p.rect.H)
		c.Rect(Rect{X:p.rect.X+p.rect.W-thickness,Y:y,W:thickness,H:h},c.theme.Border)
	}
	if p.width>p.rect.W && p.rect.W>0 {
		fraction:=p.rect.W/p.width
		width:=minFloat32(p.rect.W,maxFloat32(20,p.rect.W*fraction))
		x:=p.rect.X+(p.rect.W-width)*p.OffsetX/(p.width-p.rect.W)
		c.Rect(Rect{X:x,Y:p.rect.Y+p.rect.H-thickness,W:width,H:thickness},c.theme.Border)
	}
}

func (p *ScrollPanel) Handle(event Event) bool {
	switch event.Type {
	case PointerScroll:
		remaining:=p.ScrollBy(-event.Scroll.X*40,-event.Scroll.Y*40)
		return remaining!=(Vec2{X:-event.Scroll.X*40,Y:-event.Scroll.Y*40})
	case PointerDown:
		if event.Button!=MouseLeft { break }
		if p.height>p.rect.H && event.X>=p.rect.X+p.rect.W-10 {
			p.dragAxis=Vertical;p.dragStart=event.Y;p.dragOffset=p.Offset;return true
		}
		if p.width>p.rect.W && event.Y>=p.rect.Y+p.rect.H-10 {
			p.dragAxis=Horizontal;p.dragStart=event.X;p.dragOffset=p.OffsetX;return true
		}
	case PointerMove:
		if p.dragAxis==Vertical && p.rect.H>0 {
			p.ScrollBy(0,p.dragOffset+(event.Y-p.dragStart)*p.height/p.rect.H-p.Offset);return true
		}
		if p.dragAxis==Horizontal && p.rect.W>0 {
			p.ScrollBy(p.dragOffset+(event.X-p.dragStart)*p.width/p.rect.W-p.OffsetX,0);return true
		}
	case PointerUp:
		if p.dragAxis!=Overlay && event.Button==MouseLeft { p.dragAxis=Overlay;return true }
	}
	return p.State.Handle(event)
}
