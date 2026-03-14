package difftrace

import "sort"

// OccurrenceMatch is a per-function-occurrence aligned pair.
type OccurrenceMatch struct {
	FunctionName       string  `json:"function_name"`
	OccurrenceOrdinalA int     `json:"occurrence_ordinal_a"`
	OccurrenceOrdinalB int     `json:"occurrence_ordinal_b"`
	SourceIndexA       int     `json:"source_index_a"`
	SourceIndexB       int     `json:"source_index_b"`
	EncoderIndex       int     `json:"encoder_index"`
	PipelineIDA        int     `json:"pipeline_id_a"`
	PipelineIDB        int     `json:"pipeline_id_b"`
	LeftUs             int     `json:"left_us"`
	RightUs            int     `json:"right_us"`
	DeltaUs            int     `json:"delta_us"`
	MatchMethod        string  `json:"match_method"`
	Confidence         float64 `json:"confidence"`
}

func buildOccurrenceMatches(aligned AlignmentResult) []OccurrenceMatch {
	ordA := make(map[int]int, len(aligned.TraceA))
	ordB := make(map[int]int, len(aligned.TraceB))

	countA := map[string]int{}
	for _, d := range aligned.TraceA {
		key := occurrenceKey(d)
		countA[key]++
		ordA[d.SourceIndex] = countA[key]
	}
	countB := map[string]int{}
	for _, d := range aligned.TraceB {
		key := occurrenceKey(d)
		countB[key]++
		ordB[d.SourceIndex] = countB[key]
	}

	out := make([]OccurrenceMatch, 0, len(aligned.Matches))
	for _, m := range aligned.Matches {
		out = append(out, OccurrenceMatch{
			FunctionName:       safeFunctionName(m.FunctionName),
			OccurrenceOrdinalA: ordA[m.SourceIndexA],
			OccurrenceOrdinalB: ordB[m.SourceIndexB],
			SourceIndexA:       m.SourceIndexA,
			SourceIndexB:       m.SourceIndexB,
			EncoderIndex:       m.EncoderIndex,
			PipelineIDA:        m.PipelineIDA,
			PipelineIDB:        m.PipelineIDB,
			LeftUs:             m.DurationAUs,
			RightUs:            m.DurationBUs,
			DeltaUs:            m.DeltaUs,
			MatchMethod:        m.MatchMethod,
			Confidence:         m.Confidence,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].FunctionName == out[j].FunctionName {
			if out[i].OccurrenceOrdinalA == out[j].OccurrenceOrdinalA {
				return out[i].SourceIndexA < out[j].SourceIndexA
			}
			return out[i].OccurrenceOrdinalA < out[j].OccurrenceOrdinalA
		}
		return out[i].FunctionName < out[j].FunctionName
	})
	return out
}

func occurrenceKey(d Dispatch) string {
	if d.FunctionName == "" {
		return "(unnamed)#" + d.FunctionKey
	}
	return d.FunctionName
}
