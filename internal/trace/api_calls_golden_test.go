package trace

import (
	"bytes"
	"os"
	"testing"
)

func TestDumpOutputMatchesExpected(t *testing.T) {
	trace := &Trace{
		Path: "/tmp/test_standalone.gputrace",
	}

	// Read expected output
	expected, err := os.ReadFile("/tmp/expected_dump.txt")
	if err != nil {
		t.Skipf("Expected output file not found: %v", err)
	}

	// Generate actual output
	var buf bytes.Buffer
	err = trace.FormatAPICallList(&buf)
	if err != nil {
		t.Fatalf("FormatAPICallList failed: %v", err)
	}

	actual := buf.Bytes()

	// Compare
	if !bytes.Equal(expected, actual) {
		t.Errorf("Output does not match expected:\nExpected:\n%s\nActual:\n%s", expected, actual)
	}
}
