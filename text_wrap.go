package nativeforms

import (
	"strings"
	"unicode"
)

var textBreakReplacer = strings.NewReplacer("\r\n", "\n", "\r", "\n", "\u0085", "\n", "\u2028", "\n", "\u2029", "\n")

func hasNonLFTextBreak(text string) bool {
	for i := 0; i < len(text); i++ {
		b := text[i]
		if b == '\r' {
			return true
		}
		if b >= 0x80 {
			if b == 0xc2 && i+1 < len(text) && text[i+1] == 0x85 {
				return true
			}
			if b == 0xe2 && i+2 < len(text) && text[i+1] == 0x80 && (text[i+2] == 0xa8 || text[i+2] == 0xa9) {
				return true
			}
		}
	}
	return false
}

func normalizeTextBreaks(text string) string {
	if !hasNonLFTextBreak(text) {
		return text
	}
	return textBreakReplacer.Replace(text)
}

func hardTextBreak(r []rune, at int) int {
	if at >= len(r) {
		return 0
	}
	switch r[at] {
	case '\r':
		if at+1 < len(r) && r[at+1] == '\n' {
			return 2
		}
		return 1
	case '\n', '\u0085', '\u2028', '\u2029':
		return 1
	}
	return 0
}

func wrappingSpace(r rune) bool { return r == ' ' || r == '\t' || r == '\u2009' }
func textGlue(r rune) bool {
	return r == '\u00a0' || r == '\u202f' || r == '\u2060' || r == '\ufeff' || r == '\u2011'
}

func textBreakBlocked(r []rune, at int) bool {
	if at <= 0 || at >= len(r) {
		return false
	}
	if !graphemeBreak(r, at) || textGlue(r[at-1]) || textGlue(r[at]) {
		return true
	}
	left := at - 1
	for left >= 0 && wrappingSpace(r[left]) {
		left--
	}
	return (left >= 0 && strings.ContainsRune("([{«“„‘", r[left])) ||
		strings.ContainsRune(")]}»”’.,;:!?…%‰", r[at])
}

func textBreakOpportunity(r []rune, at int) bool {
	if at == len(r) {
		return true
	}
	if at <= 0 || wrappingSpace(r[at]) || textBreakBlocked(r, at) {
		return false
	}
	left, right := r[at-1], r[at]
	if wrappingSpace(left) || left == '\u200b' {
		return true
	}
	if left == '—' || right == '—' {
		return left != right
	}

	if left == '-' || left == '‐' || left == '/' {
		return at > 1 && !(r[at-2] >= '0' && r[at-2] <= '9' && right >= '0' && right <= '9')
	}
	return false
}

func trimWrappingSpacesEnd(r []rune, start, end int) int {
	for end > start && wrappingSpace(r[end-1]) {
		end--
	}
	return end
}

func fittedTextWidthWith(r []rune, start, end int, size float32, measure func(string, float32) float32) float32 {
	end = trimWrappingSpacesEnd(r, start, end)
	return measure(string(r[start:end]), size)
}

func fittedAtlasWidthRunes(r []rune, start, end int, size float32) float32 {
	end = trimWrappingSpacesEnd(r, start, end)
	return walkAtlasRunes(r[start:end], size, nil, nil)
}

func binarySearchLineEnd(r []rune, start, end int, width, size float32, fittedWidth func(int) float32) int {
	if width <= 0 || size <= 0 || fittedWidth(end) <= width {
		return end
	}
	lo, hi := start, end
	for lo < hi {
		mid := lo + (hi-lo+1)/2
		if fittedWidth(mid) <= width {
			lo = mid
		} else {
			hi = mid - 1
		}
	}
	return finishWrappedLineEnd(r, start, end, lo)
}

func wrappedLineEndAtlas(r []rune, start, end int, width, size float32) int {
	return binarySearchLineEnd(r, start, end, width, size, func(mid int) float32 {
		return fittedAtlasWidthRunes(r, start, mid, size)
	})
}

func wrappedLineEndWith(r []rune, start, end int, width, size float32, measure func(string, float32) float32) int {
	return binarySearchLineEnd(r, start, end, width, size, func(mid int) float32 {
		return fittedTextWidthWith(r, start, mid, size, measure)
	})
}

func finishWrappedLineEnd(r []rune, start, end, lo int) int {
	fit := nearestGrapheme(r, lo)
	for at := fit; at > start; at = previousGrapheme(r, at) {
		if textBreakOpportunity(r[:end], at) {
			return at
		}
	}

	wordEnd, glued := start, false
	for wordEnd < end {
		glued = glued || textGlue(r[wordEnd])
		wordEnd = nextGrapheme(r, wordEnd)
		if textBreakOpportunity(r[:end], wordEnd) {
			break
		}
	}
	if glued {
		return wordEnd
	}

	for at := fit; at > start; at = previousGrapheme(r, at) {
		if !textBreakBlocked(r[:end], at) && !wrappingSpace(r[at]) {
			return at
		}
	}
	for at := nextGrapheme(r, start); at < end; at = nextGrapheme(r, at) {
		if !textBreakBlocked(r[:end], at) && !wrappingSpace(r[at]) {
			return at
		}
	}
	return end
}

func WrapText(text string, width, size float32) string {
	return wrapTextWith(text, width, size, measureTextLine)
}

func ellipsizeTextWith(text string, width float32, measure func(string) float32, ellipsis string) string {
	if width <= 0 {
		return ""
	}
	text = normalizeTextBreaks(text)
	if strings.ContainsRune(text, '\n') {
		text = strings.ReplaceAll(text, "\n", " ")
	}
	if measure(text) <= width {
		return text
	}
	if measure(ellipsis) > width {
		return ""
	}
	var runeStack [256]rune
	r := stringToRunesInto(text, runeStack[:0])
	var stackEnds [256]int
	ends := stackEnds[:0]
	for end := nextGrapheme(r, 0); end <= len(r); end = nextGrapheme(r, end) {
		ends = append(ends, end)
		if end == len(r) {
			break
		}
	}
	candidateAt := func(idx int) string {
		end := ends[idx]
		for end > 0 && unicode.IsSpace(r[end-1]) {
			end--
		}
		return string(r[:end]) + ellipsis
	}
	lo, hi := -1, len(ends)-1
	for lo < hi {
		mid := lo + (hi-lo+1)/2
		if measure(candidateAt(mid)) <= width {
			lo = mid
		} else {
			hi = mid - 1
		}
	}
	if lo < 0 {
		return ellipsis
	}
	return candidateAt(lo)
}

func wrapTextWith(text string, width, size float32, measure func(string, float32) float32) string {
	text = normalizeTextBreaks(text)
	if width <= 0 || size <= 0 {
		return text
	}
	var output strings.Builder
	for paragraphIndex, paragraph := range strings.Split(text, "\n") {
		if paragraphIndex > 0 {
			output.WriteByte('\n')
		}
		var runeStack [256]rune
		r := stringToRunesInto(paragraph, runeStack[:0])
		for at := 0; at < len(r); {
			end := wrappedLineEndWith(r, at, len(r), width, size, measure)
			inkEnd := end
			if end < len(r) {
				inkEnd = trimWrappingSpacesEnd(r, at, end)
			}
			output.WriteString(string(r[at:inkEnd]))
			at = end
			if at < len(r) {
				output.WriteByte('\n')
			}
		}
	}
	return output.String()
}
