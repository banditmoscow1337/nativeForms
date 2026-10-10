package nativeforms

// atlasKerning implements horizontal OpenType 'kern' format 0. Unsupported or
// malformed subtables are ignored, so a damaged optional table cannot prevent
// a valid outline font from being used. No kerning crosses font boundaries.
func (f *TrueTypeFace) atlasKerning(left, right uint16) float32 {
	table := f.tables["kern"]
	if len(table) < 4 || u16(table, 0) != 0 {
		return 0 // Apple fixed-point version 1 is a different format.
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
			continue // format 0, horizontal, no minimum/cross-stream/reserved bits
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
