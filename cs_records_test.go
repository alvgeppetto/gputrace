package gputrace

import (
	"testing"
)

func TestParseCSRecords(t *testing.T) {
	trace, err := Open("/tmp/fast-llm-mlx-test.gputrace")
	if err != nil {
		t.Skip("Test trace not available:", err)
	}

	records, err := trace.ParseCSRecords()
	if err != nil {
		t.Fatalf("ParseCSRecords failed: %v", err)
	}

	t.Logf("Found %d CS records", len(records))

	if len(records) == 0 {
		t.Fatal("Expected at least one CS record")
	}

	// Check that we have both types
	kernelCount := 0
	uuidCount := 0
	for _, rec := range records {
		if rec.IsKernelName {
			kernelCount++
			t.Logf("Kernel: %s (addr: 0x%x)", rec.Identifier, rec.Address)
		} else {
			uuidCount++
			t.Logf("UUID: %s (addr: 0x%x)", rec.Identifier, rec.Address)
		}
	}

	t.Logf("Kernel names: %d, UUIDs: %d", kernelCount, uuidCount)

	if kernelCount == 0 {
		t.Error("Expected at least one kernel name CS record")
	}
}

func TestGetKernelNameCSRecords(t *testing.T) {
	trace, err := Open("/tmp/fast-llm-mlx-test.gputrace")
	if err != nil {
		t.Skip("Test trace not available:", err)
	}

	kernels, err := trace.GetKernelNameCSRecords()
	if err != nil {
		t.Fatalf("GetKernelNameCSRecords failed: %v", err)
	}

	t.Logf("Found %d kernel name CS records", len(kernels))

	// Verify some expected kernels from the trace
	expectedKernels := []string{
		"vs_Multiplyfloat32",
		"block_softmax_float32",
		"g3_copyfloat32float32",
		"vv_Addfloat32",
	}

	found := make(map[string]bool)
	for _, rec := range kernels {
		found[rec.Identifier] = true
		t.Logf("Kernel: %s", rec.Identifier)
	}

	for _, expected := range expectedKernels {
		if !found[expected] {
			t.Errorf("Expected kernel '%s' not found", expected)
		}
	}
}

func TestFormatCSRecords(t *testing.T) {
	trace, err := Open("/tmp/fast-llm-mlx-test.gputrace")
	if err != nil {
		t.Skip("Test trace not available:", err)
	}

	records, err := trace.ParseCSRecords()
	if err != nil {
		t.Fatalf("ParseCSRecords failed: %v", err)
	}

	// Take first 10 records for formatting test
	sample := records
	if len(sample) > 10 {
		sample = records[:10]
	}

	output := FormatCSRecords(sample)
	t.Log(output)

	if len(output) == 0 {
		t.Error("Expected non-empty formatted output")
	}
}

func TestCountCSRecords(t *testing.T) {
	trace, err := Open("/tmp/fast-llm-mlx-test.gputrace")
	if err != nil {
		t.Skip("Test trace not available:", err)
	}

	count, err := trace.CountCSRecords()
	if err != nil {
		t.Fatalf("CountCSRecords failed: %v", err)
	}

	t.Logf("Total CS records: %d", count)

	if count == 0 {
		t.Error("Expected at least one CS record")
	}
}
