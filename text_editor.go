package nativeforms

import "unicode"

// TextEditor stores rune offsets at grapheme boundaries. The selection is
// the half-open interval between Anchor and Caret. Preedit is kept separate
// from committed text, so an IME update never enters undo history.
type TextEditor struct {
	Text string
	Caret, Anchor int
	Preedit string
	undo, redo []editSnapshot
	Limit int // maximum undo snapshots; zero uses 128
}

type editSnapshot struct { text string; caret, anchor int }

func NewTextEditor(text string) *TextEditor {
	n:=len([]rune(text))
	return &TextEditor{Text:text,Caret:n,Anchor:n}
}

func (e *TextEditor) SetText(text string) {
	e.Text=text
	e.Caret=len([]rune(text))
	e.Anchor=e.Caret
	e.Preedit=""
	e.undo=nil
	e.redo=nil
}

func (e *TextEditor) selection() (int,int) {
	a,b:=e.Anchor,e.Caret
	if a>b { a,b=b,a }
	return a,b
}

func (e *TextEditor) Selection() string {
	a,b:=e.selection()
	r:=[]rune(e.Text)
	a=min(max(a,0),len(r));b=min(max(b,a),len(r))
	return string(r[a:b])
}

func (e *TextEditor) SelectAll() { e.Anchor=0;e.Caret=len([]rune(e.Text)) }

func (e *TextEditor) Move(to int, extend bool) {
	r:=[]rune(e.Text)
	to=min(max(to,0),len(r))
	to=nearestGrapheme(r,to)
	e.Caret=to
	if !extend { e.Anchor=to }
}

func (e *TextEditor) Step(direction int, word, extend bool) {
	r:=[]rune(e.Text)
	if !extend && e.Caret!=e.Anchor && !word {
		a,b:=e.selection()
		if direction<0 { e.Move(a,false) } else { e.Move(b,false) }
		return
	}
	if word { e.Move(wordBoundary(r,e.Caret,direction),extend);return }
	if direction<0 { e.Move(previousGrapheme(r,e.Caret),extend)
	} else { e.Move(nextGrapheme(r,e.Caret),extend) }
}

func (e *TextEditor) Replace(value string) bool {
	a,b:=e.selection()
	r:=[]rune(e.Text)
	a=min(max(a,0),len(r));b=min(max(b,a),len(r))
	if a==b && value=="" { return false }
	updated:=string(r[:a])+value+string(r[b:])
	if updated==e.Text { return false }
	e.record()
	e.Text=updated
	e.Caret=a+len([]rune(value))
	updatedRunes:=[]rune(updated)
	if !graphemeBreak(updatedRunes,e.Caret) { e.Caret=nextGrapheme(updatedRunes,e.Caret) }
	e.Anchor=e.Caret
	e.Preedit=""
	return true
}

func (e *TextEditor) Delete(backward, word bool) bool {
	if e.Caret==e.Anchor {
		r:=[]rune(e.Text)
		pos:=e.Caret
		if word { pos=wordBoundary(r,pos,map[bool]int{true:-1,false:1}[backward])
		} else if backward { pos=previousGrapheme(r,pos)
		} else { pos=nextGrapheme(r,pos) }
		e.Anchor=pos
	}
	return e.Replace("")
}

func (e *TextEditor) record() {
	limit:=e.Limit
	if limit<=0 { limit=128 }
	e.undo=append(e.undo,editSnapshot{e.Text,e.Caret,e.Anchor})
	if len(e.undo)>limit { copy(e.undo,e.undo[len(e.undo)-limit:]);e.undo=e.undo[:limit] }
	e.redo=nil
}

func (e *TextEditor) Undo() bool { return e.restore(&e.undo,&e.redo) }
func (e *TextEditor) Redo() bool { return e.restore(&e.redo,&e.undo) }
func (e *TextEditor) restore(from,to *[]editSnapshot) bool {
	if len(*from)==0 { return false }
	*to=append(*to,editSnapshot{e.Text,e.Caret,e.Anchor})
	last:=len(*from)-1
	s:=(*from)[last]
	*from=(*from)[:last]
	e.Text,e.Caret,e.Anchor=s.text,s.caret,s.anchor
	e.Preedit=""
	return true
}

func (e *TextEditor) CommitComposition(value string) bool {
	e.Preedit=""
	return e.Replace(value)
}

// Boundaries implement the common UAX #29 extended-cluster rules (CR/LF,
// combining marks, spacing marks, Hangul, ZWJ emoji and flag pairs). The
// standard library has no Grapheme_Cluster_Break property table, so rare
// Indic conjunct and prepend cases remain outside this implementation.
func previousGrapheme(r []rune, at int) int {
	at=min(max(at,0),len(r))
	for i:=at-1;i>=0;i-- { if graphemeBreak(r,i) { return i } }
	return 0
}
func nextGrapheme(r []rune, at int) int {
	at=min(max(at,0),len(r))
	for i:=at+1;i<len(r);i++ { if graphemeBreak(r,i) { return i } }
	return len(r)
}
func nearestGrapheme(r []rune, at int) int {
	if at<=0 { return 0 };if at>=len(r) { return len(r) }
	if graphemeBreak(r,at) { return at }
	return previousGrapheme(r,at)
}

func graphemeBreak(r []rune,i int) bool {
	if i<=0 || i>=len(r) { return true }
	a,b:=r[i-1],r[i]
	if a=='\r' && b=='\n' { return false }
	if a=='\r' || a=='\n' || b=='\r' || b=='\n' || unicode.IsControl(a) || unicode.IsControl(b) { return true }
	if hangulLink(a,b) { return false }
	if unicode.Is(unicode.Mn,b) || unicode.Is(unicode.Me,b) || unicode.Is(unicode.Mc,b) || b==0x200d || isVariation(b) || isSkinTone(b) { return false }
	if a==0x200d && isPictograph(b) {
		for j:=i-2;j>=0;j-- {
			if unicode.Is(unicode.Mn,r[j]) || isVariation(r[j]) { continue }
			if isPictograph(r[j]) { return false }
			break
		}
	}
	if isRegional(a) && isRegional(b) {
		n:=0
		for j:=i-1;j>=0 && isRegional(r[j]);j-- { n++ }
		return n%2==0
	}
	return true
}
func isVariation(r rune) bool { return r>=0xfe00 && r<=0xfe0f || r>=0xe0100 && r<=0xe01ef }
func isSkinTone(r rune) bool { return r>=0x1f3fb && r<=0x1f3ff }
func isRegional(r rune) bool { return r>=0x1f1e6 && r<=0x1f1ff }
func isPictograph(r rune) bool { return r>=0x1f000 && r<=0x1faff || r>=0x2600 && r<=0x27bf }
func hangulClass(r rune) int {
	switch {
	case r>=0x1100 && r<=0x115f || r>=0xa960 && r<=0xa97c: return 1
	case r>=0x1160 && r<=0x11a7 || r>=0xd7b0 && r<=0xd7c6: return 2
	case r>=0x11a8 && r<=0x11ff || r>=0xd7cb && r<=0xd7fb: return 3
	case r>=0xac00 && r<=0xd7a3:
		if (r-0xac00)%28==0 { return 4 };return 5
	}
	return 0
}
func hangulLink(a,b rune) bool {
	x,y:=hangulClass(a),hangulClass(b)
	return x==1 && (y==1 || y==2 || y==4 || y==5) ||
		(x==2 || x==4) && (y==2 || y==3) || (x==3 || x==5) && y==3
}
func wordRune(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) || r=='_' }
func wordBoundary(r []rune,at,direction int) int {
	at=min(max(at,0),len(r))
	if direction<0 {
		for at>0 && !wordRune(r[at-1]) { at-- }
		for at>0 && wordRune(r[at-1]) { at-- }
		return nearestGrapheme(r,at)
	}
	for at<len(r) && !wordRune(r[at]) { at++ }
	for at<len(r) && wordRune(r[at]) { at++ }
	return nearestGrapheme(r,at)
}
