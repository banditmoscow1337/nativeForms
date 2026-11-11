package editor

// join keeps the heap invariant while sharing all untouched subtrees.
func (d *Document) join(left, right *piece, entry *historyEntry) *piece {
	if left == nil {
		return right
	}
	if right == nil {
		return left
	}
	if left.priority >= right.priority {
		n := d.editablePiece(left, entry)
		n.right = d.join(n.right, right, entry)
		update(n)
		return n
	}
	n := d.editablePiece(right, entry)
	n.left = d.join(left, n.left, entry)
	update(n)
	return n
}

func (d *Document) split(root *piece, offset int, entry *historyEntry) (*piece, *piece) {
	if root == nil {
		return nil, nil
	}
	leftBytes := pieceBytes(root.left)
	if offset < leftBytes {
		left, middle := d.split(root.left, offset, entry)
		tail := d.editablePiece(root, entry)
		tail.left = nil
		update(tail)
		return left, d.join(middle, tail, entry)
	}
	end := leftBytes + len(root.text)
	if offset > end {
		middle, right := d.split(root.right, offset-end, entry)
		head := d.editablePiece(root, entry)
		head.right = nil
		update(head)
		return d.join(head, middle, entry), right
	}
	if offset == leftBytes {
		left := root.left
		tail := d.editablePiece(root, entry)
		tail.left = nil
		update(tail)
		return left, tail
	}
	if offset == end {
		right := root.right
		head := d.editablePiece(root, entry)
		head.right = nil
		update(head)
		return head, right
	}
	within := offset - leftBytes
	leftChild, rightChild := root.left, root.right
	text := root.text
	if root.tx == d.curTx {
		d.freeCreatedPiece(root, entry)
	} else if entry != nil && root.tx != 0 {
		entry.retired = append(entry.retired, root)
	}
	return d.join(leftChild, d.newPieceTx(text[:within], d.nextPriority(), entry), entry),
		d.join(d.newPieceTx(text[within:], d.nextPriority(), entry), rightChild, entry)
}

// replaceExactPiece replaces a single piece in-place along the root-to-node
// path when [start, end) matches the exact boundaries of an existing piece and
// the replacement text fits in one piece.
func (d *Document) replaceExactPiece(root *piece, start, end int, text string, entry *historyEntry) (*piece, bool) {
	if root == nil || start >= end || len(text) == 0 || len(text) > pieceSize {
		return nil, false
	}
	var path [64]*piece
	var dir [64]uint8
	depth := 0
	cur := root
	s, e := start, end
	for cur != nil {
		if depth >= len(path) {
			return nil, false
		}
		leftBytes := pieceBytes(cur.left)
		if e <= leftBytes {
			path[depth] = cur
			dir[depth] = 0
			depth++
			cur = cur.left
		} else if s >= leftBytes+len(cur.text) {
			path[depth] = cur
			dir[depth] = 1
			depth++
			s -= leftBytes + len(cur.text)
			e -= leftBytes + len(cur.text)
			cur = cur.right
		} else if s == leftBytes && e == leftBytes+len(cur.text) {
			child := d.allocPiece(entry)
			initPiece(child, text, cur.priority, d.curTx)
			child.left = cur.left
			child.right = cur.right
			update(child)
			if cur.tx != 0 {
				entry.retired = append(entry.retired, cur)
			}
			for i := depth - 1; i >= 0; i-- {
				parent := path[i]
				if parent.tx != 0 {
					entry.retired = append(entry.retired, parent)
				}
				cloned := d.allocPiece(entry)
				*cloned = *parent
				cloned.tx = d.curTx
				if dir[i] == 0 {
					cloned.left = child
				} else {
					cloned.right = child
				}
				update(cloned)
				child = cloned
			}
			return child, true
		} else {
			return nil, false
		}
	}
	return nil, false
}

func (d *Document) applySorted(root *piece, edits []Edit, entry *historyEntry) *piece {
	for i := len(edits) - 1; i >= 0; i-- {
		edit := edits[i]
		left, tail := d.split(root, edit.Start, entry)
		right := tail
		if edit.End > edit.Start {
			var deleted *piece
			deleted, right = d.split(tail, edit.End-edit.Start, entry)
			d.releaseSubtree(deleted, entry)
		}
		root = d.join(d.join(left, d.makePiecesTx(edit.Text, entry), entry), right, entry)
	}
	return root
}

func validBoundary(root *piece, offset int) bool {
	return offset >= 0 && offset <= pieceBytes(root) &&
		(offset == pieceBytes(root) || byteAt(root, offset)&0xc0 != 0x80)
}

func validRange(root *piece, start, end int) bool {
	return start <= end && validBoundary(root, start) && validBoundary(root, end)
}

func byteAt(root *piece, offset int) byte {
	for root != nil {
		left := pieceBytes(root.left)
		if offset < left {
			root = root.left
		} else if offset < left+len(root.text) {
			return root.text[offset-left]
		} else {
			offset -= left + len(root.text)
			root = root.right
		}
	}
	return 0
}

func sliceSinglePieceNode(n *piece, start, end int) (string, bool) {
	if start == end {
		return "", true
	}
	for n != nil {
		left := pieceBytes(n.left)
		if end <= left {
			n = n.left
		} else if start >= left+len(n.text) {
			start -= left + len(n.text)
			end -= left + len(n.text)
			n = n.right
		} else if start >= left && end <= left+len(n.text) {
			return n.text[start-left : end-left], true
		} else {
			return "", false
		}
	}
	return "", false
}

func readRange(root *piece, start, end int) string {
	if start >= end {
		return ""
	}
	if s, ok := sliceSinglePieceNode(root, start, end); ok {
		return s
	}
	buf := appendRangeBytes(make([]byte, 0, end-start), root, start, end)
	return string(buf)
}

// SliceSinglePiece returns a substring without allocating when [start, end)
// resides completely inside one piece node.
func (d *Document) SliceSinglePiece(start, end int) (string, bool) {
	if !validRange(d.root, start, end) {
		return "", false
	}
	return sliceSinglePieceNode(d.root, start, end)
}

// AppendRange appends the byte range [start, end) to dst without intermediate
// heap allocations when capacity is sufficient.
func (d *Document) AppendRange(dst []byte, start, end int) ([]byte, error) {
	if !validRange(d.root, start, end) {
		return dst, ErrRange
	}
	return appendRangeBytes(dst, d.root, start, end), nil
}

func appendRangeBytes(dst []byte, root *piece, start, end int) []byte {
	if root == nil || start >= end {
		return dst
	}
	left := pieceBytes(root.left)
	if start < left {
		dst = appendRangeBytes(dst, root.left, start, min(end, left))
	}
	if start < left+len(root.text) && end > left {
		dst = append(dst, root.text[max(0, start-left):min(len(root.text), end-left)]...)
	}
	if end > left+len(root.text) {
		dst = appendRangeBytes(dst, root.right, max(0, start-left-len(root.text)), end-left-len(root.text))
	}
	return dst
}
