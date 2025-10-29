// gputrace-analyze searches for timing and performance data in .gputrace files
package main

import (
	"encoding/binary"
	"flag"
	"fmt"
	"os"
	"path/filepath"
)

func main() {
	flag.Parse()
	if flag.NArg() != 1 {
		fmt.Fprintf(os.Stderr, "Usage: %s <path-to.gputrace>\n", os.Args[0])
		os.Exit(1)
	}

	tracePath := flag.Arg(0)

	// Read capture file
	capturePath := filepath.Join(tracePath, "capture")
	data, err := os.ReadFile(capturePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading capture: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("=== Analyzing %s ===\n\n", filepath.Base(tracePath))
	fmt.Printf("Capture file size: %d bytes\n\n", len(data))

	// Search for potential timestamp patterns
	findTimestampPatterns(data)

	// Search for thread configuration (1024, 1, 1)
	findThreadConfig(data)

	// Search for dispatch counts (7, 14, etc.)
	findDispatchCounts(data)

	// Search for encoder labels with nearby uint64 values (timestamps?)
	findLabelsWithTimestamps(data)
}

func findTimestampPatterns(data []byte) {
	fmt.Println("=== Searching for uint64 timestamp patterns ===")

	// Look for sequences of uint64 values that might be timestamps
	// Mach absolute time is typically large (> 1e15)
	// or could be nanoseconds since some epoch

	candidates := make(map[uint64][]int) // value -> offsets

	for i := 0; i < len(data)-16; i++ {
		val := binary.LittleEndian.Uint64(data[i : i+8])

		// Skip obviously wrong values
		if val == 0 || val == 0xFFFFFFFFFFFFFFFF {
			continue
		}

		// Timestamps are likely in certain ranges
		// Mach time: > 1e15
		// Nanoseconds: 1e9 to 1e18
		if val > 1e9 && val < 1e18 {
			candidates[val] = append(candidates[val], i)
		}
	}

	fmt.Printf("Found %d potential timestamp values\n", len(candidates))

	// Look for patterns: pairs of uint64s (start/end timestamps)
	fmt.Println("\nLooking for timestamp pairs (start/end)...")
	pairs := 0
	for i := 0; i < len(data)-16; i++ {
		val1 := binary.LittleEndian.Uint64(data[i : i+8])
		val2 := binary.LittleEndian.Uint64(data[i+8 : i+16])

		if val1 > 1e12 && val2 > val1 && (val2-val1) < 1e10 {
			duration := val2 - val1
			fmt.Printf("  Offset 0x%04x: start=%d end=%d duration=%d (%.2fms)\n",
				i, val1, val2, duration, float64(duration)/1e6)
			pairs++
			if pairs >= 10 {
				break
			}
		}
	}
	fmt.Println()
}

func findThreadConfig(data []byte) {
	fmt.Println("=== Searching for thread configuration (1024, 1, 1) ===")

	// Pattern: 00 04 00 00  01 00 00 00  01 00 00 00
	pattern1024 := []byte{0x00, 0x04, 0x00, 0x00} // 1024 in little-endian
	pattern1 := []byte{0x01, 0x00, 0x00, 0x00}    // 1 in little-endian

	count := 0
	for i := 0; i < len(data)-24; i++ {
		if matchesPattern(data[i:], pattern1024) &&
			matchesPattern(data[i+4:], pattern1) &&
			matchesPattern(data[i+8:], pattern1) {

			// Check if followed by another (1024, 1, 1) - threadsPerThreadgroup
			if i+24 < len(data) &&
				matchesPattern(data[i+12:], pattern1024) &&
				matchesPattern(data[i+16:], pattern1) &&
				matchesPattern(data[i+20:], pattern1) {

				fmt.Printf("  Offset 0x%04x: dispatchThreads(1024,1,1) threadsPerThreadgroup(1024,1,1)\n", i)
				count++
				if count >= 5 {
					break
				}
			}
		}
	}
	if count == 0 {
		fmt.Println("  (none found)")
	}
	fmt.Println()
}

func findDispatchCounts(data []byte) {
	fmt.Println("=== Searching for dispatch counts (7, 14) ===")

	// Look for small integers (1-100) that might be dispatch counts
	// Pattern: small uint32 near encoder records

	found := make(map[uint32][]int)

	for i := 0; i < len(data)-4; i++ {
		val := binary.LittleEndian.Uint32(data[i : i+4])

		// Dispatch counts are typically 1-100
		if val >= 1 && val <= 100 {
			found[val] = append(found[val], i)
		}
	}

	fmt.Printf("Most common small integers:\n")
	for val := uint32(1); val <= 20; val++ {
		if offsets := found[val]; len(offsets) > 0 && len(offsets) < 50 {
			fmt.Printf("  Value %2d appears %d times\n", val, len(offsets))
			if val == 7 || val == 14 {
				fmt.Printf("    ** Matches expected dispatch count! **\n")
				// Show first few offsets
				for i, offset := range offsets {
					if i >= 3 {
						break
					}
					fmt.Printf("       Offset 0x%04x\n", offset)
				}
			}
		}
	}
	fmt.Println()
}

func findLabelsWithTimestamps(data []byte) {
	fmt.Println("=== Searching for encoder labels with nearby timestamps ===")

	// Find "Stage1_Normalize", "Stage2_ReLU", "Stage3_Scale"
	labels := []string{"Stage1_Normalize", "Stage2_ReLU", "Stage3_Scale"}

	for _, label := range labels {
		offset := findString(data, label)
		if offset == -1 {
			continue
		}

		fmt.Printf("\n'%s' found at offset 0x%04x\n", label, offset)

		// Look for uint64 values nearby (within +/- 128 bytes)
		start := max(0, offset-128)
		end := min(len(data)-8, offset+len(label)+128)

		fmt.Println("  Nearby uint64 values:")
		for i := start; i < end-8; i += 4 {
			val := binary.LittleEndian.Uint64(data[i : i+8])
			if val > 1e12 && val < 1e18 {
				relOffset := i - offset
				fmt.Printf("    Offset 0x%04x (label%+4d): %d", i, relOffset, val)
				if val > 1e15 {
					fmt.Printf(" (Mach time)")
				}
				fmt.Println()
			}
		}
	}
	fmt.Println()
}

// Helper functions

func matchesPattern(data []byte, pattern []byte) bool {
	if len(data) < len(pattern) {
		return false
	}
	for i := range pattern {
		if data[i] != pattern[i] {
			return false
		}
	}
	return true
}

func findString(data []byte, s string) int {
	sBytes := []byte(s)
	for i := 0; i < len(data)-len(sBytes); i++ {
		if matchesPattern(data[i:], sBytes) {
			return i
		}
	}
	return -1
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
