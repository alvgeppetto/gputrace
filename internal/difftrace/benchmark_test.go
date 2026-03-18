package difftrace

import (
	"fmt"
	"testing"
)

func BenchmarkDiffCore_2500Dispatches(b *testing.B) {
	mk := func(label string, offset int) *TraceData {
		ds := make([]Dispatch, 2500)
		for i := range ds {
			name := fmt.Sprintf("k_%d", i%37)
			if i%19 == 0 {
				name = ""
			}
			dur := 6 + (i % 11)
			if label == "a" && i%97 == 0 {
				dur += 200
			}
			ds[i] = Dispatch{
				SourceIndex:  i + offset,
				FunctionName: name,
				FunctionKey:  testFunctionKey(name, i%23),
				PipelineID:   i % 23,
				EncoderIndex: 2,
				DurationUs:   dur,
			}
		}
		return &TraceData{Label: label, Dispatches: ds}
	}
	a := mk("a", 0)
	bTrace := mk("b", 3)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		aligned := AlignDispatches(a, bTrace, AlignOptions{})
		_ = BuildReport(a, bTrace, aligned, ReportOptions{Limit: 20, MinDeltaUs: 30})
	}
}
