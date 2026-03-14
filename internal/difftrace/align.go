package difftrace

import (
	"math"
	"sort"
)

// AlignDispatches matches dispatches between two traces.
func AlignDispatches(a, b *TraceData, opts AlignOptions) AlignmentResult {
	if opts.SequenceDPCellLimit <= 0 {
		opts.SequenceDPCellLimit = 120000
	}
	result := AlignmentResult{
		TraceA: append([]Dispatch(nil), a.Dispatches...),
		TraceB: append([]Dispatch(nil), b.Dispatches...),
	}

	matchedA := make([]bool, len(result.TraceA))
	matchedB := make([]bool, len(result.TraceB))

	byKeyB := make(map[string][]int)
	for i, d := range result.TraceB {
		byKeyB[d.FunctionKey] = append(byKeyB[d.FunctionKey], i)
	}
	nextByKey := make(map[string]int)
	lastB := -1

	for ai, da := range result.TraceA {
		list := byKeyB[da.FunctionKey]
		pos := nextByKey[da.FunctionKey]
		for pos < len(list) && list[pos] <= lastB {
			pos++
		}
		if pos >= len(list) {
			nextByKey[da.FunctionKey] = pos
			continue
		}
		bi := list[pos]
		nextByKey[da.FunctionKey] = pos + 1
		lastB = bi
		matchedA[ai] = true
		matchedB[bi] = true
		result.Matches = append(result.Matches, newMatch(result.TraceA[ai], result.TraceB[bi], "function_occurrence", 0.95))
	}

	// Sequence fallback for unmatched regions.
	anchors := make([]struct{ ai, bi int }, 0, len(result.Matches)+2)
	anchors = append(anchors, struct{ ai, bi int }{-1, -1})
	for _, m := range result.Matches {
		ai := findDispatchBySourceIndex(result.TraceA, m.SourceIndexA)
		bi := findDispatchBySourceIndex(result.TraceB, m.SourceIndexB)
		if ai >= 0 && bi >= 0 {
			anchors = append(anchors, struct{ ai, bi int }{ai, bi})
		}
	}
	sort.Slice(anchors, func(i, j int) bool {
		if anchors[i].ai == anchors[j].ai {
			return anchors[i].bi < anchors[j].bi
		}
		return anchors[i].ai < anchors[j].ai
	})
	anchors = append(anchors, struct{ ai, bi int }{len(result.TraceA), len(result.TraceB)})

	for i := 0; i+1 < len(anchors); i++ {
		left := anchors[i]
		right := anchors[i+1]
		ua := collectRangeUnmatched(matchedA, left.ai+1, right.ai)
		ub := collectRangeUnmatched(matchedB, left.bi+1, right.bi)
		if len(ua) == 0 || len(ub) == 0 {
			continue
		}
		pairs := alignRegion(result.TraceA, result.TraceB, ua, ub, opts.SequenceDPCellLimit)
		for _, p := range pairs {
			if matchedA[p[0]] || matchedB[p[1]] {
				continue
			}
			matchedA[p[0]] = true
			matchedB[p[1]] = true
			da := result.TraceA[p[0]]
			db := result.TraceB[p[1]]
			method := "sequence_alignment"
			conf := 0.72
			if da.FunctionKey == db.FunctionKey {
				conf = 0.78
			} else if da.PipelineID == db.PipelineID {
				method = "sequence_pipeline"
				conf = 0.60
			}
			result.Matches = append(result.Matches, newMatch(da, db, method, conf))
		}
	}

	sort.Slice(result.Matches, func(i, j int) bool {
		if result.Matches[i].SourceIndexA == result.Matches[j].SourceIndexA {
			return result.Matches[i].SourceIndexB < result.Matches[j].SourceIndexB
		}
		return result.Matches[i].SourceIndexA < result.Matches[j].SourceIndexA
	})

	for i, ok := range matchedA {
		if ok {
			continue
		}
		result.UnmatchedA = append(result.UnmatchedA, result.TraceA[i])
	}
	for i, ok := range matchedB {
		if ok {
			continue
		}
		result.UnmatchedB = append(result.UnmatchedB, result.TraceB[i])
	}

	return result
}

func newMatch(a, b Dispatch, method string, confidence float64) MatchPair {
	name := a.FunctionName
	if name == "" {
		name = b.FunctionName
	}
	kernelID := a.KernelID
	if kernelID == "" {
		kernelID = b.KernelID
	}
	if kernelID == "" {
		kernelID = kernelIdentity(name, a.PipelineHash, a.ThreadgroupSig)
	}
	enc := a.EncoderIndex
	if enc < 0 {
		enc = b.EncoderIndex
	}
	return MatchPair{
		SourceIndexA:    a.SourceIndex,
		SourceIndexB:    b.SourceIndex,
		FunctionName:    name,
		KernelID:        kernelID,
		EncoderIndex:    enc,
		PipelineIDA:     a.PipelineID,
		PipelineIDB:     b.PipelineID,
		PipelineHashA:   a.PipelineHash,
		PipelineHashB:   b.PipelineHash,
		ThreadgroupSigA: a.ThreadgroupSig,
		ThreadgroupSigB: b.ThreadgroupSig,
		DurationAUs:     a.DurationUs,
		DurationBUs:     b.DurationUs,
		DeltaUs:         a.DurationUs - b.DurationUs,
		MatchMethod:     method,
		Confidence:      confidence,
	}
}

func collectRangeUnmatched(matched []bool, from, to int) []int {
	if from < 0 {
		from = 0
	}
	if to > len(matched) {
		to = len(matched)
	}
	if from >= to {
		return nil
	}
	out := make([]int, 0, to-from)
	for i := from; i < to; i++ {
		if !matched[i] {
			out = append(out, i)
		}
	}
	return out
}

func alignRegion(a, b []Dispatch, ia, ib []int, dpCellLimit int) [][2]int {
	if len(ia) == 0 || len(ib) == 0 {
		return nil
	}
	if len(ia)*len(ib) <= dpCellLimit {
		return alignRegionDP(a, b, ia, ib)
	}
	return alignRegionGreedy(a, b, ia, ib)
}

func alignRegionDP(a, b []Dispatch, ia, ib []int) [][2]int {
	m := len(ia)
	n := len(ib)
	cols := n + 1
	dp := make([]int, (m+1)*(n+1))
	bt := make([]byte, (m+1)*(n+1)) // 1 diag, 2 up, 3 left
	gap := -3

	idx := func(i, j int) int { return i*cols + j }
	for i := 1; i <= m; i++ {
		dp[idx(i, 0)] = dp[idx(i-1, 0)] + gap
		bt[idx(i, 0)] = 2
	}
	for j := 1; j <= n; j++ {
		dp[idx(0, j)] = dp[idx(0, j-1)] + gap
		bt[idx(0, j)] = 3
	}

	for i := 1; i <= m; i++ {
		da := a[ia[i-1]]
		for j := 1; j <= n; j++ {
			db := b[ib[j-1]]
			diag := dp[idx(i-1, j-1)] + pairScore(da, db)
			up := dp[idx(i-1, j)] + gap
			left := dp[idx(i, j-1)] + gap
			best := diag
			move := byte(1)
			if up > best {
				best = up
				move = 2
			}
			if left > best {
				best = left
				move = 3
			}
			dp[idx(i, j)] = best
			bt[idx(i, j)] = move
		}
	}

	var pairs [][2]int
	i, j := m, n
	for i > 0 && j > 0 {
		move := bt[idx(i, j)]
		switch move {
		case 1:
			da := a[ia[i-1]]
			db := b[ib[j-1]]
			if pairScore(da, db) > 0 {
				pairs = append(pairs, [2]int{ia[i-1], ib[j-1]})
			}
			i--
			j--
		case 2:
			i--
		case 3:
			j--
		default:
			i--
			j--
		}
	}
	for l, r := 0, len(pairs)-1; l < r; l, r = l+1, r-1 {
		pairs[l], pairs[r] = pairs[r], pairs[l]
	}
	return pairs
}

func pairScore(a, b Dispatch) int {
	score := -2
	switch {
	case a.FunctionKey != "" && a.FunctionKey == b.FunctionKey:
		score = 6
	case a.FunctionName == "" && b.FunctionName == "" && a.PipelineID == b.PipelineID:
		score = 5
	case a.PipelineID != 0 && a.PipelineID == b.PipelineID:
		score = 2
	case a.EncoderIndex == b.EncoderIndex:
		score = 1
	}
	if absInt(a.DurationUs-b.DurationUs) <= 8 {
		score++
	}
	if absInt(a.DurationUs-b.DurationUs) >= 300 {
		score--
	}
	return score
}

func alignRegionGreedy(a, b []Dispatch, ia, ib []int) [][2]int {
	var pairs [][2]int
	i, j := 0, 0
	for i < len(ia) && j < len(ib) {
		da := a[ia[i]]
		db := b[ib[j]]
		if da.FunctionKey == db.FunctionKey || (da.FunctionName == "" && db.FunctionName == "" && da.PipelineID == db.PipelineID) {
			pairs = append(pairs, [2]int{ia[i], ib[j]})
			i++
			j++
			continue
		}
		if matchAhead(a, b, ia, ib, i, j, 4) {
			j++
			continue
		}
		i++
	}
	return pairs
}

func matchAhead(a, b []Dispatch, ia, ib []int, i, j, maxAhead int) bool {
	da := a[ia[i]]
	for k := 1; k <= maxAhead && j+k < len(ib); k++ {
		db := b[ib[j+k]]
		if da.FunctionKey == db.FunctionKey {
			return true
		}
	}
	return false
}

func findDispatchBySourceIndex(ds []Dispatch, source int) int {
	for i := range ds {
		if ds[i].SourceIndex == source {
			return i
		}
	}
	return -1
}

func absInt(v int) int {
	return int(math.Abs(float64(v)))
}
