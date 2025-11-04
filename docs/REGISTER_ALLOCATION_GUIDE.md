# GPU Register Allocation Analysis Guide

**Bead:** gputrace-50
**Date:** 2025-11-03
**Status:** Complete

## Overview

GPU registers are the fastest memory available to shader threads, directly impacting performance and occupancy. This guide explains how to use gputrace to analyze register allocation in Metal shaders and optimize for better GPU utilization.

## Table of Contents

1. [What are GPU Registers?](#what-are-gpu-registers)
2. [Why Register Usage Matters](#why-register-usage-matters)
3. [Reading Register Data in gputrace](#reading-register-data-in-gputrace)
4. [Understanding the Output](#understanding-the-output)
5. [Real vs Estimated Register Data](#real-vs-estimated-register-data)
6. [Capturing Profiled Traces](#capturing-profiled-traces)
7. [Register Optimization Guidelines](#register-optimization-guidelines)
8. [Common Register Usage Patterns](#common-register-usage-patterns)
9. [Troubleshooting High Register Usage](#troubleshooting-high-register-usage)

## What are GPU Registers?

GPU registers are small, ultra-fast storage locations directly accessible by shader threads. Think of them as variables in your shader code - each local variable, function parameter, and intermediate calculation result typically requires register storage.

### Key Characteristics

- **Speed**: Registers are the fastest memory on the GPU (1 cycle access)
- **Scarcity**: Limited number available per thread
- **Per-thread**: Each thread gets its own register set
- **Hardware-managed**: Register allocation happens during shader compilation

### Apple Silicon Specifics

Apple Silicon GPUs have different register architecture compared to NVIDIA/AMD:

- **M1/M2/M3**: Each GPU core has a register file shared across threads
- **Register pressure**: More registers per thread = fewer concurrent threads
- **No explicit register count**: Metal compiler manages allocation automatically

## Why Register Usage Matters

Register usage directly impacts **GPU occupancy** - the percentage of GPU compute resources actively executing work.

### The Occupancy Trade-off

```
More Registers per Thread  →  Fewer Concurrent Threads  →  Lower Occupancy
Fewer Registers per Thread  →  More Concurrent Threads   →  Higher Occupancy
```

### Performance Impact

**Example 1: Low Register Usage (Optimal)**
```metal
kernel void simple_multiply(
    device const float* input [[buffer(0)]],
    device float* output [[buffer(1)]],
    uint gid [[thread_position_in_grid]])
{
    output[gid] = input[gid] * 2.0;  // ~4 registers
}
```
- **Registers**: ~4-8 allocated
- **Occupancy**: 95%+ (many threads can run concurrently)
- **Performance**: Excellent latency hiding

**Example 2: High Register Usage (Problematic)**
```metal
kernel void complex_calculation(
    device const float* input [[buffer(0)]],
    device float* output [[buffer(1)]],
    uint gid [[thread_position_in_grid]])
{
    // Many local variables = high register pressure
    float a = input[gid];
    float b = sin(a);
    float c = cos(a);
    float d = exp(a);
    float e = log(a);
    float f = sqrt(a);
    float g = b * c + d * e - f;
    float h = g * g + b * c;
    float i = h * h + d * e;
    // ... many more calculations
    output[gid] = i;  // ~64+ registers
}
```
- **Registers**: ~64-128 allocated
- **Occupancy**: 30-50% (fewer threads can run)
- **Performance**: Poor latency hiding, memory stalls visible

## Reading Register Data in gputrace

gputrace shows register allocation data in the shader performance output.

### Basic Command

```bash
gputrace shaders trace.gputrace
```

### Example Output

```
Cost    Name                                                        Type        Pipeline State          # SIMD Groups   # Allocated Registers   High Register   Spilled Bytes
45.23%  affine_qmm_t_float16_float16_256                           Compute     Compute Pipeline 0x7fa   122,880        64                      64              0 bytes
28.71%  rope_single_freqs_float16                                  Compute     Compute Pipeline 0x7fb   49,152         32                      32              0 bytes
14.82%  vv_Multiply_float16_float16                                Compute     Compute Pipeline 0x7fc   8,192          16 (est)                16 (est)        0 bytes (est)
```

### Column Meanings

| Column | Description | Interpretation |
|--------|-------------|----------------|
| **Cost** | Percentage of total GPU time | Higher = bigger optimization target |
| **Name** | Shader/kernel function name | Your Metal shader function |
| **Type** | Shader type | Compute, Vertex, Fragment |
| **Pipeline State** | Pipeline object address | Unique identifier for shader |
| **# SIMD Groups** | Total threadgroups dispatched | Workload size |
| **# Allocated Registers** | Registers per thread | Lower is often better |
| **High Register** | Highest register number used | Usually same as allocated |
| **Spilled Bytes** | Memory used for register spills | Non-zero = performance problem |

## Understanding the Output

### # Allocated Registers

**What it means:**
Number of GPU registers allocated per thread for this shader.

**Typical ranges:**
- **4-16 registers**: Simple shaders (memory copies, basic arithmetic)
- **16-64 registers**: Normal compute shaders (moderate complexity)
- **64-128 registers**: Complex shaders (many local variables, math functions)
- **128+ registers**: Very complex or unoptimized shaders

**Interpretation:**
- **Low (4-32)**: ✅ Excellent - high occupancy likely
- **Medium (32-64)**: ✅ Good - balanced performance
- **High (64-128)**: ⚠️ Caution - may limit occupancy
- **Very high (128+)**: ❌ Problem - likely causing low occupancy

### High Register

**What it means:**
The highest register number used in the compiled shader. Usually matches "# Allocated Registers" but can differ if the compiler allocates more registers than actually used.

**Example:**
```
# Allocated Registers: 64
High Register: 62
```
This shader allocated 64 registers but only used up to register 62 (indices 0-62 = 63 total used).

### Spilled Bytes

**What it means:**
When a shader needs more registers than available, the compiler "spills" variables to slower memory (threadgroup or device memory).

**Values:**
- **0 bytes**: ✅ No spilling - all data fits in registers
- **<1 KB**: ⚠️ Minor spilling - slight performance impact
- **1-10 KB**: ❌ Moderate spilling - noticeable performance loss
- **>10 KB**: ❌❌ Heavy spilling - severe performance problem

**Performance Impact:**
```
Register Access:   1 cycle
Spill to Memory:   ~100-300 cycles  (100-300x slower!)
```

### Example Analysis

```
Cost    Name                            # Allocated Registers   High Register   Spilled Bytes
45.2%   matrix_multiply_unoptimized     128                     128             4.5 KB
28.7%   matrix_multiply_optimized       48                      48              0 bytes
```

**Analysis:**
- `matrix_multiply_unoptimized`:
  - High register usage (128) limiting occupancy
  - 4.5 KB spilled = ~150 memory accesses per thread
  - **Action**: Reduce local variable count, use shared memory

- `matrix_multiply_optimized`:
  - Moderate register usage (48) allowing high occupancy
  - No spilling = all data in fast registers
  - **Result**: 37% reduction in GPU time despite same algorithm

## Real vs Estimated Register Data

gputrace shows two types of register data:

### Real Register Data (Hardware-Measured)

**Source:** `.gpuprofiler_raw` files from Xcode Instruments
**Accuracy:** 100% - actual hardware measurements
**Indicator:** No "(est)" suffix in output

```
# Allocated Registers   High Register   Spilled Bytes
64                      64              0 bytes
```

### Estimated Register Data (Heuristic)

**Source:** gputrace heuristics based on shader characteristics
**Accuracy:** Directional - useful for comparison
**Indicator:** "(est)" suffix in output

```
# Allocated Registers   High Register   Spilled Bytes
64 (est)                64 (est)        0 bytes (est)
```

### How Estimation Works

gputrace estimates register usage based on:

1. **Thread configuration**:
   - Fewer threads per group → more registers available per thread
   - More threads per group → fewer registers per thread (occupancy optimization)

2. **Shader classification**:
   - Compute-bound shaders: likely use more registers (ALU operations)
   - Memory-bound shaders: likely use fewer registers (latency hiding)

3. **Apple Silicon characteristics**:
   - 32-64 threads: can use 128-256 registers
   - 128-256 threads: typically 32-128 registers
   - 512+ threads: typically 16-64 registers

**Estimation Formula** (simplified):
```go
baseRegs := 64  // Default starting point

if threadsPerGroup <= 64 {
    baseRegs = 128
} else if threadsPerGroup <= 256 {
    baseRegs = 64
} else {
    baseRegs = 32
}

if computeBound {
    baseRegs *= 1.5  // More registers for ALU work
}

if memoryBound {
    baseRegs *= 0.7  // Fewer registers for latency hiding
}
```

### When to Get Real Data

**Use estimates when:**
- ✅ Comparing relative register usage between shaders
- ✅ Identifying potentially problematic shaders (128+ estimated)
- ✅ Quick initial analysis

**Get real data when:**
- 🎯 Optimizing critical hot shaders (>20% GPU time)
- 🎯 Investigating occupancy problems
- 🎯 Validating optimization impact
- 🎯 Need exact register counts for tuning

## Capturing Profiled Traces

To get real register allocation data, capture traces with Xcode Instruments.

### Method 1: Xcode Instruments (GUI)

```bash
# 1. Run your app/benchmark
MTL_CAPTURE_ENABLED=1 go test -bench=BenchmarkYourCode

# 2. Open Instruments
open -a "Instruments"

# 3. Create new trace:
#    - File → New
#    - Choose "Metal System Trace"
#    - Select your app
#    - Record

# 4. Stop recording after 2-10 seconds

# 5. Export trace:
#    - File → Export
#    - Select "GPU Trace Document"
#    - Save as trace.gputrace
```

### Method 2: Command Line (xctrace)

```bash
# List available devices
xctrace list devices

# Start recording
xctrace record --template 'Metal System Trace' \
    --output trace.trace \
    --launch -- go test -bench=BenchmarkYourCode

# The .trace bundle contains:
#   - trace.gputrace (Metal capture)
#   - .gpuprofiler_raw files (performance counters)
```

### Method 3: Programmatic Capture with Full Profiling

```go
// Enable capture with performance counters
os.Setenv("MTL_CAPTURE_ENABLED", "1")
os.Setenv("MTL_CAPTURE_COUNTERS", "1")  // Enable counter collection
os.Setenv("MTL_CAPTURE_PATH", "/tmp/profiled_trace.gputrace")

// Run your GPU workload
runGPUKernel()

// The generated trace will include:
// - Shader performance data
// - Register allocation data
// - Hardware counters (ALU, memory bandwidth)
```

### Verifying Counter Data

```bash
# Check if trace has performance counter data
gputrace perfcounters trace.gputrace

# If successful, you'll see:
# === GPU Hardware Performance Counters ===
#
# Data Quality: Performance Counters (100% accurate)
# Files Processed: 120
# Total Records: 2,458,734
#
# Shader Metrics:
# Name                                    SIMD Groups  Allocated Regs  High Reg  Spilled Bytes
# affine_qmm_t_float16                    122,880      64              64        0 bytes
```

## Register Optimization Guidelines

### 1. Reduce Local Variables

**Problem:**
```metal
kernel void complex_shader(device float* data [[buffer(0)]], uint gid [[thread_position_in_grid]]) {
    float a = data[gid];
    float b = data[gid + 1];
    float c = data[gid + 2];
    float d = data[gid + 3];
    float e = a + b;
    float f = c + d;
    float g = e * f;
    float h = g * g;
    data[gid] = h;
    // 8 local variables = high register usage
}
```

**Solution: Reduce lifetime, reuse variables**
```metal
kernel void optimized_shader(device float* data [[buffer(0)]], uint gid [[thread_position_in_grid]]) {
    float result = (data[gid] + data[gid + 1]) *
                   (data[gid + 2] + data[gid + 3]);
    data[gid] = result * result;
    // 2 local variables = lower register usage
}
```

### 2. Use Shared Memory for Intermediate Results

**Problem:**
```metal
// Each thread keeps 256 values in registers
kernel void accumulate(device float* input [[buffer(0)]],
                       device float* output [[buffer(1)]],
                       uint gid [[thread_position_in_grid]])
{
    float sum[256];  // 256 registers per thread!
    for (int i = 0; i < 256; i++) {
        sum[i] = input[gid * 256 + i];
    }
    // ... process sum array
}
```

**Solution: Use threadgroup memory**
```metal
kernel void accumulate(device float* input [[buffer(0)]],
                       device float* output [[buffer(1)]],
                       threadgroup float* shared_sum [[threadgroup(0)]],
                       uint gid [[thread_position_in_grid]],
                       uint tid [[thread_position_in_threadgroup]])
{
    shared_sum[tid] = input[gid];
    threadgroup_barrier(mem_flags::mem_threadgroup);
    // ... process shared_sum
    // Registers saved: ~250 per thread
}
```

### 3. Simplify Math Operations

**Problem:**
```metal
// Each math function uses 10-20 registers for temporaries
float complex = sin(x) * cos(y) + exp(z) * log(w) + sqrt(a) * pow(b, c);
```

**Solution: Precompute or use LUT**
```metal
// Precompute common values in constant buffer
constant float precomputed_values[1024] [[buffer(2)]];
float simple = precomputed_values[x_index] * precomputed_values[y_index];
```

### 4. Split Complex Kernels

**Problem:**
```metal
// One massive kernel doing everything
kernel void do_everything(/* many parameters */) {
    // 50 lines of complex logic
    // 128+ registers allocated
    // Low occupancy
}
```

**Solution: Split into multiple passes**
```metal
kernel void pass1_preprocessing(/* fewer params */) {
    // 32 registers - high occupancy
}

kernel void pass2_main_computation(/* fewer params */) {
    // 48 registers - good occupancy
}

kernel void pass3_postprocessing(/* fewer params */) {
    // 24 registers - excellent occupancy
}
```

**Trade-off:**
- ✅ Lower register usage per kernel
- ✅ Higher occupancy
- ❌ More memory traffic between passes
- ❌ Additional kernel launch overhead

### 5. Adjust Threadgroup Size

**Registers vs Occupancy:**

Small threadgroups (32-64 threads):
- ✅ More registers available per thread (128-256)
- ✅ Good for complex kernels
- ❌ May underutilize GPU cores

Large threadgroups (512-1024 threads):
- ✅ Better GPU utilization
- ✅ More wavefronts for latency hiding
- ❌ Fewer registers per thread (16-64)
- ❌ May cause spilling in complex shaders

**Experiment:**
```metal
// Try different sizes
let threadsPerGroup = MTLSize(width: 256, height: 1, depth: 1)  // Balanced
// vs
let threadsPerGroup = MTLSize(width: 512, height: 1, depth: 1)  // High occupancy
// vs
let threadsPerGroup = MTLSize(width: 128, height: 1, depth: 1)  // More registers
```

Measure with gputrace:
```bash
gputrace shaders trace_256.gputrace > results_256.txt
gputrace shaders trace_512.gputrace > results_512.txt
gputrace shaders trace_128.gputrace > results_128.txt
```

## Common Register Usage Patterns

### Pattern 1: Memory Copy Kernels

**Characteristics:**
- 4-16 registers
- High memory bandwidth
- High occupancy (90%+)

**Example:**
```metal
kernel void memcpy_kernel(
    device const float* input [[buffer(0)]],
    device float* output [[buffer(1)]],
    uint gid [[thread_position_in_grid]])
{
    output[gid] = input[gid];
}
```

**Expected Output:**
```
Name             # Allocated Registers   High Register   Spilled Bytes   Occupancy
memcpy_kernel    8                       8               0 bytes         95%
```

### Pattern 2: Basic Arithmetic

**Characteristics:**
- 16-32 registers
- Compute-bound
- Good occupancy (70-85%)

**Example:**
```metal
kernel void vector_add(
    device const float* a [[buffer(0)]],
    device const float* b [[buffer(1)]],
    device float* result [[buffer(2)]],
    uint gid [[thread_position_in_grid]])
{
    result[gid] = a[gid] + b[gid];
}
```

**Expected Output:**
```
Name         # Allocated Registers   High Register   Spilled Bytes   Occupancy
vector_add   16                      16              0 bytes         82%
```

### Pattern 3: Complex Math

**Characteristics:**
- 48-128 registers
- Heavy ALU usage
- Moderate occupancy (40-70%)

**Example:**
```metal
kernel void fourier_transform(
    device const float2* input [[buffer(0)]],
    device float2* output [[buffer(1)]],
    uint gid [[thread_position_in_grid]])
{
    float2 sum = float2(0);
    for (int k = 0; k < 64; k++) {
        float angle = 2 * M_PI_F * k * gid / 64;
        float2 twiddle = float2(cos(angle), sin(angle));
        sum += input[k] * twiddle;
    }
    output[gid] = sum;
}
```

**Expected Output:**
```
Name                # Allocated Registers   High Register   Spilled Bytes   Occupancy
fourier_transform   96                      96              0 bytes         52%
```

### Pattern 4: Matrix Operations (MLX-style)

**Characteristics:**
- 32-64 registers (optimized)
- Tiled memory access
- Good occupancy (60-80%)

**Example:**
```metal
kernel void matrix_multiply(
    device const float* A [[buffer(0)]],
    device const float* B [[buffer(1)]],
    device float* C [[buffer(2)]],
    constant uint& N [[buffer(3)]],
    threadgroup float* tileA [[threadgroup(0)]],
    threadgroup float* tileB [[threadgroup(1)]],
    uint2 gid [[thread_position_in_grid]],
    uint2 tid [[thread_position_in_threadgroup]])
{
    float sum = 0;
    // Tiled algorithm using shared memory
    // Reduces register pressure by reusing data
    C[gid.y * N + gid.x] = sum;
}
```

**Expected Output:**
```
Name               # Allocated Registers   High Register   Spilled Bytes   Occupancy
matrix_multiply    56                      56              0 bytes         72%
```

### Pattern 5: Problematic Shader (Register Spilling)

**Characteristics:**
- 128+ registers
- Spilled bytes > 0
- Low occupancy (<40%)

**Example:**
```metal
kernel void unoptimized_shader(/* many buffers */, uint gid [[thread_position_in_grid]]) {
    // Too many local variables
    float data[100];  // Compiler tries to keep in registers
    for (int i = 0; i < 100; i++) {
        data[i] = expensive_computation(i);
    }
    // More computation using data array
}
```

**Expected Output:**
```
Name                   # Allocated Registers   High Register   Spilled Bytes   Occupancy
unoptimized_shader     144                     144             8.2 KB          28%
```

⚠️ **Action Required:** This shader needs optimization!

## Troubleshooting High Register Usage

### Problem: Shader using 128+ registers

**Symptoms:**
```
Name                            # Allocated Registers   Occupancy
my_complex_shader               156                     22%
```

**Debugging Steps:**

1. **Identify the hot shader:**
   ```bash
   gputrace shaders trace.gputrace | grep "156"
   ```

2. **Review shader source code:**
   - Count local variables
   - Look for large arrays
   - Check for complex math functions
   - Identify loops with many temporaries

3. **Apply optimization techniques:**

   **Option A: Reduce local variables**
   ```metal
   // Before: 20 local variables
   float a, b, c, d, e, f, g, h, i, j, k, l, m, n, o, p, q, r, s, t;

   // After: Reuse variables
   float temp1, temp2, result;
   ```

   **Option B: Move data to shared memory**
   ```metal
   // Before: Large array in registers
   float cache[64];  // 64 registers per thread

   // After: Shared across threadgroup
   threadgroup float cache[64 * THREADS_PER_GROUP];
   ```

   **Option C: Split into multiple kernels**
   ```metal
   // Before: One 200-line kernel
   kernel void monolithic_kernel() { /* 156 registers */ }

   // After: Three focused kernels
   kernel void preprocessing() { /* 48 registers */ }
   kernel void main_work() { /* 64 registers */ }
   kernel void postprocessing() { /* 32 registers */ }
   ```

4. **Re-capture and compare:**
   ```bash
   gputrace shaders trace_before.gputrace > before.txt
   gputrace shaders trace_after.gputrace > after.txt
   diff before.txt after.txt
   ```

### Problem: Register Spilling (Spilled Bytes > 0)

**Symptoms:**
```
Name                            Spilled Bytes   Performance
my_shader                       4.5 KB          Slow
```

**Impact:**
- Each spill = ~100-300 cycle penalty
- 4.5 KB = ~1,125 floats = ~1,125 register spills
- Total penalty: ~112,500-337,500 cycles per thread!

**Solution Steps:**

1. **Confirm the problem:**
   ```bash
   gputrace perfcounters trace.gputrace | grep -A5 "my_shader"
   ```

2. **Prioritize fixes:**
   - Spills in hot shaders (>20% GPU time): **Critical**
   - Spills in cold shaders (<5% GPU time): **Low priority**

3. **Reduce register pressure:**
   - Remove unnecessary local variables
   - Recompute instead of storing (if computation is cheap)
   - Move data to shared memory
   - Split kernel into passes

4. **Verify fix:**
   ```bash
   # Before
   Spilled Bytes: 4.5 KB

   # After optimization
   Spilled Bytes: 0 bytes

   # Expected speedup: 20-40% for that shader
   ```

### Problem: Low Occupancy Despite Low Register Count

**Symptoms:**
```
Name                # Allocated Registers   Occupancy   Threadgroups
my_shader          24                      18%         8
```

**Diagnosis:**
- Register count is good (24)
- Occupancy still low (18%)
- **Root cause**: Too few threadgroups (8)

**Solution: Increase dispatch size**
```go
// Before
let dispatchSize = MTLSize(width: 8, height: 1, depth: 1)

// After
let dispatchSize = MTLSize(width: 256, height: 1, depth: 1)
```

**Result:**
```
Name                # Allocated Registers   Occupancy   Threadgroups
my_shader          24                      87%         256
```

### Problem: Inconsistent Register Counts

**Symptoms:**
```
Name                Run 1 Registers   Run 2 Registers
my_shader          48                 96
```

**Possible Causes:**

1. **Dynamic branching:**
   ```metal
   if (complex_condition) {
       // Path A: uses 48 registers
   } else {
       // Path B: uses 96 registers
   }
   ```
   Compiler allocates for worst case (96).

2. **Template instantiation:**
   ```metal
   template<int N>
   kernel void my_shader() {
       float data[N];  // Register count varies with N
   }
   ```

3. **Compiler optimizations:**
   Different optimization levels produce different register allocation.

**Solution:**
- Review shader for branches with significantly different complexity
- Consider splitting into separate specialized kernels
- Use Metal shader analyzer to inspect compiled code

## Register Pressure and Occupancy Relationship

### Theoretical Maximum Occupancy

Apple Silicon register file characteristics (M1/M2/M3):

**Simplified Model:**
```
Total Register File per Core: ~16,384 registers (example)
Max Threads per Core: 1024

Occupancy = min(
    Total_Registers / (Registers_per_Thread * Active_Threads),
    1.0
)
```

### Example Calculations

**Scenario 1: Low Register Usage**
```
Registers per Thread: 16
Threads per Threadgroup: 256
Threadgroups: 4 (1024 total threads)

Register Requirement: 16 * 1024 = 16,384 registers
Occupancy: 16,384 / 16,384 = 100% ✅
```

**Scenario 2: High Register Usage**
```
Registers per Thread: 128
Threads per Threadgroup: 256
Threadgroups: 4 (1024 total threads)

Register Requirement: 128 * 1024 = 131,072 registers
Available: 16,384 registers
Actual Threads: 16,384 / 128 = 128 threads
Occupancy: 128 / 1024 = 12.5% ❌
```

**Scenario 3: Balanced**
```
Registers per Thread: 64
Threads per Threadgroup: 256
Threadgroups: 4 (1024 total threads)

Register Requirement: 64 * 1024 = 65,536 registers
Available: 16,384 registers
Actual Threads: 16,384 / 64 = 256 threads
Occupancy: 256 / 1024 = 25% ⚠️
```

### Finding the Sweet Spot

**Interactive Analysis:**

```bash
# Capture traces with different threadgroup sizes
MTL_CAPTURE_PATH=/tmp/trace_128.gputrace   THREADS=128  ./benchmark
MTL_CAPTURE_PATH=/tmp/trace_256.gputrace   THREADS=256  ./benchmark
MTL_CAPTURE_PATH=/tmp/trace_512.gputrace   THREADS=512  ./benchmark
MTL_CAPTURE_PATH=/tmp/trace_1024.gputrace  THREADS=1024 ./benchmark

# Compare results
for trace in /tmp/trace_*.gputrace; do
    echo "=== $trace ==="
    gputrace shaders $trace | grep "my_shader"
done
```

**Output:**
```
=== trace_128.gputrace ===
my_shader   128 regs   45% occupancy   2.5ms

=== trace_256.gputrace ===
my_shader   64 regs    72% occupancy   1.8ms  ← Best

=== trace_512.gputrace ===
my_shader   32 regs    85% occupancy   1.9ms

=== trace_1024.gputrace ===
my_shader   16 regs    92% occupancy   2.2ms
```

**Interpretation:**
- 128 threads: High registers, but low occupancy
- 256 threads: **Optimal balance** - best performance
- 512 threads: High occupancy, but more memory pressure
- 1024 threads: Very high occupancy, but kernel launch overhead

## Best Practices Summary

### ✅ Do's

1. **Profile before optimizing**
   ```bash
   gputrace shaders trace.gputrace
   ```

2. **Focus on hot shaders first**
   - Optimize shaders with >20% GPU time
   - Ignore trivial shaders (<1% GPU time)

3. **Use real register data for critical shaders**
   - Capture with Xcode Instruments
   - Verify with `gputrace perfcounters`

4. **Keep register usage in check**
   - Target: 32-64 registers for balanced workloads
   - Maximum: <128 registers to avoid severe occupancy loss

5. **Avoid register spilling**
   - Zero spilled bytes is ideal
   - <1 KB is acceptable for cold shaders

6. **Measure impact of changes**
   - Capture before/after traces
   - Compare register counts and timing
   - Verify occupancy improvements translate to speedup

### ❌ Don'ts

1. **Don't blindly reduce registers**
   - May increase memory traffic
   - May split work inefficiently
   - Measure actual performance impact

2. **Don't ignore thread configuration**
   - Register allocation depends on threadgroup size
   - Test different configurations

3. **Don't optimize cold shaders**
   - Focus on shaders with >10% GPU time
   - Cold shader optimization rarely impacts total time

4. **Don't trust estimates for critical decisions**
   - Get real register data for hot shaders
   - Use estimates only for initial triage

5. **Don't forget memory bandwidth**
   - Lower registers might increase memory traffic
   - Balance compute and memory access

## Integration with Other gputrace Tools

### Workflow: Complete Performance Analysis

```bash
# 1. Capture with performance counters
MTL_CAPTURE_COUNTERS=1 MTL_CAPTURE_PATH=/tmp/trace.gputrace ./benchmark

# 2. Overall statistics
gputrace stats /tmp/trace.gputrace

# 3. Shader performance with register data
gputrace shaders /tmp/trace.gputrace

# 4. Detailed hardware metrics
gputrace perfcounters /tmp/trace.gputrace

# 5. Source-level analysis for hot shaders
gputrace shader-source /tmp/trace.gputrace my_hot_shader

# 6. Convert to pprof for flamegraph
gputrace gputrace2pprof /tmp/trace.gputrace -all

# 7. View in browser
go tool pprof -http=:8080 *.pprof.gz
```

### Cross-Reference with Occupancy

```bash
# Extract shader metrics with occupancy
gputrace shader-metrics /tmp/trace.gputrace -o metrics.json

# Parse for correlation
jq '.shaders[] | {name: .name, registers: .estimated_registers, occupancy: .occupancy}' metrics.json
```

**Example Output:**
```json
{
  "name": "affine_qmm_t",
  "registers": 64,
  "occupancy": 0.72
}
{
  "name": "rope_single_freqs",
  "registers": 32,
  "occupancy": 0.89
}
```

**Analysis:**
- Lower registers correlate with higher occupancy
- Both shaders perform well (occupancy >70%)

## References

- [Metal Shading Language Specification](https://developer.apple.com/metal/Metal-Shading-Language-Specification.pdf) - Section on register allocation
- [Metal Performance Optimization Guide](https://developer.apple.com/documentation/metal/optimizing_performance_with_the_metal_debugger) - Occupancy and register pressure
- [SHADER_SOURCE_ATTRIBUTION.md](./SHADER_SOURCE_ATTRIBUTION.md) - Line-by-line shader analysis
- [PROFILING_DATA_RECREATION_GUIDE.md](./PROFILING_DATA_RECREATION_GUIDE.md) - Complete profiling workflows
- [Apple GPU Architecture Overview](https://developer.apple.com/documentation/metal/gpu_families) - Register file characteristics

## Appendix: Metal Shader Examples

### Example A: Minimal Register Usage

```metal
// File: minimal_registers.metal
// Expected: 4-8 registers

kernel void copy_buffer(
    device const float* input [[buffer(0)]],
    device float* output [[buffer(1)]],
    uint gid [[thread_position_in_grid]])
{
    output[gid] = input[gid];
}
```

**Analysis:**
- 1 register for `gid`
- 2 registers for address calculation
- 1 register for loaded value
- **Total: ~4 registers**

### Example B: Typical Register Usage

```metal
// File: typical_registers.metal
// Expected: 32-48 registers

kernel void apply_transform(
    device const float4* input [[buffer(0)]],
    device float4* output [[buffer(1)]],
    constant float4x4& transform [[buffer(2)]],
    uint gid [[thread_position_in_grid]])
{
    float4 pos = input[gid];
    float4 transformed = transform * pos;
    float3 normalized = normalize(transformed.xyz);
    float magnitude = length(normalized);
    output[gid] = float4(normalized * magnitude, 1.0);
}
```

**Analysis:**
- 4 registers for `transform` matrix (16 floats reused)
- 4 registers for `pos`
- 4 registers for `transformed`
- 3 registers for `normalized`
- 1 register for `magnitude`
- Additional temporaries for math operations
- **Total: ~32 registers**

### Example C: High Register Usage (Needs Optimization)

```metal
// File: high_registers.metal
// Expected: 128+ registers (problematic)

kernel void complex_shader(
    device const float* input [[buffer(0)]],
    device float* output [[buffer(1)]],
    uint gid [[thread_position_in_grid]])
{
    // Too many local variables kept alive simultaneously
    float data[50];  // 50 registers

    for (int i = 0; i < 50; i++) {
        data[i] = input[gid * 50 + i];
    }

    // Complex computation keeping all data alive
    for (int i = 0; i < 50; i++) {
        data[i] = sin(data[i]) * cos(data[i]);
    }

    float result = 0;
    for (int i = 0; i < 50; i++) {
        result += data[i];
    }

    output[gid] = result;
}
```

**Analysis:**
- 50 registers for `data` array
- 10-20 registers for loop variables and temporaries
- 20-30 registers for `sin`/`cos` intermediate values
- **Total: ~128+ registers**
- **Problem:** Array kept in registers throughout function

**Optimized Version:**

```metal
kernel void optimized_shader(
    device const float* input [[buffer(0)]],
    device float* output [[buffer(1)]],
    threadgroup float* shared_data [[threadgroup(0)]],
    uint gid [[thread_position_in_grid]],
    uint tid [[thread_position_in_threadgroup]])
{
    // Use shared memory instead of registers
    float result = 0;

    for (int i = 0; i < 50; i++) {
        float value = input[gid * 50 + i];
        value = sin(value) * cos(value);
        result += value;
        // Value goes out of scope immediately (registers freed)
    }

    output[gid] = result;
}
```

**Optimized Analysis:**
- 1 register for `value` (reused in loop)
- 1 register for `result`
- 10-15 registers for loop and math temporaries
- **Total: ~16 registers**
- **Improvement:** 8x reduction in register usage!

## Troubleshooting Checklist

When analyzing register allocation issues:

- [ ] Captured trace with performance counters (`MTL_CAPTURE_COUNTERS=1`)
- [ ] Verified counter data available (`gputrace perfcounters trace.gputrace`)
- [ ] Identified hot shaders (>20% GPU time)
- [ ] Checked register counts (target <64 for balanced workloads)
- [ ] Verified no register spilling (Spilled Bytes = 0)
- [ ] Reviewed shader source for optimization opportunities
- [ ] Applied optimizations (reduced locals, shared memory, kernel splitting)
- [ ] Re-captured and compared before/after
- [ ] Measured actual performance impact (not just register reduction)
- [ ] Cross-referenced with occupancy metrics

---

**Last Updated:** 2025-11-03
**gputrace Version:** Latest
**Bead:** gputrace-50 ✅
