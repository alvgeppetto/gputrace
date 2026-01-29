package cmd

import (
	"encoding/json"
	"fmt"
	"sort"

	"github.com/spf13/cobra"
	"github.com/tmc/gputrace/internal/trace"
)

var analyzeUsageCmd = &cobra.Command{
	Use:   "analyze-usage [trace-path]",
	Short:  "Analyze buffer usage across kernels",
	Hidden: true,
	Args:  cobra.ExactArgs(1),
	RunE:  runAnalyzeUsage,
}

var analyzeFormat string

func init() {
	analyzeUsageCmd.Flags().StringVar(&analyzeFormat, "format", "text", "Output format (text, dot, json)")
	rootCmd.AddCommand(analyzeUsageCmd)
}

func runAnalyzeUsage(cmd *cobra.Command, args []string) error {
	tracePath := args[0]
	t, err := trace.Open(tracePath)
	if err != nil {
		return fmt.Errorf("open trace: %w", err)
	}
	defer t.Close()

	records, err := t.ParseMTSPRecords()
	if err != nil {
		return fmt.Errorf("parse records: %w", err)
	}

	// 1. Build Symbol Table (Addr -> Name)
	bufferNames := make(map[uint64]string)
	kernelNames := make(map[uint64]string)

	scanForSymbols := func(recs []trace.MTSPRecord) {
		for _, rec := range recs {
			if rec.Type == trace.RecordTypeCtU {
				if ctu, err := rec.ParseCtURecord(); err == nil {
					bufferNames[ctu.Address] = ctu.Name
				}
			}
			if (rec.Type == trace.RecordTypeCS || rec.Type == trace.RecordTypeCSuwuw) && rec.Label != "" {
				kernelNames[rec.Address] = rec.Label
			}
		}
	}
	for _, data := range t.DeviceResources {
		if recs, err := t.ParseMTSPFromData(data); err == nil {
			scanForSymbols(recs)
		}
	}
	scanForSymbols(records)

	// 2. Aggregate Usage
	type usageStats struct {
		Dispatches int
		Kernels    map[uint64]int // KernelAddr -> count
	}
	bufferUsage := make(map[uint64]*usageStats)

	var scanRecords func([]trace.MTSPRecord)
	scanRecords = func(recs []trace.MTSPRecord) {
		for _, rec := range recs {
			// Check nested
			if nested, err := t.ParseNestedRecords(rec); err == nil && len(nested) > 0 {
				scanRecords(nested)
			}

			var bindings []uint64
			var funcAddr uint64

			switch rec.Type {
			case trace.RecordTypeCt:
				if ct, err := rec.ParseCtRecord(); err == nil {
					bindings = ct.BufferBindings
					funcAddr = ct.FunctionAddr
				}
			case trace.RecordTypeCtt:
				if ctt, err := rec.ParseCttRecord(); err == nil {
					bindings = ctt.BufferBindings
					funcAddr = ctt.FunctionAddr
				}
			case trace.RecordTypeCtulul:
				if ctulul, err := rec.ParseCtululRecord(); err == nil {
					bindings = ctulul.BufferBindings
					// Ctulul doesn't explicitly show FuncAddr, maybe context dependent?
					// Or PipelineAddr. Let's group by Pipeline if Func missing?
					funcAddr = ctulul.PipelineAddr // Fallback unique ID
				}
			}

			if len(bindings) > 0 {
				for _, bAddr := range bindings {
					if _, ok := bufferUsage[bAddr]; !ok {
						bufferUsage[bAddr] = &usageStats{Kernels: make(map[uint64]int)}
					}
					bufferUsage[bAddr].Dispatches++
					bufferUsage[bAddr].Kernels[funcAddr]++
				}
			}
		}
	}
	scanRecords(records)

	// 3. Output
	if analyzeFormat == "json" {
		type kernelUsage struct {
			Name  string `json:"name"`
			Count int    `json:"count"`
		}
		type bufferUsageJSON struct {
			Address    string        `json:"address"`
			Name       string        `json:"name"`
			Dispatches int           `json:"dispatches"`
			Kernels    []kernelUsage `json:"kernels"`
		}
		var out []bufferUsageJSON
		for bAddr, stats := range bufferUsage {
			name := bufferNames[bAddr]
			if name == "" {
				name = fmt.Sprintf("Buffer@0x%x", bAddr)
			}
			entry := bufferUsageJSON{
				Address:    fmt.Sprintf("0x%x", bAddr),
				Name:       name,
				Dispatches: stats.Dispatches,
			}
			for kAddr, count := range stats.Kernels {
				kName := kernelNames[kAddr]
				if kName == "" {
					kName = fmt.Sprintf("Kernel/Pipeline@0x%x", kAddr)
				}
				entry.Kernels = append(entry.Kernels, kernelUsage{Name: kName, Count: count})
			}
			out = append(out, entry)
		}
		data, err := json.MarshalIndent(out, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal json: %w", err)
		}
		fmt.Println(string(data))
		return nil
	}

	if analyzeFormat == "dot" {
		fmt.Printf("digraph G {\n")
		fmt.Printf("  rankdir=LR;\n")
		for bAddr, stats := range bufferUsage {
			if stats.Dispatches < 2 {
				continue
			} // Filter noise
			bName := fmt.Sprintf("Buffer_0x%x", bAddr)
			if name, ok := bufferNames[bAddr]; ok {
				bName = fmt.Sprintf("%s\n(0x%x)", name, bAddr)
			}
			fmt.Printf("  \"%d\" [shape=box, label=%q];\n", bAddr, bName)

			for kAddr := range stats.Kernels {
				kName := fmt.Sprintf("Kernel_0x%x", kAddr)
				if name, ok := kernelNames[kAddr]; ok {
					kName = name
				}
				// TODO: We need read/write direction to make arrows meaningful.
				// For now, undirected or bidirectional? Or just Kernel -> Buffer?
				// Ctulul is "Dispatch", so Kernel uses Buffer.
				fmt.Printf("  \"%d\" -> \"%d\";\n", kAddr, bAddr)
				fmt.Printf("  \"%d\" [shape=ellipse, label=%q];\n", kAddr, kName)
			}
		}
		fmt.Printf("}\n")
		return nil
	}

	fmt.Printf("Trace Buffer Usage Analysis\n")
	fmt.Printf("=============================\n")

	// Sort by most used
	var sortedBuffers []uint64
	for bAddr := range bufferUsage {
		sortedBuffers = append(sortedBuffers, bAddr)
	}
	sort.Slice(sortedBuffers, func(i, j int) bool {
		return bufferUsage[sortedBuffers[i]].Dispatches > bufferUsage[sortedBuffers[j]].Dispatches
	})

	for _, bAddr := range sortedBuffers {
		stats := bufferUsage[bAddr]
		name := bufferNames[bAddr]
		if name == "" {
			name = fmt.Sprintf("Buffer@0x%x", bAddr)
		} else {
			name = fmt.Sprintf("%s (0x%x)", name, bAddr)
		}

		fmt.Printf("\n%s: Used in %d dispatches\n", name, stats.Dispatches)
		for kAddr, count := range stats.Kernels {
			kName := kernelNames[kAddr]
			if kName == "" {
				kName = fmt.Sprintf("Kernel/Pipeline@0x%x", kAddr)
			}
			fmt.Printf("  - %s: %d\n", kName, count)
		}
	}

	return nil
}
