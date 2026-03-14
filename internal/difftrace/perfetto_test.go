package difftrace

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestWritePerfettoSharedIDs(t *testing.T) {
	a := &TraceData{Path: "a.gputrace", Label: "a", Dispatches: []Dispatch{
		{SourceIndex: 0, FunctionName: "foo", FunctionKey: functionKey("foo", 1), KernelID: kernelIdentity("foo", "ph1", "1x1x1/1x1x1"), PipelineID: 1, PipelineHash: "ph1", ThreadgroupSig: "1x1x1/1x1x1", EncoderIndex: 2, DurationUs: 10, CumulativeUs: 10},
		{SourceIndex: 1, FunctionName: "", FunctionKey: kernelIdentity("", "phu", "16x1x1/32x1x1"), KernelID: kernelIdentity("", "phu", "16x1x1/32x1x1"), PipelineID: 7, PipelineHash: "phu", ThreadgroupSig: "16x1x1/32x1x1", EncoderIndex: 2, DurationUs: 20, CumulativeUs: 30},
	}}
	b := &TraceData{Path: "b.gputrace", Label: "b", Dispatches: []Dispatch{
		{SourceIndex: 0, FunctionName: "foo", FunctionKey: functionKey("foo", 4), KernelID: kernelIdentity("foo", "ph2", "1x1x1/1x1x1"), PipelineID: 4, PipelineHash: "ph2", ThreadgroupSig: "1x1x1/1x1x1", EncoderIndex: 2, DurationUs: 8, CumulativeUs: 8},
		{SourceIndex: 1, FunctionName: "", FunctionKey: kernelIdentity("", "phu", "16x1x1/32x1x1"), KernelID: kernelIdentity("", "phu", "16x1x1/32x1x1"), PipelineID: 8, PipelineHash: "phu", ThreadgroupSig: "16x1x1/32x1x1", EncoderIndex: 2, DurationUs: 12, CumulativeUs: 20},
	}}

	aligned := AlignDispatches(a, b, AlignOptions{})
	outPath := filepath.Join(t.TempDir(), "diff_perfetto.json")
	if err := WritePerfetto(outPath, a, b, aligned); err != nil {
		t.Fatalf("write perfetto: %v", err)
	}
	raw, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read perfetto: %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("unmarshal perfetto: %v", err)
	}
	eventsAny, ok := payload["traceEvents"].([]any)
	if !ok || len(eventsAny) == 0 {
		t.Fatalf("traceEvents missing or empty")
	}

	hasStartFlow := false
	hasEndFlow := false
	for _, ev := range eventsAny {
		obj, ok := ev.(map[string]any)
		if !ok {
			continue
		}
		if obj["cat"] != "match" {
			continue
		}
		if obj["ph"] == "s" {
			hasStartFlow = true
		}
		if obj["ph"] == "f" {
			hasEndFlow = true
		}
	}
	if !hasStartFlow || !hasEndFlow {
		t.Fatalf("expected flow start/end events for shared match ids")
	}
}
