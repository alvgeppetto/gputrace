package trace

// EncoderTiming represents GPU timing information for a compute encoder.
// This is a core type used throughout the system for representing timing data.
type EncoderTiming struct {
	Label          string  `json:"label"`
	KernelName     string  `json:"kernel_name,omitempty"`
	StartTimestamp uint64  `json:"start_timestamp"`
	EndTimestamp   uint64  `json:"end_timestamp"`
	DurationNs     uint64  `json:"duration_ns"`
	DurationMs     float64 `json:"duration_ms"`
	Percentage     float32 `json:"percentage"`
	QueueID        uint64  `json:"queue_id,omitempty"`
	CommandQueue   string  `json:"command_queue,omitempty"`
}
