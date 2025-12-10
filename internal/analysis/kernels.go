package analysis

import (
	"fmt"
	"sort"
	"strings"

	"github.com/tmc/gputrace/internal/command"
	"github.com/tmc/gputrace/internal/trace"
)

// KernelStat represents statistics for a single kernel function.
type KernelStat struct {
	Name            string   // Function name (e.g., "g3_copybfloat16bfloat16")
	DispatchCount   int      // Number of times dispatched
	PipelineAddress uint64   // Last observed pipeline state address
	DebugGroups     []string // Associated debug groups
	EncoderLabels   []string // Encoder labels where this kernel appears
}

// KernelReport contains the analysis results for all kernels in a trace.
type KernelReport struct {
	Kernels       []*KernelStat
	TotalKernels  int
	UnknownCount  int
	DispatchCount int
}

// AnalyzeKernels performs detailed analysis of kernels in the trace,
// counting dispatches and associating them with debug groups.
func AnalyzeKernels(t *trace.Trace) (*KernelReport, error) {
	// Initialize report
	report := &KernelReport{
		Kernels: make([]*KernelStat, 0),
	}

	// Track kernels by name
	kernelMap := make(map[string]*KernelStat)

	// Get pipeline -> function map
	pipelineMap := t.BuildPipelineFunctionMap()

	// Parse command buffers to count dispatches
	cbs, err := t.ParseCommandBuffers()
	if err != nil {
		return nil, fmt.Errorf("parse command buffers: %w", err)
	}

	for _, cb := range cbs {
		dcb, err := command.ParseDetailedCommandBuffer(t, cb.Index)
		if err != nil {
			// Skip invalid command buffers
			continue
		}

		// Get dispatches in this command buffer
		data := t.CaptureData
		var cbEnd int64
		if cb.Index+1 < len(cbs) {
			cbEnd = cbs[cb.Index+1].Offset
		} else {
			cbEnd = int64(len(data))
		}

		if cb.Offset >= int64(len(data)) {
			continue
		}
		cbData := data[cb.Offset:cbEnd]
		dispatches, err := t.ParseDispatchInRegion(cbData, cb.Offset)
		if err != nil {
			continue
		}

		// Match dispatches to encoders
		dispatchIdx := 0
		for _, encoder := range dcb.Encoders {
			// Find dispatches that follow this encoder
			// and come before the next encoder.

			nextEncoderOffset := int64(1<<63 - 1) // Max int64
			// Find next encoder
			for _, other := range dcb.Encoders {
				if other.Offset > encoder.Offset {
					if other.Offset < nextEncoderOffset {
						nextEncoderOffset = other.Offset
					}
				}
			}

			// Count dispatches for this encoder
			encoderDispatches := 0
			for i := dispatchIdx; i < len(dispatches); i++ {
				d := dispatches[i]
				if d.Offset > encoder.Offset && d.Offset < nextEncoderOffset {
					encoderDispatches++
					dispatchIdx = i + 1
				} else if d.Offset >= nextEncoderOffset {
					break
				}
			}

			if encoderDispatches > 0 {
				kernelName := "unknown"

				// Strategy 1: Use Encoder Label if it looks like a kernel name
				if encoder.Label != "" {
					if isActualEncoderLabel(encoder.Label) {
						kernelName = encoder.Label
					} else if looksLikeKernelName(encoder.Label) {
						kernelName = encoder.Label
					}
				}

				// Strategy 2: Check calls for setComputePipelineState
				if kernelName == "unknown" {
					for _, call := range dcb.Calls {
						if call.Offset > encoder.Offset && call.Offset < nextEncoderOffset {
							if call.Type == 14 { // setComputePipelineState
								if name, ok := pipelineMap[uint64(call.TargetAddr)]; ok {
									kernelName = name
									break
								}
							}
						}
					}
				}

				// Add to stats
				stat, exists := kernelMap[kernelName]
				if !exists {
					stat = &KernelStat{
						Name: kernelName,
					}
					kernelMap[kernelName] = stat
				}
				stat.DispatchCount += encoderDispatches

				// Associate debug group
				debugGroup := t.GetDebugGroupForLabel(encoder.Label)
				if debugGroup != "" {
					if !contains(stat.DebugGroups, debugGroup) {
						stat.DebugGroups = append(stat.DebugGroups, debugGroup)
					}
				}

				// Associate encoder label
				if encoder.Label != "" {
					if !contains(stat.EncoderLabels, encoder.Label) {
						stat.EncoderLabels = append(stat.EncoderLabels, encoder.Label)
					}
				}
			}
		}

		// Handle any remaining dispatches
		if dispatchIdx < len(dispatches) {
			remaining := len(dispatches) - dispatchIdx
			stat, exists := kernelMap["unknown"]
			if !exists {
				stat = &KernelStat{Name: "unknown"}
				kernelMap["unknown"] = stat
			}
			stat.DispatchCount += remaining
		}
	}

	// Convert map to slice
	for _, stat := range kernelMap {
		report.Kernels = append(report.Kernels, stat)
		if stat.Name == "unknown" {
			report.UnknownCount += stat.DispatchCount
		}
		report.DispatchCount += stat.DispatchCount
	}

	// Sort by dispatch count (descending)
	sort.Slice(report.Kernels, func(i, j int) bool {
		return report.Kernels[i].DispatchCount > report.Kernels[j].DispatchCount
	})

	report.TotalKernels = len(report.Kernels)

	return report, nil
}

// Helper functions duplicated/adapted from trace package since they aren't exported
// Ideally these should be exported from trace package

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

func isActualEncoderLabel(label string) bool {
	if len(label) == 0 {
		return false
	}
	// Must have underscores
	if !strings.Contains(label, "_") {
		return false
	}
	// Filter out debug group hierarchies
	if strings.Contains(label, ":") {
		return false
	}
	// Should start with lowercase
	firstChar := label[0]
	isLowercase := firstChar >= 'a' && firstChar <= 'z'
	return isLowercase
}

func looksLikeKernelName(s string) bool {
	if len(s) < 3 || len(s) > 64 {
		return false
	}
	hasUnderscore := strings.Contains(s, "_")
	hasLower := strings.ToLower(s) != s
	hasDigit := false
	for _, r := range s {
		if r >= '0' && r <= '9' {
			hasDigit = true
			break
		}
	}
	if s == "root" || s == "buffers" || s == "buffer" || s == "textures" || s == "heaps" {
		return false
	}
	return hasUnderscore || (hasLower && (hasDigit || len(s) > 8))
}
