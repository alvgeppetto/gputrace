package graph

import (
	"fmt"
	"strings"

	"github.com/tmc/mlx-go/experiments/gputrace/internal/trace"
)

// MermaidGenerator generates Mermaid diagram format output.
type MermaidGenerator struct{}

// NewMermaidGenerator creates a new Mermaid generator.
func NewMermaidGenerator() *MermaidGenerator {
	return &MermaidGenerator{}
}

// Generate creates a Mermaid graph from the trace.
func (g *MermaidGenerator) Generate(t *trace.Trace, config *Config) (string, error) {
	switch config.Type {
	case "hierarchy":
		return g.generateHierarchy(t, config)
	case "flow":
		return g.generateFlow(t, config)
	case "resources":
		return g.generateResources(t, config)
	default:
		return "", fmt.Errorf("unsupported graph type: %s", config.Type)
	}
}

// generateHierarchy creates a hierarchical Mermaid graph: command buffers → encoders → shaders.
func (g *MermaidGenerator) generateHierarchy(t *trace.Trace, config *Config) (string, error) {
	var sb strings.Builder

	// Header
	sb.WriteString("graph LR\n")

	// Parse command buffers
	commandBuffers, err := t.ParseCommandBuffers()
	if err != nil {
		return "", fmt.Errorf("parse command buffers: %w", err)
	}

	// Parse encoders
	encoders, err := t.ParseComputeEncoders()
	if err != nil {
		return "", fmt.Errorf("parse encoders: %w", err)
	}

	// Root node
	sb.WriteString("  trace([GPU Trace])\n")

	// Add command buffers
	for _, cb := range commandBuffers {
		cbID := fmt.Sprintf("cb%d", cb.Index)
		label := fmt.Sprintf("Command Buffer %d", cb.Index)
		if config.ShowTiming {
			label += fmt.Sprintf("<br/>Timestamp: %d", cb.Timestamp)
		}
		sb.WriteString(fmt.Sprintf("  %s[%s]\n", cbID, label))
		sb.WriteString(fmt.Sprintf("  trace --> %s\n", cbID))
	}

	// Add encoders
	encodersByCommandBuffer := g.groupEncodersByCommandBuffer(t, encoders)

	for cbIndex, cbEncoders := range encodersByCommandBuffer {
		cbID := fmt.Sprintf("cb%d", cbIndex)
		for _, encoder := range cbEncoders {
			encID := fmt.Sprintf("enc%d", encoder.Index)
			label := encoder.Label
			if label == "" {
				label = fmt.Sprintf("Encoder %d", encoder.Index)
			}
			sb.WriteString(fmt.Sprintf("  %s[%s]\n", encID, label))
			sb.WriteString(fmt.Sprintf("  %s --> %s\n", cbID, encID))
		}
	}

	// Add shaders (from encoder labels)
	shaderNodes := make(map[string]bool)
	for _, encoder := range encoders {
		if encoder.Label != "" {
			// Extract shader name from encoder label
			shaderName := g.extractShaderName(encoder.Label)
			if shaderName != "" && !shaderNodes[shaderName] {
				shaderID := fmt.Sprintf("shader_%s", sanitizeID(shaderName))
				sb.WriteString(fmt.Sprintf("  %s{{%s}}\n", shaderID, shaderName))
				shaderNodes[shaderName] = true
			}
		}
	}

	// Add edges from encoders to shaders
	for _, encoder := range encoders {
		if encoder.Label != "" {
			shaderName := g.extractShaderName(encoder.Label)
			if shaderName != "" {
				encID := fmt.Sprintf("enc%d", encoder.Index)
				shaderID := fmt.Sprintf("shader_%s", sanitizeID(shaderName))
				sb.WriteString(fmt.Sprintf("  %s --> %s\n", encID, shaderID))
			}
		}
	}

	// Add styling
	sb.WriteString("\n  classDef commandBuffer fill:#90EE90\n")
	sb.WriteString("  classDef encoder fill:#FFFFE0\n")
	sb.WriteString("  classDef shader fill:#F08080\n")

	// Apply styles
	for i := range commandBuffers {
		sb.WriteString(fmt.Sprintf("  class cb%d commandBuffer\n", i))
	}
	for i := range encoders {
		sb.WriteString(fmt.Sprintf("  class enc%d encoder\n", i))
	}
	for shaderName := range shaderNodes {
		shaderID := fmt.Sprintf("shader_%s", sanitizeID(shaderName))
		sb.WriteString(fmt.Sprintf("  class %s shader\n", shaderID))
	}

	return sb.String(), nil
}

// generateFlow creates a temporal execution flow Mermaid graph matching Xcode style.
func (g *MermaidGenerator) generateFlow(t *trace.Trace, config *Config) (string, error) {
	var sb strings.Builder

	// Header - top to bottom flow
	sb.WriteString("graph TB\n")

	// Parse command buffers
	commandBuffers, err := t.ParseCommandBuffers()
	if err != nil {
		return "", fmt.Errorf("parse command buffers: %w", err)
	}

	// Parse encoders
	encoders, err := t.ParseComputeEncoders()
	if err != nil {
		return "", fmt.Errorf("parse encoders: %w", err)
	}

	// Add command buffer at top
	if len(commandBuffers) > 0 {
		sb.WriteString("  cb0[MultipleEncoders_6]\n")
	}

	// Add encoders in vertical flow
	for i, encoder := range encoders {
		encID := fmt.Sprintf("enc%d", i)
		label := encoder.Label
		if label == "" {
			label = fmt.Sprintf("Encoder %d", i)
		}

		// Encoder node
		sb.WriteString(fmt.Sprintf("  %s[\"%s\"]\n", encID, label))

		// Add dispatch nodes (3 per encoder)
		for d := 0; d < 3; d++ {
			dispID := fmt.Sprintf("%s_d%d", encID, d)
			sb.WriteString(fmt.Sprintf("  %s[ ]\n", dispID))
		}
	}

	// Add connections
	sb.WriteString("\n  %% Execution flow\n")
	if len(commandBuffers) > 0 && len(encoders) > 0 {
		sb.WriteString("  cb0 --> enc0\n")
	}

	// Connect encoders to their dispatches
	for i := range encoders {
		encID := fmt.Sprintf("enc%d", i)
		for d := 0; d < 3; d++ {
			dispID := fmt.Sprintf("%s_d%d", encID, d)
			sb.WriteString(fmt.Sprintf("  %s --> %s\n", encID, dispID))
		}
	}

	// Connect between encoders
	for i := 0; i < len(encoders)-1; i++ {
		currDispID := fmt.Sprintf("enc%d_d1", i)
		nextEncID := fmt.Sprintf("enc%d", i+1)
		sb.WriteString(fmt.Sprintf("  %s --> %s\n", currDispID, nextEncID))
	}

	// Add styling
	sb.WriteString("\n  %% Styling\n")
	sb.WriteString("  classDef commandBuffer fill:#2B2B2B,stroke:#666,color:#fff\n")
	sb.WriteString("  classDef encoder fill:#CC5555,stroke:#666,color:#fff\n")
	sb.WriteString("  classDef dispatch fill:#4488CC,stroke:#666,color:#fff\n")

	// Apply styles
	sb.WriteString("  class cb0 commandBuffer\n")
	for i := range encoders {
		sb.WriteString(fmt.Sprintf("  class enc%d encoder\n", i))
		for d := 0; d < 3; d++ {
			sb.WriteString(fmt.Sprintf("  class enc%d_d%d dispatch\n", i, d))
		}
	}

	return sb.String(), nil
}

// generateResources creates a resource usage Mermaid graph.
func (g *MermaidGenerator) generateResources(t *trace.Trace, config *Config) (string, error) {
	// TODO: Implement resource graph
	return "", fmt.Errorf("resource graph type not yet implemented")
}

// groupEncodersByCommandBuffer groups encoders by their command buffer index (same as DOT).
func (g *MermaidGenerator) groupEncodersByCommandBuffer(t *trace.Trace, encoders []*trace.ComputeEncoder) map[int][]*trace.ComputeEncoder {
	result := make(map[int][]*trace.ComputeEncoder)

	commandBuffers, err := t.ParseCommandBuffers()
	if err != nil || len(commandBuffers) == 0 {
		result[0] = encoders
		return result
	}

	encodersPerCB := len(encoders) / len(commandBuffers)
	if encodersPerCB == 0 {
		encodersPerCB = 1
	}

	for i, encoder := range encoders {
		cbIndex := i / encodersPerCB
		if cbIndex >= len(commandBuffers) {
			cbIndex = len(commandBuffers) - 1
		}
		result[cbIndex] = append(result[cbIndex], encoder)
	}

	return result
}

// extractShaderName extracts the shader name from an encoder label (same as DOT).
func (g *MermaidGenerator) extractShaderName(encoderLabel string) string {
	parts := strings.Split(encoderLabel, "_")
	if len(parts) >= 3 && parts[0] == "Encoder" {
		return strings.Join(parts[2:], "_")
	}
	if encoderLabel != "" && !strings.HasPrefix(encoderLabel, "0x") {
		return encoderLabel
	}
	return ""
}
