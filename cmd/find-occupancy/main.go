package main

import (
	"encoding/binary"
	"fmt"
	"log"
	"math"
	"os"
)

func main() {
	if len(os.Args) < 3 {
		log.Fatalf("Usage: %s <counter_file.raw> <target_value>", os.Args[0])
	}

	data, err := os.ReadFile(os.Args[1])
	if err != nil {
		log.Fatalf("Failed to read file: %v", err)
	}

	targetVal := 0.09
	if len(os.Args) > 2 {
		fmt.Sscanf(os.Args[2], "%f", &targetVal)
	}

	fmt.Printf("Searching for float32 value %.4f (±0.005) in %s...\n", targetVal, os.Args[1])
	fmt.Printf("File size: %d bytes\n\n", len(data))

	// Find all record markers (0x4E 0x00 0x00 0x00)
	recordOffsets := make([]int, 0)
	for i := 0; i < len(data)-4; i++ {
		if data[i] == 0x4E && data[i+1] == 0x00 && data[i+2] == 0x00 && data[i+3] == 0x00 {
			recordOffsets = append(recordOffsets, i)
		}
	}

	fmt.Printf("Found %d record markers\n\n", len(recordOffsets))

	// Search for target value as float32
	fmt.Printf("Scanning for float32 values near %.4f:\n\n", targetVal)

	foundMatches := 0
	for i := 0; i < len(data)-4; i += 4 {
		bits := binary.LittleEndian.Uint32(data[i : i+4])
		val := math.Float32frombits(bits)

		// Check if value is close to target
		if math.Abs(float64(val)-targetVal) < 0.005 {
			// Find which record this belongs to
			recordIdx := -1
			offsetInRecord := 0
			for ri, roff := range recordOffsets {
				if i >= roff {
					recordIdx = ri
					offsetInRecord = i - roff
				} else {
					break
				}
			}

			fmt.Printf("Offset 0x%04x: %.6f (record %d, offset +0x%03x)\n",
				i, val, recordIdx, offsetInRecord)
			foundMatches++

			// Show surrounding bytes
			if i >= 16 && i+20 < len(data) {
				fmt.Printf("  Context: ")
				for j := i - 16; j < i+20; j++ {
					if j == i {
						fmt.Printf("[%02x", data[j])
					} else if j == i+3 {
						fmt.Printf("%02x] ", data[j])
					} else {
						fmt.Printf("%02x ", data[j])
					}
				}
				fmt.Printf("\n\n")
			}
		}
	}

	if foundMatches == 0 {
		fmt.Printf("No matches found. Let's try broader search:\n\n")

		// Try searching for values in range 0.01 to 1.0
		fmt.Printf("Float32 values in range 0.01-1.0:\n")
		for i := 0; i < len(data)-4; i += 4 {
			bits := binary.LittleEndian.Uint32(data[i : i+4])
			val := math.Float32frombits(bits)

			if val >= 0.01 && val <= 1.0 {
				recordIdx := -1
				offsetInRecord := 0
				for ri, roff := range recordOffsets {
					if i >= roff {
						recordIdx = ri
						offsetInRecord = i - roff
					} else {
						break
					}
				}

				fmt.Printf("Offset 0x%04x: %.6f (record %d, offset +0x%03x)\n",
					i, val, recordIdx, offsetInRecord)
			}
		}
	}
}
