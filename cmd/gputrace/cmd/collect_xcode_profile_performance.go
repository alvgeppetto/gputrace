package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// PerformanceInfo represents performance data extracted from Xcode.
type PerformanceInfo struct {
	Available bool   `json:"available"`
	Status    string `json:"status"` // "ready", "not_available", "already_shown"
}

var performanceCmd = &cobra.Command{
	Use:   "performance",
	Short: "Performance data commands",
	Long: `Commands for working with GPU performance data in Xcode.

Subcommands:
  show      Click the "Show Performance" button to reveal performance data
  status    Check if performance data is available
  summary   Extract summary statistics (planned)
  counters  Extract GPU counter values (planned)
  memory    Extract memory usage info (planned)

Example:
  gputrace xp performance show
  gputrace xp performance status --json
`,
}

func init() {
	collectXcodeProfileCmd.AddCommand(performanceCmd)

	// performance show
	showCmd := &cobra.Command{
		Use:   "show",
		Short: "Click the Show Performance button",
		Long:  `Clicks the "Show Performance" button in Xcode to reveal GPU performance data.`,
		RunE:  runPerformanceShow,
	}
	performanceCmd.AddCommand(showCmd)

	// performance status
	statusCmd := &cobra.Command{
		Use:   "status",
		Short: "Check if performance data is available",
		Long:  `Checks whether the "Show Performance" button is available and enabled.`,
		RunE:  runPerformanceStatus,
	}
	performanceCmd.AddCommand(statusCmd)

	// Performance view navigation commands
	viewCommands := []struct {
		name  string
		short string
	}{
		{"overview", "Select the Overview tab"},
		{"timeline", "Select the Timeline tab"},
		{"shaders", "Select the Shaders tab"},
		{"counters", "Select the Counters tab"},
		{"cost-graph", "Select the Cost Graph tab"},
		{"heat-map", "Select the Heat Map tab"},
		{"encoders", "Select the Encoders tab"},
		{"cost", "Select the Cost tab"},
	}

	for _, vc := range viewCommands {
		vcName := vc.name
		cmd := &cobra.Command{
			Use:   vcName,
			Short: vc.short,
			RunE: func(cmd *cobra.Command, args []string) error {
				return runPerformanceView(vcName)
			},
		}
		performanceCmd.AddCommand(cmd)
	}

	// performance summary (placeholder for future data extraction)
	summaryCmd := &cobra.Command{
		Use:    "summary",
		Short:  "Extract summary statistics",
		Long:   `Extracts summary statistics from the performance view. (Coming soon)`,
		Hidden: true,
		RunE:   runPerformanceSummary,
	}
	performanceCmd.AddCommand(summaryCmd)

	// performance memory (placeholder for future data extraction)
	memoryCmd := &cobra.Command{
		Use:    "memory",
		Short:  "Extract memory usage info",
		Long:   `Extracts memory allocation and usage information. (Coming soon)`,
		Hidden: true,
		RunE:   runPerformanceMemory,
	}
	performanceCmd.AddCommand(memoryCmd)
}

func runPerformanceShow(cmd *cobra.Command, args []string) error {
	if err := setupMacgo(); err != nil {
		return err
	}

	appAX, err := FindXcodeApp()
	if err != nil {
		if collectProfileJSON {
			return outputJSONError("XCODE_NOT_RUNNING", "Xcode not running", "Start Xcode first")
		}
		return fmt.Errorf("Xcode not running: %w", err)
	}
	defer cfRelease(appAX)

	windowAX, err := findTargetWindow(appAX, "")
	if err != nil {
		if collectProfileJSON {
			return outputJSONError("NO_WINDOWS", "no trace window found", "Open a trace file first")
		}
		return err
	}

	btn := findShowPerformanceButton(windowAX)
	if btn == 0 {
		if collectProfileJSON {
			return outputJSONError("NOT_AVAILABLE", "Show Performance button not found", "Replay may not be complete")
		}
		return fmt.Errorf("Show Performance button not found (replay may not be complete)")
	}

	if !IsElementEnabled(btn) {
		if collectProfileJSON {
			return outputJSONError("DISABLED", "Show Performance button is disabled", "Wait for replay to complete")
		}
		return fmt.Errorf("Show Performance button is disabled")
	}

	if !collectProfileJSON {
		fmt.Println("Clicking Show Performance...")
	}

	if err := axAction(btn, "AXPress"); err != nil {
		if collectProfileJSON {
			return outputJSONError("CLICK_FAILED", fmt.Sprintf("failed to click: %v", err), "Try again")
		}
		return fmt.Errorf("failed to click: %w", err)
	}

	if collectProfileJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(map[string]interface{}{
			"success": true,
			"action":  "show_performance",
		})
	}

	fmt.Println("Done")
	return nil
}

func runPerformanceStatus(cmd *cobra.Command, args []string) error {
	if err := setupMacgo(); err != nil {
		return err
	}

	appAX, err := FindXcodeApp()
	if err != nil {
		if collectProfileJSON {
			return outputJSONError("XCODE_NOT_RUNNING", "Xcode not running", "Start Xcode first")
		}
		return fmt.Errorf("Xcode not running: %w", err)
	}
	defer cfRelease(appAX)

	windowAX, err := findTargetWindow(appAX, "")
	if err != nil {
		if collectProfileJSON {
			return outputJSONError("NO_WINDOWS", "no trace window found", "Open a trace file first")
		}
		return err
	}

	btn := findShowPerformanceButton(windowAX)
	info := PerformanceInfo{}

	if btn == 0 {
		info.Available = false
		info.Status = "not_available"
	} else if !IsElementEnabled(btn) {
		info.Available = false
		info.Status = "disabled"
	} else {
		info.Available = true
		info.Status = "ready"
	}

	if collectProfileJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(info)
	}

	switch info.Status {
	case "ready":
		fmt.Println("Performance data available - use 'performance show' to view")
	case "disabled":
		fmt.Println("Performance button disabled - replay may be in progress")
	case "not_available":
		fmt.Println("Performance data not available - complete replay first")
	}
	return nil
}

func runPerformanceSummary(cmd *cobra.Command, args []string) error {
	if collectProfileJSON {
		return outputJSONError("NOT_IMPLEMENTED", "performance summary not yet implemented", "Coming soon")
	}
	fmt.Println("Performance summary extraction not yet implemented.")
	fmt.Println("This will extract summary statistics from the Xcode performance view.")
	return nil
}

func runPerformanceMemory(cmd *cobra.Command, args []string) error {
	if collectProfileJSON {
		return outputJSONError("NOT_IMPLEMENTED", "performance memory not yet implemented", "Coming soon")
	}
	fmt.Println("Performance memory extraction not yet implemented.")
	fmt.Println("This will extract memory usage information from Xcode.")
	return nil
}

// runPerformanceView clicks the appropriate tab button in the performance view.
func runPerformanceView(viewName string) error {
	if err := setupMacgo(); err != nil {
		return err
	}

	appAX, err := FindXcodeApp()
	if err != nil {
		if collectProfileJSON {
			return outputJSONError("XCODE_NOT_RUNNING", "Xcode not running", "Start Xcode first")
		}
		return fmt.Errorf("Xcode not running: %w", err)
	}
	defer cfRelease(appAX)

	windowAX, err := findTargetWindow(appAX, "")
	if err != nil {
		if collectProfileJSON {
			return outputJSONError("NO_WINDOWS", "no trace window found", "Open a trace file first")
		}
		return err
	}

	// Map view names to button names in Xcode UI
	buttonNames := map[string]string{
		"overview":   "Overview",
		"timeline":   "Timeline",
		"shaders":    "Shaders",
		"counters":   "Counters",
		"cost-graph": "Cost Graph",
		"heat-map":   "Heat Map",
		"encoders":   "Encoders",
		"cost":       "Cost",
	}

	buttonName, ok := buttonNames[viewName]
	if !ok {
		return fmt.Errorf("unknown view: %s", viewName)
	}

	// Find and click the button
	btn := findButtonBFS(windowAX, buttonName, 1000)
	if btn == 0 {
		if collectProfileJSON {
			return outputJSONError("NOT_FOUND", fmt.Sprintf("%s button not found", buttonName), "Open performance view first")
		}
		return fmt.Errorf("%s button not found (open performance view first)", buttonName)
	}

	if !IsElementEnabled(btn) {
		if collectProfileJSON {
			return outputJSONError("DISABLED", fmt.Sprintf("%s button is disabled", buttonName), "")
		}
		return fmt.Errorf("%s button is disabled", buttonName)
	}

	if !collectProfileJSON {
		fmt.Printf("Selecting %s view...\n", buttonName)
	}

	if err := axAction(btn, "AXPress"); err != nil {
		if collectProfileJSON {
			return outputJSONError("CLICK_FAILED", fmt.Sprintf("failed to click: %v", err), "Try again")
		}
		return fmt.Errorf("failed to click: %w", err)
	}

	if collectProfileJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(map[string]interface{}{
			"success": true,
			"view":    viewName,
		})
	}

	fmt.Println("Done")
	return nil
}
