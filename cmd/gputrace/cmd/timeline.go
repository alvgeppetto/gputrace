package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/tmc/mlx-go/experiments/gputrace"
)

var (
	timelineOutput string
	timelineFormat string
)

var timelineCmd = &cobra.Command{
	Use:   "timeline <trace.gputrace>",
	Short: "Generate timeline visualization from GPU trace",
	Long: `Generate an interactive timeline visualization showing:
  - Chronological API call sequence with timestamps
  - Concurrent command buffer execution
  - Encoder lifecycle (creation -> encoding -> commit)
  - Buffer binding events mapped to kernels
  - GPU execution timeline

Output formats:
  - chrome: Chrome tracing format (chrome://tracing)
  - json: Raw timeline data in JSON format

Examples:
  # Generate Chrome tracing format
  gputrace timeline trace.gputrace -o timeline.json

  # View in Chrome
  # 1. Open chrome://tracing in Chrome
  # 2. Click "Load" and select timeline.json
  # 3. Use WASD keys to navigate, mouse wheel to zoom

  # Generate raw JSON for custom processing
  gputrace timeline trace.gputrace -o timeline.json --format json`,
	Args: cobra.ExactArgs(1),
	RunE: runTimeline,
}

func init() {
	rootCmd.AddCommand(timelineCmd)

	timelineCmd.Flags().StringVarP(&timelineOutput, "output", "o", "timeline.json", "Output file path")
	timelineCmd.Flags().StringVar(&timelineFormat, "format", "chrome", "Output format: chrome, json")
}

func runTimeline(cmd *cobra.Command, args []string) error {
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

	// Generate timeline data
	timeline, err := generateTimeline(trace)
	if err != nil {
		return fmt.Errorf("failed to generate timeline: %w", err)
	}

	// Export based on format
	switch timelineFormat {
	case "chrome":
		if err := exportChromeTracing(timeline, timelineOutput); err != nil {
			return fmt.Errorf("failed to export Chrome tracing: %w", err)
		}
	case "json":
		if err := exportTimelineJSON(timeline, timelineOutput); err != nil {
			return fmt.Errorf("failed to export JSON: %w", err)
		}
	default:
		return fmt.Errorf("unknown format: %s (supported: chrome, json)", timelineFormat)
	}

	fmt.Printf("✓ Timeline written to: %s\n", timelineOutput)
	if timelineFormat == "chrome" {
		fmt.Println("\nView in Chrome:")
		fmt.Println("  1. Open chrome://tracing")
		fmt.Println("  2. Click 'Load' and select", timelineOutput)
		fmt.Println("  3. Use WASD to navigate, mouse wheel to zoom")
	}

	return nil
}

// Timeline represents the complete timeline data.
type Timeline struct {
	StartTime  uint64          `json:"start_time"`
	EndTime    uint64          `json:"end_time"`
	Duration   uint64          `json:"duration"`
	Events     []TimelineEvent `json:"events"`
	Encoders   []EncoderInfo   `json:"encoders"`
	Kernels    []KernelInfo    `json:"kernels"`
	APICallseq []APICall       `json:"api_calls"`
}

// TimelineEvent represents a single event in the timeline.
type TimelineEvent struct {
	Name      string                 `json:"name"`
	Category  string                 `json:"cat,omitempty"`
	Phase     string                 `json:"ph"` // B, E, X, i, M
	Timestamp uint64                 `json:"ts"`
	Duration  uint64                 `json:"dur,omitempty"`
	ProcessID int                    `json:"pid"`
	ThreadID  int                    `json:"tid"`
	Args      map[string]interface{} `json:"args,omitempty"`
}

// EncoderInfo contains information about an encoder.
type EncoderInfo struct {
	Index     int    `json:"index"`
	Label     string `json:"label"`
	Type      string `json:"type"`
	StartTime uint64 `json:"start_time"`
	EndTime   uint64 `json:"end_time"`
	Duration  uint64 `json:"duration"`
}

// KernelInfo contains information about a kernel execution.
type KernelInfo struct {
	Name      string `json:"name"`
	Encoder   int    `json:"encoder"`
	StartTime uint64 `json:"start_time"`
	EndTime   uint64 `json:"end_time"`
	Duration  uint64 `json:"duration"`
}

// APICall represents an API call event.
type APICall struct {
	Name      string                 `json:"name"`
	Timestamp uint64                 `json:"timestamp"`
	Args      map[string]interface{} `json:"args,omitempty"`
}

// generateTimeline creates timeline data from a trace.
func generateTimeline(trace *gputrace.Trace) (*Timeline, error) {
	timeline := &Timeline{
		Events:     make([]TimelineEvent, 0),
		Encoders:   make([]EncoderInfo, 0),
		Kernels:    make([]KernelInfo, 0),
		APICallseq: make([]APICall, 0),
	}

	// Extract timing metrics
	extractor := gputrace.NewTimingMetricsExtractor(trace)
	metrics, err := extractor.Extract()
	if err != nil {
		return nil, fmt.Errorf("extract timing: %w", err)
	}

	// Calculate timeline bounds
	if len(metrics.EncoderTimings) > 0 {
		timeline.StartTime = metrics.EncoderTimings[0].StartTimestamp
		timeline.EndTime = metrics.EncoderTimings[0].EndTimestamp

		for _, encoder := range metrics.EncoderTimings {
			if encoder.StartTimestamp < timeline.StartTime {
				timeline.StartTime = encoder.StartTimestamp
			}
			if encoder.EndTimestamp > timeline.EndTime {
				timeline.EndTime = encoder.EndTimestamp
			}
		}
	}

	timeline.Duration = timeline.EndTime - timeline.StartTime

	// Add encoder events
	for i, encoder := range metrics.EncoderTimings {
		encoderInfo := EncoderInfo{
			Index:     i,
			Label:     encoder.Label,
			Type:      "compute", // Default type
			StartTime: encoder.StartTimestamp,
			EndTime:   encoder.EndTimestamp,
			Duration:  encoder.DurationNs,
		}
		timeline.Encoders = append(timeline.Encoders, encoderInfo)

		// Create timeline event for encoder
		event := TimelineEvent{
			Name:      encoder.Label,
			Category:  "encoder",
			Phase:     "X", // Complete event
			Timestamp: encoder.StartTimestamp / 1000, // Convert to microseconds
			Duration:  encoder.DurationNs / 1000,     // Convert to microseconds
			ProcessID: 1,
			ThreadID:  1,
			Args: map[string]interface{}{
				"index":       i,
				"duration_ms": float64(encoder.DurationNs) / 1e6,
				"duration_us": float64(encoder.DurationNs) / 1e3,
			},
		}
		timeline.Events = append(timeline.Events, event)
	}

	// Add kernel events (if we have kernel-level timing)
	if len(metrics.KernelTimings) > 0 {
		// Distribute kernels across encoder timeline
		// This is approximate since we don't have exact per-invocation timing
		for i, kernel := range metrics.KernelTimings {
			encoderIdx := i % len(timeline.Encoders)
			if len(timeline.Encoders) == 0 {
				break
			}

			encoder := timeline.Encoders[encoderIdx]
			// Create kernel event within encoder timeframe
			kernelInfo := KernelInfo{
				Name:      kernel.Name,
				Encoder:   encoderIdx,
				StartTime: encoder.StartTime,
				EndTime:   encoder.EndTime,
				Duration:  uint64(kernel.AvgDuration.Nanoseconds()),
			}
			timeline.Kernels = append(timeline.Kernels, kernelInfo)

			// Create timeline event for kernel
			event := TimelineEvent{
				Name:      kernel.Name,
				Category:  "kernel",
				Phase:     "X",
				Timestamp: encoder.StartTime / 1000, // Convert to microseconds
				Duration:  uint64(kernel.AvgDuration.Microseconds()),
				ProcessID: 1,
				ThreadID:  2, // Use different thread for kernels
				Args: map[string]interface{}{
					"invocations": kernel.InvocationCount,
					"avg_ns":      kernel.AvgDuration.Nanoseconds(),
					"min_ns":      kernel.MinDuration.Nanoseconds(),
					"max_ns":      kernel.MaxDuration.Nanoseconds(),
					"avg_us":      kernel.AvgDuration.Microseconds(),
				},
			}
			timeline.Events = append(timeline.Events, event)
		}
	}

	// Add command buffer events
	commandBuffers, err := trace.ParseCommandBuffers()
	if err == nil {
		for i, cb := range commandBuffers {
			event := TimelineEvent{
				Name:      fmt.Sprintf("CommandBuffer %d", i),
				Category:  "command_buffer",
				Phase:     "i", // Instant event
				Timestamp: uint64(cb.Offset),
				ProcessID: 1,
				ThreadID:  0, // Use thread 0 for command buffers
				Args: map[string]interface{}{
					"offset": cb.Offset,
					"index":  i,
				},
			}
			timeline.Events = append(timeline.Events, event)
		}
	}

	return timeline, nil
}

// exportChromeTracing exports timeline in Chrome tracing format.
func exportChromeTracing(timeline *Timeline, outputPath string) error {
	f, err := os.Create(outputPath)
	if err != nil {
		return err
	}
	defer f.Close()

	// Add process and thread name metadata events
	metadataEvents := []TimelineEvent{
		{
			Name:      "process_name",
			Phase:     "M",
			ProcessID: 1,
			ThreadID:  0,
			Args: map[string]interface{}{
				"name": "GPU Trace",
			},
		},
		{
			Name:      "thread_name",
			Phase:     "M",
			ProcessID: 1,
			ThreadID:  0,
			Args: map[string]interface{}{
				"name": "Command Buffers",
			},
		},
		{
			Name:      "thread_name",
			Phase:     "M",
			ProcessID: 1,
			ThreadID:  1,
			Args: map[string]interface{}{
				"name": "Encoders",
			},
		},
		{
			Name:      "thread_name",
			Phase:     "M",
			ProcessID: 1,
			ThreadID:  2,
			Args: map[string]interface{}{
				"name": "Kernels",
			},
		},
	}

	// Combine metadata events with timeline events
	allEvents := append(metadataEvents, timeline.Events...)

	// Chrome tracing format
	tracing := map[string]interface{}{
		"traceEvents":     allEvents,
		"displayTimeUnit": "ms",
		"metadata": map[string]interface{}{
			"start_time":    timeline.StartTime,
			"end_time":      timeline.EndTime,
			"duration_ns":   timeline.Duration,
			"encoder_count": len(timeline.Encoders),
			"kernel_count":  len(timeline.Kernels),
		},
	}

	encoder := json.NewEncoder(f)
	encoder.SetIndent("", "  ")
	return encoder.Encode(tracing)
}

// exportTimelineJSON exports raw timeline data as JSON.
func exportTimelineJSON(timeline *Timeline, outputPath string) error {
	f, err := os.Create(outputPath)
	if err != nil {
		return err
	}
	defer f.Close()

	encoder := json.NewEncoder(f)
	encoder.SetIndent("", "  ")
	return encoder.Encode(timeline)
}
