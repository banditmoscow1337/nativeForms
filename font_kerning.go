package nativeforms

func kernPairFilterBit(key uint32) (word, bit uint32) {
	h := key * 2654435761
	return h >> 26, (h >> 20) & 63
}

func (f *TrueTypeFace) parseKerning() {
	f.kernParsed = true
	table := f.table("kern", f.kern)
	if len(table) < 4 || u16(table, 0) != 0 {
		return
	}
	at := 4
	allSorted := true
	totalPairs := 0
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
		if f.kernLeft == nil {
			words := max(1, (int(f.glyphCount)+63)/64)
			if words > 1024 {
				words = 1024
			}
			f.kernLeft = make([]uint64, words)
		}
		var prevKey uint32
		for j := 0; j < count; j++ {
			p := 14 + j*6
			key := u32(sub, p)
			if j > 0 && key < prevKey {
				allSorted = false
			}
			prevKey = key
			left := int(key >> 16)
			if idx := left >> 6; idx < len(f.kernLeft) {
				f.kernLeft[idx] |= 1 << (left & 63)
			}
			w, b := kernPairFilterBit(key)
			f.kernFilter[w] |= 1 << b
			totalPairs++
		}
	}
	if totalPairs == 0 {
		return
	}
	f.hasKern = true
	if allSorted {
		// Keep the sorted binary kern table directly and avoid allocating a Go map.
		return
	}
	f.kernPairs = make(map[uint32]int16, totalPairs)
	at = 4
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
		override := coverage&8 != 0
		for j := 0; j < count; j++ {
			p := 14 + j*6
			key := u32(sub, p)
			adj := i16(sub, p+4)
			if override {
				f.kernPairs[key] = adj
			} else {
				f.kernPairs[key] += adj
			}
		}
	}
}

func (f *TrueTypeFace) atlasKerning(left, right uint16) float32 {
	if !f.kernParsed {
		return f.atlasKerningRaw(left, right)
	}
	idx := int(left >> 6)
	if !f.hasKern || idx >= len(f.kernLeft) || f.kernLeft[idx]&(1<<(left&63)) == 0 {
		return 0
	}
	key := uint32(left)<<16 | uint32(right)
	w, b := kernPairFilterBit(key)
	if f.kernFilter[w]&(1<<b) == 0 {
		return 0
	}
	if f.kernPairs != nil {
		if adj, ok := f.kernPairs[key]; ok {
			return float32(adj) / float32(f.units)
		}
		return 0
	}
	return f.atlasKerningRaw(left, right)
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
