package nativeforms

import (
	"encoding/binary"
	"fmt"
)

type TrueTypeFace struct {
	tables     map[string][]byte
	head, hhea []byte
	maxp, hmtx []byte
	loca, glyf []byte
	kern       []byte
	units      uint16
	glyphCount uint16
	metrics    uint16
	locaFormat int16
	cmap       []byte
	fastReady  bool
	asciiGlyph [96]uint16
	cyrGlyph   [96]uint16
	kernParsed bool
	hasKern    bool
	kernLeft   []uint64
	kernFilter [64]uint64
	kernPairs  map[uint32]int16
}

func (f *TrueTypeFace) table(name string, cached []byte) []byte {
	if len(cached) > 0 {
		return cached
	}
	if f.tables != nil {
		return f.tables[name]
	}
	return nil
}

func ParseTrueType(data []byte) (*TrueTypeFace, error) {
	return parseTrueType(data, true)
}

func parseTrueType(data []byte, copyTables bool) (*TrueTypeFace, error) {
	if len(data) < 12 {
		return nil, fmt.Errorf("expected a TrueType outline font")
	}
	if sig := binary.BigEndian.Uint32(data[:4]); sig != 0x00010000 && sig != 0x74727565 {
		return nil, fmt.Errorf("expected a TrueType outline font")
	}
	n := int(u16(data, 4))
	if n > 256 || 12+n*16 > len(data) {
		return nil, fmt.Errorf("invalid TrueType directory")
	}
	var head, hhea, maxp, hmtx, loca, glyf, cmap, kern []byte
	for i := 0; i < n; i++ {
		p := 12 + i*16
		tag := string(data[p : p+4])
		start, length := uint64(u32(data, p+8)), uint64(u32(data, p+12))
		if start+length > uint64(len(data)) {
			return nil, fmt.Errorf("invalid font table %s", tag)
		}
		slice := data[start : start+length]
		switch tag {
		case "head":
			head = slice
		case "hhea":
			hhea = slice
		case "maxp":
			maxp = slice
		case "hmtx":
			hmtx = slice
		case "loca":
			loca = slice
		case "glyf":
			glyf = slice
		case "cmap":
			cmap = slice
		case "kern":
			kern = slice
		}
	}
	for _, item := range []struct {
		name string
		buf  []byte
	}{
		{"head", head}, {"hhea", hhea}, {"maxp", maxp},
		{"hmtx", hmtx}, {"loca", loca}, {"glyf", glyf}, {"cmap", cmap},
	} {
		if len(item.buf) == 0 {
			return nil, fmt.Errorf("missing TrueType table %s", item.name)
		}
	}
	if len(head) < 54 || len(hhea) < 36 || len(maxp) < 6 {
		return nil, fmt.Errorf("truncated font header")
	}
	f := &TrueTypeFace{
		units:      u16(head, 18),
		locaFormat: i16(head, 50),
		glyphCount: u16(maxp, 4),
		metrics:    u16(hhea, 34),
	}
	if f.units == 0 || f.glyphCount == 0 || f.metrics == 0 || f.metrics > f.glyphCount ||
		(f.locaFormat != 0 && f.locaFormat != 1) {
		return nil, fmt.Errorf("invalid TrueType metrics")
	}
	locaSize := 2
	if f.locaFormat == 1 {
		locaSize = 4
	}
	hmtxLen := int(f.metrics)*4 + int(f.glyphCount-f.metrics)*2
	locaLen := (int(f.glyphCount) + 1) * locaSize
	if len(loca) < locaLen || len(hmtx) < hmtxLen {
		return nil, fmt.Errorf("truncated TrueType metrics")
	}
	if len(cmap) < 4 {
		return nil, fmt.Errorf("truncated cmap")
	}
	best := 0
	var selectedCmap []byte
	for i := 0; i < int(u16(cmap, 2)); i++ {
		p := 4 + i*8
		if p+8 > len(cmap) {
			return nil, fmt.Errorf("truncated cmap records")
		}
		platform, encoding := u16(cmap, p), u16(cmap, p+2)
		offset := int(u32(cmap, p+4))
		if offset+2 > len(cmap) {
			continue
		}
		format := u16(cmap, offset)
		score := 0
		if format == 4 && (platform == 0 || (platform == 3 && encoding == 1)) {
			score = 1
		}
		if format == 12 && (platform == 0 || (platform == 3 && encoding == 10)) {
			score = 2
		}
		if score > best {
			if format == 4 && offset+4 <= len(cmap) {
				end := offset + int(u16(cmap, offset+2))
				if end <= len(cmap) {
					selectedCmap = cmap[offset:end]
					best = score
				}
			}
			if format == 12 && offset+8 <= len(cmap) {
				end := uint64(offset) + uint64(u32(cmap, offset+4))
				if end <= uint64(len(cmap)) {
					selectedCmap = cmap[offset:end]
					best = score
				}
			}
		}
	}
	if len(selectedCmap) == 0 {
		return nil, fmt.Errorf("unsupported Unicode cmap")
	}
	headSlice := head[:54]
	hheaSlice := hhea[:36]
	maxpSlice := maxp[:6]
	hmtxSlice := hmtx[:hmtxLen]
	locaSlice := loca[:locaLen]
	if !copyTables {
		f.head = headSlice
		f.hhea = hheaSlice
		f.maxp = maxpSlice
		f.hmtx = hmtxSlice
		f.loca = locaSlice
		f.glyf = glyf
		f.cmap = selectedCmap
		f.kern = kern
		f.parseKerning()
		if !f.hasKern || f.kernPairs != nil {
			f.kern = nil
		}
	} else {
		// Pack required font tables into a single contiguous byte slab so the
		// multi-megabyte font file buffer can be GC'd with minimal heap fragmentation.
		f.kern = kern
		f.parseKerning()
		keepKern := f.hasKern && f.kernPairs == nil
		kernLen := 0
		if keepKern {
			kernLen = len(kern)
		}
		totalLen := len(headSlice) + len(hheaSlice) + len(maxpSlice) + len(hmtxSlice) + len(locaSlice) + len(glyf) + len(selectedCmap) + kernLen
		slab := make([]byte, totalLen)
		pos := 0
		take := func(src []byte) []byte {
			n := copy(slab[pos:], src)
			res := slab[pos : pos+n : pos+n]
			pos += n
			return res
		}
		f.head = take(headSlice)
		f.hhea = take(hheaSlice)
		f.maxp = take(maxpSlice)
		f.hmtx = take(hmtxSlice)
		f.loca = take(locaSlice)
		f.glyf = take(glyf)
		f.cmap = take(selectedCmap)
		if keepKern {
			f.kern = take(kern)
		} else {
			f.kern = nil
		}
	}
	for i := range f.asciiGlyph {
		f.asciiGlyph[i] = f.glyphIndexRaw(rune(32 + i))
	}
	for i := range f.cyrGlyph {
		f.cyrGlyph[i] = f.glyphIndexRaw(rune(0x0400 + i))
	}
	f.fastReady = true
	return f, nil
}

func u16(b []byte, i int) uint16 { return binary.BigEndian.Uint16(b[i : i+2]) }
func i16(b []byte, i int) int16  { return int16(u16(b, i)) }
func u32(b []byte, i int) uint32 { return binary.BigEndian.Uint32(b[i : i+4]) }

func (f *TrueTypeFace) glyphIndex(r rune) uint16 {
	if f.fastReady {
		if r >= 32 && r < 128 {
			return f.asciiGlyph[r-32]
		}
		if r >= 0x0400 && r < 0x0460 {
			return f.cyrGlyph[r-0x0400]
		}
	}
	return f.glyphIndexRaw(r)
}

func (f *TrueTypeFace) glyphIndexRaw(r rune) uint16 {
	c := f.cmap
	if len(c) < 16 {
		return 0
	}
	if u16(c, 0) == 12 {
		if r < 0 {
			return 0
		}
		n := int(u32(c, 12))
		if n > (len(c)-16)/12 {
			return 0
		}
		lo, hi := 0, n
		for lo < hi {
			mid := (lo + hi) / 2
			p := 16 + mid*12
			if uint32(r) > u32(c, p+4) {
				lo = mid + 1
			} else {
				hi = mid
			}
		}
		if lo < n {
			p := 16 + lo*12
			if uint32(r) >= u32(c, p) {
				id := u32(c, p+8) + uint32(r) - u32(c, p)
				if id < uint32(f.glyphCount) {
					return uint16(id)
				}
			}
		}
		return 0
	}
	if u16(c, 0) != 4 || r < 0 || r > 0xffff {
		return 0
	}
	segCount := int(u16(c, 6)) / 2
	endBase, startBase := 14, 16+segCount*2
	deltaBase, rangeBase := startBase+segCount*2, startBase+segCount*4
	if segCount == 0 || rangeBase+segCount*2 > len(c) {
		return 0
	}
	ur := uint16(r)
	lo, hi := 0, segCount
	for lo < hi {
		mid := (lo + hi) / 2
		if ur > u16(c, endBase+mid*2) {
			lo = mid + 1
		} else {
			hi = mid
		}
	}
	if lo >= segCount || ur < u16(c, startBase+lo*2) {
		return 0
	}
	delta := uint16(i16(c, deltaBase+lo*2))
	offset := int(u16(c, rangeBase+lo*2))
	var id uint16
	if offset == 0 {
		id = uint16(uint32(ur) + uint32(delta))
	} else {
		p := rangeBase + lo*2 + offset + 2*int(ur-u16(c, startBase+lo*2))
		if p+2 > len(c) {
			return 0
		}
		id = u16(c, p)
		if id != 0 {
			id = uint16(uint32(id) + uint32(delta))
		}
	}
	if id < f.glyphCount {
		return id
	}
	return 0
}

func (f *TrueTypeFace) advance(id uint16) float32 {
	if id >= f.glyphCount {
		return 0.6
	}
	index := min(int(id), int(f.metrics)-1)
	hmtx := f.table("hmtx", f.hmtx)
	return float32(u16(hmtx, index*4)) / float32(f.units)
}
