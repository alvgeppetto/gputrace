package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/tmc/mlx-go/experiments/gputrace"
)

var apiCallsCmd = &cobra.Command{
	Use:   "api-calls <trace.gputrace>",
	Short: "Display API call sequences from a GPU trace",
	Long: `Display the sequence of Metal API calls captured in a GPU trace.

Shows the full API call sequence including:
- Command buffer creation
- Encoder creation and configuration
- Compute pipeline state setup
- Buffer bindings
- Dispatch calls
- Encoder completion

Each call is numbered and indented to show the command buffer hierarchy.

Examples:
  # Show all API calls
  gputrace api-calls trace.gputrace

  # Show first 100 API calls
  gputrace api-calls trace.gputrace | head -100

  # Search for specific API calls
  gputrace api-calls trace.gputrace | grep setBuffer`,
	Args: cobra.ExactArgs(1),
	RunE: runAPICalls,
}

func init() {
	rootCmd.AddCommand(apiCallsCmd)
}

func runAPICalls(cmd *cobra.Command, args []string) error {
	tracePath := args[0]
	if err := checkTraceFile(tracePath); err != nil {
		return err
	}

	trace, err := gputrace.Open(tracePath)
	if err != nil {
		return fmt.Errorf("failed to open trace: %w", err)
	}

	// Use FormatAPICallList which prints to stdout
	if err := trace.FormatAPICallList(cmd.OutOrStdout()); err != nil {
		return fmt.Errorf("failed to format API calls: %w", err)
	}

	return nil
}
