package serve

import (
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/tmc/gputrace"
)

// NodeType enum matching frontend types.ts
type NodeType string

const (
	NodeTypeEncoder  NodeType = "encoder"
	NodeTypeDispatch NodeType = "dispatch"
	NodeTypeBarrier  NodeType = "barrier"
	NodeTypeBuffer   NodeType = "buffer"
	NodeTypeRoot     NodeType = "root"
	NodeTypeGroup    NodeType = "group" // Used for debug groups or CBs
)

// TraceStats matching frontend types.ts
type TraceStats struct {
	Duration string `json:"duration"`
	Memory   string `json:"memory"`
	Threads  string `json:"threads"`
}

// ResourceRef matching frontend types.ts
type ResourceRef struct {
	ID     string `json:"id"`
	Label  string `json:"label"`
	Type   string `json:"type"` // 'buffer' | 'texture'
	Size   string `json:"size"`
	Access string `json:"access"` // 'Read' | 'Write' | 'Read/Write'
	Status string `json:"status"` // 'Tracked' | 'Untracked' | 'Shared'
}

// TraceItem matching frontend types.ts
type TraceItem struct {
	ID          string                 `json:"id"`
	Label       string                 `json:"label"`
	Type        NodeType               `json:"type"`
	Description string                 `json:"description,omitempty"`
	Stats       *TraceStats            `json:"stats,omitempty"`
	Children    []*TraceItem           `json:"children,omitempty"`
	Properties  map[string]interface{} `json:"properties,omitempty"`
	Inputs      []ResourceRef          `json:"inputs,omitempty"`
	Outputs     []ResourceRef          `json:"outputs,omitempty"`
}

func apiTraceHandler(trace *gputrace.Trace) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		root, err := buildTraceHierarchy(trace)
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to build trace hierarchy: %v", err), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(root)
	}
}

func buildTraceHierarchy(t *gputrace.Trace) (*TraceItem, error) {
	// Root Node
	root := &TraceItem{
		ID:    "root",
		Label: filepath.Base(t.Path),
		Type:  NodeTypeRoot,
		Stats: &TraceStats{
			Threads: "1", // Dummy
		},
		Children: []*TraceItem{},
	}

	// Calculate total duration for stats
	timings, _ := gputrace.ExtractTimingData(t)
	totalDurationMs := 0.0
	timingMap := make(map[string]*gputrace.EncoderTiming)
	for _, timing := range timings {
		totalDurationMs += timing.DurationMs
		// Map by Label (Assuming unique labels or taking last/first)
		timingMap[timing.Label] = timing
	}
	root.Stats.Duration = fmt.Sprintf("%.2f ms", totalDurationMs)

	// Get Command Buffers
	cbs, err := t.ParseCommandBuffers()
	if err != nil {
		return nil, fmt.Errorf("parse command buffers: %w", err)
	}

	// Use existing capture data from trace struct
	captureData := t.CaptureData
	if len(captureData) == 0 {
		return nil, fmt.Errorf("capture data not loaded")
	}

	// Iterate Command Buffers
	for i, cb := range cbs {
		cbNode := &TraceItem{
			ID:    fmt.Sprintf("cb-%d", cb.Index),
			Label: fmt.Sprintf("Command Buffer %d", cb.Index),
			Type:  NodeTypeGroup,
			Properties: map[string]interface{}{
				"Timestamp": cb.Timestamp,
				"UUID":      cb.UUID,
				"Offset":    cb.Offset,
			},
			Children: []*TraceItem{},
		}

		// Parse details for this CB
		dcb, err := gputrace.ParseDetailedCommandBuffer(t, cb.Index)
		if err != nil {
			fmt.Printf("Warning: failed to parse detailed CB %d: %v\n", cb.Index, err)
			continue
		}

		// Get region data for dispatches
		var cbEnd int64
		if i+1 < len(cbs) {
			cbEnd = cbs[i+1].Offset
		} else {
			cbEnd = int64(len(captureData))
		}

		// Ensure bounds
		if dcb.Offset < 0 { dcb.Offset = 0 }
		if cbEnd > int64(len(captureData)) { cbEnd = int64(len(captureData)) }
		if dcb.Offset > cbEnd {
			continue
		}

		cbRegion := captureData[dcb.Offset:cbEnd]
		dispatches, err := t.ParseDispatchInRegion(cbRegion, dcb.Offset)
		if err != nil {
			fmt.Printf("Warning: failed to parse dispatches for CB %d: %v\n", cb.Index, err)
		}

		// Map encoders to debug groups
		var currentGroupNode *TraceItem
		var currentGroupName string

		for encIdx, encoder := range dcb.Encoders {
			groupName := t.GetDebugGroupForLabel(encoder.Label)

			if groupName != currentGroupName {
				if groupName != "" {
					groupID := fmt.Sprintf("cb-%d-group-%s-%d", cb.Index, sanitizeID(groupName), encIdx)
					newGroup := &TraceItem{
						ID:    groupID,
						Label: groupName,
						Type:  NodeTypeGroup,
						Children: []*TraceItem{},
					}
					cbNode.Children = append(cbNode.Children, newGroup)
					currentGroupNode = newGroup
				} else {
					currentGroupNode = nil
				}
				currentGroupName = groupName
			}

			// Create Encoder Node
			encNode := &TraceItem{
				ID:    fmt.Sprintf("cb-%d-enc-%d", cb.Index, encoder.Index),
				Label: encoder.Label,
				Type:  NodeTypeEncoder,
				Description: fmt.Sprintf("Encoder at 0x%x", encoder.Address),
				Properties: map[string]interface{}{
					"Address": fmt.Sprintf("0x%x", encoder.Address),
					"Offset":  encoder.Offset,
				},
				Children: []*TraceItem{},
			}

			if timing, ok := timingMap[encoder.Label]; ok {
				encNode.Stats = &TraceStats{
					Duration: fmt.Sprintf("%.2f ms", timing.DurationMs),
				}
				encNode.Properties["Duration"] = fmt.Sprintf("%.2f ms", timing.DurationMs)
				encNode.Properties["% of GPU"] = fmt.Sprintf("%.2f%%", timing.Percentage)
			}

			nextOffset := cbEnd
			if encIdx+1 < len(dcb.Encoders) {
				nextOffset = dcb.Encoders[encIdx+1].Offset
			}

			for dispIdx, disp := range dispatches {
				if disp.Offset >= encoder.Offset && disp.Offset < nextOffset {
					dispID := fmt.Sprintf("disp-%d-%d", cb.Index, dispIdx)
					label := "Dispatch"

					dispNode := &TraceItem{
						ID:    dispID,
						Label: fmt.Sprintf("%s [%d, %d, %d]", label, disp.ThreadsX, disp.ThreadsY, disp.ThreadsZ),
						Type:  NodeTypeDispatch,
						Stats: &TraceStats{
							Threads: fmt.Sprintf("(%d, %d, %d)", disp.ThreadsX, disp.ThreadsY, disp.ThreadsZ),
						},
						Properties: map[string]interface{}{
							"Grid Size": fmt.Sprintf("(%d, %d, %d)", disp.ThreadsX, disp.ThreadsY, disp.ThreadsZ),
							"Threadgroup Size": fmt.Sprintf("(%d, %d, %d)", disp.ThreadsPerGroupX, disp.ThreadsPerGroupY, disp.ThreadsPerGroupZ),
							"Offset": disp.Offset,
						},
					}
					encNode.Children = append(encNode.Children, dispNode)
				}
			}

			if currentGroupNode != nil {
				currentGroupNode.Children = append(currentGroupNode.Children, encNode)
			} else {
				cbNode.Children = append(cbNode.Children, encNode)
			}
		}

		root.Children = append(root.Children, cbNode)
	}

	return root, nil
}

func sanitizeID(s string) string {
	s = strings.ReplaceAll(s, " ", "-")
	s = strings.ReplaceAll(s, ":", "-")
	return s
}
