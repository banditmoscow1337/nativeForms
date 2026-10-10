package nativeforms

import (
	"strings"
	"unicode"
)

var textBreakReplacer = strings.NewReplacer("\r\n", "\n", "\r", "\n", "\u0085", "\n", "\u2028", "\n", "\u2029", "\n")

func normalizeTextBreaks(text string) string { return textBreakReplacer.Replace(text) }

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

// A deliberately small RU/EN subset, not a complete UAX #14 implementation.
// Never break clusters, explicit glue, after opening or before closing marks.
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
	// Avoid introducing breaks in negative numbers and common date fragments.
	if left == '-' || left == '‐' || left == '/' {
		return at > 1 && !(r[at-2] >= '0' && r[at-2] <= '9' && right >= '0' && right <= '9')
	}
	return false
}

func fittedTextWidth(r []rune, start, end int, size float32) float32 {
	return fittedTextWidthWith(r, start, end, size, measureTextLine)
}

func fittedTextWidthWith(r []rune, start, end int, size float32, measure func(string, float32) float32) float32 {
	for end > start && wrappingSpace(r[end-1]) {
		end--
	}
	return measure(string(r[start:end]), size)
}

// wrappedLineEnd retains all rune offsets for editors. Trailing breakable
// spaces do not affect fitting; labels may discard them when emitting lines.
func wrappedLineEnd(r []rune, start, end int, width, size float32) int {
	return wrappedLineEndWith(r, start, end, width, size, measureTextLine)
}

func wrappedLineEndWith(r []rune, start, end int, width, size float32, measure func(string, float32) float32) int {
	if width <= 0 || size <= 0 || fittedTextWidthWith(r, start, end, size, measure) <= width {
		return end
	}
	lo, hi := start, end
	for lo < hi {
		mid := lo + (hi-lo+1)/2
		if fittedTextWidthWith(r, start, mid, size, measure) <= width {
			lo = mid
		} else {
			hi = mid - 1
		}
	}
	fit := nearestGrapheme(r, lo)
	for at := fit; at > start; at = previousGrapheme(r, at) {
		if textBreakOpportunity(r[:end], at) {
			return at
		}
	}
	// Keep an explicitly glued first word together, even if it overflows. This
	// preserves amounts such as "10\u00a0000" and nonbreaking compound names.
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
	// Emergency wrapping of a long word is allowed only at safe cluster edges.
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

// WrapText wraps Russian/English prose at spaces, hyphens, slashes, em dashes
// and zero-width spaces. Explicit glue and common paired punctuation are kept
// together. Long words may break at grapheme boundaries; no hyphens are added.
// CRLF/CR/NEL/LS/PS become LF in this display string. Editor text is untouched.
func WrapText(text string, width, size float32) string {
	return wrapTextWith(text, width, size, measureTextLine)
}

func ellipsizeTextWith(text string, width float32, measure func(string) float32, ellipsis string) string {
	if width <= 0 {
		return ""
	}
	text = strings.ReplaceAll(normalizeTextBreaks(text), "\n", " ")
	if measure(text) <= width {
		return text
	}
	if measure(ellipsis) > width {
		return ""
	}
	r := []rune(text)
	best := ellipsis
	for end := nextGrapheme(r, 0); end <= len(r); end = nextGrapheme(r, end) {
		candidate := strings.TrimRightFunc(string(r[:end]), unicode.IsSpace) + ellipsis
		if measure(candidate) > width {
			break
		}
		best = candidate
		if end == len(r) {
			break
		}
	}
	return best
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
		r := []rune(paragraph)
		for at := 0; at < len(r); {
			end := wrappedLineEndWith(r, at, len(r), width, size, measure)
			inkEnd := end
			if end < len(r) {
				for inkEnd > at && wrappingSpace(r[inkEnd-1]) {
					inkEnd--
				}
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
