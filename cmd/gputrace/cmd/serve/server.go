package serve

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
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

	// Debug: List files in staticFS
	entries, _ := staticFiles.ReadDir("static")
	fmt.Println("Files in embedded static/:")
	for _, e := range entries {
		fmt.Println(" -", e.Name())
		if e.IsDir() {
			subEntries, _ := staticFiles.ReadDir("static/" + e.Name())
			for _, se := range subEntries {
				fmt.Println("   -", se.Name())
			}
		}
	}

	mux := http.NewServeMux()
	setupRoutes(mux, trace)

	fmt.Printf("Serving trace at http://127.0.0.1:%d\n", port)
	return http.ListenAndServe(fmt.Sprintf("127.0.0.1:%d", port), mux)
}

func setupRoutes(mux *http.ServeMux, trace *gputrace.Trace) {
	// API routes
	mux.HandleFunc("/api/stats", apiStatsHandler(trace))
	mux.HandleFunc("/api/kernels", apiKernelsHandler(trace))
	mux.HandleFunc("/api/api-calls", apiCallsAPIHandler(trace))
	mux.HandleFunc("/api/trace", apiTraceHandler(trace))

	// Serve static files
	staticFS, err := fs.Sub(staticFiles, "static")
	if err != nil {
		panic(err)
	}

	fileServer := http.FileServer(http.FS(staticFS))

	// Handle assets specifically
	// Ensure that requests to /assets/ map to the assets directory in staticFS
	mux.Handle("/assets/", fileServer)

	// Catch-all handler for the SPA
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" && r.URL.Path != "/index.html" {
			// If it's not root/index and hasn't been handled by API/assets,
			// try to serve via fileServer (e.g. favicon.ico, etc.)
			fileServer.ServeHTTP(w, r)
			return
		}

		// Serve index.html
		serveStaticFile(w, "static/index.html")
	})
}

func serveStaticFile(w http.ResponseWriter, path string) {
	data, err := staticFiles.ReadFile(path)
	if err != nil {
		http.Error(w, fmt.Sprintf("File not found: %s", path), http.StatusNotFound)
		return
	}
	// Detect content type
	ext := filepath.Ext(path)
	contentType := "text/plain"
	switch ext {
	case ".html":
		contentType = "text/html"
	case ".js":
		contentType = "application/javascript"
	case ".css":
		contentType = "text/css"
	case ".json":
		contentType = "application/json"
	case ".svg":
		contentType = "image/svg+xml"
	}
	w.Header().Set("Content-Type", contentType)
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
