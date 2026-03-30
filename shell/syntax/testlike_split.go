package syntax

func wordTestLikeSplit(w *Word) *TestLikeSplit {
	if w == nil || len(w.Parts) == 0 {
		return nil
	}

	literalPrefixOnly := true
	return wordTestLikeSplitParts(w, w.Parts, &literalPrefixOnly)
}

func wordTestLikeSplitParts(w *Word, parts []WordPart, literalPrefixOnly *bool) *TestLikeSplit {
	if !*literalPrefixOnly {
		return nil
	}
	for _, part := range parts {
		if !*literalPrefixOnly {
			return nil
		}
		switch part := part.(type) {
		case *Lit:
			if split := wordTestLikeSplitLiteral(w, part.Pos(), part.Value); split != nil {
				return split
			}
		case *SglQuoted:
			if split := wordTestLikeSplitLiteral(w, quotedContentStart(part.Left, part.Dollar), part.Value); split != nil {
				return split
			}
		case *DblQuoted:
			if split := wordTestLikeSplitParts(w, part.Parts, literalPrefixOnly); split != nil {
				return split
			}
		default:
			*literalPrefixOnly = false
		}
	}
	return nil
}

func wordTestLikeSplitLiteral(w *Word, start Pos, text string) *TestLikeSplit {
	for i := 0; i < len(text); i++ {
		op := matchTestLikeOperator(text[i:])
		if op == "" {
			continue
		}
		opPos := posAddCol(start, i)
		opEnd := posAddCol(opPos, len(op))
		if split := buildTestLikeSplit(w, op, opPos, opEnd); split != nil {
			return split
		}
	}
	return nil
}

func matchTestLikeOperator(text string) string {
	switch {
	case len(text) >= 2 && text[:2] == "==":
		return "=="
	case len(text) >= 2 && text[:2] == "!=":
		return "!="
	case len(text) >= 2 && text[:2] == "=~":
		return "=~"
	case len(text) >= 1 && text[0] == '=':
		return "="
	default:
		return ""
	}
}

func buildTestLikeSplit(w *Word, op string, opPos, opEnd Pos) *TestLikeSplit {
	leftParts, rightParts, ok := splitWordPartsForSpan(w.Parts, opPos, opEnd)
	if !ok || len(leftParts) == 0 || len(rightParts) == 0 {
		return nil
	}
	split := &TestLikeSplit{
		Left:        newSyntheticWord(leftParts, sliceWordRaw(w, w.Pos(), opPos)),
		Operator:    op,
		OperatorPos: opPos,
		OperatorEnd: opEnd,
		Right:       newSyntheticWord(rightParts, sliceWordRaw(w, opEnd, w.End())),
	}
	if split.Left == nil || split.Right == nil {
		return nil
	}
	if split.Left.UnquotedText() == "" || split.Right.UnquotedText() == "" {
		return nil
	}
	return split
}

func newSyntheticWord(parts []WordPart, raw string) *Word {
	if len(parts) == 0 {
		return nil
	}
	return &Word{
		Parts: parts,
		raw:   raw,
	}
}

func sliceWordRaw(w *Word, start, end Pos) string {
	if w == nil || w.raw == "" || !w.Pos().IsValid() || !start.IsValid() || !end.IsValid() {
		return ""
	}
	rawBase := w.Pos().Offset()
	lo := int(start.Offset() - rawBase)
	hi := int(end.Offset() - rawBase)
	if lo < 0 || hi < lo || hi > len(w.raw) {
		return ""
	}
	return w.raw[lo:hi]
}

func splitWordPartsForSpan(parts []WordPart, start, end Pos) (left, right []WordPart, ok bool) {
	found := false
	for _, part := range parts {
		if found {
			right = append(right, part)
			continue
		}
		if part.End().Offset() <= start.Offset() {
			left = append(left, part)
			continue
		}
		if part.Pos().Offset() >= end.Offset() {
			return nil, nil, false
		}

		leftPart, rightPart, partOK := splitWordPartForSpan(part, start, end)
		if !partOK {
			return nil, nil, false
		}
		if leftPart != nil {
			left = append(left, leftPart)
		}
		if rightPart != nil {
			right = append(right, rightPart)
		}
		found = true
	}
	return left, right, found
}

func splitWordPartForSpan(part WordPart, start, end Pos) (left, right WordPart, ok bool) {
	switch part := part.(type) {
	case *Lit:
		return splitLitForSpan(part, start, end)
	case *SglQuoted:
		return splitSglQuotedForSpan(part, start, end)
	case *DblQuoted:
		innerLeft, innerRight, ok := splitWordPartsForSpan(part.Parts, start, end)
		if !ok {
			return nil, nil, false
		}
		if len(innerLeft) > 0 {
			left = &DblQuoted{
				Left:   part.Left,
				Right:  posAddCol(start, -1),
				Dollar: part.Dollar,
				Parts:  innerLeft,
			}
		}
		if len(innerRight) > 0 {
			right = &DblQuoted{
				Left:   end,
				Right:  part.Right,
				Dollar: part.Dollar,
				Parts:  innerRight,
			}
		}
		return left, right, true
	default:
		return nil, nil, false
	}
}

func splitLitForSpan(lit *Lit, start, end Pos) (left, right WordPart, ok bool) {
	startIdx := int(start.Offset() - lit.Pos().Offset())
	endIdx := int(end.Offset() - lit.Pos().Offset())
	if startIdx < 0 || endIdx < startIdx || endIdx > len(lit.Value) {
		return nil, nil, false
	}
	if startIdx > 0 {
		left = &Lit{
			ValuePos: lit.ValuePos,
			ValueEnd: start,
			Value:    lit.Value[:startIdx],
		}
	}
	if endIdx < len(lit.Value) {
		right = &Lit{
			ValuePos: end,
			ValueEnd: lit.ValueEnd,
			Value:    lit.Value[endIdx:],
		}
	}
	return left, right, true
}

func splitSglQuotedForSpan(part *SglQuoted, start, end Pos) (left, right WordPart, ok bool) {
	contentStart := quotedContentStart(part.Left, part.Dollar)
	startIdx := int(start.Offset() - contentStart.Offset())
	endIdx := int(end.Offset() - contentStart.Offset())
	if startIdx < 0 || endIdx < startIdx || endIdx > len(part.Value) {
		return nil, nil, false
	}
	if startIdx > 0 {
		left = &SglQuoted{
			Left:   part.Left,
			Right:  posAddCol(start, -1),
			Dollar: part.Dollar,
			Value:  part.Value[:startIdx],
		}
	}
	if endIdx < len(part.Value) {
		right = &SglQuoted{
			Left:   end,
			Right:  part.Right,
			Dollar: part.Dollar,
			Value:  part.Value[endIdx:],
		}
	}
	return left, right, true
}

func quotedContentStart(left Pos, dollar bool) Pos {
	if dollar {
		return posAddCol(left, 2)
	}
	return posAddCol(left, 1)
}
