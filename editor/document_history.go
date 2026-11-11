package editor

type historyEntry struct {
	before, after *piece
	forward       []Edit
	backward      []Edit
	fwd1, bwd1    [1]Edit
	retired       []*piece
	created       []*piece
	retireBuf     [32]*piece
	createBuf     [32]*piece
}

func (e *historyEntry) reset() {
	e.before, e.after = nil, nil
	e.forward, e.backward = nil, nil
	e.fwd1[0], e.bwd1[0] = Edit{}, Edit{}
	clear(e.retired)
	clear(e.created)
	if cap(e.retired) > len(e.retireBuf) {
		e.retired = e.retired[:0]
	} else {
		e.retired = e.retireBuf[:0]
	}
	if cap(e.created) > len(e.createBuf) {
		e.created = e.created[:0]
	} else {
		e.created = e.createBuf[:0]
	}
}

type largeDocPools struct {
	inPool       bool
	nodes        [4096]piece
	historySlots [260]historyEntry
	undoRing     [512]*historyEntry
	redoStack    [260]*historyEntry
	freeHistory  [260]*historyEntry
	freePieces   [512]*piece
}

func (d *Document) allocHistoryEntry() *historyEntry {
	d.ensurePools()
	if n := len(d.freeHistory); n > 0 {
		entry := d.freeHistory[n-1]
		d.freeHistory = d.freeHistory[:n-1]
		entry.reset()
		return entry
	}
	if d.historyUsed < len(d.historySlots) {
		entry := &d.historySlots[d.historyUsed]
		d.historyUsed++
		entry.reset()
		return entry
	}
	slab := make([]historyEntry, 32)
	for i := len(slab) - 1; i >= 1; i-- {
		e := &slab[i]
		e.retired = e.retireBuf[:0]
		e.created = e.createBuf[:0]
		d.freeHistory = append(d.freeHistory, e)
	}
	entry := &slab[0]
	entry.reset()
	return entry
}

func (d *Document) releaseHistoryEntry(entry *historyEntry) {
	if entry == nil || entry == &d.scratchEntry {
		return
	}
	entry.reset()
	d.freeHistory = append(d.freeHistory, entry)
}

func (d *Document) pushUndo(entry *historyEntry) {
	d.ensurePools()
	if d.undoCount == len(d.undoRing) {
		newCap := len(d.undoRing) * 2
		grown := make([]*historyEntry, newCap)
		mask := len(d.undoRing) - 1
		for i := 0; i < d.undoCount; i++ {
			grown[i] = d.undoRing[(d.undoHead+i)&mask]
		}
		d.undoRing = grown
		d.undoHead = 0
	}
	mask := len(d.undoRing) - 1
	d.undoRing[(d.undoHead+d.undoCount)&mask] = entry
	d.undoCount++
}

func (d *Document) popOldestUndo() *historyEntry {
	if d.undoCount == 0 {
		return nil
	}
	mask := len(d.undoRing) - 1
	entry := d.undoRing[d.undoHead]
	d.undoRing[d.undoHead] = nil
	d.undoHead = (d.undoHead + 1) & mask
	d.undoCount--
	return entry
}

func (d *Document) popNewestUndo() *historyEntry {
	if d.undoCount == 0 {
		return nil
	}
	mask := len(d.undoRing) - 1
	d.undoCount--
	idx := (d.undoHead + d.undoCount) & mask
	entry := d.undoRing[idx]
	d.undoRing[idx] = nil
	return entry
}

func (d *Document) trimUndoTo(limit int) {
	for d.undoCount > limit {
		oldest := d.popOldestUndo()
		if oldest != nil {
			d.freePieces = append(d.freePieces, oldest.retired...)
			d.releaseHistoryEntry(oldest)
		}
	}
}

func (d *Document) clearRedo() {
	for i := len(d.redoStack) - 1; i >= 0; i-- {
		entry := d.redoStack[i]
		d.redoStack[i] = nil
		if entry != nil {
			d.freePieces = append(d.freePieces, entry.created...)
			d.releaseHistoryEntry(entry)
		}
	}
	d.redoStack = d.redoStack[:0]
}

func (d *Document) beginTx() *historyEntry {
	d.clearRedo()
	var entry *historyEntry
	if d.HistoryLimit > 0 {
		d.trimUndoTo(d.HistoryLimit - 1)
		entry = d.allocHistoryEntry()
	} else {
		d.trimUndoTo(0)
		entry = &d.scratchEntry
		entry.reset()
	}
	d.curTx++
	if d.curTx == 0 {
		d.curTx = 1
	}
	return entry
}

func (d *Document) commitTx(entry *historyEntry, before *piece, forward, backward []Edit) {
	d.version++
	if d.HistoryLimit > 0 {
		entry.before = before
		entry.after = d.root
		entry.forward = forward
		entry.backward = backward
		d.pushUndo(entry)
	} else {
		d.freePieces = append(d.freePieces, entry.retired...)
		entry.reset()
	}
}

func (d *Document) changeForEdits(edits []Edit) Change {
	if len(edits) == 1 {
		return Change{BeforeVersion: d.version - 1, Version: d.version, Edits: d.singleEditSlice(edits[0])}
	}
	return Change{BeforeVersion: d.version - 1, Version: d.version, Edits: copyEdits(edits)}
}
