package serve

import (
	"encoding/json"
	"net/http"
	"sort"
	"strings"

	"github.com/tmc/gputrace"
)

type KernelStats struct {
	Name          string  `json:"name"`
	PipelineState string  `json:"pipeline_state"`
	Dispatches    int     `json:"dispatches"`
	TotalTime     float64 `json:"total_time_ms"`
	AvgTime       float64 `json:"avg_time_ms"`
	Percentage    float64 `json:"percentage"`
}

func apiKernelsHandler(trace *gputrace.Trace) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Use AnalyzeKernels-like logic to get kernel stats
		// Since we don't have direct access to internal/analysis, we'll implement a simplified version here
		// relying on what's available in trace.

		// 1. Get dispatches count (estimated)
		// Ideally we would want per-kernel dispatch counts.
		// The trace object has KernelNames, but we need to map them to dispatches.
		// AnalyzeMTSPRecords or similar logic is needed.

		// Let's try to get timing data first as it often contains kernel names and durations.
		timings, err := gputrace.ExtractTimingData(trace)

		kernelMap := make(map[string]*KernelStats)
		totalGPUTime := 0.0

		if err == nil && len(timings) > 0 {
			// If we have timing data, use it for everything
			for _, t := range timings {
				name := t.Label
				// Clean up name if needed (sometimes has "Encode: " prefix)
				if strings.HasPrefix(name, "Encode: ") {
					name = strings.TrimPrefix(name, "Encode: ")
				}

				if _, exists := kernelMap[name]; !exists {
					kernelMap[name] = &KernelStats{Name: name}
				}
				k := kernelMap[name]
				k.Dispatches++
				k.TotalTime += t.DurationMs
				totalGPUTime += t.DurationMs
			}
		} else {
			// Fallback if no timing data: just list kernels from trace.KernelNames
			// We won't have dispatch counts easily without re-implementing parsing logic
			// But we can check EncoderLabels which might align with kernels in some traces

			// Use KernelNames
			for _, name := range trace.KernelNames {
				if _, exists := kernelMap[name]; !exists {
					kernelMap[name] = &KernelStats{Name: name}
				}
			}

			// Try to estimate dispatches from encoder labels if they match kernel names
			for _, label := range trace.EncoderLabels {
				if k, exists := kernelMap[label]; exists {
					k.Dispatches++
				}
			}
		}

		// Calculate averages and percentages
		var stats []KernelStats
		for _, k := range kernelMap {
			if k.Dispatches > 0 {
				k.AvgTime = k.TotalTime / float64(k.Dispatches)
			}
			if totalGPUTime > 0 {
				k.Percentage = (k.TotalTime / totalGPUTime) * 100
			}
			stats = append(stats, *k)
		}

		// Sort by total time descending by default
		sort.Slice(stats, func(i, j int) bool {
			return stats[i].TotalTime > stats[j].TotalTime
		})

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(stats)
	}
}

type APICallNode struct {
	ID       int            `json:"id"`
	Type     string         `json:"type"` // "CommandBuffer", "Encoder", "Call"
	Label    string         `json:"label"`
	Children []*APICallNode `json:"children,omitempty"`
	Details  string         `json:"details,omitempty"`
}

func apiCallsAPIHandler(trace *gputrace.Trace) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Build hierarchy:
		// Command Buffer -> Encoder -> API Call
		// This is a simplified view since we don't have full API call parsing exposed easily yet.
		// We'll rely on MTSP structure: Culul (CB) -> CS/Ct/etc (Encoders/Calls)

		root := []*APICallNode{}

		// Add Command Buffers (dummy for now as we don't parse them into a list yet)
		// We can iterate DebugGroups to create a structure

		// Map debug groups to encoders
		debugGroupMap := make(map[string][]*APICallNode)

		// Use EncoderLabels to build a flat list or grouped list
		// In a real implementation, we would parse the MTSP stream sequentially
		// and build the tree. Here we'll approximate using labels.

		// Create a "Default" command buffer
		cb := &APICallNode{
			ID:    0,
			Type:  "CommandBuffer",
			Label: "Command Buffer 0",
			Children: []*APICallNode{},
		}

		// Group encoders by debug group if available
		if len(trace.DebugGroupLabels) > 0 {
			for i, group := range trace.DebugGroupLabels {
				node := &APICallNode{
					ID:    i + 1,
					Type:  "DebugGroup",
					Label: group,
					Children: []*APICallNode{},
				}
				debugGroupMap[group] = append(debugGroupMap[group], node)
				cb.Children = append(cb.Children, node)
			}
		}

		// Add Encoders
		for i, label := range trace.EncoderLabels {
			encoderNode := &APICallNode{
				ID:    1000 + i,
				Type:  "Encoder",
				Label: label,
			}

			// Find if this encoder belongs to a debug group
			groupName := trace.GetDebugGroupForLabel(label)
			if groupName != "" && len(debugGroupMap[groupName]) > 0 {
				// Add to the first instance of this group
				parent := debugGroupMap[groupName][0]
				parent.Children = append(parent.Children, encoderNode)
			} else {
				// Add directly to CB
				cb.Children = append(cb.Children, encoderNode)
			}
		}

		root = append(root, cb)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(root)
	}
}
