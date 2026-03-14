package difftrace

import (
	"encoding/json"
	"os"
	"sort"
)

type perfettoTrace struct {
	TraceEvents     []perfettoEvent `json:"traceEvents"`
	DisplayTimeUnit string          `json:"displayTimeUnit,omitempty"`
}

type perfettoEvent struct {
	Name string         `json:"name"`
	Cat  string         `json:"cat,omitempty"`
	Ph   string         `json:"ph"`
	PID  int            `json:"pid,omitempty"`
	TID  int            `json:"tid,omitempty"`
	TS   int            `json:"ts,omitempty"`
	Dur  int            `json:"dur,omitempty"`
	ID   int            `json:"id,omitempty"`
	Args map[string]any `json:"args,omitempty"`
}

// WritePerfetto writes a combined Chrome/Perfetto trace for both sides with shared match IDs.
func WritePerfetto(path string, a, b *TraceData, aligned AlignmentResult) error {
	events := make([]perfettoEvent, 0, len(aligned.TraceA)+len(aligned.TraceB)+len(aligned.Matches)*2+16)
	const leftPID = 1
	const rightPID = 2

	events = append(events,
		perfettoEvent{Name: "process_name", Ph: "M", PID: leftPID, Args: map[string]any{"name": "left (A)"}},
		perfettoEvent{Name: "process_name", Ph: "M", PID: rightPID, Args: map[string]any{"name": "right (B)"}},
	)

	for _, enc := range a.Encoders {
		events = append(events, perfettoEvent{
			Name: "thread_name",
			Ph:   "M",
			PID:  leftPID,
			TID:  enc.Index,
			Args: map[string]any{"name": "A encoder " + itoa(enc.Index)},
		})
	}
	for _, enc := range b.Encoders {
		events = append(events, perfettoEvent{
			Name: "thread_name",
			Ph:   "M",
			PID:  rightPID,
			TID:  enc.Index,
			Args: map[string]any{"name": "B encoder " + itoa(enc.Index)},
		})
	}

	aBySource := map[int]Dispatch{}
	for _, d := range aligned.TraceA {
		aBySource[d.SourceIndex] = d
	}
	bBySource := map[int]Dispatch{}
	for _, d := range aligned.TraceB {
		bBySource[d.SourceIndex] = d
	}

	matchIDA := map[int]int{}
	matchIDB := map[int]int{}
	for i, m := range aligned.Matches {
		id := i + 1
		matchIDA[m.SourceIndexA] = id
		matchIDB[m.SourceIndexB] = id
	}

	appendDispatches := func(pid int, traceName string, ds []Dispatch, matchID map[int]int) {
		sorted := append([]Dispatch(nil), ds...)
		sort.Slice(sorted, func(i, j int) bool { return sorted[i].SourceIndex < sorted[j].SourceIndex })
		for _, d := range sorted {
			args := map[string]any{
				"trace":                 traceName,
				"source_index":          d.SourceIndex,
				"pipeline_id":           d.PipelineID,
				"pipeline_hash":         d.PipelineHash,
				"threadgroup_signature": d.ThreadgroupSig,
				"function_name":         safeFunctionName(d.FunctionName),
				"kernel_id":             d.KernelID,
			}
			if id, ok := matchID[d.SourceIndex]; ok {
				args["shared_match_id"] = id
			}
			events = append(events, perfettoEvent{
				Name: safeFunctionName(d.FunctionName),
				Cat:  "dispatch",
				Ph:   "X",
				PID:  pid,
				TID:  d.EncoderIndex,
				TS:   dispatchStartUS(d),
				Dur:  d.DurationUs,
				Args: args,
			})
		}
	}

	appendDispatches(leftPID, "A", aligned.TraceA, matchIDA)
	appendDispatches(rightPID, "B", aligned.TraceB, matchIDB)

	for i, m := range aligned.Matches {
		id := i + 1
		da, okA := aBySource[m.SourceIndexA]
		db, okB := bBySource[m.SourceIndexB]
		if !okA || !okB {
			continue
		}
		events = append(events,
			perfettoEvent{
				Name: "matched_dispatch",
				Cat:  "match",
				Ph:   "s",
				PID:  leftPID,
				TID:  da.EncoderIndex,
				TS:   dispatchStartUS(da),
				ID:   id,
			},
			perfettoEvent{
				Name: "matched_dispatch",
				Cat:  "match",
				Ph:   "f",
				PID:  rightPID,
				TID:  db.EncoderIndex,
				TS:   dispatchStartUS(db),
				ID:   id,
			},
		)
	}

	out := perfettoTrace{
		TraceEvents:     events,
		DisplayTimeUnit: "us",
	}
	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o644)
}

func dispatchStartUS(d Dispatch) int {
	start := d.CumulativeUs - d.DurationUs
	if start < 0 {
		return 0
	}
	return start
}
