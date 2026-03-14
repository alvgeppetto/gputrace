package difftrace

import "testing"

func TestAlignDispatches_FunctionOccurrence(t *testing.T) {
	a := &TraceData{Dispatches: []Dispatch{
		{SourceIndex: 0, FunctionName: "foo", FunctionKey: functionKey("foo", 1), PipelineID: 1, EncoderIndex: 2, DurationUs: 10},
		{SourceIndex: 1, FunctionName: "bar", FunctionKey: functionKey("bar", 2), PipelineID: 2, EncoderIndex: 2, DurationUs: 20},
		{SourceIndex: 2, FunctionName: "foo", FunctionKey: functionKey("foo", 1), PipelineID: 1, EncoderIndex: 2, DurationUs: 11},
	}}
	b := &TraceData{Dispatches: []Dispatch{
		{SourceIndex: 0, FunctionName: "foo", FunctionKey: functionKey("foo", 1), PipelineID: 1, EncoderIndex: 2, DurationUs: 12},
		{SourceIndex: 1, FunctionName: "foo", FunctionKey: functionKey("foo", 1), PipelineID: 1, EncoderIndex: 2, DurationUs: 13},
		{SourceIndex: 2, FunctionName: "bar", FunctionKey: functionKey("bar", 2), PipelineID: 2, EncoderIndex: 2, DurationUs: 19},
	}}

	aligned := AlignDispatches(a, b, AlignOptions{})
	if len(aligned.Matches) != 2 {
		t.Fatalf("matches=%d want 2", len(aligned.Matches))
	}
	if len(aligned.UnmatchedA) != 1 {
		t.Fatalf("unmatchedA=%d want 1", len(aligned.UnmatchedA))
	}
	if len(aligned.UnmatchedB) != 1 {
		t.Fatalf("unmatchedB=%d want 1", len(aligned.UnmatchedB))
	}
	if aligned.Matches[0].FunctionName != "foo" {
		t.Fatalf("first match function=%q want foo", aligned.Matches[0].FunctionName)
	}
	if aligned.Matches[1].FunctionName != "bar" {
		t.Fatalf("second match function=%q want bar", aligned.Matches[1].FunctionName)
	}
}

func TestAlignDispatches_SequenceFallback(t *testing.T) {
	a := &TraceData{Dispatches: []Dispatch{
		{SourceIndex: 0, FunctionName: "x", FunctionKey: functionKey("x", 10), PipelineID: 10, EncoderIndex: 1, DurationUs: 50},
		{SourceIndex: 1, FunctionName: "", FunctionKey: functionKey("", 77), PipelineID: 77, EncoderIndex: 1, DurationUs: 40},
		{SourceIndex: 2, FunctionName: "y", FunctionKey: functionKey("y", 11), PipelineID: 11, EncoderIndex: 1, DurationUs: 51},
	}}
	b := &TraceData{Dispatches: []Dispatch{
		{SourceIndex: 0, FunctionName: "x", FunctionKey: functionKey("x", 10), PipelineID: 10, EncoderIndex: 1, DurationUs: 48},
		{SourceIndex: 1, FunctionName: "y", FunctionKey: functionKey("y", 11), PipelineID: 11, EncoderIndex: 1, DurationUs: 49},
		{SourceIndex: 2, FunctionName: "", FunctionKey: functionKey("", 77), PipelineID: 77, EncoderIndex: 1, DurationUs: 42},
	}}

	aligned := AlignDispatches(a, b, AlignOptions{SequenceDPCellLimit: 1000})
	if len(aligned.Matches) < 2 {
		t.Fatalf("matches=%d want >=2", len(aligned.Matches))
	}
	foundUnnamed := false
	for _, m := range aligned.Matches {
		if m.PipelineIDA == 77 || m.PipelineIDB == 77 {
			foundUnnamed = true
			break
		}
	}
	if !foundUnnamed {
		t.Fatalf("expected fallback to match unnamed pipeline 77")
	}
}
