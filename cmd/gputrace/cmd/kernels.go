package cmd

import (
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/tmc/mlx-go/experiments/gputrace"
)

var (
	kernelsFilter  string
	kernelsVerbose bool
)

var kernelsCmd = &cobra.Command{
	Use:   "kernels <trace.gputrace>",
	Short: "List kernel functions and their pipeline state mappings",
	Long: `List all kernel functions found in a GPU trace with their pipeline state addresses.

This command extracts the mapping between pipeline state objects and their
associated kernel functions, making it easy to understand which Metal functions
are being executed.

Examples:
  # List all kernels
  gputrace kernels trace.gputrace

  # Filter by kernel name (case-insensitive substring match)
  gputrace kernels trace.gputrace --filter copy
  gputrace kernels trace.gputrace --filter steel_gemm

  # Verbose output with additional details
  gputrace kernels trace.gputrace -v`,
	Args: cobra.ExactArgs(1),
	RunE: runKernels,
}

func init() {
	rootCmd.AddCommand(kernelsCmd)

	kernelsCmd.Flags().StringVarP(&kernelsFilter, "filter", "f", "", "Filter kernels by name (case-insensitive substring match)")
	kernelsCmd.Flags().BoolVarP(&kernelsVerbose, "verbose", "v", false, "Show verbose output with additional details")
}

func runKernels(cmd *cobra.Command, args []string) error {
	tracePath := args[0]

	if err := checkTraceFile(tracePath); err != nil {
		return err
	}

	trace, err := gputrace.Open(tracePath)
	if err != nil {
		return fmt.Errorf("failed to open trace: %w", err)
	}

	// Build pipeline→function mapping
	pipelineMap := trace.BuildPipelineFunctionMap()

	// Collect and sort by function name
	type kernelInfo struct {
		name         string
		pipelineAddr uint64
	}
	var kernels []kernelInfo

	filterLower := strings.ToLower(kernelsFilter)
	for addr, name := range pipelineMap {
		// Apply filter if specified
		if kernelsFilter != "" && !strings.Contains(strings.ToLower(name), filterLower) {
			continue
		}
		kernels = append(kernels, kernelInfo{name: name, pipelineAddr: addr})
	}

	// Sort by name
	sort.Slice(kernels, func(i, j int) bool {
		return kernels[i].name < kernels[j].name
	})

	// Output
	if kernelsFilter != "" {
		fmt.Printf("=== Kernels matching %q ===\n", kernelsFilter)
	} else {
		fmt.Printf("=== Kernel Functions ===\n")
	}
	fmt.Printf("Total: %d kernels\n\n", len(kernels))

	if len(kernels) == 0 {
		if kernelsFilter != "" {
			fmt.Printf("No kernels found matching filter %q\n", kernelsFilter)
		} else {
			fmt.Printf("No kernel→pipeline mappings found in trace\n")
		}
		return nil
	}

	// Print table
	if kernelsVerbose {
		fmt.Printf("%-50s  %-18s\n", "Name", "Pipeline State")
		fmt.Printf("%-50s  %-18s\n", strings.Repeat("-", 50), strings.Repeat("-", 18))
	}

	for _, k := range kernels {
		if kernelsVerbose {
			fmt.Printf("%-50s  0x%x\n", k.name, k.pipelineAddr)
		} else {
			fmt.Printf("%s\n", k.name)
		}
	}

	return nil
}
