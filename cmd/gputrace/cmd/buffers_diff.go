package cmd

import (
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"github.com/tmc/mlx-go/experiments/gputrace"
)

var buffersDiffCmd = &cobra.Command{
	Use:   "diff <trace1.gputrace> <trace2.gputrace>",
	Short: "Compare buffer usage between two GPU traces",
	Long: `Compare buffer usage between two GPU traces.

This command shows:
  - Buffers added/removed between traces
  - Size changes for buffers present in both
  - Memory usage delta
  - Detailed breakdown of changes

Useful for:
  - Tracking memory optimization changes
  - Detecting memory regressions
  - Understanding buffer allocation differences

Examples:
  gputrace buffers diff baseline.gputrace optimized.gputrace
  gputrace buffers diff trace1.gputrace trace2.gputrace`,
	Args: cobra.ExactArgs(2),
	RunE: runBuffersDiff,
}

func init() {
	buffersCmd.AddCommand(buffersDiffCmd)
}

func runBuffersDiff(cmd *cobra.Command, args []string) error {
	trace1Path := args[0]
	trace2Path := args[1]

	// Verify both trace files exist
	if err := checkTraceFile(trace1Path); err != nil {
		return fmt.Errorf("trace1: %w", err)
	}
	if err := checkTraceFile(trace2Path); err != nil {
		return fmt.Errorf("trace2: %w", err)
	}

	// Open traces
	trace1, err := gputrace.Open(trace1Path)
	if err != nil {
		return fmt.Errorf("failed to open trace1: %w", err)
	}
	defer trace1.Close()

	trace2, err := gputrace.Open(trace2Path)
	if err != nil {
		return fmt.Errorf("failed to open trace2: %w", err)
	}
	defer trace2.Close()

	// Extract buffer information from both traces
	buffers1, err := extractBufferInfo(trace1Path, trace1, false)
	if err != nil {
		return fmt.Errorf("failed to extract buffer info from trace1: %w", err)
	}

	buffers2, err := extractBufferInfo(trace2Path, trace2, false)
	if err != nil {
		return fmt.Errorf("failed to extract buffer info from trace2: %w", err)
	}

	// Build maps for comparison
	bufMap1 := make(map[string]BufferInfo)
	for _, buf := range buffers1 {
		bufMap1[buf.ID] = buf
	}

	bufMap2 := make(map[string]BufferInfo)
	for _, buf := range buffers2 {
		bufMap2[buf.ID] = buf
	}

	// Calculate differences
	diff := compareBuffers(bufMap1, bufMap2)

	// Display comparison
	displayBufferDiff(diff, trace1Path, trace2Path)

	return nil
}

// BufferDiff represents the differences between two buffer sets.
type BufferDiff struct {
	Added      []BufferInfo
	Removed    []BufferInfo
	SizeChange []BufferSizeChange
	TotalSize1 uint64
	TotalSize2 uint64
}

// BufferSizeChange represents a buffer that exists in both traces but with different size.
type BufferSizeChange struct {
	ID       string
	Filename string
	OldSize  uint64
	NewSize  uint64
}

func (b BufferSizeChange) Delta() int64 {
	return int64(b.NewSize) - int64(b.OldSize)
}

func (b BufferSizeChange) PercentChange() float64 {
	if b.OldSize == 0 {
		return 100.0
	}
	return (float64(b.NewSize) - float64(b.OldSize)) / float64(b.OldSize) * 100.0
}

// compareBuffers compares two buffer maps and returns the differences.
func compareBuffers(bufMap1, bufMap2 map[string]BufferInfo) BufferDiff {
	diff := BufferDiff{
		Added:      make([]BufferInfo, 0),
		Removed:    make([]BufferInfo, 0),
		SizeChange: make([]BufferSizeChange, 0),
	}

	// Find added and size changes
	for id, buf2 := range bufMap2 {
		if buf1, exists := bufMap1[id]; exists {
			// Buffer exists in both - check for size change
			if buf1.Size != buf2.Size {
				diff.SizeChange = append(diff.SizeChange, BufferSizeChange{
					ID:       id,
					Filename: buf2.Filename,
					OldSize:  buf1.Size,
					NewSize:  buf2.Size,
				})
			}
		} else {
			// Buffer only in trace2 - added
			diff.Added = append(diff.Added, buf2)
		}
		diff.TotalSize2 += buf2.Size
	}

	// Find removed buffers
	for id, buf1 := range bufMap1 {
		if _, exists := bufMap2[id]; !exists {
			diff.Removed = append(diff.Removed, buf1)
		}
		diff.TotalSize1 += buf1.Size
	}

	// Sort for consistent output
	sort.Slice(diff.Added, func(i, j int) bool {
		return diff.Added[i].Size > diff.Added[j].Size
	})
	sort.Slice(diff.Removed, func(i, j int) bool {
		return diff.Removed[i].Size > diff.Removed[j].Size
	})
	sort.Slice(diff.SizeChange, func(i, j int) bool {
		return abs(diff.SizeChange[i].Delta()) > abs(diff.SizeChange[j].Delta())
	})

	return diff
}

func abs(n int64) int64 {
	if n < 0 {
		return -n
	}
	return n
}

// displayBufferDiff displays the buffer comparison results.
func displayBufferDiff(diff BufferDiff, trace1Path, trace2Path string) {
	fmt.Printf("=== GPU Trace Buffer Comparison ===\n\n")
	fmt.Printf("Trace 1: %s\n", trace1Path)
	fmt.Printf("Trace 2: %s\n\n", trace2Path)

	// Summary statistics
	fmt.Printf("=== Summary ===\n")
	fmt.Printf("Total Size (Trace 1): %s (%.2f MB)\n", formatBytes(diff.TotalSize1), float64(diff.TotalSize1)/(1024*1024))
	fmt.Printf("Total Size (Trace 2): %s (%.2f MB)\n", formatBytes(diff.TotalSize2), float64(diff.TotalSize2)/(1024*1024))

	delta := int64(diff.TotalSize2) - int64(diff.TotalSize1)
	deltaSign := "+"
	if delta < 0 {
		deltaSign = ""
	}
	fmt.Printf("Memory Delta: %s%s (%.2f MB)\n", deltaSign, formatBytes(uint64(abs(delta))), float64(delta)/(1024*1024))

	if diff.TotalSize1 > 0 {
		percentChange := (float64(diff.TotalSize2) - float64(diff.TotalSize1)) / float64(diff.TotalSize1) * 100.0
		fmt.Printf("Percent Change: %+.2f%%\n", percentChange)
	}
	fmt.Printf("\n")

	fmt.Printf("Buffers Added: %d\n", len(diff.Added))
	fmt.Printf("Buffers Removed: %d\n", len(diff.Removed))
	fmt.Printf("Buffers Changed: %d\n\n", len(diff.SizeChange))

	// Added buffers
	if len(diff.Added) > 0 {
		fmt.Printf("=== Added Buffers (%d) ===\n", len(diff.Added))
		fmt.Printf("%-8s %-25s %12s %10s\n", "ID", "Filename", "Size", "Size (MB)")
		fmt.Println(strings.Repeat("-", 80))

		var totalAdded uint64
		for _, buf := range diff.Added {
			sizeMB := float64(buf.Size) / (1024 * 1024)
			fmt.Printf("%-8s %-25s %12s %10.2f\n",
				buf.ID,
				buf.Filename,
				formatBytes(buf.Size),
				sizeMB,
			)
			totalAdded += buf.Size
		}
		fmt.Printf("\nTotal Added: %s (%.2f MB)\n\n", formatBytes(totalAdded), float64(totalAdded)/(1024*1024))
	}

	// Removed buffers
	if len(diff.Removed) > 0 {
		fmt.Printf("=== Removed Buffers (%d) ===\n", len(diff.Removed))
		fmt.Printf("%-8s %-25s %12s %10s\n", "ID", "Filename", "Size", "Size (MB)")
		fmt.Println(strings.Repeat("-", 80))

		var totalRemoved uint64
		for _, buf := range diff.Removed {
			sizeMB := float64(buf.Size) / (1024 * 1024)
			fmt.Printf("%-8s %-25s %12s %10.2f\n",
				buf.ID,
				buf.Filename,
				formatBytes(buf.Size),
				sizeMB,
			)
			totalRemoved += buf.Size
		}
		fmt.Printf("\nTotal Removed: %s (%.2f MB)\n\n", formatBytes(totalRemoved), float64(totalRemoved)/(1024*1024))
	}

	// Size changes
	if len(diff.SizeChange) > 0 {
		fmt.Printf("=== Size Changes (%d) ===\n", len(diff.SizeChange))
		fmt.Printf("%-8s %-25s %12s %12s %12s %10s\n", "ID", "Filename", "Old Size", "New Size", "Delta", "Change %")
		fmt.Println(strings.Repeat("-", 100))

		for _, change := range diff.SizeChange {
			delta := change.Delta()
			deltaSign := "+"
			if delta < 0 {
				deltaSign = ""
			}

			fmt.Printf("%-8s %-25s %12s %12s %s%12s %+9.1f%%\n",
				change.ID,
				change.Filename,
				formatBytes(change.OldSize),
				formatBytes(change.NewSize),
				deltaSign,
				formatBytes(uint64(abs(delta))),
				change.PercentChange(),
			)
		}
		fmt.Println()
	}

	// Final verdict
	if len(diff.Added) == 0 && len(diff.Removed) == 0 && len(diff.SizeChange) == 0 {
		fmt.Println("✓ No differences found - buffers are identical")
	} else {
		if delta < 0 {
			fmt.Printf("✓ Memory usage decreased by %s (%.2f MB)\n", formatBytes(uint64(-delta)), float64(-delta)/(1024*1024))
		} else if delta > 0 {
			fmt.Printf("⚠ Memory usage increased by %s (%.2f MB)\n", formatBytes(uint64(delta)), float64(delta)/(1024*1024))
		} else {
			fmt.Println("• Memory usage unchanged (buffers reorganized)")
		}
	}
}
