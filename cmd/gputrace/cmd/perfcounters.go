package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/tmc/mlx-go/experiments/gputrace"
)

var perfCountersCmd = &cobra.Command{
	Use:   "perfcounters <trace.gputrace>",
	Short: "Display hardware performance counter metrics",
	Long: `Parse and display GPU hardware performance counters from profiled traces.

This command extracts detailed GPU execution metrics including:
  - Shader execution counts and timing
  - Register allocation (actual hardware data)
  - Register spill statistics
  - ALU utilization percentages
  - Kernel occupancy metrics
  - Memory bandwidth usage

Requires a profiled trace captured with GPU performance counters enabled.
The trace must have a .gpuprofiler_raw directory with Counters_f_*.raw files.

Examples:
  gputrace perfcounters trace.gputrace
  gputrace perfcounters --verbose trace.gputrace`,
	Args: cobra.ExactArgs(1),
	RunE: runPerfCounters,
}

func init() {
	rootCmd.AddCommand(perfCountersCmd)
}

func runPerfCounters(cmd *cobra.Command, args []string) error {
	tracePath := args[0]

	// Verify trace file exists
	if err := checkTraceFile(tracePath); err != nil {
		return err
	}

	// Open trace
	trace, err := gputrace.Open(tracePath)
	if err != nil {
		return fmt.Errorf("failed to open trace: %w", err)
	}

	// Check if trace has performance counter data
	if !trace.HasPerfCounters() {
		fmt.Println("No performance counter data found.")
		fmt.Println()
		fmt.Println("This trace was not captured with GPU profiling enabled.")
		fmt.Println("To capture profiled traces:")
		fmt.Println("  1. Use Xcode Instruments GPU profiler")
		fmt.Println("  2. Enable 'Shader Profiler' instrument")
		fmt.Println("  3. Export trace with performance counters")
		return nil
	}

	// Parse performance counters
	fmt.Println("Parsing hardware performance counters...")
	stats, err := trace.ParsePerfCounters()
	if err != nil {
		return fmt.Errorf("failed to parse performance counters: %w", err)
	}

	// Display summary
	fmt.Printf("\n=== GPU Hardware Performance Counters ===\n\n")
	fmt.Printf("Files Processed:  %d\n", stats.FilesProcessed)
	fmt.Printf("Total Records:    %d\n", stats.TotalRecords)
	fmt.Printf("Dispatch Count:   %d\n", stats.DispatchCount)
	fmt.Printf("Confidence Level: %.1f%%\n\n", stats.ConfidenceLevel*100)

	// Display shader metrics if available
	if len(stats.ShaderMetrics) > 0 {
		fmt.Printf("=== Shader Hardware Metrics ===\n\n")
		fmt.Printf("%-50s %10s %10s %10s %10s\n", "Shader Name", "Exec Count", "SIMD Grps", "Registers", "Spilled")
		fmt.Println("------------------------------------------------------------------------------------------------------------------")

		for _, metric := range stats.ShaderMetrics {
			fmt.Printf("%-50s %10d %10d %10d %10d\n",
				truncate(metric.ShaderName, 50),
				metric.ExecutionCount,
				metric.SIMDGroups,
				metric.AllocatedRegs,
				metric.SpilledBytes)
		}
	} else {
		fmt.Println("Note: Full shader metrics extraction is still in development.")
		fmt.Println("Record parsing is complete, but field-level metric extraction")
		fmt.Println("requires additional reverse engineering of the counter file format.")
		fmt.Println()
		fmt.Println("Current implementation can:")
		fmt.Println("  ✓ Locate and open .gpuprofiler_raw files")
		fmt.Println("  ✓ Parse record boundaries (0x4E markers)")
		fmt.Println("  ✓ Count total records and dispatches")
		fmt.Println()
		fmt.Println("Future work:")
		fmt.Println("  • Extract register allocation from records")
		fmt.Println("  • Parse ALU utilization metrics")
		fmt.Println("  • Extract kernel occupancy data")
		fmt.Println("  • Map metrics to shader names")
	}

	return nil
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
