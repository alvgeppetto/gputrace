package serve

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/tmc/gputrace"
)

// TimelineEvent matches the frontend expectations
type TimelineEvent struct {
	ID         string  `json:"id"`
	TrackID    string  `json:"track_id"`
	Label      string  `json:"label"`
	StartMs    float64 `json:"start_ms"`
	DurationMs float64 `json:"duration_ms"`
	Type       string  `json:"type"` // encoder, dispatch
	Color      string  `json:"color,omitempty"`
}

// TimelineTrack matches the frontend expectations
type TimelineTrack struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Type  string `json:"type"` // encoder, compute
}

// TimelineResponse contains all data needed for the timeline view
type TimelineResponse struct {
	TotalDurationMs float64          `json:"total_duration_ms"`
	Tracks          []TimelineTrack  `json:"tracks"`
	Events          []TimelineEvent  `json:"events"`
	Counters        map[string][]int `json:"counters,omitempty"` // Placeholder for perf counters
}

func apiTimelineHandler(trace *gputrace.Trace) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		timings, err := gputrace.ExtractTimingData(trace)
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to extract timing: %v", err), http.StatusInternalServerError)
			return
		}

		response := TimelineResponse{
			Tracks: []TimelineTrack{
				{ID: "compute", Label: "Compute Encoders", Type: "encoder"},
			},
			Events: []TimelineEvent{},
		}

		var maxEndTime float64

		// If no timing data is available (e.g. no .gpuprofiler_raw or similar), we might return empty
		// or synthetic data. gputrace.ExtractTimingData handles this gracefully usually.

		// Normalize times
		var minStart uint64
		if len(timings) > 0 {
			minStart = timings[0].StartTimestamp
			for _, t := range timings {
				if t.StartTimestamp < minStart {
					minStart = t.StartTimestamp
				}
			}
		}

		// Convert to events
		// Timebase: we need conversion factor from Mach time to ms.
		// Usually 1 tick = 1 ns on Apple Silicon, so / 1e6 for ms.
		const timeScale = 1e6

		for i, t := range timings {
			startMs := float64(t.StartTimestamp-minStart) / timeScale
			endMs := float64(t.EndTimestamp-minStart) / timeScale
			if endMs > maxEndTime {
				maxEndTime = endMs
			}

			response.Events = append(response.Events, TimelineEvent{
				ID:         fmt.Sprintf("ev-%d", i),
				TrackID:    "compute",
				Label:      t.Label,
				StartMs:    startMs,
				DurationMs: t.DurationMs,
				Type:       "encoder",
				Color:      "bg-blue-600/50", // Default color
			})
		}

		response.TotalDurationMs = maxEndTime

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}
}
