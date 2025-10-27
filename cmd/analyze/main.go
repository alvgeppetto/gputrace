package main

import (
	"encoding/binary"
	"fmt"
	"log"
	"os"

	"github.com/tmc/mlx-go/experiments/gputrace"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "Usage: %s <path-to-.gputrace>\n", os.Args[0])
		os.Exit(1)
	}

	tracePath := os.Args[1]

	trace, err := gputrace.Open(tracePath)
	if err != nil {
		log.Fatalf("Failed to open trace: %v", err)
	}

	fmt.Printf("=== Trace Analysis: %s ===\n\n", tracePath)

	// Metadata
	fmt.Println("Metadata:")
	if trace.Metadata != nil {
		fmt.Printf("  UUID: %s\n", trace.Metadata.UUID)
		fmt.Printf("  Capture Version: %d\n", trace.Metadata.CaptureVersion)
		fmt.Printf("  Graphics API: %d\n", trace.Metadata.GraphicsAPI)
		fmt.Printf("  Device ID: %d\n", trace.Metadata.DeviceID)
	}
	fmt.Println()

	// Labels found
	fmt.Printf("Encoder Labels Found: %d\n", len(trace.EncoderLabels))
	for i, label := range trace.EncoderLabels {
		fmt.Printf("  [%d] %s\n", i, label)
	}
	fmt.Println()

	fmt.Printf("Kernel Names Found: %d\n", len(trace.KernelNames))
	for i, name := range trace.KernelNames {
		fmt.Printf("  [%d] %s\n", i, name)
	}
	fmt.Println()

	fmt.Printf("Buffer Labels Found: %d\n", len(trace.BufferLabels))
	for i, label := range trace.BufferLabels {
		fmt.Printf("  [%d] %s\n", i, label)
	}
	fmt.Println()

	if trace.CommandQueueLabel != "" {
		fmt.Printf("Command Queue Label: %s\n\n", trace.CommandQueueLabel)
	}

	// Analyze capture data structure
	fmt.Printf("Capture Data Size: %d bytes\n", len(trace.CaptureData))
	if len(trace.CaptureData) >= 16 {
		header, err := gputrace.ReadMTSPHeader(trace.CaptureData)
		if err == nil {
			fmt.Printf("  Magic: %s\n", string(header.Magic[:]))
			fmt.Printf("  Version: %d\n", header.Version)
			fmt.Printf("  Size: %d\n", header.Size)
			fmt.Printf("  Offset: %d\n", header.Offset)
		}
	}
	fmt.Println()

	// Try to find potential timestamp patterns
	fmt.Println("=== Searching for Timestamp Patterns ===")
	for _, label := range trace.EncoderLabels {
		fmt.Printf("\nAnalyzing label: %q\n", label)
		offset := findLabelOffset(trace.CaptureData, label)
		if offset == -1 {
			fmt.Printf("  Label not found in capture data\n")
			continue
		}

		fmt.Printf("  Label offset: 0x%x (%d)\n", offset, offset)

		// Show hex dump around label
		start := offset - 128
		if start < 0 {
			start = 0
		}
		end := offset + 128
		if end > len(trace.CaptureData) {
			end = len(trace.CaptureData)
		}

		fmt.Printf("\n  Hex dump (label at 0x%x):\n", offset)
		dumpHex(trace.CaptureData[start:end], start, offset)

		// Try various offsets to find potential timestamps
		fmt.Printf("\n  Scanning for potential timestamps:\n")
		scanForTimestamps(trace.CaptureData, offset, label)
	}

	// MTSP Record analysis
	fmt.Println("\n=== MTSP Record Analysis ===")
	report, err := trace.AnalyzeMTSPRecords()
	if err != nil {
		fmt.Printf("Error analyzing MTSP records: %v\n", err)
	} else {
		fmt.Println(report)
	}

	// Store analysis
	fmt.Println("\n=== Store0 Analysis ===")
	storeReport, err := trace.AnalyzeStoreStructure()
	if err != nil {
		fmt.Printf("Error analyzing store0: %v\n", err)
	} else {
		// Show first part of store analysis
		lines := 0
		for i, c := range storeReport {
			if c == '\n' {
				lines++
				if lines > 30 {
					fmt.Printf(storeReport[:i])
					fmt.Println("\n... (truncated, store0 appears to be empty/zeros)")
					break
				}
			}
		}
		if lines <= 30 {
			fmt.Print(storeReport)
		}
	}

	// Try timing extraction
	fmt.Println("\n=== Attempting Timing Extraction ===")
	timings, err := trace.ExtractTimingData()
	if err != nil {
		fmt.Printf("Error: %v\n", err)
	} else if len(timings) == 0 {
		fmt.Printf("No timing data extracted (store0 likely contains no timing data)\n")
	} else {
		for _, timing := range timings {
			fmt.Printf("  %s:\n", timing.Label)
			fmt.Printf("    Start: %d (0x%x)\n", timing.StartTimestamp, timing.StartTimestamp)
			fmt.Printf("    End:   %d (0x%x)\n", timing.EndTimestamp, timing.EndTimestamp)
			fmt.Printf("    Duration: %.2f ms\n", timing.DurationMs)
		}
	}
}

func findLabelOffset(data []byte, label string) int {
	labelBytes := []byte(label)
	for i := 0; i <= len(data)-len(labelBytes); i++ {
		match := true
		for j := 0; j < len(labelBytes); j++ {
			if data[i+j] != labelBytes[j] {
				match = false
				break
			}
		}
		if match {
			return i
		}
	}
	return -1
}

func dumpHex(data []byte, baseOffset, highlightOffset int) {
	for i := 0; i < len(data); i += 16 {
		end := i + 16
		if end > len(data) {
			end = len(data)
		}

		offset := baseOffset + i
		fmt.Printf("    %08x: ", offset)

		// Hex bytes
		for j := i; j < end; j++ {
			if baseOffset+j == highlightOffset {
				fmt.Printf("[%02x]", data[j])
			} else {
				fmt.Printf("%02x ", data[j])
			}
		}

		// Padding
		for j := end; j < i+16; j++ {
			fmt.Printf("   ")
		}

		// ASCII
		fmt.Printf(" |")
		for j := i; j < end; j++ {
			if data[j] >= 32 && data[j] <= 126 {
				fmt.Printf("%c", data[j])
			} else {
				fmt.Printf(".")
			}
		}
		fmt.Printf("|\n")
	}
}

func scanForTimestamps(data []byte, labelOffset int, label string) {
	// Mach absolute time is typically > 1e15 (around 1e18 range)
	// Try various offsets looking for 8-byte values in this range

	offsets := []int{-256, -128, -96, -80, -72, -68, -64, -48, -32, -16, 0, 8, 16, 24, 32, 48, 64, 96, 128}

	for _, deltaOffset := range offsets {
		checkOffset := labelOffset + deltaOffset
		if checkOffset < 0 || checkOffset+8 > len(data) {
			continue
		}

		val := binary.LittleEndian.Uint64(data[checkOffset : checkOffset+8])

		// Check if it looks like a Mach timestamp (1e15 to 1e19)
		if val > 1000000000000000 && val < 10000000000000000000 {
			fmt.Printf("    Offset %+4d (0x%x): %d (0x%016x) - potential timestamp\n",
				deltaOffset, checkOffset, val, val)
		}
	}
}
