package difftrace

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestReportJSONGolden(t *testing.T) {
	a := &TraceData{Path: "a.gputrace", Label: "a", Dispatches: []Dispatch{
		{SourceIndex: 0, FunctionName: "foo", FunctionKey: functionKey("foo", 1), PipelineID: 1, EncoderIndex: 2, DurationUs: 100},
		{SourceIndex: 1, FunctionName: "", FunctionKey: functionKey("", 9), PipelineID: 9, EncoderIndex: 2, DurationUs: 30},
		{SourceIndex: 2, FunctionName: "bar", FunctionKey: functionKey("bar", 2), PipelineID: 2, EncoderIndex: 2, DurationUs: 40},
	}}
	b := &TraceData{Path: "b.gputrace", Label: "b", Dispatches: []Dispatch{
		{SourceIndex: 0, FunctionName: "foo", FunctionKey: functionKey("foo", 1), PipelineID: 1, EncoderIndex: 2, DurationUs: 80},
		{SourceIndex: 1, FunctionName: "", FunctionKey: functionKey("", 9), PipelineID: 9, EncoderIndex: 2, DurationUs: 45},
		{SourceIndex: 2, FunctionName: "bar", FunctionKey: functionKey("bar", 2), PipelineID: 2, EncoderIndex: 2, DurationUs: 44},
	}}
	aligned := AlignDispatches(a, b, AlignOptions{})
	report := BuildReport(a, b, aligned, ReportOptions{Limit: 10, MinDeltaUs: 0})
	got, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	got = append(got, '\n')

	golden := filepath.Join("testdata", "report_golden.json")
	if os.Getenv("UPDATE_GOLDEN") == "1" {
		if err := os.WriteFile(golden, got, 0o644); err != nil {
			t.Fatalf("write golden: %v", err)
		}
	}
	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatalf("read golden: %v", err)
	}
	if string(got) != string(want) {
		t.Fatalf("golden mismatch\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}
