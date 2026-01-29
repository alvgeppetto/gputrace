package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/tmc/gputrace"
)

var (
	exportCountersOutput string
)

var exportCountersCmd = &cobra.Command{
	Use:   "export-counters <trace.gputrace>",
	Short:  "Export performance counters in Xcode Counters.csv format",
	Hidden: true,
	Long: `Export performance counter data in Xcode Instruments Counters.csv format.

Generates a 246-column CSV file matching the exact format used by Xcode
Instruments when exporting GPU performance counter data. This includes:

Metadata Columns (1-5):
  - Index: Sequential row number
  - Encoder FunctionIndex: Encoder function index
  - CommandBuffer Label: Command buffer identifier
  - Encoder Label: Encoder identifier
  - (Empty column)

Performance Metrics (6-246):
  241 performance counter metrics including:
  - ALU Utilization, Kernel Occupancy
  - Memory bandwidth (Buffer/Texture Device Memory Bytes)
  - Cache miss rates (L1, Texture Cache)
  - Shader-specific metrics (VS/FS/Compute)
  - Pipeline utilization and limiters
  - Invocation counts and statistics

Data Source:
  Currently uses SYNTHETIC/ESTIMATED counter values (same approach as
  the 'timeline' command). This provides the correct CSV structure and
  realistic placeholder values.

  When actual counter data becomes available (either from Metal replay
  with MTLCounterSampleBuffer or .gpuprofiler_raw parsing), the values
  will be replaced with real hardware measurements.

Output Format:
  Standard CSV with quoted strings, matching Xcode's export format exactly.
  Can be imported into spreadsheet tools or compared with Xcode's output.

Examples:
  # Export counters to CSV file
  gputrace export-counters trace.gputrace -o counters.csv

  # Export to stdout
  gputrace export-counters trace.gputrace

  # Compare with Xcode's export
  diff <(gputrace export-counters trace.gputrace) xcode_counters.csv

Use Cases:
  - Validate CSV format matches Xcode structure
  - Import into analysis tools (Excel, pandas, etc.)
  - Automate performance reporting
  - Compare across different trace captures

Related Commands:
  - gputrace timeline: Visual timeline with counter tracks
  - gputrace perfcounters: Parse .gpuprofiler_raw files
  - gputrace replay-counters: Collect fresh counters via replay`,
	Args: cobra.ExactArgs(1),
	RunE: runExportCounters,
}

func init() {
	rootCmd.AddCommand(exportCountersCmd)

	exportCountersCmd.Flags().StringVarP(&exportCountersOutput, "output", "o", "",
		"Output CSV file (default: stdout)")
}

func runExportCounters(cmd *cobra.Command, args []string) error {
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

	// Create CSV exporter
	exporter := gputrace.NewCountersCSVExporter(trace)

	// Determine output writer
	var writer *os.File
	if exportCountersOutput != "" {
		f, err := os.Create(exportCountersOutput)
		if err != nil {
			return fmt.Errorf("failed to create output file: %w", err)
		}
		defer f.Close()
		writer = f
	} else {
		writer = os.Stdout
	}

	// Export CSV
	if err := exporter.ExportCountersCSV(writer); err != nil {
		return fmt.Errorf("failed to export counters CSV: %w", err)
	}

	// Print success message to stderr (not stdout which has CSV data)
	if exportCountersOutput != "" {
		fmt.Fprintf(os.Stderr, "✓ Exported counters to: %s\n", exportCountersOutput)
	}

	return nil
}
