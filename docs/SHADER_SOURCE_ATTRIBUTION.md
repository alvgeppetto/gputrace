# Shader Source-Level Performance Attribution

**Bead:** gputrace-58
**Date:** 2025-11-03
**Status:** Complete

## Overview

Shader source attribution provides **line-by-line performance analysis** for Metal shaders, similar to `go tool pprof -list` for CPU profiles or `perf annotate` for assembly code. This enables developers to identify expensive operations at the source code level, not just the function level.

## Quick Start

```bash
# Analyze a specific shader
gputrace shader-source trace.gputrace rope_single_freqs

# Generate HTML view with interactive visualization
gputrace shader-source trace.gputrace affine_qmm_t --format html -o shader.html

# Show optimization hints
gputrace shader-source trace.gputrace vv_Multiply --hints
```

## Features

### 1. Line-by-Line Performance Metrics

Each source line shows:
- **Time%**: Percentage of shader's GPU time attributed to this line
- **ALU%**: Estimated ALU utilization (0-100%)
- **Type**: Instruction classification (compute/memory/control)
- **Source Code**: The actual Metal shader source line

### 2. Hot Spot Identification

Automatically identifies the top 20% most expensive lines, marked with `>`:

```
> Line  142:  15.3% |  float4 result = texture.sample(sampler, uv);
  Line  145:   8.1% |  float3 normal = normalize(input.normal);
> Line  148:  12.7% |  float diffuse = max(dot(normal, lightDir), 0.0);
```

### 3. Instruction Type Classification

Lines are classified by operation type:
- **compute** (c): Arithmetic operations, math functions
- **memory** (m): Buffer access, texture sampling
- **control** (o): Branches, loops, returns

### 4. Optimization Hints

Actionable suggestions for expensive operations:

```
Line  142:  15.3% | float4 result = texture.sample(sampler, uv);
      ├─ 💡 Consider texture cache optimization
Line  148:  12.7% | float x = sqrt(a * a + b * b);
      ├─ 💡 sqrt is expensive; consider approximation if precision allows
```

### 5. Multiple Output Formats

- **text**: Terminal-friendly annotated source (default)
- **html**: Interactive HTML with syntax highlighting
- **json**: Structured data for custom analysis

## How It Works

### Architecture

```
┌─────────────────────┐
│  GPU Trace          │
│  (.gputrace)        │
└──────────┬──────────┘
           │
           ├─> Extract Shader Metrics (timing, invocations, occupancy)
           │
┌──────────v──────────┐
│  Shader Metrics     │
│  (per-shader stats) │
└──────────┬──────────┘
           │
           ├─> Map to Source File (.metal)
           │
┌──────────v──────────┐
│  Source File        │
│  (rope.metal)       │
└──────────┬──────────┘
           │
           ├─> Parse and Analyze Each Line
           │   - Classify instruction type
           │   - Estimate relative cost
           │   - Identify hot spots
           │
┌──────────v──────────┐
│  Source Attribution │
│  (line-level data)  │
└──────────┬──────────┘
           │
           └─> Format Output (text/html/json)
```

### Data Flow

1. **Extract Shader Metrics**
   - Parse command buffers and encoders from trace
   - Calculate per-shader timing, invocations, occupancy
   - Identify shader by name

2. **Locate Source File**
   - Use `ShaderSourceMapper` to find .metal file
   - Get kernel definition start line
   - Read and parse source code

3. **Analyze Each Source Line**
   - Classify instruction type (compute/memory/control)
   - Estimate complexity (1-10 scale)
   - Calculate relative cost using heuristics

4. **Distribute Metrics**
   - Allocate shader-level metrics to lines proportionally
   - Based on estimated cost of each line
   - Generate per-line percentages

5. **Identify Hot Spots**
   - Sort lines by cost
   - Mark top 20% as hot spots
   - Add optimization hints

6. **Generate Output**
   - Format as text/HTML/JSON
   - Include metrics, hints, and source code

## Command Usage

### Basic Usage

```bash
gputrace shader-source <trace.gputrace> <shader-name>
```

### Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `-f, --format` | string | text | Output format (text, html, json) |
| `-o, --output` | string | stdout | Output file path |
| `--hints` | bool | true | Show optimization hints |

### Examples

#### Example 1: Text Output (Default)

```bash
$ gputrace shader-source trace.gputrace rope_single_freqs

=== Shader Source Attribution: rope_single_freqs ===

Source: /path/to/mlx/backend/metal/rope.metal
Total GPU Time: 2.45 ms
Invocations: 128
Occupancy: 87.3%

Hot Spots (top 20% by cost):
  Line   42:  18.5% | float cos_val = cos(position * freq);
  Line   43:  16.2% | float sin_val = sin(position * freq);
  Line   47:  12.8% | output[idx] = input[idx] * cos_val + ...;

Annotated Source:
Line     Time%     ALU% Type | Source
----------------------------------------------------------------------------------------------------
   40      3.2%     4.1%    c | float position = float(gid);
   41      5.8%     7.3%    m | float freq = frequencies[gid % freq_size];
>  42     18.5%    23.1%    c | float cos_val = cos(position * freq);
      ├─ 💡 Transcendental functions are expensive; consider LUT or approximation
>  43     16.2%    20.5%    c | float sin_val = sin(position * freq);
      ├─ 💡 Transcendental functions are expensive; consider LUT or approximation
   45      4.2%     5.3%    m | float2 input_val = float2(input[idx], input[idx + 1]);
   46      8.3%    10.5%    c | float2 rotated = float2(
   47     12.8%    16.1%    c |     input_val.x * cos_val - input_val.y * sin_val,
   48      9.7%    12.2%    c |     input_val.x * sin_val + input_val.y * cos_val);
   49      6.4%     8.1%    m | output[idx] = rotated.x;
   50      5.9%     7.4%    m | output[idx + 1] = rotated.y;
```

#### Example 2: HTML Output

```bash
$ gputrace shader-source trace.gputrace affine_qmm_t --format html -o shader.html
✓ Written to: shader.html

# Open in browser
open shader.html
```

The HTML output provides:
- Syntax-highlighted Metal source code
- Color-coded performance metrics
- Visual hot spot highlighting (red background)
- Interactive hover tooltips
- Dark theme optimized for readability

#### Example 3: JSON Export

```bash
$ gputrace shader-source trace.gputrace vv_Multiply --format json -o analysis.json
```

JSON structure:
```json
{
  "ShaderName": "vv_Multiply",
  "SourceFile": "/path/to/binary_ops.metal",
  "Metrics": {
    "name": "vv_Multiply",
    "total_duration_ns": 1234567,
    "invocation_count": 64,
    "occupancy": 0.92
  },
  "Lines": [
    {
      "LineNumber": 42,
      "SourceCode": "    float result = a[gid] * b[gid];",
      "GPUTimePercent": 15.3,
      "ALUUtilization": 18.7,
      "InstructionType": "compute",
      "Complexity": 2,
      "IsHotSpot": true,
      "Hints": []
    }
  ],
  "HotSpots": [...]
}
```

## Instruction Classification

### Compute Operations

**Detected Patterns:**
- Arithmetic: `*`, `+`, `-`, `/`
- Math functions: `sqrt`, `exp`, `log`, `sin`, `cos`, `tan`

**Cost Factors:**
- Basic arithmetic: 2x
- Math functions (sqrt, exp, log): 4x
- Trigonometric (sin, cos): 5x

**Example:**
```metal
float x = a * b + c;           // compute, complexity 2
float y = sqrt(x * x + z * z); // compute, complexity 4
```

### Memory Operations

**Detected Patterns:**
- Buffer access: `device` pointer dereference with `[]` or `*`
- Texture operations: `texture.sample()`, `texture.read()`, `texture.write()`

**Cost Factors:**
- Buffer access: 3x
- Texture operations: 5x

**Example:**
```metal
float val = input[gid];                      // memory, complexity 3
float4 color = tex.sample(sampler, coords);  // memory, complexity 5
```

### Control Flow

**Detected Patterns:**
- Conditionals: `if`, `else`
- Loops: `for`, `while`
- Returns: `return`

**Cost Factors:**
- Branches: 2x (potential divergence)

**Example:**
```metal
if (condition) {  // control, complexity 2
    // ...
}
```

## Optimization Hints

### Memory Access Hints

| Pattern | Hint |
|---------|------|
| `texture.sample()` | Consider texture cache optimization |
| `buffer[index]` (not threadgroup) | Consider using threadgroup memory for repeated access |
| Scattered access | Consider memory coalescing for better bandwidth |

### Compute Hints

| Operation | Hint |
|-----------|------|
| Division (`/`) | Division is expensive; consider multiplication by reciprocal |
| `sqrt()` | sqrt is expensive; consider approximation if precision allows |
| `exp()`, `log()` | Transcendental functions are expensive; consider LUT or approximation |
| `sin()`, `cos()` | Consider precomputation or fast approximations |

### Control Flow Hints

| Pattern | Hint |
|---------|------|
| `if` statements | Branch divergence may reduce GPU efficiency |
| Complex loops | Consider unrolling or reducing iterations |

## Comparison with Other Tools

### vs. `go tool pprof -list`

**Similarities:**
- Line-by-line performance attribution
- Hot spot identification
- Source code annotation
- Multiple output formats

**Differences:**
- pprof uses sampling; we use static analysis + trace metrics
- pprof has exact per-line samples; we use cost estimates
- We provide Metal-specific optimization hints

### vs. `perf annotate`

**Similarities:**
- Assembly/source annotation with performance data
- Percentage attribution to instructions
- Hot path highlighting

**Differences:**
- perf uses hardware counters; we use trace timing + heuristics
- perf shows assembly; we show Metal source
- We classify instruction types (compute/memory)

### vs. Metal Shader Profiler

**Similarities:**
- Shader-level performance analysis
- GPU metrics (occupancy, bandwidth)

**Differences:**
- Metal Profiler requires Xcode; this is CLI-based
- Metal Profiler shows GPU instructions; we show source
- We integrate with existing .gputrace workflow

## Limitations

### 1. Static Analysis Heuristics

**Limitation:** Per-line costs are estimated using pattern matching, not measured.

**Impact:** Attribution percentages are approximate, not exact.

**Mitigation:** Use relative comparisons (Line A is 2x more expensive than Line B) rather than absolute percentages.

### 2. No Compiler Optimization Visibility

**Limitation:** Metal compiler may reorder, fuse, or eliminate operations.

**Impact:** Source line may not map 1:1 to GPU instructions.

**Mitigation:** Focus on identifying expensive operations (sin/cos, divisions) rather than exact timing.

### 3. No Hardware Counter Integration

**Limitation:** We don't have access to real ALU utilization, cache hit rates, etc. per line.

**Impact:** ALU% and bandwidth estimates are extrapolated from shader-level metrics.

**Mitigation:** Use as directional guidance; validate with Metal Frame Capture for critical optimizations.

### 4. Source Location Dependency

**Limitation:** Requires .metal source files to be indexed.

**Impact:** Won't work if source files aren't available.

**Mitigation:** Ensure MLX or custom shader paths are accessible. See SHADER_SOURCE_MAPPING.md.

## Best Practices

### 1. Start with Hot Spots

Focus optimization efforts on the top 20% most expensive lines:

```bash
# Identify hot spots
gputrace shader-source trace.gputrace my_kernel | grep ">"
```

### 2. Cross-Reference with Aggregate Metrics

Compare line-level attribution with shader-level metrics:

```bash
# Get shader overview
gputrace shaders trace.gputrace

# Then drill into specific shader
gputrace shader-source trace.gputrace expensive_shader
```

### 3. Validate with Metal Frame Capture

Use this tool for quick identification, then validate with Xcode:

1. Identify expensive operations with `shader-source`
2. Capture frame in Xcode Instruments
3. Verify with Metal Shader Profiler
4. Implement optimizations
5. Re-analyze with `shader-source` to confirm

### 4. Use HTML for Presentations

Generate HTML reports for team reviews:

```bash
gputrace shader-source trace.gputrace my_kernel --format html -o report.html
```

### 5. Automate with JSON

Integrate into CI/CD pipelines:

```bash
# Extract hot spots programmatically
gputrace shader-source trace.gputrace kernel --format json | \
  jq '.HotSpots[] | select(.GPUTimePercent > 10)'
```

## Integration

### With Existing gputrace Commands

```bash
# 1. Overview of all shaders
gputrace shaders trace.gputrace

# 2. Identify expensive shader
gputrace shader-metrics trace.gputrace --sort time

# 3. Source-level analysis
gputrace shader-source trace.gputrace rope_single_freqs

# 4. Export to pprof for further analysis
gputrace2pprof trace.gputrace -all
```

### With MLX Workflow

```bash
# 1. Capture MLX trace
python mlx_benchmark.py  # Generates trace.gputrace

# 2. Analyze shader performance
gputrace shader-source trace.gputrace affine_qmm_t --hints

# 3. Optimize shader in MLX backend
vim mlx/backend/metal/affine.metal

# 4. Re-capture and compare
python mlx_benchmark.py  # New trace
gputrace shader-source new_trace.gputrace affine_qmm_t
```

## Troubleshooting

### "shader not found in trace"

**Cause:** Shader name doesn't match trace data.

**Solution:**
```bash
# List available shaders
gputrace shaders trace.gputrace

# Use exact name
gputrace shader-source trace.gputrace "rope_single_freqs_float16"
```

### "source file not found"

**Cause:** .metal files not in indexed locations.

**Solution:**
```bash
# Check indexed kernels
export MLX_SHADER_PATH="/custom/path/to/shaders"
gputrace shader-source trace.gputrace my_kernel
```

See SHADER_SOURCE_MAPPING.md for source indexing details.

### Unexpected Cost Estimates

**Cause:** Static analysis heuristics may not match actual GPU behavior.

**Solution:** Use as directional guide; validate critical findings with Metal Frame Capture.

## Future Enhancements

Planned improvements:

1. **AIR/GPU Instruction Correlation**
   - Parse Metal Intermediate Language (AIR) for precise instruction mapping
   - Map source lines to actual GPU instructions

2. **Hardware Counter Integration**
   - Real per-line ALU utilization from GPU counters
   - Actual cache hit rates and memory bandwidth

3. **Compiler Optimization Visibility**
   - Show which source lines were optimized/fused
   - Display GPU instruction scheduling

4. **Interactive Diffing**
   - Compare two traces side-by-side
   - Show optimization impact per source line

5. **Call Graph Attribution**
   - Attribute costs through function calls
   - Show caller/callee relationships

## References

- [SHADER_SOURCE_MAPPING.md](./SHADER_SOURCE_MAPPING.md) - Source file indexing
- [SHADER_PPROF_GUIDE.md](./SHADER_PPROF_GUIDE.md) - pprof integration
- [Metal Shading Language Specification](https://developer.apple.com/metal/Metal-Shading-Language-Specification.pdf)
- [go tool pprof documentation](https://github.com/google/pprof/blob/main/doc/README.md)
- [perf annotate](https://man7.org/linux/man-pages/man1/perf-annotate.1.html)

## Files

**Implementation:**
- `shader_source_attribution.go` (450 lines) - Core attribution engine
- `cmd/gputrace/cmd/shader_source.go` (130 lines) - CLI command
- `docs/SHADER_SOURCE_ATTRIBUTION.md` (this file)

**Dependencies:**
- `shader_source_mapper.go` - Source file location
- `shader_metrics.go` - Performance metrics extraction
- Existing shader analysis infrastructure

**Total:** 580 lines of new code + comprehensive documentation
