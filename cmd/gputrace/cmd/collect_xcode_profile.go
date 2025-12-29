package cmd

import (
	"context"
	"embed"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/tmc/gputrace/internal/osa"
	"github.com/tmc/macgo"
)

//go:embed applescripts/*.applescript
var embeddedScripts embed.FS

// scriptsDir is the directory to check for script overrides (for faster iteration)
var scriptsDir = filepath.Join(os.Getenv("HOME"), ".config", "gputrace", "applescripts")

// loadScript loads an AppleScript, checking for a disk override first.
// This allows editing scripts without rebuilding the binary.
func loadScript(name string) string {
	// Check for override on disk first
	diskPath := filepath.Join(scriptsDir, name)
	if data, err := os.ReadFile(diskPath); err == nil {
		if collectProfileDebug {
			fmt.Printf("    [Using script from %s]\n", diskPath)
		}
		return string(data)
	}

	// Fall back to embedded script
	data, err := embeddedScripts.ReadFile("applescripts/" + name)
	if err != nil {
		// Should not happen with valid embedded scripts
		return ""
	}
	return string(data)
}

var collectXcodeProfileCmd = &cobra.Command{
	Use:   "collect-xcode-profile <trace_file>",
	Short: "Automate Xcode replay and export (Experimental)",
	Long: `Automates the process of opening a GPU trace in Xcode, replaying it to capture performance counters,
and exporting the result.

This command uses AppleScript to control Xcode's UI. It is experimental and relies on Xcode's UI structure remaining consistent.
Requires 'Accessibility' permissions for the terminal/script runner if strictly UI scripting is used, though we attempt to use Menu scripting where possible.

Example:
  gputrace collect-xcode-profile my_capture.gputrace --output my_capture_profiled.gputrace
`,
	Args: cobra.ExactArgs(1),
	RunE: runCollectXcodeProfile,
}

var (
	collectProfileOutput   string
	collectProfileTimeout  time.Duration
	collectProfileDebug    bool
	collectProfileNoBundle bool
)

func init() {
	rootCmd.AddCommand(collectXcodeProfileCmd)
	collectXcodeProfileCmd.Flags().StringVarP(&collectProfileOutput, "output", "o", "", "Output path for the exported trace (default: <input>_profiled.gputrace)")
	collectXcodeProfileCmd.Flags().DurationVar(&collectProfileTimeout, "timeout", 5*time.Minute, "Timeout for the entire operation")
	collectXcodeProfileCmd.Flags().BoolVar(&collectProfileDebug, "debug", false, "Print Xcode UI hierarchy for debugging")
	collectXcodeProfileCmd.Flags().BoolVar(&collectProfileNoBundle, "no-bundle", false, "Skip macgo app bundle (use Terminal's Accessibility permission)")
}

func runCollectXcodeProfile(cmd *cobra.Command, args []string) error {
	// Calculate absolute paths BEFORE macgo.Start() changes working directory
	inputPath, err := filepath.Abs(args[0])
	if err != nil {
		return fmt.Errorf("invalid input path: %w", err)
	}

	if collectProfileOutput == "" {
		ext := filepath.Ext(inputPath)
		base := inputPath[:len(inputPath)-len(ext)]
		collectProfileOutput = base + "_profiled" + ext
	}
	outputPath, err := filepath.Abs(collectProfileOutput)
	if err != nil {
		return fmt.Errorf("invalid output path: %w", err)
	}

	// Use macgo for proper TCC permissions (unless --no-bundle)
	if collectProfileNoBundle || os.Getenv("GPUTRACE_SKIP_MACGO") != "" {
		fmt.Printf("Skipping macgo (using current process identity)\n")
	} else {
		// Use ServicesLauncherV1
		os.Setenv("MACGO_SERVICES_VERSION", "1")

		cfg := &macgo.Config{
			AppName:  "gputrace",
			BundleID: "com.tmc.gputrace",
			Custom: []string{
				"com.apple.security.automation.apple-events",
			},
			Debug:            os.Getenv("MACGO_DEBUG") == "1",
			CodeSignIdentity: "Apple Development", // Use stable signing identity
		}

		// Always call macgo.Start() - it handles both:
		// 1. First run: creates app bundle and relaunches via LaunchServices
		// 2. Relaunched run (inside .app): sets up I/O forwarding back to parent
		if err := macgo.Start(cfg); err != nil {
			fmt.Printf(Colorize("macgo app bundle setup failed: %v\n", ColorRed), err)
			fmt.Printf("\nThe app bundle is required for Accessibility permissions.\n")
			fmt.Printf("Try these steps:\n")
			fmt.Printf("  1. Reset TCC: tccutil reset Accessibility com.tmc.gputrace\n")
			fmt.Printf("  2. Set debug: export MACGO_DEBUG=1\n")
			fmt.Printf("  3. Re-run the command\n")
			fmt.Printf("\nOr use --no-bundle if Terminal.app has Accessibility permission.\n")
			return fmt.Errorf("macgo setup failed: %w", err)
		}
	}

	// Pre-flight: Check Accessibility permission (warning only - actual test below)
	if !osa.HasAccessibilityPermission() {
		fmt.Printf(Colorize("Note: Accessibility check returned false, but continuing to test...\n", ColorYellow))
	}

	// Pre-flight: Test Automation permission by controlling System Events
	testScript := `tell application "System Events" to set frontmost of process "Finder" to true`
	if err := runOSA(testScript); err != nil {
		if strings.Contains(err.Error(), "-1743") {
			fmt.Printf(Colorize("\nAutomation permission required.\n", ColorYellow))
			fmt.Printf("gputrace.app needs permission to control System Events.\n\n")
			fmt.Printf("Opening System Settings...\n")
			// Open System Settings to the Automation pane
			_ = exec.Command("open", "x-apple.systempreferences:com.apple.preference.security?Privacy_Automation").Run()
			fmt.Printf("\nPlease grant permission:\n")
			fmt.Printf("  1. Find 'gputrace.app' in the list\n")
			fmt.Printf("  2. Enable the 'System Events' checkbox\n")
			fmt.Printf("  3. Re-run this command\n")
			return fmt.Errorf("automation permission required")
		}
		fmt.Printf(Colorize("\nAccessibility permission required.\n", ColorYellow))
		fmt.Printf("Please grant Accessibility permission to gputrace.app in:\n")
		fmt.Printf("  System Settings -> Privacy & Security -> Accessibility\n")
		return fmt.Errorf("accessibility permission required: %w", err)
	}

	fmt.Printf(Colorize("Collect Profile: Automating Xcode GPU trace...\n", ColorBold))
	fmt.Printf("  Input:  %s\n", inputPath)
	fmt.Printf("  Output: %s\n", outputPath)

	ctx, cancel := context.WithTimeout(context.Background(), collectProfileTimeout)
	defer cancel()

	// Step 1: Open File in Xcode
	fmt.Println("  Step 1: Opening trace in Xcode...")
	fmt.Printf("    File: %s\n", inputPath)

	// Check file exists
	if _, err := os.Stat(inputPath); os.IsNotExist(err) {
		return fmt.Errorf("trace file does not exist: %s", inputPath)
	}

	openCmd := exec.CommandContext(ctx, "open", "-a", "Xcode", inputPath)
	if output, err := openCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to open trace in Xcode: %w\n    output: %s", err, string(output))
	}

	// Wait for Xcode to launch
	time.Sleep(3 * time.Second)

	// Step 2: Wait for window and dismiss dialogs
	fmt.Println("  Step 2: Waiting for Xcode window...")
	if err := runOSAWithDebug(waitForXcodeScript(), collectProfileDebug); err != nil {
		if strings.Contains(err.Error(), "-1743") {
			fmt.Printf(Colorize("\nAutomation permission required for Xcode control.\n", ColorYellow))
			fmt.Printf("gputrace.app needs permission to control System Events.\n\n")
			fmt.Printf("Opening System Settings -> Automation...\n")
			exec.Command("open", "x-apple.systempreferences:com.apple.preference.security?Privacy_Automation").Run()
			fmt.Printf("\nPlease enable 'System Events' for gputrace.app, then re-run the command.\n")
			return fmt.Errorf("automation permission required")
		}
		if strings.Contains(err.Error(), "-25211") {
			fmt.Printf(Colorize("\nAccessibility permission required.\n", ColorYellow))
			fmt.Printf("Opening System Settings -> Accessibility...\n")
			exec.Command("open", "x-apple.systempreferences:com.apple.preference.security?Privacy_Accessibility").Run()
			fmt.Printf("\nPlease enable gputrace.app, then re-run the command.\n")
			return fmt.Errorf("accessibility permission required")
		}
		return fmt.Errorf("waiting for Xcode window: %w", err)
	}

	// Debug mode: dump menu items to see what's available
	if collectProfileDebug {
		fmt.Println("  Debug: Dumping Xcode menu items...")
		if output, err := osa.Execute(debugUIHierarchyScript()); err == nil {
			fmt.Printf("%s\n", output)
		} else {
			fmt.Printf("    Debug error: %v\n", err)
			if strings.Contains(err.Error(), "assistive access") {
				fmt.Printf("    (Accessibility permission needed for UI scripting)\n")
			}
		}
	}

	// Step 3: Find and click Replay button
	fmt.Println("  Step 3: Starting replay...")
	replayErr := runOSAWithDebug(replayScript(), collectProfileDebug)
	if replayErr != nil {
		return fmt.Errorf("replay automation failed: %w\n\nTo replay manually:\n  1. Click the 'Replay' button in Xcode (or Document > Replay)\n  2. Wait for the replay to finish\n  3. File > Export to save with counters", replayErr)
	}

	// Step 4: Wait for replay to complete (poll for status indicator)
	fmt.Println("  Step 4: Waiting for replay to complete...")
	if err := waitForReplayCompletion(ctx); err != nil {
		fmt.Printf(Colorize("    Warning: %v\n", ColorYellow), err)
		fmt.Printf("    Continuing with export...\n")
	}

	// Step 5: Export the trace
	fmt.Println("  Step 5: Exporting trace...")
	exportErr := runOSAWithDebug(exportScript(filepath.Dir(outputPath), filepath.Base(outputPath)), collectProfileDebug)
	if exportErr != nil {
		return fmt.Errorf("export automation failed: %w\n\nTo export manually:\n  1. File > Export...\n  2. Save as: %s\n  3. Save to: %s", exportErr, filepath.Base(outputPath), filepath.Dir(outputPath))
	}

	// Wait for export to complete
	time.Sleep(2 * time.Second)

	// Check if output file exists
	if _, err := os.Stat(outputPath); err == nil {
		fmt.Printf(Colorize("\nDone! Output saved to: %s\n", ColorGreen), outputPath)
	} else {
		fmt.Printf(Colorize("\nNote: Output file not found at expected location.\n", ColorYellow))
		fmt.Printf("Check Xcode for the exported file.\n")
	}
	return nil
}

// runOSA executes an AppleScript in-process via NSAppleScript.
// This inherits TCC permissions from the app bundle (unlike osascript subprocess).
func runOSA(script string) error {
	return runOSAWithDebug(script, false)
}

// runOSAWithDebug executes an AppleScript in-process with optional debug output.
func runOSAWithDebug(script string, debug bool) error {
	if debug {
		// Show first 200 chars of script
		preview := script
		if len(preview) > 200 {
			preview = preview[:200] + "..."
		}
		fmt.Printf("    [DEBUG] Running script: %s\n", strings.ReplaceAll(preview, "\n", " "))
	}

	result, err := osa.Execute(script)

	if debug {
		fmt.Printf("    [DEBUG] Output: %s\n", strings.TrimSpace(result))
		if err != nil {
			fmt.Printf("    [DEBUG] Error: %v\n", err)
		}
	}

	if err != nil {
		return err
	}
	if result != "" {
		fmt.Printf("    AppleScript result: %s\n", result)
	}
	return nil
}

// waitForXcodeScript waits for Xcode window to appear.
func waitForXcodeScript() string {
	return loadScript("wait_for_xcode.applescript")
}

// replayScript attempts to find and click the Replay button.
func replayScript() string {
	return loadScript("replay.applescript")
}

// exportScript exports the trace via File menu.
func exportScript(outputDir, outputName string) string {
	script := loadScript("export.applescript")
	script = strings.ReplaceAll(script, "{{OUTPUT_DIR}}", outputDir)
	script = strings.ReplaceAll(script, "{{OUTPUT_NAME}}", outputName)
	return script
}

// debugUIHierarchyScript returns a script that dumps the Xcode UI hierarchy.
func debugUIHierarchyScript() string {
	return loadScript("debug_ui.applescript")
}

// waitForReplayCompletion polls Xcode to detect when replay is complete.
// It looks for status text changes or progress indicators.
func waitForReplayCompletion(ctx context.Context) error {
	// Poll for up to 30 seconds for replay completion
	pollInterval := 2 * time.Second
	maxPollTime := 30 * time.Second
	start := time.Now()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if time.Since(start) > maxPollTime {
			return fmt.Errorf("timed out waiting for replay completion")
		}

		// Check if replay appears complete by looking for status indicators
		output, err := osa.Execute(checkReplayStatusScript())
		if err == nil && strings.Contains(output, "complete") {
			fmt.Println("    Replay completed")
			return nil
		}

		time.Sleep(pollInterval)
		fmt.Printf("    Still waiting for replay... (%.0fs)\n", time.Since(start).Seconds())
	}
}

// checkReplayStatusScript checks if replay is still running.
func checkReplayStatusScript() string {
	return loadScript("check_replay_status.applescript")
}
