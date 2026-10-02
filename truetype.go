package nativeforms

import (
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"math"
)

// TrueTypeFace reads quadratic TrueType outlines without font libraries.
// CFF, variations, GSUB/GPOS and hinting are outside this initial engine.
type TrueTypeFace struct {
	data []byte
	tables map[string][]byte
	units uint16
	glyphCount uint16
	metrics uint16
	locaFormat int16
	cmap []byte
}

func ParseTrueType(data []byte) (*TrueTypeFace, error) {
	if len(data) < 12 || binary.BigEndian.Uint32(data[:4]) != 0x00010000 {
		return nil, fmt.Errorf("expected a TrueType outline font")
	}
	n := int(u16(data,4))
	if n > 256 || 12+n*16 > len(data) { return nil, fmt.Errorf("invalid TrueType directory") }
	f := &TrueTypeFace{data:data, tables:make(map[string][]byte)}
	for i:=0; i<n; i++ {
		p:=12+i*16
		tag:=string(data[p:p+4])
		start,length:=uint64(u32(data,p+8)),uint64(u32(data,p+12))
		if start+length > uint64(len(data)) { return nil,fmt.Errorf("invalid font table %s",tag) }
		f.tables[tag]=data[start:start+length]
	}
	for _, tag := range []string{"head","hhea","maxp","hmtx","loca","glyf","cmap"} {
		if len(f.tables[tag])==0 { return nil,fmt.Errorf("missing TrueType table %s",tag) }
	}
	if len(f.tables["head"])<54 || len(f.tables["hhea"])<36 || len(f.tables["maxp"])<6 {
		return nil,fmt.Errorf("truncated font header")
	}
	f.units=u16(f.tables["head"],18)
	f.locaFormat=i16(f.tables["head"],50)
	f.glyphCount=u16(f.tables["maxp"],4)
	f.metrics=u16(f.tables["hhea"],34)
	if f.units==0 || f.glyphCount==0 || f.metrics==0 || f.metrics>f.glyphCount ||
		(f.locaFormat!=0 && f.locaFormat!=1) {
		return nil,fmt.Errorf("invalid TrueType metrics")
	}
	locaSize:=2
	if f.locaFormat==1 { locaSize=4 }
	if len(f.tables["loca"]) < (int(f.glyphCount)+1)*locaSize ||
		len(f.tables["hmtx"]) < int(f.metrics)*4+int(f.glyphCount-f.metrics)*2 {
		return nil,fmt.Errorf("truncated TrueType metrics")
	}
	cmap:=f.tables["cmap"]
	if len(cmap)<4 { return nil,fmt.Errorf("truncated cmap") }
	best:=0
	for i:=0; i<int(u16(cmap,2)); i++ {
		p:=4+i*8
		if p+8>len(cmap) { return nil,fmt.Errorf("truncated cmap records") }
		platform,encoding:=u16(cmap,p),u16(cmap,p+2)
		offset:=int(u32(cmap,p+4))
		if offset+2>len(cmap) { continue }
		format:=u16(cmap,offset)
		score:=0
		if format==4 && (platform==0 || (platform==3 && encoding==1)) { score=1 }
		if format==12 && (platform==0 || (platform==3 && encoding==10)) { score=2 }
		if score>best {
			if format==4 && offset+4<=len(cmap) {
				end:=offset+int(u16(cmap,offset+2))
				if end<=len(cmap) { f.cmap=cmap[offset:end]; best=score }
			}
			if format==12 && offset+8<=len(cmap) {
				end:=uint64(offset)+uint64(u32(cmap,offset+4))
				if end<=uint64(len(cmap)) { f.cmap=cmap[offset:end]; best=score }
			}
		}
	}
	if len(f.cmap)==0 { return nil,fmt.Errorf("unsupported Unicode cmap") }
	return f,nil
}

func u16(b []byte, i int) uint16 { return binary.BigEndian.Uint16(b[i:i+2]) }
func i16(b []byte, i int) int16 { return int16(u16(b,i)) }
func u32(b []byte, i int) uint32 { return binary.BigEndian.Uint32(b[i:i+4]) }

func (f *TrueTypeFace) glyphIndex(r rune) uint16 {
	c:=f.cmap
	if len(c)<16 { return 0 }
	if u16(c,0)==12 {
		if r<0 || len(c)<16 { return 0 }
		n:=int(u32(c,12))
		if n>(len(c)-16)/12 { return 0 }
		lo,hi:=0,n
		for lo<hi {
			mid:=(lo+hi)/2; p:=16+mid*12
			if uint32(r)>u32(c,p+4) { lo=mid+1 } else { hi=mid }
		}
		if lo<n {
			p:=16+lo*12
			if uint32(r)>=u32(c,p) {
				id:=u32(c,p+8)+uint32(r)-u32(c,p)
				if id<uint32(f.glyphCount) { return uint16(id) }
			}
		}
		return 0
	}
	if u16(c,0)!=4 || r<0 || r>0xffff { return 0 }
	segCount:=int(u16(c,6))/2
	endBase,startBase:=14,16+segCount*2
	deltaBase,rangeBase:=startBase+segCount*2,startBase+segCount*4
	if segCount==0 || rangeBase+segCount*2>len(c) { return 0 }
	for i:=0;i<segCount;i++ {
		if uint16(r)>u16(c,endBase+i*2) { continue }
		if uint16(r)<u16(c,startBase+i*2) { return 0 }
		delta:=uint16(i16(c,deltaBase+i*2))
		offset:=int(u16(c,rangeBase+i*2))
		var id uint16
		if offset==0 {
			id=uint16(uint32(uint16(r))+uint32(delta))
		} else {
			p:=rangeBase+i*2+offset+2*int(uint16(r)-u16(c,startBase+i*2))
			if p+2>len(c) { return 0 }
			id=u16(c,p)
			if id!=0 { id=uint16(uint32(id)+uint32(delta)) }
		}
		if id<f.glyphCount { return id }
		return 0
	}
	return 0
}

func (f *TrueTypeFace) advance(id uint16) float32 {
	if id>=f.glyphCount { return 0.6 }
	index:=int(id)
	if index>=int(f.metrics) { index=int(f.metrics)-1 }
	return float32(u16(f.tables["hmtx"],index*4))/float32(f.units)
}

type fontPoint struct { x,y float32; on bool }
type outline struct { points []fontPoint; ends []int }

func (f *TrueTypeFace) glyph(id uint16, depth int) (outline,error) {
	if id>=f.glyphCount || depth>8 { return outline{},fmt.Errorf("invalid composite glyph") }
	loca,glyf:=f.tables["loca"],f.tables["glyf"]
	var start,end uint32
	if f.locaFormat==0 {
		start=uint32(u16(loca,int(id)*2))*2
		end=uint32(u16(loca,(int(id)+1)*2))*2
	} else {
		start=u32(loca,int(id)*4)
		end=u32(loca,(int(id)+1)*4)
	}
	if start==end { return outline{},nil }
	if end<start || uint64(end)>uint64(len(glyf)) || end-start<10 {
		return outline{},fmt.Errorf("invalid glyph range")
	}
	b:=glyf[start:end]
	count:=int(i16(b,0))
	if count>=0 { return parseSimpleGlyph(b,count) }
	result:=outline{}
	p:=10
	for component:=0;component<64;component++ {
		if p+4>len(b) { return outline{},fmt.Errorf("truncated composite") }
		flags,id2:=u16(b,p),u16(b,p+2); p+=4
		if flags&0x0002==0 { return outline{},fmt.Errorf("point-aligned composites are unsupported") }
		var x,y float32
		if flags&0x0001!=0 {
			if p+4>len(b) { return outline{},fmt.Errorf("truncated component offset") }
			x,y=float32(i16(b,p)),float32(i16(b,p+2));p+=4
		} else {
			if p+2>len(b) { return outline{},fmt.Errorf("truncated component offset") }
			x,y=float32(int8(b[p])),float32(int8(b[p+1]));p+=2
		}
		a,d,bx,cy:=float32(1),float32(1),float32(0),float32(0)
		readScale:=func() (float32,error) {
			if p+2>len(b) { return 0,fmt.Errorf("truncated component transform") }
			v:=float32(i16(b,p))/16384;p+=2;return v,nil
		}
		if flags&0x0008!=0 {
			v,e:=readScale();if e!=nil{return outline{},e};a,d=v,v
		} else if flags&0x0040!=0 {
			v,e:=readScale();if e!=nil{return outline{},e};a=v
			v,e=readScale();if e!=nil{return outline{},e};d=v
		} else if flags&0x0080!=0 {
			var e error
			a,e=readScale();if e!=nil{return outline{},e}
			bx,e=readScale();if e!=nil{return outline{},e}
			cy,e=readScale();if e!=nil{return outline{},e}
			d,e=readScale();if e!=nil{return outline{},e}
		}
		child,e:=f.glyph(id2,depth+1);if e!=nil{return outline{},e}
		base:=len(result.points)
		for _,q:=range child.points {
			result.points=append(result.points,fontPoint{x:a*q.x+bx*q.y+x,y:cy*q.x+d*q.y+y,on:q.on})
		}
		for _,v:=range child.ends { result.ends=append(result.ends,base+v) }
		if flags&0x0020==0 { return result,nil }
	}
	return outline{},fmt.Errorf("too many glyph components")
}

func parseSimpleGlyph(b []byte, count int) (outline,error) {
	if count>1024 || 10+count*2+2>len(b) { return outline{},fmt.Errorf("invalid contour count") }
	if count==0 { return outline{},nil }
	o:=outline{ends:make([]int,count)}
	p:=10
	for i:=range o.ends {
		o.ends[i]=int(u16(b,p));p+=2
		if i>0 && o.ends[i]<=o.ends[i-1] { return outline{},fmt.Errorf("invalid contour endpoints") }
	}
	n:=o.ends[count-1]+1
	if n>32768 || p+2>len(b) { return outline{},fmt.Errorf("invalid point count") }
	instructions:=int(u16(b,p));p+=2+instructions
	if p>len(b) { return outline{},fmt.Errorf("truncated glyph instructions") }
	flags:=make([]byte,0,n)
	for len(flags)<n {
		if p>=len(b) { return outline{},fmt.Errorf("truncated glyph flags") }
		flag:=b[p];p++
		repeat:=1
		if flag&8!=0 {
			if p>=len(b) { return outline{},fmt.Errorf("truncated repeat flag") }
			repeat+=int(b[p]);p++
		}
		if repeat>n-len(flags) { return outline{},fmt.Errorf("invalid glyph flags") }
		for j:=0;j<repeat;j++ { flags=append(flags,flag) }
	}
	o.points=make([]fontPoint,n)
	readCoordinates:=func(xAxis bool) error {
		value:=int32(0)
		for i,flag:=range flags {
			short,positive:=byte(2),byte(16)
			if !xAxis { short,positive=4,32 }
			delta:=int32(0)
			if flag&short!=0 {
				if p>=len(b) { return fmt.Errorf("truncated glyph coordinates") }
				delta=int32(b[p]);p++
				if flag&positive==0 { delta=-delta }
			} else if flag&positive==0 {
				if p+2>len(b) { return fmt.Errorf("truncated glyph coordinates") }
				delta=int32(i16(b,p));p+=2
			}
			value+=delta
			if xAxis { o.points[i].x=float32(value);o.points[i].on=flag&1!=0
			} else { o.points[i].y=float32(value) }
		}
		return nil
	}
	if err:=readCoordinates(true);err!=nil{return outline{},err}
	if err:=readCoordinates(false);err!=nil{return outline{},err}
	return o,nil
}

type floatPoint struct { x,y float32 }
type segment struct { a,b floatPoint }

func (f *TrueTypeFace) drawGlyph(atlas *image.NRGBA, id uint16, cellX,cellY int) error {
	o,err:=f.glyph(id,0);if err!=nil{return err}
	if len(o.points)==0 { return nil }
	scale:=float32(fontReferenceSize)/float32(f.units)
	transform:=func(p fontPoint) floatPoint {
		return floatPoint{x:float32(cellX)+5+p.x*scale,y:float32(cellY)+52-p.y*scale}
	}
	var segments []segment
	start:=0
	addCurve:=func(a,b,c floatPoint) {
		last:=a
		for i:=1;i<=12;i++ {
			t:=float32(i)/12
			next:=floatPoint{x:(1-t)*(1-t)*a.x+2*(1-t)*t*b.x+t*t*c.x,
				y:(1-t)*(1-t)*a.y+2*(1-t)*t*b.y+t*t*c.y}
			segments=append(segments,segment{last,next});last=next
		}
	}
	for _,end:=range o.ends {
		contour:=o.points[start:end+1];start=end+1
		if len(contour)==0 { continue }
		last,first:=contour[len(contour)-1],contour[0]
		begin:=first
		if !first.on {
			if last.on { begin=last } else {
				begin=fontPoint{x:(first.x+last.x)*0.5,y:(first.y+last.y)*0.5,on:true}
			}
		}
		current:=transform(begin)
		for i:=0;i<len(contour); {
			next:=contour[i]
			if next.on {
				target:=transform(next)
				segments=append(segments,segment{current,target})
				current=target;i++
			} else {
				after:=contour[(i+1)%len(contour)]
				if !after.on { after=fontPoint{x:(next.x+after.x)*0.5,y:(next.y+after.y)*0.5,on:true};i++
				} else { i+=2 }
				target:=transform(after)
				addCurve(current,transform(next),target)
				current=target
			}
		}
		segments=append(segments,segment{current,transform(begin)})
	}
	if len(segments)==0 { return nil }
	minX,maxX,minY,maxY:=segments[0].a.x,segments[0].a.x,segments[0].a.y,segments[0].a.y
	for _,s:=range segments {
		minX=float32(math.Min(float64(minX),float64(s.b.x)))
		maxX=float32(math.Max(float64(maxX),float64(s.b.x)))
		minY=float32(math.Min(float64(minY),float64(s.b.y)))
		maxY=float32(math.Max(float64(maxY),float64(s.b.y)))
	}
	left:=max(cellX,int(math.Floor(float64(minX))))
	top:=max(cellY,int(math.Floor(float64(minY))))
	right:=min(cellX+fontCellSize,int(math.Ceil(float64(maxX))))
	bottom:=min(cellY+fontCellSize,int(math.Ceil(float64(maxY))))
	for y:=top;y<bottom;y++ {
		for x:=left;x<right;x++ {
			hits:=0
			for _,offsetY:=range []float32{0.25,0.75} {
				for _,offsetX:=range []float32{0.25,0.75} {
					px,py:=float32(x)+offsetX,float32(y)+offsetY
					winding:=0
					for _,s:=range segments {
						if s.a.y<=py && s.b.y>py || s.b.y<=py && s.a.y>py {
							intersection:=s.a.x+(py-s.a.y)*(s.b.x-s.a.x)/(s.b.y-s.a.y)
							if intersection>px {
								if s.b.y>s.a.y { winding++ } else { winding-- }
							}
						}
					}
					if winding!=0 { hits++ }
				}
			}
			if hits>0 { atlas.SetNRGBA(x,y,color.NRGBA{R:255,G:255,B:255,A:uint8(hits*255/4)}) }
		}
	}
	return nil
}
