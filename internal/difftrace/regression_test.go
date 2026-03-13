//go:build darwin

package difftrace

import (
	"os"
	"testing"
)

func TestRegression_GoVsPythonPerfdata(t *testing.T) {
	goTrace := os.Getenv("GPUTRACE_DIFFTRACE_GO_TRACE")
	pyTrace := os.Getenv("GPUTRACE_DIFFTRACE_PY_TRACE")
	if goTrace == "" || pyTrace == "" {
		t.Skip("set GPUTRACE_DIFFTRACE_GO_TRACE and GPUTRACE_DIFFTRACE_PY_TRACE to run this regression test")
	}

	if _, err := os.Stat(goTrace); err != nil {
		t.Skipf("missing go trace: %v", err)
	}
	if _, err := os.Stat(pyTrace); err != nil {
		t.Skipf("missing python trace: %v", err)
	}

	a, err := LoadTraceData(goTrace, -1, nil)
	if err != nil {
		t.Fatalf("load go trace: %v", err)
	}
	b, err := LoadTraceData(pyTrace, -1, nil)
	if err != nil {
		t.Fatalf("load py trace: %v", err)
	}
	aligned := AlignDispatches(a, b, AlignOptions{})
	report := BuildReport(a, b, aligned, ReportOptions{Limit: 100, MinDeltaUs: 0})
	t.Logf("summary: delta=%dus count_delta=%d cause=%q", report.Summary.TotalDeltaUs, report.Summary.DispatchCountDelta, report.Summary.LikelyCause)
	if len(report.TopDispatchOutliers) > 0 {
		top := report.TopDispatchOutliers[0]
		t.Logf("top outlier: fn=%q enc=%d a_idx=%d b_idx=%d delta=%dus", safeFunctionName(top.FunctionName), top.EncoderIndex, top.SourceIndexA, top.SourceIndexB, top.DeltaUs)
	}

	if report.Summary.TotalDeltaUs <= 1000 {
		t.Fatalf("unexpected total delta: got %d us, want > 1000 us", report.Summary.TotalDeltaUs)
	}

	mustContain := map[string]bool{
		"affine_qmm_t_float16_t_gs_64_b_4_alN_true_batch_0": false,
		"gg2_copyfloat16float16":                            false,
		"gather_frontfloat16_int32_int_2":                   false,
		"g2_Addfloat16":                                     false,
	}
	for _, fd := range report.TopFunctionDeltas {
		if _, ok := mustContain[fd.FunctionName]; ok {
			mustContain[fd.FunctionName] = true
		}
	}
	for name, ok := range mustContain {
		if !ok {
			t.Fatalf("missing expected function delta: %s", name)
		}
	}

	if len(report.TimelineSpikeWindows) == 0 {
		t.Fatalf("expected at least one spike window")
	}
	foundEncoder2 := false
	for _, w := range report.TimelineSpikeWindows {
		if w.EncoderIndex == 2 {
			foundEncoder2 = true
			break
		}
	}
	if !foundEncoder2 {
		t.Fatalf("expected at least one spike window in encoder 2")
	}

	if len(report.UnnamedDispatchDeltas) == 0 {
		t.Fatalf("expected unnamed dispatch summary to be non-empty")
	}

	encoder2MatchedDelta := 0
	for _, m := range report.MatchedPairs {
		if m.EncoderIndex == 2 {
			encoder2MatchedDelta += m.DeltaUs
		}
	}
	if encoder2MatchedDelta <= 0 {
		t.Fatalf("expected positive encoder 2 matched delta, got %d us", encoder2MatchedDelta)
	}
	if encoder2MatchedDelta < 900 {
		t.Fatalf("expected encoder 2 matched delta >= 900us, got %d us", encoder2MatchedDelta)
	}
}
