package nativeforms

func (f *TrueTypeFace) parseKerning() {
	f.kernParsed = true
	table := f.table("kern", f.kern)
	if len(table) < 4 || u16(table, 0) != 0 {
		return
	}
	at := 4
	for i, subCount := 0, int(u16(table, 2)); i < subCount && at+6 <= len(table); i++ {
		length := int(u16(table, at+2))
		if length < 6 || length > len(table)-at {
			break
		}
		sub := table[at : at+length]
		at += length
		coverage := u16(sub, 4)
		if u16(sub, 0) != 0 || coverage&0xfff7 != 1 || len(sub) < 14 {
			continue
		}
		count := int(u16(sub, 6))
		if count <= 0 || count > (len(sub)-14)/6 {
			continue
		}
		if f.kernPairs == nil {
			f.kernPairs = make(map[uint32]int32, count)
		}
		override := coverage&8 != 0
		for j := 0; j < count; j++ {
			p := 14 + j*6
			key := u32(sub, p)
			adj := int32(i16(sub, p+4))
			if override {
				f.kernPairs[key] = adj
			} else {
				f.kernPairs[key] += adj
			}
			left := key >> 16
			f.kernLeft[left>>6] |= 1 << (left & 63)
		}
	}
	f.hasKern = len(f.kernPairs) > 0
}

func (f *TrueTypeFace) atlasKerning(left, right uint16) float32 {
	if !f.kernParsed {
		return f.atlasKerningRaw(left, right)
	}
	if !f.hasKern || f.kernLeft[left>>6]&(1<<(left&63)) == 0 {
		return 0
	}
	if adj, ok := f.kernPairs[uint32(left)<<16|uint32(right)]; ok {
		return float32(adj) / float32(f.units)
	}
	return 0
}

func (f *TrueTypeFace) atlasKerningRaw(left, right uint16) float32 {
	table := f.table("kern", f.kern)
	if len(table) < 4 || u16(table, 0) != 0 {
		return 0
	}
	value := int32(0)
	key := uint32(left)<<16 | uint32(right)
	at := 4
	for i, count := 0, int(u16(table, 2)); i < count && at+6 <= len(table); i++ {
		length := int(u16(table, at+2))
		if length < 6 || length > len(table)-at {
			break
		}
		sub := table[at : at+length]
		at += length
		coverage := u16(sub, 4)
		if u16(sub, 0) != 0 || coverage&0xfff7 != 1 || len(sub) < 14 {
			continue
		}
		count := int(u16(sub, 6))
		if count > (len(sub)-14)/6 {
			continue
		}
		lo, hi := 0, count
		for lo < hi {
			mid := lo + (hi-lo)/2
			if u32(sub, 14+mid*6) < key {
				lo = mid + 1
			} else {
				hi = mid
			}
		}
		if lo < count && u32(sub, 14+lo*6) == key {
			adjustment := int32(i16(sub, 18+lo*6))
			if coverage&8 != 0 {
				value = adjustment
			} else {
				value += adjustment
			}
		}
	}
	return float32(value) / float32(f.units)
}
