//go:build darwin

package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRunCommandCaptureTimesOut(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, _, err := runCommandCapture(ctx, exec.Command("/bin/sleep", "5"))
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("timeout took too long: %s", elapsed)
	}
}

func TestLimitedBufferCapsCapturedOutput(t *testing.T) {
	buf := &limitedBuffer{limit: 4}
	n, err := buf.Write([]byte("abcdef"))
	if err != nil {
		t.Fatal(err)
	}
	if n != 6 {
		t.Fatalf("Write returned %d, want 6", n)
	}
	if !bytes.Contains(buf.Bytes(), []byte("abcd")) {
		t.Fatalf("buffer did not keep prefix: %q", buf.Bytes())
	}
	if !bytes.Contains(buf.Bytes(), []byte("output truncated")) {
		t.Fatalf("buffer did not mark truncation: %q", buf.Bytes())
	}
}

func TestFilterModesAcceptsRepeatedAndCommaSeparatedValues(t *testing.T) {
	got := filterModes(
		[]string{"a", "b", "c", "d"},
		[]string{"b,c", "b", "missing"},
	)
	want := []string{"b", "c"}
	if len(got) != len(want) {
		t.Fatalf("filterModes length = %d, want %d: %v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("filterModes[%d] = %q, want %q; got %v", i, got[i], want[i], got)
		}
	}
}

func TestPrivateReplayerSourceIncludesDatasourceReadinessMode(t *testing.T) {
	data, err := os.ReadFile("private_replayer_probe.go")
	if err != nil {
		t.Fatal(err)
	}
	source := string(data)
	for _, want := range []string{
		"datasource_ready_then_query_derived_counters_encode_streamdata",
		"runDatasourceReadyThenQueryDerivedCounters",
		"readiness-query-configuration",
		"readiness-query-performance-state",
		"derived-query-after-readiness",
	} {
		if !strings.Contains(source, want) {
			t.Fatalf("private_replayer_probe.go missing %q", want)
		}
	}
}

func TestParseMemoryPressureFreePercent(t *testing.T) {
	got, ok := parseMemoryPressureFreePercent("The system has 103079215104 (6291456 pages with a page size of 16384).\nSystem-wide memory free percentage: 95%\n")
	if !ok {
		t.Fatal("parseMemoryPressureFreePercent did not parse valid output")
	}
	if got != 95 {
		t.Fatalf("parseMemoryPressureFreePercent = %d, want 95", got)
	}
}

func TestParseMemoryPressureFreePercentRejectsMissingValue(t *testing.T) {
	if got, ok := parseMemoryPressureFreePercent("memory pressure: normal\n"); ok {
		t.Fatalf("parseMemoryPressureFreePercent unexpectedly parsed %d", got)
	}
}

func TestFetchPipelineCandidatesOrSentinel(t *testing.T) {
	if got := fetchPipelineCandidatesOrSentinel("1:-1:2:3"); got != "1:-1:2:3" {
		t.Fatalf("fetchPipelineCandidatesOrSentinel kept %q, want explicit candidates", got)
	}
	if got := fetchPipelineCandidatesOrSentinel(""); got != "0:-1:0:0" {
		t.Fatalf("fetchPipelineCandidatesOrSentinel fallback = %q, want sentinel", got)
	}
}

func TestHasConcreteReplayCandidate(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   string
		want bool
	}{
		{name: "empty", in: "", want: false},
		{name: "sentinel", in: "0:-1:0:0", want: false},
		{name: "concrete", in: "0:-1:0:2213524325345637853", want: true},
		{name: "mixed", in: "0:-1:0:0,0:-1:3:4", want: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := hasConcreteReplayCandidate(tc.in); got != tc.want {
				t.Fatalf("hasConcreteReplayCandidate(%q) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

func TestCollectFetchTextureCandidatesUsesHexResourceNames(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "1EB804A569E9B5DD"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "MTLBuffer-1-0"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	got := collectFetchTextureCandidates(dir)
	if got != "0:-1:0:2213524325345637853" {
		t.Fatalf("collectFetchTextureCandidates = %q", got)
	}
}

func TestArchiveClassHintsDetectsTimelinePayloadClasses(t *testing.T) {
	got := archiveClassHints([]byte("prefix DYWorkloadGPUTimelineInfo suffix DYGPUTimelineInfo"))
	want := []string{"DYWorkloadGPUTimelineInfo", "DYGPUTimelineInfo"}
	if len(got) != len(want) {
		t.Fatalf("archiveClassHints length = %d, want %d: %v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("archiveClassHints[%d] = %q, want %q; got %v", i, got[i], want[i], got)
		}
	}
}

func TestCopyTimingRowsJSONRequiresStableIDs(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "rows.json")
	dst := filepath.Join(dir, "copied.json")
	rows := []xctraceIntervalRow{
		{
			StartNs:         10,
			DurationNs:      5,
			Process:         "ferrite",
			CommandBufferID: 7,
			EncoderID:       8,
		},
	}
	data, err := json.Marshal(rows)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(src, data, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := copyTimingRowsJSON(src, dst); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dst); err != nil {
		t.Fatal(err)
	}

	rows[0].EncoderID = 0
	data, err = json.Marshal(rows)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(src, data, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := copyTimingRowsJSON(src, dst); err == nil {
		t.Fatal("expected rows without stable IDs to be rejected")
	}
}
