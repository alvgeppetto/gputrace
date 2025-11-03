package gputrace

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// CommandBuffer represents a Metal command buffer captured in the trace.
type CommandBuffer struct {
	// Index in the trace (0-based)
	Index int

	// Timestamp when the command buffer was committed
	Timestamp uint64

	// UUID uniquely identifying this command buffer
	UUID string

	// Offset in the capture file where this CUUU record appears
	Offset int64
}

// ComputeEncoder represents a Metal compute command encoder in the trace.
type ComputeEncoder struct {
	// Index in the trace (0-based)
	Index int

	// Address/ID of the encoder
	Address uint64

	// Label/name of the encoder (from CS record)
	Label string

	// Offset in the capture file where this CS record appears
	Offset int64
}

// DispatchCall represents a compute kernel dispatch call in the trace.
type DispatchCall struct {
	// Index in the trace (0-based)
	Index int

	// Offset in the capture file where this dispatch marker appears
	Offset int64
}

// ParseCommandBuffers extracts all command buffers from the trace by finding CUUU markers.
// CUUU markers indicate Metal Command buffer records.
func (t *Trace) ParseCommandBuffers() ([]*CommandBuffer, error) {
	capturePath := filepath.Join(t.Path, "capture")

	data, err := os.ReadFile(capturePath)
	if err != nil {
		return nil, fmt.Errorf("read capture file: %w", err)
	}

	var commandBuffers []*CommandBuffer
	cuuuMarker := []byte("CUUU")

	offset := 0
	for {
		// Find next CUUU marker
		pos := bytes.Index(data[offset:], cuuuMarker)
		if pos == -1 {
			break
		}

		absolutePos := int64(offset + pos)

		// Parse CUUU record structure:
		// +0x00: "CUUU" (4 bytes)
		// +0x04: padding? (4 bytes)
		// +0x08: timestamp (8 bytes)
		// +0x10: UUID hex string (null-terminated)

		if offset+pos+16 > len(data) {
			break
		}

		// Extract timestamp
		timestampBytes := data[offset+pos+8 : offset+pos+16]
		timestamp := binary.LittleEndian.Uint64(timestampBytes)

		// Extract UUID (null-terminated hex string after timestamp)
		uuidStart := offset + pos + 16
		uuidEnd := uuidStart
		for uuidEnd < len(data) && data[uuidEnd] != 0 {
			uuidEnd++
		}
		uuid := string(data[uuidStart:uuidEnd])

		commandBuffers = append(commandBuffers, &CommandBuffer{
			Index:     len(commandBuffers),
			Timestamp: timestamp,
			UUID:      uuid,
			Offset:    absolutePos,
		})

		offset += pos + 4
	}

	return commandBuffers, nil
}

// ParseXDICIndex parses the xdic index file to understand function call mappings.
// The index file maps function indices to offsets in the capture file.
type XDICIndex struct {
	Magic          [4]byte
	Version        uint32
	EntrySize      uint32
	EntryCount     uint32
	EntryCountCopy uint32

	// Entries maps function index to file offset(s)
	Entries map[uint32][]uint32
}

// ParseIndex reads and parses the xdic index file.
func (t *Trace) ParseIndex() (*XDICIndex, error) {
	indexPath := filepath.Join(t.Path, "index")

	f, err := os.Open(indexPath)
	if err != nil {
		return nil, fmt.Errorf("open index: %w", err)
	}
	defer f.Close()

	index := &XDICIndex{
		Entries: make(map[uint32][]uint32),
	}

	// Read header
	if err := binary.Read(f, binary.LittleEndian, &index.Magic); err != nil {
		return nil, fmt.Errorf("read magic: %w", err)
	}

	if string(index.Magic[:]) != "xdic" {
		return nil, fmt.Errorf("invalid magic: expected 'xdic', got %q", index.Magic)
	}

	// Read header values
	header := make([]uint32, 4)
	if err := binary.Read(f, binary.LittleEndian, &header); err != nil {
		return nil, fmt.Errorf("read header: %w", err)
	}

	index.Version = header[0]
	index.EntrySize = header[1]
	index.EntryCount = header[2]
	index.EntryCountCopy = header[3]

	// Skip to entry data (at offset 0x20)
	if _, err := f.Seek(0x20, io.SeekStart); err != nil {
		return nil, fmt.Errorf("seek to entries: %w", err)
	}

	// Read entries - each entry is 8 bytes (two uint32s)
	for i := uint32(0); i < index.EntryCount; i++ {
		var val1, val2 uint32
		if err := binary.Read(f, binary.LittleEndian, &val1); err != nil {
			break
		}
		if err := binary.Read(f, binary.LittleEndian, &val2); err != nil {
			break
		}

		// 0xffffffff indicates no mapping
		if val1 != 0xffffffff {
			index.Entries[i] = append(index.Entries[i], val1)
		}
		if val2 != 0xffffffff && val2 != val1 {
			index.Entries[i] = append(index.Entries[i], val2)
		}
	}

	return index, nil
}

// CountCommandBuffers returns the number of command buffers in the trace.
func (t *Trace) CountCommandBuffers() (int, error) {
	commandBuffers, err := t.ParseCommandBuffers()
	if err != nil {
		return 0, err
	}
	return len(commandBuffers), nil
}

// ParseComputeEncoders extracts all compute command encoders from the trace.
// Compute encoders are identified by CS (Compute Shader) records which contain
// encoder labels/kernel names. Each CS record represents one encoder execution.
func (t *Trace) ParseComputeEncoders() ([]*ComputeEncoder, error) {
	capturePath := filepath.Join(t.Path, "capture")

	data, err := os.ReadFile(capturePath)
	if err != nil {
		return nil, fmt.Errorf("read capture file: %w", err)
	}

	var computeEncoders []*ComputeEncoder

	// CS record structure:
	// +0x00: size (4 bytes) - typically 0x08
	// +0x04: "CS" magic (2 bytes) + padding (2 bytes)
	// +0x08: address (8 bytes)
	// +0x10: label string (null-terminated)

	for i := 0; i < len(data)-20; i++ {
		// Look for CS record marker
		if data[i] == 0x43 && data[i+1] == 0x53 {
			absolutePos := int64(i)

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

			computeEncoders = append(computeEncoders, &ComputeEncoder{
				Index:   len(computeEncoders),
				Address: address,
				Label:   label,
				Offset:  absolutePos,
			})
		}
	}

	return computeEncoders, nil
}

// isPrintableBytes checks if a byte slice contains only printable ASCII characters.
func isPrintableBytes(b []byte) bool {
	for _, c := range b {
		if c < 0x20 || c > 0x7E {
			return false
		}
	}
	return len(b) > 0
}

// CountComputeEncoders returns the number of compute encoders in the trace.
func (t *Trace) CountComputeEncoders() (int, error) {
	computeEncoders, err := t.ParseComputeEncoders()
	if err != nil {
		return 0, err
	}
	return len(computeEncoders), nil
}

// ParseDispatchCalls extracts all compute kernel dispatch calls from the trace.
// Dispatch calls are identified by the "ul@3" marker pattern.
func (t *Trace) ParseDispatchCalls() ([]*DispatchCall, error) {
	capturePath := filepath.Join(t.Path, "capture")

	data, err := os.ReadFile(capturePath)
	if err != nil {
		return nil, fmt.Errorf("read capture file: %w", err)
	}

	var dispatchCalls []*DispatchCall
	dispatchMarker := []byte("ul@3")

	offset := 0
	for {
		// Find next dispatch marker
		pos := bytes.Index(data[offset:], dispatchMarker)
		if pos == -1 {
			break
		}

		absolutePos := int64(offset + pos)

		dispatchCalls = append(dispatchCalls, &DispatchCall{
			Index:  len(dispatchCalls),
			Offset: absolutePos,
		})

		offset += pos + 4
	}

	return dispatchCalls, nil
}

// CountDispatchCalls returns the number of dispatch calls in the trace.
func (t *Trace) CountDispatchCalls() (int, error) {
	dispatchCalls, err := t.ParseDispatchCalls()
	if err != nil {
		return 0, err
	}
	return len(dispatchCalls), nil
}
