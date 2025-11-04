package command

import (
	"bytes"
	"os"
	"testing"
)

func TestParseDetailedCommandBuffer(t *testing.T) {
	trace := &Trace{
		Path: "/tmp/llm-tool_1762199057.gputrace",
	}

	// Parse the second command buffer (index 1)
	dcb, err := trace.ParseDetailedCommandBuffer(1)
	if err != nil {
		t.Fatalf("ParseDetailedCommandBuffer failed: %v", err)
	}

	t.Logf("Command Buffer #%d:", dcb.Index)
	t.Logf("  UUID: %s", dcb.UUID)
	t.Logf("  Timestamp: %d", dcb.Timestamp)
	t.Logf("  API Calls: %d", len(dcb.Calls))
	t.Logf("  Encoders: %d", len(dcb.Encoders))

	// Verify encoder
	if len(dcb.Encoders) != 1 {
		t.Errorf("Expected 1 encoder, got %d", len(dcb.Encoders))
	}

	if len(dcb.Encoders) > 0 {
		encoder := dcb.Encoders[0]
		t.Logf("\n  Encoder #0:")
		t.Logf("    Address: 0x%x", encoder.Address)
		t.Logf("    Offset: 0x%x", encoder.Offset)

		// Expected address from the example: 0x9be8edc00
		if encoder.Address != 0x9be8edc00 {
			t.Errorf("Expected encoder address 0x9be8edc00, got 0x%x", encoder.Address)
		}
	}

	// Verify API calls
	if len(dcb.Calls) == 0 {
		t.Error("Expected API calls, got 0")
	}

	t.Logf("\n  First 5 API calls:")
	for i, call := range dcb.Calls {
		if i >= 5 {
			break
		}
		t.Logf("    Call #%d: type=%d obj=0x%x target=0x%x",
			i, call.Type, call.ObjectAddr, call.TargetAddr)
	}
}

func TestDumpCommandBuffer(t *testing.T) {
	trace := &Trace{
		Path: "/tmp/llm-tool_1762199057.gputrace",
	}

	var buf bytes.Buffer
	err := trace.DumpCommandBuffer(&buf, 1)
	if err != nil {
		t.Fatalf("DumpCommandBuffer failed: %v", err)
	}

	output := buf.String()
	t.Logf("Command Buffer dump:\n%s", output)

	// Verify output contains expected elements
	if !bytes.Contains(buf.Bytes(), []byte("Command Buffer #1")) {
		t.Error("Output missing command buffer header")
	}

	if !bytes.Contains(buf.Bytes(), []byte("UUID:")) {
		t.Error("Output missing UUID")
	}

	if !bytes.Contains(buf.Bytes(), []byte("computeCommandEncoder")) {
		t.Error("Output missing encoder creation")
	}
}

func TestParseDispatchInRegion(t *testing.T) {
	trace := &Trace{
		Path: "/tmp/llm-tool_1762199057.gputrace",
	}

	// Get command buffer #1 data
	commandBuffers, err := trace.ParseCommandBuffers()
	if err != nil {
		t.Fatalf("ParseCommandBuffers failed: %v", err)
	}

	if len(commandBuffers) < 2 {
		t.Fatal("Need at least 2 command buffers for this test")
	}

	// Read capture data
	capturePath := trace.Path + "/capture"
	data, err := os.ReadFile(capturePath)
	if err != nil {
		t.Fatalf("Failed to read capture: %v", err)
	}

	cbStart := commandBuffers[1].Offset
	cbEnd := commandBuffers[2].Offset
	cbData := data[cbStart:cbEnd]

	// Parse dispatches
	dispatches, err := trace.ParseDispatchInRegion(cbData, cbStart)
	if err != nil {
		t.Fatalf("ParseDispatchInRegion failed: %v", err)
	}

	t.Logf("Found %d dispatch calls in command buffer #1", len(dispatches))

	// Command buffer #1 should have 5 dispatch calls based on the example
	expectedDispatches := 5
	if len(dispatches) != expectedDispatches {
		t.Errorf("Expected %d dispatches, got %d", expectedDispatches, len(dispatches))
	}

	// Show details of each dispatch
	for i, dispatch := range dispatches {
		t.Logf("\nDispatch #%d:", i+1)
		t.Logf("  Threads: {%d, %d, %d}", dispatch.ThreadsX, dispatch.ThreadsY, dispatch.ThreadsZ)
		t.Logf("  ThreadsPerGroup: {%d, %d, %d}",
			dispatch.ThreadsPerGroupX, dispatch.ThreadsPerGroupY, dispatch.ThreadsPerGroupZ)
		t.Logf("  Offset: 0x%x", dispatch.Offset)
	}

	// Verify first dispatch (from the example: dispatchThreads:{3072, 1, 1} threadsPerThreadgroup:{1024, 1, 1})
	if len(dispatches) > 0 {
		d := dispatches[0]
		if d.ThreadsX != 3072 || d.ThreadsY != 1 || d.ThreadsZ != 1 {
			t.Errorf("First dispatch threads mismatch: expected {3072,1,1}, got {%d,%d,%d}",
				d.ThreadsX, d.ThreadsY, d.ThreadsZ)
		}
		if d.ThreadsPerGroupX != 1024 || d.ThreadsPerGroupY != 1 || d.ThreadsPerGroupZ != 1 {
			t.Errorf("First dispatch threadsPerGroup mismatch: expected {1024,1,1}, got {%d,%d,%d}",
				d.ThreadsPerGroupX, d.ThreadsPerGroupY, d.ThreadsPerGroupZ)
		}
	}
}
