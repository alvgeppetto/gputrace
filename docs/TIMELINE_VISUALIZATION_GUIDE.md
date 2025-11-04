# GPU Timeline Visualization Guide

**Command:** `gputrace timeline`
**Purpose:** Generate interactive timeline visualizations of GPU execution
**Output:** Chrome Tracing format compatible with chrome://tracing and Perfetto

## Overview

The timeline command creates interactive visualizations showing:
- **Encoder execution sequence** - Chronological order of compute encoder operations
- **Kernel launches** - Individual shader/kernel executions with timing
- **Command buffer lifecycle** - Creation, encoding, and commit events
- **Concurrent execution** - Visual representation of GPU parallelism
- **Duration analysis** - Total execution time and per-encoder breakdown

## Quick Start

### Generate Timeline

```bash
# Basic usage
gputrace timeline trace.gputrace -o timeline.json

# Specify output location
gputrace timeline trace.gputrace -o ~/Desktop/gpu_timeline.json
```

### View in Chrome

1. Open Chrome browser
2. Navigate to `chrome://tracing`
3. Click the **Load** button
4. Select your `timeline.json` file
5. Use navigation controls:
   - **W/S** - Zoom in/out
   - **A/D** - Pan left/right
   - **Mouse wheel** - Zoom at cursor position
   - **Click and drag** - Pan timeline
   - **Click event** - View event details

### View in Perfetto

Alternative viewer with more features:

1. Open https://ui.perfetto.dev
2. Click **Open trace file**
3. Select your `timeline.json` file
4. Explore with advanced query capabilities

## Output Format

### Chrome Tracing Format

```json
{
  "displayTimeUnit": "ns",
  "metadata": {
    "duration": 26500000,
    "encoder_count": 31,
    "kernel_count": 31,
    "start_time": 0,
    "end_time": 26500000
  },
  "traceEvents": [
    {
      "name": "block_softmax_float32",
      "category": "encoder",
      "phase": "X",
      "timestamp": 0,
      "duration": 1000000,
      "pid": 1,
      "tid": 1,
      "args": {
        "duration_ms": 1.0,
        "index": 0
      }
    }
  ]
}
```

### Event Types

**Process/Thread Structure:**
- **PID 1**: GPU Encoders (main execution timeline)
- **TID 1**: Encoder execution thread
- **TID 2**: Kernel execution thread (future)

**Event Categories:**
- `encoder` - Compute encoder execution blocks
- `kernel` - Individual shader/kernel launches
- `command_buffer` - Command buffer lifecycle events

**Event Phases:**
- `X` - Complete event (has duration)
- `B` - Begin event
- `E` - End event
- `i` - Instant event

## Use Cases

### 1. Execution Order Analysis

**Purpose:** Understand the sequence of GPU operations

**Workflow:**
```bash
gputrace timeline trace.gputrace -o timeline.json
# Open in chrome://tracing
# Zoom out to see full execution sequence
# Identify encoder ordering and dependencies
```

**What to Look For:**
- Sequential vs parallel execution patterns
- Encoder submission order
- Command buffer batching behavior
- GPU idle time between operations

### 2. Performance Bottleneck Identification

**Purpose:** Find slow encoders and kernels

**Workflow:**
```bash
gputrace timeline trace.gputrace -o timeline.json
# Open in chrome://tracing
# Look for long-duration events (wider bars)
# Click events to see exact durations
# Compare relative execution times
```

**What to Look For:**
- Unusually long encoder durations
- Outlier kernel execution times
- Repeated slow operations
- Imbalanced workload distribution

### 3. Concurrency Analysis

**Purpose:** Verify GPU utilization and parallelism

**Workflow:**
```bash
gputrace timeline trace.gputrace -o timeline.json
# Open in chrome://tracing
# Zoom to see vertical alignment
# Check for gaps between operations
# Verify overlapping execution
```

**What to Look For:**
- Gaps between encoders (potential optimization)
- Overlapping execution (good parallelism)
- Single-threaded execution patterns (bad)
- Resource contention indicators

### 4. Before/After Optimization Comparison

**Purpose:** Validate optimization effectiveness

**Workflow:**
```bash
# Capture baseline
gputrace timeline baseline.gputrace -o baseline_timeline.json

# Make optimization changes
# ...

# Capture optimized version
gputrace timeline optimized.gputrace -o optimized_timeline.json

# Open both in separate chrome://tracing tabs
# Compare:
#   - Total execution time (metadata.duration)
#   - Number of encoders (metadata.encoder_count)
#   - Individual encoder durations
#   - Execution pattern changes
```

**Metrics to Compare:**
- Total duration reduction
- Encoder count changes (fewer = better batching)
- Kernel execution time improvements
- Reduced gaps/idle time

## Example Output Analysis

### Test Trace Analysis

```bash
$ gputrace timeline /tmp/fast-llm-mlx-final.gputrace -o timeline.json
✓ Timeline written to: timeline.json

Metadata:
  Duration: 26.5 ms
  Encoders: 31
  Kernels: 31
```

### Timeline Visualization Interpretation

Opening `timeline.json` in chrome://tracing shows:

**Process 1: GPU Encoders**
```
┌─────────────────────────────────────────────────────┐
│ TID 1: Encoder Timeline                              │
├─────────────────────────────────────────────────────┤
│ [block_softmax_float32]  1ms                         │
│   [UUID-encoder]         1ms                         │
│     [vs_Multiply]        0.5ms                       │
│       [vv_Add]           0.5ms                       │
│         ...              ...                         │
└─────────────────────────────────────────────────────┘
         0ms    5ms    10ms   15ms   20ms   25ms
```

**Key Observations:**
1. Sequential execution (no overlapping bars)
2. Uniform encoder durations (~1ms each)
3. Total execution time: 26.5ms
4. 31 distinct encoder operations

## Integration with Other Commands

### Timeline + Stats

```bash
# Get high-level statistics
gputrace stats trace.gputrace

# Generate detailed timeline
gputrace timeline trace.gputrace -o timeline.json

# Compare:
#   - stats: Quick overview (command buffers, encoders, dispatches)
#   - timeline: Detailed execution visualization
```

### Timeline + Shaders

```bash
# Identify expensive shaders
gputrace shaders trace.gputrace > shaders.txt

# Visualize their execution
gputrace timeline trace.gputrace -o timeline.json

# In chrome://tracing:
#   - Find expensive shader names from shaders.txt
#   - Locate them in timeline
#   - Analyze their execution patterns
```

### Timeline + Timing

```bash
# Extract precise timing data
gputrace timing trace.gputrace --format=csv > timing.csv

# Visualize execution order
gputrace timeline trace.gputrace -o timeline.json

# Cross-reference:
#   - timing.csv: Exact duration numbers
#   - timeline.json: Visual representation
```

## Advanced Usage

### Custom JSON Processing

```bash
# Generate timeline
gputrace timeline trace.gputrace -o timeline.json

# Extract metadata
jq '.metadata' timeline.json

# Count events
jq '.traceEvents | length' timeline.json

# Find longest events
jq '.traceEvents | sort_by(.duration) | reverse | .[0:5]' timeline.json

# Filter by category
jq '.traceEvents | map(select(.category == "encoder"))' timeline.json
```

### Scripted Analysis

```bash
#!/bin/bash
# analyze_timeline.sh - Automated timeline analysis

TRACE=$1
OUTPUT="${TRACE%.gputrace}_timeline.json"

# Generate timeline
gputrace timeline "$TRACE" -o "$OUTPUT"

# Extract key metrics
DURATION=$(jq '.metadata.duration' "$OUTPUT")
ENCODERS=$(jq '.metadata.encoder_count' "$OUTPUT")
KERNELS=$(jq '.metadata.kernel_count' "$OUTPUT")

# Report
echo "Timeline Analysis: $TRACE"
echo "  Duration: $(echo "scale=2; $DURATION / 1000000" | bc) ms"
echo "  Encoders: $ENCODERS"
echo "  Kernels: $KERNELS"
echo "  Avg per encoder: $(echo "scale=2; $DURATION / $ENCODERS / 1000" | bc) µs"
```

### Batch Processing

```bash
# Analyze multiple traces
for trace in traces/*.gputrace; do
  basename=$(basename "$trace" .gputrace)
  gputrace timeline "$trace" -o "timelines/${basename}_timeline.json"
  echo "Processed: $basename"
done

# Compare all timelines
ls -lh timelines/*.json
```

## Troubleshooting

### Issue: Timeline is Empty

**Symptoms:** JSON file contains no traceEvents

**Causes:**
- Trace file has no encoder data
- Trace is from incompatible Metal version
- Trace file is corrupted

**Solutions:**
```bash
# Verify trace has encoder data
gputrace stats trace.gputrace | grep "Encoders"

# Check trace format
file trace.gputrace

# Try regenerating trace with MTL_CAPTURE_ENABLED=1
```

### Issue: Events Have Zero Duration

**Symptoms:** All events show 0ms or very small durations

**Causes:**
- Timing data not available in trace
- Using synthetic timing estimates
- Trace captured without proper timing

**Solutions:**
```bash
# Check timing sources
gputrace timing trace.gputrace --format=text | head

# Verify trace has timing data
gputrace stats trace.gputrace | grep -i time
```

### Issue: Chrome Tracing Won't Load File

**Symptoms:** Error loading timeline.json in chrome://tracing

**Causes:**
- Invalid JSON format
- File too large for Chrome
- Unsupported tracing format version

**Solutions:**
```bash
# Validate JSON
jq empty timeline.json

# Check file size
ls -lh timeline.json

# Reduce trace size by filtering
jq '.traceEvents | .[0:1000]' timeline.json > timeline_small.json
```

### Issue: Missing Shader Names

**Symptoms:** Events show UUIDs instead of shader names

**Causes:**
- Encoder labels not present in trace
- Trace captured without debug info
- Label correlation failed

**Solutions:**
```bash
# Verify labels exist
gputrace encoders trace.gputrace

# Re-capture trace with debug symbols enabled
MTL_SHADER_VALIDATION=1 MTL_CAPTURE_ENABLED=1 ./your_app
```

## Performance Considerations

### Large Traces

For traces with many operations (>10,000 events):

```bash
# Generate full timeline
gputrace timeline large_trace.gputrace -o full_timeline.json

# Chrome may be slow - use Perfetto instead
# Open https://ui.perfetto.dev
# Perfetto handles large traces better
```

### Memory Usage

Timeline generation memory usage scales with:
- Number of encoders × 100 bytes
- Number of kernels × 150 bytes
- Metadata overhead: ~1KB

Approximate memory needed:
```
Memory ≈ (encoders × 100 + kernels × 150) bytes
```

For 1000 encoders: ~250KB memory

## Output Format Reference

### Metadata Fields

| Field | Type | Description |
|-------|------|-------------|
| `duration` | int64 | Total execution time (nanoseconds) |
| `encoder_count` | int | Number of compute encoders |
| `kernel_count` | int | Number of kernel dispatches |
| `start_time` | int64 | Timeline start timestamp (ns) |
| `end_time` | int64 | Timeline end timestamp (ns) |

### Event Fields

| Field | Type | Description |
|-------|------|-------------|
| `name` | string | Encoder/kernel name or UUID |
| `category` | string | Event category (encoder/kernel/etc) |
| `phase` | string | Event phase (X for complete) |
| `timestamp` | int64 | Event start time (ns) |
| `duration` | int64 | Event duration (ns) |
| `pid` | int | Process ID (always 1 for GPU) |
| `tid` | int | Thread ID (1 for encoders) |
| `args` | object | Additional event metadata |

### Args Object

| Field | Type | Description |
|-------|------|-------------|
| `duration_ms` | float64 | Duration in milliseconds |
| `index` | int | Encoder/kernel index |

## Related Commands

| Command | Purpose | Relationship to Timeline |
|---------|---------|--------------------------|
| `stats` | Quick overview | High-level summary before detailed timeline |
| `shaders` | Shader performance | Identify expensive shaders to find in timeline |
| `timing` | Precise timing data | Numerical data behind timeline visualization |
| `encoders` | Encoder listing | Names/labels shown in timeline |
| `command-buffers` | Command structure | Underlying command organization |

## Best Practices

### 1. Start with Stats

```bash
# Always check basic stats first
gputrace stats trace.gputrace

# Then generate timeline for detailed analysis
gputrace timeline trace.gputrace -o timeline.json
```

### 2. Use Descriptive Encoder Labels

In your Metal code:
```objective-c
// Good: Descriptive label
encoder.label = @"FFN_Layer1_MatMul";

// Bad: No label (shows UUID in timeline)
[commandBuffer computeCommandEncoder];
```

### 3. Capture Multiple Iterations

```bash
# Capture multiple runs for consistency
for i in {1..5}; do
  MTL_CAPTURE_ENABLED=1 ./your_app
  mv trace.gputrace trace_$i.gputrace
  gputrace timeline trace_$i.gputrace -o timeline_$i.json
done

# Compare for variability
```

### 4. Combine with Performance Analysis

```bash
# Complete analysis workflow
gputrace stats trace.gputrace > stats.txt
gputrace shaders trace.gputrace > shaders.txt
gputrace timing trace.gputrace --format=csv > timing.csv
gputrace timeline trace.gputrace -o timeline.json

# Now you have:
# - stats.txt: Overview numbers
# - shaders.txt: Shader performance ranking
# - timing.csv: Precise timing data
# - timeline.json: Visual execution analysis
```

## Examples

### Example 1: Finding Sequential Bottlenecks

```bash
# Generate timeline
gputrace timeline model_inference.gputrace -o timeline.json

# In chrome://tracing:
# 1. Zoom out to see full timeline
# 2. Look for long sequential chains
# 3. Identify opportunities for parallelization
# 4. Note encoder names for optimization
```

### Example 2: Measuring Optimization Impact

```bash
# Before optimization
MTL_CAPTURE_ENABLED=1 ./app_v1
gputrace timeline trace_v1.gputrace -o v1_timeline.json

# After optimization (batch operations)
MTL_CAPTURE_ENABLED=1 ./app_v2
gputrace timeline trace_v2.gputrace -o v2_timeline.json

# Compare in chrome://tracing:
# v1: 50 encoders, 125ms total
# v2: 10 encoders, 85ms total
# Result: 32% speedup + 80% fewer encoder submissions
```

### Example 3: Debugging Performance Regression

```bash
# Baseline (known good)
gputrace timeline baseline.gputrace -o baseline_timeline.json

# Current (regression suspected)
gputrace timeline current.gputrace -o current_timeline.json

# Open both timelines and compare:
# - New encoders added?
# - Longer durations?
# - Different execution order?
# - Synchronization points?
```

## Summary

The `gputrace timeline` command provides:

✅ **Interactive visualization** of GPU execution
✅ **Chrome Tracing format** for wide tool compatibility
✅ **Encoder and kernel timelines** with precise durations
✅ **Metadata extraction** for automated analysis
✅ **Integration** with other gputrace commands

Use it for:
- Understanding execution order
- Finding performance bottlenecks
- Validating optimizations
- Debugging GPU behavior
- Documenting GPU workloads

Next steps:
- Try it on your traces: `gputrace timeline trace.gputrace -o timeline.json`
- Explore in Chrome: `chrome://tracing`
- Compare with timing data: `gputrace timing trace.gputrace`
- Share visualizations with your team
