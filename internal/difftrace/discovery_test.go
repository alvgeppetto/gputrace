package difftrace

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDiscoverBenchPair(t *testing.T) {
	dir := t.TempDir()
	mkTraceDir := func(name string, mtime time.Time) string {
		p := filepath.Join(dir, name)
		if err := os.MkdirAll(p, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", p, err)
		}
		if err := os.Chtimes(p, mtime, mtime); err != nil {
			t.Fatalf("chtimes %s: %v", p, err)
		}
		return p
	}
	mkFile := func(name string) {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatalf("write file %s: %v", p, err)
		}
	}

	now := time.Now()
	mkTraceDir("BenchmarkA_GoDecode_go_decode-perfdata.gputrace", now.Add(-2*time.Hour))
	mkTraceDir("BenchmarkA_PythonDecode_py_decode-perfdata.gputrace", now.Add(-2*time.Hour))
	mkTraceDir("BenchmarkB_GoDecode_go_decode-perfdata.gputrace", now.Add(-1*time.Hour))
	mkTraceDir("BenchmarkB_PythonDecode_py_decode-perfdata.gputrace", now.Add(-30*time.Minute))
	mkTraceDir("BenchmarkB_GoDecode_go_decode.gputrace", now.Add(-40*time.Minute))
	mkTraceDir("BenchmarkB_PythonDecode_py_decode.gputrace", now.Add(-35*time.Minute))
	mkFile("BenchmarkB_GoDecode_go_decode_counters.csv")
	mkFile("BenchmarkB_PythonDecode_py_decode_counters.csv")

	pair, err := DiscoverBenchPair(dir)
	if err != nil {
		t.Fatalf("discover pair: %v", err)
	}
	if pair.Stem != "BenchmarkB" {
		t.Fatalf("stem=%q want BenchmarkB", pair.Stem)
	}
	if filepath.Base(pair.Left) != "BenchmarkB_GoDecode_go_decode-perfdata.gputrace" {
		t.Fatalf("left=%q", pair.Left)
	}
	if filepath.Base(pair.Right) != "BenchmarkB_PythonDecode_py_decode-perfdata.gputrace" {
		t.Fatalf("right=%q", pair.Right)
	}
	if filepath.Base(pair.LeftRaw) != "BenchmarkB_GoDecode_go_decode.gputrace" {
		t.Fatalf("left raw=%q", pair.LeftRaw)
	}
	if filepath.Base(pair.RightRaw) != "BenchmarkB_PythonDecode_py_decode.gputrace" {
		t.Fatalf("right raw=%q", pair.RightRaw)
	}
	if pair.LeftCSV == "" || pair.RightCSV == "" {
		t.Fatalf("expected sibling CSV detection, got left=%q right=%q", pair.LeftCSV, pair.RightCSV)
	}
}

func TestDiscoverBenchPair_NoPairs(t *testing.T) {
	dir := t.TempDir()
	if _, err := DiscoverBenchPair(dir); err == nil {
		t.Fatalf("expected error for no pairs")
	}
}
