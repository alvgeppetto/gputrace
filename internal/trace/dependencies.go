package trace

import (
	"bytes"
	"encoding/binary"
	"fmt"
)

// DependencyGraph represents the data flow between operations.
type DependencyGraph struct {
	Nodes []DependencyNode
	Edges []DependencyEdge
}

type DependencyNode struct {
	ID    int
	Label string
}

type DependencyEdge struct {
	From   int
	To     int
	Buffer string // Name of the buffer causing dependency
}

// DependencyEvent represents a trace event relevant to dependencies.
type DependencyEvent struct {
	Offset    int64
	Type      EventType
	Label     string // For CS
	Address   uint64 // For Bind/Use
	Name      string // For Bind
	IsWrite   bool   // For Bind (heuristic)
}

type EventType int

const (
	EventCS EventType = iota
	EventBind
	EventUse
)

// BuildDependencyGraph analyzes the trace to construct a dependency graph.
func (t *Trace) BuildDependencyGraph() (*DependencyGraph, error) {
	events, err := t.ParseDependencyEvents()
	if err != nil {
		return nil, err
	}

	graph := &DependencyGraph{}

	// Track buffer state: Address -> Last Writer Node ID
	lastWriter := make(map[uint64]int)
	// Track buffer names: Address -> Name
	bufferNames := make(map[uint64]string)

	currentNodeID := -1

	// Create "Root" node for initial inputs?
	// For now, implicit.

	for _, ev := range events {
		switch ev.Type {
		case EventCS:
			// New Operation/Node
			currentNodeID = len(graph.Nodes)
			graph.Nodes = append(graph.Nodes, DependencyNode{
				ID:    currentNodeID,
				Label: ev.Label,
			})

		case EventBind:
			bufferNames[ev.Address] = ev.Name
			if currentNodeID != -1 {
				if ev.IsWrite {
					// Current node writes to this buffer
					lastWriter[ev.Address] = currentNodeID
				} else {
					// Current node reads from this buffer
					if writerID, ok := lastWriter[ev.Address]; ok && writerID != currentNodeID {
						// Add edge
						graph.Edges = append(graph.Edges, DependencyEdge{
							From:   writerID,
							To:     currentNodeID,
							Buffer: ev.Name,
						})
					}
				}
			}

		case EventUse:
			if currentNodeID != -1 {
				// Usage implies dependency on the last writer
				// We treat 'Use' as a Read for now, unless we can determine otherwise.
				// If the current node *is* the last writer (e.g. it bound it as write),
				// then this is just using its own output (no self-dependency needed).

				if writerID, ok := lastWriter[ev.Address]; ok && writerID != currentNodeID {
					// Avoid duplicate edges?
					// For now, record all, dedupe later if needed.
					name := bufferNames[ev.Address]
					if name == "" {
						name = fmt.Sprintf("0x%x", ev.Address)
					}
					graph.Edges = append(graph.Edges, DependencyEdge{
						From:   writerID,
						To:     currentNodeID,
						Buffer: name,
					})
				}
			}
		}
	}

	// Deduplicate edges
	graph.Edges = deduplicateEdges(graph.Edges)

	return graph, nil
}

func deduplicateEdges(edges []DependencyEdge) []DependencyEdge {
	seen := make(map[string]bool)
	var unique []DependencyEdge
	for _, e := range edges {
		key := fmt.Sprintf("%d-%d", e.From, e.To)
		// We only care about unique From-To pairs for visualization,
		// but maybe we want to list all buffers?
		// Let's keep one edge per pair, but maybe concatenate buffer names?
		if !seen[key] {
			seen[key] = true
			unique = append(unique, e)
		}
	}
	return unique
}

// ParseDependencyEvents extracts relevant events from the capture file.
func (t *Trace) ParseDependencyEvents() ([]DependencyEvent, error) {
	data := t.CaptureData
	if len(data) == 0 {
		return nil, fmt.Errorf("no capture data")
	}

	var events []DependencyEvent

	// Markers
	csMarker := []byte("CS\x00\x00")
	ctBindMarker := []byte("CtU<b>ulul\x00\x00")
	ctUseMarker := []byte("Ctulul\x00\x00")

	offset := 0
	for offset < len(data) {
		// Find next markers
		// This is inefficient (O(N*3)), can be optimized if needed.
		csPos := bytes.Index(data[offset:], csMarker)
		bindPos := bytes.Index(data[offset:], ctBindMarker)
		usePos := bytes.Index(data[offset:], ctUseMarker)

		// Find the earliest marker
		nextPos := -1
		markerType := 0 // 1=CS, 2=Bind, 3=Use

		if csPos != -1 {
			nextPos = csPos
			markerType = 1
		}
		if bindPos != -1 {
			if nextPos == -1 || bindPos < nextPos {
				nextPos = bindPos
				markerType = 2
			}
		}
		if usePos != -1 {
			if nextPos == -1 || usePos < nextPos {
				nextPos = usePos
				markerType = 3
			}
		}

		if nextPos == -1 {
			break
		}

		absolutePos := offset + nextPos

		switch markerType {
		case 1: // CS
			if absolutePos+12 <= len(data) {
				// Label starts at +12
				labelStart := absolutePos + 12
				labelEnd := labelStart
				for labelEnd < len(data) && data[labelEnd] != 0 {
					labelEnd++
				}
				label := string(data[labelStart:labelEnd])

				events = append(events, DependencyEvent{
					Offset: int64(absolutePos),
					Type:   EventCS,
					Label:  label,
				})
				offset = labelEnd
			} else {
				offset = absolutePos + 4
			}

		case 2: // Bind
			base := absolutePos + 12
			if base+16 <= len(data) {
				// EncoderAddr := binary.LittleEndian.Uint64(data[base : base+8])
				bufferAddr := binary.LittleEndian.Uint64(data[base+8 : base+16])

				strStart := base + 16
				strEnd := strStart
				for strEnd < len(data) && data[strEnd] != 0 {
					strEnd++
				}
				if strEnd < len(data) {
					name := string(data[strStart:strEnd])

					// Parse Flags (heuristic)
					// Check bytes after string. Look for 0x01 or 0x80.
					isWrite := false
					checkStart := strEnd + 1
					checkEnd := checkStart + 32
					if checkEnd > len(data) { checkEnd = len(data) }

					// In Squeeze example: ... 80 01 ...
					// 0x01 usually means Write access in Metal (MTLResourceUsageWrite)
					for k := checkStart; k < checkEnd; k++ {
						if data[k] == 0x01 { // Found a write flag
							isWrite = true
							break
						}
					}

					events = append(events, DependencyEvent{
						Offset:  int64(absolutePos),
						Type:    EventBind,
						Address: bufferAddr,
						Name:    name,
						IsWrite: isWrite,
					})
					offset = strEnd
				} else {
					offset += 12
				}
			} else {
				offset += 12
			}

		case 3: // Use
			base := absolutePos + 8 // Marker is 8 bytes
			if base+16 <= len(data) {
				// val1 := binary.LittleEndian.Uint64(data[base : base+8])
				val2 := binary.LittleEndian.Uint64(data[base+8 : base+16])

				events = append(events, DependencyEvent{
					Offset:  int64(absolutePos),
					Type:    EventUse,
					Address: val2,
				})
				offset = base + 16
			} else {
				offset += 8
			}
		}

		offset++
	}

	return events, nil
}
