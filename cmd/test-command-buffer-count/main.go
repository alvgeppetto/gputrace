package main

import (
	"fmt"
	"log"

	"github.com/tmc/gputrace"
)

func main() {
	tracePath := "/tmp/llm-tool_1762199057.gputrace"

	// Open the trace
	trace, err := gputrace.Open(tracePath)
	if err != nil {
		log.Fatalf("Failed to open trace: %v", err)
	}
	defer trace.Close()

	// Parse MTSP records to count command buffers properly
	records, err := trace.ParseMTSPRecords()
	if err != nil {
		log.Fatalf("Failed to parse MTSP records: %v", err)
	}

	// Count different record types
	recordCounts := make(map[string]int)
	for _, rec := range records {
		recordCounts[rec.Type]++
	}

	fmt.Printf("=== GPU Trace Analysis ===\n")
	fmt.Printf("Trace: %s\n", tracePath)
	fmt.Printf("\n")
	fmt.Printf("MTSP Record Counts:\n")
	for typ, count := range recordCounts {
		fmt.Printf("  %-10s: %d\n", typ, count)
	}
	fmt.Printf("  %-10s: %d\n", "TOTAL", len(records))
	fmt.Printf("\n")

	// Try different interpretations
	fmt.Printf("Possible Command Buffer Interpretations:\n")
	fmt.Printf("  Culul records: %d (individual command buffers)\n", recordCounts["Culul"])
	fmt.Printf("  Ci records:    %d (indirect command buffers / ICBs)\n", recordCounts["Ci"])
	fmt.Printf("  Cul records:   %d\n", recordCounts["Cul"])
	fmt.Printf("  Ct records:    %d (command type/dispatch records)\n", recordCounts["Ct"])
	fmt.Printf("\n")

	// Check if there are unique command buffer sequences
	// Maybe the 231 represents command buffer *submissions* not individual ICBs
	fmt.Printf("Theory: 231 might represent command buffer submissions\n")
	fmt.Printf("  If each submission contains ~4-5 ICBs (Ci records):\n")
	fmt.Printf("  1075 Ci / 231 = %.2f ICBs per submission\n", float64(recordCounts["Ci"])/231.0)
	fmt.Printf("  1066 Culul / 231 = %.2f Culul per submission\n", float64(recordCounts["Culul"])/231.0)
	fmt.Printf("\n")

	// Check dispatch estimates
	estimate, err := trace.EstimateDispatches()
	if err == nil {
		fmt.Printf("Dispatch Estimate:\n")
		fmt.Printf("  Count: %d\n", estimate.Count)
		fmt.Printf("  Method: %s\n", estimate.Method)
		fmt.Printf("  Confidence: %.0f%%\n", estimate.Confidence*100)
		fmt.Printf("  Notes: %s\n", estimate.Notes)
		fmt.Printf("\n")
	}

	// Parse index file for actual command buffer count
	fmt.Printf("=== Index File Analysis ===\n")
	indexData, err := trace.ParseIndexFile()
	if err != nil {
		fmt.Printf("Error parsing index: %v\n", err)
	} else {
		fmt.Printf("Magic: %s\n", indexData.Magic)
		fmt.Printf("Command Buffers: %d\n", indexData.CommandBuffers)
		fmt.Printf("Compute Encoders: %d\n", indexData.ComputeEncoders)
		fmt.Printf("Dispatch Calls: %d\n\n", indexData.DispatchCalls)

		if indexData.CommandBuffers > 0 {
			fmt.Printf("✅ FOUND: %d command buffers from index file\n", indexData.CommandBuffers)
		}
	}
}
