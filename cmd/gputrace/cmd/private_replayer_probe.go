//go:build darwin

package cmd

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"github.com/tmc/apple/x/plist"
	"github.com/tmc/gputrace"
	"github.com/tmc/gputrace/internal/counter"
	gputracetrace "github.com/tmc/gputrace/internal/trace"
)

const privateReplayerCaptureLimit = 64 * 1024

var (
	privateReplayerJSON              bool
	privateReplayerTimeout           time.Duration
	privateReplayerPath              string
	privateReplayerOutDir            string
	privateReplayerDirect            bool
	privateReplayerSigned            bool
	privateReplayerSkipCLI           bool
	privateReplayerDirectModes       []string
	privateReplayerSignedModes       []string
	privateReplayerClang             string
	privateReplayerTimingRowsJSON    string
	privateReplayerMinFreeGiB        float64
	privateReplayerMinMemFree        int
	privateReplayerAllowLowResources bool
)

type privateReplayerProbeOutput struct {
	TracePath            string                           `json:"trace_path"`
	ReplayerPath         string                           `json:"replayer_path"`
	FrameworkPath        string                           `json:"framework_path"`
	TimeoutSeconds       float64                          `json:"timeout_seconds"`
	ResourcePreflight    privateReplayerResourcePreflight `json:"resource_preflight"`
	ReplayCandidates     replayActivationCandidates       `json:"replay_candidates"`
	Commands             []privateReplayerResult          `json:"commands"`
	ProfilerPayload      filePresence                     `json:"profiler_payload"`
	StreamData           filePresence                     `json:"streamData"`
	StreamDataStats      *streamDataProbeStats            `json:"streamData_stats,omitempty"`
	ServiceStreams       serviceStreamSummary             `json:"service_streams"`
	TimingClaimsAllowed  bool                             `json:"timing_claims_allowed"`
	CounterClaimsAllowed bool                             `json:"counter_claims_allowed"`
	Viable               bool                             `json:"viable"`
	Reason               string                           `json:"reason"`
}

type privateReplayerResourcePreflight struct {
	OutputDir         string  `json:"output_dir"`
	CheckedPath       string  `json:"checked_path,omitempty"`
	FreeGiB           float64 `json:"free_gib,omitempty"`
	MemoryFreePercent int     `json:"memory_free_percent,omitempty"`
	MinFreeGiB        float64 `json:"min_free_gib"`
	MinMemoryFree     int     `json:"min_memory_free_percent"`
	AllowLowResources bool    `json:"allow_low_resources"`
}

type replayActivationCandidates struct {
	ResourceUsage string `json:"resource_usage,omitempty"`
	FetchPipeline string `json:"fetch_pipeline,omitempty"`
	UpdateLibrary string `json:"update_library,omitempty"`
	FetchTexture  string `json:"fetch_texture,omitempty"`
	FetchBuffer   string `json:"fetch_buffer,omitempty"`
	AnyConcrete   bool   `json:"any_concrete"`
}

type serviceStreamSummary struct {
	ResponseCount             int `json:"response_count"`
	MarkerPayloadCount        int `json:"marker_payload_count"`
	NestedMarkerPayloadCount  int `json:"nested_marker_payload_count"`
	NonMarkerDataPayloadCount int `json:"non_marker_data_payload_count"`
	DerivedCounterRows        int `json:"derived_counter_rows"`
	StreamDataFileCount       int `json:"streamData_file_count"`
	UsableStreamDataFileCount int `json:"usable_streamData_file_count"`
}

type streamDataProbeStats struct {
	ParseError                string `json:"parse_error,omitempty"`
	NumEncoders               int    `json:"num_encoders"`
	NumGPUCommands            int    `json:"num_gpu_commands"`
	NumPipelines              int    `json:"num_pipelines"`
	DispatchCount             int    `json:"dispatch_count"`
	EncoderTimingCount        int    `json:"encoder_timing_count"`
	DerivedCounterSampleCount int    `json:"derived_counter_sample_count"`
	TotalTimeUs               int    `json:"total_time_us"`
	TimingUsable              bool   `json:"timing_usable"`
	CounterUsable             bool   `json:"counter_usable"`
}

type privateReplayerResult struct {
	Name            string                        `json:"name"`
	Cmd             []string                      `json:"cmd"`
	OutputDir       string                        `json:"output_dir,omitempty"`
	ElapsedMillis   int64                         `json:"elapsed_millis"`
	TimedOut        bool                          `json:"timed_out"`
	ExitCode        int                           `json:"exit_code,omitempty"`
	Signal          string                        `json:"signal,omitempty"`
	StdoutBytes     int                           `json:"stdout_bytes"`
	StderrBytes     int                           `json:"stderr_bytes"`
	StdoutPath      string                        `json:"stdout_path,omitempty"`
	StderrPath      string                        `json:"stderr_path,omitempty"`
	StdoutPreview   string                        `json:"stdout_preview,omitempty"`
	StderrPreview   string                        `json:"stderr_preview,omitempty"`
	FileCount       int                           `json:"file_count"`
	ProfilerFiles   []string                      `json:"profiler_files,omitempty"`
	ServicePayloads []replayServicePayloadSummary `json:"service_payloads,omitempty"`
}

type replayServicePayloadSummary struct {
	Path                           string                 `json:"path"`
	Kind                           string                 `json:"kind"`
	Keys                           []string               `json:"keys,omitempty"`
	ArchiveClassHints              []string               `json:"archive_class_hints,omitempty"`
	DataValueBytes                 map[string]int         `json:"data_value_bytes,omitempty"`
	ResponseCount                  int                    `json:"response_count,omitempty"`
	ResponseDataBytes              []int                  `json:"response_data_bytes,omitempty"`
	ResponseDataPayloads           []responseDataSummary  `json:"response_data_payloads,omitempty"`
	ResponseErrors                 []responseErrorSummary `json:"response_errors,omitempty"`
	NumberOfPasses                 int                    `json:"number_of_passes,omitempty"`
	CounterLists                   [][]string             `json:"counter_lists,omitempty"`
	Counters                       []string               `json:"counters,omitempty"`
	AverageSampleRowCount          int                    `json:"average_sample_row_count,omitempty"`
	FirstAverageSampleByCounter    map[string][]uint64    `json:"first_average_sample_by_counter,omitempty"`
	FirstAverageSampleByGRCCounter map[string][]uint64    `json:"first_average_sample_by_grc_counter,omitempty"`
	StreamingAPSData               *bool                  `json:"streaming_aps_data,omitempty"`
	BatchFilteringStarted          *bool                  `json:"batch_filtering_started,omitempty"`
	Error                          string                 `json:"error,omitempty"`
}

type responseDataSummary struct {
	Bytes             int      `json:"bytes"`
	Kind              string   `json:"kind,omitempty"`
	Keys              []string `json:"keys,omitempty"`
	ArchiveClassHints []string `json:"archive_class_hints,omitempty"`
}

type responseErrorSummary struct {
	Domain             string `json:"domain,omitempty"`
	Code               int    `json:"code,omitempty"`
	Description        string `json:"description,omitempty"`
	RecoverySuggestion string `json:"recovery_suggestion,omitempty"`
}

var privateReplayerProbeCmd = &cobra.Command{
	Use:   "private-replayer-probe <trace.gputrace>",
	Short: "Probe Apple's private MTLReplayer profiler CLI surface",
	Long: `Probe whether Apple's private MTLReplayer binary can generate profiler
payloads for a .gputrace bundle from a bounded, non-UI command-line invocation.

This command does not use Xcode UI automation, Accessibility, screenshots, or
synthetic data. It reports whether MTLReplayer creates .gpuprofiler_raw,
streamData, or known profiler raw-file families. A failed probe is evidence
about local CLI viability, not a parser error.`,
	Args: cobra.ExactArgs(1),
	RunE: runPrivateReplayerProbe,
}

var experimentalMTLReplayerCmd = &cobra.Command{
	Use:   "experimental-mtlreplayer <trace.gputrace>",
	Short: "Try to generate a real profiler payload with Apple's private MTLReplayer",
	Long: `Try to generate .gpuprofiler_raw/streamData by driving Apple's private
MTLReplayer/GPUToolsReplay surfaces from a non-UI command-line runner.

This is an experimental, fail-closed backend. It returns an error unless a real
profiler payload with streamData is produced. It never fabricates timing or
	counter data.`,
	Args:         cobra.ExactArgs(1),
	SilenceUsage: true,
	RunE:         runExperimentalMTLReplayer,
}

func init() {
	rootCmd.AddCommand(privateReplayerProbeCmd)
	rootCmd.AddCommand(experimentalMTLReplayerCmd)
	privateReplayerProbeCmd.Flags().BoolVar(&privateReplayerJSON, "json", false, "Output in JSON format")
	privateReplayerProbeCmd.Flags().DurationVar(&privateReplayerTimeout, "timeout", 3*time.Second, "Timeout per MTLReplayer invocation")
	privateReplayerProbeCmd.Flags().StringVar(&privateReplayerPath, "mtlreplayer", "", "Path to MTLReplayer binary")
	privateReplayerProbeCmd.Flags().StringVar(&privateReplayerOutDir, "out-dir", "", "Directory for temporary probe outputs")
	privateReplayerProbeCmd.Flags().BoolVar(&privateReplayerSkipCLI, "skip-mtlreplayer-cli", true, "Skip direct MTLReplayer.app -CLI invocations")
	privateReplayerProbeCmd.Flags().BoolVar(&privateReplayerDirect, "direct-framework", true, "Also probe GPUToolsReplay._GTMTLReplay_CLI through an isolated helper")
	privateReplayerProbeCmd.Flags().BoolVar(&privateReplayerSigned, "signed-service", true, "Also probe Apple-signed GPUToolsReplayService through an isolated helper")
	privateReplayerProbeCmd.Flags().StringArrayVar(&privateReplayerDirectModes, "direct-mode", nil, "Direct-framework helper mode to run; repeat to run a subset")
	privateReplayerProbeCmd.Flags().StringArrayVar(&privateReplayerSignedModes, "signed-mode", nil, "Signed-service helper mode to run; repeat to run a subset")
	privateReplayerProbeCmd.Flags().StringVar(&privateReplayerTimingRowsJSON, "timing-rows-json", "", "Existing timing rows JSON for trace_timing_rows* signed-service modes")
	privateReplayerProbeCmd.Flags().StringVar(&privateReplayerClang, "clang", "clang", "C compiler used for the isolated direct-framework helper")
	privateReplayerProbeCmd.Flags().Float64Var(&privateReplayerMinFreeGiB, "min-out-dir-free-gib", 24, "Minimum free GiB required on the output directory volume before launching replay tools")
	privateReplayerProbeCmd.Flags().IntVar(&privateReplayerMinMemFree, "min-memory-free-percent", 10, "Minimum memory_pressure free percentage required before launching replay tools")
	privateReplayerProbeCmd.Flags().BoolVar(&privateReplayerAllowLowResources, "allow-low-resources", false, "Run replay tools even if disk or memory preflight is below threshold")
	experimentalMTLReplayerCmd.Flags().BoolVar(&privateReplayerJSON, "json", false, "Output in JSON format")
	experimentalMTLReplayerCmd.Flags().DurationVar(&privateReplayerTimeout, "timeout", 10*time.Second, "Timeout per MTLReplayer invocation")
	experimentalMTLReplayerCmd.Flags().StringVar(&privateReplayerPath, "mtlreplayer", "", "Path to MTLReplayer binary")
	experimentalMTLReplayerCmd.Flags().StringVar(&privateReplayerOutDir, "out-dir", "", "Directory for generated profiler outputs")
	experimentalMTLReplayerCmd.Flags().BoolVar(&privateReplayerSkipCLI, "skip-mtlreplayer-cli", true, "Skip direct MTLReplayer.app -CLI invocations")
	experimentalMTLReplayerCmd.Flags().BoolVar(&privateReplayerDirect, "direct-framework", true, "Also try GPUToolsReplay._GTMTLReplay_CLI through an isolated helper")
	experimentalMTLReplayerCmd.Flags().BoolVar(&privateReplayerSigned, "signed-service", true, "Also probe Apple-signed GPUToolsReplayService through an isolated helper")
	experimentalMTLReplayerCmd.Flags().StringArrayVar(&privateReplayerDirectModes, "direct-mode", nil, "Direct-framework helper mode to run; repeat to run a subset")
	experimentalMTLReplayerCmd.Flags().StringArrayVar(&privateReplayerSignedModes, "signed-mode", nil, "Signed-service helper mode to run; repeat to run a subset")
	experimentalMTLReplayerCmd.Flags().StringVar(&privateReplayerClang, "clang", "clang", "C compiler used for the isolated direct-framework helper")
	experimentalMTLReplayerCmd.Flags().Float64Var(&privateReplayerMinFreeGiB, "min-out-dir-free-gib", 24, "Minimum free GiB required on the output directory volume before launching replay tools")
	experimentalMTLReplayerCmd.Flags().IntVar(&privateReplayerMinMemFree, "min-memory-free-percent", 10, "Minimum memory_pressure free percentage required before launching replay tools")
	experimentalMTLReplayerCmd.Flags().BoolVar(&privateReplayerAllowLowResources, "allow-low-resources", false, "Run replay tools even if disk or memory preflight is below threshold")
}

func runPrivateReplayerProbe(cmd *cobra.Command, args []string) error {
	loadPrivateReplayerFlags(cmd)
	output, err := probePrivateReplayer(args[0])
	if err != nil {
		return err
	}
	if privateReplayerJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(output)
	}
	printPrivateReplayerOutput(output)
	return nil
}

func runExperimentalMTLReplayer(cmd *cobra.Command, args []string) error {
	loadPrivateReplayerFlags(cmd)
	output, err := probePrivateReplayer(args[0])
	if err != nil {
		return err
	}
	if privateReplayerJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(output); err != nil {
			return err
		}
	} else {
		printPrivateReplayerOutput(output)
	}
	if !output.Viable {
		return fmt.Errorf("experimental MTLReplayer backend failed closed: %s", output.Reason)
	}
	return nil
}

func loadPrivateReplayerFlags(cmd *cobra.Command) {
	if v, err := cmd.Flags().GetBool("json"); err == nil {
		privateReplayerJSON = v
	}
	if v, err := cmd.Flags().GetDuration("timeout"); err == nil {
		privateReplayerTimeout = v
	}
	if v, err := cmd.Flags().GetString("mtlreplayer"); err == nil {
		privateReplayerPath = v
	}
	if v, err := cmd.Flags().GetString("out-dir"); err == nil {
		privateReplayerOutDir = v
	}
	if v, err := cmd.Flags().GetBool("skip-mtlreplayer-cli"); err == nil {
		privateReplayerSkipCLI = v
	}
	if v, err := cmd.Flags().GetBool("direct-framework"); err == nil {
		privateReplayerDirect = v
	}
	if v, err := cmd.Flags().GetBool("signed-service"); err == nil {
		privateReplayerSigned = v
	}
	if v, err := cmd.Flags().GetStringArray("direct-mode"); err == nil {
		privateReplayerDirectModes = v
	}
	if v, err := cmd.Flags().GetStringArray("signed-mode"); err == nil {
		privateReplayerSignedModes = v
	}
	if v, err := cmd.Flags().GetString("clang"); err == nil {
		privateReplayerClang = v
	}
	if v, err := cmd.Flags().GetFloat64("min-out-dir-free-gib"); err == nil {
		privateReplayerMinFreeGiB = v
	}
	if v, err := cmd.Flags().GetInt("min-memory-free-percent"); err == nil {
		privateReplayerMinMemFree = v
	}
	if v, err := cmd.Flags().GetBool("allow-low-resources"); err == nil {
		privateReplayerAllowLowResources = v
	}
}

func probePrivateReplayer(tracePath string) (*privateReplayerProbeOutput, error) {
	if err := checkTraceFile(tracePath); err != nil {
		return nil, err
	}
	replayer := privateReplayerPath
	if replayer == "" {
		replayer = "/System/Library/CoreServices/MTLReplayer.app/Contents/MacOS/MTLReplayer"
	}
	if _, err := os.Stat(replayer); err != nil {
		return nil, fmt.Errorf("MTLReplayer not found: %s", replayer)
	}
	outDir := privateReplayerOutDir
	if outDir == "" {
		outDir = filepath.Join(os.TempDir(), "gputrace-private-replayer-probe")
	}
	resourcePreflight := collectPrivateReplayerResourcePreflight(outDir)
	if err := preflightPrivateReplayerResources(outDir); err != nil {
		return nil, err
	}
	if err := os.RemoveAll(outDir); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return nil, err
	}
	framework := "/System/Library/PrivateFrameworks/GPUToolsReplay.framework/GPUToolsReplay"
	replayCandidates := collectReplayActivationCandidates(tracePath)

	results := []privateReplayerResult{}
	if !privateReplayerSkipCLI {
		results = append(results,
			runPrivateReplayerCommand("list_devices", replayer, []string{"-CLI", tracePath, "--list-devices"}, privateReplayerTimeout),
			runPrivateReplayerCommand(
				"profile_data",
				replayer,
				[]string{
					"-CLI",
					tracePath,
					"-profileTrace",
					"-collectProfilerData",
					"-collectPerformanceTiming",
					"-gpuTimelineData",
					"-collectPipelinePerformanceStatistics",
					"--output",
					filepath.Join(outDir, "profile-data"),
					"--frame",
					"0",
				},
				privateReplayerTimeout,
			),
			runPrivateReplayerCommand(
				"profile_full_analysis",
				replayer,
				[]string{
					"-CLI",
					tracePath,
					"-profileTrace",
					"--performAnalysis",
					filepath.Join(outDir, "profile-full-analysis"),
					"--performShaderProfilingAnalysis",
					filepath.Join(outDir, "profile-full-analysis"),
					"-collectProfilerData",
					"max",
					filepath.Join(outDir, "profile-full-analysis"),
					"-collectPerformanceTiming",
					"-collectRawCounters",
					filepath.Join(outDir, "profile-full-analysis"),
					"-collectDerivedCounters",
					"-collectPipelinePerformanceStatistics",
					"-gpuTimelineData",
					"-perfectPatching",
					"1",
					filepath.Join(outDir, "profile-full-analysis"),
					"--output",
					filepath.Join(outDir, "profile-full-analysis"),
					"--frame",
					"0",
					"-maxProfilingTime",
					"1",
					"-maxProfilingFrames",
					"1",
					"-waitUntilCompleteEachFrame",
				},
				privateReplayerTimeout,
			),
			runPrivateReplayerCommand(
				"raw_and_derived_counters",
				replayer,
				[]string{
					"-CLI",
					tracePath,
					"-profileTrace",
					"-collectRawCounters",
					"-collectDerivedCounters",
					"-collectPerformanceTiming",
					"-collectProfilerData",
					"-collectPipelinePerformanceStatistics",
					"-gpuTimelineData",
					"--output",
					filepath.Join(outDir, "raw-derived-counters"),
					"--frame",
					"0",
				},
				privateReplayerTimeout,
			),
		)
	}
	if privateReplayerDirect {
		results = append(results, runDirectFrameworkProbes(tracePath, framework, outDir, privateReplayerTimeout, privateReplayerDirectModes)...)
	}
	if privateReplayerSigned {
		results = append(results, runSignedReplayServiceProbes(tracePath, outDir, privateReplayerTimeout, privateReplayerSignedModes, privateReplayerTimingRowsJSON)...)
	}

	profilerDir := findProbeProfilerDir(outDir)
	streamData := filePresence{}
	var streamStats *streamDataProbeStats
	if profilerDir != "" {
		streamData = presence(filepath.Join(profilerDir, "streamData"))
		streamStats = summarizeEncodedStreamData(profilerDir)
	}
	output := privateReplayerProbeOutput{
		TracePath:         tracePath,
		ReplayerPath:      replayer,
		FrameworkPath:     framework,
		TimeoutSeconds:    privateReplayerTimeout.Seconds(),
		ResourcePreflight: resourcePreflight,
		ReplayCandidates:  replayCandidates,
		Commands:          results,
		ProfilerPayload:   presence(profilerDir),
		StreamData:        streamData,
		StreamDataStats:   streamStats,
		ServiceStreams:    summarizeServiceStreams(results),
	}
	output.TimingClaimsAllowed = output.ProfilerPayload.Present && output.StreamData.Present && streamStats != nil && streamStats.TimingUsable
	output.CounterClaimsAllowed = output.ProfilerPayload.Present && output.StreamData.Present && streamStats != nil && streamStats.CounterUsable
	output.Viable = output.TimingClaimsAllowed
	if output.Viable && streamStats.CounterUsable {
		output.Reason = "Apple-signed replay service produced streamData with usable timing rows and derived counter samples"
	} else if output.Viable {
		output.Reason = "Profiler probe produced a profiler payload with streamData and usable timing rows"
	} else if output.ProfilerPayload.Present && output.StreamData.Present {
		output.Reason = "Profiler probe produced streamData, but it did not contain usable timing/counter rows"
	} else if hasSignedReplayServiceLoad(results) {
		output.Reason = "Apple-signed GPUToolsReplayService launched headlessly and loaded the trace, but bounded profile requests emitted only service response archives and did not emit .gpuprofiler_raw/streamData"
	} else if hasRawCounterServiceFailure(results) {
		output.Reason = "GPUToolsReplay profiler path reached AGXGPURawCounterSourceGroup but failed to instantiate it; private raw-counter service or entitlement is unavailable to this headless helper"
	} else {
		output.Reason = "MTLReplayer/GPUToolsReplay did not produce .gpuprofiler_raw/streamData from bounded non-UI probes"
	}
	return &output, nil
}

func preflightPrivateReplayerResources(outDir string) error {
	if privateReplayerAllowLowResources {
		return nil
	}
	if privateReplayerMinFreeGiB > 0 {
		freeBytes, checkedPath, err := availableBytesForPath(outDir)
		if err != nil {
			return fmt.Errorf("resource preflight failed for output directory %s: %w", outDir, err)
		}
		freeGiB := float64(freeBytes) / (1024 * 1024 * 1024)
		if freeGiB < privateReplayerMinFreeGiB {
			return fmt.Errorf("refusing to launch replay tools: output volume at %s has %.1f GiB free, below %.1f GiB threshold (override with --allow-low-resources)", checkedPath, freeGiB, privateReplayerMinFreeGiB)
		}
	}
	if privateReplayerMinMemFree > 0 {
		freePercent, err := currentMemoryFreePercent()
		if err != nil {
			return fmt.Errorf("resource preflight failed reading memory pressure: %w", err)
		}
		if freePercent < privateReplayerMinMemFree {
			return fmt.Errorf("refusing to launch replay tools: memory_pressure free percentage is %d%%, below %d%% threshold (override with --allow-low-resources)", freePercent, privateReplayerMinMemFree)
		}
	}
	return nil
}

func collectPrivateReplayerResourcePreflight(outDir string) privateReplayerResourcePreflight {
	return collectResourcePreflight(outDir, privateReplayerMinFreeGiB, privateReplayerMinMemFree, privateReplayerAllowLowResources)
}

func collectResourcePreflight(outDir string, minFreeGiB float64, minMemFree int, allowLowResources bool) privateReplayerResourcePreflight {
	summary := privateReplayerResourcePreflight{
		OutputDir:         outDir,
		MinFreeGiB:        minFreeGiB,
		MinMemoryFree:     minMemFree,
		AllowLowResources: allowLowResources,
	}
	if freeBytes, checkedPath, err := availableBytesForPath(outDir); err == nil {
		summary.CheckedPath = checkedPath
		summary.FreeGiB = float64(freeBytes) / (1024 * 1024 * 1024)
	}
	if freePercent, err := currentMemoryFreePercent(); err == nil {
		summary.MemoryFreePercent = freePercent
	}
	return summary
}

func availableBytesForPath(path string) (uint64, string, error) {
	checkPath := path
	for {
		var stat syscall.Statfs_t
		if err := syscall.Statfs(checkPath, &stat); err == nil {
			return uint64(stat.Bavail) * uint64(stat.Bsize), checkPath, nil
		}
		parent := filepath.Dir(checkPath)
		if parent == checkPath {
			var stat syscall.Statfs_t
			if err := syscall.Statfs(parent, &stat); err != nil {
				return 0, parent, err
			}
			return uint64(stat.Bavail) * uint64(stat.Bsize), parent, nil
		}
		checkPath = parent
	}
}

func currentMemoryFreePercent() (int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	stdout, stderr, err := runCommandCapture(ctx, exec.CommandContext(ctx, "memory_pressure", "-Q"))
	if err != nil {
		return 0, fmt.Errorf("%w: %s", err, previewOutput(stderr))
	}
	freePercent, ok := parseMemoryPressureFreePercent(string(stdout))
	if !ok {
		return 0, fmt.Errorf("could not parse memory_pressure output: %s", previewOutput(stdout))
	}
	return freePercent, nil
}

func parseMemoryPressureFreePercent(output string) (int, bool) {
	const marker = "System-wide memory free percentage:"
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, marker) {
			continue
		}
		value := strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(strings.TrimPrefix(line, marker)), "%"))
		var percent int
		if _, err := fmt.Sscanf(value, "%d", &percent); err != nil {
			return 0, false
		}
		return percent, true
	}
	return 0, false
}

func summarizeEncodedStreamData(profilerDir string) *streamDataProbeStats {
	stats, err := counter.ParseStreamData(profilerDir)
	if err != nil {
		return &streamDataProbeStats{ParseError: err.Error()}
	}
	out := &streamDataProbeStats{
		NumEncoders:               stats.NumEncoders,
		NumGPUCommands:            stats.NumGPUCommands,
		NumPipelines:              stats.NumPipelines,
		DispatchCount:             len(stats.Dispatches),
		EncoderTimingCount:        len(stats.EncoderTimings),
		DerivedCounterSampleCount: stats.DerivedCounterSampleCount(),
		TotalTimeUs:               stats.TotalTimeUs,
	}
	out.TimingUsable = out.DispatchCount > 0 || out.EncoderTimingCount > 0 || out.TotalTimeUs > 0
	out.CounterUsable = out.DerivedCounterSampleCount > 0
	return out
}

func summarizeServiceStreams(results []privateReplayerResult) serviceStreamSummary {
	summary := serviceStreamSummary{}
	for _, result := range results {
		for _, path := range result.ProfilerFiles {
			if filepath.Base(path) == "streamData" {
				summary.StreamDataFileCount++
				if stats := summarizeEncodedStreamData(filepath.Dir(path)); stats != nil && (stats.TimingUsable || stats.CounterUsable) {
					summary.UsableStreamDataFileCount++
				}
			}
		}
		for _, payload := range result.ServicePayloads {
			if strings.HasPrefix(payload.Kind, "response") || payload.Kind == "nskeyed_archive" {
				summary.ResponseCount++
			}
			switch payload.Kind {
			case "timeline_marker", "batch_filter_marker":
				summary.MarkerPayloadCount++
			case "derived_counter_summary":
				summary.DerivedCounterRows += payload.AverageSampleRowCount
				if payload.AverageSampleRowCount > 0 {
					summary.NonMarkerDataPayloadCount++
				}
			case "response_data_nested_archive":
				if payloadHasOnlyMarkerNestedData(payload) {
					summary.NestedMarkerPayloadCount++
				} else {
					summary.NonMarkerDataPayloadCount++
				}
			default:
				if len(payload.ResponseDataPayloads) > 0 || len(payload.DataValueBytes) > 0 {
					summary.NonMarkerDataPayloadCount++
				}
			}
		}
	}
	return summary
}

func payloadHasOnlyMarkerNestedData(payload replayServicePayloadSummary) bool {
	if len(payload.ResponseDataPayloads) == 0 {
		return false
	}
	for _, nested := range payload.ResponseDataPayloads {
		if nested.Kind != "nskeyed_dictionary" || len(nested.Keys) != 1 {
			return false
		}
		key := nested.Keys[0]
		if key != "Streaming APS Data" && key != "Batch Filtering Started" {
			return false
		}
	}
	return true
}

func hasRawCounterServiceFailure(results []privateReplayerResult) bool {
	for _, result := range results {
		if strings.Contains(result.StdoutPreview, "AGXGPURawCounterSourceGroup") ||
			strings.Contains(result.StderrPreview, "AGXGPURawCounterSourceGroup") ||
			strings.Contains(result.StdoutPreview, "GPURawCounterErrorDomain") ||
			strings.Contains(result.StderrPreview, "GPURawCounterErrorDomain") {
			return true
		}
	}
	return false
}

func hasSignedReplayServiceLoad(results []privateReplayerResult) bool {
	for _, result := range results {
		if strings.Contains(result.StdoutPreview, "load ok=1") ||
			strings.Contains(result.StderrPreview, "load ok=1") {
			return true
		}
	}
	return false
}

func printPrivateReplayerOutput(output *privateReplayerProbeOutput) {
	fmt.Printf("MTLReplayer: %s\n", output.ReplayerPath)
	fmt.Printf("Framework:   %s\n", output.FrameworkPath)
	fmt.Printf("Trace:       %s\n", output.TracePath)
	if output.ResourcePreflight.CheckedPath != "" || output.ResourcePreflight.MemoryFreePercent != 0 {
		fmt.Printf("Preflight:   output=%s free=%.1fGiB memory_free=%d%%\n",
			output.ResourcePreflight.OutputDir,
			output.ResourcePreflight.FreeGiB,
			output.ResourcePreflight.MemoryFreePercent,
		)
	}
	for _, result := range output.Commands {
		fmt.Printf("%-24s elapsed=%dms timed_out=%v exit=%d signal=%s files=%d\n",
			result.Name,
			result.ElapsedMillis,
			result.TimedOut,
			result.ExitCode,
			result.Signal,
			result.FileCount,
		)
	}
	fmt.Printf("Profiler payload: %v\n", output.ProfilerPayload.Present)
	fmt.Printf("streamData:       %v\n", output.StreamData.Present)
	fmt.Printf("timing claims:    %v\n", output.TimingClaimsAllowed)
	fmt.Printf("counter claims:   %v\n", output.CounterClaimsAllowed)
	fmt.Printf("service streams:  responses=%d marker=%d nested_marker=%d non_marker=%d streamData=%d usable=%d\n",
		output.ServiceStreams.ResponseCount,
		output.ServiceStreams.MarkerPayloadCount,
		output.ServiceStreams.NestedMarkerPayloadCount,
		output.ServiceStreams.NonMarkerDataPayloadCount,
		output.ServiceStreams.StreamDataFileCount,
		output.ServiceStreams.UsableStreamDataFileCount,
	)
	fmt.Printf("Viable:           %v\n", output.Viable)
	fmt.Println(output.Reason)
}

func runDirectFrameworkProbes(tracePath, framework, outDir string, timeout time.Duration, selectedModes []string) []privateReplayerResult {
	helperDir := filepath.Join(outDir, "direct-framework-helper")
	_ = os.MkdirAll(helperDir, 0o755)
	source := filepath.Join(helperDir, "gtmtlreplay_probe_helper.c")
	binary := filepath.Join(helperDir, "gtmtlreplay_probe_helper")
	if err := os.WriteFile(source, []byte(gtmtlReplayProbeHelperSource), 0o644); err != nil {
		return []privateReplayerResult{{
			Name:   "direct_framework_write_helper",
			Signal: err.Error(),
		}}
	}
	compile := runExternalCommand(
		"direct_framework_compile_helper",
		[]string{privateReplayerClang, "-Wall", "-Wextra", "-O0", "-g", "-o", binary, source, "-ldl"},
		"",
		timeout,
	)
	results := []privateReplayerResult{compile}
	if compile.Signal != "" || compile.ExitCode != 0 || compile.TimedOut {
		return results
	}
	directModes := []string{
		"main_like",
		"options_null_callback",
		"options_dummy_callback",
		"fake_apr_timing",
		"fake_apr_profiler",
		"fake_apr_real_offsets_timing",
		"fake_apr_real_offsets_profiler",
		"fake_apr_real_offsets_raw_derived",
		"fake_apr_profile_dictionary",
		"fake_apr_test_profiling",
		"fake_apr_wait_complete_profiler",
	}
	if len(expandModeFilters(selectedModes)) > 0 {
		directModes = append(directModes,
			"main_like_full_analysis",
			"fake_apr_actual_flags_profiler",
			"fake_apr_actual_flags_analysis",
		)
		directModes = append(directModes, expandModeFilters(selectedModes)...)
	}
	modes := filterModes(directModes, selectedModes)
	for _, mode := range modes {
		modeOut := filepath.Join(outDir, "direct-framework-"+mode)
		_ = os.MkdirAll(modeOut, 0o755)
		results = append(results, runExternalCommand(
			"direct_framework_"+mode,
			[]string{binary, mode, framework, tracePath, modeOut},
			modeOut,
			timeout,
		))
	}
	return results
}

func runSignedReplayServiceProbes(tracePath, outDir string, timeout time.Duration, selectedModes []string, timingRowsJSON string) []privateReplayerResult {
	helperDir := filepath.Join(outDir, "signed-replay-service-helper")
	_ = os.MkdirAll(helperDir, 0o755)
	source := filepath.Join(helperDir, "gt_replay_service_probe_helper.m")
	binary := filepath.Join(helperDir, "gt_replay_service_probe_helper")
	if err := os.WriteFile(source, []byte(gtReplayServiceProbeHelperSource), 0o644); err != nil {
		return []privateReplayerResult{{
			Name:   "signed_service_write_helper",
			Signal: err.Error(),
		}}
	}
	compile := runExternalCommand(
		"signed_service_compile_helper",
		[]string{privateReplayerClang, "-fobjc-arc", "-fblocks", "-Wall", "-Wextra", "-O0", "-g", "-framework", "Foundation", "-framework", "Metal", "-o", binary, source},
		"",
		timeout,
	)
	results := []privateReplayerResult{compile}
	if compile.Signal != "" || compile.ExitCode != 0 || compile.TimedOut {
		return results
	}
	modes := []string{
		"runtime_dump",
		"runtime_class_prefix_dump",
		"runtime_class_profile_substring_dump",
		"xcode_shaderprofiler_runtime_dump",
		"xcode_capture_archive_inspect",
		"xcode_shaderprofiler_archive_payload_encode_streamdata",
		"xcode_shaderprofiler_archive_gather_encode_streamdata",
		"xcode_shaderprofiler_archive_gather_device_encode_streamdata",
		"xcode_shaderprofiler_archive_gather_archiveinfo_encode_streamdata",
		"xcode_shaderprofiler_archive_gather_deviceprofile_encode_streamdata",
		"xcode_shaderprofiler_archive_gather_deviceproxy_encode_streamdata",
		"xcode_shaderprofiler_archive_construct_encode_streamdata",
		"xcode_shaderprofiler_archive_construct_base_encode_streamdata",
		"xcode_shaderprofiler_archive_construct_deviceproxy_encode_streamdata",
		"xcode_shaderprofiler_trace_data_encode_streamdata",
		"xcode_shaderprofiler_trace_data_base_encode_streamdata",
		"xcode_shaderprofiler_result_shell_encode_streamdata",
		"xcode_shaderprofiler_result_shell_data_encode_streamdata",
		"trace_timing_rows_encode_streamdata",
		"trace_timing_rows_plus_derived_counters_encode_streamdata",
		"profile_constant_dump",
		"timeline_raw_no_query",
		"timeline_raw_wait_complete",
		"timeline_raw_empty_profile_data",
		"timeline_raw_v2_profile_data",
		"timeline_raw_v2_uppercase_profile_data",
		"timeline_raw_profile_version_0",
		"timeline_raw_profile_version_2",
		"timeline_raw_profile_version_3",
		"timeline_raw_profile_version_5",
		"timeline_raw_priority_1",
		"timeline_aps_options_all",
		"timeline_aps_options_streaming_profile",
		"timeline_aps_options_streaming_counters",
		"timeline_aps_trace_data",
		"timeline_aps_usc_timeline_config",
		"timeline_aps_usc_timeline_wait_complete",
		"timeline_aps_usc_counters_config",
		"timeline_aps_usc_profiling_config",
		"timeline_aps_usc_timeline_determination_encode_streamdata",
		"timeline_aps_usc_profiling_determination_encode_streamdata",
		"timeline_encode_streamdata_launch_cli_args_run_30s",
		"timeline_encode_streamdata_launch_min_env_run_30s",
		"timeline_encode_streamdata_observer_run_30s",
		"timeline_encode_streamdata_launch_cli_args_observer_run_30s",
		"timeline_aps_usc_profiling_determination_encode_streamdata_launch_cli_args_run_30s",
		"timeline_profiled_profiler_mode_0",
		"timeline_profiled_profiler_mode_1",
		"timeline_profiled_profiler_mode_2",
		"timeline_encode_streamdata",
		"timeline_encode_streamdata_resume_run_30s",
		"timeline_encode_streamdata_pause_resume_run_30s",
		"timeline_encode_streamdata_proxy_resume_run_30s",
		"timeline_encode_streamdata_proxy_pause_resume_run_30s",
		"timeline_encode_streamdata_proxy_resume_run_5s",
		"timeline_encode_streamdata_proxy_pause_resume_run_5s",
		"timeline_encode_streamdata_display_on_run_5s",
		"timeline_encode_streamdata_display_on_observer_run_5s",
		"timeline_direct_profiledata_profiler_raw_url_display_on_run_5s",
		"service_introspection_profile_encode_streamdata_run_5s",
		"profile_bulk_download_candidates_encode_streamdata",
		"profile_bulk_download_wait_5s_encode_streamdata",
		"profile_bulk_download_proxy_resume_wait_5s_encode_streamdata",
		"profile_bulk_download_wait_complete_encode_streamdata",
		"timeline_encode_streamdata_flush_rpackets_run_30s",
		"timeline_encode_streamdata_query_perf_during_run_30s",
		"display_request_candidates",
		"profile_during_display_request_candidates_encode_streamdata",
		"timeline_encode_streamdata_perf_state_3_run_30s",
		"timeline_encode_streamdata_perf_state_5_run_30s",
		"timeline_encode_streamdata_perf_state_8_run_30s",
		"timeline_analysis_flags_encode_streamdata_run_30s",
		"timeline_analysis_flags_encode_streamdata_perf_state_3_run_30s",
		"timeline_profiler_raw_url",
		"timeline_profiler_raw_url_no_stream_run_30s",
		"timeline_profiler_raw_url_encode_streamdata",
		"timeline_encode_streamdata_wait_complete",
		"timeline_gputimeline_encode_streamdata",
		"timeline_shaderprofiler_encode_streamdata",
		"timeline_session_mode_0_encode_streamdata",
		"timeline_session_mode_1_encode_streamdata",
		"timeline_session_mode_2_encode_streamdata",
		"timeline_session_mode_2_streamdata_to_load_encode_streamdata",
		"timeline_session_mode_2_streamdata_to_load_encode_streamdata_resume_run_30s",
		"timeline_session_mode_2_profiler_raw_url_encode_streamdata_wait_complete",
		"timeline_session_fallback_profiler_raw_url_encode_streamdata_wait_complete",
		"timeline_session_mode_2_profiler_raw_url_no_stream_run_5s",
		"timeline_session_fallback_profiler_raw_url_no_stream_run_5s",
		"timeline_direct_profiledata_profiler_raw_url_no_stream_run_5s",
		"timeline_direct_profiledata_profiler_raw_url_encode_streamdata_run_5s",
		"timeline_no_shader_raw",
		"query_then_timeline_raw",
		"query_device_capabilities",
		"query_configuration",
		"query_derived_counters",
		"query_derived_counters_encode_streamdata",
		"query_performance_state",
		"query_session_info",
		"update_config_then_timeline_raw",
		"update_config_then_timeline_encode_streamdata",
		"update_config_then_query_performance_state",
		"update_config_then_timeline_encode_streamdata_perf_state_3_run_30s",
		"update_config_then_timeline_encode_streamdata_perf_state_5_run_30s",
		"update_config_then_timeline_encode_streamdata_perf_state_8_run_30s",
		"update_config_display_on_then_query_performance_state",
		"update_config_display_on_then_timeline_encode_streamdata_perf_state_3_run_30s",
		"update_config_then_derived_counters",
		"update_config_then_derived_counters_encode_streamdata",
		"update_config_then_query_derived_counters_encode_streamdata",
		"derived_counters",
		"derived_counters_encode_streamdata",
		"batch_filtered_counters",
	}
	resourceUsageCandidates := collectResourceUsageCandidates(tracePath)
	fetchPipelineCandidates := collectFetchPipelineCandidates(tracePath)
	updateLibraryCandidates := collectUpdateLibraryCandidates(tracePath)
	fetchTextureCandidates := collectFetchTextureCandidates(tracePath)
	fetchBufferCandidates := collectFetchBufferCandidates(tracePath)
	if resourceUsageCandidates != "" {
		modes = append(modes, "query_resource_usage_candidates")
		modes = append(modes, "profile_then_resource_usage_candidates")
		modes = append(modes, "fetch_threadgroup_candidates")
		modes = append(modes, "fetch_post_vertex_candidates")
		modes = append(modes, "fetch_wireframe_candidates")
		modes = append(modes, "profile_during_fetch_threadgroup_candidates_encode_streamdata")
		modes = append(modes, "profile_during_fetch_post_vertex_candidates_encode_streamdata")
		modes = append(modes, "profile_during_fetch_wireframe_candidates_encode_streamdata")
		modes = append(modes, "profile_during_fetch_threadgroup_ingest_fetch_payloads_encode_streamdata")
		modes = append(modes, "profile_during_fetch_wireframe_ingest_fetch_payloads_encode_streamdata")
		modes = append(modes, "shaderdebug_kernel_candidates")
		modes = append(modes, "profile_during_shaderdebug_kernel_candidates_encode_streamdata")
	} else {
		modes = append(modes, "query_resource_usage_0")
	}
	if fetchPipelineCandidates != "" || fetchTextureCandidates != "" {
		modes = append(modes, "fetch_pipeline_binary_candidates")
		modes = append(modes, "fetch_texture_candidates")
		modes = append(modes, "fetch_into_texture_candidates")
		modes = append(modes, "fetch_into_texture_then_timeline_encode_streamdata")
		modes = append(modes, "fetch_into_texture_wait_complete_then_timeline_encode_streamdata_wait_complete")
		modes = append(modes, "profile_during_fetch_into_texture_encode_streamdata")
		modes = append(modes, "profile_during_fetch_into_texture_wait_complete_encode_streamdata")
		modes = append(modes, "profile_during_fetch_into_texture_session_request_wait_complete_encode_streamdata")
		modes = append(modes, "update_config_then_profile_during_fetch_into_texture_wait_complete_encode_streamdata")
		modes = append(modes, "update_config_display_on_then_profile_during_fetch_into_texture_wait_complete_encode_streamdata")
		modes = append(modes, "profile_during_fetch_texture_wait_complete_encode_streamdata")
		modes = append(modes, "query_raster_map_candidates")
		modes = append(modes, "decode_generic_acceleration_structure_candidates")
		modes = append(modes, "decode_ab_candidates")
		modes = append(modes, "decode_icb_candidates")
		modes = append(modes, "profile_during_decode_ab_candidates_encode_streamdata")
		modes = append(modes, "profile_during_decode_icb_candidates_encode_streamdata")
		modes = append(modes, "update_library_candidates")
		modes = append(modes, "profile_during_update_library_candidates_encode_streamdata")
		modes = append(modes, "raytrace_candidates")
	}
	if fetchBufferCandidates != "" {
		modes = append(modes,
			"fetch_buffer_candidates",
			"profile_during_fetch_buffer_wait_complete_encode_streamdata",
			"profile_during_fetch_buffer_ingest_fetch_payloads_encode_streamdata",
		)
	}
	if len(expandModeFilters(selectedModes)) > 0 {
		modes = append(modes,
			"query_raster_map_candidates",
			"decode_generic_acceleration_structure_candidates",
			"raytrace_candidates",
		)
	}
	modes = append(modes, "profile_all_runtime_classes")
	modes = append(modes, "profile_base_priority_1_nil_data")
	modes = append(modes, "profile_base_priority_1_v2_data")
	modes = append(modes, "batch_filtered_counters_encode_streamdata")
	modes = append(modes, "batch_filtered_counters_encode_streamdata_wait_complete")
	modes = append(modes, "batch_filtered_aps_usc_counters_encode_streamdata_wait_complete")
	modes = append(modes, "derived_aps_usc_counters_encode_streamdata_wait_complete")
	modes = append(modes, "datasource_ready_then_query_derived_counters_encode_streamdata")
	modes = append(modes, "timeline_encode_streamdata_run_30s")
	modes = append(modes, "timeline_aps_usc_timeline_encode_streamdata_run_30s")
	modes = append(modes, expandModeFilters(selectedModes)...)
	modes = filterModes(modes, selectedModes)
	for _, mode := range modes {
		modeOut := filepath.Join(outDir, "signed-service-"+mode)
		_ = os.MkdirAll(modeOut, 0o755)
		argv := []string{binary, mode, tracePath, modeOut}
		if mode == "query_resource_usage_candidates" {
			argv = append(argv, resourceUsageCandidates)
		}
		if mode == "profile_then_resource_usage_candidates" {
			argv = append(argv, resourceUsageCandidates)
		}
		if mode == "fetch_threadgroup_candidates" {
			argv = append(argv, resourceUsageCandidates)
		}
		if mode == "fetch_post_vertex_candidates" {
			argv = append(argv, resourceUsageCandidates)
		}
		if mode == "fetch_wireframe_candidates" {
			argv = append(argv, resourceUsageCandidates)
		}
		if mode == "profile_during_fetch_threadgroup_candidates_encode_streamdata" {
			argv = append(argv, resourceUsageCandidates)
		}
		if mode == "profile_during_fetch_post_vertex_candidates_encode_streamdata" {
			argv = append(argv, resourceUsageCandidates)
		}
		if mode == "profile_during_fetch_wireframe_candidates_encode_streamdata" {
			argv = append(argv, resourceUsageCandidates)
		}
		if mode == "profile_during_fetch_threadgroup_ingest_fetch_payloads_encode_streamdata" {
			argv = append(argv, resourceUsageCandidates)
		}
		if mode == "profile_during_fetch_wireframe_ingest_fetch_payloads_encode_streamdata" {
			argv = append(argv, resourceUsageCandidates)
		}
		if mode == "shaderdebug_kernel_candidates" {
			argv = append(argv, resourceUsageCandidates)
		}
		if mode == "profile_during_shaderdebug_kernel_candidates_encode_streamdata" {
			argv = append(argv, resourceUsageCandidates, "0.5")
		}
		if mode == "fetch_pipeline_binary_candidates" {
			argv = append(argv, "", fetchPipelineCandidates)
		}
		if mode == "fetch_texture_candidates" {
			argv = append(argv, "", fetchPipelineCandidatesOrSentinel(firstNonEmpty(fetchTextureCandidates, fetchPipelineCandidates)))
		}
		if mode == "fetch_into_texture_candidates" {
			argv = append(argv, "", fetchPipelineCandidatesOrSentinel(firstNonEmpty(fetchTextureCandidates, fetchPipelineCandidates)))
		}
		if mode == "fetch_into_texture_then_timeline_encode_streamdata" {
			argv = append(argv, "", fetchPipelineCandidatesOrSentinel(firstNonEmpty(fetchTextureCandidates, fetchPipelineCandidates)))
		}
		if mode == "fetch_into_texture_wait_complete_then_timeline_encode_streamdata_wait_complete" {
			argv = append(argv, "", fetchPipelineCandidatesOrSentinel(firstNonEmpty(fetchTextureCandidates, fetchPipelineCandidates)))
		}
		if mode == "profile_during_fetch_into_texture_encode_streamdata" {
			argv = append(argv, "", fetchPipelineCandidatesOrSentinel(firstNonEmpty(fetchTextureCandidates, fetchPipelineCandidates)))
		}
		if mode == "profile_during_fetch_into_texture_wait_complete_encode_streamdata" {
			argv = append(argv, "", fetchPipelineCandidatesOrSentinel(firstNonEmpty(fetchTextureCandidates, fetchPipelineCandidates)))
		}
		if mode == "profile_during_fetch_into_texture_session_request_wait_complete_encode_streamdata" {
			argv = append(argv, "", fetchPipelineCandidatesOrSentinel(firstNonEmpty(fetchTextureCandidates, fetchPipelineCandidates)))
		}
		if mode == "update_config_then_profile_during_fetch_into_texture_wait_complete_encode_streamdata" {
			argv = append(argv, "", fetchPipelineCandidatesOrSentinel(firstNonEmpty(fetchTextureCandidates, fetchPipelineCandidates)))
		}
		if mode == "update_config_display_on_then_profile_during_fetch_into_texture_wait_complete_encode_streamdata" {
			argv = append(argv, "", fetchPipelineCandidatesOrSentinel(firstNonEmpty(fetchTextureCandidates, fetchPipelineCandidates)))
		}
		if mode == "profile_during_fetch_texture_wait_complete_encode_streamdata" {
			argv = append(argv, "", fetchPipelineCandidatesOrSentinel(firstNonEmpty(fetchTextureCandidates, fetchPipelineCandidates)))
		}
		if mode == "fetch_buffer_candidates" {
			argv = append(argv, "", fetchPipelineCandidatesOrSentinel(fetchBufferCandidates))
		}
		if mode == "profile_during_fetch_buffer_wait_complete_encode_streamdata" {
			argv = append(argv, "", fetchPipelineCandidatesOrSentinel(fetchBufferCandidates))
		}
		if mode == "profile_during_fetch_buffer_ingest_fetch_payloads_encode_streamdata" {
			argv = append(argv, "", fetchPipelineCandidatesOrSentinel(fetchBufferCandidates))
		}
		if mode == "query_raster_map_candidates" {
			argv = append(argv, "", fetchPipelineCandidatesOrSentinel(fetchPipelineCandidates))
		}
		if mode == "decode_generic_acceleration_structure_candidates" {
			argv = append(argv, "", fetchPipelineCandidatesOrSentinel(fetchPipelineCandidates))
		}
		if mode == "decode_ab_candidates" {
			argv = append(argv, "", fetchPipelineCandidatesOrSentinel(fetchPipelineCandidates))
		}
		if mode == "decode_icb_candidates" {
			argv = append(argv, "", fetchPipelineCandidatesOrSentinel(fetchPipelineCandidates))
		}
		if mode == "profile_during_decode_ab_candidates_encode_streamdata" {
			argv = append(argv, "", fetchPipelineCandidatesOrSentinel(fetchPipelineCandidates))
		}
		if mode == "profile_during_decode_icb_candidates_encode_streamdata" {
			argv = append(argv, "", fetchPipelineCandidatesOrSentinel(fetchPipelineCandidates))
		}
		if mode == "update_library_candidates" {
			argv = append(argv, "", fetchPipelineCandidatesOrSentinel(firstNonEmpty(updateLibraryCandidates, fetchPipelineCandidates)))
		}
		if mode == "profile_during_update_library_candidates_encode_streamdata" {
			argv = append(argv, "", fetchPipelineCandidatesOrSentinel(firstNonEmpty(updateLibraryCandidates, fetchPipelineCandidates)))
		}
		if mode == "raytrace_candidates" {
			argv = append(argv, "", fetchPipelineCandidatesOrSentinel(fetchPipelineCandidates))
		}
		if mode == "trace_timing_rows_encode_streamdata" || mode == "trace_timing_rows_plus_derived_counters_encode_streamdata" {
			rowsPath := filepath.Join(modeOut, "trace-timing-rows.json")
			if timingRowsJSON != "" {
				if err := copyTimingRowsJSON(timingRowsJSON, rowsPath); err == nil {
					argv = append(argv, rowsPath)
				}
			} else if err := writeTraceTimingRowsJSON(tracePath, rowsPath); err == nil {
				argv = append(argv, rowsPath)
			}
		}
		result := runExternalCommand(
			"signed_service_"+mode,
			argv,
			modeOut,
			timeout,
		)
		cleanupSignedReplayServiceProcesses()
		results = append(results, result)
	}
	return results
}

func cleanupSignedReplayServiceProcesses() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, _, _ = runCommandCapture(ctx, exec.CommandContext(ctx, "pkill", "-TERM", "-f", "GPUToolsReplayService.xpc"))
	_, _, _ = runCommandCapture(ctx, exec.CommandContext(ctx, "pkill", "-TERM", "-f", "/System/Library/CoreServices/MTLReplayer.app/Contents/MacOS/MTLReplayer"))
}

func writeTraceTimingRowsJSON(tracePath, rowsPath string) error {
	traceFile, err := gputrace.Open(tracePath)
	if err != nil {
		return err
	}
	extractor := gputrace.NewTimingMetricsExtractor(traceFile)
	metrics, err := extractor.Extract()
	if err != nil {
		return err
	}
	rows := make([]xctraceIntervalRow, 0, len(metrics.EncoderTimings))
	var cumulativeNs uint64
	for i, timing := range metrics.EncoderTimings {
		if timing == nil || timing.DurationNs == 0 {
			continue
		}
		startNs := timing.StartTimestamp
		if startNs == 0 {
			startNs = cumulativeNs
		}
		label := timing.Label
		if timing.KernelName != "" {
			label = timing.KernelName
		}
		rows = append(rows, xctraceIntervalRow{
			StartNs:         startNs,
			DurationNs:      timing.DurationNs,
			Process:         "gputrace",
			Label:           label,
			CommandBufferID: 1,
			EncoderID:       uint64(i + 1),
		})
		cumulativeNs = startNs + timing.DurationNs
	}
	if len(rows) == 0 {
		return fmt.Errorf("no trace timing rows available")
	}
	data, err := json.Marshal(rows)
	if err != nil {
		return err
	}
	return os.WriteFile(rowsPath, data, 0o644)
}

func copyTimingRowsJSON(srcPath, dstPath string) error {
	data, err := os.ReadFile(srcPath)
	if err != nil {
		return err
	}
	var rows []xctraceIntervalRow
	if err := json.Unmarshal(data, &rows); err != nil {
		return err
	}
	if len(rows) == 0 {
		return fmt.Errorf("timing rows JSON is empty")
	}
	for i, row := range rows {
		if row.StartNs == 0 || row.DurationNs == 0 || row.CommandBufferID == 0 || row.EncoderID == 0 {
			return fmt.Errorf("timing row %d lacks start/duration/command-buffer/encoder IDs", i)
		}
	}
	return os.WriteFile(dstPath, data, 0o644)
}

func collectReplayActivationCandidates(tracePath string) replayActivationCandidates {
	candidates := replayActivationCandidates{
		ResourceUsage: collectResourceUsageCandidates(tracePath),
		FetchPipeline: collectFetchPipelineCandidates(tracePath),
		UpdateLibrary: collectUpdateLibraryCandidates(tracePath),
		FetchTexture:  collectFetchTextureCandidates(tracePath),
		FetchBuffer:   collectFetchBufferCandidates(tracePath),
	}
	candidates.AnyConcrete = hasConcreteReplayCandidate(candidates.FetchPipeline) ||
		hasConcreteReplayCandidate(candidates.UpdateLibrary) ||
		hasConcreteReplayCandidate(candidates.FetchTexture) ||
		hasConcreteReplayCandidate(candidates.FetchBuffer)
	return candidates
}

func hasConcreteReplayCandidate(candidates string) bool {
	for _, part := range strings.Split(candidates, ",") {
		fields := strings.Split(strings.TrimSpace(part), ":")
		if len(fields) != 4 {
			continue
		}
		streamRef, err := strconv.ParseUint(fields[3], 10, 64)
		if err == nil && streamRef != 0 {
			return true
		}
	}
	return false
}

func collectResourceUsageCandidates(tracePath string) string {
	seen := map[uint64]bool{}
	candidates := []uint64{}
	add := func(value uint64) {
		if !seen[value] {
			seen[value] = true
			candidates = append(candidates, value)
		}
	}
	traceFile, err := gputracetrace.Open(tracePath)
	if err != nil {
		return "0"
	}
	if records, err := traceFile.ParseMTSPRecords(); err == nil {
		for _, record := range records {
			if dispatch, err := record.ParseDispatchRecord(); err == nil {
				add(dispatch.EncoderID)
			}
			if ct, err := record.ParseCtRecord(); err == nil {
				add(ct.PipelineAddr)
				add(ct.FunctionAddr)
			}
			if record.Address != 0 {
				add(record.Address)
			}
		}
	}
	if dispatches, err := traceFile.ParseDispatchCalls(); err == nil {
		for _, dispatch := range dispatches {
			add(uint64(dispatch.Offset))
		}
	}
	if cbs, err := traceFile.ParseCommandBuffers(); err == nil {
		for _, cb := range cbs {
			add(uint64(cb.Offset))
		}
	}
	add(0)
	add(1)
	add(2)
	add(3)
	tokens := []string{
		"0:-1:0",
		"1:-1:1",
		"2:-1:2",
		"3:-1:3",
	}
	if len(candidates) > 6 {
		candidates = candidates[:6]
	}
	for _, candidate := range candidates {
		tokens = append(tokens,
			fmt.Sprintf("0:-1:%d", candidate),
			fmt.Sprintf("%d:-1:%d", int32(candidate), candidate),
		)
	}
	return strings.Join(uniqueStrings(tokens), ",")
}

func collectFetchPipelineCandidates(tracePath string) string {
	traceFile, err := gputracetrace.Open(tracePath)
	if err != nil {
		return ""
	}
	streamRefs := []uint64{}
	seen := map[uint64]bool{}
	if records, err := traceFile.ParseMTSPRecords(); err == nil {
		for _, record := range records {
			if ct, err := record.ParseCtRecord(); err == nil {
				for _, value := range []uint64{ct.PipelineAddr, ct.FunctionAddr, record.Address} {
					if value != 0 && !seen[value] {
						seen[value] = true
						streamRefs = append(streamRefs, value)
					}
				}
			}
		}
	}
	if len(streamRefs) == 0 {
		if entries, err := os.ReadDir(tracePath); err == nil {
			for _, entry := range entries {
				if entry.IsDir() {
					continue
				}
				name := entry.Name()
				if len(name) != 16 {
					continue
				}
				value, err := strconv.ParseUint(name, 16, 64)
				if err != nil || value == 0 || seen[value] {
					continue
				}
				seen[value] = true
				streamRefs = append(streamRefs, value)
				if len(streamRefs) >= 4 {
					break
				}
			}
		}
	}
	if len(streamRefs) == 0 {
		return ""
	}
	if len(streamRefs) > 4 {
		streamRefs = streamRefs[:4]
	}
	tokens := []string{}
	for _, streamRef := range streamRefs {
		tokens = append(tokens,
			fmt.Sprintf("0:-1:0:%d", streamRef),
			fmt.Sprintf("0:-1:%d:%d", streamRef, streamRef),
		)
	}
	return strings.Join(uniqueStrings(tokens), ",")
}

func collectUpdateLibraryCandidates(tracePath string) string {
	entries, err := os.ReadDir(tracePath)
	if err != nil {
		return ""
	}
	seen := map[uint64]bool{}
	values := []uint64{}
	add := func(value uint64) {
		if value < 0x100000000 || value > 0x0000ffffffffffff {
			return
		}
		if value == 0 || seen[value] || len(values) >= 4 {
			return
		}
		seen[value] = true
		values = append(values, value)
	}
	for _, entry := range entries {
		if entry.IsDir() || len(values) >= 4 {
			continue
		}
		name := entry.Name()
		if !strings.HasPrefix(name, "device-resources-") &&
			!strings.HasPrefix(name, "delta-device-resources-") &&
			!strings.HasPrefix(name, "unused-device-resources-") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(tracePath, name))
		if err != nil {
			continue
		}
		for _, marker := range [][]byte{[]byte("function\x00"), []byte("functions\x00"), []byte("library\x00")} {
			searchFrom := 0
			for len(values) < 4 {
				pos := bytes.Index(data[searchFrom:], marker)
				if pos < 0 {
					break
				}
				start := searchFrom + pos
				for _, offset := range []int{12, 16, 20} {
					if start+offset+8 <= len(data) {
						add(binary.LittleEndian.Uint64(data[start+offset : start+offset+8]))
					}
				}
				searchFrom = start + len(marker)
			}
			if len(values) >= 4 {
				break
			}
		}
		if len(values) >= 4 {
			break
		}
		traceFile := &gputracetrace.Trace{}
		records, err := traceFile.ParseMTSPFromData(data)
		if err != nil {
			continue
		}
		for _, record := range records {
			label := strings.ToLower(record.Label)
			if label == "function" ||
				label == "functions" ||
				label == "library" ||
				strings.Contains(label, "pipeline") {
				add(record.Address)
				add(record.SecondaryAddr)
			}
			if len(values) >= 4 {
				break
			}
		}
	}
	if len(values) == 0 {
		return ""
	}
	tokens := []string{}
	for _, value := range values {
		tokens = append(tokens,
			fmt.Sprintf("0:-1:0:%d", value),
			fmt.Sprintf("0:-1:%d:%d", value, value),
		)
	}
	return strings.Join(uniqueStrings(tokens), ",")
}

func collectFetchTextureCandidates(tracePath string) string {
	entries, err := os.ReadDir(tracePath)
	if err != nil {
		return ""
	}
	seen := map[uint64]bool{}
	tokens := []string{}
	add := func(value uint64) {
		if value == 0 || seen[value] || len(tokens) >= 4 {
			return
		}
		seen[value] = true
		tokens = append(tokens, fmt.Sprintf("0:-1:0:%d", value))
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.HasPrefix(name, "device-resources-") ||
			strings.HasPrefix(name, "delta-device-resources-") ||
			strings.HasPrefix(name, "unused-device-resources-") {
			data, err := os.ReadFile(filepath.Join(tracePath, name))
			if err == nil {
				traceFile := &gputracetrace.Trace{}
				if records, err := traceFile.ParseMTSPFromData(data); err == nil {
					for _, record := range records {
						label := strings.ToLower(record.Label)
						if strings.Contains(label, "texture") {
							add(record.Address)
							add(record.SecondaryAddr)
						}
					}
				}
			}
		}
		if len(tokens) >= 4 {
			break
		}
	}
	for _, entry := range entries {
		if entry.IsDir() || len(tokens) >= 4 {
			continue
		}
		name := entry.Name()
		if len(name) != 16 {
			continue
		}
		value, err := strconv.ParseUint(name, 16, 64)
		if err != nil || value == 0 {
			continue
		}
		add(value)
	}
	return strings.Join(uniqueStrings(tokens), ",")
}

func collectFetchBufferCandidates(tracePath string) string {
	entries, err := os.ReadDir(tracePath)
	if err != nil {
		return ""
	}
	seen := map[uint64]bool{}
	tokens := []string{}
	add := func(value uint64) {
		if value == 0 || seen[value] || len(tokens) >= 4 {
			return
		}
		seen[value] = true
		tokens = append(tokens, fmt.Sprintf("0:-1:0:%d", value))
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.HasPrefix(name, "device-resources-") ||
			strings.HasPrefix(name, "delta-device-resources-") ||
			strings.HasPrefix(name, "unused-device-resources-") {
			data, err := os.ReadFile(filepath.Join(tracePath, name))
			if err == nil {
				traceFile := &gputracetrace.Trace{}
				if records, err := traceFile.ParseMTSPFromData(data); err == nil {
					for _, record := range records {
						label := strings.ToLower(record.Label)
						if strings.Contains(label, "buffer") {
							add(record.Address)
							add(record.SecondaryAddr)
						}
					}
				}
			}
		}
		if len(tokens) >= 4 {
			break
		}
	}
	return strings.Join(uniqueStrings(tokens), ",")
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func fetchPipelineCandidatesOrSentinel(candidates string) string {
	if strings.TrimSpace(candidates) != "" {
		return candidates
	}
	return "0:-1:0:0"
}

func uniqueStrings(values []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		if seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

func filterModes(modes []string, selected []string) []string {
	selected = expandModeFilters(selected)
	if len(selected) == 0 {
		return uniqueStrings(modes)
	}
	allowed := map[string]bool{}
	for _, mode := range selected {
		allowed[mode] = true
	}
	out := make([]string, 0, len(modes))
	for _, mode := range modes {
		if allowed[mode] {
			out = append(out, mode)
		}
	}
	return uniqueStrings(out)
}

func expandModeFilters(filters []string) []string {
	out := []string{}
	for _, filter := range filters {
		for _, mode := range strings.Split(filter, ",") {
			mode = strings.TrimSpace(mode)
			if mode != "" {
				out = append(out, mode)
			}
		}
	}
	return uniqueStrings(out)
}

func runPrivateReplayerCommand(name, replayer string, args []string, timeout time.Duration) privateReplayerResult {
	outputDir := ""
	for i := 0; i+1 < len(args); i++ {
		if args[i] == "--output" {
			outputDir = args[i+1]
			_ = os.MkdirAll(outputDir, 0o755)
			break
		}
	}
	fullArgs := append([]string{}, args...)
	if outputDir != "" {
		if err := preflightPrivateReplayerResources(outputDir); err != nil {
			return privateReplayerResult{
				Name:          name,
				Cmd:           append([]string{replayer}, fullArgs...),
				OutputDir:     outputDir,
				Signal:        err.Error(),
				StderrPreview: err.Error(),
			}
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	start := time.Now()
	command := exec.CommandContext(ctx, replayer, fullArgs...)
	stdoutPath, stderrPath := commandLogPaths(outputDir, name)
	stdout, stderr, err := runCommandCaptureWithLogs(ctx, command, stdoutPath, stderrPath)
	result := privateReplayerResult{
		Name:          name,
		Cmd:           append([]string{replayer}, fullArgs...),
		OutputDir:     outputDir,
		ElapsedMillis: time.Since(start).Milliseconds(),
		TimedOut:      ctx.Err() == context.DeadlineExceeded,
		StdoutBytes:   len(stdout),
		StderrBytes:   len(stderr),
		StdoutPath:    stdoutPath,
		StderrPath:    stderrPath,
		StdoutPreview: previewOutput(stdout),
		StderrPreview: previewOutput(stderr),
	}
	if outputDir != "" {
		result.FileCount, result.ProfilerFiles = summarizeProbeFiles(outputDir)
		result.ServicePayloads = summarizeReplayServicePayloads(outputDir)
	}
	if err == nil {
		return result
	}
	if exitErr, ok := err.(*exec.ExitError); ok {
		if status, ok := exitErr.Sys().(syscall.WaitStatus); ok {
			if status.Signaled() {
				result.Signal = status.Signal().String()
			} else {
				result.ExitCode = status.ExitStatus()
			}
			return result
		}
		result.ExitCode = exitErr.ExitCode()
		return result
	}
	result.Signal = err.Error()
	return result
}

func runExternalCommand(name string, argv []string, outputDir string, timeout time.Duration) privateReplayerResult {
	if outputDir != "" {
		if err := preflightPrivateReplayerResources(outputDir); err != nil {
			return privateReplayerResult{
				Name:          name,
				Cmd:           append([]string{}, argv...),
				OutputDir:     outputDir,
				Signal:        err.Error(),
				StderrPreview: err.Error(),
			}
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	start := time.Now()
	command := exec.CommandContext(ctx, argv[0], argv[1:]...)
	stdoutPath, stderrPath := commandLogPaths(outputDir, name)
	stdout, stderr, err := runCommandCaptureWithLogs(ctx, command, stdoutPath, stderrPath)
	result := privateReplayerResult{
		Name:          name,
		Cmd:           append([]string{}, argv...),
		OutputDir:     outputDir,
		ElapsedMillis: time.Since(start).Milliseconds(),
		TimedOut:      ctx.Err() == context.DeadlineExceeded,
		StdoutBytes:   len(stdout),
		StderrBytes:   len(stderr),
		StdoutPath:    stdoutPath,
		StderrPath:    stderrPath,
		StdoutPreview: previewOutput(stdout),
		StderrPreview: previewOutput(stderr),
	}
	if outputDir != "" {
		result.FileCount, result.ProfilerFiles = summarizeProbeFiles(outputDir)
		result.ServicePayloads = summarizeReplayServicePayloads(outputDir)
	}
	if err == nil {
		return result
	}
	if exitErr, ok := err.(*exec.ExitError); ok {
		if status, ok := exitErr.Sys().(syscall.WaitStatus); ok {
			if status.Signaled() {
				result.Signal = status.Signal().String()
			} else {
				result.ExitCode = status.ExitStatus()
			}
			return result
		}
		result.ExitCode = exitErr.ExitCode()
		return result
	}
	result.Signal = err.Error()
	return result
}

func commandLogPaths(outputDir, name string) (string, string) {
	if outputDir == "" {
		return "", ""
	}
	_ = os.MkdirAll(outputDir, 0o755)
	safeName := strings.NewReplacer("/", "_", " ", "_", ":", "_").Replace(name)
	return filepath.Join(outputDir, safeName+".stdout.txt"), filepath.Join(outputDir, safeName+".stderr.txt")
}

func runCommandCaptureWithLogs(ctx context.Context, command *exec.Cmd, stdoutPath, stderrPath string) ([]byte, []byte, error) {
	if stdoutPath == "" && stderrPath == "" {
		return runCommandCapture(ctx, command)
	}
	stdout := &limitedBuffer{limit: privateReplayerCaptureLimit}
	stderr := &limitedBuffer{limit: privateReplayerCaptureLimit}
	var stdoutFile *os.File
	var stderrFile *os.File
	var err error
	if stdoutPath != "" {
		stdoutFile, err = os.Create(stdoutPath)
		if err != nil {
			return stdout.Bytes(), stderr.Bytes(), err
		}
		defer stdoutFile.Close()
		command.Stdout = io.MultiWriter(stdoutFile, stdout)
	} else {
		command.Stdout = stdout
	}
	if stderrPath != "" {
		stderrFile, err = os.Create(stderrPath)
		if err != nil {
			return stdout.Bytes(), stderr.Bytes(), err
		}
		defer stderrFile.Close()
		command.Stderr = io.MultiWriter(stderrFile, stderr)
	} else {
		command.Stderr = stderr
	}
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := command.Start(); err != nil {
		return stdout.Bytes(), stderr.Bytes(), err
	}
	waitDone := make(chan error, 1)
	go func() {
		waitDone <- command.Wait()
	}()
	select {
	case err := <-waitDone:
		return stdout.Bytes(), stderr.Bytes(), err
	case <-ctx.Done():
		terminateProcessGroup(command.Process.Pid)
		select {
		case err := <-waitDone:
			return stdout.Bytes(), stderr.Bytes(), errors.Join(ctx.Err(), err)
		case <-time.After(2 * time.Second):
			killProcessGroup(command.Process.Pid)
			err := <-waitDone
			return stdout.Bytes(), stderr.Bytes(), errors.Join(ctx.Err(), err)
		}
	}
}

func runCommandCapture(ctx context.Context, command *exec.Cmd) ([]byte, []byte, error) {
	stdout := &limitedBuffer{limit: privateReplayerCaptureLimit}
	stderr := &limitedBuffer{limit: privateReplayerCaptureLimit}
	command.Stdout = stdout
	command.Stderr = stderr
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := command.Start(); err != nil {
		return stdout.Bytes(), stderr.Bytes(), err
	}
	waitDone := make(chan error, 1)
	go func() {
		waitDone <- command.Wait()
	}()
	select {
	case err := <-waitDone:
		return stdout.Bytes(), stderr.Bytes(), err
	case <-ctx.Done():
		terminateProcessGroup(command.Process.Pid)
		select {
		case err := <-waitDone:
			return stdout.Bytes(), stderr.Bytes(), errors.Join(ctx.Err(), err)
		case <-time.After(2 * time.Second):
			killProcessGroup(command.Process.Pid)
			err := <-waitDone
			return stdout.Bytes(), stderr.Bytes(), errors.Join(ctx.Err(), err)
		}
	}
}

func previewOutput(output []byte) string {
	const limit = 1200
	output = bytes.TrimSpace(output)
	if len(output) <= limit {
		return string(output)
	}
	const side = limit / 2
	return string(output[:side]) + "...<truncated>..." + string(output[len(output)-side:])
}

type limitedBuffer struct {
	buf       bytes.Buffer
	limit     int
	truncated bool
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	if b.limit <= 0 {
		return len(p), nil
	}
	remaining := b.limit - b.buf.Len()
	if remaining <= 0 {
		b.truncated = true
		return len(p), nil
	}
	if len(p) > remaining {
		_, _ = b.buf.Write(p[:remaining])
		b.truncated = true
		return len(p), nil
	}
	_, _ = b.buf.Write(p)
	return len(p), nil
}

func (b *limitedBuffer) Bytes() []byte {
	data := b.buf.Bytes()
	if !b.truncated {
		return data
	}
	suffix := []byte("\n...<output truncated>...\n")
	out := make([]byte, 0, len(data)+len(suffix))
	out = append(out, data...)
	out = append(out, suffix...)
	return out
}

var _ io.Writer = (*limitedBuffer)(nil)

func terminateProcessGroup(pid int) {
	if pid <= 0 {
		return
	}
	_ = syscall.Kill(-pid, syscall.SIGTERM)
	_ = syscall.Kill(pid, syscall.SIGTERM)
}

func killProcessGroup(pid int) {
	if pid <= 0 {
		return
	}
	_ = syscall.Kill(-pid, syscall.SIGKILL)
	_ = syscall.Kill(pid, syscall.SIGKILL)
}

func summarizeProbeFiles(root string) (int, []string) {
	files := []string{}
	profilerFiles := []string{}
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		files = append(files, path)
		name := filepath.Base(path)
		if filepath.Ext(filepath.Dir(path)) == ".gpuprofiler_raw" ||
			name == "streamData" ||
			matchAny(name, "Counters_f_*", "Profiling_f_*", "Timeline_f_*", "kdebug*") {
			profilerFiles = append(profilerFiles, path)
		}
		return nil
	})
	sort.Strings(profilerFiles)
	return len(files), profilerFiles
}

func summarizeReplayServicePayloads(root string) []replayServicePayloadSummary {
	payloads := []replayServicePayloadSummary{}
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || filepath.Ext(path) != ".bin" {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		payload, ok := summarizeReplayServicePayload(path, data)
		if ok {
			payloads = append(payloads, payload)
		}
		return nil
	})
	sort.Slice(payloads, func(i, j int) bool {
		return payloads[i].Path < payloads[j].Path
	})
	return payloads
}

func summarizeReplayServicePayload(path string, data []byte) (replayServicePayloadSummary, bool) {
	root, err := decodeNSKeyedArchive(data)
	if err != nil {
		return replayServicePayloadSummary{}, false
	}
	payload := replayServicePayloadSummary{
		Path:              path,
		Kind:              "nskeyed_archive",
		ArchiveClassHints: archiveClassHints(data),
	}
	dict, ok := root.(map[string]any)
	if !ok {
		if responses, ok := root.([]any); ok {
			payload.Kind = "response_array"
			payload.ResponseCount = len(responses)
			for _, response := range responses {
				if responseDict, ok := response.(map[string]any); ok {
					if data, ok := bytesFromAny(responseDict["data"]); ok {
						payload.ResponseDataBytes = append(payload.ResponseDataBytes, len(data))
						payload.ResponseDataPayloads = append(payload.ResponseDataPayloads, summarizeResponseData(data))
					}
					if errSummary, ok := summarizeResponseError(responseDict["error"]); ok {
						payload.ResponseErrors = append(payload.ResponseErrors, errSummary)
					}
				}
			}
			return payload, true
		}
		payload.Kind = "nskeyed_archive_non_dictionary"
		return payload, true
	}
	payload.Keys = sortedMapKeys(dict)
	dataValueBytes := map[string]int{}
	for _, key := range payload.Keys {
		if data, ok := bytesFromAny(dict[key]); ok {
			dataValueBytes[key] = len(data)
		}
	}
	if len(dataValueBytes) > 0 {
		payload.DataValueBytes = dataValueBytes
	}
	if data, ok := bytesFromAny(dict["data"]); ok && len(data) > 0 {
		payload.ResponseDataBytes = append(payload.ResponseDataBytes, len(data))
		payload.ResponseDataPayloads = append(payload.ResponseDataPayloads, summarizeResponseData(data))
		if nested, ok := summarizeNestedResponseData(dict); ok {
			payload.Kind = nested.Kind
			if len(nested.Keys) > 0 {
				payload.Keys = nested.Keys
			}
			payload.NumberOfPasses = nested.NumberOfPasses
			payload.CounterLists = nested.CounterLists
			payload.Counters = nested.Counters
			payload.AverageSampleRowCount = nested.AverageSampleRowCount
			payload.FirstAverageSampleByCounter = nested.FirstAverageSampleByCounter
			payload.FirstAverageSampleByGRCCounter = nested.FirstAverageSampleByGRCCounter
		}
	}
	if v, ok := dict["Streaming APS Data"].(bool); ok {
		payload.Kind = "timeline_marker"
		payload.StreamingAPSData = &v
	}
	if v, ok := dict["Batch Filtering Started"].(bool); ok {
		payload.Kind = "batch_filter_marker"
		payload.BatchFilteringStarted = &v
	}
	if _, ok := dict["AverageSamples"]; ok {
		payload.Kind = "derived_counter_summary"
		payload.NumberOfPasses = intFromAny(dict["numberOfPasses"])
		payload.CounterLists = stringMatrixFromAny(dict["counterLists"])
		payload.Counters = stringSliceFromAny(dict["counters"])
		rows := findNumericPairRows(dict["AverageSamples"])
		payload.AverageSampleRowCount = len(rows)
		if len(rows) > 0 {
			if len(payload.Counters) == len(rows[0]) {
				payload.FirstAverageSampleByCounter = pairMap(payload.Counters, rows[0])
			}
			if len(payload.CounterLists) > 0 && len(payload.CounterLists[0]) == len(rows[0]) {
				payload.FirstAverageSampleByGRCCounter = pairMap(payload.CounterLists[0], rows[0])
			}
		}
	}
	if _, ok := dict["Apple M3 Ultra"]; ok {
		payload.Kind = "device_capabilities"
	}
	if len(payload.Keys) == 0 && len(dataValueBytes) == 0 {
		if summary, ok := summarizeNestedResponseData(dict); ok {
			return summary, true
		}
	}
	return payload, true
}

func summarizeNestedResponseData(dict map[string]any) (replayServicePayloadSummary, bool) {
	data, ok := bytesFromAny(dict["data"])
	if !ok || len(data) == 0 {
		return replayServicePayloadSummary{}, false
	}
	nested, err := decodeNSKeyedArchive(data)
	if err != nil {
		return replayServicePayloadSummary{}, false
	}
	payload := replayServicePayloadSummary{
		Kind:              "response_data_nested_archive",
		ResponseDataBytes: []int{len(data)},
		ArchiveClassHints: archiveClassHints(data),
	}
	switch nestedValue := nested.(type) {
	case map[string]any:
		payload.Keys = sortedMapKeys(nestedValue)
		if _, ok := nestedValue["AverageSamples"]; ok {
			payload.Kind = "derived_counter_summary"
			payload.NumberOfPasses = intFromAny(nestedValue["numberOfPasses"])
			payload.CounterLists = stringMatrixFromAny(nestedValue["counterLists"])
			payload.Counters = stringSliceFromAny(nestedValue["counters"])
			rows := findNumericPairRows(nestedValue["AverageSamples"])
			payload.AverageSampleRowCount = len(rows)
			if len(rows) > 0 {
				if len(payload.Counters) == len(rows[0]) {
					payload.FirstAverageSampleByCounter = pairMap(payload.Counters, rows[0])
				}
				if len(payload.CounterLists) > 0 && len(payload.CounterLists[0]) == len(rows[0]) {
					payload.FirstAverageSampleByGRCCounter = pairMap(payload.CounterLists[0], rows[0])
				}
			}
		}
	case []any:
		payload.Kind = "response_data_nested_array"
		payload.ResponseCount = len(nestedValue)
	default:
		payload.Kind = fmt.Sprintf("response_data_nested_%T", nestedValue)
	}
	return payload, true
}

func decodeNSKeyedArchive(data []byte) (any, error) {
	var archive map[string]any
	if _, err := plist.Unmarshal(data, &archive); err != nil {
		return nil, err
	}
	objects, ok := archive["$objects"].([]any)
	if !ok {
		return archive, nil
	}
	top, ok := archive["$top"].(map[string]any)
	if !ok {
		return archive, nil
	}
	rootUID, ok := top["root"].(plist.UID)
	if !ok {
		return archive, nil
	}
	return decodeNSKeyedObject(objects, int(rootUID), map[int]bool{}), nil
}

func decodeNSKeyedObject(objects []any, idx int, seen map[int]bool) any {
	if idx < 0 || idx >= len(objects) {
		return nil
	}
	if idx == 0 {
		return nil
	}
	if seen[idx] {
		return nil
	}
	obj := objects[idx]
	switch v := obj.(type) {
	case plist.UID:
		return decodeNSKeyedObject(objects, int(v), seen)
	case map[string]any:
		seen[idx] = true
		defer delete(seen, idx)
		if data, ok := v["NS.data"]; ok {
			return decodeNSKeyedRef(objects, data, seen)
		}
		if keys, ok := v["NS.keys"].([]any); ok {
			vals, _ := v["NS.objects"].([]any)
			out := map[string]any{}
			for i, keyRef := range keys {
				if i >= len(vals) {
					break
				}
				key := fmt.Sprint(decodeNSKeyedRef(objects, keyRef, seen))
				out[key] = decodeNSKeyedRef(objects, vals[i], seen)
			}
			return out
		}
		if refs, ok := v["NS.objects"].([]any); ok {
			out := make([]any, 0, len(refs))
			for _, ref := range refs {
				out = append(out, decodeNSKeyedRef(objects, ref, seen))
			}
			return out
		}
		out := map[string]any{}
		for key, value := range v {
			if key == "$class" {
				continue
			}
			out[key] = decodeNSKeyedRef(objects, value, seen)
		}
		return out
	default:
		return v
	}
}

func decodeNSKeyedRef(objects []any, value any, seen map[int]bool) any {
	if uid, ok := value.(plist.UID); ok {
		return decodeNSKeyedObject(objects, int(uid), seen)
	}
	return value
}

func summarizeResponseData(data []byte) responseDataSummary {
	summary := responseDataSummary{
		Bytes:             len(data),
		ArchiveClassHints: archiveClassHints(data),
	}
	root, err := decodeNSKeyedArchive(data)
	if err != nil {
		summary.Kind = "raw"
		return summary
	}
	switch v := root.(type) {
	case map[string]any:
		summary.Kind = "nskeyed_dictionary"
		summary.Keys = sortedMapKeys(v)
	case []any:
		summary.Kind = "nskeyed_array"
	default:
		summary.Kind = fmt.Sprintf("%T", v)
	}
	return summary
}

func archiveClassHints(data []byte) []string {
	candidates := []string{
		"DYWorkloadGPUTimelineInfo",
		"DYGPUTimelineInfo",
		"DYTimelineCounterGroup",
		"DYGTMTLDeviceProfile",
		"GTMTLReplayActivityCollectCounters",
		"GTReplayProfileTimeline",
		"GTReplayProfileDerivedCounters",
		"GTReplayProfileBatchFilteredCounters",
		"GTReplayProfileReplyStream",
		"GTMutableShaderProfilerStreamData",
	}
	hints := []string{}
	for _, candidate := range candidates {
		if bytes.Contains(data, []byte(candidate)) {
			hints = append(hints, candidate)
		}
	}
	return hints
}

func summarizeResponseError(v any) (responseErrorSummary, bool) {
	errMap, ok := v.(map[string]any)
	if !ok || len(errMap) == 0 {
		return responseErrorSummary{}, false
	}
	out := responseErrorSummary{
		Domain: stringFromAny(errMap["NSDomain"]),
		Code:   intFromAny(errMap["NSCode"]),
	}
	if userInfo, ok := errMap["NSUserInfo"].(map[string]any); ok {
		out.Description = stringFromAny(userInfo["NSLocalizedDescription"])
		out.RecoverySuggestion = stringFromAny(userInfo["NSLocalizedRecoverySuggestion"])
	}
	return out, true
}

func stringFromAny(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func bytesFromAny(v any) ([]byte, bool) {
	switch data := v.(type) {
	case []byte:
		return data, true
	case map[string]any:
		return bytesFromAny(data["NS.data"])
	case []any:
		out := make([]byte, 0, len(data))
		for _, item := range data {
			n, ok := uint64FromAny(item)
			if !ok || n > 255 {
				return nil, false
			}
			out = append(out, byte(n))
		}
		return out, true
	default:
		return nil, false
	}
}

func sortedMapKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func intFromAny(v any) int {
	switch n := v.(type) {
	case int:
		return n
	case int64:
		return int(n)
	case uint64:
		return int(n)
	case uint:
		return int(n)
	case float64:
		return int(n)
	default:
		return 0
	}
}

func stringSliceFromAny(v any) []string {
	items, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		if s, ok := item.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

func stringMatrixFromAny(v any) [][]string {
	rows, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([][]string, 0, len(rows))
	for _, row := range rows {
		out = append(out, stringSliceFromAny(row))
	}
	return out
}

func findNumericPairRows(v any) [][][]uint64 {
	rows := [][][]uint64{}
	var walk func(any)
	walk = func(node any) {
		items, ok := node.([]any)
		if !ok || len(items) == 0 {
			return
		}
		row := make([][]uint64, 0, len(items))
		allPairs := true
		for _, item := range items {
			pair, ok := numericPair(item)
			if !ok {
				allPairs = false
				break
			}
			row = append(row, pair)
		}
		if allPairs {
			rows = append(rows, row)
			return
		}
		for _, item := range items {
			walk(item)
		}
	}
	walk(v)
	return rows
}

func numericPair(v any) ([]uint64, bool) {
	items, ok := v.([]any)
	if !ok || len(items) != 2 {
		return nil, false
	}
	a, okA := uint64FromAny(items[0])
	b, okB := uint64FromAny(items[1])
	if !okA || !okB {
		return nil, false
	}
	return []uint64{a, b}, true
}

func uint64FromAny(v any) (uint64, bool) {
	switch n := v.(type) {
	case uint64:
		return n, true
	case uint:
		return uint64(n), true
	case int:
		if n >= 0 {
			return uint64(n), true
		}
	case int64:
		if n >= 0 {
			return uint64(n), true
		}
	}
	return 0, false
}

func pairMap(names []string, row [][]uint64) map[string][]uint64 {
	out := map[string][]uint64{}
	for i, name := range names {
		if i < len(row) {
			out[name] = row[i]
		}
	}
	return out
}

func findProbeProfilerDir(root string) string {
	found := ""
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || !d.IsDir() || found != "" {
			return nil
		}
		if filepath.Ext(path) == ".gpuprofiler_raw" {
			found = path
		}
		return nil
	})
	return found
}

func matchAny(name string, patterns ...string) bool {
	for _, pattern := range patterns {
		if ok, _ := filepath.Match(pattern, name); ok {
			return true
		}
	}
	return false
}

const gtmtlReplayProbeHelperSource = `
#include <dlfcn.h>
#include <stdint.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/stat.h>

typedef int (*main_like_fn)(int argc, char **argv);
typedef int (*options_fn)(const char *archive, void *options, void *callback);
typedef void (*init_fn)(void *pool);

static void callback_stub(void) {}

struct fake_pool {
    unsigned char bytes[0x80];
};

struct fake_allocator {
    unsigned char bytes[0xc8];
};

static void *checked_alloc(size_t size) {
    void *ptr = calloc(1, size);
    if (!ptr) {
        return NULL;
    }
    return ptr;
}

static int init_fake_apr(void *handle) {
    init_fn init = (init_fn)dlsym(handle, "GTMTLReplayController_init");
    if (!init) {
        fprintf(stderr, "dlsym init: %s\n", dlerror());
        return 120;
    }
    struct fake_pool *root = checked_alloc(sizeof(struct fake_pool));
    struct fake_allocator *allocator = checked_alloc(sizeof(struct fake_allocator));
    if (!root || !allocator) {
        return 121;
    }
    *(uint64_t *)(allocator->bytes + 0x08) = 0x8000;
    *(uint64_t *)(allocator->bytes + 0x10) = 0x8000;
    *(void **)(root->bytes + 0x30) = allocator;
    init(root);
    return 0;
}

static void seed_cli_options(void *options, const char *out_dir, uint32_t flags) {
    /*
     * arm64e MTLReplayer builds GTMTLReplayCLIOptions at sp+0x40.  The CLI
     * parser writes flags at sp+0x1b8, so the framework entrypoint sees them
     * at options+0x178.  The string/int slots below are from the arm64e
     * ParseArguments jump table in MTLReplayer.
     */
    *(char **)((char *)options + 0x030) = (char *)out_dir; /* old guess */
    *(char **)((char *)options + 0x0c8) = (char *)out_dir; /* -dumpProfileDictionary */
    *(char **)((char *)options + 0x0f0) = (char *)out_dir; /* --output/-outputPath */
    *(char **)((char *)options + 0x140) = (char *)out_dir; /* -collectRawCounters */
    *(char **)((char *)options + 0x150) = (char *)out_dir; /* resourceTracking path */
    *(uint32_t *)((char *)options + 0x0d8) = 1;            /* loopCount default */
    *(uint32_t *)((char *)options + 0x0dc) = 1;            /* maxProfilingTime */
    *(uint32_t *)((char *)options + 0x0e0) = 1;            /* maxProfilingFrames */
    *(uint8_t *)((char *)options + 0x0e5) = 1;             /* waitUntilCompleteEachFrame */
    *(uint32_t *)((char *)options + 0x158) = 0;            /* collectProfilerData id */
    *(uint32_t *)((char *)options + 0x164) = 0xffffffff;   /* collectProfilerData level */
    *(uint64_t *)((char *)options + 0x168) = 0;            /* --frame start */
    *(uint64_t *)((char *)options + 0x170) = 0;            /* --frame end */
    *(uint32_t *)((char *)options + 0x178) = flags;        /* real arm64e flags */
    *(uint32_t *)((char *)options + 0x0b8) = flags;        /* old guess */
}

static int call_with_fake_apr(options_fn fn, void *handle, const char *trace, const char *out_dir, uint32_t flags) {
    int init_rc = init_fake_apr(handle);
    if (init_rc != 0) {
        return init_rc;
    }
    void *options = checked_alloc(0x200);
    if (!options) {
        return 122;
    }
    mkdir(out_dir, 0777);
    seed_cli_options(options, out_dir, flags);
    int rc = fn(trace, options, (void *)&callback_stub);
    printf("rc=%d\n", rc);
    return rc;
}

#define FLAG_COLLECT_DERIVED_COUNTERS 0x100u
#define FLAG_PROFILE_TRACE 0x200u
#define FLAG_PERFORM_ANALYSIS 0x800u
#define FLAG_PERFORM_SHADER_PROFILING_ANALYSIS 0x2000u
#define FLAG_COLLECT_RAW_COUNTERS 0x4000u
#define FLAG_TEST_PROFILING 0x8000u
#define FLAG_COLLECT_PERFORMANCE_TIMING 0x10000u
#define FLAG_COLLECT_PIPELINE_PERFORMANCE_STATISTICS 0x80000u
#define FLAG_GPU_TIMELINE_DATA 0x100000u
#define FLAG_COLLECT_PROFILER_DATA 0x1000000u
#define FLAG_PERFECT_PATCHING 0x2000000u

int main(int argc, char **argv) {
    if (argc < 5) {
        fprintf(stderr, "usage: %s <mode> <framework> <trace> <out-dir>\n", argv[0]);
        return 2;
    }
    const char *mode = argv[1];
    const char *framework = argv[2];
    const char *trace = argv[3];
    const char *out_dir = argv[4];
    void *handle = dlopen(framework, RTLD_NOW);
    if (!handle) {
        fprintf(stderr, "dlopen: %s\n", dlerror());
        return 111;
    }
    void *sym = dlsym(handle, "GTMTLReplay_CLI");
    if (!sym) {
        fprintf(stderr, "dlsym: %s\n", dlerror());
        return 112;
    }
    if (strcmp(mode, "main_like") == 0) {
        main_like_fn fn = (main_like_fn)sym;
        char *call_argv[] = {
            "GTMTLReplay_CLI",
            (char *)trace,
            "-profileTrace",
            "-collectProfilerData",
            "--output",
            (char *)out_dir,
            "--frame",
            "0",
            NULL,
        };
        int rc = fn(8, call_argv);
        printf("rc=%d\n", rc);
        return rc;
    }
    if (strcmp(mode, "main_like_full_analysis") == 0) {
        main_like_fn fn = (main_like_fn)sym;
        char *call_argv[] = {
            "GTMTLReplay_CLI",
            (char *)trace,
            "-profileTrace",
            "--performAnalysis",
            (char *)out_dir,
            "--performShaderProfilingAnalysis",
            (char *)out_dir,
            "-collectProfilerData",
            "max",
            (char *)out_dir,
            "-collectPerformanceTiming",
            "-collectRawCounters",
            (char *)out_dir,
            "-collectDerivedCounters",
            "-collectPipelinePerformanceStatistics",
            "-gpuTimelineData",
            "-perfectPatching",
            "1",
            (char *)out_dir,
            "--output",
            (char *)out_dir,
            "--frame",
            "0",
            "-maxProfilingTime",
            "1",
            "-maxProfilingFrames",
            "1",
            "-waitUntilCompleteEachFrame",
            NULL,
        };
        int rc = fn(28, call_argv);
        printf("rc=%d\n", rc);
        return rc;
    }
    if (strcmp(mode, "fake_apr_timing") == 0) {
        options_fn fn = (options_fn)sym;
        return call_with_fake_apr(fn, handle, trace, out_dir, FLAG_COLLECT_PERFORMANCE_TIMING | FLAG_PROFILE_TRACE);
    }
    if (strcmp(mode, "fake_apr_profiler") == 0) {
        options_fn fn = (options_fn)sym;
        return call_with_fake_apr(fn, handle, trace, out_dir, 0x200000 | FLAG_PROFILE_TRACE);
    }
	if (strcmp(mode, "fake_apr_real_offsets_timing") == 0) {
		options_fn fn = (options_fn)sym;
		return call_with_fake_apr(fn, handle, trace, out_dir,
			FLAG_PROFILE_TRACE |
			FLAG_COLLECT_PERFORMANCE_TIMING);
	}
    if (strcmp(mode, "fake_apr_real_offsets_profiler") == 0) {
        options_fn fn = (options_fn)sym;
        return call_with_fake_apr(fn, handle, trace, out_dir,
            FLAG_PROFILE_TRACE |
            FLAG_COLLECT_PERFORMANCE_TIMING |
            FLAG_COLLECT_RAW_COUNTERS |
            FLAG_COLLECT_DERIVED_COUNTERS |
            0x40000 |
            0x80000 |
            0x200000);
    }
    if (strcmp(mode, "fake_apr_actual_flags_profiler") == 0) {
        options_fn fn = (options_fn)sym;
        return call_with_fake_apr(fn, handle, trace, out_dir,
            FLAG_PROFILE_TRACE |
            FLAG_COLLECT_PERFORMANCE_TIMING |
            FLAG_COLLECT_RAW_COUNTERS |
            FLAG_COLLECT_DERIVED_COUNTERS |
            FLAG_COLLECT_PIPELINE_PERFORMANCE_STATISTICS |
            FLAG_GPU_TIMELINE_DATA |
            FLAG_COLLECT_PROFILER_DATA);
    }
    if (strcmp(mode, "fake_apr_actual_flags_analysis") == 0) {
        options_fn fn = (options_fn)sym;
        return call_with_fake_apr(fn, handle, trace, out_dir,
            FLAG_PROFILE_TRACE |
            FLAG_PERFORM_ANALYSIS |
            FLAG_PERFORM_SHADER_PROFILING_ANALYSIS |
            FLAG_COLLECT_PERFORMANCE_TIMING |
            FLAG_COLLECT_RAW_COUNTERS |
            FLAG_COLLECT_DERIVED_COUNTERS |
            FLAG_COLLECT_PIPELINE_PERFORMANCE_STATISTICS |
            FLAG_GPU_TIMELINE_DATA |
            FLAG_COLLECT_PROFILER_DATA |
            FLAG_PERFECT_PATCHING);
    }
    if (strcmp(mode, "fake_apr_real_offsets_raw_derived") == 0) {
        options_fn fn = (options_fn)sym;
        return call_with_fake_apr(fn, handle, trace, out_dir,
            FLAG_PROFILE_TRACE | FLAG_COLLECT_PERFORMANCE_TIMING | FLAG_COLLECT_RAW_COUNTERS | FLAG_COLLECT_DERIVED_COUNTERS);
    }
    if (strcmp(mode, "fake_apr_profile_dictionary") == 0) {
        options_fn fn = (options_fn)sym;
        return call_with_fake_apr(fn, handle, trace, out_dir,
            FLAG_PROFILE_TRACE | FLAG_COLLECT_PERFORMANCE_TIMING | FLAG_COLLECT_RAW_COUNTERS | FLAG_COLLECT_DERIVED_COUNTERS | 0x40000 | 0x80000 | 0x200000);
    }
    if (strcmp(mode, "fake_apr_test_profiling") == 0) {
        options_fn fn = (options_fn)sym;
        return call_with_fake_apr(fn, handle, trace, out_dir,
            FLAG_TEST_PROFILING |
            FLAG_PROFILE_TRACE |
            FLAG_COLLECT_PERFORMANCE_TIMING |
            FLAG_COLLECT_PIPELINE_PERFORMANCE_STATISTICS |
            FLAG_GPU_TIMELINE_DATA |
            FLAG_COLLECT_PROFILER_DATA);
    }
    if (strcmp(mode, "fake_apr_wait_complete_profiler") == 0) {
        options_fn fn = (options_fn)sym;
        return call_with_fake_apr(fn, handle, trace, out_dir,
            FLAG_PROFILE_TRACE | FLAG_COLLECT_PERFORMANCE_TIMING | FLAG_COLLECT_RAW_COUNTERS | FLAG_COLLECT_DERIVED_COUNTERS | FLAG_COLLECT_PIPELINE_PERFORMANCE_STATISTICS | FLAG_GPU_TIMELINE_DATA | FLAG_COLLECT_PROFILER_DATA);
    }
    if (strcmp(mode, "options_null_callback") == 0 ||
        strcmp(mode, "options_dummy_callback") == 0) {
        options_fn fn = (options_fn)sym;
        void *options = checked_alloc(0x800);
        if (!options) {
            return 113;
        }
        void *callback = NULL;
        if (strcmp(mode, "options_dummy_callback") == 0) {
            callback = (void *)&callback_stub;
        }
        int rc = fn(trace, options, callback);
        printf("rc=%d\n", rc);
        free(options);
        return rc;
    }
    fprintf(stderr, "unknown mode: %s\n", mode);
    return 3;
}
`

const gtReplayServiceProbeHelperSource = `
#import <Foundation/Foundation.h>
#import <dlfcn.h>
#import <mach/mach.h>
#import <mach-o/dyld.h>
#import <Metal/Metal.h>
#import <objc/message.h>
#import <objc/runtime.h>
#include <sys/stat.h>

enum {
    GTProbeAuxFetchCandidateLimit = 4
};

typedef id (*GTTransportServiceDaemonConnectionNewFn)(id);

typedef struct {
    struct {
        int dispatchIndex;
        int dispatchICBIndex;
    } index;
    uint64_t uid;
} GTReplayDispatchUID;

typedef struct {
    uint64_t x;
    uint64_t y;
    uint64_t z;
} GTReplayPoint3D;

typedef struct {
    uint64_t width;
    uint64_t height;
    uint64_t depth;
} GTReplaySize3D;

typedef struct {
    GTReplayPoint3D origin;
    GTReplaySize3D size;
} GTReplayRegion3D;

typedef struct {
    uint64_t location;
    uint64_t length;
} GTRange;

@interface GTTransportClient : NSObject
- (instancetype)initWithConnection:(id)connection;
- (id)allServices;
- (id)launcher;
- (id)replayer;
@end

@interface GTLaunchRequest : NSObject
@property BOOL preferXPCService;
@property BOOL disableDisplay;
@property(copy) NSDictionary *environment;
@property(copy) NSArray *arguments;
@property(copy) NSString *deviceUDID;
@property(copy) NSUUID *sessionUUID;
@end

@interface GTLaunchServiceXPCProxy : NSObject
- (BOOL)launchReplayService:(id)request error:(NSError **)error;
@end

@interface GTReplayRequestBatch : NSObject
@property(copy) NSArray *requests;
@property(copy) void (^completionHandler)(id response);
@end

@interface GTReplayQueryDeviceCapabilities : NSObject
@end

@interface GTReplayQueryConfiguration : NSObject
@end

@interface GTReplayQueryDerivedCounters : NSObject
@end

@interface GTReplayQueryPerformanceState : NSObject
@end

@interface GTReplayQueryResourceUsage : NSObject
@property GTReplayDispatchUID dispatchUID;
@end

@interface GTReplayQuerySessionInfo : NSObject
@end

@interface GTReplayQueryICBTranslation : NSObject
@property GTReplayDispatchUID dispatchUID;
@end

@interface GTReplayQueryRasterMap : NSObject
@property GTReplayDispatchUID dispatchUID;
@property uint64_t streamRef;
@end

@interface GTReplayFetchPipelineBinaries : NSObject
@property GTReplayDispatchUID dispatchUID;
@property uint64_t streamRef;
@end

@interface GTReplayFetchBuffer : NSObject
@property GTReplayDispatchUID dispatchUID;
@property uint64_t streamRef;
@property GTRange range;
@end

@interface GTReplayFetchThreadgroup : NSObject
@property GTReplayDispatchUID dispatchUID;
@property unsigned int index;
@end

@interface GTReplayFetchPostVertex : NSObject
@property GTReplayDispatchUID dispatchUID;
@property BOOL objectShaderThreadgroupBoundsPresent;
@property GTReplayPoint3D objectShaderThreadgroupBeginBounds;
@property GTReplayPoint3D objectShaderThreadgroupEndBounds;
@end

@interface GTReplayFetchWireframe : NSObject
@property GTReplayDispatchUID dispatchUID;
@property BOOL solid;
@end

@interface GTReplayFetchTexture : NSObject
@property GTReplayDispatchUID dispatchUID;
@property uint64_t streamRef;
@property unsigned int slice;
@property unsigned int level;
@property unsigned int depth;
@property unsigned int plane;
@property GTReplaySize3D size;
@property GTReplayRegion3D region;
@property BOOL resolveMultisampleTexture;
@end

@interface GTReplayFetchIntoTexture : NSObject
@property GTReplayDispatchUID dispatchUID;
@property(strong) id<MTLTexture> dest;
@property(strong) id<MTLSharedEvent> event;
@property uint64_t streamRef;
@property unsigned int slice;
@property unsigned int level;
@property unsigned int depth;
@property unsigned int plane;
@property BOOL resolveMultisampleTexture;
@end

@interface GTReplayDecodeGenericAccelerationStructure : NSObject
@property GTReplayDispatchUID dispatchUID;
@property uint64_t streamRef;
@end

@interface GTReplayDecodeAB : NSObject
@property GTReplayDispatchUID dispatchUID;
@property unsigned short type;
@property unsigned int index;
@end

@interface GTReplayDecodeICB : NSObject
@property GTReplayDispatchUID dispatchUID;
@property uint64_t streamRef;
@end

@interface GTReplayRaytraceRequest : NSObject
@property GTReplayDispatchUID dispatchUID;
@property uint64_t streamRef;
@property(copy) void (^streamHandler)(id response);
@end

@interface GTReplayConfiguration : NSObject
@property BOOL forceLoadActionClear;
@property BOOL forceLoadUnusedResources;
@property BOOL forceResourcesResident;
@property BOOL forceWaitUntilCompleted;
@property BOOL disableOptimizeRestores;
@property BOOL disableHeapTextureCompression;
@property BOOL enableStopOnError;
@property BOOL enableDisplayOnDevice;
@property BOOL enableReplayFromOtherPlatforms;
@property BOOL enableValidation;
@property BOOL enableCapture;
@property BOOL enableHUD;
@property BOOL enableLiveICBs;
@end

@interface GTReplayUpdateConfiguration : NSObject
@property(strong) GTReplayConfiguration *configuration;
@end

@interface GTReplayUpdateLibrary : NSObject
@property GTReplayDispatchUID dispatchUID;
@property uint64_t streamRef;
@property(copy) NSURL *shaderURL;
@property(copy) NSData *shaderIR;
@property(copy) NSString *shaderSource;
@end

@interface GTReplayUpdateLibraryCache : NSObject
@property(copy) NSString *uuid;
@property(copy) NSData *data;
@end

@interface GTReplayProfileRequest : NSObject
@property NSInteger priority;
@property NSInteger profileDataVersion;
@property(copy) NSData *profileData;
@property(copy) void (^streamHandler)(id response);
@end

@interface GTReplayProfileTimeline : GTReplayProfileRequest
@property BOOL shaderProfiling;
@property BOOL saveProfilerRaw;
@end

@interface GTReplayProfileDerivedCounters : GTReplayProfileRequest
@end

@interface GTReplayProfileBatchFilteredCounters : GTReplayProfileRequest
@end

@interface GTReplayShaderDebugRequest : NSObject
@property GTReplayDispatchUID dispatchUID;
@property NSInteger programDataVersion;
@property(copy) NSData *programData;
@property(copy) void (^completionHandler)(id response);
@end

@interface GTReplayShaderDebugKernel : GTReplayShaderDebugRequest
@property GTReplayPoint3D minThreadPositionInGrid;
@property GTReplayPoint3D maxThreadPositionInGrid;
@end

@interface GTMTLReplayServiceXPCProxy : NSObject
- (uint64_t)registerObserver:(id)observer;
- (void)deregisterObserver:(uint64_t)observerID;
- (BOOL)load:(NSURL *)url error:(NSError **)error;
- (id)query:(id)batch;
- (id)profile:(id)request;
- (id)raytrace:(id)request;
- (id)shaderdebug:(id)request;
- (id)flushRpackets;
- (id)fetch:(id)batch;
- (id)fetchInto:(id)batch;
- (id)decode:(id)batch;
- (id)update:(id)batch;
- (void)display:(id)request;
- (BOOL)resume:(uint64_t)tokenID;
- (BOOL)pause:(uint64_t)tokenID;
- (BOOL)cancel:(uint64_t)tokenID;
- (void)terminateProcess;
@end

@interface GTProbeReplayObserver : NSObject
@end

@interface GTProbeDeviceInfoProxy : NSObject {
	id _backing;
	id _archive;
}
- (instancetype)initWithBacking:(id)backing archive:(id)archive;
- (id)metadataValueForKey:(id)key;
@end

static NSString *stringFromObject(id object);
static id valueOrNil(id object, NSString *key);

@implementation GTProbeReplayObserver
- (BOOL)respondsToSelector:(SEL)selector {
	return YES;
}

- (NSMethodSignature *)methodSignatureForSelector:(SEL)selector {
	return [NSMethodSignature signatureWithObjCTypes:"v@:@"];
}

- (void)forwardInvocation:(NSInvocation *)invocation {
	SEL selector = [invocation selector];
	id argument = nil;
	if ([[invocation methodSignature] numberOfArguments] > 2) {
		[invocation getArgument:&argument atIndex:2];
	}
	fprintf(stdout, "observer callback selector=%s arg=%s\n",
	        selector ? sel_getName(selector) : "",
	        [stringFromObject(argument) UTF8String]);
}
@end

@implementation GTProbeDeviceInfoProxy
- (instancetype)initWithBacking:(id)backing archive:(id)archive {
	self = [super init];
	if (self) {
		_backing = backing;
		_archive = archive;
	}
	return self;
}

- (id)metadataValueForKey:(id)key {
	if (![key isKindOfClass:[NSString class]]) {
		return nil;
	}
	id value = valueOrNil(_backing, key);
	if (value) {
		return value;
	}
	if ([_archive respondsToSelector:@selector(metadataValueForKey:)]) {
		@try {
			return ((id (*)(id, SEL, id))objc_msgSend)(_archive, @selector(metadataValueForKey:), key);
		} @catch (NSException *exception) {
			return nil;
		}
	}
	return nil;
}

- (BOOL)respondsToSelector:(SEL)selector {
	return selector == @selector(metadataValueForKey:) || [_backing respondsToSelector:selector] || [_archive respondsToSelector:selector] || [super respondsToSelector:selector];
}

- (id)forwardingTargetForSelector:(SEL)selector {
	if ([_backing respondsToSelector:selector]) {
		return _backing;
	}
	if ([_archive respondsToSelector:selector]) {
		return _archive;
	}
	return [super forwardingTargetForSelector:selector];
}

- (NSMethodSignature *)methodSignatureForSelector:(SEL)selector {
	NSMethodSignature *signature = [_backing methodSignatureForSelector:selector];
	if (signature) {
		return signature;
	}
	signature = [_archive methodSignatureForSelector:selector];
	if (signature) {
		return signature;
	}
	return [super methodSignatureForSelector:selector];
}

- (NSString *)description {
	return [NSString stringWithFormat:@"<GTProbeDeviceInfoProxy backing=%@>", _backing];
}
@end

static NSString *stringFromObject(id object) {
    if (!object || object == [NSNull null]) {
        return @"";
    }
    return [NSString stringWithFormat:@"%@", object];
}

static id valueOrNil(id object, NSString *key) {
    @try {
        return [object valueForKey:key];
    } @catch (NSException *exception) {
        return nil;
    }
}

static BOOL boolValueOrNo(id object, NSString *key) {
    id value = valueOrNil(object, key);
    if ([value respondsToSelector:@selector(boolValue)]) {
        return [value boolValue];
    }
    return NO;
}

static uint64_t unsignedLongLongValueOrZero(id object, NSString *key) {
	id value = valueOrNil(object, key);
	if ([value respondsToSelector:@selector(unsignedLongLongValue)]) {
		return [value unsignedLongLongValue];
	}
	return 0;
}

static void writeBytes(NSData *data, NSString *outDir, NSString *prefix, int index) {
	if (!data) {
		return;
	}
	NSString *file = [outDir stringByAppendingPathComponent:[NSString stringWithFormat:@"%@-%03d.bin", prefix, index]];
	[data writeToFile:file atomically:NO];
}

static void writeObject(id object, NSString *outDir, NSString *prefix, int index) {
	if (!object) {
		return;
	}
	NSError *error = nil;
	NSData *data = [NSKeyedArchiver archivedDataWithRootObject:object requiringSecureCoding:NO error:&error];
	if (!data) {
		fprintf(stdout, "%s archive object failed: %s\n", [prefix UTF8String], [[error description] UTF8String]);
		return;
	}
	writeBytes(data, outDir, [prefix stringByAppendingString:@"-object"], index);
}

static NSData *responseData(id response) {
	id data = valueOrNil(response, @"data");
	if ([data isKindOfClass:[NSData class]]) {
		return data;
	}
    return nil;
}

static id unarchivedObjectFromData(NSData *data) {
	if (!data) {
		return nil;
	}
	@try {
#pragma clang diagnostic push
#pragma clang diagnostic ignored "-Wdeprecated-declarations"
		return [NSKeyedUnarchiver unarchiveObjectWithData:data];
#pragma clang diagnostic pop
	} @catch (NSException *exception) {
		fprintf(stdout, "unarchive exception=%s\n", [[exception description] UTF8String]);
	}
	@try {
#pragma clang diagnostic push
#pragma clang diagnostic ignored "-Wdeprecated-declarations"
		NSKeyedUnarchiver *unarchiver = [[NSKeyedUnarchiver alloc] initForReadingWithData:data];
		Class deviceInfoClass = NSClassFromString(@"DYDeviceInfo");
		if (deviceInfoClass && [unarchiver respondsToSelector:@selector(setClass:forClassName:)]) {
			[unarchiver setClass:deviceInfoClass forClassName:@"DYGTDeviceInfo"];
		}
		Class deviceProfileClass = NSClassFromString(@"DYGTMTLDeviceProfile");
		if (deviceProfileClass && [unarchiver respondsToSelector:@selector(setClass:forClassName:)]) {
			[unarchiver setClass:deviceProfileClass forClassName:@"DYGTMTLDeviceProfile"];
		}
		id object = [unarchiver decodeObjectForKey:NSKeyedArchiveRootObjectKey];
		[unarchiver finishDecoding];
		return object;
#pragma clang diagnostic pop
	} @catch (NSException *exception) {
		fprintf(stdout, "mapped unarchive exception=%s\n", [[exception description] UTF8String]);
		return nil;
	}
}

static id normalizedProfilerPayload(id payload) {
	for (int depth = 0; depth < 4 && payload; depth++) {
		if ([payload isKindOfClass:[NSArray class]] && [(NSArray *)payload count] == 1) {
			payload = [(NSArray *)payload objectAtIndex:0];
			continue;
		}
		NSData *nestedData = responseData(payload);
		if (nestedData) {
			id nested = unarchivedObjectFromData(nestedData);
			if (nested && nested != payload) {
				payload = nested;
				continue;
			}
		}
		break;
	}
	return payload;
}

static void logResponse(id response, NSString *label) {
	NSData *data = responseData(response);
	id error = valueOrNil(response, @"error");
	id requestID = valueOrNil(response, @"requestID");
	NSString *keys = @"";
	if ([response isKindOfClass:[NSDictionary class]]) {
		keys = [[[((NSDictionary *)response) allKeys] valueForKey:@"description"] componentsJoinedByString:@","];
	}
	fprintf(stdout, "%s response class=%s requestID=%s data=%lu error=%s keys=%s object=%s\n",
	        [label UTF8String],
	        [stringFromObject([response class]) UTF8String],
	        [stringFromObject(requestID) UTF8String],
	        (unsigned long)(data ? [data length] : 0),
	        [stringFromObject(error) UTF8String],
	        [keys UTF8String],
	        [stringFromObject(response) UTF8String]);
	fflush(stdout);
}

static id firstService(id services, NSString *name) {
    for (id service in services) {
        NSString *serviceName = stringFromObject(valueOrNil(service, @"serviceName"));
        if ([serviceName isEqualToString:name]) {
            return service;
        }
    }
    return nil;
}

static NSString *deviceUDIDFromServices(id services) {
    id launchService = firstService(services, @"GTLaunchService");
    NSString *udid = stringFromObject(valueOrNil(launchService, @"deviceUDID"));
    if ([udid length] > 0) {
        return udid;
    }
    for (id service in services) {
        udid = stringFromObject(valueOrNil(service, @"deviceUDID"));
        if ([udid length] > 0) {
            return udid;
        }
    }
	return nil;
}

static NSArray *launchArgumentsForMode(NSString *mode, NSString *tracePath, NSString *outDir) {
	NSMutableArray *arguments = [NSMutableArray array];
	if ([mode containsString:@"launch_cli_args"]) {
		NSString *profileOutDir = [outDir stringByAppendingPathComponent:@"launch-cli-output"];
		mkdir([profileOutDir fileSystemRepresentation], 0777);
		[arguments addObjectsFromArray:@[
			@"-CLI",
			tracePath,
			@"-profileTrace",
			@"-collectProfilerData",
			@"-collectPerformanceTiming",
			@"-gpuTimelineData",
			@"-collectPipelinePerformanceStatistics",
			@"-collectRawCounters",
			@"-collectDerivedCounters",
			@"--frame",
			@"0",
			@"--output",
			profileOutDir,
		]];
	}
	return arguments;
}

static NSDictionary *launchEnvironmentForMode(NSString *mode) {
	if ([mode containsString:@"launch_min_env"]) {
		return @{
			@"MTL_SHADER_VALIDATION": @"0",
			@"METAL_DEVICE_WRAPPER_TYPE": @"0",
		};
	}
	return @{};
}

static NSDictionary *apsConfigDictionary(NSString *initializerName);
static id directProfileDataObject(NSString *mode);
static NSData *archivedEmptyDictionary(void);
static NSData *archivedShaderProfilerSessionRequest(NSString *mode);
static id newMutableProfilerStreamData(NSString *mode);
static id newProfileRequest(NSString *mode);
static void dumpRuntimeClass(NSString *className);
static NSString *currentProfileOutDir = nil;

static void dumpObjectField(id object, NSString *label, NSString *key) {
	id value = valueOrNil(object, key);
	if (value) {
		fprintf(stdout, "%s %s=%s\n", [label UTF8String], [key UTF8String], [stringFromObject(value) UTF8String]);
	}
}

static void dumpObjectSnapshot(id object, NSString *label) {
	if (!object) {
		fprintf(stdout, "%s object=(nil)\n", [label UTF8String]);
		return;
	}
	fprintf(stdout, "%s class=%s object=%s\n",
	        [label UTF8String],
	        [stringFromObject([object class]) UTF8String],
	        [stringFromObject(object) UTF8String]);
	for (NSString *key in @[
		     @"serviceName",
		     @"deviceUDID",
		     @"dispatcherId",
		     @"tokenId",
		     @"serviceProperties",
		     @"processInfo",
		     @"archiveURL",
		     @"URL",
		     @"dataFileURL",
		     @"streamDataToLoad",
		     @"bulkDataProxy",
		     @"token",
		     @"data",
		     @"error",
		     @"requestID",
	     ]) {
		dumpObjectField(object, label, key);
	}
	dumpRuntimeClass(NSStringFromClass([object class]));
}

static void dumpTransportServices(id services, NSString *label) {
	unsigned long serviceCount = [services respondsToSelector:@selector(count)] ? (unsigned long)[services count] : 0;
	fprintf(stdout, "%s service_count=%lu services=%s\n",
	        [label UTF8String],
	        serviceCount,
	        [stringFromObject(services) UTF8String]);
	int idx = 0;
	for (id service in services) {
		dumpObjectSnapshot(service, [NSString stringWithFormat:@"%@ service[%d]", label, idx++]);
	}
}

static NSString *baseProfileMode(NSString *mode) {
	mode = [mode stringByReplacingOccurrencesOfString:@"_display_on" withString:@""];
	mode = [mode stringByReplacingOccurrencesOfString:@"_launch_cli_args" withString:@""];
	mode = [mode stringByReplacingOccurrencesOfString:@"_launch_min_env" withString:@""];
	mode = [mode stringByReplacingOccurrencesOfString:@"_observer" withString:@""];
	mode = [mode stringByReplacingOccurrencesOfString:@"_no_stream" withString:@""];
	mode = [mode stringByReplacingOccurrencesOfString:@"_flush_rpackets" withString:@""];
	mode = [mode stringByReplacingOccurrencesOfString:@"_pause_resume" withString:@""];
	mode = [mode stringByReplacingOccurrencesOfString:@"_query_perf_during" withString:@""];
	if ([mode isEqualToString:@"timeline_aps_usc_timeline_wait_complete"]) {
		return @"timeline_aps_usc_timeline_config";
	}
	if ([mode hasSuffix:@"_wait_complete"]) {
		return [mode substringToIndex:[mode length] - [@"_wait_complete" length]];
	}
	if ([mode hasSuffix:@"_run_30s"]) {
		return [mode substringToIndex:[mode length] - [@"_run_30s" length]];
	}
	if ([mode hasSuffix:@"_run_5s"]) {
		return [mode substringToIndex:[mode length] - [@"_run_5s" length]];
	}
	if ([mode hasSuffix:@"_run_10s"]) {
		return [mode substringToIndex:[mode length] - [@"_run_10s" length]];
	}
	if ([mode hasSuffix:@"_run_60s"]) {
		return [mode substringToIndex:[mode length] - [@"_run_60s" length]];
	}
	if ([mode hasSuffix:@"_run_90s"]) {
		return [mode substringToIndex:[mode length] - [@"_run_90s" length]];
	}
	return mode;
}

static uint32_t performanceStateForMode(NSString *mode) {
	if ([mode containsString:@"perf_state_3"]) {
		return 3;
	}
	if ([mode containsString:@"perf_state_5"]) {
		return 5;
	}
	if ([mode containsString:@"perf_state_8"]) {
		return 8;
	}
	return 2;
}

static id directProfileDataObject(NSString *mode) {
	uint32_t performanceState = performanceStateForMode(mode);
	NSMutableDictionary *dict = [@{
		@"profilerMode": @2,
		@"profiledProfilerMode": @2,
		@"performanceState": @(performanceState),
		@"profiledPerformanceState": @(performanceState),
		@"executionMode": @0,
		@"profiledExecutionMode": @0,
		@"supportsFileFormatV2": @YES,
		@"SupportsFileFormatV2": @YES,
		@"supportsSeparateAPSData": @YES,
		@"saveProfilerRaw": @YES,
		@"shaderProfiling": @YES,
		@"collectProfilerData": @YES,
		@"collectPerformanceTiming": @YES,
		@"collectPipelinePerformanceStatistics": @YES,
		@"gpuTimelineData": @YES,
		@"APSTraceData": @YES,
		@"APSTimelineData": @YES,
		@"APSData": @YES,
	} mutableCopy];
	if ([mode containsString:@"profiler_raw_url"] && [currentProfileOutDir length] > 0) {
		NSString *rawDir = [currentProfileOutDir stringByAppendingPathComponent:@"direct-profiledata-service-url.gpuprofiler_raw"];
		mkdir([rawDir fileSystemRepresentation], 0777);
		NSURL *rawURL = [NSURL fileURLWithPath:rawDir isDirectory:YES];
		dict[@"streamDataToLoad"] = rawURL;
		dict[@"StreamDataToLoad"] = rawURL;
		dict[@"Profiler Raw URL"] = rawURL;
		dict[@"Profiler Raw"] = @YES;
		dict[@"ProfilerRawURL"] = rawURL;
		dict[@"profilerRawURL"] = rawURL;
	}
	fprintf(stdout, "direct profileData object mode=%s class=%s keys=%s\n",
	        [mode UTF8String],
	        [stringFromObject([dict class]) UTF8String],
	        [stringFromObject([dict allKeys]) UTF8String]);
	return dict;
}

static NSData *archivedProfileDictionary(NSString *mode) {
	mode = baseProfileMode(mode);
	if ([mode containsString:@"session_fallback"]) {
		NSError *error = nil;
		NSURL *rawURL = nil;
		if ([mode containsString:@"profiler_raw_url"] && [currentProfileOutDir length] > 0) {
			NSString *rawDir = [currentProfileOutDir stringByAppendingPathComponent:@"session-fallback-service-url.gpuprofiler_raw"];
			mkdir([rawDir fileSystemRepresentation], 0777);
			rawURL = [NSURL fileURLWithPath:rawDir isDirectory:YES];
		}
		NSMutableDictionary *dict = [@{
			@"profilerMode": @2,
			@"profiledProfilerMode": @2,
			@"performanceState": @(performanceStateForMode(mode)),
			@"profiledPerformanceState": @(performanceStateForMode(mode)),
			@"executionMode": @0,
			@"profiledExecutionMode": @0,
			@"supportsFileFormatV2": @YES,
			@"SupportsFileFormatV2": @YES,
			@"supportsSeparateAPSData": @YES,
			@"saveProfilerRaw": @YES,
			@"shaderProfiling": @YES,
			@"collectProfilerData": @YES,
			@"collectPerformanceTiming": @YES,
			@"gpuTimelineData": @YES,
			@"APSTraceData": @YES,
			@"APSTimelineData": @YES,
			@"APSData": @YES,
		} mutableCopy];
		if (rawURL) {
			dict[@"streamDataToLoad"] = rawURL;
			dict[@"StreamDataToLoad"] = rawURL;
			dict[@"Profiler Raw URL"] = rawURL;
			dict[@"Profiler Raw"] = @YES;
			dict[@"ProfilerRawURL"] = rawURL;
			dict[@"profilerRawURL"] = rawURL;
		}
		NSData *data = [NSKeyedArchiver archivedDataWithRootObject:dict requiringSecureCoding:YES error:&error];
		if (!data) {
			fprintf(stdout, "archive session fallback dictionary failed: %s\n", [[error description] UTF8String]);
		}
		return data;
	}
	if ([mode containsString:@"session_request"] || [mode containsString:@"session_mode_"]) {
		return archivedShaderProfilerSessionRequest(mode);
	}
	NSError *error = nil;
	NSMutableDictionary *dict = [NSMutableDictionary dictionary];
	uint32_t performanceState = performanceStateForMode(mode);
	if ([mode isEqualToString:@"timeline_raw_v2_profile_data"]) {
		dict[@"supportsFileFormatV2"] = @YES;
		dict[@"profiledPerformanceState"] = @(performanceState);
		dict[@"profiledExecutionMode"] = @0;
	} else if ([mode isEqualToString:@"timeline_raw_v2_uppercase_profile_data"]) {
		dict[@"SupportsFileFormatV2"] = @YES;
		dict[@"supportsFileFormatV2"] = @YES;
		dict[@"profiledPerformanceState"] = @(performanceState);
		dict[@"profiledExecutionMode"] = @0;
	} else if ([mode isEqualToString:@"timeline_aps_options_all"]) {
		dict[@"APS Options"] = @"all";
		dict[@"supportsSeparateAPSData"] = @YES;
	} else if ([mode isEqualToString:@"timeline_aps_options_streaming_profile"]) {
		dict[@"APS Options"] = @"Streaming APS Profiling";
		dict[@"supportsSeparateAPSData"] = @YES;
	} else if ([mode isEqualToString:@"timeline_aps_options_streaming_counters"]) {
		dict[@"APS Options"] = @"Streaming APS Counters";
		dict[@"supportsSeparateAPSData"] = @YES;
	} else if ([mode isEqualToString:@"timeline_aps_trace_data"]) {
		dict[@"APSTraceData"] = @YES;
		dict[@"APSTraceDataFile"] = @YES;
		dict[@"APSData"] = @YES;
		dict[@"APSCounterData"] = @YES;
		dict[@"APSTimelineData"] = @YES;
		dict[@"supportsSeparateAPSData"] = @YES;
	} else if ([mode isEqualToString:@"timeline_aps_usc_timeline_config"]) {
		dict[@"APSTraceData"] = @YES;
		dict[@"APSData"] = @YES;
		dict[@"APSTimelineData"] = @YES;
		dict[@"APS_USC"] = apsConfigDictionary(@"initForTimeline");
		dict[@"supportsSeparateAPSData"] = @YES;
	} else if ([mode isEqualToString:@"timeline_aps_usc_timeline_encode_streamdata"]) {
		dict[@"APSTraceData"] = @YES;
		dict[@"APSData"] = @YES;
		dict[@"APSTimelineData"] = @YES;
		dict[@"APS_USC"] = apsConfigDictionary(@"initForTimeline");
		dict[@"supportsSeparateAPSData"] = @YES;
		dict[@"SupportsFileFormatV2"] = @YES;
		dict[@"supportsFileFormatV2"] = @YES;
	} else if ([mode isEqualToString:@"timeline_aps_usc_counters_config"]) {
		dict[@"APSTraceData"] = @YES;
		dict[@"APSCounterData"] = @YES;
		dict[@"APS_USC"] = apsConfigDictionary(@"initForCounters");
		dict[@"supportsSeparateAPSData"] = @YES;
	} else if ([mode isEqualToString:@"timeline_aps_usc_profiling_config"]) {
		dict[@"APSTraceData"] = @YES;
		dict[@"APSData"] = @YES;
		dict[@"APSCounterData"] = @YES;
		dict[@"APSTimelineData"] = @YES;
		dict[@"APS_USC"] = apsConfigDictionary(@"initForProfiling");
		dict[@"supportsSeparateAPSData"] = @YES;
	} else if ([mode isEqualToString:@"timeline_aps_usc_timeline_determination_encode_streamdata"]) {
		dict[@"APSTraceData"] = @YES;
		dict[@"APSData"] = @YES;
		dict[@"APSTimelineData"] = @YES;
		dict[@"APS_USC"] = apsConfigDictionary(@"initForTimelineConfigurationDetermination");
		dict[@"supportsSeparateAPSData"] = @YES;
		dict[@"SupportsFileFormatV2"] = @YES;
		dict[@"supportsFileFormatV2"] = @YES;
	} else if ([mode isEqualToString:@"timeline_aps_usc_profiling_determination_encode_streamdata"]) {
		dict[@"APSTraceData"] = @YES;
		dict[@"APSData"] = @YES;
		dict[@"APSCounterData"] = @YES;
		dict[@"APSTimelineData"] = @YES;
		dict[@"APS_USC"] = apsConfigDictionary(@"initForProfilingConfigurationDetermination");
		dict[@"supportsSeparateAPSData"] = @YES;
		dict[@"SupportsFileFormatV2"] = @YES;
		dict[@"supportsFileFormatV2"] = @YES;
	} else if ([mode isEqualToString:@"batch_filtered_aps_usc_counters_encode_streamdata"] ||
	           [mode isEqualToString:@"derived_aps_usc_counters_encode_streamdata"]) {
		dict[@"APSTraceData"] = @YES;
		dict[@"APSCounterData"] = @YES;
		dict[@"APS_USC"] = apsConfigDictionary(@"initForCounters");
		dict[@"supportsSeparateAPSData"] = @YES;
		dict[@"SupportsFileFormatV2"] = @YES;
		dict[@"supportsFileFormatV2"] = @YES;
	} else if ([mode isEqualToString:@"timeline_profiled_profiler_mode_0"]) {
		dict[@"profiledProfilerMode"] = @0;
		dict[@"profiledPerformanceState"] = @(performanceState);
		dict[@"profiledExecutionMode"] = @0;
		dict[@"supportsSeparateAPSData"] = @YES;
	} else if ([mode isEqualToString:@"timeline_profiled_profiler_mode_1"]) {
		dict[@"profiledProfilerMode"] = @1;
		dict[@"profiledPerformanceState"] = @(performanceState);
		dict[@"profiledExecutionMode"] = @0;
		dict[@"supportsSeparateAPSData"] = @YES;
	} else if ([mode isEqualToString:@"timeline_profiled_profiler_mode_2"]) {
		dict[@"profiledProfilerMode"] = @2;
		dict[@"profiledPerformanceState"] = @(performanceState);
		dict[@"profiledExecutionMode"] = @0;
		dict[@"supportsSeparateAPSData"] = @YES;
	} else if ([mode containsString:@"encode_streamdata"]) {
		dict[@"SupportsFileFormatV2"] = @YES;
		dict[@"supportsFileFormatV2"] = @YES;
		dict[@"profiledProfilerMode"] = @2;
		dict[@"profiledPerformanceState"] = @(performanceState);
		dict[@"profiledExecutionMode"] = @0;
		dict[@"supportsSeparateAPSData"] = @YES;
	}
	if ([mode containsString:@"analysis_flags"]) {
		dict[@"performAnalysis"] = @YES;
		dict[@"performShaderProfilingAnalysis"] = @YES;
		dict[@"perfectPatching"] = @YES;
		dict[@"waitUntilCompleteEachFrame"] = @YES;
		dict[@"collectProfilerData"] = @YES;
		dict[@"collectPerformanceTiming"] = @YES;
		dict[@"collectPipelinePerformanceStatistics"] = @YES;
		dict[@"gpuTimelineData"] = @YES;
		dict[@"collectRawCounters"] = @YES;
		dict[@"collectDerivedCounters"] = @YES;
		dict[@"testProfiling"] = @YES;
		dict[@"profileTrace"] = @YES;
		dict[@"APSTraceData"] = @YES;
		dict[@"APSTraceDataFile"] = @YES;
		dict[@"APSData"] = @YES;
		dict[@"APSCounterData"] = @YES;
		dict[@"APSTimelineData"] = @YES;
	}
	if ([mode containsString:@"profiler_raw_url"] && [currentProfileOutDir length] > 0) {
		NSString *rawDir = [currentProfileOutDir stringByAppendingPathComponent:@"service-url.gpuprofiler_raw"];
		mkdir([rawDir fileSystemRepresentation], 0777);
		dict[@"Profiler Raw URL"] = [NSURL fileURLWithPath:rawDir isDirectory:YES];
		dict[@"Profiler Raw"] = @YES;
		dict[@"Streaming Shader Profiling Data"] = @YES;
		dict[@"Streaming GPU Timeline Data"] = @YES;
		dict[@"supportsSeparateAPSData"] = @YES;
		dict[@"SupportsFileFormatV2"] = @YES;
		dict[@"supportsFileFormatV2"] = @YES;
		dict[@"APSTraceData"] = @YES;
		dict[@"APSTraceDataFile"] = @YES;
	}
	NSData *data = [NSKeyedArchiver archivedDataWithRootObject:dict requiringSecureCoding:YES error:&error];
	if (!data) {
		fprintf(stdout, "archive profileData failed: %s\n", [[error description] UTF8String]);
	}
	return data;
}

static NSData *archivedEmptyDictionary(void) {
	NSError *error = nil;
	NSData *data = [NSKeyedArchiver archivedDataWithRootObject:@{} requiringSecureCoding:YES error:&error];
	if (!data) {
		fprintf(stdout, "archive empty dictionary failed: %s\n", [[error description] UTF8String]);
	}
	return data;
}

static NSData *archivedShaderProfilerSessionRequest(NSString *mode) {
	void *replayHandle = dlopen("/System/Library/PrivateFrameworks/GPUToolsReplay.framework/GPUToolsReplay", RTLD_NOW);
	fprintf(stdout, "session request GPUToolsReplay dlopen=%p err=%s\n", replayHandle, dlerror());
	Class requestClass = NSClassFromString(@"GTShaderProfilerSessionRequest");
	if (!requestClass) {
		fprintf(stdout, "GTShaderProfilerSessionRequest missing; falling back to dictionary\n");
		return archivedEmptyDictionary();
	}
	id request = [requestClass new];
	uint32_t performanceState = performanceStateForMode(mode);
	uint32_t profilerMode = 2;
	if ([mode containsString:@"session_mode_0"]) {
		profilerMode = 0;
	} else if ([mode containsString:@"session_mode_1"]) {
		profilerMode = 1;
	}
	if ([request respondsToSelector:@selector(setProfilerMode:)]) {
		((void (*)(id, SEL, uint32_t))objc_msgSend)(request, @selector(setProfilerMode:), profilerMode);
	}
	if ([request respondsToSelector:@selector(setPerformanceState:)]) {
		((void (*)(id, SEL, uint32_t))objc_msgSend)(request, @selector(setPerformanceState:), performanceState);
	}
	if ([request respondsToSelector:@selector(setExecutionMode:)]) {
		((void (*)(id, SEL, uint32_t))objc_msgSend)(request, @selector(setExecutionMode:), 0);
	}
	NSURL *streamDataURL = nil;
	if (([mode containsString:@"streamdata_to_load"] || [mode containsString:@"profiler_raw_url"]) && [request respondsToSelector:@selector(setStreamDataToLoad:)]) {
		NSString *rawDir = [currentProfileOutDir length] > 0
			? [currentProfileOutDir stringByAppendingPathComponent:([mode containsString:@"profiler_raw_url"] ? @"session-service-url.gpuprofiler_raw" : @"session-load.gpuprofiler_raw")]
			: [NSTemporaryDirectory() stringByAppendingPathComponent:@"session-load.gpuprofiler_raw"];
		mkdir([rawDir fileSystemRepresentation], 0777);
		streamDataURL = [NSURL fileURLWithPath:rawDir isDirectory:YES];
		((void (*)(id, SEL, id))objc_msgSend)(request, @selector(setStreamDataToLoad:), streamDataURL);
		fprintf(stdout, "session request streamDataToLoad URL=%s\n", [[streamDataURL absoluteString] UTF8String]);
	}
	NSError *error = nil;
	NSData *data = [NSKeyedArchiver archivedDataWithRootObject:request requiringSecureCoding:NO error:&error];
	if (!data) {
		fprintf(stdout, "archive session request failed: %s\n", [[error description] UTF8String]);
		NSMutableDictionary *fallback = [@{
			@"profilerMode": @(profilerMode),
			@"profiledProfilerMode": @(profilerMode),
			@"performanceState": @(performanceState),
			@"profiledPerformanceState": @(performanceState),
			@"executionMode": @0,
			@"profiledExecutionMode": @0,
			@"supportsFileFormatV2": @YES,
			@"SupportsFileFormatV2": @YES,
			@"supportsSeparateAPSData": @YES,
		} mutableCopy];
		if (streamDataURL) {
			fallback[@"streamDataToLoad"] = streamDataURL;
			fallback[@"StreamDataToLoad"] = streamDataURL;
			fallback[@"Profiler Raw URL"] = streamDataURL;
			fallback[@"Profiler Raw"] = @YES;
			fallback[@"ProfilerRawURL"] = streamDataURL;
			fallback[@"profilerRawURL"] = streamDataURL;
			fallback[@"saveProfilerRaw"] = @YES;
			fallback[@"shaderProfiling"] = @YES;
			fallback[@"collectProfilerData"] = @YES;
			fallback[@"collectPerformanceTiming"] = @YES;
			fallback[@"gpuTimelineData"] = @YES;
		}
		data = [NSKeyedArchiver archivedDataWithRootObject:fallback requiringSecureCoding:YES error:&error];
		if (!data) {
			fprintf(stdout, "archive session fallback failed: %s\n", [[error description] UTF8String]);
		}
	}
	fprintf(stdout, "session request profileData mode=%s profilerMode=%u bytes=%lu\n",
	        [mode UTF8String],
	        profilerMode,
	        (unsigned long)[data length]);
	return data;
}

static id newMutableProfilerStreamData(NSString *mode) {
	void *replayHandle = dlopen("/System/Library/PrivateFrameworks/GPUToolsReplay.framework/GPUToolsReplay", RTLD_NOW);
	fprintf(stdout, "streamData GPUToolsReplay dlopen=%p err=%s\n", replayHandle, dlerror());
	Class streamDataClass = NSClassFromString(@"GTMutableShaderProfilerStreamData");
	id mutableStreamData = nil;
	if (streamDataClass && [streamDataClass instancesRespondToSelector:@selector(initWithNewFileFormatV2Support:)]) {
		id allocated = [streamDataClass alloc];
		mutableStreamData = ((id (*)(id, SEL, BOOL))objc_msgSend)(allocated, @selector(initWithNewFileFormatV2Support:), YES);
	} else if (streamDataClass) {
		mutableStreamData = [streamDataClass new];
	}
	if ([mutableStreamData respondsToSelector:@selector(setTraceName:)]) {
		[mutableStreamData setValue:@"headless-cli-probe" forKey:@"traceName"];
	}
	if ([mutableStreamData respondsToSelector:@selector(setProfiledPerformanceState:)]) {
		[mutableStreamData setValue:@(performanceStateForMode(mode)) forKey:@"profiledPerformanceState"];
	}
	if ([mutableStreamData respondsToSelector:@selector(setProfiledProfilerMode:)]) {
		[mutableStreamData setValue:@2 forKey:@"profiledProfilerMode"];
	}
	if ([mutableStreamData respondsToSelector:@selector(setProfiledExecutionMode:)]) {
		[mutableStreamData setValue:@0 forKey:@"profiledExecutionMode"];
	}
	return mutableStreamData;
}

static NSDictionary *apsConfigDictionary(NSString *initializerName) {
	void *replayHandle = dlopen("/System/Library/PrivateFrameworks/GPUToolsReplay.framework/GPUToolsReplay", RTLD_NOW);
	if (!replayHandle) {
		fprintf(stdout, "APS config dlopen failed: %s\n", dlerror());
		return @{};
	}
	Class configClass = NSClassFromString(@"GTGPUAPSConfig");
	if (!configClass) {
		fprintf(stdout, "APS config class missing\n");
		return @{};
	}
	SEL initializer = NSSelectorFromString(initializerName);
	id config = [configClass alloc];
	if ([config respondsToSelector:initializer]) {
#pragma clang diagnostic push
#pragma clang diagnostic ignored "-Warc-performSelector-leaks"
		config = [config performSelector:initializer];
#pragma clang diagnostic pop
	} else {
		config = [config init];
	}
	if ([config respondsToSelector:@selector(setBufferSizeInKb:)]) {
		[config setValue:@4096 forKey:@"bufferSizeInKb"];
	}
	if ([config respondsToSelector:@selector(setCountPeriod:)]) {
		[config setValue:@32768 forKey:@"countPeriod"];
	}
	if ([config respondsToSelector:@selector(setPulsePeriod:)]) {
		[config setValue:@2048 forKey:@"pulsePeriod"];
	}
	if ([config respondsToSelector:@selector(setDuration:)]) {
		[config setValue:@1000000000 forKey:@"duration"];
	}
	if ([config respondsToSelector:@selector(setSystemTimePeriod:)]) {
		[config setValue:@1000000 forKey:@"systemTimePeriod"];
	}
	if ([config respondsToSelector:@selector(setEmitThreadControlFlow:)]) {
		[config setValue:@YES forKey:@"emitThreadControlFlow"];
	}
	if ([config respondsToSelector:@selector(setCliqueAdvanceReason:)]) {
		[config setValue:@YES forKey:@"cliqueAdvanceReason"];
	}
	if ([config respondsToSelector:@selector(toDictionary)]) {
#pragma clang diagnostic push
#pragma clang diagnostic ignored "-Warc-performSelector-leaks"
		id dictionary = [config performSelector:@selector(toDictionary)];
#pragma clang diagnostic pop
		if ([dictionary isKindOfClass:[NSDictionary class]]) {
			fprintf(stdout, "APS config %s keys=%lu dict=%s\n",
			        [initializerName UTF8String],
			        (unsigned long)[dictionary count],
			        [[dictionary description] UTF8String]);
			return dictionary;
		}
	}
	return @{};
}

static int runQueryClassWithDispatchUID(GTMTLReplayServiceXPCProxy *replayer, NSString *outDir, NSString *className, GTReplayDispatchUID dispatchUID);
static int runQueryClass(GTMTLReplayServiceXPCProxy *replayer, NSString *outDir, NSString *className);

static int runQuery(GTMTLReplayServiceXPCProxy *replayer, NSString *outDir) {
    return runQueryClass(replayer, outDir, @"GTReplayQueryDeviceCapabilities");
}

static int runQueryClass(GTMTLReplayServiceXPCProxy *replayer, NSString *outDir, NSString *className) {
    GTReplayDispatchUID dispatchUID = {{0, -1}, 0};
    return runQueryClassWithDispatchUID(replayer, outDir, className, dispatchUID);
}

static int runQueryClassWithDispatchUID(GTMTLReplayServiceXPCProxy *replayer, NSString *outDir, NSString *className, GTReplayDispatchUID dispatchUID) {
    Class batchClass = NSClassFromString(@"GTReplayRequestBatch");
    Class queryClass = NSClassFromString(className);
    if (!batchClass || !queryClass) {
        fprintf(stderr, "missing query classes for %s\n", [className UTF8String]);
        return 30;
    }
    __block int queryResponses = 0;
    id batch = [batchClass new];
    id query = [queryClass new];
    if ([query respondsToSelector:@selector(setDispatchUID:)]) {
        [(GTReplayQueryResourceUsage *)query setDispatchUID:dispatchUID];
    }
    [batch setRequests:@[query]];
    [batch setCompletionHandler:^(id response) {
        logResponse(response, @"query");
        writeBytes(responseData(response), outDir, @"query", queryResponses++);
    }];
    id token = [replayer query:batch];
    fprintf(stdout, "query class=%s dispatchIndex=%d dispatchICBIndex=%d uid=%llu token=%s\n",
            [className UTF8String],
            dispatchUID.index.dispatchIndex,
            dispatchUID.index.dispatchICBIndex,
            dispatchUID.uid,
            [stringFromObject(token) UTF8String]);
    [[NSRunLoop currentRunLoop] runUntilDate:[NSDate dateWithTimeIntervalSinceNow:1.0]];
    return 0;
}

static int runQueryDerivedCountersEncodeStreamData(GTMTLReplayServiceXPCProxy *replayer, NSString *outDir) {
	Class batchClass = NSClassFromString(@"GTReplayRequestBatch");
	Class queryClass = NSClassFromString(@"GTReplayQueryDerivedCounters");
	if (!batchClass || !queryClass) {
		fprintf(stderr, "missing derived counter query classes\n");
		return 31;
	}
	id mutableStreamData = newMutableProfilerStreamData(@"");
	fprintf(stdout, "query derived mutable streamData=%s\n", [stringFromObject(mutableStreamData) UTF8String]);
	__block int queryResponses = 0;
	__block int addedCount = 0;
	id batch = [batchClass new];
	id query = [queryClass new];
	[batch setRequests:@[query]];
	[batch setCompletionHandler:^(id response) {
		logResponse(response, @"query-derived-streamdata");
		NSData *data = responseData(response);
		writeObject(response, outDir, @"query-derived-object", queryResponses);
		writeBytes(data, outDir, @"query-derived", queryResponses++);
		if (!data || !mutableStreamData) {
			return;
		}
		id payload = unarchivedObjectFromData(data);
		if (!payload) {
			payload = data;
		}
		payload = normalizedProfilerPayload(payload);
		BOOL added = NO;
		@try {
			if ([mutableStreamData respondsToSelector:@selector(addAPSCounterData:)]) {
				added = ((BOOL (*)(id, SEL, id))objc_msgSend)(mutableStreamData, @selector(addAPSCounterData:), payload);
			}
			if (!added && [mutableStreamData respondsToSelector:@selector(addAPSData:)]) {
				added = ((BOOL (*)(id, SEL, id))objc_msgSend)(mutableStreamData, @selector(addAPSData:), payload);
			}
		} @catch (NSException *exception) {
			fprintf(stdout, "query-derived streamData add exception=%s\n", [[exception description] UTF8String]);
		}
		if (added) {
			addedCount++;
		}
		fprintf(stdout, "query-derived streamData add bytes=%lu payloadClass=%s added=%d\n",
		        (unsigned long)[data length],
		        [stringFromObject([payload class]) UTF8String],
		        added ? 1 : 0);
	}];
	id token = [replayer query:batch];
	fprintf(stdout, "query derived encode streamData token=%s\n", [stringFromObject(token) UTF8String]);
	[[NSRunLoop currentRunLoop] runUntilDate:[NSDate dateWithTimeIntervalSinceNow:2.0]];
	if (mutableStreamData) {
		NSString *rawDir = [outDir stringByAppendingPathComponent:@"encoded.gpuprofiler_raw"];
		mkdir([rawDir fileSystemRepresentation], 0777);
		NSURL *streamURL = [NSURL fileURLWithPath:rawDir isDirectory:YES];
		NSError *encodeError = nil;
		id encoded = nil;
		if ([mutableStreamData respondsToSelector:@selector(encode:error:)]) {
			encoded = ((id (*)(id, SEL, id, NSError **))objc_msgSend)(mutableStreamData, @selector(encode:error:), streamURL, &encodeError);
		}
		NSString *streamDataPath = [rawDir stringByAppendingPathComponent:@"streamData"];
		fprintf(stdout, "query-derived streamData encode added=%d rawDir=%s encoded=%s err=%s streamData_exists=%d\n",
		        addedCount,
		        [rawDir UTF8String],
		        [stringFromObject(encoded) UTF8String],
		        [[encodeError description] UTF8String],
		        [[NSFileManager defaultManager] fileExistsAtPath:streamDataPath] ? 1 : 0);
	}
	fprintf(stdout, "query-derived responses=%d added=%d\n", queryResponses, addedCount);
	return 0;
}

static NSArray *candidateDispatchUIDStrings(int argc, char **argv) {
    if (argc < 5) {
        return @[@"0"];
    }
    NSString *csv = [NSString stringWithUTF8String:argv[4]];
    NSArray *parts = [csv componentsSeparatedByString:@","];
    return [parts count] > 0 ? parts : @[@"0"];
}

static GTReplayDispatchUID parseDispatchUID(NSString *part) {
    NSArray *fields = [part componentsSeparatedByString:@":"];
    GTReplayDispatchUID dispatchUID = {{0, -1}, 0};
    if ([fields count] == 3) {
        dispatchUID.index.dispatchIndex = (int)strtol([[fields objectAtIndex:0] UTF8String], NULL, 10);
        dispatchUID.index.dispatchICBIndex = (int)strtol([[fields objectAtIndex:1] UTF8String], NULL, 10);
        dispatchUID.uid = strtoull([[fields objectAtIndex:2] UTF8String], NULL, 10);
        return dispatchUID;
    }
    unsigned long long value = strtoull([part UTF8String], NULL, 10);
    dispatchUID.index.dispatchIndex = (int)value;
    dispatchUID.index.dispatchICBIndex = -1;
    dispatchUID.uid = value;
    return dispatchUID;
}

static int runResourceUsageCandidates(GTMTLReplayServiceXPCProxy *replayer, NSString *outDir, int argc, char **argv) {
    int rc = 0;
    int idx = 0;
    for (NSString *part in candidateDispatchUIDStrings(argc, argv)) {
        GTReplayDispatchUID dispatchUID = parseDispatchUID(part);
        NSString *candidateDir = [outDir stringByAppendingPathComponent:[NSString stringWithFormat:@"candidate-%02d-%d-%d-%llu",
                                                                          idx,
                                                                          dispatchUID.index.dispatchIndex,
                                                                          dispatchUID.index.dispatchICBIndex,
                                                                          dispatchUID.uid]];
        mkdir([candidateDir fileSystemRepresentation], 0777);
        int queryRC = runQueryClassWithDispatchUID(replayer, candidateDir, @"GTReplayQueryResourceUsage", dispatchUID);
        if (queryRC != 0 && rc == 0) {
            rc = queryRC;
        }
        idx++;
    }
    return rc;
}

static NSArray *candidateFetchStrings(int argc, char **argv) {
    if (argc < 6) {
        return @[];
    }
    NSArray *parts = [[NSString stringWithUTF8String:argv[5]] componentsSeparatedByString:@","];
    return [parts count] > 0 ? parts : @[];
}

static int runFetchPipelineBinaryCandidates(GTMTLReplayServiceXPCProxy *replayer, NSString *outDir, int argc, char **argv) {
    Class batchClass = NSClassFromString(@"GTReplayRequestBatch");
    Class fetchClass = NSClassFromString(@"GTReplayFetchPipelineBinaries");
    if (!batchClass || !fetchClass) {
        fprintf(stderr, "missing fetch pipeline classes\n");
        return 50;
    }
    int idx = 0;
    for (NSString *part in candidateFetchStrings(argc, argv)) {
        NSArray *fields = [part componentsSeparatedByString:@":"];
        if ([fields count] != 4) {
            continue;
        }
        GTReplayDispatchUID dispatchUID = {{0, -1}, 0};
        dispatchUID.index.dispatchIndex = (int)strtol([[fields objectAtIndex:0] UTF8String], NULL, 10);
        dispatchUID.index.dispatchICBIndex = (int)strtol([[fields objectAtIndex:1] UTF8String], NULL, 10);
        dispatchUID.uid = strtoull([[fields objectAtIndex:2] UTF8String], NULL, 10);
        unsigned long long streamRef = strtoull([[fields objectAtIndex:3] UTF8String], NULL, 10);
        NSString *candidateDir = [outDir stringByAppendingPathComponent:[NSString stringWithFormat:@"candidate-%02d-%d-%d-%llu-%llu",
                                                                          idx,
                                                                          dispatchUID.index.dispatchIndex,
                                                                          dispatchUID.index.dispatchICBIndex,
                                                                          dispatchUID.uid,
                                                                          streamRef]];
        mkdir([candidateDir fileSystemRepresentation], 0777);
        __block int fetchResponses = 0;
        id batch = [batchClass new];
        id request = [fetchClass new];
        [(GTReplayFetchPipelineBinaries *)request setDispatchUID:dispatchUID];
        [(GTReplayFetchPipelineBinaries *)request setStreamRef:streamRef];
        [batch setRequests:@[request]];
        [batch setCompletionHandler:^(id response) {
            logResponse(response, @"fetch");
            writeBytes(responseData(response), candidateDir, @"fetch", fetchResponses++);
        }];
        id token = [replayer fetch:batch];
        fprintf(stdout, "fetch pipeline dispatchIndex=%d dispatchICBIndex=%d uid=%llu streamRef=%llu token=%s\n",
                dispatchUID.index.dispatchIndex,
                dispatchUID.index.dispatchICBIndex,
                dispatchUID.uid,
                streamRef,
                [stringFromObject(token) UTF8String]);
        [[NSRunLoop currentRunLoop] runUntilDate:[NSDate dateWithTimeIntervalSinceNow:1.0]];
        idx++;
    }
    return 0;
}

static id newAuxiliaryFetchRequest(NSString *className, GTReplayDispatchUID dispatchUID) {
	Class fetchClass = NSClassFromString(className);
	if (!fetchClass) {
		return nil;
	}
	id request = [fetchClass new];
	if ([request respondsToSelector:@selector(setDispatchUID:)]) {
		((void (*)(id, SEL, GTReplayDispatchUID))objc_msgSend)(request, @selector(setDispatchUID:), dispatchUID);
	}
	if ([className isEqualToString:@"GTReplayFetchThreadgroup"] && [request respondsToSelector:@selector(setIndex:)]) {
		[(GTReplayFetchThreadgroup *)request setIndex:0];
	}
	if ([className isEqualToString:@"GTReplayFetchPostVertex"]) {
		GTReplayPoint3D begin = {0, 0, 0};
		GTReplayPoint3D end = {1, 1, 1};
		[(GTReplayFetchPostVertex *)request setObjectShaderThreadgroupBoundsPresent:YES];
		[(GTReplayFetchPostVertex *)request setObjectShaderThreadgroupBeginBounds:begin];
		[(GTReplayFetchPostVertex *)request setObjectShaderThreadgroupEndBounds:end];
	}
	if ([className isEqualToString:@"GTReplayFetchWireframe"] && [request respondsToSelector:@selector(setSolid:)]) {
		[(GTReplayFetchWireframe *)request setSolid:NO];
	}
	return request;
}

static BOOL tryAddPayloadToMutableStreamData(id mutableStreamData, id payload, NSString *label) {
	if (!mutableStreamData || !payload) {
		return NO;
	}
	SEL selectors[] = {
		@selector(addAPSTimelineData:),
		@selector(addGPUTimelineData:),
		@selector(addShaderProfilerData:),
		@selector(addBatchIdFilteredCounterData:),
		@selector(addAPSCounterData:),
		@selector(addAPSData:),
	};
	const char *names[] = {
		"addAPSTimelineData",
		"addGPUTimelineData",
		"addShaderProfilerData",
		"addBatchIdFilteredCounterData",
		"addAPSCounterData",
		"addAPSData",
	};
	BOOL anyAdded = NO;
	for (size_t i = 0; i < sizeof(selectors) / sizeof(selectors[0]); i++) {
		if (![mutableStreamData respondsToSelector:selectors[i]]) {
			continue;
		}
		BOOL added = NO;
		@try {
			added = ((BOOL (*)(id, SEL, id))objc_msgSend)(mutableStreamData, selectors[i], payload);
		} @catch (NSException *exception) {
			fprintf(stdout, "%s %s exception=%s\n",
			        [label UTF8String],
			        names[i],
			        [[exception description] UTF8String]);
		}
		fprintf(stdout, "%s %s payloadClass=%s added=%d\n",
		        [label UTF8String],
		        names[i],
		        [stringFromObject([payload class]) UTF8String],
		        added ? 1 : 0);
		anyAdded = anyAdded || added;
	}
	return anyAdded;
}

static BOOL tryAddPayloadToMutableStreamDataWithSelector(id mutableStreamData, id payload, NSString *label, SEL selector) {
	if (!mutableStreamData || !payload || !selector || ![mutableStreamData respondsToSelector:selector]) {
		return NO;
	}
	fprintf(stdout, "%s %s begin payloadClass=%s\n",
	        [label UTF8String],
	        sel_getName(selector),
	        [stringFromObject([payload class]) UTF8String]);
	BOOL added = NO;
	@try {
		added = ((BOOL (*)(id, SEL, id))objc_msgSend)(mutableStreamData, selector, payload);
	} @catch (NSException *exception) {
		fprintf(stdout, "%s %s exception=%s\n",
		        [label UTF8String],
		        sel_getName(selector),
		        [[exception description] UTF8String]);
	}
	fprintf(stdout, "%s %s payloadClass=%s added=%d\n",
	        [label UTF8String],
	        sel_getName(selector),
	        [stringFromObject([payload class]) UTF8String],
	        added ? 1 : 0);
	return added;
}

static int runAuxiliaryFetchCandidates(GTMTLReplayServiceXPCProxy *replayer, NSString *outDir, int argc, char **argv, NSString *className, NSString *label, BOOL waitComplete, id ingestStreamData) {
	Class batchClass = NSClassFromString(@"GTReplayRequestBatch");
	Class fetchClass = NSClassFromString(className);
	if (!batchClass || !fetchClass || ![replayer respondsToSelector:@selector(fetch:)]) {
		fprintf(stderr, "missing auxiliary fetch class=%s batch=%p fetchClass=%p responds=%d\n",
		        [className UTF8String],
		        batchClass,
		        fetchClass,
		        [replayer respondsToSelector:@selector(fetch:)] ? 1 : 0);
		return 58;
	}
	int idx = 0;
	for (NSString *part in candidateDispatchUIDStrings(argc, argv)) {
		if (idx >= GTProbeAuxFetchCandidateLimit) {
			fprintf(stdout, "%s candidate limit reached=%d\n", [label UTF8String], GTProbeAuxFetchCandidateLimit);
			break;
		}
		GTReplayDispatchUID dispatchUID = parseDispatchUID(part);
		NSString *candidateDir = [outDir stringByAppendingPathComponent:[NSString stringWithFormat:@"candidate-%02d-%d-%d-%llu",
		                                                                  idx,
		                                                                  dispatchUID.index.dispatchIndex,
		                                                                  dispatchUID.index.dispatchICBIndex,
		                                                                  dispatchUID.uid]];
		mkdir([candidateDir fileSystemRepresentation], 0777);
		__block int responses = 0;
		id batch = [batchClass new];
		id request = newAuxiliaryFetchRequest(className, dispatchUID);
		if (!request) {
			fprintf(stderr, "failed to create auxiliary fetch request class=%s\n", [className UTF8String]);
			if (idx == 0) {
				return 59;
			}
			idx++;
			continue;
		}
		[batch setRequests:@[request]];
		[batch setCompletionHandler:^(id response) {
			logResponse(response, label);
			NSData *data = responseData(response);
			if (ingestStreamData && data) {
				id payload = unarchivedObjectFromData(data);
				if (!payload) {
					payload = data;
				}
				payload = normalizedProfilerPayload(payload);
				BOOL added = tryAddPayloadToMutableStreamData(ingestStreamData, payload, [label stringByAppendingString:@"-ingest"]);
				fprintf(stdout, "%s ingest bytes=%lu payloadClass=%s added=%d\n",
				        [label UTF8String],
				        (unsigned long)[data length],
				        [stringFromObject([payload class]) UTF8String],
				        added ? 1 : 0);
			}
			writeObject(response, candidateDir, [label stringByAppendingString:@"-object"], responses);
			writeBytes(data, candidateDir, label, responses++);
		}];
		id token = [replayer fetch:batch];
		fprintf(stdout, "%s dispatchIndex=%d dispatchICBIndex=%d uid=%llu request=%s token=%s\n",
		        [label UTF8String],
		        dispatchUID.index.dispatchIndex,
		        dispatchUID.index.dispatchICBIndex,
		        dispatchUID.uid,
		        [stringFromObject(request) UTF8String],
		        [stringFromObject(token) UTF8String]);
		[[NSRunLoop currentRunLoop] runUntilDate:[NSDate dateWithTimeIntervalSinceNow:(waitComplete ? 0.1 : 1.0)]];
		if (waitComplete && [token respondsToSelector:@selector(waitUntilCompleted)]) {
			fprintf(stdout, "%s token waitUntilCompleted begin candidate=%d\n", [label UTF8String], idx);
#pragma clang diagnostic push
#pragma clang diagnostic ignored "-Warc-performSelector-leaks"
			[token performSelector:@selector(waitUntilCompleted)];
#pragma clang diagnostic pop
			fprintf(stdout, "%s token waitUntilCompleted done candidate=%d responses=%d\n", [label UTF8String], idx, responses);
			[[NSRunLoop currentRunLoop] runUntilDate:[NSDate dateWithTimeIntervalSinceNow:0.1]];
		}
		idx++;
	}
	return 0;
}

static int runFetchTextureCandidates(GTMTLReplayServiceXPCProxy *replayer, NSString *outDir, int argc, char **argv, BOOL waitComplete) {
	Class batchClass = NSClassFromString(@"GTReplayRequestBatch");
	Class fetchClass = NSClassFromString(@"GTReplayFetchTexture");
	if (!batchClass || !fetchClass || ![replayer respondsToSelector:@selector(fetch:)]) {
		fprintf(stderr, "missing fetch texture classes batch=%p fetchTexture=%p responds=%d\n",
		        batchClass,
		        fetchClass,
		        [replayer respondsToSelector:@selector(fetch:)] ? 1 : 0);
		return 53;
	}
	int idx = 0;
	for (NSString *part in candidateFetchStrings(argc, argv)) {
		NSArray *fields = [part componentsSeparatedByString:@":"];
		if ([fields count] != 4) {
			continue;
		}
		GTReplayDispatchUID dispatchUID = {{0, -1}, 0};
		dispatchUID.index.dispatchIndex = (int)strtol([[fields objectAtIndex:0] UTF8String], NULL, 10);
		dispatchUID.index.dispatchICBIndex = (int)strtol([[fields objectAtIndex:1] UTF8String], NULL, 10);
		dispatchUID.uid = strtoull([[fields objectAtIndex:2] UTF8String], NULL, 10);
		unsigned long long streamRef = strtoull([[fields objectAtIndex:3] UTF8String], NULL, 10);
		NSString *candidateDir = [outDir stringByAppendingPathComponent:[NSString stringWithFormat:@"candidate-%02d-%d-%d-%llu-%llu",
		                                                                  idx,
		                                                                  dispatchUID.index.dispatchIndex,
		                                                                  dispatchUID.index.dispatchICBIndex,
		                                                                  dispatchUID.uid,
		                                                                  streamRef]];
		mkdir([candidateDir fileSystemRepresentation], 0777);
		__block int responses = 0;
		id batch = [batchClass new];
		id request = [fetchClass new];
		GTReplaySize3D size = {1, 1, 1};
		GTReplayPoint3D origin = {0, 0, 0};
		GTReplayRegion3D region = {origin, size};
		[(GTReplayFetchTexture *)request setDispatchUID:dispatchUID];
		[(GTReplayFetchTexture *)request setStreamRef:streamRef];
		[(GTReplayFetchTexture *)request setSize:size];
		[(GTReplayFetchTexture *)request setRegion:region];
		[batch setRequests:@[request]];
		[batch setCompletionHandler:^(id response) {
			logResponse(response, @"fetch-texture");
			writeObject(response, candidateDir, @"fetch-texture-object", responses);
			writeBytes(responseData(response), candidateDir, @"fetch-texture", responses++);
		}];
		id token = [replayer fetch:batch];
		fprintf(stdout, "fetch texture dispatchIndex=%d dispatchICBIndex=%d uid=%llu streamRef=%llu token=%s\n",
		        dispatchUID.index.dispatchIndex,
		        dispatchUID.index.dispatchICBIndex,
		        dispatchUID.uid,
		        streamRef,
		        [stringFromObject(token) UTF8String]);
		[[NSRunLoop currentRunLoop] runUntilDate:[NSDate dateWithTimeIntervalSinceNow:(waitComplete ? 0.1 : 1.0)]];
		if (waitComplete && [token respondsToSelector:@selector(waitUntilCompleted)]) {
			fprintf(stdout, "fetchTexture token waitUntilCompleted begin candidate=%d\n", idx);
#pragma clang diagnostic push
#pragma clang diagnostic ignored "-Warc-performSelector-leaks"
			[token performSelector:@selector(waitUntilCompleted)];
#pragma clang diagnostic pop
			fprintf(stdout, "fetchTexture token waitUntilCompleted done candidate=%d responses=%d\n", idx, responses);
			[[NSRunLoop currentRunLoop] runUntilDate:[NSDate dateWithTimeIntervalSinceNow:0.1]];
		}
		idx++;
	}
	return 0;
}

static int runFetchBufferCandidates(GTMTLReplayServiceXPCProxy *replayer, NSString *outDir, int argc, char **argv, BOOL waitComplete, id ingestStreamData) {
	Class batchClass = NSClassFromString(@"GTReplayRequestBatch");
	Class fetchClass = NSClassFromString(@"GTReplayFetchBuffer");
	if (!batchClass || !fetchClass || ![replayer respondsToSelector:@selector(fetch:)]) {
		fprintf(stderr, "missing fetch buffer classes batch=%p fetchBuffer=%p responds=%d\n",
		        batchClass,
		        fetchClass,
		        [replayer respondsToSelector:@selector(fetch:)] ? 1 : 0);
		return 61;
	}
	int idx = 0;
	for (NSString *part in candidateFetchStrings(argc, argv)) {
		if (idx >= GTProbeAuxFetchCandidateLimit) {
			fprintf(stdout, "fetch-buffer candidate limit reached=%d\n", GTProbeAuxFetchCandidateLimit);
			break;
		}
		NSArray *fields = [part componentsSeparatedByString:@":"];
		if ([fields count] != 4) {
			continue;
		}
		GTReplayDispatchUID dispatchUID = {{0, -1}, 0};
		dispatchUID.index.dispatchIndex = (int)strtol([[fields objectAtIndex:0] UTF8String], NULL, 10);
		dispatchUID.index.dispatchICBIndex = (int)strtol([[fields objectAtIndex:1] UTF8String], NULL, 10);
		dispatchUID.uid = strtoull([[fields objectAtIndex:2] UTF8String], NULL, 10);
		unsigned long long streamRef = strtoull([[fields objectAtIndex:3] UTF8String], NULL, 10);
		NSString *candidateDir = [outDir stringByAppendingPathComponent:[NSString stringWithFormat:@"candidate-%02d-%d-%d-%llu-%llu",
		                                                                  idx,
		                                                                  dispatchUID.index.dispatchIndex,
		                                                                  dispatchUID.index.dispatchICBIndex,
		                                                                  dispatchUID.uid,
		                                                                  streamRef]];
		mkdir([candidateDir fileSystemRepresentation], 0777);
		__block int responses = 0;
		id batch = [batchClass new];
		id request = [fetchClass new];
		GTRange range = {0, 256};
		[(GTReplayFetchBuffer *)request setDispatchUID:dispatchUID];
		[(GTReplayFetchBuffer *)request setStreamRef:streamRef];
		if ([request respondsToSelector:@selector(setRange:)]) {
			[(GTReplayFetchBuffer *)request setRange:range];
		}
		[batch setRequests:@[request]];
		[batch setCompletionHandler:^(id response) {
			logResponse(response, @"fetch-buffer");
			NSData *data = responseData(response);
			if (ingestStreamData && data) {
				id payload = unarchivedObjectFromData(data);
				if (!payload) {
					payload = data;
				}
				payload = normalizedProfilerPayload(payload);
				BOOL added = tryAddPayloadToMutableStreamData(ingestStreamData, payload, @"fetch-buffer-ingest");
				fprintf(stdout, "fetch-buffer ingest bytes=%lu payloadClass=%s added=%d\n",
				        (unsigned long)[data length],
				        [stringFromObject([payload class]) UTF8String],
				        added ? 1 : 0);
			}
			writeObject(response, candidateDir, @"fetch-buffer-object", responses);
			writeBytes(data, candidateDir, @"fetch-buffer", responses++);
		}];
		id token = [replayer fetch:batch];
		fprintf(stdout, "fetch buffer dispatchIndex=%d dispatchICBIndex=%d uid=%llu streamRef=%llu range=%llu:%llu token=%s\n",
		        dispatchUID.index.dispatchIndex,
		        dispatchUID.index.dispatchICBIndex,
		        dispatchUID.uid,
		        streamRef,
		        (unsigned long long)range.location,
		        (unsigned long long)range.length,
		        [stringFromObject(token) UTF8String]);
		[[NSRunLoop currentRunLoop] runUntilDate:[NSDate dateWithTimeIntervalSinceNow:(waitComplete ? 0.1 : 1.0)]];
		if (waitComplete && [token respondsToSelector:@selector(waitUntilCompleted)]) {
			fprintf(stdout, "fetchBuffer token waitUntilCompleted begin candidate=%d\n", idx);
#pragma clang diagnostic push
#pragma clang diagnostic ignored "-Warc-performSelector-leaks"
			[token performSelector:@selector(waitUntilCompleted)];
#pragma clang diagnostic pop
			fprintf(stdout, "fetchBuffer token waitUntilCompleted done candidate=%d responses=%d\n", idx, responses);
			[[NSRunLoop currentRunLoop] runUntilDate:[NSDate dateWithTimeIntervalSinceNow:0.1]];
		}
		idx++;
	}
	return 0;
}

static id<MTLTexture> newProbeTexture(void) {
	id<MTLDevice> device = MTLCreateSystemDefaultDevice();
	if (!device) {
		fprintf(stdout, "fetchInto no Metal device\n");
		return nil;
	}
	MTLTextureDescriptor *descriptor = [MTLTextureDescriptor texture2DDescriptorWithPixelFormat:MTLPixelFormatRGBA8Unorm
	                                                                                      width:4
	                                                                                     height:4
	                                                                                  mipmapped:NO];
	descriptor.usage = MTLTextureUsageShaderRead | MTLTextureUsageShaderWrite | MTLTextureUsageRenderTarget;
	descriptor.storageMode = MTLStorageModePrivate;
	return [device newTextureWithDescriptor:descriptor];
}

static id<MTLSharedEvent> newProbeSharedEvent(void) {
	id<MTLDevice> device = MTLCreateSystemDefaultDevice();
	if (!device || ![device respondsToSelector:@selector(newSharedEvent)]) {
		return nil;
	}
	return [device newSharedEvent];
}

static int runFetchIntoTextureCandidates(GTMTLReplayServiceXPCProxy *replayer, NSString *outDir, int argc, char **argv, BOOL waitComplete) {
	Class batchClass = NSClassFromString(@"GTReplayRequestBatch");
	Class fetchIntoClass = NSClassFromString(@"GTReplayFetchIntoTexture");
	if (!batchClass || !fetchIntoClass || ![replayer respondsToSelector:@selector(fetchInto:)]) {
		fprintf(stderr, "missing fetchInto texture classes batch=%p fetchInto=%p responds=%d\n",
		        batchClass,
		        fetchIntoClass,
		        [replayer respondsToSelector:@selector(fetchInto:)] ? 1 : 0);
		return 54;
	}
	int idx = 0;
	for (NSString *part in candidateFetchStrings(argc, argv)) {
		NSArray *fields = [part componentsSeparatedByString:@":"];
		if ([fields count] != 4) {
			continue;
		}
		GTReplayDispatchUID dispatchUID = {{0, -1}, 0};
		dispatchUID.index.dispatchIndex = (int)strtol([[fields objectAtIndex:0] UTF8String], NULL, 10);
		dispatchUID.index.dispatchICBIndex = (int)strtol([[fields objectAtIndex:1] UTF8String], NULL, 10);
		dispatchUID.uid = strtoull([[fields objectAtIndex:2] UTF8String], NULL, 10);
		unsigned long long streamRef = strtoull([[fields objectAtIndex:3] UTF8String], NULL, 10);
		NSString *candidateDir = [outDir stringByAppendingPathComponent:[NSString stringWithFormat:@"candidate-%02d-%d-%d-%llu-%llu",
		                                                                  idx,
		                                                                  dispatchUID.index.dispatchIndex,
		                                                                  dispatchUID.index.dispatchICBIndex,
		                                                                  dispatchUID.uid,
		                                                                  streamRef]];
		mkdir([candidateDir fileSystemRepresentation], 0777);
		id<MTLTexture> texture = newProbeTexture();
		id<MTLSharedEvent> event = newProbeSharedEvent();
		if (!texture) {
			fprintf(stdout, "fetchInto candidate=%d no texture\n", idx);
			idx++;
			continue;
		}
		__block int responses = 0;
		id batch = [batchClass new];
		id request = [fetchIntoClass new];
		[(GTReplayFetchIntoTexture *)request setDispatchUID:dispatchUID];
		[(GTReplayFetchIntoTexture *)request setStreamRef:streamRef];
		[(GTReplayFetchIntoTexture *)request setDest:texture];
		if (event && [request respondsToSelector:@selector(setEvent:)]) {
			[(GTReplayFetchIntoTexture *)request setEvent:event];
		}
		[batch setRequests:@[request]];
		[batch setCompletionHandler:^(id response) {
			logResponse(response, @"fetch-into");
			writeObject(response, candidateDir, @"fetch-into-object", responses);
			writeBytes(responseData(response), candidateDir, @"fetch-into", responses++);
		}];
		id token = [replayer fetchInto:batch];
		fprintf(stdout, "fetchInto texture dispatchIndex=%d dispatchICBIndex=%d uid=%llu streamRef=%llu texture=%s event=%s token=%s\n",
		        dispatchUID.index.dispatchIndex,
		        dispatchUID.index.dispatchICBIndex,
		        dispatchUID.uid,
		        streamRef,
		        [stringFromObject(texture) UTF8String],
		        [stringFromObject(event) UTF8String],
		        [stringFromObject(token) UTF8String]);
		NSTimeInterval fetchPollSeconds = waitComplete ? 0.1 : 1.0;
		[[NSRunLoop currentRunLoop] runUntilDate:[NSDate dateWithTimeIntervalSinceNow:fetchPollSeconds]];
		if (waitComplete && [token respondsToSelector:@selector(waitUntilCompleted)]) {
			fprintf(stdout, "fetchInto token waitUntilCompleted begin candidate=%d\n", idx);
#pragma clang diagnostic push
#pragma clang diagnostic ignored "-Warc-performSelector-leaks"
			[token performSelector:@selector(waitUntilCompleted)];
#pragma clang diagnostic pop
			fprintf(stdout, "fetchInto token waitUntilCompleted done candidate=%d responses=%d\n", idx, responses);
			[[NSRunLoop currentRunLoop] runUntilDate:[NSDate dateWithTimeIntervalSinceNow:0.1]];
		}
		idx++;
	}
	return 0;
}

static void encodeMutableStreamData(id mutableStreamData, NSString *outDir, NSString *label) {
	if (!mutableStreamData) {
		return;
	}
	NSString *rawDir = [outDir stringByAppendingPathComponent:@"encoded.gpuprofiler_raw"];
	mkdir([rawDir fileSystemRepresentation], 0777);
	NSURL *streamURL = [NSURL fileURLWithPath:rawDir isDirectory:YES];
	NSError *encodeError = nil;
	id encoded = nil;
	if ([mutableStreamData respondsToSelector:@selector(encode:error:)]) {
		encoded = ((id (*)(id, SEL, id, NSError **))objc_msgSend)(mutableStreamData, @selector(encode:error:), streamURL, &encodeError);
	}
	NSString *streamDataPath = [rawDir stringByAppendingPathComponent:@"streamData"];
	fprintf(stdout, "%s streamData encode rawDir=%s encoded=%s err=%s streamData_exists=%d\n",
	        [label UTF8String],
	        [rawDir UTF8String],
	        [stringFromObject(encoded) UTF8String],
	        [[encodeError description] UTF8String],
	        [[NSFileManager defaultManager] fileExistsAtPath:streamDataPath] ? 1 : 0);
}

static int runProfileDuringFetchIntoTextureCandidates(GTMTLReplayServiceXPCProxy *replayer, NSString *outDir, int argc, char **argv, BOOL waitComplete) {
	currentProfileOutDir = outDir;
	NSString *profileMode = @"timeline_encode_streamdata";
	if ([outDir containsString:@"session_request"]) {
		profileMode = @"timeline_session_request_encode_streamdata";
	}
	id request = newProfileRequest(profileMode);
	id mutableStreamData = newMutableProfilerStreamData(profileMode);
	if (!request || !mutableStreamData) {
		fprintf(stderr, "missing profile-during-fetch request=%p streamData=%p\n", request, mutableStreamData);
		return 55;
	}
	__block int streamResponses = 0;
	if ([request respondsToSelector:@selector(setStreamHandler:)]) {
		[(GTReplayProfileRequest *)request setStreamHandler:^(id response) {
			logResponse(response, @"profile-during-fetch-stream");
			NSData *data = responseData(response);
			if (data) {
				id payload = unarchivedObjectFromData(data);
				if (!payload) {
					payload = data;
				}
				payload = normalizedProfilerPayload(payload);
				BOOL added = NO;
				@try {
					if ([mutableStreamData respondsToSelector:@selector(addAPSTimelineData:)]) {
						added = ((BOOL (*)(id, SEL, id))objc_msgSend)(mutableStreamData, @selector(addAPSTimelineData:), payload);
					}
					if (!added && [mutableStreamData respondsToSelector:@selector(addAPSData:)]) {
						added = ((BOOL (*)(id, SEL, id))objc_msgSend)(mutableStreamData, @selector(addAPSData:), payload);
					}
				} @catch (NSException *exception) {
					fprintf(stdout, "profile-during-fetch streamData add exception=%s\n", [[exception description] UTF8String]);
				}
				fprintf(stdout, "profile-during-fetch streamData add bytes=%lu payloadClass=%s added=%d\n",
				        (unsigned long)[data length],
				        [stringFromObject([payload class]) UTF8String],
				        added ? 1 : 0);
			}
			writeObject(response, outDir, @"profile-during-fetch-stream", streamResponses);
			writeBytes(data, outDir, @"profile-during-fetch-stream", streamResponses++);
		}];
	}
	id profileToken = [replayer profile:request];
	fprintf(stdout, "profile-during-fetch profile token=%s\n", [stringFromObject(profileToken) UTF8String]);
	[[NSRunLoop currentRunLoop] runUntilDate:[NSDate dateWithTimeIntervalSinceNow:0.5]];
	int fetchRC = runFetchIntoTextureCandidates(replayer, outDir, argc, argv, waitComplete);
	[[NSRunLoop currentRunLoop] runUntilDate:[NSDate dateWithTimeIntervalSinceNow:(waitComplete ? 0.5 : 3.0)]];
	if ([profileToken respondsToSelector:@selector(cancel)]) {
		BOOL cancelled = boolValueOrNo(profileToken, @"cancel");
		fprintf(stdout, "profile-during-fetch profile token cancel=%d\n", cancelled ? 1 : 0);
	}
	encodeMutableStreamData(mutableStreamData, outDir, @"profile-during-fetch");
	fprintf(stdout, "profile-during-fetch stream responses=%d fetchRC=%d\n", streamResponses, fetchRC);
	return fetchRC;
}

static int runProfileDuringFetchTextureCandidates(GTMTLReplayServiceXPCProxy *replayer, NSString *outDir, int argc, char **argv, BOOL waitComplete) {
	currentProfileOutDir = outDir;
	NSString *profileMode = @"timeline_encode_streamdata";
	id request = newProfileRequest(profileMode);
	id mutableStreamData = newMutableProfilerStreamData(profileMode);
	if (!request || !mutableStreamData) {
		fprintf(stderr, "missing profile-during-fetchTexture request=%p streamData=%p\n", request, mutableStreamData);
		return 56;
	}
	__block int streamResponses = 0;
	if ([request respondsToSelector:@selector(setStreamHandler:)]) {
		[(GTReplayProfileRequest *)request setStreamHandler:^(id response) {
			logResponse(response, @"profile-during-fetch-texture-stream");
			NSData *data = responseData(response);
			if (data) {
				id payload = unarchivedObjectFromData(data);
				if (!payload) {
					payload = data;
				}
				payload = normalizedProfilerPayload(payload);
				BOOL added = NO;
				@try {
					if ([mutableStreamData respondsToSelector:@selector(addAPSTimelineData:)]) {
						added = ((BOOL (*)(id, SEL, id))objc_msgSend)(mutableStreamData, @selector(addAPSTimelineData:), payload);
					}
					if (!added && [mutableStreamData respondsToSelector:@selector(addAPSData:)]) {
						added = ((BOOL (*)(id, SEL, id))objc_msgSend)(mutableStreamData, @selector(addAPSData:), payload);
					}
				} @catch (NSException *exception) {
					fprintf(stdout, "profile-during-fetchTexture streamData add exception=%s\n", [[exception description] UTF8String]);
				}
				fprintf(stdout, "profile-during-fetchTexture streamData add bytes=%lu payloadClass=%s added=%d\n",
				        (unsigned long)[data length],
				        [stringFromObject([payload class]) UTF8String],
				        added ? 1 : 0);
			}
			writeObject(response, outDir, @"profile-during-fetch-texture-stream", streamResponses);
			writeBytes(data, outDir, @"profile-during-fetch-texture-stream", streamResponses++);
		}];
	}
	id profileToken = [replayer profile:request];
	fprintf(stdout, "profile-during-fetchTexture profile token=%s\n", [stringFromObject(profileToken) UTF8String]);
	[[NSRunLoop currentRunLoop] runUntilDate:[NSDate dateWithTimeIntervalSinceNow:0.5]];
	int fetchRC = runFetchTextureCandidates(replayer, outDir, argc, argv, waitComplete);
	[[NSRunLoop currentRunLoop] runUntilDate:[NSDate dateWithTimeIntervalSinceNow:(waitComplete ? 0.5 : 3.0)]];
	if ([profileToken respondsToSelector:@selector(cancel)]) {
		BOOL cancelled = boolValueOrNo(profileToken, @"cancel");
		fprintf(stdout, "profile-during-fetchTexture profile token cancel=%d\n", cancelled ? 1 : 0);
	}
	encodeMutableStreamData(mutableStreamData, outDir, @"profile-during-fetchTexture");
	fprintf(stdout, "profile-during-fetchTexture stream responses=%d fetchRC=%d\n", streamResponses, fetchRC);
	return fetchRC;
}

static int runProfileDuringFetchBufferCandidates(GTMTLReplayServiceXPCProxy *replayer, NSString *outDir, int argc, char **argv, BOOL waitComplete) {
	currentProfileOutDir = outDir;
	NSString *profileMode = @"timeline_encode_streamdata";
	id request = newProfileRequest(profileMode);
	id mutableStreamData = newMutableProfilerStreamData(profileMode);
	if (!request || !mutableStreamData) {
		fprintf(stderr, "missing profile-during-fetchBuffer request=%p streamData=%p\n", request, mutableStreamData);
		return 62;
	}
	__block int streamResponses = 0;
	if ([request respondsToSelector:@selector(setStreamHandler:)]) {
		[(GTReplayProfileRequest *)request setStreamHandler:^(id response) {
			logResponse(response, @"profile-during-fetch-buffer-stream");
			NSData *data = responseData(response);
			if (data) {
				id payload = unarchivedObjectFromData(data);
				if (!payload) {
					payload = data;
				}
				payload = normalizedProfilerPayload(payload);
				BOOL added = tryAddPayloadToMutableStreamData(mutableStreamData, payload, @"profile-during-fetchBuffer-streamData");
				fprintf(stdout, "profile-during-fetchBuffer streamData add bytes=%lu payloadClass=%s added=%d\n",
				        (unsigned long)[data length],
				        [stringFromObject([payload class]) UTF8String],
				        added ? 1 : 0);
			}
			writeObject(response, outDir, @"profile-during-fetch-buffer-stream", streamResponses);
			writeBytes(data, outDir, @"profile-during-fetch-buffer-stream", streamResponses++);
		}];
	}
	id profileToken = [replayer profile:request];
	fprintf(stdout, "profile-during-fetchBuffer profile token=%s\n", [stringFromObject(profileToken) UTF8String]);
	[[NSRunLoop currentRunLoop] runUntilDate:[NSDate dateWithTimeIntervalSinceNow:0.5]];
	BOOL ingestFetchPayloads = [outDir containsString:@"ingest_fetch_payloads"];
	int fetchRC = runFetchBufferCandidates(replayer, outDir, argc, argv, waitComplete, ingestFetchPayloads ? mutableStreamData : nil);
	[[NSRunLoop currentRunLoop] runUntilDate:[NSDate dateWithTimeIntervalSinceNow:(waitComplete ? 0.5 : 3.0)]];
	if ([profileToken respondsToSelector:@selector(cancel)]) {
		BOOL cancelled = boolValueOrNo(profileToken, @"cancel");
		fprintf(stdout, "profile-during-fetchBuffer profile token cancel=%d\n", cancelled ? 1 : 0);
	}
	encodeMutableStreamData(mutableStreamData, outDir, @"profile-during-fetchBuffer");
	fprintf(stdout, "profile-during-fetchBuffer stream responses=%d fetchRC=%d\n", streamResponses, fetchRC);
	return fetchRC;
}

static int runProfileDuringAuxiliaryFetchCandidates(GTMTLReplayServiceXPCProxy *replayer, NSString *outDir, int argc, char **argv, NSString *className, NSString *label) {
	currentProfileOutDir = outDir;
	NSString *profileMode = @"timeline_encode_streamdata";
	id request = newProfileRequest(profileMode);
	id mutableStreamData = newMutableProfilerStreamData(profileMode);
	if (!request || !mutableStreamData) {
		fprintf(stderr, "missing profile-during-%s request=%p streamData=%p\n", [label UTF8String], request, mutableStreamData);
		return 60;
	}
	__block int streamResponses = 0;
	if ([request respondsToSelector:@selector(setStreamHandler:)]) {
		[(GTReplayProfileRequest *)request setStreamHandler:^(id response) {
			logResponse(response, [@"profile-during-" stringByAppendingString:label]);
			NSData *data = responseData(response);
			if (data) {
				id payload = unarchivedObjectFromData(data);
				if (!payload) {
					payload = data;
				}
				payload = normalizedProfilerPayload(payload);
				BOOL added = NO;
				@try {
					if ([mutableStreamData respondsToSelector:@selector(addAPSTimelineData:)]) {
						added = ((BOOL (*)(id, SEL, id))objc_msgSend)(mutableStreamData, @selector(addAPSTimelineData:), payload);
					}
					if (!added && [mutableStreamData respondsToSelector:@selector(addAPSData:)]) {
						added = ((BOOL (*)(id, SEL, id))objc_msgSend)(mutableStreamData, @selector(addAPSData:), payload);
					}
				} @catch (NSException *exception) {
					fprintf(stdout, "profile-during-%s streamData add exception=%s\n",
					        [label UTF8String],
					        [[exception description] UTF8String]);
				}
				fprintf(stdout, "profile-during-%s streamData add bytes=%lu payloadClass=%s added=%d\n",
				        [label UTF8String],
				        (unsigned long)[data length],
				        [stringFromObject([payload class]) UTF8String],
				        added ? 1 : 0);
			}
			writeObject(response, outDir, [@"profile-during-" stringByAppendingString:label], streamResponses);
			writeBytes(data, outDir, [@"profile-during-" stringByAppendingString:label], streamResponses++);
		}];
	}
	id profileToken = [replayer profile:request];
	fprintf(stdout, "profile-during-%s profile token=%s\n", [label UTF8String], [stringFromObject(profileToken) UTF8String]);
	[[NSRunLoop currentRunLoop] runUntilDate:[NSDate dateWithTimeIntervalSinceNow:0.5]];
	BOOL ingestFetchPayloads = [outDir containsString:@"ingest_fetch_payloads"];
	int fetchRC = runAuxiliaryFetchCandidates(replayer, outDir, argc, argv, className, label, NO, ingestFetchPayloads ? mutableStreamData : nil);
	[[NSRunLoop currentRunLoop] runUntilDate:[NSDate dateWithTimeIntervalSinceNow:0.5]];
	if ([profileToken respondsToSelector:@selector(cancel)]) {
		BOOL cancelled = boolValueOrNo(profileToken, @"cancel");
		fprintf(stdout, "profile-during-%s profile token cancel=%d\n", [label UTF8String], cancelled ? 1 : 0);
	}
	encodeMutableStreamData(mutableStreamData, outDir, [@"profile-during-" stringByAppendingString:label]);
	fprintf(stdout, "profile-during-%s stream responses=%d fetchRC=%d\n", [label UTF8String], streamResponses, fetchRC);
	return fetchRC;
}

static void runDisplayRequest(GTMTLReplayServiceXPCProxy *replayer, NSString *outDir, NSString *label, id request) {
	if (![replayer respondsToSelector:@selector(display:)]) {
		fprintf(stdout, "%s display unsupported\n", [label UTF8String]);
		return;
	}
	@try {
		((void (*)(id, SEL, id))objc_msgSend)(replayer, @selector(display:), request);
		fprintf(stdout, "%s display sent requestClass=%s request=%s\n",
		        [label UTF8String],
		        [stringFromObject([request class]) UTF8String],
		        [stringFromObject(request) UTF8String]);
		writeObject(request, outDir, [label stringByAppendingString:@"-request"], 0);
	} @catch (NSException *exception) {
		fprintf(stdout, "%s display exception=%s\n", [label UTF8String], [[exception description] UTF8String]);
	}
	[[NSRunLoop currentRunLoop] runUntilDate:[NSDate dateWithTimeIntervalSinceNow:1.0]];
}

static int runDisplayRequestCandidates(GTMTLReplayServiceXPCProxy *replayer, NSString *outDir) {
	runDisplayRequest(replayer, outDir, @"display-empty-dictionary", @{});
	runDisplayRequest(replayer, outDir, @"display-null", nil);
	return 0;
}

static int runProfileDuringDisplayRequestCandidates(GTMTLReplayServiceXPCProxy *replayer, NSString *outDir) {
	currentProfileOutDir = outDir;
	NSString *profileMode = @"timeline_encode_streamdata";
	id request = newProfileRequest(profileMode);
	id mutableStreamData = newMutableProfilerStreamData(profileMode);
	if (!request || !mutableStreamData) {
		fprintf(stderr, "missing profile-during-display request=%p streamData=%p\n", request, mutableStreamData);
		return 61;
	}
	__block int streamResponses = 0;
	if ([request respondsToSelector:@selector(setStreamHandler:)]) {
		[(GTReplayProfileRequest *)request setStreamHandler:^(id response) {
			logResponse(response, @"profile-during-display-stream");
			NSData *data = responseData(response);
			if (data) {
				id payload = unarchivedObjectFromData(data);
				if (!payload) {
					payload = data;
				}
				payload = normalizedProfilerPayload(payload);
				BOOL added = NO;
				@try {
					if ([mutableStreamData respondsToSelector:@selector(addAPSTimelineData:)]) {
						added = ((BOOL (*)(id, SEL, id))objc_msgSend)(mutableStreamData, @selector(addAPSTimelineData:), payload);
					}
					if (!added && [mutableStreamData respondsToSelector:@selector(addAPSData:)]) {
						added = ((BOOL (*)(id, SEL, id))objc_msgSend)(mutableStreamData, @selector(addAPSData:), payload);
					}
				} @catch (NSException *exception) {
					fprintf(stdout, "profile-during-display streamData add exception=%s\n", [[exception description] UTF8String]);
				}
				fprintf(stdout, "profile-during-display streamData add bytes=%lu payloadClass=%s added=%d\n",
				        (unsigned long)[data length],
				        [stringFromObject([payload class]) UTF8String],
				        added ? 1 : 0);
			}
			writeObject(response, outDir, @"profile-during-display-stream", streamResponses);
			writeBytes(data, outDir, @"profile-during-display-stream", streamResponses++);
		}];
	}
	id profileToken = [replayer profile:request];
	fprintf(stdout, "profile-during-display profile token=%s\n", [stringFromObject(profileToken) UTF8String]);
	[[NSRunLoop currentRunLoop] runUntilDate:[NSDate dateWithTimeIntervalSinceNow:0.5]];
	runDisplayRequestCandidates(replayer, outDir);
	[[NSRunLoop currentRunLoop] runUntilDate:[NSDate dateWithTimeIntervalSinceNow:3.0]];
	if ([profileToken respondsToSelector:@selector(cancel)]) {
		BOOL cancelled = boolValueOrNo(profileToken, @"cancel");
		fprintf(stdout, "profile-during-display profile token cancel=%d\n", cancelled ? 1 : 0);
	}
	encodeMutableStreamData(mutableStreamData, outDir, @"profile-during-display");
	fprintf(stdout, "profile-during-display stream responses=%d\n", streamResponses);
	return 0;
}

static int runQueryRasterMapCandidates(GTMTLReplayServiceXPCProxy *replayer, NSString *outDir, int argc, char **argv) {
    Class batchClass = NSClassFromString(@"GTReplayRequestBatch");
    Class queryClass = NSClassFromString(@"GTReplayQueryRasterMap");
    if (!batchClass || !queryClass) {
        fprintf(stderr, "missing raster map query classes\n");
        return 51;
    }
    int idx = 0;
    for (NSString *part in candidateFetchStrings(argc, argv)) {
        NSArray *fields = [part componentsSeparatedByString:@":"];
        if ([fields count] != 4) {
            continue;
        }
        GTReplayDispatchUID dispatchUID = {{0, -1}, 0};
        dispatchUID.index.dispatchIndex = (int)strtol([[fields objectAtIndex:0] UTF8String], NULL, 10);
        dispatchUID.index.dispatchICBIndex = (int)strtol([[fields objectAtIndex:1] UTF8String], NULL, 10);
        dispatchUID.uid = strtoull([[fields objectAtIndex:2] UTF8String], NULL, 10);
        unsigned long long streamRef = strtoull([[fields objectAtIndex:3] UTF8String], NULL, 10);
        NSString *candidateDir = [outDir stringByAppendingPathComponent:[NSString stringWithFormat:@"candidate-%02d-%d-%d-%llu-%llu",
                                                                          idx,
                                                                          dispatchUID.index.dispatchIndex,
                                                                          dispatchUID.index.dispatchICBIndex,
                                                                          dispatchUID.uid,
                                                                          streamRef]];
        mkdir([candidateDir fileSystemRepresentation], 0777);
        __block int responses = 0;
        id batch = [batchClass new];
        id request = [queryClass new];
        [(GTReplayQueryRasterMap *)request setDispatchUID:dispatchUID];
        [(GTReplayQueryRasterMap *)request setStreamRef:streamRef];
        [batch setRequests:@[request]];
        [batch setCompletionHandler:^(id response) {
            logResponse(response, @"rastermap");
            writeObject(response, candidateDir, @"rastermap-object", responses);
            writeBytes(responseData(response), candidateDir, @"rastermap", responses++);
        }];
        id token = [replayer query:batch];
        fprintf(stdout, "query raster map dispatchIndex=%d dispatchICBIndex=%d uid=%llu streamRef=%llu token=%s\n",
                dispatchUID.index.dispatchIndex,
                dispatchUID.index.dispatchICBIndex,
                dispatchUID.uid,
                streamRef,
                [stringFromObject(token) UTF8String]);
        [[NSRunLoop currentRunLoop] runUntilDate:[NSDate dateWithTimeIntervalSinceNow:1.0]];
        idx++;
    }
    return 0;
}

static int runDecodeGenericAccelerationStructureCandidates(GTMTLReplayServiceXPCProxy *replayer, NSString *outDir, int argc, char **argv) {
    Class batchClass = NSClassFromString(@"GTReplayRequestBatch");
    Class decodeClass = NSClassFromString(@"GTReplayDecodeGenericAccelerationStructure");
    if (!batchClass || !decodeClass) {
        fprintf(stderr, "missing decode generic acceleration structure classes\n");
        return 52;
    }
    int idx = 0;
    NSMutableSet *seen = [NSMutableSet set];
    for (NSString *part in candidateFetchStrings(argc, argv)) {
        NSArray *fields = [part componentsSeparatedByString:@":"];
        if ([fields count] != 4) {
            continue;
        }
        unsigned long long streamRef = strtoull([[fields objectAtIndex:3] UTF8String], NULL, 10);
        NSNumber *streamKey = @(streamRef);
        if ([seen containsObject:streamKey]) {
            continue;
        }
        [seen addObject:streamKey];
        NSString *candidateDir = [outDir stringByAppendingPathComponent:[NSString stringWithFormat:@"candidate-%02d-%llu", idx, streamRef]];
        mkdir([candidateDir fileSystemRepresentation], 0777);
        __block int responses = 0;
        id batch = [batchClass new];
        id request = [decodeClass new];
        [(GTReplayDecodeGenericAccelerationStructure *)request setStreamRef:streamRef];
        [batch setRequests:@[request]];
        [batch setCompletionHandler:^(id response) {
            logResponse(response, @"decode");
            writeObject(response, candidateDir, @"decode-object", responses);
            writeBytes(responseData(response), candidateDir, @"decode", responses++);
        }];
        id token = [replayer decode:batch];
        fprintf(stdout, "decode generic acceleration structure streamRef=%llu token=%s\n",
                streamRef,
                [stringFromObject(token) UTF8String]);
        [[NSRunLoop currentRunLoop] runUntilDate:[NSDate dateWithTimeIntervalSinceNow:1.0]];
        idx++;
    }
    return 0;
}

static id newDecodeRequest(NSString *className, GTReplayDispatchUID dispatchUID, unsigned long long streamRef, int idx) {
	Class decodeClass = NSClassFromString(className);
	if (!decodeClass) {
		return nil;
	}
	id request = [decodeClass new];
	if ([request respondsToSelector:@selector(setDispatchUID:)]) {
		((void (*)(id, SEL, GTReplayDispatchUID))objc_msgSend)(request, @selector(setDispatchUID:), dispatchUID);
	}
	if ([request respondsToSelector:@selector(setStreamRef:)]) {
		((void (*)(id, SEL, uint64_t))objc_msgSend)(request, @selector(setStreamRef:), (uint64_t)streamRef);
	}
	if ([className isEqualToString:@"GTReplayDecodeAB"]) {
		if ([request respondsToSelector:@selector(setType:)]) {
			[(GTReplayDecodeAB *)request setType:0];
		}
		if ([request respondsToSelector:@selector(setIndex:)]) {
			[(GTReplayDecodeAB *)request setIndex:(unsigned int)idx];
		}
	}
	return request;
}

static int runDecodeCandidates(GTMTLReplayServiceXPCProxy *replayer, NSString *outDir, int argc, char **argv, NSString *className, NSString *label) {
	Class batchClass = NSClassFromString(@"GTReplayRequestBatch");
	Class decodeClass = NSClassFromString(className);
	if (!batchClass || !decodeClass || ![replayer respondsToSelector:@selector(decode:)]) {
		fprintf(stderr, "missing decode classes class=%s batch=%p decodeClass=%p responds=%d\n",
		        [className UTF8String],
		        batchClass,
		        decodeClass,
		        [replayer respondsToSelector:@selector(decode:)] ? 1 : 0);
		return 63;
	}
	int idx = 0;
	for (NSString *part in candidateFetchStrings(argc, argv)) {
		if (idx >= GTProbeAuxFetchCandidateLimit) {
			fprintf(stdout, "%s candidate limit reached=%d\n", [label UTF8String], GTProbeAuxFetchCandidateLimit);
			break;
		}
		NSArray *fields = [part componentsSeparatedByString:@":"];
		if ([fields count] != 4) {
			continue;
		}
		GTReplayDispatchUID dispatchUID = {{0, -1}, 0};
		dispatchUID.index.dispatchIndex = (int)strtol([[fields objectAtIndex:0] UTF8String], NULL, 10);
		dispatchUID.index.dispatchICBIndex = (int)strtol([[fields objectAtIndex:1] UTF8String], NULL, 10);
		dispatchUID.uid = strtoull([[fields objectAtIndex:2] UTF8String], NULL, 10);
		unsigned long long streamRef = strtoull([[fields objectAtIndex:3] UTF8String], NULL, 10);
		NSString *candidateDir = [outDir stringByAppendingPathComponent:[NSString stringWithFormat:@"candidate-%02d-%d-%d-%llu-%llu",
		                                                                  idx,
		                                                                  dispatchUID.index.dispatchIndex,
		                                                                  dispatchUID.index.dispatchICBIndex,
		                                                                  dispatchUID.uid,
		                                                                  streamRef]];
		mkdir([candidateDir fileSystemRepresentation], 0777);
		__block int responses = 0;
		id batch = [batchClass new];
		id request = newDecodeRequest(className, dispatchUID, streamRef, idx);
		if (!request) {
			fprintf(stderr, "failed to create decode request class=%s\n", [className UTF8String]);
			return 64;
		}
		[batch setRequests:@[request]];
		[batch setCompletionHandler:^(id response) {
			logResponse(response, label);
			writeObject(response, candidateDir, [label stringByAppendingString:@"-object"], responses);
			writeBytes(responseData(response), candidateDir, label, responses++);
		}];
		id token = [replayer decode:batch];
		fprintf(stdout, "%s dispatchIndex=%d dispatchICBIndex=%d uid=%llu streamRef=%llu request=%s token=%s\n",
		        [label UTF8String],
		        dispatchUID.index.dispatchIndex,
		        dispatchUID.index.dispatchICBIndex,
		        dispatchUID.uid,
		        streamRef,
		        [stringFromObject(request) UTF8String],
		        [stringFromObject(token) UTF8String]);
		[[NSRunLoop currentRunLoop] runUntilDate:[NSDate dateWithTimeIntervalSinceNow:1.0]];
		idx++;
	}
	return 0;
}

static int runProfileDuringDecodeCandidates(GTMTLReplayServiceXPCProxy *replayer, NSString *outDir, int argc, char **argv, NSString *className, NSString *label) {
	currentProfileOutDir = outDir;
	NSString *profileMode = @"timeline_encode_streamdata";
	id request = newProfileRequest(profileMode);
	id mutableStreamData = newMutableProfilerStreamData(profileMode);
	if (!request || !mutableStreamData) {
		fprintf(stderr, "missing profile-during-%s request=%p streamData=%p\n", [label UTF8String], request, mutableStreamData);
		return 65;
	}
	__block int streamResponses = 0;
	if ([request respondsToSelector:@selector(setStreamHandler:)]) {
		[(GTReplayProfileRequest *)request setStreamHandler:^(id response) {
			logResponse(response, [@"profile-during-" stringByAppendingString:label]);
			NSData *data = responseData(response);
			if (data) {
				id payload = unarchivedObjectFromData(data);
				if (!payload) {
					payload = data;
				}
				payload = normalizedProfilerPayload(payload);
				BOOL added = tryAddPayloadToMutableStreamData(mutableStreamData, payload, [@"profile-during-" stringByAppendingString:label]);
				fprintf(stdout, "profile-during-%s streamData add bytes=%lu payloadClass=%s added=%d\n",
				        [label UTF8String],
				        (unsigned long)[data length],
				        [stringFromObject([payload class]) UTF8String],
				        added ? 1 : 0);
			}
			writeObject(response, outDir, [@"profile-during-" stringByAppendingString:label], streamResponses);
			writeBytes(data, outDir, [@"profile-during-" stringByAppendingString:label], streamResponses++);
		}];
	}
	id profileToken = [replayer profile:request];
	fprintf(stdout, "profile-during-%s profile token=%s\n", [label UTF8String], [stringFromObject(profileToken) UTF8String]);
	[[NSRunLoop currentRunLoop] runUntilDate:[NSDate dateWithTimeIntervalSinceNow:0.5]];
	int decodeRC = runDecodeCandidates(replayer, outDir, argc, argv, className, label);
	[[NSRunLoop currentRunLoop] runUntilDate:[NSDate dateWithTimeIntervalSinceNow:0.5]];
	if ([profileToken respondsToSelector:@selector(cancel)]) {
		BOOL cancelled = boolValueOrNo(profileToken, @"cancel");
		fprintf(stdout, "profile-during-%s profile token cancel=%d\n", [label UTF8String], cancelled ? 1 : 0);
	}
	encodeMutableStreamData(mutableStreamData, outDir, [@"profile-during-" stringByAppendingString:label]);
	fprintf(stdout, "profile-during-%s stream responses=%d decodeRC=%d\n", [label UTF8String], streamResponses, decodeRC);
	return decodeRC;
}

static id newUpdateLibraryRequest(GTReplayDispatchUID dispatchUID, unsigned long long streamRef) {
	Class updateClass = NSClassFromString(@"GTReplayUpdateLibrary");
	if (!updateClass) {
		return nil;
	}
	id request = [updateClass new];
	if ([request respondsToSelector:@selector(setDispatchUID:)]) {
		((void (*)(id, SEL, GTReplayDispatchUID))objc_msgSend)(request, @selector(setDispatchUID:), dispatchUID);
	}
	if ([request respondsToSelector:@selector(setStreamRef:)]) {
		((void (*)(id, SEL, uint64_t))objc_msgSend)(request, @selector(setStreamRef:), (uint64_t)streamRef);
	}
	if ([request respondsToSelector:@selector(setShaderIR:)]) {
		[(GTReplayUpdateLibrary *)request setShaderIR:[NSData data]];
	}
	if ([request respondsToSelector:@selector(setShaderSource:)]) {
		[(GTReplayUpdateLibrary *)request setShaderSource:@""];
	}
	return request;
}

static int runUpdateLibraryCandidates(GTMTLReplayServiceXPCProxy *replayer, NSString *outDir, int argc, char **argv, BOOL waitComplete) {
	Class batchClass = NSClassFromString(@"GTReplayRequestBatch");
	Class updateClass = NSClassFromString(@"GTReplayUpdateLibrary");
	if (!batchClass || !updateClass || ![replayer respondsToSelector:@selector(update:)]) {
		fprintf(stderr, "missing update library classes batch=%p updateClass=%p responds=%d\n",
		        batchClass,
		        updateClass,
		        [replayer respondsToSelector:@selector(update:)] ? 1 : 0);
		return 66;
	}
	int idx = 0;
	for (NSString *part in candidateFetchStrings(argc, argv)) {
		if (idx >= GTProbeAuxFetchCandidateLimit) {
			fprintf(stdout, "update-library candidate limit reached=%d\n", GTProbeAuxFetchCandidateLimit);
			break;
		}
		NSArray *fields = [part componentsSeparatedByString:@":"];
		if ([fields count] != 4) {
			continue;
		}
		GTReplayDispatchUID dispatchUID = {{0, -1}, 0};
		dispatchUID.index.dispatchIndex = (int)strtol([[fields objectAtIndex:0] UTF8String], NULL, 10);
		dispatchUID.index.dispatchICBIndex = (int)strtol([[fields objectAtIndex:1] UTF8String], NULL, 10);
		dispatchUID.uid = strtoull([[fields objectAtIndex:2] UTF8String], NULL, 10);
		unsigned long long streamRef = strtoull([[fields objectAtIndex:3] UTF8String], NULL, 10);
		NSString *candidateDir = [outDir stringByAppendingPathComponent:[NSString stringWithFormat:@"candidate-%02d-%d-%d-%llu-%llu",
		                                                                  idx,
		                                                                  dispatchUID.index.dispatchIndex,
		                                                                  dispatchUID.index.dispatchICBIndex,
		                                                                  dispatchUID.uid,
		                                                                  streamRef]];
		mkdir([candidateDir fileSystemRepresentation], 0777);
		__block int responses = 0;
		id batch = [batchClass new];
		id request = newUpdateLibraryRequest(dispatchUID, streamRef);
		if (!request) {
			fprintf(stderr, "failed to create update-library request\n");
			return 67;
		}
		[batch setRequests:@[request]];
		[batch setCompletionHandler:^(id response) {
			logResponse(response, @"update-library");
			writeObject(response, candidateDir, @"update-library-object", responses);
			writeBytes(responseData(response), candidateDir, @"update-library", responses++);
		}];
		id token = nil;
		@try {
			token = [replayer update:batch];
		} @catch (NSException *exception) {
			fprintf(stdout, "update-library exception=%s\n", [[exception description] UTF8String]);
			return 68;
		}
		fprintf(stdout, "update-library dispatchIndex=%d dispatchICBIndex=%d uid=%llu streamRef=%llu request=%s token=%s\n",
		        dispatchUID.index.dispatchIndex,
		        dispatchUID.index.dispatchICBIndex,
		        dispatchUID.uid,
		        streamRef,
		        [stringFromObject(request) UTF8String],
		        [stringFromObject(token) UTF8String]);
		[[NSRunLoop currentRunLoop] runUntilDate:[NSDate dateWithTimeIntervalSinceNow:(waitComplete ? 1.0 : 0.25)]];
		idx++;
	}
	return 0;
}

static int runProfileDuringUpdateLibraryCandidates(GTMTLReplayServiceXPCProxy *replayer, NSString *outDir, int argc, char **argv) {
	currentProfileOutDir = outDir;
	NSString *profileMode = @"timeline_encode_streamdata";
	id request = newProfileRequest(profileMode);
	id mutableStreamData = newMutableProfilerStreamData(profileMode);
	if (!request || !mutableStreamData) {
		fprintf(stderr, "missing profile-during-update-library request=%p streamData=%p\n", request, mutableStreamData);
		return 69;
	}
	__block int streamResponses = 0;
	if ([request respondsToSelector:@selector(setStreamHandler:)]) {
		[(GTReplayProfileRequest *)request setStreamHandler:^(id response) {
			logResponse(response, @"profile-during-update-library");
			NSData *data = responseData(response);
			if (data) {
				id payload = unarchivedObjectFromData(data);
				if (!payload) {
					payload = data;
				}
				payload = normalizedProfilerPayload(payload);
				BOOL added = tryAddPayloadToMutableStreamData(mutableStreamData, payload, @"profile-during-update-library");
				fprintf(stdout, "profile-during-update-library streamData add bytes=%lu payloadClass=%s added=%d\n",
				        (unsigned long)[data length],
				        [stringFromObject([payload class]) UTF8String],
				        added ? 1 : 0);
			}
			writeObject(response, outDir, @"profile-during-update-library", streamResponses);
			writeBytes(data, outDir, @"profile-during-update-library", streamResponses++);
		}];
	}
	id profileToken = [replayer profile:request];
	fprintf(stdout, "profile-during-update-library profile token=%s\n", [stringFromObject(profileToken) UTF8String]);
	[[NSRunLoop currentRunLoop] runUntilDate:[NSDate dateWithTimeIntervalSinceNow:0.5]];
	int updateRC = runUpdateLibraryCandidates(replayer, outDir, argc, argv, NO);
	[[NSRunLoop currentRunLoop] runUntilDate:[NSDate dateWithTimeIntervalSinceNow:0.5]];
	if ([profileToken respondsToSelector:@selector(cancel)]) {
		BOOL cancelled = boolValueOrNo(profileToken, @"cancel");
		fprintf(stdout, "profile-during-update-library profile token cancel=%d\n", cancelled ? 1 : 0);
	}
	encodeMutableStreamData(mutableStreamData, outDir, @"profile-during-update-library");
	fprintf(stdout, "profile-during-update-library stream responses=%d updateRC=%d\n", streamResponses, updateRC);
	return updateRC;
}

static void appendUniqueHandle(NSMutableArray *handles, uint64_t handle) {
	for (NSNumber *number in handles) {
		if ([number unsignedLongLongValue] == handle) {
			return;
		}
	}
	[handles addObject:@(handle)];
}

static int runProfileBulkDownloadCandidates(GTMTLReplayServiceXPCProxy *replayer, NSString *outDir, NSString *mode) {
	currentProfileOutDir = outDir;
	id bulkDataProxy = valueOrNil(replayer, @"bulkDataProxy");
	if (!bulkDataProxy) {
		fprintf(stderr, "profile bulk download missing bulkDataProxy\n");
		return 70;
	}
	if (![bulkDataProxy respondsToSelector:@selector(downloadData:error:)]) {
		fprintf(stderr, "profile bulk download bulkDataProxy does not respond to downloadData:error: class=%s\n",
		        [stringFromObject([bulkDataProxy class]) UTF8String]);
		return 71;
	}
	NSString *profileMode = @"timeline_encode_streamdata";
	id request = newProfileRequest(profileMode);
	id mutableStreamData = newMutableProfilerStreamData(profileMode);
	if (!request || !mutableStreamData) {
		fprintf(stderr, "missing profile bulk download request=%p streamData=%p\n", request, mutableStreamData);
		return 72;
	}
	__block int streamResponses = 0;
	__block NSMutableArray *streamRequestIDs = [NSMutableArray array];
	if ([request respondsToSelector:@selector(setStreamHandler:)]) {
		[(GTReplayProfileRequest *)request setStreamHandler:^(id response) {
			logResponse(response, @"profile-bulk-download-stream");
			dumpObjectSnapshot(response, @"profile-bulk-download response");
			uint64_t requestID = unsignedLongLongValueOrZero(response, @"requestID");
			if (requestID != 0) {
				appendUniqueHandle(streamRequestIDs, requestID);
			}
			NSData *data = responseData(response);
			if (data) {
				id payload = unarchivedObjectFromData(data);
				if (!payload) {
					payload = data;
				}
				payload = normalizedProfilerPayload(payload);
				BOOL added = tryAddPayloadToMutableStreamData(mutableStreamData, payload, @"profile-bulk-download-stream");
				fprintf(stdout, "profile-bulk-download streamData add requestID=%llu bytes=%lu payloadClass=%s added=%d\n",
				        requestID,
				        (unsigned long)[data length],
				        [stringFromObject([payload class]) UTF8String],
				        added ? 1 : 0);
			}
			writeObject(response, outDir, @"profile-bulk-download-stream", streamResponses);
			writeBytes(data, outDir, @"profile-bulk-download-stream", streamResponses++);
		}];
	}
	id profileToken = [replayer profile:request];
	uint64_t tokenID = unsignedLongLongValueOrZero(profileToken, @"tokenId");
	dumpObjectSnapshot(profileToken, @"profile-bulk-download token");
	dumpObjectSnapshot(bulkDataProxy, @"profile-bulk-download bulkDataProxy");
	fprintf(stdout, "profile-bulk-download profile token=%s tokenID=%llu bulkProxy=%s\n",
	        [stringFromObject(profileToken) UTF8String],
	        tokenID,
	        [stringFromObject(bulkDataProxy) UTF8String]);
	if ([mode containsString:@"proxy_resume"] && [replayer respondsToSelector:@selector(resume:)]) {
		BOOL resumed = ((BOOL (*)(id, SEL, uint64_t))objc_msgSend)(replayer, @selector(resume:), tokenID);
		fprintf(stdout, "profile-bulk-download proxy resume=%d\n", resumed ? 1 : 0);
	} else if ([mode containsString:@"resume"] && [profileToken respondsToSelector:@selector(resume)]) {
		BOOL resumed = boolValueOrNo(profileToken, @"resume");
		fprintf(stdout, "profile-bulk-download token resume=%d\n", resumed ? 1 : 0);
	}
	if ([mode containsString:@"wait_complete"] && [profileToken respondsToSelector:@selector(waitUntilCompleted)]) {
		fprintf(stdout, "profile-bulk-download token waitUntilCompleted begin\n");
#pragma clang diagnostic push
#pragma clang diagnostic ignored "-Warc-performSelector-leaks"
		[profileToken performSelector:@selector(waitUntilCompleted)];
#pragma clang diagnostic pop
		fprintf(stdout, "profile-bulk-download token waitUntilCompleted done\n");
		[[NSRunLoop currentRunLoop] runUntilDate:[NSDate dateWithTimeIntervalSinceNow:0.5]];
	} else {
		double waitSeconds = [mode containsString:@"wait_5s"] ? 5.0 : 1.0;
		fprintf(stdout, "profile-bulk-download run loop collection begin seconds=%.1f\n", waitSeconds);
		[[NSRunLoop currentRunLoop] runUntilDate:[NSDate dateWithTimeIntervalSinceNow:waitSeconds]];
		fprintf(stdout, "profile-bulk-download run loop collection done\n");
	}
	NSMutableArray *handles = [NSMutableArray array];
	for (uint64_t handle = 0; handle <= 32; handle++) {
		appendUniqueHandle(handles, handle);
	}
	for (NSNumber *number in streamRequestIDs) {
		appendUniqueHandle(handles, [number unsignedLongLongValue]);
	}
	if (tokenID != 0) {
		appendUniqueHandle(handles, tokenID);
		for (uint64_t offset = 1; offset <= 2; offset++) {
			appendUniqueHandle(handles, tokenID + offset);
			if (tokenID > offset) {
				appendUniqueHandle(handles, tokenID - offset);
			}
		}
	}
	fprintf(stdout, "profile-bulk-download candidate handles=%s\n", [[handles description] UTF8String]);
	int downloads = 0;
	for (NSNumber *number in handles) {
		uint64_t handle = [number unsignedLongLongValue];
		NSError *downloadError = nil;
		id downloaded = nil;
		@try {
			downloaded = ((id (*)(id, SEL, uint64_t, NSError **))objc_msgSend)(bulkDataProxy, @selector(downloadData:error:), handle, &downloadError);
		} @catch (NSException *exception) {
			fprintf(stdout, "profile-bulk-download handle=%llu exception=%s\n",
			        handle,
			        [[exception description] UTF8String]);
			continue;
		}
		NSUInteger length = [downloaded isKindOfClass:[NSData class]] ? [(NSData *)downloaded length] : 0;
		fprintf(stdout, "profile-bulk-download handle=%llu class=%s length=%lu error=%s\n",
		        handle,
		        [stringFromObject([downloaded class]) UTF8String],
		        (unsigned long)length,
		        [[downloadError description] UTF8String]);
		if (!downloaded) {
			continue;
		}
		if ([downloaded isKindOfClass:[NSData class]]) {
			NSString *file = [outDir stringByAppendingPathComponent:[NSString stringWithFormat:@"bulk-download-%llu.bin", handle]];
			[(NSData *)downloaded writeToFile:file atomically:NO];
			id payload = unarchivedObjectFromData(downloaded);
			if (!payload) {
				payload = downloaded;
			}
			payload = normalizedProfilerPayload(payload);
			BOOL added = tryAddPayloadToMutableStreamData(mutableStreamData, payload, [NSString stringWithFormat:@"profile-bulk-download-%llu", handle]);
			fprintf(stdout, "profile-bulk-download streamData add handle=%llu payloadClass=%s added=%d\n",
			        handle,
			        [stringFromObject([payload class]) UTF8String],
			        added ? 1 : 0);
		} else {
			writeObject(downloaded, outDir, [NSString stringWithFormat:@"bulk-download-%llu", handle], 0);
			id payload = normalizedProfilerPayload(downloaded);
			BOOL added = tryAddPayloadToMutableStreamData(mutableStreamData, payload, [NSString stringWithFormat:@"profile-bulk-download-%llu", handle]);
			fprintf(stdout, "profile-bulk-download object add handle=%llu payloadClass=%s added=%d\n",
			        handle,
			        [stringFromObject([payload class]) UTF8String],
			        added ? 1 : 0);
		}
		downloads++;
	}
	[[NSRunLoop currentRunLoop] runUntilDate:[NSDate dateWithTimeIntervalSinceNow:0.25]];
	if ([profileToken respondsToSelector:@selector(cancel)]) {
		BOOL cancelled = boolValueOrNo(profileToken, @"cancel");
		fprintf(stdout, "profile-bulk-download profile token cancel=%d\n", cancelled ? 1 : 0);
	}
	encodeMutableStreamData(mutableStreamData, outDir, @"profile-bulk-download");
	fprintf(stdout, "profile-bulk-download stream responses=%d requestIDs=%s downloads=%d\n",
	        streamResponses,
	        [[streamRequestIDs description] UTF8String],
	        downloads);
	return 0;
}

static int runRaytraceCandidates(GTMTLReplayServiceXPCProxy *replayer, NSString *outDir, int argc, char **argv) {
    Class requestClass = NSClassFromString(@"GTReplayRaytraceRequest");
    if (!requestClass) {
        fprintf(stderr, "missing raytrace request class\n");
        return 53;
    }
    int idx = 0;
    for (NSString *part in candidateFetchStrings(argc, argv)) {
        NSArray *fields = [part componentsSeparatedByString:@":"];
        if ([fields count] != 4) {
            continue;
        }
        GTReplayDispatchUID dispatchUID = {{0, -1}, 0};
        dispatchUID.index.dispatchIndex = (int)strtol([[fields objectAtIndex:0] UTF8String], NULL, 10);
        dispatchUID.index.dispatchICBIndex = (int)strtol([[fields objectAtIndex:1] UTF8String], NULL, 10);
        dispatchUID.uid = strtoull([[fields objectAtIndex:2] UTF8String], NULL, 10);
        unsigned long long streamRef = strtoull([[fields objectAtIndex:3] UTF8String], NULL, 10);
        NSString *candidateDir = [outDir stringByAppendingPathComponent:[NSString stringWithFormat:@"candidate-%02d-%d-%d-%llu-%llu",
                                                                          idx,
                                                                          dispatchUID.index.dispatchIndex,
                                                                          dispatchUID.index.dispatchICBIndex,
                                                                          dispatchUID.uid,
                                                                          streamRef]];
        mkdir([candidateDir fileSystemRepresentation], 0777);
        __block int responses = 0;
        id request = [requestClass new];
        [(GTReplayRaytraceRequest *)request setDispatchUID:dispatchUID];
        [(GTReplayRaytraceRequest *)request setStreamRef:streamRef];
        [(GTReplayRaytraceRequest *)request setStreamHandler:^(id response) {
            logResponse(response, @"raytrace");
            writeObject(response, candidateDir, @"raytrace-object", responses);
            writeBytes(responseData(response), candidateDir, @"raytrace", responses++);
        }];
        id token = [replayer raytrace:request];
        fprintf(stdout, "raytrace dispatchIndex=%d dispatchICBIndex=%d uid=%llu streamRef=%llu token=%s\n",
                dispatchUID.index.dispatchIndex,
                dispatchUID.index.dispatchICBIndex,
                dispatchUID.uid,
                streamRef,
                [stringFromObject(token) UTF8String]);
        [[NSRunLoop currentRunLoop] runUntilDate:[NSDate dateWithTimeIntervalSinceNow:1.0]];
        if ([token respondsToSelector:@selector(cancel)]) {
            BOOL cancelled = boolValueOrNo(token, @"cancel");
            fprintf(stdout, "raytrace token cancel=%d\n", cancelled ? 1 : 0);
        }
        idx++;
    }
    return 0;
}

static int runShaderDebugKernelCandidates(GTMTLReplayServiceXPCProxy *replayer, NSString *outDir, int argc, char **argv) {
	Class requestClass = NSClassFromString(@"GTReplayShaderDebugKernel");
	if (!requestClass) {
		fprintf(stderr, "missing shader debug kernel class\n");
		return 70;
	}
	double waitSeconds = 3.0;
	if (argc >= 6) {
		waitSeconds = strtod(argv[5], NULL);
		if (waitSeconds <= 0) {
			waitSeconds = 3.0;
		}
	}
	int idx = 0;
	for (NSString *part in candidateDispatchUIDStrings(argc, argv)) {
        GTReplayDispatchUID dispatchUID = parseDispatchUID(part);
        NSString *candidateDir = [outDir stringByAppendingPathComponent:[NSString stringWithFormat:@"candidate-%02d-%d-%d-%llu",
                                                                          idx,
                                                                          dispatchUID.index.dispatchIndex,
                                                                          dispatchUID.index.dispatchICBIndex,
                                                                          dispatchUID.uid]];
        mkdir([candidateDir fileSystemRepresentation], 0777);
        __block int responses = 0;
		id request = [requestClass new];
		[(GTReplayShaderDebugKernel *)request setDispatchUID:dispatchUID];
		[(GTReplayShaderDebugKernel *)request setProgramDataVersion:0];
		[(GTReplayShaderDebugKernel *)request setProgramData:archivedEmptyDictionary()];
		GTReplayPoint3D minPoint = {0, 0, 0};
        GTReplayPoint3D maxPoint = {1, 1, 1};
        [(GTReplayShaderDebugKernel *)request setMinThreadPositionInGrid:minPoint];
        [(GTReplayShaderDebugKernel *)request setMaxThreadPositionInGrid:maxPoint];
        [(GTReplayShaderDebugKernel *)request setCompletionHandler:^(id response) {
            logResponse(response, @"shaderdebug");
            writeBytes(responseData(response), candidateDir, @"shaderdebug", responses++);
        }];
        id token = [replayer shaderdebug:request];
        fprintf(stdout, "shaderdebug kernel dispatchIndex=%d dispatchICBIndex=%d uid=%llu token=%s\n",
                dispatchUID.index.dispatchIndex,
                dispatchUID.index.dispatchICBIndex,
                dispatchUID.uid,
                [stringFromObject(token) UTF8String]);
		[[NSRunLoop currentRunLoop] runUntilDate:[NSDate dateWithTimeIntervalSinceNow:waitSeconds]];
        if ([token respondsToSelector:@selector(cancel)]) {
            BOOL cancelled = boolValueOrNo(token, @"cancel");
            fprintf(stdout, "shaderdebug token cancel=%d\n", cancelled ? 1 : 0);
        }
        fprintf(stdout, "shaderdebug responses=%d\n", responses);
        idx++;
        if (responses > 0) {
            break;
        }
    }
    return 0;
}

static int runProfileDuringShaderDebugKernelCandidates(GTMTLReplayServiceXPCProxy *replayer, NSString *outDir, int argc, char **argv) {
	currentProfileOutDir = outDir;
	NSString *profileMode = @"timeline_encode_streamdata";
	id request = newProfileRequest(profileMode);
	id mutableStreamData = newMutableProfilerStreamData(profileMode);
	if (!request || !mutableStreamData) {
		fprintf(stderr, "missing profile-during-shaderdebug request=%p streamData=%p\n", request, mutableStreamData);
		return 57;
	}
	__block int streamResponses = 0;
	if ([request respondsToSelector:@selector(setStreamHandler:)]) {
		[(GTReplayProfileRequest *)request setStreamHandler:^(id response) {
			logResponse(response, @"profile-during-shaderdebug-stream");
			NSData *data = responseData(response);
			if (data) {
				id payload = unarchivedObjectFromData(data);
				if (!payload) {
					payload = data;
				}
				payload = normalizedProfilerPayload(payload);
				BOOL added = NO;
				@try {
					if ([mutableStreamData respondsToSelector:@selector(addAPSTimelineData:)]) {
						added = ((BOOL (*)(id, SEL, id))objc_msgSend)(mutableStreamData, @selector(addAPSTimelineData:), payload);
					}
					if (!added && [mutableStreamData respondsToSelector:@selector(addAPSData:)]) {
						added = ((BOOL (*)(id, SEL, id))objc_msgSend)(mutableStreamData, @selector(addAPSData:), payload);
					}
				} @catch (NSException *exception) {
					fprintf(stdout, "profile-during-shaderdebug streamData add exception=%s\n", [[exception description] UTF8String]);
				}
				fprintf(stdout, "profile-during-shaderdebug streamData add bytes=%lu payloadClass=%s added=%d\n",
				        (unsigned long)[data length],
				        [stringFromObject([payload class]) UTF8String],
				        added ? 1 : 0);
			}
			writeObject(response, outDir, @"profile-during-shaderdebug-stream", streamResponses);
			writeBytes(data, outDir, @"profile-during-shaderdebug-stream", streamResponses++);
		}];
	}
	id profileToken = [replayer profile:request];
	fprintf(stdout, "profile-during-shaderdebug profile token=%s\n", [stringFromObject(profileToken) UTF8String]);
	[[NSRunLoop currentRunLoop] runUntilDate:[NSDate dateWithTimeIntervalSinceNow:0.5]];
	int debugRC = runShaderDebugKernelCandidates(replayer, outDir, argc, argv);
	[[NSRunLoop currentRunLoop] runUntilDate:[NSDate dateWithTimeIntervalSinceNow:0.5]];
	if ([profileToken respondsToSelector:@selector(cancel)]) {
		BOOL cancelled = boolValueOrNo(profileToken, @"cancel");
		fprintf(stdout, "profile-during-shaderdebug profile token cancel=%d\n", cancelled ? 1 : 0);
	}
	encodeMutableStreamData(mutableStreamData, outDir, @"profile-during-shaderdebug");
	fprintf(stdout, "profile-during-shaderdebug stream responses=%d debugRC=%d\n", streamResponses, debugRC);
	return debugRC;
}

static int runUpdateConfiguration(GTMTLReplayServiceXPCProxy *replayer, NSString *outDir, NSString *mode) {
    Class batchClass = NSClassFromString(@"GTReplayRequestBatch");
    Class updateClass = NSClassFromString(@"GTReplayUpdateConfiguration");
    Class configurationClass = NSClassFromString(@"GTReplayConfiguration");
    if (!batchClass || !updateClass || !configurationClass) {
        fprintf(stderr, "missing update configuration classes\n");
        return 60;
    }
    __block int updateResponses = 0;
    id batch = [batchClass new];
    GTReplayConfiguration *configuration = [configurationClass new];
    [configuration setForceWaitUntilCompleted:YES];
    [configuration setForceResourcesResident:YES];
    [configuration setForceLoadUnusedResources:YES];
    [configuration setEnableCapture:YES];
    [configuration setEnableReplayFromOtherPlatforms:YES];
    [configuration setEnableDisplayOnDevice:[mode containsString:@"display_on"]];
    [configuration setEnableHUD:NO];
    id request = [updateClass new];
    [(GTReplayUpdateConfiguration *)request setConfiguration:configuration];
    [batch setRequests:@[request]];
    [batch setCompletionHandler:^(id response) {
        logResponse(response, @"update");
        writeBytes(responseData(response), outDir, @"update", updateResponses++);
    }];
    id token = [replayer update:batch];
    fprintf(stdout, "update configuration mode=%s displayOnDevice=%d token=%s\n",
            [mode UTF8String],
            [mode containsString:@"display_on"] ? 1 : 0,
            [stringFromObject(token) UTF8String]);
    [[NSRunLoop currentRunLoop] runUntilDate:[NSDate dateWithTimeIntervalSinceNow:1.0]];
    return 0;
}

static NSString *probeChildDir(NSString *outDir, NSString *name) {
	NSString *path = [outDir stringByAppendingPathComponent:name];
	mkdir([path fileSystemRepresentation], 0777);
	return path;
}

static int runDatasourceReadyThenQueryDerivedCounters(GTMTLReplayServiceXPCProxy *replayer, NSString *outDir, int argc, char **argv) {
	int rc = 0;
	fprintf(stdout, "datasource-readiness begin\n");
	int updateRC = runUpdateConfiguration(replayer, probeChildDir(outDir, @"readiness-update-config"), @"datasource_ready_update_config");
	fprintf(stdout, "datasource-readiness update_config rc=%d\n", updateRC);
	if (updateRC != 0 && rc == 0) {
		rc = updateRC;
	}
	int configRC = runQueryClass(replayer, probeChildDir(outDir, @"readiness-query-configuration"), @"GTReplayQueryConfiguration");
	fprintf(stdout, "datasource-readiness query_configuration rc=%d\n", configRC);
	if (configRC != 0 && rc == 0) {
		rc = configRC;
	}
	int perfRC = runQueryClass(replayer, probeChildDir(outDir, @"readiness-query-performance-state"), @"GTReplayQueryPerformanceState");
	fprintf(stdout, "datasource-readiness query_performance_state rc=%d\n", perfRC);
	if (perfRC != 0 && rc == 0) {
		rc = perfRC;
	}
	int resourceRC = runResourceUsageCandidates(replayer, probeChildDir(outDir, @"readiness-query-resource-usage"), argc, argv);
	fprintf(stdout, "datasource-readiness query_resource_usage rc=%d\n", resourceRC);
	if (resourceRC != 0 && rc == 0) {
		rc = resourceRC;
	}
	int derivedRC = runQueryDerivedCountersEncodeStreamData(replayer, probeChildDir(outDir, @"derived-query-after-readiness"));
	fprintf(stdout, "datasource-readiness query_derived_counters rc=%d\n", derivedRC);
	if (derivedRC != 0 && rc == 0) {
		rc = derivedRC;
	}
	fprintf(stdout, "datasource-readiness done rc=%d\n", rc);
	return rc;
}

static id newProfileRequest(NSString *mode) {
	NSString *baseMode = baseProfileMode(mode);
	NSString *explicitClassName = nil;
	if ([baseMode hasPrefix:@"profile_class_"]) {
		explicitClassName = [baseMode substringFromIndex:[@"profile_class_" length]];
	}
	if ([baseMode hasPrefix:@"profile_base_"]) {
		explicitClassName = @"GTReplayProfileRequest";
	}
	Class requestClass = explicitClassName ? NSClassFromString(explicitClassName) : NSClassFromString(@"GTReplayProfileTimeline");
	if ([baseMode hasPrefix:@"derived_counters"] || [baseMode hasPrefix:@"derived_aps_"]) {
		requestClass = NSClassFromString(@"GTReplayProfileDerivedCounters");
	} else if ([baseMode hasPrefix:@"batch_filtered_counters"] || [baseMode hasPrefix:@"batch_filtered_aps_"]) {
		requestClass = NSClassFromString(@"GTReplayProfileBatchFilteredCounters");
    }
    if (!requestClass) {
        return nil;
    }
	id request = [requestClass new];
	if ([request respondsToSelector:@selector(setPriority:)]) {
		int priority = ([baseMode isEqualToString:@"timeline_raw_priority_1"] || [baseMode containsString:@"priority_1"]) ? 1 : 0;
		[(GTReplayProfileRequest *)request setPriority:priority];
	}
	if ([request respondsToSelector:@selector(setProfileDataVersion:)]) {
		int version = 1;
		if ([baseMode isEqualToString:@"timeline_raw_profile_version_0"]) {
			version = 0;
		} else if ([baseMode isEqualToString:@"timeline_raw_profile_version_2"]) {
			version = 2;
		} else if ([baseMode isEqualToString:@"timeline_raw_profile_version_3"]) {
			version = 3;
		} else if ([baseMode isEqualToString:@"timeline_raw_profile_version_5"]) {
			version = 5;
		}
		[(GTReplayProfileRequest *)request setProfileDataVersion:version];
	}
	if (((explicitClassName && ![baseMode containsString:@"nil_data"]) ||
	     [baseMode isEqualToString:@"timeline_raw_empty_profile_data"] ||
	     [baseMode isEqualToString:@"timeline_raw_v2_profile_data"] ||
	     [baseMode isEqualToString:@"timeline_raw_v2_uppercase_profile_data"] ||
	     [baseMode hasPrefix:@"timeline_aps_"] ||
	     [baseMode hasPrefix:@"batch_filtered_aps_"] ||
	     [baseMode hasPrefix:@"derived_aps_"] ||
	     [baseMode hasPrefix:@"timeline_profiled_profiler_mode_"] ||
	     [baseMode containsString:@"profiler_raw_url"] ||
	     [baseMode containsString:@"encode_streamdata"]) &&
	    [request respondsToSelector:@selector(setProfileData:)]) {
		if ([baseMode containsString:@"direct_profiledata"]) {
			((void (*)(id, SEL, id))objc_msgSend)(request, @selector(setProfileData:), directProfileDataObject(baseMode));
		} else {
			NSString *profileMode = [baseMode containsString:@"v2_data"] ? @"timeline_raw_v2_uppercase_profile_data" : baseMode;
			[(GTReplayProfileRequest *)request setProfileData:archivedProfileDictionary(profileMode)];
		}
	}
    if ([request respondsToSelector:@selector(setShaderProfiling:)]) {
        [(GTReplayProfileTimeline *)request setShaderProfiling:![baseMode isEqualToString:@"timeline_no_shader_raw"]];
    }
    if ([request respondsToSelector:@selector(setSaveProfilerRaw:)]) {
        [(GTReplayProfileTimeline *)request setSaveProfilerRaw:YES];
    }
    return request;
}

static int runProfile(GTMTLReplayServiceXPCProxy *replayer, NSString *mode, NSString *outDir) {
	currentProfileOutDir = outDir;
    id request = newProfileRequest(mode);
    if (!request) {
        fprintf(stderr, "missing profile request class for %s\n", [mode UTF8String]);
        return 40;
    }
	BOOL encodeStreamData = [mode containsString:@"encode_streamdata"];
	id mutableStreamData = nil;
	if (encodeStreamData) {
		mutableStreamData = newMutableProfilerStreamData(mode);
		fprintf(stdout, "mutable streamData=%s\n", [stringFromObject(mutableStreamData) UTF8String]);
	}
	BOOL noStreamHandler = [mode containsString:@"no_stream"];
    __block int streamResponses = 0;
    if (!noStreamHandler && [request respondsToSelector:@selector(setStreamHandler:)]) {
		[(GTReplayProfileRequest *)request setStreamHandler:^(id response) {
			logResponse(response, @"stream");
			NSData *data = responseData(response);
			if (encodeStreamData && mutableStreamData && data) {
				BOOL added = NO;
				id payload = unarchivedObjectFromData(data);
				if (!payload) {
					payload = data;
				}
				payload = normalizedProfilerPayload(payload);
				@try {
					if ([mode containsString:@"gputimeline"] && [mutableStreamData respondsToSelector:@selector(addGPUTimelineData:)]) {
						added = ((BOOL (*)(id, SEL, id))objc_msgSend)(mutableStreamData, @selector(addGPUTimelineData:), payload);
					} else if ([mode containsString:@"shaderprofiler"] && [mutableStreamData respondsToSelector:@selector(addShaderProfilerData:)]) {
						added = ((BOOL (*)(id, SEL, id))objc_msgSend)(mutableStreamData, @selector(addShaderProfilerData:), payload);
					} else if ([mode containsString:@"batch_filtered"] && [mutableStreamData respondsToSelector:@selector(addBatchIdFilteredCounterData:)]) {
						added = ((BOOL (*)(id, SEL, id))objc_msgSend)(mutableStreamData, @selector(addBatchIdFilteredCounterData:), payload);
					} else if ([mode containsString:@"derived_counter"] && [mutableStreamData respondsToSelector:@selector(addAPSCounterData:)]) {
						added = ((BOOL (*)(id, SEL, id))objc_msgSend)(mutableStreamData, @selector(addAPSCounterData:), payload);
					} else if ([mode containsString:@"timeline"] && [mutableStreamData respondsToSelector:@selector(addAPSTimelineData:)]) {
						added = ((BOOL (*)(id, SEL, id))objc_msgSend)(mutableStreamData, @selector(addAPSTimelineData:), payload);
					} else if ([mutableStreamData respondsToSelector:@selector(addAPSData:)]) {
						added = ((BOOL (*)(id, SEL, id))objc_msgSend)(mutableStreamData, @selector(addAPSData:), payload);
					}
				} @catch (NSException *exception) {
					fprintf(stdout, "streamData add exception=%s\n", [[exception description] UTF8String]);
				}
				fprintf(stdout, "streamData add mode=%s bytes=%lu payloadClass=%s added=%d\n",
				        [mode UTF8String],
				        (unsigned long)[data length],
				        [stringFromObject([payload class]) UTF8String],
				        added ? 1 : 0);
			}
			writeObject(response, outDir, @"stream", streamResponses);
			writeBytes(data, outDir, @"stream", streamResponses++);
		}];
	} else if (noStreamHandler) {
		fprintf(stdout, "stream handler disabled for mode=%s\n", [mode UTF8String]);
	}
	id token = [replayer profile:request];
	fprintf(stdout, "profile mode=%s token=%s\n", [mode UTF8String], [stringFromObject(token) UTF8String]);
	if (token) {
		dumpRuntimeClass(NSStringFromClass([token class]));
	}
	if ([mode containsString:@"query_perf_during"]) {
		fprintf(stdout, "query performance state while profile active begin\n");
		int queryRC = runQueryClass(replayer, outDir, @"GTReplayQueryPerformanceState");
		fprintf(stdout, "query performance state while profile active rc=%d\n", queryRC);
	}
	if ([mode containsString:@"_resume"] && [token respondsToSelector:@selector(resume)]) {
		BOOL resumed = boolValueOrNo(token, @"resume");
		fprintf(stdout, "token resume=%d\n", resumed ? 1 : 0);
	}
	if ([mode containsString:@"proxy_resume"] || [mode containsString:@"proxy_pause_resume"]) {
		uint64_t tokenID = unsignedLongLongValueOrZero(token, @"tokenId");
		fprintf(stdout, "proxy lifecycle tokenID=%llu responds resume=%d pause=%d cancel=%d\n",
		        (unsigned long long)tokenID,
		        [replayer respondsToSelector:@selector(resume:)] ? 1 : 0,
		        [replayer respondsToSelector:@selector(pause:)] ? 1 : 0,
		        [replayer respondsToSelector:@selector(cancel:)] ? 1 : 0);
		if ([mode containsString:@"proxy_pause_resume"] && [replayer respondsToSelector:@selector(pause:)]) {
			BOOL paused = ((BOOL (*)(id, SEL, uint64_t))objc_msgSend)(replayer, @selector(pause:), tokenID);
			fprintf(stdout, "proxy pause=%d\n", paused ? 1 : 0);
			[[NSRunLoop currentRunLoop] runUntilDate:[NSDate dateWithTimeIntervalSinceNow:0.5]];
		}
		if ([replayer respondsToSelector:@selector(resume:)]) {
			BOOL resumed = ((BOOL (*)(id, SEL, uint64_t))objc_msgSend)(replayer, @selector(resume:), tokenID);
			fprintf(stdout, "proxy resume=%d\n", resumed ? 1 : 0);
		}
	}
	if ([mode containsString:@"pause_resume"]) {
		if ([token respondsToSelector:@selector(pause)]) {
			BOOL paused = boolValueOrNo(token, @"pause");
			fprintf(stdout, "token pause=%d\n", paused ? 1 : 0);
		}
		[[NSRunLoop currentRunLoop] runUntilDate:[NSDate dateWithTimeIntervalSinceNow:0.5]];
		if ([token respondsToSelector:@selector(resume)]) {
			BOOL resumed = boolValueOrNo(token, @"resume");
			fprintf(stdout, "token pause_resume resume=%d\n", resumed ? 1 : 0);
		}
	}
	if ([mode hasSuffix:@"_wait_complete"] && [token respondsToSelector:@selector(waitUntilCompleted)]) {
		fprintf(stdout, "token waitUntilCompleted begin\n");
#pragma clang diagnostic push
#pragma clang diagnostic ignored "-Warc-performSelector-leaks"
		[token performSelector:@selector(waitUntilCompleted)];
#pragma clang diagnostic pop
		fprintf(stdout, "token waitUntilCompleted done\n");
        [[NSRunLoop currentRunLoop] runUntilDate:[NSDate dateWithTimeIntervalSinceNow:1.0]];
	} else if ([mode hasSuffix:@"_run_5s"] || [mode hasSuffix:@"_run_10s"] || [mode hasSuffix:@"_run_30s"] || [mode hasSuffix:@"_run_60s"] || [mode hasSuffix:@"_run_90s"]) {
		double seconds = 30.0;
		if ([mode hasSuffix:@"_run_5s"]) {
			seconds = 5.0;
		} else if ([mode hasSuffix:@"_run_10s"]) {
			seconds = 10.0;
		} else if ([mode hasSuffix:@"_run_60s"]) {
			seconds = 60.0;
		} else if ([mode hasSuffix:@"_run_90s"]) {
			seconds = 90.0;
		}
		fprintf(stdout, "token run loop collection begin seconds=%.0f\n", seconds);
		[[NSRunLoop currentRunLoop] runUntilDate:[NSDate dateWithTimeIntervalSinceNow:seconds]];
		fprintf(stdout, "token run loop collection done\n");
	} else {
		[[NSRunLoop currentRunLoop] runUntilDate:[NSDate dateWithTimeIntervalSinceNow:4.0]];
	}
	if ([mode containsString:@"flush_rpackets"] && [replayer respondsToSelector:@selector(flushRpackets)]) {
		id packets = [replayer flushRpackets];
		fprintf(stdout, "flushRpackets class=%s object=%s\n",
		        [stringFromObject([packets class]) UTF8String],
		        [stringFromObject(packets) UTF8String]);
		writeObject(packets, outDir, @"flush-rpackets", 0);
		if ([packets isKindOfClass:[NSData class]]) {
			writeBytes((NSData *)packets, outDir, @"flush-rpackets-data", 0);
		}
	}
	if (![mode hasSuffix:@"_wait_complete"] && [token respondsToSelector:@selector(cancel)]) {
		BOOL cancelled = boolValueOrNo(token, @"cancel");
		fprintf(stdout, "token cancel=%d\n", cancelled ? 1 : 0);
    }
	if (([mode containsString:@"proxy_resume"] || [mode containsString:@"proxy_pause_resume"]) && [replayer respondsToSelector:@selector(cancel:)]) {
		uint64_t tokenID = unsignedLongLongValueOrZero(token, @"tokenId");
		BOOL proxyCancelled = ((BOOL (*)(id, SEL, uint64_t))objc_msgSend)(replayer, @selector(cancel:), tokenID);
		fprintf(stdout, "proxy cancel=%d\n", proxyCancelled ? 1 : 0);
	}
	if (encodeStreamData && mutableStreamData) {
		NSString *rawDir = [outDir stringByAppendingPathComponent:@"encoded.gpuprofiler_raw"];
		mkdir([rawDir fileSystemRepresentation], 0777);
		NSURL *streamURL = [NSURL fileURLWithPath:rawDir isDirectory:YES];
		NSError *encodeError = nil;
		id encoded = nil;
		if ([mutableStreamData respondsToSelector:@selector(encode:error:)]) {
			encoded = ((id (*)(id, SEL, id, NSError **))objc_msgSend)(mutableStreamData, @selector(encode:error:), streamURL, &encodeError);
		}
		NSString *streamDataPath = [rawDir stringByAppendingPathComponent:@"streamData"];
		fprintf(stdout, "streamData encode rawDir=%s encoded=%s err=%s streamData_exists=%d\n",
		        [rawDir UTF8String],
		        [stringFromObject(encoded) UTF8String],
		        [[encodeError description] UTF8String],
		        [[NSFileManager defaultManager] fileExistsAtPath:streamDataPath] ? 1 : 0);
	}
    fprintf(stdout, "stream responses=%d\n", streamResponses);
	return 0;
}

static int runProfileAllRuntimeClasses(GTMTLReplayServiceXPCProxy *replayer, NSString *outDir) {
	void *replayHandle = dlopen("/System/Library/PrivateFrameworks/GPUToolsReplay.framework/GPUToolsReplay", RTLD_NOW);
	fprintf(stdout, "GPUToolsReplay dlopen=%p err=%s\n", replayHandle, dlerror());
	int count = objc_getClassList(NULL, 0);
	if (count <= 0) {
		fprintf(stdout, "objc class count=%d\n", count);
		return 0;
	}
	Class *classes = (Class *)calloc((size_t)count, sizeof(Class));
	int copied = objc_getClassList(classes, count);
	NSMutableArray *names = [NSMutableArray array];
	for (int i = 0; i < copied; i++) {
		const char *rawName = class_getName(classes[i]);
		if (!rawName) {
			continue;
		}
		NSString *name = [NSString stringWithUTF8String:rawName];
		Class cls = classes[i];
		Class requestClass = NSClassFromString(@"GTReplayProfileRequest");
		if ([name hasPrefix:@"GTReplayProfile"] && requestClass && (cls == requestClass || class_getSuperclass(cls) == requestClass)) {
			[names addObject:name];
		}
	}
	free(classes);
	[names sortUsingSelector:@selector(compare:)];
	for (NSString *className in names) {
		NSString *classOutDir = [outDir stringByAppendingPathComponent:className];
		mkdir([classOutDir fileSystemRepresentation], 0777);
		fprintf(stdout, "profile runtime class=%s\n", [className UTF8String]);
		runProfile(replayer, [@"profile_class_" stringByAppendingString:className], classOutDir);
	}
	return 0;
}

static NSArray *interestingRuntimeClasses(void) {
    return @[
        @"GTReplayRequest",
        @"GTReplayRequestBatch",
        @"GTReplayResponse",
        @"GTReplayQueryResourceUsage",
        @"GTReplayQueryConfiguration",
        @"GTReplayConfiguration",
        @"GTReplayProfileRequest",
        @"GTReplayProfileTimeline",
        @"GTReplayProfileDerivedCounters",
        @"GTReplayProfileBatchFilteredCounters",
        @"GTReplayProfileReplyStream",
        @"GTShaderProfilerSessionRequest",
        @"GTGPUAPSConfig",
        @"GTMutableShaderProfilerStreamData",
        @"GTShaderProfilerStreamData",
        @"GTUSCSamplingStreamingManagerHelper",
        @"GTUSCSamplingStreamingManager",
        @"GTUSCSamplingProfiler",
        @"GTReplayFetch",
        @"GTReplayFetchBuffer",
        @"GTReplayFetchTexture",
        @"GTReplayFetchThreadgroup",
        @"GTReplayFetchPipelineBinaries",
        @"GTReplayFetchPostVertex",
        @"GTReplayFetchWireframe",
        @"GTReplayFetchInto",
        @"GTReplayFetchIntoBuffer",
        @"GTReplayFetchIntoTexture",
        @"GTReplayDecode",
        @"GTReplayDecodeAB",
        @"GTReplayDecodeICB",
        @"GTReplayDecodeGenericAccelerationStructure",
        @"GTReplayUpdate",
        @"GTReplayUpdateConfiguration",
        @"GTReplayUpdateLibrary",
        @"GTReplayUpdateLibraryCache",
        @"GTReplayDisplay",
        @"GTReplayDisplayAttachment",
        @"GTBulkDataServiceXPCProxy",
        @"GTBulkDataTransferOptions",
        @"GTBulkDataUploadDescriptor",
        @"GTBulkDataUploadHandle",
        @"GTMTLReplayServiceXPCProxy"
    ];
}

static NSArray *runtimeClassPrefixes(void) {
    return @[
        @"GTReplay",
        @"GTBulk",
        @"GTGPUAPS",
        @"GTMutableShaderProfiler",
        @"GTShaderProfiler",
        @"GTUSC",
        @"GRC",
        @"DYGPU",
        @"MTLReplay"
    ];
}

static NSArray *runtimeClassSubstrings(void) {
    return @[
        @"Profile",
        @"Profiling",
        @"Profiler",
        @"Performance",
        @"Counter",
        @"Timeline",
        @"APS",
        @"State",
        @"Enabled"
    ];
}

static void dumpRuntimeClass(NSString *className) {
    Class cls = NSClassFromString(className);
    fprintf(stdout, "CLASS %s present=%d\n", [className UTF8String], cls ? 1 : 0);
    if (!cls) {
        return;
    }
    Class superCls = class_getSuperclass(cls);
    fprintf(stdout, "  super=%s\n", superCls ? class_getName(superCls) : "");
    unsigned int propertyCount = 0;
    objc_property_t *properties = class_copyPropertyList(cls, &propertyCount);
    for (unsigned int i = 0; i < propertyCount; i++) {
        const char *name = property_getName(properties[i]);
        const char *attrs = property_getAttributes(properties[i]);
        fprintf(stdout, "  property %s attrs=%s\n", name ? name : "", attrs ? attrs : "");
    }
    free(properties);
    unsigned int ivarCount = 0;
    Ivar *ivars = class_copyIvarList(cls, &ivarCount);
    for (unsigned int i = 0; i < ivarCount; i++) {
        const char *name = ivar_getName(ivars[i]);
        const char *type = ivar_getTypeEncoding(ivars[i]);
        fprintf(stdout, "  ivar %s type=%s offset=%td\n", name ? name : "", type ? type : "", ivar_getOffset(ivars[i]));
    }
    free(ivars);
    unsigned int methodCount = 0;
    Method *methods = class_copyMethodList(cls, &methodCount);
    for (unsigned int i = 0; i < methodCount; i++) {
        SEL selector = method_getName(methods[i]);
        const char *types = method_getTypeEncoding(methods[i]);
        fprintf(stdout, "  method %s types=%s\n", selector ? sel_getName(selector) : "", types ? types : "");
    }
    free(methods);
}

static int runRuntimeDump(void) {
    void *replayHandle = dlopen("/System/Library/PrivateFrameworks/GPUToolsReplay.framework/GPUToolsReplay", RTLD_NOW);
    fprintf(stdout, "GPUToolsReplay dlopen=%p err=%s\n", replayHandle, dlerror());
    for (NSString *className in interestingRuntimeClasses()) {
        dumpRuntimeClass(className);
    }
    return 0;
}

static int runRuntimeClassPrefixDump(void) {
    void *replayHandle = dlopen("/System/Library/PrivateFrameworks/GPUToolsReplay.framework/GPUToolsReplay", RTLD_NOW);
    fprintf(stdout, "GPUToolsReplay dlopen=%p err=%s\n", replayHandle, dlerror());
    int count = objc_getClassList(NULL, 0);
    if (count <= 0) {
        fprintf(stdout, "objc class count=%d\n", count);
        return 0;
    }
    Class *classes = (Class *)calloc((size_t)count, sizeof(Class));
    int copied = objc_getClassList(classes, count);
    NSMutableArray *names = [NSMutableArray array];
    NSArray *prefixes = runtimeClassPrefixes();
    for (int i = 0; i < copied; i++) {
        const char *rawName = class_getName(classes[i]);
        if (!rawName) {
            continue;
        }
        NSString *name = [NSString stringWithUTF8String:rawName];
        for (NSString *prefix in prefixes) {
            if ([name hasPrefix:prefix]) {
                [names addObject:name];
                break;
            }
        }
    }
    free(classes);
    [names sortUsingSelector:@selector(compare:)];
    fprintf(stdout, "runtime class prefix matches=%lu\n", (unsigned long)[names count]);
    for (NSString *className in names) {
        dumpRuntimeClass(className);
    }
    return 0;
}

static int runRuntimeClassProfileSubstringDump(void) {
    void *replayHandle = dlopen("/System/Library/PrivateFrameworks/GPUToolsReplay.framework/GPUToolsReplay", RTLD_NOW);
    fprintf(stdout, "GPUToolsReplay dlopen=%p err=%s\n", replayHandle, dlerror());
    int count = objc_getClassList(NULL, 0);
    if (count <= 0) {
        fprintf(stdout, "objc class count=%d\n", count);
        return 0;
    }
    Class *classes = (Class *)calloc((size_t)count, sizeof(Class));
    int copied = objc_getClassList(classes, count);
    NSMutableArray *names = [NSMutableArray array];
    NSArray *substrings = runtimeClassSubstrings();
    for (int i = 0; i < copied; i++) {
        const char *rawName = class_getName(classes[i]);
        if (!rawName) {
            continue;
        }
        NSString *name = [NSString stringWithUTF8String:rawName];
        for (NSString *substring in substrings) {
            if ([name rangeOfString:substring options:NSCaseInsensitiveSearch].location != NSNotFound) {
                [names addObject:name];
                break;
            }
        }
    }
    free(classes);
    [names sortUsingSelector:@selector(compare:)];
    fprintf(stdout, "runtime class substring matches=%lu\n", (unsigned long)[names count]);
    for (NSString *className in names) {
        dumpRuntimeClass(className);
    }
    return 0;
}

static BOOL loadXcodeGPUToolsFrameworks(void);

static int runXcodeShaderProfilerRuntimeDump(void) {
	loadXcodeGPUToolsFrameworks();
	int count = objc_getClassList(NULL, 0);
	if (count <= 0) {
		fprintf(stdout, "objc class count=%d\n", count);
		return 0;
	}
	Class *classes = (Class *)calloc((size_t)count, sizeof(Class));
	int copied = objc_getClassList(classes, count);
	NSMutableArray *names = [NSMutableArray array];
	NSArray *prefixes = @[
		@"DYCaptureArchive",
		@"DYDeviceInfo",
		@"DYGTDeviceInfo",
		@"DYPShaderProfiler",
		@"DYPMTLShaderProfiler",
		@"DYShaderProfiler",
		@"DYMTLShaderProfiler",
		@"GTPlatformDeviceInfo",
		@"DYPAnalyzerShaders",
		@"DYPMTLAnalyzerTaskShaderProfiler",
		@"GPUShaderProfiler"
	];
	for (int i = 0; i < copied; i++) {
		const char *rawName = class_getName(classes[i]);
		if (!rawName) {
			continue;
		}
		NSString *name = [NSString stringWithUTF8String:rawName];
		for (NSString *prefix in prefixes) {
			if ([name hasPrefix:prefix]) {
				[names addObject:name];
				break;
			}
		}
	}
	free(classes);
	[names sortUsingSelector:@selector(compare:)];
	fprintf(stdout, "xcode shader profiler runtime matches=%lu\n", (unsigned long)[names count]);
	for (NSString *className in names) {
		dumpRuntimeClass(className);
	}
	return 0;
}

static BOOL loadXcodeGPUToolsFrameworks(void) {
	NSArray *frameworks = @[
		@"/Applications/Xcode.app/Contents/SharedFrameworks/GPUToolsCore.framework/GPUToolsCore",
		@"/Applications/Xcode.app/Contents/SharedFrameworks/GPUTools.framework/GPUTools",
		@"/Applications/Xcode.app/Contents/SharedFrameworks/GLToolsCore.framework/GLToolsCore",
		@"/Applications/Xcode.app/Contents/SharedFrameworks/GPUToolsPlatform.framework/GPUToolsPlatform",
		@"/Applications/Xcode.app/Contents/SharedFrameworks/GPUToolsServices.framework/GPUToolsServices",
		@"/Applications/Xcode.app/Contents/SharedFrameworks/GPUToolsShaderProfiler.framework/GPUToolsShaderProfiler",
		@"/Applications/Xcode.app/Contents/SharedFrameworks/MTLTools.framework/MTLTools",
		@"/Applications/Xcode.app/Contents/SharedFrameworks/MTLToolsAnalysisEngine.framework/MTLToolsAnalysisEngine",
		@"/Applications/Xcode.app/Contents/Developer/Library/Xcode/Agents/GPUToolsAgent.app/Contents/MacOS/GPUToolsAgent",
		@"/Applications/Xcode.app/Contents/Developer/Platforms/MacOSX.platform/Developer/Library/GPUToolsPlatform/PlugIns/GPUToolsPlatformSupport-OSX.gtpplugin_osx/Contents/MacOS/GPUToolsPlatformSupport-OSX"
	];
	BOOL anyLoaded = NO;
	for (NSString *path in frameworks) {
		void *handle = dlopen([path fileSystemRepresentation], RTLD_NOW | RTLD_GLOBAL);
		fprintf(stdout, "xcode dlopen %s handle=%p err=%s\n", [path UTF8String], handle, dlerror());
		anyLoaded = anyLoaded || (handle != NULL);
	}
	return anyLoaded;
}

static id newXcodeDeviceInfo(void) {
	Class deviceClass = NSClassFromString(@"DYDeviceInfo");
	if (!deviceClass) {
		deviceClass = NSClassFromString(@"DYGTDeviceInfo");
	}
	id deviceInfo = deviceClass ? [deviceClass new] : nil;
	if (!deviceInfo) {
		return nil;
	}
	@try {
		if ([deviceInfo respondsToSelector:@selector(setPlatform:)]) {
			((void (*)(id, SEL, int))objc_msgSend)(deviceInfo, @selector(setPlatform:), 0);
		}
		if ([deviceInfo respondsToSelector:@selector(setNativePointerSize:)]) {
			((void (*)(id, SEL, uint32_t))objc_msgSend)(deviceInfo, @selector(setNativePointerSize:), 8);
		}
		if ([deviceInfo respondsToSelector:@selector(setRuntimeIdentifier:)]) {
			((void (*)(id, SEL, uint64_t))objc_msgSend)(deviceInfo, @selector(setRuntimeIdentifier:), 1);
		}
		if ([deviceInfo respondsToSelector:@selector(setPermanentIdentifier:)]) {
			[deviceInfo setValue:@"headless-local-metal-device" forKey:@"permanentIdentifier"];
		}
		if ([deviceInfo respondsToSelector:@selector(setName:)]) {
			[deviceInfo setValue:@"Apple GPU" forKey:@"name"];
		}
		if ([deviceInfo respondsToSelector:@selector(setProductType:)]) {
			[deviceInfo setValue:@"Mac" forKey:@"productType"];
		}
		if ([deviceInfo respondsToSelector:@selector(setHostProductType:)]) {
			[deviceInfo setValue:@"Mac" forKey:@"hostProductType"];
		}
		if ([deviceInfo respondsToSelector:@selector(setVersion:)]) {
			[deviceInfo setValue:[[NSProcessInfo processInfo] operatingSystemVersionString] forKey:@"version"];
		}
		if ([deviceInfo respondsToSelector:@selector(setHostVersion:)]) {
			[deviceInfo setValue:[[NSProcessInfo processInfo] operatingSystemVersionString] forKey:@"hostVersion"];
		}
		if ([deviceInfo respondsToSelector:@selector(setBuild:)]) {
			[deviceInfo setValue:@"headless" forKey:@"build"];
		}
		if ([deviceInfo respondsToSelector:@selector(setMetalVersion:)]) {
			[deviceInfo setValue:@"Metal" forKey:@"metalVersion"];
		}
	} @catch (NSException *exception) {
		fprintf(stdout, "xcode deviceInfo setup exception=%s\n", [[exception description] UTF8String]);
	}
	return deviceInfo;
}

static id openXcodeCaptureArchive(NSString *tracePath, NSError **archiveError) {
	Class archiveClass = NSClassFromString(@"DYCaptureArchive");
	if (!archiveClass) {
		fprintf(stderr, "missing DYCaptureArchive class\n");
		return nil;
	}
	NSURL *traceURL = [NSURL fileURLWithPath:tracePath];
	id archive = nil;
	@try {
		archive = ((id (*)(id, SEL, id, uint64_t, NSError **))objc_msgSend)([archiveClass alloc], @selector(initWithURL:options:error:), traceURL, 0, archiveError);
	} @catch (NSException *exception) {
		fprintf(stdout, "xcode capture archive init exception=%s\n", [[exception description] UTF8String]);
		return nil;
	}
	fprintf(stdout, "xcode capture archive=%s err=%s\n",
	        [stringFromObject(archive) UTF8String],
	        (archiveError && *archiveError) ? [[*archiveError description] UTF8String] : "");
	return archive;
}

static NSArray *archiveFilenamesWithPrefix(id archive, NSString *prefix) {
	if (!archive || ![archive respondsToSelector:@selector(filenamesWithPrefix:error:)]) {
		return @[];
	}
	NSError *error = nil;
	NSArray *names = nil;
	@try {
		names = ((id (*)(id, SEL, id, NSError **))objc_msgSend)(archive, @selector(filenamesWithPrefix:error:), prefix, &error);
	} @catch (NSException *exception) {
		fprintf(stdout, "archive filenamesWithPrefix prefix=%s exception=%s\n", [prefix UTF8String], [[exception description] UTF8String]);
		return @[];
	}
	fprintf(stdout, "archive filenamesWithPrefix prefix=%s count=%lu err=%s\n",
	        [prefix UTF8String],
	        (unsigned long)([names respondsToSelector:@selector(count)] ? [names count] : 0),
	        [[error description] UTF8String]);
	return [names isKindOfClass:[NSArray class]] ? names : @[];
}

static NSData *archiveDataForFilename(id archive, NSString *filename) {
	if (!archive || !filename || ![archive respondsToSelector:@selector(copyDataForFilename:error:)]) {
		return nil;
	}
	NSError *error = nil;
	NSData *data = nil;
	@try {
		data = ((id (*)(id, SEL, id, NSError **))objc_msgSend)(archive, @selector(copyDataForFilename:error:), filename, &error);
	} @catch (NSException *exception) {
		fprintf(stdout, "archive copyData filename=%s exception=%s\n", [filename UTF8String], [[exception description] UTF8String]);
		return nil;
	}
	fprintf(stdout, "archive copyData filename=%s bytes=%lu err=%s\n",
	        [filename UTF8String],
	        (unsigned long)([data isKindOfClass:[NSData class]] ? [data length] : 0),
	        [[error description] UTF8String]);
	return [data isKindOfClass:[NSData class]] ? data : nil;
}

static id unarchivedArchiveObjectForFilename(id archive, NSString *filename) {
	NSData *data = archiveDataForFilename(archive, filename);
	if (![data isKindOfClass:[NSData class]] || [data length] == 0) {
		return nil;
	}
	id object = unarchivedObjectFromData(data);
	if ([object isKindOfClass:[NSArray class]] && [(NSArray *)object count] == 1) {
		return [(NSArray *)object objectAtIndex:0];
	}
	return object;
}

typedef struct {
	uint64_t field0;
	uint64_t field1;
	uint32_t field2;
	uint32_t field3;
	uint32_t field4;
	uint32_t field5;
} GTProbeArchiveFileInfo;

typedef struct {
	uint64_t sequenceID;
	uint64_t startTimestamp;
	uint64_t endOffsetMicros;
	uint32_t labelStringIndex;
	uint32_t commandBufferIndex;
	uint32_t flags;
	uint32_t reserved;
} GTProbeEncoderInfoRow;

typedef struct {
	uint32_t functionIndex;
	uint32_t subCommandIndex;
	uint32_t reserved0;
	uint32_t pipelineIndex;
	uint64_t endOffsetMicros;
	uint32_t encoderIndex;
	int32_t reserved1;
} GTProbeGPUCommandInfoRow;

typedef struct {
	uint64_t sequenceID;
	uint64_t startTimestamp;
	uint64_t endOffsetMicros;
	uint32_t flags;
	uint32_t encoderCount;
} GTProbeCommandBufferInfoRow;

static void logArchiveInfoForFilename(id archive, NSString *filename) {
	if (!archive || !filename || ![archive respondsToSelector:@selector(getInfo:forFilename:error:)]) {
		return;
	}
	GTProbeArchiveFileInfo info;
	memset(&info, 0, sizeof(info));
	NSError *error = nil;
	BOOL ok = NO;
	@try {
		ok = ((BOOL (*)(id, SEL, GTProbeArchiveFileInfo *, id, NSError **))objc_msgSend)(archive, @selector(getInfo:forFilename:error:), &info, filename, &error);
	} @catch (NSException *exception) {
		fprintf(stdout, "archive getInfo filename=%s exception=%s\n", [filename UTF8String], [[exception description] UTF8String]);
		return;
	}
	fprintf(stdout, "archive getInfo filename=%s ok=%d fields=%llu,%llu,%u,%u,%u,%u err=%s\n",
	        [filename UTF8String],
	        ok ? 1 : 0,
	        (unsigned long long)info.field0,
	        (unsigned long long)info.field1,
	        info.field2,
	        info.field3,
	        info.field4,
	        info.field5,
	        [[error description] UTF8String]);
}

static int runXcodeCaptureArchiveInspect(NSString *tracePath, NSString *outDir) {
	if (!loadXcodeGPUToolsFrameworks()) {
		fprintf(stderr, "failed to load Xcode GPUTools frameworks\n");
		return 84;
	}
	Class archiveClass = NSClassFromString(@"DYCaptureArchive");
	if (!archiveClass) {
		fprintf(stderr, "missing DYCaptureArchive class\n");
		return 85;
	}
	dumpRuntimeClass(@"DYCaptureArchive");
	if ([archiveClass respondsToSelector:@selector(standardFunctionStreamFilenamePrefixes)]) {
		@try {
			id prefixes = ((id (*)(id, SEL))objc_msgSend)(archiveClass, @selector(standardFunctionStreamFilenamePrefixes));
			fprintf(stdout, "archive standardFunctionStreamFilenamePrefixes=%s\n", [stringFromObject(prefixes) UTF8String]);
		} @catch (NSException *exception) {
			fprintf(stdout, "archive standardFunctionStreamFilenamePrefixes exception=%s\n", [[exception description] UTF8String]);
		}
	}
	if ([archiveClass respondsToSelector:@selector(standardFunctionStreamFilenamePredicate)]) {
		@try {
			id predicate = ((id (*)(id, SEL))objc_msgSend)(archiveClass, @selector(standardFunctionStreamFilenamePredicate));
			fprintf(stdout, "archive standardFunctionStreamFilenamePredicate=%s\n", [stringFromObject(predicate) UTF8String]);
		} @catch (NSException *exception) {
			fprintf(stdout, "archive standardFunctionStreamFilenamePredicate exception=%s\n", [[exception description] UTF8String]);
		}
	}
	NSError *archiveError = nil;
	id archive = openXcodeCaptureArchive(tracePath, &archiveError);
	if (!archive) {
		return 86;
	}
	NSArray *metadataKeys = @[
		@"DYCaptureEngine.captured_frames_count",
		@"DYCaptureSession.deviceId",
		@"DYCaptureSession.graphics_api",
		@"native_pointer_size",
		@"DYCaptureSession.product_type",
		@"DYCaptureSession.os_build",
		@"DYCaptureSession.platform",
		@"DYCaptureSession.gpu_name",
		@"DYCaptureSession.device_name"
	];
	for (NSString *key in metadataKeys) {
		if (![archive respondsToSelector:@selector(metadataValueForKey:)]) {
			break;
		}
		@try {
			id value = ((id (*)(id, SEL, id))objc_msgSend)(archive, @selector(metadataValueForKey:), key);
			fprintf(stdout, "archive metadata key=%s value=%s\n", [key UTF8String], [stringFromObject(value) UTF8String]);
		} @catch (NSException *exception) {
			fprintf(stdout, "archive metadata key=%s exception=%s\n", [key UTF8String], [[exception description] UTF8String]);
		}
	}
	NSUInteger filenameCount = 0;
	if ([archive respondsToSelector:@selector(countOfFilenames)]) {
		@try {
			filenameCount = ((NSUInteger (*)(id, SEL))objc_msgSend)(archive, @selector(countOfFilenames));
			fprintf(stdout, "archive countOfFilenames=%lu\n", (unsigned long)filenameCount);
		} @catch (NSException *exception) {
			fprintf(stdout, "archive countOfFilenames exception=%s\n", [[exception description] UTF8String]);
		}
	}
	NSMutableArray *sampleNames = [NSMutableArray array];
	if (filenameCount > 0 && [archive respondsToSelector:@selector(objectInFilenamesAtIndex:)]) {
		NSUInteger limit = MIN(filenameCount, (NSUInteger)80);
		for (NSUInteger i = 0; i < limit; i++) {
			@try {
				id name = ((id (*)(id, SEL, NSUInteger))objc_msgSend)(archive, @selector(objectInFilenamesAtIndex:), i);
				if (name) {
					[sampleNames addObject:name];
					fprintf(stdout, "archive filename[%lu]=%s\n", (unsigned long)i, [stringFromObject(name) UTF8String]);
				}
			} @catch (NSException *exception) {
				fprintf(stdout, "archive filename[%lu] exception=%s\n", (unsigned long)i, [[exception description] UTF8String]);
			}
		}
	}
	if ([archiveClass respondsToSelector:@selector(standardFunctionStreamFilenamePredicate)] &&
	    [archive respondsToSelector:@selector(filenamesWithPredicate:error:)]) {
		@try {
			id predicate = ((id (*)(id, SEL))objc_msgSend)(archiveClass, @selector(standardFunctionStreamFilenamePredicate));
			NSError *predicateError = nil;
			NSArray *names = ((id (*)(id, SEL, id, NSError **))objc_msgSend)(archive, @selector(filenamesWithPredicate:error:), predicate, &predicateError);
			fprintf(stdout, "archive standardFunctionStream filenames count=%lu err=%s\n",
			        (unsigned long)([names respondsToSelector:@selector(count)] ? [names count] : 0),
			        [[predicateError description] UTF8String]);
			NSUInteger limit = MIN((NSUInteger)[names count], (NSUInteger)80);
			for (NSUInteger i = 0; i < limit; i++) {
				fprintf(stdout, "archive standardFunction filename[%lu]=%s\n",
				        (unsigned long)i,
				        [stringFromObject([names objectAtIndex:i]) UTF8String]);
			}
		} @catch (NSException *exception) {
			fprintf(stdout, "archive standardFunctionStream filenames exception=%s\n", [[exception description] UTF8String]);
		}
	}
	NSArray *prefixes = @[
		@"(control", @"(device", @"startup", @"state", @"capture", @"unsorted-capture", @"unsortedcapture",
		@"device-resources", @"delta-device-resources", @"end-device-resources",
		@"sharegroup", @"delta-sharegroup", @"end-sharegroup", @"delta-state", @"end-state",
		@"function", @"functions", @"shader", @"Shader", @"pipeline", @"Pipeline", @"trace", @"MTL", @"store"
	];
	NSMutableOrderedSet *candidateNames = [NSMutableOrderedSet orderedSet];
	for (NSString *prefix in prefixes) {
		NSArray *names = archiveFilenamesWithPrefix(archive, prefix);
		NSUInteger limit = MIN((NSUInteger)[names count], (NSUInteger)40);
		for (NSUInteger i = 0; i < limit; i++) {
			id name = [names objectAtIndex:i];
			fprintf(stdout, "archive prefix=%s filename[%lu]=%s\n", [prefix UTF8String], (unsigned long)i, [stringFromObject(name) UTF8String]);
			if ([name isKindOfClass:[NSString class]]) {
				[candidateNames addObject:name];
			}
		}
	}
	for (NSString *name in @[@"(control device info)", @"(device info)", @"(device profile)", @"capture", @"unsorted-capture"]) {
		logArchiveInfoForFilename(archive, name);
		[candidateNames addObject:name];
	}
	int copied = 0;
	for (NSString *name in candidateNames) {
		if (copied >= 20) {
			break;
		}
		NSString *lower = [name lowercaseString];
		BOOL interesting = [lower containsString:@"device"] || [lower containsString:@"function"] || [lower containsString:@"shader"] || [lower containsString:@"pipeline"] || [lower containsString:@"profile"] || [lower containsString:@"trace"] || [lower isEqualToString:@"capture"] || [lower isEqualToString:@"unsorted-capture"];
		if (!interesting) {
			continue;
		}
		NSData *data = archiveDataForFilename(archive, name);
		if ([data length] > 0 && [data length] <= 65536) {
			writeBytes(data, outDir, @"xcode-capture-archive-data", copied++);
		}
	}
	if ([archive respondsToSelector:@selector(close)]) {
#pragma clang diagnostic push
#pragma clang diagnostic ignored "-Warc-performSelector-leaks"
		[archive performSelector:@selector(close)];
#pragma clang diagnostic pop
	}
	return 0;
}

static int runXcodeShaderProfilerArchivePayload(NSString *tracePath, NSString *outDir, NSString *mode) {
	if (!loadXcodeGPUToolsFrameworks()) {
		fprintf(stderr, "failed to load Xcode GPUTools frameworks\n");
		return 73;
	}
	Class archiveClass = NSClassFromString(@"DYCaptureArchive");
	Class profilerClass = [mode containsString:@"_base_"] ? NSClassFromString(@"DYPMTLShaderProfiler_OSX") : NSClassFromString(@"DYPMTLShaderProfilerGPUDesktopReplayer_OSX");
	if (!archiveClass || !profilerClass) {
		fprintf(stderr, "missing Xcode archive/profiler classes archive=%p profiler=%p\n", archiveClass, profilerClass);
		return 74;
	}
	dumpRuntimeClass(@"DYCaptureArchive");
	dumpRuntimeClass(@"DYPMTLShaderProfilerGPUDesktopReplayer_OSX");
	dumpRuntimeClass(@"DYPMTLShaderProfiler_OSX");
	NSURL *traceURL = [NSURL fileURLWithPath:tracePath];
	NSError *archiveError = nil;
	id archive = nil;
	@try {
		archive = ((id (*)(id, SEL, id, uint64_t, NSError **))objc_msgSend)([archiveClass alloc], @selector(initWithURL:options:error:), traceURL, 0, &archiveError);
	} @catch (NSException *exception) {
		fprintf(stdout, "xcode shader profiler archive init exception=%s\n", [[exception description] UTF8String]);
		return 75;
	}
	fprintf(stdout, "xcode shader profiler archive=%s err=%s\n",
	        [stringFromObject(archive) UTF8String],
	        [[archiveError description] UTF8String]);
	if (!archive) {
		return 76;
	}
	id profiler = [profilerClass new];
	fprintf(stdout, "xcode shader profiler object=%s\n", [stringFromObject(profiler) UTF8String]);
	if ([mode containsString:@"_device_"] && [profiler respondsToSelector:@selector(_setDeviceInfo:)]) {
		id deviceInfo = newXcodeDeviceInfo();
		fprintf(stdout, "xcode shader profiler deviceInfo=%s\n", [stringFromObject(deviceInfo) UTF8String]);
		@try {
			((void (*)(id, SEL, id))objc_msgSend)(profiler, @selector(_setDeviceInfo:), deviceInfo);
		} @catch (NSException *exception) {
			fprintf(stdout, "xcode shader profiler _setDeviceInfo exception=%s\n", [[exception description] UTF8String]);
		}
	}
	if ([mode containsString:@"_archiveinfo_"] && [profiler respondsToSelector:@selector(_setDeviceInfo:)]) {
		fprintf(stdout, "xcode shader profiler deviceInfo=archive\n");
		@try {
			((void (*)(id, SEL, id))objc_msgSend)(profiler, @selector(_setDeviceInfo:), archive);
		} @catch (NSException *exception) {
			fprintf(stdout, "xcode shader profiler _setDeviceInfo archive exception=%s\n", [[exception description] UTF8String]);
		}
	}
	if ([mode containsString:@"_deviceprofile_"] && [profiler respondsToSelector:@selector(_setDeviceInfo:)]) {
		id deviceProfile = unarchivedArchiveObjectForFilename(archive, @"(device profile)");
		if (!deviceProfile) {
			deviceProfile = unarchivedArchiveObjectForFilename(archive, @"(device info)");
		}
		fprintf(stdout, "xcode shader profiler deviceProfile class=%s object=%s\n",
		        [stringFromObject([deviceProfile class]) UTF8String],
		        [stringFromObject(deviceProfile) UTF8String]);
		if (deviceProfile) {
			dumpRuntimeClass(NSStringFromClass([deviceProfile class]));
		}
		@try {
			((void (*)(id, SEL, id))objc_msgSend)(profiler, @selector(_setDeviceInfo:), deviceProfile);
		} @catch (NSException *exception) {
			fprintf(stdout, "xcode shader profiler _setDeviceInfo deviceProfile exception=%s\n", [[exception description] UTF8String]);
		}
	}
	if ([mode containsString:@"_deviceproxy_"] && [profiler respondsToSelector:@selector(_setDeviceInfo:)]) {
		id deviceProfile = unarchivedArchiveObjectForFilename(archive, @"(device profile)");
		if (!deviceProfile) {
			deviceProfile = unarchivedArchiveObjectForFilename(archive, @"(device info)");
		}
		id proxy = [[GTProbeDeviceInfoProxy alloc] initWithBacking:deviceProfile archive:archive];
		fprintf(stdout, "xcode shader profiler deviceProxy class=%s object=%s\n",
		        [stringFromObject([proxy class]) UTF8String],
		        [stringFromObject(proxy) UTF8String]);
		@try {
			((void (*)(id, SEL, id))objc_msgSend)(profiler, @selector(_setDeviceInfo:), proxy);
		} @catch (NSException *exception) {
			fprintf(stdout, "xcode shader profiler _setDeviceInfo deviceProxy exception=%s\n", [[exception description] UTF8String]);
		}
	}
	NSArray *selectors = nil;
	if ([mode containsString:@"_gather_"]) {
		selectors = @[@"gatherStatisticsFromArchive:"];
	} else if ([mode containsString:@"_construct_"]) {
		selectors = @[@"constructPayloadFromArchive:"];
	} else {
		selectors = @[
			@"constructPayloadFromArchive:",
			@"gatherStatisticsFromArchive:"
		];
	}
	int idx = 0;
	NSMutableArray *payloads = [NSMutableArray array];
	NSMutableArray *payloadLabels = [NSMutableArray array];
	for (NSString *selectorName in selectors) {
		SEL selector = NSSelectorFromString(selectorName);
		if (![profiler respondsToSelector:selector]) {
			fprintf(stdout, "xcode shader profiler selector missing=%s\n", [selectorName UTF8String]);
			continue;
		}
		id payload = nil;
		@try {
			payload = ((id (*)(id, SEL, id))objc_msgSend)(profiler, selector, archive);
		} @catch (NSException *exception) {
			fprintf(stdout, "xcode shader profiler selector=%s exception=%s\n",
			        [selectorName UTF8String],
			        [[exception description] UTF8String]);
			continue;
		}
		fprintf(stdout, "xcode shader profiler selector=%s payloadClass=%s payload=%s\n",
		        [selectorName UTF8String],
		        [stringFromObject([payload class]) UTF8String],
		        [stringFromObject(payload) UTF8String]);
		writeObject(payload, outDir, [@"xcode-shader-profiler-" stringByAppendingString:selectorName], idx);
		if ([payload isKindOfClass:[NSData class]]) {
			writeBytes((NSData *)payload, outDir, [@"xcode-shader-profiler-data-" stringByAppendingString:selectorName], idx);
		}
		id normalized = normalizedProfilerPayload(payload);
		if (normalized) {
			[payloads addObject:normalized];
			[payloadLabels addObject:[@"xcode-shader-profiler-" stringByAppendingString:selectorName]];
		}
		fprintf(stdout, "xcode shader profiler normalized selector=%s normalizedClass=%s\n",
		        [selectorName UTF8String],
		        [stringFromObject([normalized class]) UTF8String]);
		idx++;
	}
	NSString *profileMode = @"xcode_shaderprofiler_archive_payload_encode_streamdata";
	id mutableStreamData = newMutableProfilerStreamData(profileMode);
	if (!mutableStreamData) {
		fprintf(stderr, "missing mutable streamData for Xcode shader profiler archive payload\n");
		return 77;
	}
	int addedCount = 0;
	for (NSUInteger payloadIndex = 0; payloadIndex < [payloads count]; payloadIndex++) {
		id normalized = [payloads objectAtIndex:payloadIndex];
		NSString *label = [payloadLabels objectAtIndex:payloadIndex];
		BOOL added = tryAddPayloadToMutableStreamData(mutableStreamData, normalized, label);
		if (added) {
			addedCount++;
		}
		fprintf(stdout, "xcode shader profiler streamData add label=%s normalizedClass=%s added=%d\n",
		        [label UTF8String],
		        [stringFromObject([normalized class]) UTF8String],
		        added ? 1 : 0);
	}
	encodeMutableStreamData(mutableStreamData, outDir, @"xcode-shader-profiler-archive-payload");
	fprintf(stdout, "xcode shader profiler archive payloads=%d added=%d\n", idx, addedCount);
	if ([archive respondsToSelector:@selector(close)]) {
#pragma clang diagnostic push
#pragma clang diagnostic ignored "-Warc-performSelector-leaks"
		[archive performSelector:@selector(close)];
#pragma clang diagnostic pop
	}
	return 0;
}

static int runXcodeShaderProfilerTraceData(NSString *tracePath, NSString *outDir, NSString *mode) {
	if (!loadXcodeGPUToolsFrameworks()) {
		fprintf(stderr, "failed to load Xcode GPUTools frameworks\n");
		return 78;
	}
	Class archiveClass = NSClassFromString(@"DYCaptureArchive");
	Class profilerClass = [mode containsString:@"_base_"] ? NSClassFromString(@"DYPMTLShaderProfiler_OSX") : NSClassFromString(@"DYPMTLShaderProfilerGPUDesktopReplayer_OSX");
	if (!archiveClass || !profilerClass) {
		fprintf(stderr, "missing Xcode archive/profiler classes archive=%p profiler=%p\n", archiveClass, profilerClass);
		return 79;
	}
	NSURL *traceURL = [NSURL fileURLWithPath:tracePath];
	NSError *archiveError = nil;
	id archive = nil;
	@try {
		archive = ((id (*)(id, SEL, id, uint64_t, NSError **))objc_msgSend)([archiveClass alloc], @selector(initWithURL:options:error:), traceURL, 0, &archiveError);
	} @catch (NSException *exception) {
		fprintf(stdout, "xcode trace data archive init exception=%s\n", [[exception description] UTF8String]);
		return 80;
	}
	fprintf(stdout, "xcode trace data archive=%s err=%s\n",
	        [stringFromObject(archive) UTF8String],
	        [[archiveError description] UTF8String]);
	if (!archive) {
		return 81;
	}
	dumpRuntimeClass(NSStringFromClass(profilerClass));
	fprintf(stdout, "xcode trace data profiler alloc class=%s\n", class_getName(profilerClass));
	id profiler = [profilerClass new];
	fprintf(stdout, "xcode trace data profiler=%s\n", [stringFromObject(profiler) UTF8String]);
	SEL selector = @selector(getTraceDataForFunctionIndexArray:forCaptureArchive:);
	if (![profiler respondsToSelector:selector]) {
		fprintf(stderr, "xcode trace data selector missing\n");
		return 82;
	}
	NSMutableArray *payloads = [NSMutableArray array];
	NSMutableArray *labels = [NSMutableArray array];
	NSArray *batches = @[
		@[@0, @1, @2, @3, @4, @5, @6, @7],
		@[@8, @9, @10, @11, @12, @13, @14, @15],
		@[@16, @32, @64, @128, @256, @512]
	];
	int idx = 0;
	for (NSArray *batch in batches) {
		id payload = nil;
		@try {
			payload = ((id (*)(id, SEL, id, id))objc_msgSend)(profiler, selector, batch, archive);
		} @catch (NSException *exception) {
			fprintf(stdout, "xcode trace data batch=%d exception=%s\n", idx, [[exception description] UTF8String]);
			idx++;
			continue;
		}
		fprintf(stdout, "xcode trace data batch=%d indexes=%s payloadClass=%s payload=%s\n",
		        idx,
		        [[batch description] UTF8String],
		        [stringFromObject([payload class]) UTF8String],
		        [stringFromObject(payload) UTF8String]);
		NSString *label = [NSString stringWithFormat:@"xcode-trace-data-%02d", idx];
		writeObject(payload, outDir, label, idx);
		if ([payload isKindOfClass:[NSData class]]) {
			writeBytes((NSData *)payload, outDir, [label stringByAppendingString:@"-data"], idx);
		}
		id normalized = normalizedProfilerPayload(payload);
		if (normalized) {
			[payloads addObject:normalized];
			[labels addObject:label];
		}
		idx++;
	}
	id mutableStreamData = newMutableProfilerStreamData(@"xcode_shaderprofiler_trace_data_encode_streamdata");
	if (!mutableStreamData) {
		fprintf(stderr, "missing mutable streamData for Xcode trace data\n");
		return 83;
	}
	int addedCount = 0;
	for (NSUInteger payloadIndex = 0; payloadIndex < [payloads count]; payloadIndex++) {
		id normalized = [payloads objectAtIndex:payloadIndex];
		NSString *label = [labels objectAtIndex:payloadIndex];
		BOOL added = tryAddPayloadToMutableStreamData(mutableStreamData, normalized, label);
		if (added) {
			addedCount++;
		}
		fprintf(stdout, "xcode trace data streamData add label=%s normalizedClass=%s added=%d\n",
		        [label UTF8String],
		        [stringFromObject([normalized class]) UTF8String],
		        added ? 1 : 0);
	}
	encodeMutableStreamData(mutableStreamData, outDir, @"xcode-trace-data");
	fprintf(stdout, "xcode trace data payloads=%lu added=%d\n", (unsigned long)[payloads count], addedCount);
	if ([archive respondsToSelector:@selector(close)]) {
#pragma clang diagnostic push
#pragma clang diagnostic ignored "-Warc-performSelector-leaks"
		[archive performSelector:@selector(close)];
#pragma clang diagnostic pop
	}
	return 0;
}

static int runXcodeShaderProfilerResultShell(NSString *tracePath, NSString *outDir, NSString *mode) {
	if (!loadXcodeGPUToolsFrameworks()) {
		fprintf(stderr, "failed to load Xcode GPUTools frameworks\n");
		return 87;
	}
	NSError *archiveError = nil;
	id archive = openXcodeCaptureArchive(tracePath, &archiveError);
	if (!archive) {
		return 88;
	}
	Class resultClass = NSClassFromString(@"DYShaderProfilerResult");
	if (!resultClass) {
		fprintf(stderr, "missing DYShaderProfilerResult class\n");
		return 89;
	}
	dumpRuntimeClass(@"DYShaderProfilerResult");
	id result = [resultClass new];
	@try {
		if ([result respondsToSelector:@selector(setDrawCallInfoList:)]) {
			[result setValue:[NSMutableArray array] forKey:@"drawCallInfoList"];
		}
		if ([result respondsToSelector:@selector(setProgramInfoList:)]) {
			[result setValue:[NSMutableArray array] forKey:@"programInfoList"];
		}
		if ([result respondsToSelector:@selector(setProgramPipelineInfoList:)]) {
			[result setValue:[NSMutableArray array] forKey:@"programPipelineInfoList"];
		}
		if ([result respondsToSelector:@selector(setEncoderInfoList:)]) {
			[result setValue:[NSMutableArray array] forKey:@"encoderInfoList"];
		}
		if ([result respondsToSelector:@selector(setEncoderProgramInfoList:)]) {
			[result setValue:[NSMutableArray array] forKey:@"encoderProgramInfoList"];
		}
		if ([result respondsToSelector:@selector(setEncoderFunctionIndexToEncoderIndexMap:)]) {
			[result setValue:[NSMutableDictionary dictionary] forKey:@"encoderFunctionIndexToEncoderIndexMap"];
		}
		if ([result respondsToSelector:@selector(setEncoderFunctionIndexList:)]) {
			[result setValue:[NSMutableArray array] forKey:@"encoderFunctionIndexList"];
		}
		if ([result respondsToSelector:@selector(setEncoderTimeData:)]) {
			[result setValue:@[] forKey:@"encoderTimeData"];
		}
		if ([result respondsToSelector:@selector(setBlitTimeData:)]) {
			[result setValue:@[] forKey:@"blitTimeData"];
		}
		if ([result respondsToSelector:@selector(setDerivedCountersData:)]) {
			[result setValue:@{} forKey:@"derivedCountersData"];
		}
		if ([result respondsToSelector:@selector(setPerCommandBufferEncoderCount:)]) {
			[result setValue:[NSMutableArray arrayWithObject:@0] forKey:@"perCommandBufferEncoderCount"];
		}
		if ([result respondsToSelector:@selector(setEncoderIndexToLabelMap:)]) {
			[result setValue:@{} forKey:@"encoderIndexToLabelMap"];
		}
		id frameCount = nil;
		if ([archive respondsToSelector:@selector(metadataValueForKey:)]) {
			frameCount = ((id (*)(id, SEL, id))objc_msgSend)(archive, @selector(metadataValueForKey:), @"DYCaptureEngine.captured_frames_count");
		}
		uint32_t commandBufferCount = [frameCount respondsToSelector:@selector(unsignedIntValue)] ? [frameCount unsignedIntValue] : 1;
		if ([result respondsToSelector:@selector(setCommandBufferCount:)]) {
			((void (*)(id, SEL, uint32_t))objc_msgSend)(result, @selector(setCommandBufferCount:), commandBufferCount);
		}
		if ([result respondsToSelector:@selector(setConsistentStateAchieved:)]) {
			((void (*)(id, SEL, BOOL))objc_msgSend)(result, @selector(setConsistentStateAchieved:), YES);
		}
	} @catch (NSException *exception) {
		fprintf(stdout, "xcode shader profiler result shell setup exception=%s\n", [[exception description] UTF8String]);
	}
	fprintf(stdout, "xcode shader profiler result shell class=%s object=%s\n",
	        [stringFromObject([result class]) UTF8String],
	        [stringFromObject(result) UTF8String]);
	writeObject(result, outDir, @"xcode-shader-profiler-result-shell", 0);
	id payload = result;
	NSData *archivedResult = nil;
	if ([mode containsString:@"_data_"]) {
		NSError *archiveResultError = nil;
		archivedResult = [NSKeyedArchiver archivedDataWithRootObject:result requiringSecureCoding:NO error:&archiveResultError];
		fprintf(stdout, "xcode shader profiler result shell archived bytes=%lu err=%s\n",
		        (unsigned long)[archivedResult length],
		        [[archiveResultError description] UTF8String]);
		if (archivedResult) {
			writeBytes(archivedResult, outDir, @"xcode-shader-profiler-result-shell-archive", 0);
			payload = archivedResult;
		}
	}
	id mutableStreamData = newMutableProfilerStreamData(@"xcode_shaderprofiler_result_shell_encode_streamdata");
	if (!mutableStreamData) {
		fprintf(stderr, "missing mutable streamData for Xcode shader profiler result shell\n");
		return 90;
	}
	BOOL added = tryAddPayloadToMutableStreamDataWithSelector(mutableStreamData, payload, @"xcode-shader-profiler-result-shell", @selector(addShaderProfilerData:));
	fprintf(stdout, "xcode shader profiler result shell added=%d\n", added ? 1 : 0);
	encodeMutableStreamData(mutableStreamData, outDir, @"xcode-shader-profiler-result-shell");
	if ([archive respondsToSelector:@selector(close)]) {
#pragma clang diagnostic push
#pragma clang diagnostic ignored "-Warc-performSelector-leaks"
		[archive performSelector:@selector(close)];
#pragma clang diagnostic pop
	}
	return 0;
}

static uint64_t unsignedValueFromDictionary(NSDictionary *dict, NSString *key) {
	id value = [dict objectForKey:key];
	if ([value respondsToSelector:@selector(unsignedLongLongValue)]) {
		return [value unsignedLongLongValue];
	}
	return 0;
}

typedef struct {
	NSUInteger count;
	uint64_t cumulativeUs;
} GTProbeTimingRowsSummary;

static int addTraceTimingRowsToStreamData(id streamData, NSString *rowsPath, GTProbeTimingRowsSummary *summary) {
	NSData *jsonData = [NSData dataWithContentsOfFile:rowsPath];
	if (!jsonData) {
		fprintf(stderr, "failed to read timing rows json path=%s\n", [rowsPath UTF8String]);
		return 91;
	}
	NSError *jsonError = nil;
	NSArray *rows = [NSJSONSerialization JSONObjectWithData:jsonData options:0 error:&jsonError];
	if (![rows isKindOfClass:[NSArray class]] || [rows count] == 0) {
		fprintf(stderr, "invalid timing rows json err=%s\n", [[jsonError description] UTF8String]);
		return 92;
	}
	if (!streamData) {
		fprintf(stderr, "missing mutable streamData for trace timing rows\n");
		return 93;
	}
	NSUInteger count = [rows count];
	GTProbeEncoderInfoRow *encoders = calloc(count, sizeof(GTProbeEncoderInfoRow));
	GTProbeGPUCommandInfoRow *commands = calloc(count, sizeof(GTProbeGPUCommandInfoRow));
	if (!encoders || !commands) {
		free(encoders);
		free(commands);
		return 94;
	}
	uint64_t firstStart = unsignedValueFromDictionary([rows objectAtIndex:0], @"start_ns");
	uint64_t cumulativeUs = 0;
	for (NSUInteger i = 0; i < count; i++) {
		NSDictionary *row = [rows objectAtIndex:i];
		uint64_t startNs = unsignedValueFromDictionary(row, @"start_ns");
		uint64_t durationNs = unsignedValueFromDictionary(row, @"duration_ns");
		uint64_t durationUs = durationNs / 1000;
		if (durationUs == 0 && durationNs > 0) {
			durationUs = 1;
		}
		cumulativeUs += durationUs;
		encoders[i].sequenceID = (uint64_t)i + 1;
		encoders[i].startTimestamp = startNs;
		encoders[i].endOffsetMicros = cumulativeUs;
		encoders[i].labelStringIndex = 0;
		encoders[i].commandBufferIndex = 0;
		commands[i].functionIndex = 0;
		commands[i].subCommandIndex = (uint32_t)i;
		commands[i].pipelineIndex = 0;
		commands[i].endOffsetMicros = cumulativeUs;
		commands[i].encoderIndex = (uint32_t)i;
	}
	GTProbeCommandBufferInfoRow commandBuffer = {1, firstStart, cumulativeUs, 0, (uint32_t)count};
	if ([streamData respondsToSelector:@selector(addCommandBuffers:count:)]) {
		((void (*)(id, SEL, GTProbeCommandBufferInfoRow *, uint64_t))objc_msgSend)(streamData, @selector(addCommandBuffers:count:), &commandBuffer, 1);
	}
	if ([streamData respondsToSelector:@selector(addEncoders:count:)]) {
		((void (*)(id, SEL, GTProbeEncoderInfoRow *, uint64_t))objc_msgSend)(streamData, @selector(addEncoders:count:), encoders, (uint64_t)count);
	}
	if ([streamData respondsToSelector:@selector(addGPUCommands:count:)]) {
		((void (*)(id, SEL, GTProbeGPUCommandInfoRow *, uint64_t))objc_msgSend)(streamData, @selector(addGPUCommands:count:), commands, (uint64_t)count);
	}
	if (summary) {
		summary->count = count;
		summary->cumulativeUs = cumulativeUs;
	}
	fprintf(stdout, "trace timing rows added rows=%lu cumulative_us=%llu\n",
	        (unsigned long)count,
	        (unsigned long long)cumulativeUs);
	free(encoders);
	free(commands);
	return 0;
}

static int runTraceTimingRowsEncodeStreamData(NSString *rowsPath, NSString *outDir) {
	id streamData = newMutableProfilerStreamData(@"trace_timing_rows_encode_streamdata");
	if (!streamData) {
		fprintf(stderr, "missing mutable streamData for trace timing rows\n");
		return 93;
	}
	GTProbeTimingRowsSummary summary = {0, 0};
	int rc = addTraceTimingRowsToStreamData(streamData, rowsPath, &summary);
	if (rc != 0) {
		return rc;
	}
	encodeMutableStreamData(streamData, outDir, @"trace-timing-rows");
	fprintf(stdout, "trace timing rows encoded rows=%lu cumulative_us=%llu\n",
	        (unsigned long)summary.count,
	        (unsigned long long)summary.cumulativeUs);
	return 0;
}

static int runTraceTimingRowsPlusDerivedCountersEncodeStreamData(GTMTLReplayServiceXPCProxy *replayer, NSString *rowsPath, NSString *outDir) {
	id streamData = newMutableProfilerStreamData(@"trace_timing_rows_plus_derived_counters_encode_streamdata");
	if (!streamData) {
		fprintf(stderr, "missing mutable streamData for combined timing/counter rows\n");
		return 93;
	}
	GTProbeTimingRowsSummary summary = {0, 0};
	int rc = addTraceTimingRowsToStreamData(streamData, rowsPath, &summary);
	if (rc != 0) {
		return rc;
	}
	id request = newProfileRequest(@"derived_counters_encode_streamdata");
	if (!request) {
		fprintf(stderr, "missing derived counter request class for combined streamData\n");
		return 96;
	}
	__block int streamResponses = 0;
	__block int counterPayloadsAdded = 0;
	if ([request respondsToSelector:@selector(setStreamHandler:)]) {
		[(GTReplayProfileRequest *)request setStreamHandler:^(id response) {
			logResponse(response, @"combined-stream");
			NSData *data = responseData(response);
			if (data) {
				id payload = unarchivedObjectFromData(data);
				if (!payload) {
					payload = data;
				}
				payload = normalizedProfilerPayload(payload);
				BOOL added = NO;
				@try {
					if ([streamData respondsToSelector:@selector(addAPSCounterData:)]) {
						added = ((BOOL (*)(id, SEL, id))objc_msgSend)(streamData, @selector(addAPSCounterData:), payload);
					}
				} @catch (NSException *exception) {
					fprintf(stdout, "combined streamData counter add exception=%s\n", [[exception description] UTF8String]);
				}
				if (added) {
					counterPayloadsAdded++;
				}
				fprintf(stdout, "combined streamData counter add bytes=%lu payloadClass=%s added=%d\n",
				        (unsigned long)[data length],
				        [stringFromObject([payload class]) UTF8String],
				        added ? 1 : 0);
			}
			writeObject(response, outDir, @"combined-stream", streamResponses);
			writeBytes(data, outDir, @"combined-stream", streamResponses++);
		}];
	}
	id token = [replayer profile:request];
	fprintf(stdout, "combined profile token=%s\n", [stringFromObject(token) UTF8String]);
	[[NSRunLoop currentRunLoop] runUntilDate:[NSDate dateWithTimeIntervalSinceNow:4.0]];
	if ([token respondsToSelector:@selector(cancel)]) {
		BOOL cancelled = boolValueOrNo(token, @"cancel");
		fprintf(stdout, "combined token cancel=%d\n", cancelled ? 1 : 0);
	}
	encodeMutableStreamData(streamData, outDir, @"trace-timing-rows-plus-derived-counters");
	fprintf(stdout, "combined streamData encoded rows=%lu cumulative_us=%llu stream_responses=%d counter_payloads_added=%d\n",
	        (unsigned long)summary.count,
	        (unsigned long long)summary.cumulativeUs,
	        streamResponses,
	        counterPayloadsAdded);
	return 0;
}

static intptr_t gputoolsReplaySlide(void) {
    uint32_t count = _dyld_image_count();
    for (uint32_t i = 0; i < count; i++) {
        const char *name = _dyld_get_image_name(i);
        if (name && strstr(name, "GPUToolsReplay.framework/GPUToolsReplay")) {
            return _dyld_get_image_vmaddr_slide(i);
        }
    }
    return 0;
}

static int runProfileConstantDump(void) {
    void *replayHandle = dlopen("/System/Library/PrivateFrameworks/GPUToolsReplay.framework/GPUToolsReplay", RTLD_NOW);
    fprintf(stdout, "GPUToolsReplay dlopen=%p err=%s\n", replayHandle, dlerror());
    intptr_t slide = gputoolsReplaySlide();
    fprintf(stdout, "GPUToolsReplay slide=0x%llx\n", (unsigned long long)slide);
    if (!replayHandle) {
        return 0;
    }
    uintptr_t addrs[] = {
        0x29bb7d008ULL,
        0x29bb7d328ULL,
        0x29bb7d348ULL,
        0x29bb7d368ULL,
        0x29bb7d408ULL,
        0x29bb7d5c8ULL,
        0x29bb7d6e8ULL,
        0x29bb7dfc8ULL,
        0x29bb7e5c8ULL,
        0x29bb7e5e8ULL,
        0x29bb7e608ULL,
        0x29bb7e688ULL,
        0x29bb7e6c8ULL,
    };
    size_t count = sizeof(addrs) / sizeof(addrs[0]);
    for (size_t i = 0; i < count; i++) {
        vm_address_t addr = (vm_address_t)(addrs[i] + (uintptr_t)slide);
        uint64_t words[4] = {0, 0, 0, 0};
        vm_size_t copied = 0;
        kern_return_t kr = vm_read_overwrite(mach_task_self(), addr, sizeof(words), (vm_address_t)words, &copied);
        fprintf(stdout, "constant 0x%llx addr=0x%llx read_kr=%d copied=%llu words=%016llx %016llx %016llx %016llx",
                (unsigned long long)addrs[i],
                (unsigned long long)addr,
                kr,
                (unsigned long long)copied,
                (unsigned long long)words[0],
                (unsigned long long)words[1],
                (unsigned long long)words[2],
                (unsigned long long)words[3]);
        if (kr == KERN_SUCCESS && copied >= 24 && words[2] != 0 && words[3] < 4096) {
            char buf[4096];
            memset(buf, 0, sizeof(buf));
            vm_size_t stringCopied = 0;
            kern_return_t stringKR = vm_read_overwrite(mach_task_self(),
                                                       (vm_address_t)words[2],
                                                       MIN((vm_size_t)sizeof(buf) - 1, (vm_size_t)words[3]),
                                                       (vm_address_t)buf,
                                                       &stringCopied);
            fprintf(stdout, " cstring_kr=%d cstring_len=%llu cstring=%s",
                    stringKR,
                    (unsigned long long)stringCopied,
                    stringKR == KERN_SUCCESS ? buf : "");
        }
        if (kr == KERN_SUCCESS) {
            for (int wi = 0; wi < 4; wi++) {
                char buf[129];
                memset(buf, 0, sizeof(buf));
                vm_size_t stringCopied = 0;
                kern_return_t stringKR = vm_read_overwrite(mach_task_self(),
                                                           (vm_address_t)words[wi],
                                                           sizeof(buf) - 1,
                                                           (vm_address_t)buf,
                                                           &stringCopied);
                if (stringKR != KERN_SUCCESS || stringCopied == 0) {
                    continue;
                }
                BOOL printable = YES;
                int printableLen = 0;
                for (int bi = 0; bi < (int)stringCopied && buf[bi] != 0; bi++) {
                    unsigned char ch = (unsigned char)buf[bi];
                    if (ch < 0x20 || ch > 0x7e) {
                        printable = NO;
                        break;
                    }
                    printableLen++;
                }
                if (printable && printableLen > 0) {
                    fprintf(stdout, " word%d_cstring=%.*s", wi, printableLen, buf);
                }
                char utf16[65];
                memset(utf16, 0, sizeof(utf16));
                int utf16Len = 0;
                BOOL utf16Printable = YES;
                for (int bi = 0; bi + 1 < (int)stringCopied && utf16Len < (int)sizeof(utf16) - 1; bi += 2) {
                    unsigned char lo = (unsigned char)buf[bi];
                    unsigned char hi = (unsigned char)buf[bi + 1];
                    if (lo == 0 && hi == 0) {
                        break;
                    }
                    if (hi != 0 || lo < 0x20 || lo > 0x7e) {
                        utf16Printable = NO;
                        break;
                    }
                    utf16[utf16Len++] = (char)lo;
                }
                if (utf16Printable && utf16Len > 1) {
                    fprintf(stdout, " word%d_utf16=%s", wi, utf16);
                }
            }
        }
        fprintf(stdout, "\n");
    }
    return 0;
}

int main(int argc, char **argv) {
    @autoreleasepool {
        setvbuf(stdout, NULL, _IONBF, 0);
        setvbuf(stderr, NULL, _IONBF, 0);
        if (argc < 4) {
            fprintf(stderr, "usage: %s <mode> <trace.gputrace> <out-dir>\n", argv[0]);
            return 2;
        }
        NSString *mode = [NSString stringWithUTF8String:argv[1]];
        NSString *tracePath = [NSString stringWithUTF8String:argv[2]];
        NSString *outDir = [NSString stringWithUTF8String:argv[3]];
        mkdir([outDir fileSystemRepresentation], 0777);

        if ([mode isEqualToString:@"xcode_shaderprofiler_runtime_dump"]) {
            return runXcodeShaderProfilerRuntimeDump();
        }
        if ([mode isEqualToString:@"xcode_capture_archive_inspect"]) {
            return runXcodeCaptureArchiveInspect(tracePath, outDir);
        }
        if ([mode isEqualToString:@"xcode_shaderprofiler_archive_payload_encode_streamdata"] ||
            [mode isEqualToString:@"xcode_shaderprofiler_archive_gather_encode_streamdata"] ||
            [mode isEqualToString:@"xcode_shaderprofiler_archive_gather_device_encode_streamdata"] ||
            [mode isEqualToString:@"xcode_shaderprofiler_archive_gather_archiveinfo_encode_streamdata"] ||
            [mode isEqualToString:@"xcode_shaderprofiler_archive_gather_deviceprofile_encode_streamdata"] ||
            [mode isEqualToString:@"xcode_shaderprofiler_archive_gather_deviceproxy_encode_streamdata"] ||
            [mode isEqualToString:@"xcode_shaderprofiler_archive_construct_encode_streamdata"] ||
            [mode isEqualToString:@"xcode_shaderprofiler_archive_construct_base_encode_streamdata"] ||
            [mode isEqualToString:@"xcode_shaderprofiler_archive_construct_deviceproxy_encode_streamdata"]) {
            return runXcodeShaderProfilerArchivePayload(tracePath, outDir, mode);
        }
        if ([mode isEqualToString:@"xcode_shaderprofiler_trace_data_encode_streamdata"] ||
            [mode isEqualToString:@"xcode_shaderprofiler_trace_data_base_encode_streamdata"]) {
            return runXcodeShaderProfilerTraceData(tracePath, outDir, mode);
        }
        if ([mode isEqualToString:@"xcode_shaderprofiler_result_shell_encode_streamdata"] ||
            [mode isEqualToString:@"xcode_shaderprofiler_result_shell_data_encode_streamdata"]) {
            return runXcodeShaderProfilerResultShell(tracePath, outDir, mode);
        }
        if ([mode isEqualToString:@"runtime_dump"]) {
            return runRuntimeDump();
        }
        if ([mode isEqualToString:@"runtime_class_prefix_dump"]) {
            return runRuntimeClassPrefixDump();
        }
        if ([mode isEqualToString:@"runtime_class_profile_substring_dump"]) {
            return runRuntimeClassProfileSubstringDump();
        }
        if ([mode isEqualToString:@"profile_constant_dump"]) {
            return runProfileConstantDump();
        }

        void *handle = dlopen("/System/Library/PrivateFrameworks/GPUToolsTransport.framework/GPUToolsTransport", RTLD_NOW);
        if (!handle) {
            fprintf(stderr, "dlopen transport: %s\n", dlerror());
            return 10;
        }

        GTTransportServiceDaemonConnectionNewFn connectionNew = (GTTransportServiceDaemonConnectionNewFn)dlsym(handle, "GTTransportServiceDaemonConnectionNew");
        if (!connectionNew) {
            fprintf(stderr, "dlsym transport connection: %s\n", dlerror());
            return 11;
        }
        id connection = connectionNew(nil);
        Class clientClass = NSClassFromString(@"GTTransportClient");
        Class launchRequestClass = NSClassFromString(@"GTLaunchRequest");
        if (!connection || !clientClass || !launchRequestClass) {
            fprintf(stderr, "missing transport classes connection=%p client=%p launchRequest=%p\n", (__bridge void *)connection, clientClass, launchRequestClass);
            return 12;
        }
        id client = [[clientClass alloc] initWithConnection:connection];
        id services = [client allServices];
        NSString *udid = deviceUDIDFromServices(services);
        unsigned long serviceCount = [services respondsToSelector:@selector(count)] ? (unsigned long)[services count] : 0;
        fprintf(stdout, "initial service_count=%lu deviceUDID=%s\n", serviceCount, [stringFromObject(udid) UTF8String]);

        id launchRequest = [launchRequestClass new];
        [launchRequest setPreferXPCService:YES];
        [launchRequest setDisableDisplay:![mode containsString:@"display_on"]];
        NSArray *launchArguments = launchArgumentsForMode(mode, tracePath, outDir);
        NSDictionary *launchEnvironment = launchEnvironmentForMode(mode);
        [launchRequest setArguments:launchArguments];
        [launchRequest setEnvironment:launchEnvironment];
        [launchRequest setSessionUUID:[NSUUID UUID]];
        if ([udid length] > 0) {
            [launchRequest setDeviceUDID:udid];
        }
        fprintf(stdout, "launch disableDisplay=%d arguments=%s environment=%s\n",
                ![mode containsString:@"display_on"] ? 1 : 0,
                [stringFromObject(launchArguments) UTF8String],
                [stringFromObject(launchEnvironment) UTF8String]);
        NSError *launchError = nil;
        BOOL launched = [(GTLaunchServiceXPCProxy *)[client launcher] launchReplayService:launchRequest error:&launchError];
        fprintf(stdout, "launch ok=%d err=%s\n", launched ? 1 : 0, [[launchError description] UTF8String]);
        [[NSRunLoop currentRunLoop] runUntilDate:[NSDate dateWithTimeIntervalSinceNow:0.5]];

        GTMTLReplayServiceXPCProxy *replayer = [client replayer];
        fprintf(stdout, "replayer=%s\n", [stringFromObject(replayer) UTF8String]);
        if (!replayer) {
            return 20;
        }
        uint64_t observerID = 0;
        GTProbeReplayObserver *observer = nil;
        if ([mode containsString:@"observer"] && [replayer respondsToSelector:@selector(registerObserver:)]) {
            observer = [GTProbeReplayObserver new];
            observerID = [replayer registerObserver:observer];
            fprintf(stdout, "observer registered id=%llu serviceProperties=%s processInfo=%s\n",
                    (unsigned long long)observerID,
                    [stringFromObject(valueOrNil(replayer, @"serviceProperties")) UTF8String],
                    [stringFromObject(valueOrNil(replayer, @"processInfo")) UTF8String]);
        }
        NSError *loadError = nil;
        BOOL loaded = [replayer load:[NSURL fileURLWithPath:tracePath] error:&loadError];
        fprintf(stdout, "load ok=%d err=%s\n", loaded ? 1 : 0, [[loadError description] UTF8String]);
        if (!loaded) {
            if (observerID != 0 && [replayer respondsToSelector:@selector(deregisterObserver:)]) {
                [replayer deregisterObserver:observerID];
            }
            return 21;
        }

        int rc = 0;
        if ([mode isEqualToString:@"query_then_timeline_raw"]) {
            rc = runQuery(replayer, outDir);
            if (rc == 0) {
                rc = runProfile(replayer, mode, outDir);
            }
        } else if ([mode isEqualToString:@"query_device_capabilities"]) {
            rc = runQuery(replayer, outDir);
        } else if ([mode isEqualToString:@"query_configuration"]) {
            rc = runQueryClass(replayer, outDir, @"GTReplayQueryConfiguration");
        } else if ([mode isEqualToString:@"query_derived_counters"]) {
            rc = runQueryClass(replayer, outDir, @"GTReplayQueryDerivedCounters");
        } else if ([mode isEqualToString:@"query_derived_counters_encode_streamdata"]) {
            rc = runQueryDerivedCountersEncodeStreamData(replayer, outDir);
        } else if ([mode isEqualToString:@"trace_timing_rows_encode_streamdata"]) {
            if (argc < 5) {
                fprintf(stderr, "trace_timing_rows_encode_streamdata requires rows json path\n");
                rc = 95;
            } else {
                rc = runTraceTimingRowsEncodeStreamData([NSString stringWithUTF8String:argv[4]], outDir);
            }
        } else if ([mode isEqualToString:@"trace_timing_rows_plus_derived_counters_encode_streamdata"]) {
            if (argc < 5) {
                fprintf(stderr, "trace_timing_rows_plus_derived_counters_encode_streamdata requires rows json path\n");
                rc = 95;
            } else {
                rc = runTraceTimingRowsPlusDerivedCountersEncodeStreamData(replayer, [NSString stringWithUTF8String:argv[4]], outDir);
            }
        } else if ([mode isEqualToString:@"query_performance_state"]) {
            rc = runQueryClass(replayer, outDir, @"GTReplayQueryPerformanceState");
        } else if ([mode isEqualToString:@"service_introspection_profile_encode_streamdata_run_5s"]) {
            dumpTransportServices(services, @"before-profile");
            dumpObjectSnapshot(replayer, @"before-profile replayer");
            dumpObjectSnapshot(valueOrNil(replayer, @"bulkDataProxy"), @"before-profile bulkDataProxy");
            rc = runProfile(replayer, @"timeline_encode_streamdata_run_5s", outDir);
            dumpTransportServices([client allServices], @"after-profile");
            dumpObjectSnapshot(replayer, @"after-profile replayer");
            dumpObjectSnapshot(valueOrNil(replayer, @"bulkDataProxy"), @"after-profile bulkDataProxy");
        } else if ([mode isEqualToString:@"profile_bulk_download_candidates_encode_streamdata"] ||
                   [mode isEqualToString:@"profile_bulk_download_wait_5s_encode_streamdata"] ||
                   [mode isEqualToString:@"profile_bulk_download_proxy_resume_wait_5s_encode_streamdata"] ||
                   [mode isEqualToString:@"profile_bulk_download_wait_complete_encode_streamdata"]) {
            rc = runProfileBulkDownloadCandidates(replayer, outDir, mode);
        } else if ([mode isEqualToString:@"display_request_candidates"]) {
            rc = runDisplayRequestCandidates(replayer, outDir);
        } else if ([mode isEqualToString:@"profile_during_display_request_candidates_encode_streamdata"]) {
            rc = runProfileDuringDisplayRequestCandidates(replayer, outDir);
        } else if ([mode isEqualToString:@"query_resource_usage_candidates"]) {
            rc = runResourceUsageCandidates(replayer, outDir, argc, argv);
        } else if ([mode isEqualToString:@"profile_then_resource_usage_candidates"]) {
            rc = runProfile(replayer, @"timeline_raw_no_query", outDir);
            if (rc == 0) {
                rc = runResourceUsageCandidates(replayer, outDir, argc, argv);
            }
        } else if ([mode isEqualToString:@"update_config_then_timeline_raw"]) {
            rc = runUpdateConfiguration(replayer, outDir, mode);
            if (rc == 0) {
                rc = runProfile(replayer, @"timeline_raw_no_query", outDir);
            }
        } else if ([mode isEqualToString:@"update_config_then_timeline_encode_streamdata"]) {
            rc = runUpdateConfiguration(replayer, outDir, mode);
            if (rc == 0) {
                rc = runProfile(replayer, @"timeline_encode_streamdata", outDir);
            }
        } else if ([mode isEqualToString:@"update_config_then_query_performance_state"]) {
            rc = runUpdateConfiguration(replayer, outDir, mode);
            if (rc == 0) {
                rc = runQueryClass(replayer, outDir, @"GTReplayQueryPerformanceState");
            }
        } else if ([mode isEqualToString:@"update_config_display_on_then_query_performance_state"]) {
            rc = runUpdateConfiguration(replayer, outDir, mode);
            if (rc == 0) {
                rc = runQueryClass(replayer, outDir, @"GTReplayQueryPerformanceState");
            }
        } else if ([mode hasPrefix:@"update_config_then_timeline_encode_streamdata_perf_state_"]) {
            rc = runUpdateConfiguration(replayer, outDir, mode);
            if (rc == 0) {
                NSString *profileMode = [mode stringByReplacingOccurrencesOfString:@"update_config_then_" withString:@""];
                rc = runProfile(replayer, profileMode, outDir);
            }
        } else if ([mode hasPrefix:@"update_config_display_on_then_timeline_encode_streamdata_perf_state_"]) {
            rc = runUpdateConfiguration(replayer, outDir, mode);
            if (rc == 0) {
                NSString *profileMode = [mode stringByReplacingOccurrencesOfString:@"update_config_display_on_then_" withString:@""];
                profileMode = [profileMode stringByAppendingString:@"_display_on"];
                rc = runProfile(replayer, profileMode, outDir);
            }
        } else if ([mode isEqualToString:@"update_config_then_derived_counters"]) {
            rc = runUpdateConfiguration(replayer, outDir, mode);
            if (rc == 0) {
                rc = runProfile(replayer, @"derived_counters", outDir);
            }
        } else if ([mode isEqualToString:@"update_config_then_derived_counters_encode_streamdata"]) {
            rc = runUpdateConfiguration(replayer, outDir, mode);
            if (rc == 0) {
                rc = runProfile(replayer, @"derived_counters_encode_streamdata", outDir);
            }
        } else if ([mode isEqualToString:@"update_config_then_query_derived_counters_encode_streamdata"]) {
            rc = runUpdateConfiguration(replayer, outDir, mode);
            if (rc == 0) {
                rc = runQueryDerivedCountersEncodeStreamData(replayer, outDir);
            }
        } else if ([mode isEqualToString:@"datasource_ready_then_query_derived_counters_encode_streamdata"]) {
            rc = runDatasourceReadyThenQueryDerivedCounters(replayer, outDir, argc, argv);
        } else if ([mode isEqualToString:@"fetch_pipeline_binary_candidates"]) {
            rc = runFetchPipelineBinaryCandidates(replayer, outDir, argc, argv);
        } else if ([mode isEqualToString:@"fetch_texture_candidates"]) {
            rc = runFetchTextureCandidates(replayer, outDir, argc, argv, YES);
        } else if ([mode isEqualToString:@"fetch_buffer_candidates"]) {
            rc = runFetchBufferCandidates(replayer, outDir, argc, argv, YES, nil);
        } else if ([mode isEqualToString:@"fetch_threadgroup_candidates"]) {
            rc = runAuxiliaryFetchCandidates(replayer, outDir, argc, argv, @"GTReplayFetchThreadgroup", @"fetch-threadgroup", YES, nil);
        } else if ([mode isEqualToString:@"fetch_post_vertex_candidates"]) {
            rc = runAuxiliaryFetchCandidates(replayer, outDir, argc, argv, @"GTReplayFetchPostVertex", @"fetch-post-vertex", YES, nil);
        } else if ([mode isEqualToString:@"fetch_wireframe_candidates"]) {
            rc = runAuxiliaryFetchCandidates(replayer, outDir, argc, argv, @"GTReplayFetchWireframe", @"fetch-wireframe", YES, nil);
        } else if ([mode isEqualToString:@"fetch_into_texture_candidates"]) {
            rc = runFetchIntoTextureCandidates(replayer, outDir, argc, argv, NO);
        } else if ([mode isEqualToString:@"fetch_into_texture_then_timeline_encode_streamdata"]) {
            rc = runFetchIntoTextureCandidates(replayer, outDir, argc, argv, NO);
            if (rc == 0) {
                rc = runProfile(replayer, @"timeline_encode_streamdata", outDir);
            }
        } else if ([mode isEqualToString:@"fetch_into_texture_wait_complete_then_timeline_encode_streamdata_wait_complete"]) {
            rc = runFetchIntoTextureCandidates(replayer, outDir, argc, argv, YES);
            if (rc == 0) {
                rc = runProfile(replayer, @"timeline_encode_streamdata_wait_complete", outDir);
            }
        } else if ([mode isEqualToString:@"profile_during_fetch_into_texture_encode_streamdata"]) {
            rc = runProfileDuringFetchIntoTextureCandidates(replayer, outDir, argc, argv, NO);
        } else if ([mode isEqualToString:@"profile_during_fetch_into_texture_wait_complete_encode_streamdata"]) {
            rc = runProfileDuringFetchIntoTextureCandidates(replayer, outDir, argc, argv, YES);
        } else if ([mode isEqualToString:@"profile_during_fetch_into_texture_session_request_wait_complete_encode_streamdata"]) {
            rc = runProfileDuringFetchIntoTextureCandidates(replayer, outDir, argc, argv, YES);
        } else if ([mode isEqualToString:@"update_config_then_profile_during_fetch_into_texture_wait_complete_encode_streamdata"]) {
            rc = runUpdateConfiguration(replayer, outDir, mode);
            if (rc == 0) {
                rc = runProfileDuringFetchIntoTextureCandidates(replayer, outDir, argc, argv, YES);
            }
        } else if ([mode isEqualToString:@"update_config_display_on_then_profile_during_fetch_into_texture_wait_complete_encode_streamdata"]) {
            rc = runUpdateConfiguration(replayer, outDir, mode);
            if (rc == 0) {
                rc = runProfileDuringFetchIntoTextureCandidates(replayer, outDir, argc, argv, YES);
            }
        } else if ([mode isEqualToString:@"profile_during_fetch_texture_wait_complete_encode_streamdata"]) {
            rc = runProfileDuringFetchTextureCandidates(replayer, outDir, argc, argv, YES);
        } else if ([mode isEqualToString:@"profile_during_fetch_buffer_wait_complete_encode_streamdata"]) {
            rc = runProfileDuringFetchBufferCandidates(replayer, outDir, argc, argv, YES);
        } else if ([mode isEqualToString:@"profile_during_fetch_buffer_ingest_fetch_payloads_encode_streamdata"]) {
            rc = runProfileDuringFetchBufferCandidates(replayer, outDir, argc, argv, NO);
        } else if ([mode isEqualToString:@"profile_during_fetch_threadgroup_candidates_encode_streamdata"]) {
            rc = runProfileDuringAuxiliaryFetchCandidates(replayer, outDir, argc, argv, @"GTReplayFetchThreadgroup", @"fetch-threadgroup");
        } else if ([mode isEqualToString:@"profile_during_fetch_post_vertex_candidates_encode_streamdata"]) {
            rc = runProfileDuringAuxiliaryFetchCandidates(replayer, outDir, argc, argv, @"GTReplayFetchPostVertex", @"fetch-post-vertex");
        } else if ([mode isEqualToString:@"profile_during_fetch_wireframe_candidates_encode_streamdata"]) {
            rc = runProfileDuringAuxiliaryFetchCandidates(replayer, outDir, argc, argv, @"GTReplayFetchWireframe", @"fetch-wireframe");
        } else if ([mode isEqualToString:@"profile_during_fetch_threadgroup_ingest_fetch_payloads_encode_streamdata"]) {
            rc = runProfileDuringAuxiliaryFetchCandidates(replayer, outDir, argc, argv, @"GTReplayFetchThreadgroup", @"fetch-threadgroup");
        } else if ([mode isEqualToString:@"profile_during_fetch_wireframe_ingest_fetch_payloads_encode_streamdata"]) {
            rc = runProfileDuringAuxiliaryFetchCandidates(replayer, outDir, argc, argv, @"GTReplayFetchWireframe", @"fetch-wireframe");
        } else if ([mode isEqualToString:@"query_raster_map_candidates"]) {
            rc = runQueryRasterMapCandidates(replayer, outDir, argc, argv);
        } else if ([mode isEqualToString:@"decode_generic_acceleration_structure_candidates"]) {
            rc = runDecodeGenericAccelerationStructureCandidates(replayer, outDir, argc, argv);
        } else if ([mode isEqualToString:@"decode_ab_candidates"]) {
            rc = runDecodeCandidates(replayer, outDir, argc, argv, @"GTReplayDecodeAB", @"decode-ab");
        } else if ([mode isEqualToString:@"decode_icb_candidates"]) {
            rc = runDecodeCandidates(replayer, outDir, argc, argv, @"GTReplayDecodeICB", @"decode-icb");
        } else if ([mode isEqualToString:@"profile_during_decode_ab_candidates_encode_streamdata"]) {
            rc = runProfileDuringDecodeCandidates(replayer, outDir, argc, argv, @"GTReplayDecodeAB", @"decode-ab");
        } else if ([mode isEqualToString:@"profile_during_decode_icb_candidates_encode_streamdata"]) {
            rc = runProfileDuringDecodeCandidates(replayer, outDir, argc, argv, @"GTReplayDecodeICB", @"decode-icb");
        } else if ([mode isEqualToString:@"update_library_candidates"]) {
            rc = runUpdateLibraryCandidates(replayer, outDir, argc, argv, YES);
        } else if ([mode isEqualToString:@"profile_during_update_library_candidates_encode_streamdata"]) {
            rc = runProfileDuringUpdateLibraryCandidates(replayer, outDir, argc, argv);
        } else if ([mode isEqualToString:@"raytrace_candidates"]) {
            rc = runRaytraceCandidates(replayer, outDir, argc, argv);
        } else if ([mode isEqualToString:@"shaderdebug_kernel_candidates"]) {
            rc = runShaderDebugKernelCandidates(replayer, outDir, argc, argv);
        } else if ([mode isEqualToString:@"profile_during_shaderdebug_kernel_candidates_encode_streamdata"]) {
            rc = runProfileDuringShaderDebugKernelCandidates(replayer, outDir, argc, argv);
        } else if ([mode isEqualToString:@"profile_all_runtime_classes"]) {
            rc = runProfileAllRuntimeClasses(replayer, outDir);
        } else if ([mode isEqualToString:@"query_resource_usage_0"]) {
            rc = runQueryClass(replayer, outDir, @"GTReplayQueryResourceUsage");
        } else if ([mode isEqualToString:@"query_session_info"]) {
            rc = runQueryClass(replayer, outDir, @"GTReplayQuerySessionInfo");
        } else {
            rc = runProfile(replayer, mode, outDir);
        }
        if (observerID != 0 && [replayer respondsToSelector:@selector(deregisterObserver:)]) {
            [replayer deregisterObserver:observerID];
            fprintf(stdout, "observer deregistered id=%llu\n", (unsigned long long)observerID);
        }
        if ([replayer respondsToSelector:@selector(terminateProcess)]) {
            [replayer terminateProcess];
        }
        return rc;
    }
}
`
