# Profiling Data Recreation from .gputrace Files

**Complete Guide to Extracting and Converting GPU Profiling Data**

This guide demonstrates how to recreate various profiling data formats (pprof, flamegraphs, timing analysis, performance metrics) from `.gputrace` files using the gputrace toolkit.

## Table of Contents

1. [Quick Start](#quick-start)
2. [Available Tools](#available-tools)
3. [Common Workflows](#common-workflows)
4. [Output Formats](#output-formats)
5. [Advanced Analysis](#advanced-analysis)
6. [Timing Extraction](#timing-extraction)
7. [Performance Comparison](#performance-comparison)
8. [Troubleshooting](#troubleshooting)

## Quick Start

### Complete Workflow (Capture → Analyze → Visualize)

```bash
# 1. Capture GPU trace
MTL_CAPTURE_ENABLED=1 go test -bench=BenchmarkYourCode -benchtime=3x

# 2. Find the generated trace
ls /tmp/*.gputrace

# 3. Quick stats
gputrace stats /tmp/your_trace.gputrace

# 4. Convert to pprof for analysis
gputrace gputrace2pprof /tmp/your_trace.gputrace -all -prefix analysis

# 5. View in browser
go tool pprof -http=:8080 analysis.gpu.pprof.gz
```

## Available Tools

The `gputrace` command provides a comprehensive suite of analysis tools:

### Core Commands

| Command | Purpose | Output |
|---------|---------|--------|
| `stats` | Overall trace statistics | Summary of kernels, encoders, buffers |
| `gputrace2pprof` | Convert to pprof format | `.pprof.gz` files for flamegraphs |
| `shaders` | Shader performance statistics | Xcode Instruments-style table |
| `shader-metrics` | Detailed shader metrics | Per-shader execution data |
| `timing` | Timing extraction | Comprehensive timing analysis |
| `perfcounters` | Hardware performance counters | Register usage, ALU utilization |
| `encoders` | List compute encoders | Encoder labels and execution order |
| `buffers` | Buffer analysis | Buffer sizes, bindings, memory usage |
| `buffers diff` | Compare buffer usage | Memory deltas between traces |
| `command-buffers` | Command buffer analysis | Command structure and hierarchy |
| `dump` | Dump all API calls | Complete API call sequence |
| `timing-profiler` | Extract .gpuprofiler_raw timing | Hardware-measured timing (if available) |

### Getting Help

```bash
# List all commands
gputrace --help

# Get help for specific command
gputrace stats --help
gputrace gputrace2pprof --help
```

## Common Workflows

### Workflow 1: Performance Profiling (Flamegraphs)

**Goal**: Identify which GPU kernels consume the most time

```bash
# 1. Capture trace
MTL_CAPTURE_ENABLED=1 go test -bench=BenchmarkForwardPass -benchtime=5x

# 2. Convert to pprof with all formats
gputrace gputrace2pprof /tmp/forward_pass_*.gputrace -all -prefix gpu_profile

# This creates:
#   gpu_profile.gpu.pprof.gz        - Hierarchical profile
#   gpu_profile.gpu-flat.pprof.gz   - Flat profile
#   gpu_profile.combined.pprof.gz   - Combined view
#   gpu_profile.txt                 - Text report

# 3. View top shaders
go tool pprof -top gpu_profile.gpu.pprof.gz

# 4. Interactive flamegraph
go tool pprof -http=:8080 gpu_profile.gpu.pprof.gz
# In browser: View > Flame Graph
```

**What you'll see**:
- Which shaders take the most GPU time
- Execution hierarchy (command queue → encoders → kernels)
- Percentage of total GPU time per shader

### Workflow 2: Xcode Instruments-Style Analysis

**Goal**: Recreate Xcode Instruments shader profiling table

```bash
# Show shader statistics similar to Xcode
gputrace shaders /tmp/trace.gputrace

# Example output:
# === Shader Performance Statistics ===
#
# Shader Name                                    Count  Avg SIMD  Total SIMD    Cost %
# ----------------------------------------------------------------
# affine_qmm_t_float16_...                          15      8,192     122,880     45.1%
# affine_qmv_fast_float16_...                       12      4,096      49,152     28.7%
# rope_single_freqs_float16                         8      1,024       8,192     14.2%

# More detailed metrics
gputrace shader-metrics /tmp/trace.gputrace
```

### Workflow 3: Memory Analysis

**Goal**: Understand buffer allocation and memory usage

```bash
# List all buffers with sizes
gputrace buffers /tmp/trace.gputrace

# Example output:
# === GPU Trace Buffers ===
#
# Total Buffers: 23
# Total Size: 3.44 MB
#
# ID    Filename               Size       Size (MB)  Aliases
# ------------------------------------------------------------
# 45    MTLBuffer-45-0        1.00 MB        1.00
# 44    MTLBuffer-44-0        1.00 MB        1.00
# 48    MTLBuffer-48-0      256.00 KB        0.25

# Show which encoders use each buffer
gputrace buffers /tmp/trace.gputrace --bindings

# Compare memory usage between two traces
gputrace buffers diff /tmp/before.gputrace /tmp/after.gputrace

# Example output:
# === Buffer Diff: before.gputrace vs after.gputrace ===
#
# Summary:
#   Added:      5 buffers (1.5 MB)
#   Removed:    2 buffers (512 KB)
#   Modified:   3 buffers
#   Net delta: +1.0 MB
```

### Workflow 4: Timing Extraction

**Goal**: Get accurate GPU execution timing

```bash
# Extract timing from multiple sources
gputrace timing /tmp/trace.gputrace

# For traces with .gpuprofiler_raw (hardware counters)
gputrace timing-profiler /tmp/profiled_trace.gputrace

# Show detailed encoder timing
gputrace encoders /tmp/trace.gputrace
```

**Timing Sources** (in order of accuracy):
1. **kdebug events** - Kernel debug trace (most accurate)
2. **Metal signposts** - AGX signpost events
3. **MTSP timestamps** - Command buffer timestamps
4. **.gpuprofiler_raw** - Hardware performance counters
5. **Synthetic estimation** - Heuristic fallback

See [INSTRUMENTS_TIMING_ANALYSIS.md](./INSTRUMENTS_TIMING_ANALYSIS.md) for timing methodology details.

### Workflow 5: Performance Counter Analysis

**Goal**: Extract hardware metrics (register usage, occupancy, ALU utilization)

```bash
# Check if trace has performance counters
gputrace perfcounters /tmp/trace.gputrace

# If trace was captured with Xcode Instruments profiling:
# Example output:
# === GPU Hardware Performance Counters ===
#
# Files Processed:  120
# Total Records:    2,458,734
# Dispatch Count:   1,043
#
# Note: Full metric extraction requires profiled trace
```

**How to capture profiled traces**:
1. Open trace in Xcode Instruments
2. Enable "Shader Profiler" instrument
3. Click "Profile" button
4. Export trace with `.gpuprofiler_raw` directory

### Workflow 6: Command Structure Analysis

**Goal**: Understand GPU command organization

```bash
# List command buffers
gputrace command-buffers /tmp/trace.gputrace

# Example output:
# === Command Buffers ===
#
# Total: 15 command buffers
#
# CB #1: offset=0x1000, size=2,048 bytes
#   Encoders: 3
#   Dispatches: 12
#
# CB #2: offset=0x2000, size=1,536 bytes
#   Encoders: 2
#   Dispatches: 8

# Dump all API calls in order
gputrace dump /tmp/trace.gputrace > api_calls.txt
```

## Output Formats

### Pprof Format

**Files**: `*.pprof.gz` (gzip-compressed protocol buffer)

**Use with**:
- `go tool pprof` - Command-line analysis
- `pprof -http=:8080` - Interactive web UI
- Any tool that supports pprof format

**Profile types**:
- **Hierarchical** (`*.gpu.pprof.gz`) - Shows command queue → encoder → kernel hierarchy
- **Flat** (`*.gpu-flat.pprof.gz`) - All kernels at same level, sorted by time
- **Combined** (`*.combined.pprof.gz`) - Multi-view profile with both hierarchies

**Sample types**:
- `gpu_time/nanoseconds` - GPU execution time
- `dispatches/count` - Number of kernel dispatches

### Text Reports

**Files**: `*.txt`

Human-readable summary with:
- Top shaders by GPU time
- Execution counts per shader
- SIMD group statistics
- Total GPU time breakdown

**Example**:
```
=== GPU Profile Summary ===

Total GPU Time: 51.19 ms
Total Kernels: 45
Total Encoders: 15

Top Shaders by GPU Time:
  1. affine_qmm_t_float16_t_gs_64_b_4_alN_true_batch_0
     Time: 23.08 ms (45.1%)
     Executions: 15
     SIMD Groups: 122,880

  2. affine_qmv_fast_float16_t_gs_64_b_4_batch_0
     Time: 14.67 ms (28.7%)
     Executions: 12
     SIMD Groups: 49,152
```

### JSON Format (Planned)

Machine-readable structured data for programmatic analysis.

### CSV Format

Available for buffer analysis:
```bash
gputrace buffers /tmp/trace.gputrace --format csv > buffers.csv
```

## Advanced Analysis

### Comparing Before/After Optimizations

```bash
# Capture baseline
MTL_CAPTURE_ENABLED=1 go test -bench=BenchmarkCode -benchtime=5x
mv /tmp/benchmark_*.gputrace /tmp/before.gputrace

# Make optimization changes
# ...

# Capture optimized version
MTL_CAPTURE_ENABLED=1 go test -bench=BenchmarkCode -benchtime=5x
mv /tmp/benchmark_*.gputrace /tmp/after.gputrace

# Convert both to pprof
gputrace gputrace2pprof /tmp/before.gputrace -o before.pprof.gz
gputrace gputrace2pprof /tmp/after.gputrace -o after.pprof.gz

# Compare with pprof
go tool pprof -base=before.pprof.gz after.pprof.gz

# Positive values = slower (regression)
# Negative values = faster (improvement)

# Compare memory usage
gputrace buffers diff /tmp/before.gputrace /tmp/after.gputrace
```

### Multi-Trace Analysis

```bash
# Analyze multiple traces at once
for trace in /tmp/*.gputrace; do
    echo "=== $(basename $trace) ==="
    gputrace stats $trace
    echo
done

# Batch convert to pprof
for trace in /tmp/*.gputrace; do
    name=$(basename $trace .gputrace)
    gputrace gputrace2pprof $trace -o "${name}.pprof.gz"
done
```

### Filtering and Sorting

```bash
# Buffers: sort by size, filter by minimum
gputrace buffers /tmp/trace.gputrace --sort size --min-size 1MB

# Buffers: different output formats
gputrace buffers /tmp/trace.gputrace --format json
gputrace buffers /tmp/trace.gputrace --format csv
gputrace buffers /tmp/trace.gputrace --format table  # default
```

### Programmatic Usage

You can also use the gputrace library directly in Go:

```go
package main

import (
    "fmt"
    "log"

    "github.com/tmc/mlx-go/experiments/gputrace"
)

func main() {
    // Open trace
    trace, err := gputrace.Open("benchmark.gputrace")
    if err != nil {
        log.Fatal(err)
    }
    defer trace.Close()

    // Extract kernel names
    fmt.Printf("Kernels: %d\n", len(trace.KernelNames))
    for i, kernel := range trace.KernelNames {
        fmt.Printf("  %d. %s\n", i+1, kernel)
    }

    // Extract timing
    extractor := gputrace.NewTimingExtractor(trace)
    timings, err := extractor.ExtractTimingV2()
    if err != nil {
        log.Fatal(err)
    }

    // Convert to pprof
    prof, err := trace.ToPprof(timings)
    if err != nil {
        log.Fatal(err)
    }

    // Write pprof file
    if err := gputrace.WritePprof("output.pprof.gz", prof); err != nil {
        log.Fatal(err)
    }
}
```

## Timing Extraction

### Understanding Timing Sources

`.gputrace` files **do NOT contain pre-computed timing data**. Timing must be extracted or estimated:

| Source | Accuracy | Availability | Notes |
|--------|----------|--------------|-------|
| **kdebug events** | ✅ Highest | Captured during execution | Kernel debug trace events |
| **Metal signposts** | ✅ High | Captured during execution | AGX signpost events |
| **MTSP timestamps** | ✅ Good | Always present | Command buffer timestamps |
| **.gpuprofiler_raw** | ✅ Excellent | Requires Instruments profiling | Hardware performance counters |
| **Replay with counters** | ✅ Perfect | Requires implementation | Like Xcode Instruments does |
| **Synthetic estimation** | ⚠️ Approximate | Always available | Heuristic fallback |

### How Xcode Instruments Gets Timing

Xcode Instruments does NOT read timing from `.gputrace` files. Instead:

1. **Replays the GPU workload** with performance counters enabled
2. **Measures execution time** during replay using `AGXGPURawCounterSource`
3. **Computes cost percentages** from measured GPU cycles

See [INSTRUMENTS_TIMING_ANALYSIS.md](./INSTRUMENTS_TIMING_ANALYSIS.md) for complete details.

### Extracting Available Timing

```bash
# Use multi-source timing extraction (best available)
gputrace timing /tmp/trace.gputrace

# Check timing quality
gputrace stats /tmp/trace.gputrace
# Look for "Timing Quality" section

# For profiled traces with .gpuprofiler_raw
gputrace timing-profiler /tmp/profiled_trace.gputrace
```

### Synthetic Timing Heuristics

When real timing is unavailable, estimates are based on kernel patterns:

| Kernel Pattern | Estimated Duration | Use Case |
|----------------|-------------------|----------|
| matmul, gemm, conv, attention | 5 ms | Large compute kernels |
| quantize, dequantize, affine | 2 ms | Data transformation |
| normalize, softmax, layer_norm | 2 ms | Normalization |
| rope, rotary, qkv | 3 ms | Attention components |
| add, mul, relu, sigmoid | 0.5 ms | Element-wise ops |
| default | 1 ms | Unknown kernels |

**Note**: Synthetic timing is for **visualization only**, not accurate performance measurement.

## Performance Comparison

### Comparing Go vs Swift/Python Implementations

```bash
# Capture Go trace
MTL_CAPTURE_ENABLED=1 go test -bench=BenchmarkForwardPass
gputrace gputrace2pprof /tmp/forward_pass_*.gputrace -all -prefix go

# Capture Swift/Python trace (using MLX-Swift or MLX)
# ... capture trace using framework's profiling ...
gputrace gputrace2pprof /tmp/swift_trace.gputrace -all -prefix swift

# Compare shader usage
diff <(go tool pprof -top go.gpu.pprof.gz) <(go tool pprof -top swift.gpu.pprof.gz)

# Look for:
# - Different shaders being used (qmm vs qmv)
# - Presence of separate dequantization
# - SIMD group count differences
# - Overall GPU time differences
```

### Key Metrics to Compare

1. **Shader Selection**
   - Are the right kernels being used? (e.g., `qmv` for single tokens, not `qmm`)
   - Is dequantization fused or separate?

2. **Execution Counts**
   - Number of dispatches per kernel
   - Number of encoders/command buffers

3. **SIMD Groups**
   - Total SIMD groups dispatched
   - Compare against reference implementation

4. **Memory Usage**
   - Total buffer allocation
   - Buffer sizes and counts

### Example Analysis

From DBD3 session comparing Go vs Swift:

**Go Implementation** (needs optimization):
```
51.19ms total GPU time
  23.08ms (45.1%) - affine_qmm_t_float16_...        ❌ Wrong kernel
  14.67ms (28.7%) - affine_qmv_fast_float16_...     ✓  Correct kernel
  10.55ms (20.6%) - affine_dequantize_float16_...   ❌ Separate dequantize

1,102,224 SIMD groups for affine_qmv_fast           ❌ 8x too many
```

**Swift Implementation** (reference):
```
2.87ms total GPU time (17.8x faster!)
   1.21ms (42.2%) - affine_qmv_fast_float16_...     ✓  Correct kernel
   0.43ms (15.0%) - rope_single_freqs_float16       ✓  Expected
   0.38ms (13.2%) - steel_attention_...             ✓  Expected

132,482 SIMD groups for affine_qmv_fast            ✓  Correct
No separate dequantization                          ✓  Fused
```

## Troubleshooting

### No .gputrace file generated?

```bash
# Ensure environment variable is set
export MTL_CAPTURE_ENABLED=1

# Check /tmp directory
ls -lh /tmp/*.gputrace

# Verify Metal operations are actually running
# (Profiling won't work for CPU-only code)
```

### Empty or missing timing data?

This is normal! `.gputrace` files don't contain pre-computed timing.

**Solutions**:
1. Use `gputrace timing` for best available timing extraction
2. Capture with kdebug events enabled for accurate timing
3. Use synthetic timing for visualization (automatic fallback)
4. Implement replay with `MTLCounterSampleBuffer` for Instruments-quality timing

### "Command not found: gputrace"

```bash
# Build and install
cd experiments/gputrace
go build -o ~/go/bin/gputrace ./cmd/gputrace

# Ensure ~/go/bin is in PATH
export PATH="$HOME/go/bin:$PATH"
```

### Pprof shows "no samples"

The trace might have no timing data. Use synthetic timing:

```bash
# This always works, uses estimated timing
gputrace gputrace2pprof /tmp/trace.gputrace -all -prefix analysis
```

### Cannot open .gputrace in Xcode

The trace format may be incompatible with older Xcode versions. This tool parses the binary format directly and doesn't require Xcode.

### Differences vs Xcode Instruments

**Expected differences**:
- **Timing values** - Instruments uses replay with counters (we use available timing sources)
- **Cost percentages** - May differ if timing sources differ
- **Counter data** - Requires `.gpuprofiler_raw` directory

**Should be identical**:
- Kernel names
- Execution order
- Buffer sizes
- Command structure
- Number of dispatches (from MTSP records)

## Quick Reference

### One-Line Commands

```bash
# Quick profile
gputrace gputrace2pprof trace.gputrace && go tool pprof -http=:8080 trace.pprof.gz

# Top 20 shaders
go tool pprof -top -nodecount=20 trace.pprof.gz

# Stats summary
gputrace stats trace.gputrace

# Memory usage
gputrace buffers trace.gputrace

# Compare traces
gputrace buffers diff before.gputrace after.gputrace

# Shader statistics
gputrace shaders trace.gputrace
```

### Batch Processing

```bash
# Process all traces in directory
for trace in *.gputrace; do
    gputrace gputrace2pprof "$trace" -all -prefix "$(basename $trace .gputrace)"
done

# Generate comparison report
for pprof in *.pprof.gz; do
    echo "=== $pprof ==="
    go tool pprof -top -nodecount=10 "$pprof"
done
```

## See Also

- [QUICK_REFERENCE.md](./QUICK_REFERENCE.md) - Quick reference for all tools
- [SHADER_PPROF_GUIDE.md](./SHADER_PPROF_GUIDE.md) - Detailed pprof guide
- [SHADER_SOURCE_MAPPING.md](./SHADER_SOURCE_MAPPING.md) - Link GPU kernels to source
- [INSTRUMENTS_TIMING_ANALYSIS.md](./INSTRUMENTS_TIMING_ANALYSIS.md) - How Instruments derives timing
- [GPU_PROFILING_APIS_DISCOVERED.md](../GPU_PROFILING_APIS_DISCOVERED.md) - GPU profiling API reference
- [README.md](../README.md) - Package documentation

## Contributing

Found a bug or have a suggestion? Please open an issue or submit a pull request.

Areas for improvement:
- Additional timing extraction methods
- More output formats (JSON, etc.)
- Enhanced visualization options
- Performance optimization
- Documentation improvements

## License

Part of the mlx-go project. See main repository for license information.
