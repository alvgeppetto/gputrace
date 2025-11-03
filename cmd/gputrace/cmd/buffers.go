package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"github.com/tmc/mlx-go/experiments/gputrace"
)

var (
	buffersSort    string
	buffersMinSize string
	buffersFormat  string
)

var buffersCmd = &cobra.Command{
	Use:   "buffers <trace.gputrace>",
	Short: "List buffers in a GPU trace",
	Long: `Display information about Metal buffers captured in a GPU trace.

This command shows:
  - Buffer IDs and addresses
  - Buffer sizes
  - Buffer usage (total/unique)
  - Aliasing information (symlinks)
  - Buffer bindings to encoders (with --verbose)

The output can be sorted by size, ID, or name, and filtered by minimum size.

Examples:
  gputrace buffers trace.gputrace
  gputrace buffers trace.gputrace --sort size
  gputrace buffers trace.gputrace --min-size 1MB
  gputrace buffers trace.gputrace --format json`,
	Args: cobra.ExactArgs(1),
	RunE: runBuffers,
}

func init() {
	rootCmd.AddCommand(buffersCmd)

	buffersCmd.Flags().StringVar(&buffersSort, "sort", "size", "Sort by: size, id, name")
	buffersCmd.Flags().StringVar(&buffersMinSize, "min-size", "", "Minimum buffer size (e.g., 1KB, 1MB, 1GB)")
	buffersCmd.Flags().StringVar(&buffersFormat, "format", "table", "Output format: table, json, csv")
}

func runBuffers(cmd *cobra.Command, args []string) error {
	tracePath := args[0]

	// Verify trace file exists
	if err := checkTraceFile(tracePath); err != nil {
		return err
	}

	// Open trace
	trace, err := gputrace.Open(tracePath)
	if err != nil {
		return fmt.Errorf("failed to open trace: %w", err)
	}

	// Extract buffer information
	buffers, err := extractBufferInfo(tracePath)
	if err != nil {
		return fmt.Errorf("failed to extract buffer info: %w", err)
	}

	// Parse minimum size if specified
	minSize := uint64(0)
	if buffersMinSize != "" {
		parsed, err := parseSize(buffersMinSize)
		if err != nil {
			return fmt.Errorf("invalid min-size: %w", err)
		}
		minSize = parsed
	}

	// Filter by minimum size
	if minSize > 0 {
		filtered := make([]BufferInfo, 0, len(buffers))
		for _, buf := range buffers {
			if buf.Size >= minSize {
				filtered = append(filtered, buf)
			}
		}
		buffers = filtered
	}

	// Sort buffers
	sortBuffers(buffers, buffersSort)

	// Format and display
	switch buffersFormat {
	case "json":
		return formatBuffersJSON(buffers)
	case "csv":
		return formatBuffersCSV(buffers)
	default:
		return formatBuffersTable(buffers, trace)
	}
}

// BufferInfo contains information about a single buffer.
type BufferInfo struct {
	ID        string
	Filename  string
	Size      uint64
	IsSymlink bool
	Target    string // For symlinks, what they point to
	Aliases   []string
}

// extractBufferInfo scans the trace directory for buffer files.
func extractBufferInfo(tracePath string) ([]BufferInfo, error) {
	entries, err := os.ReadDir(tracePath)
	if err != nil {
		return nil, err
	}

	// Map buffer IDs to their info
	bufferMap := make(map[string]*BufferInfo)
	symlinks := make(map[string][]string) // target -> symlinks

	// First pass: find base buffers and collect symlinks
	for _, entry := range entries {
		name := entry.Name()

		if !strings.HasPrefix(name, "MTLBuffer-") {
			continue
		}

		// Extract buffer ID (e.g., "12" from "MTLBuffer-12-0")
		parts := strings.TrimPrefix(name, "MTLBuffer-")
		idEnd := strings.Index(parts, "-")
		if idEnd == -1 {
			continue
		}
		bufferID := parts[:idEnd]

		// Check if it's a symlink
		fullPath := filepath.Join(tracePath, name)
		info, err := os.Lstat(fullPath)
		if err != nil {
			continue
		}

		if info.Mode()&os.ModeSymlink != 0 {
			// It's a symlink - read target
			target, err := os.Readlink(fullPath)
			if err != nil {
				continue
			}
			symlinks[target] = append(symlinks[target], name)
		} else if strings.HasSuffix(name, "-0") {
			// Base buffer file
			fileInfo, err := os.Stat(fullPath)
			if err != nil {
				continue
			}

			bufferMap[bufferID] = &BufferInfo{
				ID:        bufferID,
				Filename:  name,
				Size:      uint64(fileInfo.Size()),
				IsSymlink: false,
			}
		}
	}

	// Second pass: associate aliases with base buffers
	for target, aliases := range symlinks {
		// Extract buffer ID from target
		parts := strings.TrimPrefix(target, "MTLBuffer-")
		idEnd := strings.Index(parts, "-")
		if idEnd == -1 {
			continue
		}
		targetID := parts[:idEnd]

		if buf, ok := bufferMap[targetID]; ok {
			buf.Aliases = aliases
		}
	}

	// Convert map to slice
	buffers := make([]BufferInfo, 0, len(bufferMap))
	for _, buf := range bufferMap {
		buffers = append(buffers, *buf)
	}

	return buffers, nil
}

// sortBuffers sorts the buffer list by the specified field.
func sortBuffers(buffers []BufferInfo, sortBy string) {
	switch sortBy {
	case "size":
		sort.Slice(buffers, func(i, j int) bool {
			return buffers[i].Size > buffers[j].Size // Descending
		})
	case "id":
		sort.Slice(buffers, func(i, j int) bool {
			return buffers[i].ID < buffers[j].ID
		})
	case "name":
		sort.Slice(buffers, func(i, j int) bool {
			return buffers[i].Filename < buffers[j].Filename
		})
	}
}

// formatBuffersTable formats buffers as a human-readable table.
func formatBuffersTable(buffers []BufferInfo, trace *gputrace.Trace) error {
	// Calculate totals
	var totalSize uint64
	totalAliases := 0
	for _, buf := range buffers {
		totalSize += buf.Size
		totalAliases += len(buf.Aliases)
	}

	// Print summary
	fmt.Printf("=== GPU Trace Buffers ===\n\n")
	fmt.Printf("Total Buffers: %d\n", len(buffers))
	fmt.Printf("Total Size: %s (%.2f MB)\n", formatBytes(totalSize), float64(totalSize)/(1024*1024))
	fmt.Printf("Total Aliases: %d\n\n", totalAliases)

	// Print table header
	fmt.Printf("%-8s %-25s %12s %10s %s\n", "ID", "Filename", "Size", "Size (MB)", "Aliases")
	fmt.Println(strings.Repeat("-", 100))

	// Print each buffer
	for _, buf := range buffers {
		sizeMB := float64(buf.Size) / (1024 * 1024)
		aliasInfo := ""
		if len(buf.Aliases) > 0 {
			if len(buf.Aliases) == 1 {
				aliasInfo = buf.Aliases[0]
			} else {
				aliasInfo = fmt.Sprintf("%d aliases", len(buf.Aliases))
			}
		}

		fmt.Printf("%-8s %-25s %12s %10.2f %s\n",
			buf.ID,
			buf.Filename,
			formatBytes(buf.Size),
			sizeMB,
			aliasInfo,
		)

		// Show all aliases if more than 1
		if len(buf.Aliases) > 1 {
			for _, alias := range buf.Aliases {
				fmt.Printf("%-8s   → %s\n", "", alias)
			}
		}
	}

	return nil
}

// formatBuffersJSON formats buffers as JSON.
func formatBuffersJSON(buffers []BufferInfo) error {
	fmt.Println("[")
	for i, buf := range buffers {
		comma := ","
		if i == len(buffers)-1 {
			comma = ""
		}
		fmt.Printf("  {\"id\": \"%s\", \"filename\": \"%s\", \"size\": %d, \"aliases\": %d}%s\n",
			buf.ID, buf.Filename, buf.Size, len(buf.Aliases), comma)
	}
	fmt.Println("]")
	return nil
}

// formatBuffersCSV formats buffers as CSV.
func formatBuffersCSV(buffers []BufferInfo) error {
	fmt.Println("ID,Filename,Size,Aliases")
	for _, buf := range buffers {
		fmt.Printf("%s,%s,%d,%d\n", buf.ID, buf.Filename, buf.Size, len(buf.Aliases))
	}
	return nil
}

// parseSize parses a size string like "1KB", "1MB", "1GB".
func parseSize(s string) (uint64, error) {
	s = strings.ToUpper(strings.TrimSpace(s))

	multiplier := uint64(1)
	if strings.HasSuffix(s, "KB") {
		multiplier = 1024
		s = strings.TrimSuffix(s, "KB")
	} else if strings.HasSuffix(s, "MB") {
		multiplier = 1024 * 1024
		s = strings.TrimSuffix(s, "MB")
	} else if strings.HasSuffix(s, "GB") {
		multiplier = 1024 * 1024 * 1024
		s = strings.TrimSuffix(s, "GB")
	}

	var value uint64
	_, err := fmt.Sscanf(s, "%d", &value)
	if err != nil {
		return 0, fmt.Errorf("invalid size format: %s", s)
	}

	return value * multiplier, nil
}

// formatBytes formats a byte count as a human-readable string.
func formatBytes(bytes uint64) string {
	const (
		KB = 1024
		MB = 1024 * KB
		GB = 1024 * MB
	)

	if bytes >= GB {
		return fmt.Sprintf("%.2f GB", float64(bytes)/GB)
	} else if bytes >= MB {
		return fmt.Sprintf("%.2f MB", float64(bytes)/MB)
	} else if bytes >= KB {
		return fmt.Sprintf("%.2f KB", float64(bytes)/KB)
	}
	return fmt.Sprintf("%d B", bytes)
}
