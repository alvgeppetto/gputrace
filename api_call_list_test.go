package gputrace

import (
	"bytes"
	"testing"
)

func TestParseAPICallList(t *testing.T) {
	trace := &Trace{
		Path: "/tmp/test_standalone.gputrace",
	}

	list, err := trace.ParseAPICallList()
	if err != nil {
		t.Fatalf("ParseAPICallList failed: %v", err)
	}

	t.Logf("Init calls: %d", len(list.InitCalls))
	t.Logf("Command buffers: %d", len(list.CommandBuffers))

	// Verify we have initialization calls
	if len(list.InitCalls) == 0 {
		t.Error("Expected initialization calls")
	}

	// Log init calls
	for _, call := range list.InitCalls {
		t.Logf("  Init #%d: type=%s addr=0x%x info=%s",
			call.CallNumber, call.Type, call.Address, call.Info)
	}

	// Verify we have command buffers
	if len(list.CommandBuffers) == 0 {
		t.Error("Expected command buffers")
	}

	// Log first command buffer calls
	if len(list.CommandBuffers) > 0 {
		cb := list.CommandBuffers[0]
		t.Logf("\nCommand Buffer #%d:", cb.Index)
		t.Logf("  Calls: %d", len(cb.Calls))

		for _, call := range cb.Calls {
			indent := ""
			if call.Indented {
				indent = "    "
			}
			t.Logf("  %s#%d: %s (details: %s)", indent, call.CallNumber, call.Type, call.Details)
		}
	}
}

func TestFormatAPICallList(t *testing.T) {
	trace := &Trace{
		Path: "/tmp/test_standalone.gputrace",
	}

	var buf bytes.Buffer
	err := trace.FormatAPICallList(&buf)
	if err != nil {
		t.Fatalf("FormatAPICallList failed: %v", err)
	}

	output := buf.String()
	t.Logf("API Call List:\n%s", output)

	// Verify output contains expected elements
	if !bytes.Contains(buf.Bytes(), []byte("newBuffer")) {
		t.Error("Output missing newBuffer call")
	}

	if !bytes.Contains(buf.Bytes(), []byte("commandBuffer")) {
		t.Error("Output missing commandBuffer call")
	}

	if !bytes.Contains(buf.Bytes(), []byte("computeCommandEncoder")) {
		t.Error("Output missing encoder call")
	}

	if !bytes.Contains(buf.Bytes(), []byte("dispatchThreadgroups")) {
		t.Error("Output missing dispatch call")
	}
}
