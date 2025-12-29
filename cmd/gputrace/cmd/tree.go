package cmd

import (
	"encoding/binary"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/tmc/gputrace/internal/trace"
)

var (
	treeGroupBy string
	treeVerbose bool
)

var treeCmd = &cobra.Command{
	Use:   "tree [trace-path]",
	Short: "Display execution tree grouped by pipeline state or encoder",
	Long: `Display a hierarchical view of GPU execution.

Grouping modes:
  - encoder:  Group by Encoder (Command Buffer), then Commands (default)
  - pipeline: Group by Compute Pipeline State, then Kernel`,
	Args: cobra.ExactArgs(1),
	RunE: runTree,
}

func init() {
	rootCmd.AddCommand(treeCmd)
	treeCmd.Flags().StringVar(&treeGroupBy, "group-by", "encoder", "Grouping mode: encoder, pipeline")
	treeCmd.Flags().BoolVarP(&treeVerbose, "verbose", "v", false, "Show detailed information")
}

func runTree(cmd *cobra.Command, args []string) error {
	tracePath := args[0]
	t, err := trace.Open(tracePath)
	if err != nil {
		return fmt.Errorf("open trace: %w", err)
	}
	defer t.Close()

	// 1. Parse top-level MTSP records (preserving hierarchy)
	records, err := t.ParseMTSPRecords()
	if err != nil {
		return fmt.Errorf("parse records: %w", err)
	}

	// 2. Build symbol table (FunctionAddr -> Name)
	addrToName := make(map[uint64]string)
	// Scan device resources first
	for addr, label := range t.DeviceLabels {
		addrToName[addr] = label
	}
	// Scan buffer labels from trace extraction
	// Note: We need a way to map buffer labels to addresses if they weren't in DeviceLabels
	// Currently extractStringsFromMTSP populates lists but not a map.
	// But `t.DeviceLabels` is populated by `extractDeviceLabels` using addresses.

	// Flatten records recursively to handle containerized traces
	// We preserve containers so that hierarchy markers (CS) are still visible
	var flattened []trace.MTSPRecord
	var flatten func([]trace.MTSPRecord)
	flatten = func(recs []trace.MTSPRecord) {
		for _, rec := range recs {
			// Always append the record itself (even if it's a container)
			flattened = append(flattened, rec)

			// Check for nested children
			nested, err := t.ParseNestedRecords(rec)
			if err == nil && len(nested) > 0 {
				flatten(nested)
			}
		}
	}
	flatten(records)

	// Scan main records (flattened)
	scanForNames(flattened, addrToName)

	// 3. Render Tree based on grouping
	switch treeGroupBy {
	case "encoder":
		return renderEncoderTree(t, flattened, addrToName)
	case "pipeline":
		return renderPipelineTree(t, flattened, addrToName)
	default:
		return fmt.Errorf("unknown group-by mode: %s", treeGroupBy)
	}
}

// scanForNames recursively scans records for CS/CSuwuw labels
func scanForNames(records []trace.MTSPRecord, addrToName map[uint64]string) {
	for _, rec := range records {
		if rec.Type == trace.RecordTypeCS {
			// Populate address types
			if rec.Label != "" {
				addrToName[rec.Address] = rec.Label
				if rec.SecondaryAddr != 0 {
					addrToName[rec.SecondaryAddr] = rec.Label
				}
			}
		} else if rec.Type == trace.RecordTypeCSuwuw && rec.Label != "" && rec.Address != 0 {
			addrToName[rec.Address] = rec.Label
		} else if rec.Type == trace.RecordTypeCtU {
			if ctu, err := rec.ParseCtURecord(); err == nil && ctu.Name != "" {
				addrToName[ctu.Address] = ctu.Name
			}
		}
		// Recurse using shared logic
		// Note: We create a dummy trace instance to access the method if needed,
		// but since scanForNames is standalone, we rely on the caller passing flattened or we re-parse.
		// Actually, for simple scanning, we can just peek into data if we suspect nested.
		// But cleaner to rely on what we have. For this pass, top-level CS are most important.
		// If we wanted deep scan we'd need to parse nested here.
		// Let's rely on top-level and device-resources for now as that covers 99% of cases.
	}
}

func renderEncoderTree(t *trace.Trace, records []trace.MTSPRecord, addrToName map[uint64]string) error {
	fmt.Println(Colorize("GpuTrace Execution Tree (Hierarchical)", ColorBold))

	// Indentation state
	indent := ""

	// Track Encoder State: EncoderID -> PipelineStateID
	encoderToPipeline := make(map[uint64]uint64)
	// Track Pipeline State: PipelineStateID -> FunctionID
	pipelineToFunc := make(map[uint64]uint64)

	// Pre-scan for Ctt records to ensure mapping is available before processing Ct records
	for _, rec := range records {
		if rec.Type == trace.RecordTypeCtt {
			if ctt, err := rec.ParseCttRecord(); err == nil {
				pipelineToFunc[ctt.PipelineAddr] = ctt.FunctionAddr
			}
		}
	}

	for _, rec := range records {
		// handle nested records first if any (though usually linear for minimal captures)
		// But here we assume linear stream for the command buffer logic

		switch rec.Type {
		case trace.RecordTypeCS:
			flags := uint32(0)
			if len(rec.Data) >= 8 {
				flags = binary.LittleEndian.Uint32(rec.Data[4:8])
			}

			// Flags analysis from Swift trace:
			// 0x...3d: PushDebugGroup
			// 0x...13: SetLabel
			// 0x...2d: Encoder Label

			if flags&0xFF == 0x3d {
				fmt.Printf("%s%s %s\n", indent, Colorize("📁", ColorBlue), Colorize(rec.Label, ColorBold))
				indent += "  "
			} else if flags&0xFF == 0x13 {
				fmt.Printf("%s%s %s\n", indent, Colorize("⌘", ColorBlue), Colorize(rec.Label, ColorYellow))
				indent += "  "
			} else if flags&0xFF == 0x2d {
				fmt.Printf("%s%s %s\n", indent, Colorize("ƒ", ColorBlue), Colorize(rec.Label, ColorPurple))
				indent += "  "
			} else {
				// Standard CS (Kernel Name often)
				if rec.Label != "" {
					fmt.Printf("%s%s  %s\n", indent, Colorize("🏷", ColorBlue), Colorize(rec.Label, ColorGreen))
				}
			}

		case trace.RecordTypeC:
			if c, err := rec.ParseCRecord(); err == nil {
				// PopDebugGroup
				if c.CommandFlags&0xFF == 0x3e {
					if len(indent) >= 2 {
						indent = indent[:len(indent)-2]
					}
					fmt.Printf("%s%s Pop Group\n", indent, Colorize("▲", ColorBlue))
				} else if c.CommandFlags&0xFF == 0x3b {
					if len(indent) >= 2 {
						indent = indent[:len(indent)-2]
					}
					fmt.Printf("%s%s End Encoding\n", indent, Colorize("▲", ColorBlue))
				} else if c.CommandFlags&0xFF == 0x17 {
					if len(indent) >= 2 {
						indent = indent[:len(indent)-2]
					}
					fmt.Printf("%s%s Commit\n", indent, Colorize("✓", ColorBlue))
				} else if c.CommandFlags&0xFF == 0x1d {
					fmt.Printf("%s%s Wait\n", indent, Colorize("⏸", ColorGray))
				}
			}

		case trace.RecordTypeCtulul:
			if ctulul, err := rec.ParseCtululRecord(); err == nil && treeVerbose {
				fmt.Printf("%s%s Set Buffer (Pipeline: %s)\n", indent, Colorize("•", ColorGray), Colorize(fmt.Sprintf("0x%x", ctulul.PipelineAddr), ColorCyan))
			}

		case trace.RecordTypeCtt:
			// Link PipelineState -> Function
			// Already handled in pre-scan
		case trace.RecordTypeCt:
			// Dispatch or Set Pipeline State?
			// In Swift trace, this appears to set the pipeline state for an encoder
			if ct, err := rec.ParseCtRecord(); err == nil {
				// We assume ct.PipelineAddr is actually the Encoder Address here
				// And ct.FunctionAddr appears to be the Pipeline ID (based on Xcode trace matching)
				encoderToPipeline[ct.PipelineAddr] = ct.FunctionAddr

				// Display Buffer Bindings in Encoder View
				if len(ct.BufferBindings) > 0 && treeVerbose {
					indentStr := indent
					fmt.Printf("%s%s Set Bindings (Pipeline: %s)\n", indentStr, Colorize("•", ColorGray), Colorize(fmt.Sprintf("0x%x", ct.FunctionAddr), ColorCyan))
					for i, b := range ct.BufferBindings {
						bName := addrToName[b]
						if bName == "" {
							fmt.Printf("%s  - Bind %d: %s\n", indentStr, i, Colorize(fmt.Sprintf("0x%x", b), ColorCyan))
						} else {
							fmt.Printf("%s  - Bind %d: %s (%s)\n", indentStr, i, Colorize(bName, ColorGreen), Colorize(fmt.Sprintf("0x%x", b), ColorCyan))
						}
					}
				}
			}

		case trace.RecordTypeC_3ul:
			if d, err := rec.ParseDispatchRecord(); err == nil {
				// Resolve Kernel Name via Chain: Encoder -> Pipeline -> Function -> Name
				pipelineID := encoderToPipeline[d.EncoderID]
				funcID := pipelineToFunc[pipelineID]
				funcName := addrToName[funcID]

				if funcName == "" {
					// Fallback: name might be directly on pipeline or encoder (unlikely but safe)
					if name := addrToName[pipelineID]; name != "" {
						funcName = name
					} else if name := addrToName[d.EncoderID]; name != "" {
						funcName = name
					}

					if funcName == "" && pipelineID != 0 {
						funcName = fmt.Sprintf("Func@0x%x", funcID)
					}
				}

				if funcName == "" {
					funcName = "UnknownKernel"
				}

				if treeVerbose {
					fmt.Printf("%s%s %s [dispatchThreads:%d,%d,%d threadsPerThreadgroup:%d,%d,%d] (Index: ?)\n",
						indent,
						Colorize("▦", ColorBlue),
						Colorize(funcName, ColorGreen),
						d.GridSize[0], d.GridSize[1], d.GridSize[2],
						d.GroupSize[0], d.GroupSize[1], d.GroupSize[2])
					fmt.Printf("%s  • Encoder: %s\n", indent, Colorize(fmt.Sprintf("0x%x", d.EncoderID), ColorCyan))
					fmt.Printf("%s  • Function: %s\n", indent, Colorize(fmt.Sprintf("0x%x", funcID), ColorCyan))
				} else {
					fmt.Printf("%s%s %s [dispatchThreads:%d,%d,%d threadsPerThreadgroup:%d,%d,%d]\n",
						indent,
						Colorize("▦", ColorBlue),
						Colorize(funcName, ColorGreen),
						d.GridSize[0], d.GridSize[1], d.GridSize[2],
						d.GroupSize[0], d.GroupSize[1], d.GroupSize[2])
				}
			}

		default:
			// Ignore others to reduce noise, or print if relevant
		}
	}
	return nil
}

func printRecordNode(rec trace.MTSPRecord, addrToName map[uint64]string, indent string) {
	switch rec.Type {
	case trace.RecordTypeCt:
		if ct, err := rec.ParseCtRecord(); err == nil {
			funcName := addrToName[ct.FunctionAddr]
			if funcName == "Unknown" {
				funcName = fmt.Sprintf("Func@0x%x", ct.FunctionAddr)
			}
			fmt.Printf("%s%s %s [Pipeline: %s]\n", indent, Colorize("▦", ColorBlue), Colorize(funcName, ColorGreen), Colorize(fmt.Sprintf("0x%x", ct.FunctionAddr), ColorCyan))
			if treeVerbose {
				for i, b := range ct.BufferBindings {
					bName := addrToName[b]
					if bName == "" {
						bName = fmt.Sprintf("0x%x", b)
					}
					fmt.Printf("%s  - Bind %d: %s\n", indent, i, bName)
				}
			}
		}
	case trace.RecordTypeCS:
		// Nested CS often marks kernels or markers
		fmt.Printf("%s%s  %s\n", indent, Colorize("🏷", ColorBlue), Colorize(rec.Label, ColorYellow))
	case trace.RecordTypeCi:
		fmt.Printf("%s%s Indirect Command (Size: %d)\n", indent, Colorize("⏭", ColorBlue), rec.Size)
		// Check for nested commands
		var t trace.Trace
		if nested, err := t.ParseNestedRecords(rec); err == nil && len(nested) > 0 {
			for _, child := range nested {
				printRecordNode(child, addrToName, indent+"  ")
			}
		}
	case trace.RecordTypeCul, trace.RecordTypeCulul:
		fmt.Printf("%s  • ICB Cmd (Cul) Addr: 0x%x\n", indent, rec.Address)
	case trace.RecordTypeCuw:
		fmt.Printf("%s  • ICB Write (Cuw) Addr: 0x%x\n", indent, rec.Address)
	default:
		// Ignore noisy small records in tree view unless relevant
	}
}

func renderPipelineTree(t *trace.Trace, records []trace.MTSPRecord, addrToName map[uint64]string) error {
	// Re-flatten for pipeline view, but respecting hierarchy for context if needed.
	// Actually, pipeline view is temporal, so flattening is fine if we just want sequential dispatches.
	// But we want to implement it robustly.

	var flattened []trace.MTSPRecord
	var flatten func([]trace.MTSPRecord)
	flatten = func(recs []trace.MTSPRecord) {
		for _, rec := range recs {
			// Flatten CS containers
			nested, err := t.ParseNestedRecords(rec)
			if err == nil && len(nested) > 0 {
				flatten(nested)
			} else {
				flattened = append(flattened, rec)
			}
		}
	}
	flatten(records)

	// Reuse existing pipeline grouping logic on flattened records
	// ... (We can adapt the existing logic here)

	type KernelNode struct {
		FunctionAddr   uint64
		CommandFlags   uint32
		BufferBindings []uint64
		Dispatches     int
	}
	type PipelineNode struct {
		Address uint64
		Kernels []*KernelNode
	}

	pipelineMap := make(map[uint64]*PipelineNode)
	var rootPipelines []*PipelineNode

	var currentPipeline *PipelineNode
	var currentKernel *KernelNode

	// Track Pipeline State: PipelineStateID -> FunctionID
	pipelineToFunc := make(map[uint64]uint64)

	// Pre-scan for Ctt records to ensure mapping is available before processing Ct records
	for _, rec := range flattened {
		if rec.Type == trace.RecordTypeCtt {
			if ctt, err := rec.ParseCttRecord(); err == nil {
				pipelineToFunc[ctt.PipelineAddr] = ctt.FunctionAddr
			}
		}
	}

	for _, rec := range flattened {
		if rec.Type == trace.RecordTypeCt {
			ct, err := rec.ParseCtRecord()
			if err != nil {
				continue
			}

			// Pipeline Change
			pNode, exists := pipelineMap[ct.PipelineAddr]
			if !exists {
				pNode = &PipelineNode{Address: ct.PipelineAddr}
				pipelineMap[ct.PipelineAddr] = pNode
				rootPipelines = append(rootPipelines, pNode)
			}
			currentPipeline = pNode

			// Kernel Change
			if len(currentPipeline.Kernels) > 0 {
				last := currentPipeline.Kernels[len(currentPipeline.Kernels)-1]
				if last.FunctionAddr == ct.FunctionAddr {
					currentKernel = last
					// Merge bindings... (simplified for brevity)
					continue
				}
			}

			// Resolve Real Function ID
			// We use pipelineToFunc to get the real Function ID
			// Ct.FunctionAddr holds the Pipeline State ID
			realFuncID := pipelineToFunc[ct.FunctionAddr]
			if realFuncID == 0 {
				realFuncID = ct.FunctionAddr
			}

			kNode := &KernelNode{
				FunctionAddr:   realFuncID,
				CommandFlags:   ct.CommandFlags,
				BufferBindings: ct.BufferBindings,
			}
			currentPipeline.Kernels = append(currentPipeline.Kernels, kNode)
			currentKernel = kNode
		}
		// Dispatch counting logic (ul@3)
		if bytesContains(rec.Data, []byte("ul@3")) && currentKernel != nil {
			currentKernel.Dispatches++
		}
	}

	fmt.Println(Colorize("GpuTrace Execution Tree (Grouped by Pipeline)", ColorBold))
	for _, p := range rootPipelines {
		fmt.Printf("%s %s\n", Colorize("▼ Compute Pipeline", ColorBlue), Colorize(fmt.Sprintf("0x%x", p.Address), ColorCyan))
		for _, k := range p.Kernels {
			name := addrToName[k.FunctionAddr]
			if name == "" {
				name = "Unknown"
			}
			fmt.Printf("  %s %s (%s)\n", Colorize("▼", ColorBlue), Colorize(name, ColorGreen), Colorize(fmt.Sprintf("0x%x", k.FunctionAddr), ColorCyan))
			if treeVerbose {
				for i, b := range k.BufferBindings {
					bName := addrToName[b]
					if bName == "" {
						fmt.Printf("    - Buffer %d: %s\n", i, Colorize(fmt.Sprintf("0x%x", b), ColorCyan))
					} else {
						fmt.Printf("    - Buffer %d: %s (%s)\n", i, Colorize(bName, ColorGreen), Colorize(fmt.Sprintf("0x%x", b), ColorCyan))
					}
				}
			}
			fmt.Printf("    %s Dispatches: %d\n", Colorize("▦", ColorBlue), k.Dispatches)
		}
	}
	return nil
}

func bytesContains(s, substr []byte) bool {
	return strings.Contains(string(s), string(substr))
}
