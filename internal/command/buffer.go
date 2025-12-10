package command

import (
	"github.com/tmc/gputrace/internal/trace"
)

// Re-export types from trace for backwards compatibility.
// These types are defined in internal/trace since that's where they're parsed.
type (
	CommandBuffer  = trace.CommandBuffer
	ComputeEncoder = trace.ComputeEncoder
	DispatchCall   = trace.DispatchCall
	XDICIndex      = trace.XDICIndex
)
