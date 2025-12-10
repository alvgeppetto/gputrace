package cmd

import (
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/tmc/gputrace"
)

var (
	kernelsFilter  string
	kernelsVerbose bool
	kernelsStats   bool
)

var kernelsCmd = &cobra.Command{
	Use:   "kernels <trace.gputrace>",
	Short: "List kernel functions and their pipeline state mappings",
	Long: `List all kernel functions found in a GPU trace with their pipeline state addresses.

This command extracts the mapping between pipeline state objects and their
associated kernel functions, making it easy to understand which Metal functions
are being executed.

It also analyzes dispatch counts and associates kernels with debug groups and encoder labels.

Examples:
  # List all kernels with dispatch counts
  gputrace kernels trace.gputrace

  # Filter by kernel name (case-insensitive substring match)
  gputrace kernels trace.gputrace --filter copy
  gputrace kernels trace.gputrace --filter steel_gemm

  # Show detailed stats including debug groups
  gputrace kernels trace.gputrace --stats

  # Verbose output with additional details
  gputrace kernels trace.gputrace -v`,
	Args: cobra.ExactArgs(1),
	RunE: runKernels,
}

func init() {
	rootCmd.AddCommand(kernelsCmd)

	kernelsCmd.Flags().StringVarP(&kernelsFilter, "filter", "f", "", "Filter kernels by name (case-insensitive substring match)")
	kernelsCmd.Flags().BoolVarP(&kernelsVerbose, "verbose", "v", false, "Show verbose output with additional details")
	kernelsCmd.Flags().BoolVarP(&kernelsStats, "stats", "s", false, "Show detailed statistics including debug groups")
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

	// Use AnalyzeKernels for comprehensive stats
	report, err := gputrace.AnalyzeKernels(trace)
	if err != nil {
		return fmt.Errorf("analyze kernels: %w", err)
	}

	// Filter results
	var filtered []*gputrace.KernelStat
	filterLower := strings.ToLower(kernelsFilter)
	for _, k := range report.Kernels {
		if kernelsFilter != "" && !strings.Contains(strings.ToLower(k.Name), filterLower) {
			continue
		}
		filtered = append(filtered, k)
	}

	// Output
	if kernelsFilter != "" {
		fmt.Printf("=== Kernels matching %q ===\n", kernelsFilter)
	} else {
		fmt.Printf("=== Kernel Functions (%d unique) ===\n", len(filtered))
	}
	fmt.Println()

	if len(filtered) == 0 {
		fmt.Println("No kernels found.")
		return nil
	}

	// Determine column widths
	maxNameLen := 30
	maxDebugGroupLen := 40
	for _, k := range filtered {
		if len(k.Name) > maxNameLen {
			maxNameLen = len(k.Name)
		}
		for _, dg := range k.DebugGroups {
			if len(dg) > maxDebugGroupLen {
				maxDebugGroupLen = len(dg)
			}
		}
	}
	if maxNameLen > 60 { maxNameLen = 60 }

	// Print table header
	fmt.Printf("%-*s  %-18s  %-10s  %s\n",
		maxNameLen, "Name", "Pipeline", "Dispatches", "Debug Groups")
	fmt.Printf("%s  %s  %s  %s\n",
		strings.Repeat("-", maxNameLen),
		strings.Repeat("-", 18),
		strings.Repeat("-", 10),
		strings.Repeat("-", 20))

	for _, k := range filtered {
		name := k.Name
		if len(name) > maxNameLen {
			name = name[:maxNameLen-3] + "..."
		}

		pipeline := fmt.Sprintf("0x%x", k.PipelineAddress)
		if k.PipelineAddress == 0 {
			pipeline = "-"
		}

		debugGroups := ""
		if len(k.DebugGroups) > 0 {
			// Show first debug group, count others
			debugGroups = k.DebugGroups[0]
			if len(k.DebugGroups) > 1 {
				debugGroups += fmt.Sprintf(" (+%d more)", len(k.DebugGroups)-1)
			}
		}

		fmt.Printf("%-*s  %-18s  %-10d  %s\n",
			maxNameLen, name, pipeline, k.DispatchCount, debugGroups)
	}

	fmt.Println()
	fmt.Printf("Total Dispatches: %d\n", report.DispatchCount)
	if report.UnknownCount > 0 {
		fmt.Printf("Unknown Pipelines: %d dispatches\n", report.UnknownCount)
	}

	// Detailed stats view
	if kernelsStats {
		fmt.Println()
		fmt.Println("=== Detailed Statistics ===")
		for _, k := range filtered {
			fmt.Printf("\nKernel: %s\n", k.Name)
			fmt.Printf("  Dispatches:   %d\n", k.DispatchCount)
			if k.PipelineAddress != 0 {
				fmt.Printf("  Pipeline:     0x%x\n", k.PipelineAddress)
			}

			if len(k.DebugGroups) > 0 {
				fmt.Println("  Debug Groups:")
				// Sort and unique
				groups := uniqueStrings(k.DebugGroups)
				sort.Strings(groups)
				for _, dg := range groups {
					fmt.Printf("    • %s\n", dg)
				}
			}

			if len(k.EncoderLabels) > 0 {
				fmt.Println("  Encoder Labels:")
				labels := uniqueStrings(k.EncoderLabels)
				sort.Strings(labels)
				for _, lbl := range labels {
					fmt.Printf("    • %s\n", lbl)
				}
			}
		}
	}

	return nil
}

func uniqueStrings(input []string) []string {
	u := make([]string, 0, len(input))
	m := make(map[string]bool)
	for _, val := range input {
		if !m[val] {
			m[val] = true
			u = append(u, val)
		}
	}
	return u
}
