// Package cmd implements the gputrace CLI commands.
package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "gputrace",
	Short: "Tools for analyzing and converting GPU trace files",
	Long: `gputrace provides tools for analyzing and converting GPU trace files (.gputrace bundles).

The toolkit includes commands for:

Basic Information:
  stats            - Display comprehensive trace statistics
  dump             - Dump raw API call sequences
  encoders         - List compute command encoders

Shader Analysis:
  shaders          - Shader performance metrics (Xcode Instruments format)
  shader-metrics   - Alternative shader analysis
  perfcounters     - Hardware performance counters

Timing Analysis:
  timing           - Extract timing data
  timing-profiler  - Advanced timing profiling

Buffer Analysis:
  buffers          - List buffers and their properties
  buffer-access    - Analyze buffer access patterns
  buffers-diff     - Compare buffer contents

Advanced Analysis:
  command-buffers  - Detailed command buffer analysis
  correlate        - Correlate shader names with addresses
  insights         - Performance insights and recommendations

Visualization & Export:
  timeline         - Generate Chrome Tracing format timeline
  gputrace2pprof   - Export to pprof format

Examples:
  # Basic statistics
  gputrace stats trace.gputrace

  # Shader performance analysis
  gputrace shaders trace.gputrace

  # Interactive timeline visualization
  gputrace timeline trace.gputrace -o timeline.json
  # Then open chrome://tracing and load timeline.json

  # Export to pprof for analysis
  gputrace gputrace2pprof trace.gputrace -all

For more information about a specific command:
  gputrace [command] --help`,
}

// Execute runs the root command.
func Execute() error {
	return rootCmd.Execute()
}

func init() {
	rootCmd.CompletionOptions.DisableDefaultCmd = true
}

// checkTraceFile verifies that a trace file path exists and is a valid .gputrace directory.
func checkTraceFile(tracePath string) error {
	info, err := os.Stat(tracePath)
	if os.IsNotExist(err) {
		return fmt.Errorf("trace file not found: %s", tracePath)
	}
	if err != nil {
		return fmt.Errorf("error accessing trace file: %w", err)
	}

	if !info.IsDir() {
		return fmt.Errorf("trace path must be a .gputrace directory bundle, got file: %s", tracePath)
	}

	return nil
}
