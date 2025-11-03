package gputrace

import (
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
)

// IndexData contains parsed information from the index file.
type IndexData struct {
	Magic            string
	CommandBuffers   int
	ComputeEncoders  int
	DispatchCalls    int
}

// ParseIndexFile parses the xdic index file to extract trace structure information.
func (t *Trace) ParseIndexFile() (*IndexData, error) {
	indexPath := filepath.Join(t.Path, "index")
	data, err := os.ReadFile(indexPath)
	if err != nil {
		return nil, fmt.Errorf("read index: %w", err)
	}

	if len(data) < 4 {
		return nil, fmt.Errorf("index file too small")
	}

	idx := &IndexData{
		Magic: string(data[0:4]),
	}

	if idx.Magic != MagicXDIC {
		return nil, fmt.Errorf("invalid magic: expected %s, got %s", MagicXDIC, idx.Magic)
	}

	// Search for command buffer count
	// The count appears as a repeated pair of uint32 values in the index file
	// We look for values in reasonable ranges (1-10000) that appear twice consecutively
	if count := findCommandBufferCount(data); count > 0 {
		idx.CommandBuffers = count
	}

	return idx, nil
}

// findCommandBufferCount searches the index file for the command buffer count.
// The count appears as two consecutive identical uint32 values.
func findCommandBufferCount(data []byte) int {
	// Scan through the file looking for repeated uint32 pairs
	// that might represent command buffer counts (typically 1-10000)
	candidates := make(map[int]int) // value -> occurrence count

	for i := 0; i < len(data)-7; i += 4 {
		val1 := binary.LittleEndian.Uint32(data[i : i+4])
		val2 := binary.LittleEndian.Uint32(data[i+4 : i+8])

		// Look for repeated values in a reasonable range
		if val1 == val2 && val1 > 0 && val1 < 10000 {
			candidates[int(val1)]++
		}
	}

	// The command buffer count typically appears multiple times
	// Find the most common value in a reasonable range (10-1000)
	maxCount := 0
	bestCandidate := 0

	for val, count := range candidates {
		if val >= 10 && val <= 1000 && count > maxCount {
			maxCount = count
			bestCandidate = val
		}
	}

	return bestCandidate
}
