//go:build darwin

package replay

import (
	"github.com/tmc/appledocs/generated/metal"
)

// CreateBlitEncoder creates a blit command encoder.
func (h *MetalCommandBufferHandle) CreateBlitEncoder() *MetalBlitEncoderHandle {
	encoder := h.cmdBuffer.BlitCommandEncoder()
	return &MetalBlitEncoderHandle{encoder: encoder}
}

// MetalBlitEncoderHandle wraps a blit command encoder.
type MetalBlitEncoderHandle struct {
	encoder metal.MTLBlitCommandEncoder
}

// SampleCounters inserts a counter sample.
func (h *MetalBlitEncoderHandle) SampleCounters(sampleBuffer *MetalCounterSampleBufferHandle, sampleIndex int) {
	h.encoder.SampleCountersInBufferAtSampleIndexWithBarrier(sampleBuffer.buffer, uint(sampleIndex), true)
}

// EndEncoding finishes encoding commands.
func (h *MetalBlitEncoderHandle) EndEncoding() {
	h.encoder.EndEncoding()
}

// Release frees the encoder.
func (h *MetalBlitEncoderHandle) Release() {
}
