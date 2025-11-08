// Package graph provides graph visualization generation for GPU traces.
package graph

import (
	"github.com/tmc/mlx-go/experiments/gputrace/internal/trace"
)

// Generator is the interface for graph output generators.
type Generator interface {
	// Generate creates a graph visualization from a trace.
	Generate(t *trace.Trace, config *Config) (string, error)
}

// Config holds configuration for graph generation.
type Config struct {
	// Type of graph to generate (hierarchy, flow, resources)
	Type string

	// ShowTiming includes timing information in nodes
	ShowTiming bool

	// ShowMemory includes memory usage information
	ShowMemory bool

	// FilterEncoder filters to specific encoder (empty = all)
	FilterEncoder string

	// FilterShader filters to specific shader (empty = all)
	FilterShader string
}
