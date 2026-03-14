package difftrace

import "testing"

func TestBuildOccurrenceMatches(t *testing.T) {
	a := &TraceData{Dispatches: []Dispatch{
		{SourceIndex: 0, FunctionName: "f", FunctionKey: functionKey("f", 1), PipelineID: 1, DurationUs: 10},
		{SourceIndex: 1, FunctionName: "f", FunctionKey: functionKey("f", 1), PipelineID: 1, DurationUs: 11},
		{SourceIndex: 2, FunctionName: "", FunctionKey: functionKey("", 9), PipelineID: 9, DurationUs: 12},
	}}
	b := &TraceData{Dispatches: []Dispatch{
		{SourceIndex: 0, FunctionName: "f", FunctionKey: functionKey("f", 1), PipelineID: 1, DurationUs: 9},
		{SourceIndex: 1, FunctionName: "", FunctionKey: functionKey("", 9), PipelineID: 9, DurationUs: 14},
		{SourceIndex: 2, FunctionName: "f", FunctionKey: functionKey("f", 1), PipelineID: 1, DurationUs: 10},
	}}
	aligned := AlignDispatches(a, b, AlignOptions{})
	report := BuildReport(a, b, aligned, ReportOptions{Limit: 10, MinDeltaUs: 0})
	if len(report.OccurrenceMatches) != len(aligned.Matches) {
		t.Fatalf("occurrence matches=%d, aligned=%d", len(report.OccurrenceMatches), len(aligned.Matches))
	}
	foundF1 := false
	for _, m := range report.OccurrenceMatches {
		if m.FunctionName == "f" && m.OccurrenceOrdinalA == 1 && m.OccurrenceOrdinalB == 1 {
			foundF1 = true
		}
	}
	if !foundF1 {
		t.Fatalf("expected occurrence ordinal match for function f")
	}
}
