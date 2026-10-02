package nativeforms

import "strings"

// WrapText inserts line breaks at word boundaries, falling back to grapheme
// boundaries for an overlong word. The same result is used by measure/paint.
func WrapText(text string,width,size float32) string {
	if width<=0 || size<=0 { return text }
	var output strings.Builder
	for paragraphIndex,paragraph:=range strings.Split(text,"\n") {
		if paragraphIndex>0 { output.WriteByte('\n') }
		r:=[]rune(paragraph)
		lineStart,at,lastSpace:=0,0,-1
		for at<len(r) {
			next:=nextGrapheme(r,at)
			if r[at]==' ' || r[at]=='\t' { lastSpace=at }
			if measureTextLine(string(r[lineStart:next]),size)>width && at>lineStart {
				end:=at
				if lastSpace>=lineStart { end=lastSpace }
				output.WriteString(string(r[lineStart:end]));output.WriteByte('\n')
				lineStart=end
				for lineStart<len(r) && (r[lineStart]==' ' || r[lineStart]=='\t') { lineStart++ }
				at=lineStart;lastSpace=-1
				continue
			}
			at=next
		}
		output.WriteString(string(r[lineStart:]))
	}
	return output.String()
}
