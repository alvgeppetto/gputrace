# gputrace Architecture

**Last Updated:** 2025-11-03
**Version:** 1.0

## Overview

`gputrace` is a Go toolkit for parsing, analyzing, and visualizing Metal GPU trace files (`.gputrace` bundles). It provides both a library API and a comprehensive CLI for working with GPU profiling data captured by Xcode Instruments.

**Design Philosophy:**
- Follow Go best practices (Russ Cox style: simple, clear, composable)
- Parse once, query many times
- Graceful degradation when data is unavailable
- Zero external dependencies for core parsing
- CLI mirrors Xcode Instruments functionality

## Project Structure

```
gputrace/
├── *.go                    # Library code (parsing, analysis, formatting)
├── cmd/gputrace/
│   ├── main.go            # CLI entry point
│   └── cmd/               # Cobra command implementations
│       ├── root.go        # Root command + shared utilities
│       ├── stats.go       # Basic statistics
│       ├── shaders.go     # Shader performance metrics
│       ├── timeline.go    # Timeline visualization
│       └── ...            # Other commands
├── docs/                  # Documentation
│   ├── ARCHITECTURE.md    # This file
│   ├── TRACE_FORMAT.md    # Binary format specifications
│   └── *.md              # User guides
└── *.md                  # Top-level documentation
```

## Core Concepts

### 1. Trace Structure

A `.gputrace` file is a directory bundle containing:

```
trace.gputrace/
├── index                          # File index (offsets, sizes)
├── store0                         # Primary data store (timing, API calls)
├── kdebug.plist                   # Kernel debug events
├── Store/                         # Additional data stores
│   ├── CommandBufferStore/        # Command buffer records
│   ├── CSRecordsStore/           # Compute shader records
│   └── MTSPRecordsStore/         # Metal Shader Performance records
└── *.gpuprofiler_raw/            # Performance counter data (optional)
    └── Counters_f_*.raw          # Hardware metrics
```

### 2. Data Layers

**Layer 1: Raw Parsing**
- Binary file reading (index_parser.go, store_parser.go)
- Record boundary detection
- Offset calculation
- Low-level data extraction

**Layer 2: Structured Data**
- API call lists (api_call_list.go)
- Command buffers (command_buffer.go)
- Timing data (timing_v2.go, enhanced_timing.go)
- Hardware counters (perfcounters.go)

**Layer 3: Analysis**
- Shader metrics (shader_metrics.go, shader_costs.go)
- Correlation (shader_correlation.go)
- Statistics (statistics.go)
- Insights (insights.go)

**Layer 4: Output**
- Formatting (shader_metrics.go FormatShadersXcodeStyle, etc.)
- Export (timeline.go exportChromeTracing, gputrace2pprof.go)
- Visualization (timeline.go)

### 3. Key Data Structures

**Trace** (`gputrace.go`)
```go
type Trace struct {
    Path               string
    Index              *Index
    Metadata           *TraceMetadata
    EncoderLabels      []string
    KernelNames        []string
    BufferLabels       []string
    CommandQueueLabel  string
    // ... internal fields
}
```

**CommandBuffer** (`command_buffer.go`)
```go
type CommandBuffer struct {
    Index      int
    Offset     int64
    Encoders   []EncoderInfo
    Dispatches []DispatchInfo
}
```

**ShaderMetricsReport** (`shader_metrics.go`)
```go
type ShaderMetricsReport struct {
    Shaders       []ShaderMetrics
    TotalDuration time.Duration
    TotalCost     float64
}
```

**Timeline** (`cmd/timeline.go`)
```go
type Timeline struct {
    StartTime    uint64
    EndTime      uint64
    Duration     uint64
    Events       []TimelineEvent
    Encoders     []EncoderInfo
    Kernels      []KernelInfo
}
```

## Library Organization

### Parsing (Data Input)

| File | Purpose | Key Functions |
|------|---------|---------------|
| `index_parser.go` | Parse `.gputrace/index` file | `ParseIndex()` |
| `store_parser.go` | Parse store files | `ParseStore()` |
| `api_call_list.go` | Extract API call sequences | `ParseAPICallList()` |
| `command_buffer.go` | Parse command buffer records | `ParseCommandBuffers()` |
| `cs_records.go` | Parse compute shader records | `ParseCSRecords()` |
| `mtsp_records.go` | Parse Metal shader performance records | `ParseMTSPRecords()` |
| `perfcounters.go` | Parse hardware performance counters | `ParsePerfCounters()` |
| `kdebug.go` | Parse kernel debug events | `ParseKDebugEvents()` |

### Analysis (Data Processing)

| File | Purpose | Key Functions |
|------|---------|---------------|
| `shader_metrics.go` | Compute shader performance metrics | `ExtractShaderMetrics()` |
| `shader_costs.go` | Calculate shader execution costs | `CalculateShaderCosts()` |
| `shader_correlation.go` | Correlate shader names with addresses | `CorrelateShaderNames()` |
| `timing_v2.go` | Extract timing information (v2 algorithm) | `ExtractTimingDataV2()` |
| `enhanced_timing.go` | Enhanced timing with kdebug integration | `ExtractEnhancedTiming()` |
| `timing_metrics.go` | Aggregate timing statistics | `NewTimingMetricsExtractor()` |
| `statistics.go` | High-level statistics | `ExtractStatistics()` |
| `insights.go` | Performance insights and recommendations | `GenerateInsights()` |
| `dispatch_count.go` | Count shader dispatches | `CountDispatches()` |
| `command_buffer_count.go` | Count command buffers and encoders | `CountCommandBuffers()` |

### Output (Data Formatting)

| File | Purpose | Key Functions |
|------|---------|---------------|
| `shader_metrics.go` | Format shader metrics (Xcode style) | `FormatShadersXcodeStyle()` |
| `pprof.go` | Export to pprof format | `ConvertToPprof()` |
| `pprof_v2.go` | Enhanced pprof export | `ConvertToPprofV2()` |
| `pprof_enhanced.go` | Advanced pprof with source mapping | `ConvertToPprofEnhanced()` |

### Utilities (Supporting Code)

| File | Purpose | Key Functions |
|------|---------|---------------|
| `gputrace.go` | Main API entry point | `Open()`, `Close()` |
| `validation.go` | Trace validation | `ValidateTrace()` |
| `device_resources.go` | Device resource tracking | `TrackDeviceResources()` |
| `buffer_access.go` | Buffer access analysis | `AnalyzeBufferAccess()` |
| `buffer_diff.go` | Buffer content diffing | `DiffBuffers()` |
| `signpost.go` | Signpost event parsing | `ParseSignposts()` |
| `synthetic_timing.go` | Generate synthetic timing estimates | `GenerateSyntheticTiming()` |

## CLI Command Structure

### Command Organization

All CLI commands follow a consistent pattern:

```go
// 1. Package declaration
package cmd

// 2. Imports
import (
    "fmt"
    "github.com/spf13/cobra"
    "github.com/tmc/mlx-go/experiments/gputrace"
)

// 3. Command-specific flags (if needed)
var cmdFlags struct {
    verbose bool
    output  string
}

// 4. Command definition
var cmdName = &cobra.Command{
    Use:   "name <trace.gputrace>",
    Short: "Brief description",
    Long:  `Detailed description with examples`,
    Args:  cobra.ExactArgs(1),
    RunE:  runCmdName,
}

// 5. Initialization
func init() {
    rootCmd.AddCommand(cmdName)
    cmdName.Flags().BoolVarP(&cmdFlags.verbose, "verbose", "v", false, "Description")
}

// 6. Run function
func runCmdName(cmd *cobra.Command, args []string) error {
    // a. Verify trace file
    if err := checkTraceFile(args[0]); err != nil {
        return err
    }

    // b. Open trace
    trace, err := gputrace.Open(args[0])
    if err != nil {
        return fmt.Errorf("failed to open trace: %w", err)
    }
    defer trace.Close()

    // c. Extract/process data
    data, err := trace.ExtractData()
    if err != nil {
        return fmt.Errorf("failed to extract data: %w", err)
    }

    // d. Format/output results
    fmt.Println(formatOutput(data))

    return nil
}
```

### Command Categories

**Basic Information:**
- `stats` - Overall trace statistics
- `dump` - Raw trace dump

**Shader Analysis:**
- `shaders` - Shader performance metrics (Xcode Instruments format)
- `shader-metrics` - Alternative shader analysis
- `encoders` - Encoder listing

**Timing Analysis:**
- `timing` - Timing data extraction
- `timing-profiler` - Advanced timing profiling

**Visualization:**
- `timeline` - Generate Chrome Tracing format visualization

**Buffer Analysis:**
- `buffers` - Buffer information
- `buffer-access` - Buffer access patterns
- `buffers-diff` - Compare buffer contents

**Advanced:**
- `command-buffers` - Command buffer details
- `correlate` - Correlate shader names with addresses
- `insights` - Performance insights and recommendations
- `perfcounters` - Hardware performance counters

**Export:**
- `gputrace2pprof` - Export to pprof format

### Shared Utilities (root.go)

**checkTraceFile(tracePath string) error**
- Validates trace file existence and format
- Used by all commands that accept trace path arguments

## Design Patterns

### 1. Graceful Degradation

Many trace files lack complete data (timing, performance counters, labels). The library handles this gracefully:

```go
// Try to get real data, fall back to estimates
var allocatedRegs int
var hasRealData bool

if trace != nil {
    allocatedRegs, _, _, hasRealData = trace.GetRegisterDataForShader(addr)
}

if hasRealData {
    fmt.Printf("%d", allocatedRegs)
} else {
    // Fall back to estimation with clear marker
    fmt.Printf("%d (est)", estimateRegisters())
}
```

### 2. Lazy Parsing

Data is parsed on-demand rather than all at once:

```go
// Open is fast - doesn't parse everything
trace := gputrace.Open("trace.gputrace")

// Parsing happens when data is requested
commandBuffers := trace.ParseCommandBuffers()  // Parse only when needed
timing := trace.ExtractTimingData()             // Parse only when needed
```

### 3. Error Wrapping

Errors are wrapped with context using `fmt.Errorf`:

```go
data, err := parseData()
if err != nil {
    return fmt.Errorf("parse data: %w", err)
}
```

This creates error chains like:
```
failed to extract shader metrics: parse command buffers: read store file: file not found
```

### 4. Consistent Naming

**Parsing functions:** `Parse*()` - Read and structure binary data
```go
func ParseCommandBuffers() ([]CommandBuffer, error)
func ParseAPICallList() (*APICallList, error)
```

**Extraction functions:** `Extract*()` - Derive higher-level information
```go
func ExtractStatistics() (*Statistics, error)
func ExtractShaderMetrics() (*ShaderMetricsReport, error)
```

**Formatting functions:** `Format*()` - Output formatted data
```go
func FormatShadersXcodeStyle(w io.Writer, report *ShaderMetricsReport) error
func FormatStatistics() string
```

### 5. Struct Embedding

Common data is embedded for reuse:

```go
type EncoderInfo struct {
    Index     int
    Label     string
    Address   uint64
    StartTime uint64
    EndTime   uint64
}

type DetailedEncoderInfo struct {
    EncoderInfo  // Embedded
    Dispatches []DispatchInfo
    Buffers    []BufferBinding
}
```

## Binary Format Handling

### Record-Based Parsing

Metal trace files use record-based binary formats. General pattern:

```go
// 1. Find record boundaries (marker bytes)
offsets := findRecordMarkers(data, markerPattern)

// 2. Parse each record
for _, offset := range offsets {
    recordSize := binary.LittleEndian.Uint32(data[offset:])
    recordData := data[offset:offset+recordSize]

    // 3. Extract fields
    field1 := binary.LittleEndian.Uint64(recordData[0x08:])
    field2 := binary.LittleEndian.Uint32(recordData[0x10:])
}
```

### Performance Counter Format

`.gpuprofiler_raw` files contain hardware metrics in Apple Performance Streaming (APS) format:

```go
// Record structure (discovered via reverse engineering)
// Offset  Size  Field
// 0x00    4     Marker (0x4E 0x00 0x00 0x00)
// 0x04    4     Record type
// 0x08    8     Pipeline state address
// 0x10+   ?     Metric fields (offsets vary by record type)
```

See `GPU_PROFILING_APIS_DISCOVERED.md` for complete reverse engineering documentation.

## Data Flow

### Typical Analysis Workflow

```
1. User runs CLI command
   │
   ├─→ checkTraceFile() validates path
   │
   ├─→ gputrace.Open() creates Trace object
   │   │
   │   ├─→ ParseIndex() reads file offsets
   │   └─→ Quick metadata scan
   │
   ├─→ Command-specific extraction (lazy)
   │   │
   │   ├─→ ParseCommandBuffers() for shader analysis
   │   ├─→ ExtractTimingData() for timing
   │   ├─→ ParsePerfCounters() for hardware metrics
   │   └─→ Correlation/analysis functions
   │
   ├─→ Formatting layer
   │   │
   │   ├─→ FormatShadersXcodeStyle() for Xcode output
   │   ├─→ exportChromeTracing() for timeline
   │   └─→ ConvertToPprof() for pprof
   │
   └─→ Output to stdout or file
```

### Example: Shader Analysis Flow

```
shaders command
    │
    ├─→ checkTraceFile("trace.gputrace")
    │
    ├─→ trace = gputrace.Open("trace.gputrace")
    │   │
    │   ├─→ ParseIndex()
    │   └─→ Load metadata
    │
    ├─→ report = trace.ExtractShaderMetrics()
    │   │
    │   ├─→ ParseCommandBuffers()
    │   │   │
    │   │   ├─→ Read CommandBufferStore
    │   │   ├─→ Parse encoder records
    │   │   └─→ Extract dispatch info
    │   │
    │   ├─→ ExtractTimingDataV2()
    │   │   │
    │   │   ├─→ Read store0
    │   │   ├─→ Parse timing records
    │   │   └─→ Correlate with encoders
    │   │
    │   ├─→ CorrelateShaderNames()
    │   │   │
    │   │   └─→ Match addresses to labels
    │   │
    │   └─→ CalculateShaderCosts()
    │       │
    │       └─→ Compute % of total GPU time
    │
    ├─→ if trace.HasPerfCounters():
    │   │
    │   └─→ ParsePerfCounters()
    │       │
    │       ├─→ Read .gpuprofiler_raw files
    │       ├─→ Extract register data
    │       └─→ Correlate with shaders
    │
    └─→ FormatShadersXcodeStyle(report, trace)
        │
        ├─→ Use real register data if available
        ├─→ Fall back to estimates
        └─→ Output Xcode Instruments format
```

## Testing Strategy

### Current State
- Manual testing with real trace files
- Example traces in `/tmp/*.gputrace`
- Verification against Xcode Instruments output

### Recommended Additions
1. **Unit tests** for parsing functions with synthetic data
2. **Golden file tests** with known trace files
3. **Regression tests** for format changes
4. **Integration tests** for full workflows

## Performance Considerations

### Memory Usage
- **Index parsing**: O(n) where n = number of files in bundle
- **Command buffer parsing**: O(m) where m = number of command buffers
- **Timeline generation**: O(e + k) where e = encoders, k = kernels
- **Performance counter parsing**: ~100-150 bytes per record

### Optimization Opportunities
1. **Streaming parsers** for large traces (currently load full files)
2. **Caching** for repeated queries on same trace
3. **Parallel parsing** of independent stores
4. **Memory-mapped I/O** for very large files

### Current Bottlenecks
- Loading entire store files into memory
- Repeated file reads for correlation
- JSON encoding for timeline export (large traces)

## Extension Points

### Adding New Commands

1. Create `cmd/gputrace/cmd/newcmd.go`:
```go
package cmd

var newCmd = &cobra.Command{
    Use:   "new <trace.gputrace>",
    Short: "Description",
    RunE:  runNew,
}

func init() {
    rootCmd.AddCommand(newCmd)
}

func runNew(cmd *cobra.Command, args []string) error {
    // Implementation
}
```

2. Add to `main.go` imports (automatic via init())

### Adding New Parsers

1. Create `newparser.go` in root:
```go
package gputrace

func (t *Trace) ParseNewData() (*NewData, error) {
    // Implementation
}
```

2. Add data structure definition
3. Document binary format in TRACE_FORMAT.md

### Adding New Export Formats

Add export function to relevant command:
```go
func exportNewFormat(data *Data, output string) error {
    // Implementation
}
```

## Known Issues and Limitations

### Current Limitations

1. **Performance Counter Field Extraction** (see PERFCOUNTERS_STATUS.md)
   - Infrastructure complete
   - Field offsets need profiled trace analysis
   - Currently falls back to estimates

2. **Timing Data Accuracy**
   - Synthetic timing when store0 lacks real data
   - Estimates based on record ordering

3. **Architecture Specificity**
   - Binary formats may vary by GPU (M1/M2/M3/M4)
   - Current implementation tested primarily on M1/M2

4. **Memory Footprint**
   - Large traces (>1GB) can consume significant memory
   - Full file loading rather than streaming

### Future Enhancements

1. **Real-time trace streaming** during capture
2. **Multi-trace comparison** tools
3. **Machine learning** for performance prediction
4. **Web UI** for interactive analysis
5. **GPU-specific optimizations** per architecture

## Dependencies

### Core Library
- **Standard library only** - No external dependencies
- Uses `encoding/binary` for binary parsing
- Uses `encoding/json` for export formats

### CLI
- `github.com/spf13/cobra` - Command-line framework
- Standard library for everything else

### Rationale
Minimal dependencies ensure:
- Long-term maintainability
- Easy embedding in other projects
- Reduced attack surface
- Faster compilation

## Code Style Guidelines

Following Russ Cox / Go best practices:

### General Principles
1. **Clarity over cleverness** - Simple code is better than clever code
2. **Errors are values** - Handle errors explicitly, never ignore
3. **Composition over inheritance** - Use embedding and interfaces
4. **Small interfaces** - Define minimal interface contracts

### Specific Rules

**Naming:**
```go
// Good
func ParseCommandBuffers() ([]CommandBuffer, error)
var encoderCount int

// Bad
func parse_command_buffers() ([]CommandBuffer, error)  // No underscores
var encoder_cnt int                                     // No abbreviations
```

**Error Handling:**
```go
// Good
if err != nil {
    return fmt.Errorf("parse data: %w", err)
}

// Bad
if err != nil {
    panic(err)  // Never panic in library code
}
```

**Comments:**
```go
// Good - complete sentence
// ParseCommandBuffers parses all command buffer records from the trace.

// Bad - fragment
// parse command buffers
```

**Function Length:**
- Prefer functions under 50 lines
- Extract helpers for clarity
- Maximum ~100 lines (with clear structure)

**Package Organization:**
- Library code in root package `gputrace`
- CLI code in `cmd/gputrace/cmd`
- No `internal/` package unless truly internal-only

## Documentation Standards

### Code Documentation
- All exported functions have doc comments
- Doc comments start with function name
- Include usage examples for complex APIs

### User Documentation
- `docs/` directory for guides
- Each major feature has a guide (e.g., TIMELINE_VISUALIZATION_GUIDE.md)
- ARCHITECTURE.md (this file) for developers

### Binary Format Documentation
- TRACE_FORMAT.md for file formats
- GPU_PROFILING_APIS_DISCOVERED.md for reverse engineering notes
- Inline comments in parsers reference offsets

## Versioning and Compatibility

### Current Version
- **Version:** 1.0 (implicit)
- **Stability:** Beta - APIs may change

### Compatibility Goals
1. **Backward compatibility** for trace file formats
2. **Deprecation warnings** before API changes
3. **Migration guides** for breaking changes

### Future Plans
- Semantic versioning (v1.0.0, v1.1.0, etc.)
- Stable API guarantee for v1.x
- Separate CLI and library versioning

## Security Considerations

### Input Validation
- All file paths validated before opening
- File size limits to prevent DoS
- Bounds checking for all binary parsing

### Sandboxing
- No arbitrary code execution
- No network access
- Only reads specified trace files

### Vulnerabilities
- **Buffer overruns** - Mitigated by bounds checking
- **Path traversal** - Mitigated by path validation
- **Malformed traces** - Graceful error handling

## Contributing Guidelines

### Before Contributing
1. Read this ARCHITECTURE.md
2. Review existing code for style
3. Check for related issues/PRs

### Code Contributions
1. Write tests for new features
2. Update documentation
3. Follow Go style guidelines
4. Run `go fmt` and `go vet`

### Documentation Contributions
1. Use Markdown for all docs
2. Include code examples
3. Test examples before submitting

## References

### Internal Documentation
- `TRACE_FORMAT.md` - Binary format specifications
- `GPU_PROFILING_APIS_DISCOVERED.md` - Reverse engineering notes
- `PROFILING_DATA_RECREATION_GUIDE.md` - User workflows
- `TIMELINE_VISUALIZATION_GUIDE.md` - Timeline feature guide
- `PERFCOUNTERS_STATUS.md` - Performance counter status

### External References
- [Metal Programming Guide](https://developer.apple.com/documentation/metal)
- [Instruments User Guide](https://help.apple.com/instruments/)
- [Chrome Tracing Format](https://docs.google.com/document/d/1CvAClvFfyA5R-PhYUmn5OOQtYMH4h6I0nSsKchNAySU/)
- [pprof Format](https://github.com/google/pprof/blob/main/proto/profile.proto)

### Apple Frameworks
- `/System/Library/Extensions/AGXMetalA*.bundle/` - GPU counter implementation
- `/System/Library/PrivateFrameworks/GPUToolsReplay.framework/` - Replay infrastructure
- `IOKit.framework` - IOReport public API

## Changelog

### 2025-11-03
- Initial ARCHITECTURE.md creation
- Documented current state (17 commands, ~18K LOC)
- Analyzed library organization and patterns
- Created comprehensive design documentation
