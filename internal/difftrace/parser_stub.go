//go:build !darwin

package difftrace

import (
	"fmt"
	"regexp"
)

// LoadTraceData is unavailable on non-darwin hosts because streamData parsing is darwin-only.
func LoadTraceData(path string, onlyEncoder int, onlyFunction *regexp.Regexp) (*TraceData, error) {
	return &TraceData{Path: path, Label: path, Warnings: []string{"streamData parser is only available on darwin"}},
		fmt.Errorf("streamData parser is only available on darwin")
}
