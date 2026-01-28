//go:build darwin

package mtlb

import (
	"os"
	"testing"
)

func TestMetalLoader(t *testing.T) {
	mtlbPath := "/tmp/mlx-lm-generate_tokens_8_to_9.gputrace/671438C4BF69309E"

	if _, err := os.Stat(mtlbPath); os.IsNotExist(err) {
		t.Skipf("MTLB test file not found: %s", mtlbPath)
	}

	data, err := os.ReadFile(mtlbPath)
	if err != nil {
		t.Fatalf("Failed to read MTLB file: %v", err)
	}

	lib, err := LoadMTLBWithMetal(data)
	if err != nil {
		t.Fatalf("LoadMTLBWithMetal failed: %v", err)
	}

	count := lib.FunctionCount()
	t.Logf("Loaded library with %d functions", count)

	if count == 0 {
		t.Error("Expected at least some functions")
	}

	// Get first 5 function names
	names := lib.FunctionNames()
	t.Logf("First 5 functions:")
	for i, name := range names {
		if i >= 5 {
			break
		}
		t.Logf("  %d: %s", i, name)
	}
}

func BenchmarkParserListFunctions(b *testing.B) {
	mtlbPath := "/tmp/mlx-lm-generate_tokens_8_to_9.gputrace/671438C4BF69309E"

	data, err := os.ReadFile(mtlbPath)
	if err != nil {
		b.Skip("MTLB test file not found")
	}

	mtlb, err := ParseMTLB(data)
	if err != nil {
		b.Fatalf("ParseMTLB failed: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = mtlb.ListFunctions()
	}
}

func BenchmarkMetalLoaderFunctionNames(b *testing.B) {
	mtlbPath := "/tmp/mlx-lm-generate_tokens_8_to_9.gputrace/671438C4BF69309E"

	data, err := os.ReadFile(mtlbPath)
	if err != nil {
		b.Skip("MTLB test file not found")
	}

	lib, err := LoadMTLBWithMetal(data)
	if err != nil {
		b.Fatalf("LoadMTLBWithMetal failed: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = lib.FunctionNames()
	}
}

func BenchmarkMetalLoaderLoad(b *testing.B) {
	mtlbPath := "/tmp/mlx-lm-generate_tokens_8_to_9.gputrace/671438C4BF69309E"

	data, err := os.ReadFile(mtlbPath)
	if err != nil {
		b.Skip("MTLB test file not found")
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = LoadMTLBWithMetal(data)
	}
}
