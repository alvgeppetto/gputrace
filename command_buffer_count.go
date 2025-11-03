package gputrace

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// DetailedCommandBuffer represents a parsed command buffer with all API calls.
type DetailedCommandBuffer struct {
	*CommandBuffer

	// API calls within this command buffer
	Calls []APICall

	// Encoders within this command buffer
	Encoders []*ComputeEncoder
}

// APICall represents a single Metal API call.
type APICall struct {
	// Type of call (from Ct record f5 field)
	// 12 = addCompletedHandler
	// 13 = fence/barrier operations
	// 14 = setComputePipelineState or setBuffer
	Type uint32

	// Object address (from f1 field)
	ObjectAddr uint64

	// Target address (from f3 field) - e.g., pipeline state, handler, fence
	TargetAddr uint32

	// Offset in capture file
	Offset int64
}

// DispatchThreads represents a dispatchThreads or dispatchThreadgroups call.
type DispatchThreads struct {
	// Thread dimensions
	ThreadsX, ThreadsY, ThreadsZ uint64

	// Threads per threadgroup dimensions
	ThreadsPerGroupX, ThreadsPerGroupY, ThreadsPerGroupZ uint64

	// Offset in capture file
	Offset int64
}

// ParseDetailedCommandBuffer extracts all API calls from a specific command buffer.
func (t *Trace) ParseDetailedCommandBuffer(cbIndex int) (*DetailedCommandBuffer, error) {
	capturePath := filepath.Join(t.Path, "capture")

	data, err := os.ReadFile(capturePath)
	if err != nil {
		return nil, fmt.Errorf("read capture file: %w", err)
	}

	// Get all command buffers
	commandBuffers, err := t.ParseCommandBuffers()
	if err != nil {
		return nil, err
	}

	if cbIndex < 0 || cbIndex >= len(commandBuffers) {
		return nil, fmt.Errorf("invalid command buffer index: %d (have %d)", cbIndex, len(commandBuffers))
	}

	cb := commandBuffers[cbIndex]

	// Determine the region of this command buffer
	var cbStart, cbEnd int64
	cbStart = cb.Offset

	if cbIndex+1 < len(commandBuffers) {
		cbEnd = commandBuffers[cbIndex+1].Offset
	} else {
		cbEnd = int64(len(data))
	}

	cbData := data[cbStart:cbEnd]

	// Parse API calls (Ct records)
	calls, err := parseAPICallsInRegion(cbData, cbStart)
	if err != nil {
		return nil, fmt.Errorf("parse API calls: %w", err)
	}

	// Parse encoders in this command buffer
	encoders, err := parseEncodersInRegion(cbData, cbStart)
	if err != nil {
		return nil, fmt.Errorf("parse encoders: %w", err)
	}

	return &DetailedCommandBuffer{
		CommandBuffer: cb,
		Calls:         calls,
		Encoders:      encoders,
	}, nil
}

func parseAPICallsInRegion(data []byte, baseOffset int64) ([]APICall, error) {
	var calls []APICall
	ctMarker := []byte("Ct\x00\x00")

	offset := 0
	for {
		pos := bytes.Index(data[offset:], ctMarker)
		if pos == -1 {
			break
		}

		absolutePos := offset + pos

		// Parse Ct record structure:
		// +0x00: "Ct\x00\x00" (4 bytes)
		// +0x04: object address (8 bytes)
		// +0x0C: unknown (4 bytes)
		// +0x10: target address (4 bytes)
		// +0x14: unknown (4 bytes)
		// +0x18: type field (4 bytes)

		if absolutePos+24 <= len(data) {
			objectAddr := binary.LittleEndian.Uint64(data[absolutePos+4 : absolutePos+12])
			targetAddr := binary.LittleEndian.Uint32(data[absolutePos+16 : absolutePos+20])
			typeField := binary.LittleEndian.Uint32(data[absolutePos+20 : absolutePos+24])

			calls = append(calls, APICall{
				Type:       typeField,
				ObjectAddr: objectAddr,
				TargetAddr: targetAddr,
				Offset:     baseOffset + int64(absolutePos),
			})
		}

		offset += pos + 4
	}

	return calls, nil
}

func parseEncodersInRegion(data []byte, baseOffset int64) ([]*ComputeEncoder, error) {
	var encoders []*ComputeEncoder

	// CS record structure:
	// +0x00: size (4 bytes) - typically 0x08
	// +0x04: "CS" magic (2 bytes) + padding (2 bytes)
	// +0x08: address (8 bytes)
	// +0x10: label string (null-terminated)

	for i := 0; i < len(data)-20; i++ {
		// Look for CS record marker
		if data[i] == 0x43 && data[i+1] == 0x53 {
			// Extract address (8 bytes after CS marker)
			addressStart := i + 4
			if addressStart+8 > len(data) {
				continue
			}
			address := binary.LittleEndian.Uint64(data[addressStart : addressStart+8])

			// Extract label (starts 12 bytes after CS marker)
			labelStart := i + 12
			if labelStart >= len(data) {
				continue
			}

			// Find null terminator for label
			labelEnd := labelStart
			for labelEnd < len(data) && data[labelEnd] != 0 && labelEnd-labelStart < 128 {
				labelEnd++
			}

			label := ""
			if labelEnd > labelStart {
				labelBytes := data[labelStart:labelEnd]
				// Check if it looks like a valid label (printable characters)
				if isPrintableBytes(labelBytes) {
					label = string(labelBytes)
				}
			}

			encoders = append(encoders, &ComputeEncoder{
				Index:   len(encoders),
				Address: address,
				Label:   label,
				Offset:  baseOffset + int64(i),
			})
		}
	}

	return encoders, nil
}

// ParseDispatchInRegion parses dispatch calls within a command buffer region.
func (t *Trace) ParseDispatchInRegion(data []byte, baseOffset int64) ([]DispatchThreads, error) {
	var dispatches []DispatchThreads
	dispatchMarker := []byte("ul@3")

	offset := 0
	for {
		pos := bytes.Index(data[offset:], dispatchMarker)
		if pos == -1 {
			break
		}

		absolutePos := offset + pos

		// Dispatch structure (discovered by reverse engineering):
		// +0x00: "ul@3" marker (4 bytes)
		// +0x04: variable data
		// +0x11: threadsX (uint64, 8 bytes)
		// +0x19: threadsY (uint64, 8 bytes)
		// +0x21: threadsZ (uint64, 8 bytes)
		// +0x29: threadsPerGroupX (uint64, 8 bytes)
		// +0x31: threadsPerGroupY (uint64, 8 bytes)
		// +0x39: threadsPerGroupZ (uint64, 8 bytes)

		if absolutePos+0x41 <= len(data) {
			threadsX := binary.LittleEndian.Uint64(data[absolutePos+0x11 : absolutePos+0x19])
			threadsY := binary.LittleEndian.Uint64(data[absolutePos+0x19 : absolutePos+0x21])
			threadsZ := binary.LittleEndian.Uint64(data[absolutePos+0x21 : absolutePos+0x29])

			threadsPerGroupX := binary.LittleEndian.Uint64(data[absolutePos+0x29 : absolutePos+0x31])
			threadsPerGroupY := binary.LittleEndian.Uint64(data[absolutePos+0x31 : absolutePos+0x39])
			threadsPerGroupZ := binary.LittleEndian.Uint64(data[absolutePos+0x39 : absolutePos+0x41])

			dispatches = append(dispatches, DispatchThreads{
				ThreadsX:         threadsX,
				ThreadsY:         threadsY,
				ThreadsZ:         threadsZ,
				ThreadsPerGroupX: threadsPerGroupX,
				ThreadsPerGroupY: threadsPerGroupY,
				ThreadsPerGroupZ: threadsPerGroupZ,
				Offset:           baseOffset + int64(absolutePos),
			})
		}

		offset += pos + 4
	}

	return dispatches, nil
}

// FormatCommandBuffer returns a human-readable representation of a command buffer.
func (dcb *DetailedCommandBuffer) FormatCommandBuffer() string {
	var buf bytes.Buffer

	fmt.Fprintf(&buf, "Command Buffer #%d (UUID: %s)\n", dcb.Index, dcb.UUID)
	fmt.Fprintf(&buf, "  Timestamp: %d\n", dcb.Timestamp)
	fmt.Fprintf(&buf, "  Offset: 0x%08x\n", dcb.Offset)
	fmt.Fprintf(&buf, "  Encoders: %d\n", len(dcb.Encoders))
	fmt.Fprintf(&buf, "  API Calls: %d\n", len(dcb.Calls))

	return buf.String()
}

// DumpCommandBuffer writes a detailed command buffer dump similar to Instruments output.
func (t *Trace) DumpCommandBuffer(w io.Writer, cbIndex int) error {
	dcb, err := t.ParseDetailedCommandBuffer(cbIndex)
	if err != nil {
		return err
	}

	fmt.Fprintf(w, "\n=== Command Buffer #%d ===\n", cbIndex)
	fmt.Fprintf(w, "UUID: %s\n", dcb.UUID)
	fmt.Fprintf(w, "Timestamp: %d\n", dcb.Timestamp)
	if len(dcb.Encoders) > 0 {
		fmt.Fprintf(w, "Address: 0x%x\n", dcb.Encoders[0].Address) // CB address from first encoder
	}
	fmt.Fprintf(w, "\n")

	// Get dispatch info
	capturePath := filepath.Join(t.Path, "capture")
	data, err := os.ReadFile(capturePath)
	if err != nil {
		return err
	}

	var cbEnd int64
	commandBuffers, _ := t.ParseCommandBuffers()
	if cbIndex+1 < len(commandBuffers) {
		cbEnd = commandBuffers[cbIndex+1].Offset
	} else {
		cbEnd = int64(len(data))
	}

	cbData := data[dcb.Offset:cbEnd]
	dispatches, err := t.ParseDispatchInRegion(cbData, dcb.Offset)
	if err != nil {
		return err
	}

	// Format API calls
	callIdx := 524 // Start numbering like the example (adjust as needed)

	for _, encoder := range dcb.Encoders {
		fmt.Fprintf(w, "#%d 0x%x = computeCommandEncoder\n", callIdx, encoder.Address)
		callIdx++
	}

	// Print calls grouped by encoder
	dispatchIdx := 0
	for _, call := range dcb.Calls {
		switch call.Type {
		case 12:
			fmt.Fprintf(w, "#%d [addCompletedHandler:0x%08x]\n", callIdx, call.TargetAddr)
		case 13:
			// Could be fence, barrier, or pipeline state
			if call.TargetAddr&0xF000 != 0 {
				fmt.Fprintf(w, "#%d [setComputePipelineState:0x%x]\n", callIdx, call.TargetAddr)
			} else {
				fmt.Fprintf(w, "#%d [fence/barrier operation:0x%x]\n", callIdx, call.TargetAddr)
			}
		case 14:
			fmt.Fprintf(w, "#%d [setComputePipelineState:0x%x]\n", callIdx, call.TargetAddr)
		default:
			fmt.Fprintf(w, "#%d [API call type %d: 0x%x -> 0x%x]\n", callIdx, call.Type, call.ObjectAddr, call.TargetAddr)
		}
		callIdx++

		// Check if there's a dispatch call near this offset
		if dispatchIdx < len(dispatches) {
			// Simplified: just print dispatches in order
			// In reality, would need to correlate with surrounding API calls
		}
	}

	// Print dispatch calls
	for i, dispatch := range dispatches {
		fmt.Fprintf(w, "    Dispatch #%d: threads:{%d, %d, %d} threadsPerGroup:{%d, %d, %d}\n",
			i+1,
			dispatch.ThreadsX, dispatch.ThreadsY, dispatch.ThreadsZ,
			dispatch.ThreadsPerGroupX, dispatch.ThreadsPerGroupY, dispatch.ThreadsPerGroupZ)
	}

	fmt.Fprintf(w, "#%d [commit]\n", callIdx)

	return nil
}
