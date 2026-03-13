package analysis

import (
	"fmt"
	"os"
	"testing"
)

func TestAnalyzeTraceStructure(t *testing.T) {
	tracePath := os.Getenv("GPUTRACE_ANALYZE_TEST_TRACE")
	if tracePath == "" {
		t.Skip("set GPUTRACE_ANALYZE_TEST_TRACE to run this integration test")
	}

	trace, err := Open(tracePath)
	if err != nil {
		t.Skipf("Skipping test, trace not available: %v", err)
	}

	report := trace.AnalyzeTraceStructure()
	fmt.Println(report)
}
