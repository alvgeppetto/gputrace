package trace

import (
	"encoding/binary"
	"testing"
)

func TestParseDependencyEvents(t *testing.T) {
	// Create minimal MTSP data with CS and Bind events
	// CS marker: 43 53 00 00 ... address ... label
	// Bind marker: 43 74 55 3c 62 3e 75 6c 75 6c 00 00 ...

	buf := make([]byte, 1024)
	offset := 0

	// 1. CS "Op1"
	copy(buf[offset:], []byte{0x43, 0x53, 0x00, 0x00})
	offset += 4
	binary.LittleEndian.PutUint64(buf[offset:], 0x1000) // Addr
	offset += 8
	copy(buf[offset:], []byte("Op1\x00"))
	offset += 4

	// 2. Bind Buffer A (0x2000) Write=True (0x01)
	copy(buf[offset:], []byte("CtU<b>ulul\x00\x00"))
	offset += 12
	binary.LittleEndian.PutUint64(buf[offset:], 0x1000) // Encoder
	offset += 8
	binary.LittleEndian.PutUint64(buf[offset:], 0x2000) // Buffer Addr
	offset += 8
	copy(buf[offset:], []byte("BufA\x00"))
	offset += 5
	// Flags at +11 relative to string end?
	// Our parser checks 32 bytes after string.
	// Let's put 0x01 at offset+10
	buf[offset+10] = 0x01
	offset += 32

	// 3. CS "Op2"
	copy(buf[offset:], []byte{0x43, 0x53, 0x00, 0x00})
	offset += 4
	binary.LittleEndian.PutUint64(buf[offset:], 0x1000)
	offset += 8
	copy(buf[offset:], []byte("Op2\x00"))
	offset += 4

	// 4. Use Buffer A (0x2000)
	copy(buf[offset:], []byte("Ctulul\x00\x00"))
	offset += 8
	binary.LittleEndian.PutUint64(buf[offset:], 0x9999) // Val1
	offset += 8
	binary.LittleEndian.PutUint64(buf[offset:], 0x2000) // Val2 (Buffer Addr)
	offset += 8

	trace := &Trace{CaptureData: buf[:offset]}

	events, err := trace.ParseDependencyEvents()
	if err != nil {
		t.Fatalf("ParseDependencyEvents failed: %v", err)
	}

	if len(events) != 4 {
		t.Errorf("Expected 4 events, got %d", len(events))
	}

	if events[1].Type != EventBind || !events[1].IsWrite {
		t.Errorf("Event 1 should be Bind Write, got %v write=%v", events[1].Type, events[1].IsWrite)
	}

	if events[3].Type != EventUse || events[3].Address != 0x2000 {
		t.Errorf("Event 3 should be Use 0x2000, got %v 0x%x", events[3].Type, events[3].Address)
	}

	// Test Graph Construction
	graph, err := trace.BuildDependencyGraph()
	if err != nil {
		t.Fatalf("BuildDependencyGraph failed: %v", err)
	}

	if len(graph.Nodes) != 2 {
		t.Errorf("Expected 2 nodes, got %d", len(graph.Nodes))
	}

	if len(graph.Edges) != 1 {
		t.Errorf("Expected 1 edge, got %d", len(graph.Edges))
	} else {
		edge := graph.Edges[0]
		if edge.From != 0 || edge.To != 1 {
			t.Errorf("Expected edge 0->1, got %d->%d", edge.From, edge.To)
		}
	}
}
