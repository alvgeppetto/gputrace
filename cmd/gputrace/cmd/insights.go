package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/tmc/mlx-go/experiments/gputrace"
)

var insightsCmd = &cobra.Command{
	Use:   "insights <trace.gputrace>",
	Short: "Generate actionable performance insights",
	Long: `Analyze GPU trace and generate actionable performance insights.

This command analyzes profiling data to identify:
  - Bottlenecks (memory-bound vs compute-bound shaders)
  - Optimization opportunities (excessive dispatches, suboptimal occupancy)
  - Performance anti-patterns (branch divergence, poor threadgroup config)
  - Actionable recommendations for improvement

Example output:
  [1] 🔴 [CRITICAL] steel_gemm is a major bottleneck
      This shader consumes 61% of total GPU time (125.3 ms)

      Likely MEMORY-BOUND: Low thread count suggests memory bandwidth limitation.

      Recommendations:
        • Consider reducing memory bandwidth via data tiling
        • Use shared memory / threadgroup memory for data reuse
        • Profile this shader in detail to identify hotspots

Examples:
  # Generate insights report
  gputrace insights trace.gputrace

  # Export insights as JSON
  gputrace insights trace.gputrace --format json`,
	Args: cobra.ExactArgs(1),
	RunE: runInsights,
}

var (
	insightsFormat  string
	insightsVerbose bool
)

func init() {
	rootCmd.AddCommand(insightsCmd)
	insightsCmd.Flags().StringVarP(&insightsFormat, "format", "f", "text", "Output format: text, json")
	insightsCmd.Flags().BoolVarP(&insightsVerbose, "verbose", "v", false, "Show verbose output")
}

func runInsights(cmd *cobra.Command, args []string) error {
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
	defer trace.Close()

	// Generate insights
	fmt.Fprintln(os.Stderr, "Analyzing GPU performance...")
	report, err := trace.GenerateInsights()
	if err != nil {
		return fmt.Errorf("failed to generate insights: %w", err)
	}

	// Format output
	switch insightsFormat {
	case "json":
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		return encoder.Encode(report)

	case "text":
		fallthrough
	default:
		fmt.Print(gputrace.FormatInsightsReport(report))
	}

	return nil
}
