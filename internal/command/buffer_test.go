package command

import (
	"testing"
)

func TestParseCommandBuffers(t *testing.T) {
	trace := &Trace{
		Path: "/tmp/llm-tool_1762199057.gputrace",
	}

	commandBuffers, err := trace.ParseCommandBuffers()
	if err != nil {
		t.Fatalf("ParseCommandBuffers failed: %v", err)
	}

	t.Logf("Found %d command buffers", len(commandBuffers))

	// Verify we found command buffers
	if len(commandBuffers) == 0 {
		t.Error("Expected to find command buffers, got 0")
	}

	// Show first few command buffers
	for i, cb := range commandBuffers {
		if i >= 5 {
			break
		}
		t.Logf("CommandBuffer[%d]: UUID=%s, Timestamp=%d, Offset=0x%x",
			cb.Index, cb.UUID, cb.Timestamp, cb.Offset)
	}
}

func TestParseIndex(t *testing.T) {
	trace := &Trace{
		Path: "/tmp/llm-tool_1762199057.gputrace",
	}

	index, err := trace.ParseIndex()
	if err != nil {
		t.Fatalf("ParseIndex failed: %v", err)
	}

	t.Logf("Index version: %d", index.Version)
	t.Logf("Entry size: %d", index.EntrySize)
	t.Logf("Entry count: %d", index.EntryCount)
	t.Logf("Unique function indices: %d", len(index.Entries))
}

func TestCountCommandBuffers(t *testing.T) {
	trace := &Trace{
		Path: "/tmp/llm-tool_1762199057.gputrace",
	}

	count, err := trace.CountCommandBuffers()
	if err != nil {
		t.Fatalf("CountCommandBuffers failed: %v", err)
	}

	t.Logf("Total command buffers: %d", count)

	// The user mentioned there should be 70 command buffers in this trace
	expectedCount := 70
	if count != expectedCount {
		t.Errorf("Expected %d command buffers, got %d", expectedCount, count)
	}
}
