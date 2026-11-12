package nativeforms

import "sort"

func (e *CodeEditor) usesFastNormalState() bool {
	if e.Document == nil {
		return true
	}
	if e.Lexer == nil || e.Lexer == defaultGoSyntaxLexer {
		return !e.Document.MayContainMultilineTokens()
	}
	if e.Lexer == defaultGoModSyntaxLexer || e.Lexer == defaultConfigLexer || e.Lexer == defaultPlainTextLexer {
		return true
	}
	return !e.Document.MayContainMultilineTokens()
}

func (e *CodeEditor) advanceLineState(line string, state goLexState) goLexState {
	if e.Lexer != nil {
		return e.Lexer.AdvanceState(line, state)
	}
	return advanceGoLexState(line, state)
}

func (e *CodeEditor) lexLineInto(line string, state goLexState, dst []StyledRange) ([]StyledRange, goLexState) {
	if e.Lexer != nil {
		return e.Lexer.LexLine(line, state, dst)
	}
	return scanGoLineInto(line, state, true, dst)
}

func (e *CodeEditor) goStateAt(line int) goLexState {
	if len(e.lexStates) == 0 {
		e.lexStates = sharedNormalLexStates[:1:1]
		e.lexStatesShared = true
	}
	if len(e.lexStates) <= line {
		e.rebuildLexStates(len(e.lexStates) - 1)
	}
	return e.lexStates[line]
}

func (e *CodeEditor) ensureWritableLexStates(count int, copyCount int) {
	if !e.lexStatesShared && cap(e.lexStates) >= count+1 {
		e.lexStates = e.lexStates[:count+1]
		return
	}
	grown := make([]goLexState, count+1, max(count+1, len(e.lexStates)*2))
	if copyCount > 0 {
		copy(grown, e.lexStates[:min(copyCount, len(e.lexStates))])
	}
	e.lexStates = grown
	e.lexStatesShared = false
}

// Carry states are prepared once per version, outside the paint loop. A cold
// jump then only tokenizes the visible lines.
func (e *CodeEditor) rebuildLexStates(fromLine int) {
	if e.Document == nil {
		return
	}
	count := e.Document.LineCount()
	fastNormal := e.usesFastNormalState()
	if fastNormal && count+1 <= len(sharedNormalLexStates) {
		if e.lexStatesShared || fromLine <= 0 || e.lexStates[min(fromLine, len(e.lexStates)-1)] == goNormal {
			e.lexStates = sharedNormalLexStates[: count+1 : count+1]
			e.lexStatesShared = true
			return
		}
	}
	if fromLine < 0 || fromLine >= len(e.lexStates) {
		fromLine = 0
		e.ensureWritableLexStates(count, 0)
		e.lexStates[0] = goNormal
	} else {
		e.ensureWritableLexStates(count, fromLine+1)
	}
	if fromLine >= count {
		return
	}
	state := e.lexStates[fromLine]
	if state == goNormal && fastNormal {
		clear(e.lexStates[fromLine+1 : count+1])
		return
	}
	e.scratchLineBytes = e.Document.ScanLinesFrom(fromLine, e.scratchLineBytes[:0], func(line int, segment string) bool {
		state = e.advanceLineState(segment, state)
		e.lexStates[line+1] = state
		return true
	})
}

func (e *CodeEditor) updateLexStatesFrom(fromLine, removedLines, insertedLines int) {
	if e.Document == nil {
		return
	}
	count := e.Document.LineCount()
	fastNormal := e.usesFastNormalState()
	if fastNormal && count+1 <= len(sharedNormalLexStates) &&
		(e.lexStatesShared || (fromLine >= 0 && fromLine < len(e.lexStates) && e.lexStates[fromLine] == goNormal)) {
		e.lexStates = sharedNormalLexStates[: count+1 : count+1]
		e.lexStatesShared = true
		return
	}
	if fromLine < 0 || len(e.lexStates) == 0 || fromLine >= len(e.lexStates) {
		e.rebuildLexStates(0)
		return
	}
	delta := insertedLines - removedLines
	if len(e.lexStates)+delta != count+1 {
		e.rebuildLexStates(fromLine)
		return
	}
	if e.lexStatesShared {
		old := e.lexStates
		e.lexStates = make([]goLexState, count+1, max(count+1, len(old)*2))
		e.lexStatesShared = false
		copy(e.lexStates, old[:fromLine+1])
		copy(e.lexStates[fromLine+1+insertedLines:], old[fromLine+1+removedLines:])
	} else if delta > 0 {
		if cap(e.lexStates) < count+1 {
			grown := make([]goLexState, len(e.lexStates)+delta, max(count+1, cap(e.lexStates)*2))
			copy(grown, e.lexStates[:fromLine+1])
			copy(grown[fromLine+1+insertedLines:], e.lexStates[fromLine+1+removedLines:])
			e.lexStates = grown
		} else {
			oldLen := len(e.lexStates)
			e.lexStates = e.lexStates[:count+1]
			copy(e.lexStates[fromLine+1+insertedLines:], e.lexStates[fromLine+1+removedLines:oldLen])
		}
	} else if delta < 0 {
		copy(e.lexStates[fromLine+1+insertedLines:], e.lexStates[fromLine+1+removedLines:])
		e.lexStates = e.lexStates[:count+1]
	}
	state := e.lexStates[fromLine]
	minScanEnd := min(count, fromLine+max(1, insertedLines+1))
	if state == goNormal && fastNormal {
		for line := fromLine; line < count; line++ {
			if line+1 >= minScanEnd && e.lexStates[line+1] == goNormal {
				return
			}
			e.lexStates[line+1] = goNormal
		}
		return
	}
	e.scratchLineBytes = e.Document.ScanLinesFrom(fromLine, e.scratchLineBytes[:0], func(line int, segment string) bool {
		state = e.advanceLineState(segment, state)
		if line+1 >= minScanEnd && e.lexStates[line+1] == state {
			return false
		}
		e.lexStates[line+1] = state
		return true
	})
}

func (e *CodeEditor) applyStyleRanges(dst []CodeStyle, layout codeLineLayout, lineStart int, ranges []StyledRange) {
	if len(dst) == 0 {
		return
	}
	lineEnd := lineStart + layout.offsets[len(layout.offsets)-1]
	index := sort.Search(len(ranges), func(i int) bool { return ranges[i].End > lineStart })
	for ; index < len(ranges) && ranges[index].Start < lineEnd; index++ {
		r := ranges[index]
		start := sort.SearchInts(layout.offsets, max(0, r.Start-lineStart))
		end := sort.SearchInts(layout.offsets, min(lineEnd-lineStart, r.End-lineStart))
		for i := start; i < end && i < len(dst); i++ {
			dst[i] = r.Style
		}
	}
}

func (e *CodeEditor) rowStyles(text string, layout codeLineLayout, line, start int) []CodeStyle {
	n := max(0, len(layout.offsets)-1)
	var styles []CodeStyle
	if cap(e.scratchStyles) < n {
		styles = make([]CodeStyle, n)
	} else {
		styles = e.scratchStyles[:n]
		clear(styles)
	}
	e.scratchStyles = styles
	local, _ := e.lexLineInto(text, e.goStateAt(line), e.scratchSpans[:0])
	e.scratchSpans = local
	e.applyStyleRanges(styles, layout, 0, local)
	e.applyStyleRanges(styles, layout, start, e.styles)
	e.applyStyleRanges(styles, layout, start, e.semantic)
	return styles
}

func (e *CodeEditor) cachedLayoutAndStyles(text string, line, start, end int) (codeLineLayout, []CodeStyle) {
	if len(e.lineCache) == 0 {
		layout := e.layoutLine(text)
		return layout, e.rowStyles(text, layout, line, start)
	}
	spec := e.effectiveFontSpec(e.fontSpec())
	fontGen := e.fontContext().layoutGeneration()
	tabW := e.TabWidth
	if tabW <= 0 {
		tabW = 4
	}
	slot := &e.lineCache[uint(line)%uint(len(e.lineCache))]
	if slot.valid &&
		slot.line == line &&
		slot.version == e.lastVersion &&
		slot.start == start &&
		slot.end == end &&
		slot.spec == spec &&
		slot.fontGen == fontGen &&
		slot.tabWidth == tabW &&
		slot.stylesLen == len(e.styles) &&
		slot.semanticLen == len(e.semantic) {
		return codeLineLayout{
			display: slot.display,
			offsets: slot.offsets,
			columns: slot.columns,
			carets:  slot.carets,
			hasTabs: slot.hasTabs,
		}, slot.styles
	}
	layout := e.layoutLine(text)
	styles := e.rowStyles(text, layout, line, start)
	slot.display = append(slot.display[:0], layout.display...)
	slot.offsets = append(slot.offsets[:0], layout.offsets...)
	slot.columns = append(slot.columns[:0], layout.columns...)
	slot.carets = append(slot.carets[:0], layout.carets...)
	slot.styles = append(slot.styles[:0], styles...)
	slot.hasTabs = layout.hasTabs
	slot.line = line
	slot.version = e.lastVersion
	slot.start = start
	slot.end = end
	slot.spec = spec
	slot.fontGen = fontGen
	slot.tabWidth = tabW
	slot.stylesLen = len(e.styles)
	slot.semanticLen = len(e.semantic)
	slot.valid = true
	return codeLineLayout{
		display: slot.display,
		offsets: slot.offsets,
		columns: slot.columns,
		carets:  slot.carets,
		hasTabs: slot.hasTabs,
	}, slot.styles
}

func isWhitespaceSpan(s string) bool {
	for i := 0; i < len(s); i++ {
		b := s[i]
		if b != ' ' && b != '\t' && b != '\r' {
			return false
		}
	}
	return true
}
