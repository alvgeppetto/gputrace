package serve

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/tmc/gputrace"
)

// ResourceTimelineResponse matches frontend requirements
type ResourceTimelineResponse struct {
	Timestamps []float64 `json:"timestamps"` // Time points in ms
	System     []uint64  `json:"system"`     // Bytes
	Video      []uint64  `json:"video"`      // Bytes
	Shared     []uint64  `json:"shared"`     // Bytes
	Stats      struct {
		PeakMemoryBytes uint64 `json:"peak_memory_bytes"`
		TotalBuffers    int    `json:"total_buffers"`
	} `json:"stats"`
}

func apiResourcesHandler(trace *gputrace.Trace) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		timeline, err := gputrace.ExtractBufferTimeline(trace)
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to extract buffer timeline: %v", err), http.StatusInternalServerError)
			return
		}

		// Convert analysis to timeseries data
		// The analysis gives us "first seen" and "last seen" record indices.
		// We need to map record indices to approximate time or just use record index as x-axis.
		// For now, using record index as a proxy for time is acceptable for visualization.

		// Create 100 sample points across the range
		const numPoints = 100
		minIdx := timeline.MinRecordIndex
		maxIdx := timeline.MaxRecordIndex
		rangeIdx := maxIdx - minIdx
		if rangeIdx <= 0 {
			rangeIdx = 1
		}
		step := float64(rangeIdx) / float64(numPoints)

		resp := ResourceTimelineResponse{
			Timestamps: make([]float64, numPoints),
			System:     make([]uint64, numPoints),
			Video:      make([]uint64, numPoints),
			Shared:     make([]uint64, numPoints),
		}
		resp.Stats.PeakMemoryBytes = timeline.PeakMemoryBytes
		resp.Stats.TotalBuffers = timeline.TotalBuffers

		// Fill data points
		for i := 0; i < numPoints; i++ {
			currentIdx := float64(minIdx) + (float64(i) * step)
			resp.Timestamps[i] = currentIdx // Using index as timestamp for now

			// Calculate active memory at this point
			var activeBytes uint64
			for _, lifecycle := range timeline.BufferEvents {
				if float64(lifecycle.FirstSeen) <= currentIdx && float64(lifecycle.LastSeen) >= currentIdx {
					// We don't have memory storage mode in lifecycle (Shared vs Private)
					// So we'll put everything in "Shared" for now, or distribute if we can find metadata
					// But BufferLifecycle struct doesn't have StorageMode.
					// We'll put everything in Shared as default for Unified Memory.
					activeBytes += lifecycle.Size
				}
			}
			resp.Shared[i] = activeBytes
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}
}
