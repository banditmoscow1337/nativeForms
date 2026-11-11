package editor

import "math/bits"

type piece struct {
	left, right        *piece
	text               string
	priority           uint64
	bytes, lines       int32
	utf16              int32
	ownLines, ownUTF16 int16
	tx                 uint32
	ownLex, subLex     uint8
}

func pieceBytes(n *piece) int {
	if n == nil {
		return 0
	}
	return int(n.bytes)
}

func pieceLines(n *piece) int {
	if n == nil {
		return 0
	}
	return int(n.lines)
}

func pieceUTF16(n *piece) int {
	if n == nil {
		return 0
	}
	return int(n.utf16)
}

func countMultiByteUTF16(text string) int {
	count := 0
	for _, r := range text {
		count++
		if r > 0xffff {
			count++
		}
	}
	return count
}

func utf16Count(text string) int {
	for i := 0; i < len(text); i++ {
		if text[i] >= 0x80 {
			return countMultiByteUTF16(text)
		}
	}
	return len(text)
}

func computePieceMetrics(text string) (lines, utf16 int, lex uint8) {
	hasMulti := false
	for i := 0; i < len(text); i++ {
		b := text[i]
		if b >= 0x80 {
			hasMulti = true
			continue
		}
		switch b {
		case '\n':
			lines++
		case '`':
			lex |= lexFlagBacktick
		case '/':
			lex |= lexFlagSlash
		case '*':
			lex |= lexFlagStar
		}
	}
	if !hasMulti {
		return lines, len(text), lex
	}
	return lines, countMultiByteUTF16(text), lex
}

func initPiece(n *piece, text string, priority uint64, tx uint32) {
	lines, u16, lex := computePieceMetrics(text)
	*n = piece{
		text:     text,
		priority: priority,
		bytes:    int32(len(text)),
		lines:    int32(lines),
		utf16:    int32(u16),
		ownLines: int16(lines),
		ownUTF16: int16(u16),
		tx:       tx,
		ownLex:   lex,
		subLex:   lex,
	}
}

func update(n *piece) {
	l, r := n.left, n.right
	b := int32(len(n.text))
	ln := int32(n.ownLines)
	u := int32(n.ownUTF16)
	lx := n.ownLex
	if l != nil {
		b += l.bytes
		ln += l.lines
		u += l.utf16
		lx |= l.subLex
	}
	if r != nil {
		b += r.bytes
		ln += r.lines
		u += r.utf16
		lx |= r.subLex
	}
	n.bytes = b
	n.lines = ln
	n.utf16 = u
	n.subLex = lx
}

func (d *Document) allocPiece(entry *historyEntry) *piece {
	var n *piece
	if freeCount := len(d.freePieces); freeCount > 0 {
		n = d.freePieces[freeCount-1]
		d.freePieces = d.freePieces[:freeCount-1]
	} else {
		if cap(d.nodeSlab) == 0 && d.historySlots == nil {
			d.ensurePools()
		}
		if len(d.nodeSlab) == cap(d.nodeSlab) {
			d.nodeSlab = make([]piece, 0, 1024)
		}
		idx := len(d.nodeSlab)
		d.nodeSlab = d.nodeSlab[:idx+1]
		n = &d.nodeSlab[idx]
	}
	if entry != nil {
		entry.created = append(entry.created, n)
	}
	return n
}

func (d *Document) freeCreatedPiece(n *piece, entry *historyEntry) {
	if entry != nil {
		for i := len(entry.created) - 1; i >= 0; i-- {
			if entry.created[i] == n {
				last := len(entry.created) - 1
				entry.created[i] = entry.created[last]
				entry.created[last] = nil
				entry.created = entry.created[:last]
				break
			}
		}
	}
	*n = piece{}
	d.freePieces = append(d.freePieces, n)
}

func (d *Document) editablePiece(n *piece, entry *historyEntry) *piece {
	if n.tx == d.curTx {
		return n
	}
	if entry != nil && n.tx != 0 {
		entry.retired = append(entry.retired, n)
	}
	c := d.allocPiece(entry)
	*c = *n
	c.tx = d.curTx
	return c
}

func (d *Document) newPieceTx(text string, priority uint64, entry *historyEntry) *piece {
	n := d.allocPiece(entry)
	initPiece(n, text, priority, d.curTx)
	return n
}

func (d *Document) releaseSubtree(n *piece, entry *historyEntry) {
	if n == nil {
		return
	}
	left, right := n.left, n.right
	if n.tx == d.curTx {
		d.freeCreatedPiece(n, entry)
	} else if entry != nil && n.tx != 0 {
		entry.retired = append(entry.retired, n)
	}
	d.releaseSubtree(left, entry)
	d.releaseSubtree(right, entry)
}

func (d *Document) nextPriority() uint64 {
	d.seed += 0x9e3779b97f4a7c15
	z := d.seed
	z = (z ^ (z >> 30)) * 0xbf58476d1ce4e5b9
	z = (z ^ (z >> 27)) * 0x94d049bb133111eb
	return (z ^ (z >> 31)) >> 8
}

func nextPieceChunkSize(text string) int {
	size := min(pieceSize, len(text))
	for size < len(text) && text[size]&0xc0 == 0x80 {
		size--
	}
	return size
}

func countTextPieces(text string) int {
	count := 0
	for len(text) > 0 {
		size := nextPieceChunkSize(text)
		count++
		text = text[size:]
	}
	return count
}

// makeInitialPieces builds a balanced treap in O(N) using a single contiguous
// slice allocation and a Cartesian tree right-spine stack.
func (d *Document) makeInitialPieces(text string) *piece {
	if len(text) == 0 {
		d.nodeSlab = make([]piece, 0, 64)
		return nil
	}
	count := countTextPieces(text)
	extra := 128
	if count > 64 {
		extra = 0
	}
	slab := make([]piece, count, count+extra)
	if extra > 0 {
		d.nodeSlab = slab
	}

	var spineBuf [128]*piece
	spine := spineBuf[:0]
	for i := 0; i < count; i++ {
		size := nextPieceChunkSize(text)
		cur := &slab[i]
		priority := (uint64(bits.TrailingZeros64(uint64(i+1))) + 1) << 56 | d.nextPriority()
		initPiece(cur, text[:size], priority, 0)
		text = text[size:]

		var lastPopped *piece
		for len(spine) > 0 && spine[len(spine)-1].priority < cur.priority {
			top := spine[len(spine)-1]
			spine = spine[:len(spine)-1]
			update(top)
			lastPopped = top
		}
		cur.left = lastPopped
		if len(spine) > 0 {
			spine[len(spine)-1].right = cur
		}
		spine = append(spine, cur)
	}
	for i := len(spine) - 1; i >= 0; i-- {
		update(spine[i])
	}
	return spine[0]
}

func (d *Document) makePiecesTx(text string, entry *historyEntry) *piece {
	var root *piece
	for len(text) > 0 {
		size := nextPieceChunkSize(text)
		root = d.join(root, d.newPieceTx(text[:size], d.nextPriority(), entry), entry)
		text = text[size:]
	}
	return root
}
