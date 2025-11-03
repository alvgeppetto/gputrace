package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/tmc/mlx-go/experiments/gputrace"
)

func main() {
	var (
		outputJSON    = flag.String("json", "", "Output JSON to file")
		outputCSV     = flag.String("csv", "", "Output CSV to file")
		compareWith   = flag.String("compare", "", "Compare with baseline trace")
		showTable     = flag.Bool("table", true, "Show human-readable table")
	)

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [options] <trace.gputrace>\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "Extract and export comprehensive timing metrics from GPU traces.\n\n")
		fmt.Fprintf(os.Stderr, "Options:\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nExamples:\n")
		fmt.Fprintf(os.Stderr, "  # Show timing table\n")
		fmt.Fprintf(os.Stderr, "  %s trace.gputrace\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  # Export to JSON and CSV\n")
		fmt.Fprintf(os.Stderr, "  %s -json timing.json -csv timing.csv trace.gputrace\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  # Compare two traces for regressions\n")
		fmt.Fprintf(os.Stderr, "  %s -compare baseline.gputrace current.gputrace\n\n", os.Args[0])
	}

	flag.Parse()

	if flag.NArg() < 1 {
		flag.Usage()
		os.Exit(1)
	}

	tracePath := flag.Arg(0)

	// Open trace
	trace, err := gputrace.Open(tracePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error opening trace: %v\n", err)
		os.Exit(1)
	}

	// Extract timing metrics
	extractor := gputrace.NewTimingMetricsExtractor(trace)
	metrics, err := extractor.Extract()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error extracting timing metrics: %v\n", err)
		os.Exit(1)
	}

	// Show table if requested
	if *showTable {
		report := gputrace.FormatTimingMetrics(metrics)
		fmt.Println(report)
	}

	// Export JSON if requested
	if *outputJSON != "" {
		f, err := os.Create(*outputJSON)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error creating JSON file: %v\n", err)
			os.Exit(1)
		}
		defer f.Close()

		if err := gputrace.ExportTimingMetricsJSON(f, metrics); err != nil {
			fmt.Fprintf(os.Stderr, "Error writing JSON: %v\n", err)
			os.Exit(1)
		}
		fmt.Fprintf(os.Stderr, "Exported JSON to %s\n", *outputJSON)
	}

	// Export CSV if requested
	if *outputCSV != "" {
		f, err := os.Create(*outputCSV)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error creating CSV file: %v\n", err)
			os.Exit(1)
		}
		defer f.Close()

		if err := gputrace.ExportTimingMetricsCSV(f, metrics); err != nil {
			fmt.Fprintf(os.Stderr, "Error writing CSV: %v\n", err)
			os.Exit(1)
		}
		fmt.Fprintf(os.Stderr, "Exported CSV to %s\n", *outputCSV)
	}

	// Compare traces if requested
	if *compareWith != "" {
		baselineTrace, err := gputrace.Open(*compareWith)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error opening baseline trace: %v\n", err)
			os.Exit(1)
		}

		baselineExtractor := gputrace.NewTimingMetricsExtractor(baselineTrace)
		baselineMetrics, err := baselineExtractor.Extract()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error extracting baseline metrics: %v\n", err)
			os.Exit(1)
		}

		comparison := gputrace.CompareTraces(baselineMetrics, metrics)
		fmt.Println("\n" + gputrace.FormatTimingComparison(comparison))

		if comparison.RegressionCount > 0 {
			os.Exit(2) // Exit with code 2 to indicate regressions found
		}
	}
}
