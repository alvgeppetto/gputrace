package serve

import (
	"embed"
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"

	"github.com/tmc/gputrace"
)

//go:embed static/*
var staticFiles embed.FS

// StartServer starts the HTTP server.
func StartServer(tracePath string, port int) error {
	trace, err := gputrace.Open(tracePath)
	if err != nil {
		return fmt.Errorf("failed to open trace: %w", err)
	}

	mux := http.NewServeMux()
	setupRoutes(mux, trace)

	return http.ListenAndServe(fmt.Sprintf("127.0.0.1:%d", port), mux)
}

func setupRoutes(mux *http.ServeMux, trace *gputrace.Trace) {
	// Serve static files
	// The embed.FS has "static" as the root directory, so we need to strip "/static/" from the request path
	// but also ensure we are serving from the "static" directory in the FS.
	fs := http.FileServer(http.FS(staticFiles))
	mux.Handle("/static/", fs)

	// Pages
	mux.HandleFunc("/", dashboardHandler)
	mux.HandleFunc("/kernels", kernelsHandler)
	mux.HandleFunc("/api-calls", apiCallsHandler)

	// API
	mux.HandleFunc("/api/stats", apiStatsHandler(trace))
	mux.HandleFunc("/api/kernels", apiKernelsHandler(trace))
	mux.HandleFunc("/api/api-calls", apiCallsAPIHandler(trace))
}

func dashboardHandler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	serveStaticFile(w, "static/index.html")
}

func kernelsHandler(w http.ResponseWriter, r *http.Request) {
	serveStaticFile(w, "static/kernels.html")
}

func apiCallsHandler(w http.ResponseWriter, r *http.Request) {
	serveStaticFile(w, "static/api-calls.html")
}

func serveStaticFile(w http.ResponseWriter, path string) {
	data, err := staticFiles.ReadFile(path)
	if err != nil {
		http.Error(w, fmt.Sprintf("File not found: %s", path), http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/html")
	w.Write(data)
}

func apiStatsHandler(trace *gputrace.Trace) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		stats, err := gputrace.ExtractStatistics(trace)
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to extract stats: %v", err), http.StatusInternalServerError)
			return
		}

		// Use the same structure as the CLI stats command for consistency
		type StatsJSON struct {
			BufferUsageBytes uint64         `json:"buffer_usage_bytes"`
			BufferUsageGB    float64        `json:"buffer_usage_gb"`
			BufferSizeSum    uint64         `json:"buffer_size_sum"`
			UniqueBuffers    int            `json:"unique_buffers"`
			UniqueKernels    int            `json:"unique_kernels"`
			CommandBuffers   int            `json:"command_buffers"`
			ComputeEncoders  int            `json:"compute_encoders"`
			DispatchCalls    int            `json:"dispatch_calls"`
			TotalRecords     int            `json:"total_records"`
			RecordTypes      map[string]int `json:"record_types"`
		}

		type MetadataJSON struct {
			UUID           string `json:"uuid"`
			CaptureVersion int    `json:"capture_version"`
			GraphicsAPI    int    `json:"graphics_api"`
			DeviceID       int    `json:"device_id"`
			TraceName      string `json:"trace_name"`
		}

		response := struct {
			Statistics StatsJSON     `json:"statistics"`
			Metadata   *MetadataJSON `json:"metadata,omitempty"`
		}{
			Statistics: StatsJSON{
				BufferUsageBytes: stats.BufferUsageBytes,
				BufferUsageGB:    stats.BufferUsageGB,
				BufferSizeSum:    stats.BufferSizeSum,
				UniqueBuffers:    stats.UniqueBuffers,
				UniqueKernels:    stats.UniqueKernels,
				CommandBuffers:   stats.CommandBuffers,
				ComputeEncoders:  stats.ComputeEncoders,
				DispatchCalls:    stats.DispatchCalls,
				TotalRecords:     stats.TotalRecords,
				RecordTypes:      stats.RecordTypes,
			},
		}

		if trace.Metadata != nil {
			response.Metadata = &MetadataJSON{
				UUID:           trace.Metadata.UUID,
				CaptureVersion: trace.Metadata.CaptureVersion,
				GraphicsAPI:    trace.Metadata.GraphicsAPI,
				DeviceID:       trace.Metadata.DeviceID,
				TraceName:      filepath.Base(trace.Path),
			}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}
}
