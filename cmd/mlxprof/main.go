package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"

	"github.com/google/pprof/profile"
	"github.com/tmc/gputrace/internal/export"
	"github.com/tmc/gputrace/internal/trace"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	var (
		cpuProfile = flag.String("cpu", "cpu.pprof", "Input CPU profile")
		gpuTrace   = flag.String("gpu", "", "Input GPU trace (.gputrace)")
		output     = flag.String("o", "merged.pprof", "Output merged profile")
		runCmd     = flag.Bool("run", false, "Run the command (capture mode)")
	)
	flag.Parse()

	if *runCmd {
		args := flag.Args()
		if len(args) == 0 {
			return fmt.Errorf("no command specified to run")
		}
		return runCapture(args, *output)
	}

	if *gpuTrace == "" {
		return fmt.Errorf("gpu trace required (use -gpu)")
	}

	return mergeProfiles(*cpuProfile, *gpuTrace, *output)
}

func runCapture(args []string, output string) error {
	// 1. Setup Environment
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Env = os.Environ()
	cmd.Env = append(cmd.Env, "MTL_CAPTURE_ENABLED=1")
	cmd.Env = append(cmd.Env, "GPUPROFILER_TRACE_DESTINATION=trace.gputrace")

	// 2. Run
	fmt.Printf("Running: %v\n", args)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("command execution failed: %w", err)
	}

	// 3. Post-process
	// Assume the app wrote cpu.pprof itself (for now) via standard runtime/pprof
	// TODO: Inject a wrapper or use a signal to trigger pprof?
	// For this prototype, we assume the user app writes cpu.pprof.

	return mergeProfiles("cpu.pprof", "trace.gputrace", output)
}

func mergeProfiles(cpuPath, gpuPath, outputPath string) error {
	fmt.Printf("Merging %s and %s -> %s\n", cpuPath, gpuPath, outputPath)

	// Load CPU Profile
	fCPU, err := os.Open(cpuPath)
	if err != nil {
		return fmt.Errorf("open cpu profile: %w", err)
	}
	defer fCPU.Close()
	cpuProf, err := profile.Parse(fCPU)
	if err != nil {
		return fmt.Errorf("parse cpu profile: %w", err)
	}

	// Load GPU Trace
	t, err := trace.Open(gpuPath)
	if err != nil {
		return fmt.Errorf("open gpu trace: %w", err)
	}
	defer t.Close()

	// Convert GPU Trace to Pprof
	gpuProf, err := export.ToPprofWithMetrics(t, nil, nil)
	if err != nil {
		return fmt.Errorf("convert gpu trace: %w", err)
	}

	// Schema Unification
	// We want to merge CPU and GPU profiles.
	// Logic:
	// 1. Adopt CPU profile's PeriodType.
	// 2. Normalize Value types.
	//    CPU: [samples count, cpu nanoseconds]  (Typical Go pprof)
	//    GPU: [time nanoseconds, count, edges, alu, occ, read, write]  (From export.ToPprofWithMetrics)
	//
	// Strategy:
	// Transform GPU profile to match CPU profile structure:
	//   Values: [count, nanoseconds]
	//   Mapping:
	//     GPU Count -> Index 0
	//     GPU Time  -> Index 1

	if len(cpuProf.SampleType) == 2 && cpuProf.SampleType[1].Unit == "nanoseconds" {
		fmt.Println("Adapting GPU profile to match Go CPU profile format...")
		gpuProf.PeriodType = cpuProf.PeriodType
		gpuProf.SampleType = cpuProf.SampleType

		// Remap GPU samples
		for _, s := range gpuProf.Sample {
			// Original GPU: [time, count, edges]
			// Target: [count, time]

			// Extract from original (assuming order from pprof_enhanced.go: time, count, edges)
			// But check pprof_enhanced.go guarantees.
			// It sets: {Type: "time", Unit: "nanoseconds"}, {Type: "count", Unit: "count"}, {Type: "edges", Unit: "count"}

			var timeVal int64
			var countVal int64

			if len(s.Value) >= 2 {
				timeVal = s.Value[0]
				countVal = s.Value[1]
			} else if len(s.Value) == 1 {
				timeVal = s.Value[0]
				countVal = 1
			}

			s.Value = []int64{countVal, timeVal}
		}
	} else {
		// Fallback: Just force PeriodType to match to try standard merge,
		// but if SampleTypes differ in count/unit, pprof.Merge will still fail or drop data.
		fmt.Printf("Warning: CPU profile has unexpected format: %v. Attempting best-effort merge.\n", cpuProf.SampleType)
		gpuProf.PeriodType = cpuProf.PeriodType
		// We can't easily unify values if we don't know the schema.
		// But let's try to match PeriodType at least.
	}

	// Merge
	// Ideally we align timestamps here.
	// For P0, we just merge them content-wise.
	// Note: pprof.Merge treats profiles as samples from the same binary.
	// We are merging "Go" and "Metal".

	merged, err := profile.Merge([]*profile.Profile{cpuProf, gpuProf})
	if err != nil {
		return fmt.Errorf("merge profiles: %w", err)
	}

	// Write
	outF, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("create output: %w", err)
	}
	defer outF.Close()

	return merged.Write(outF)
}
