# Xcode Counters.csv Format Analysis

**Bead:** gputrace-23
**Priority:** P2
**Status:** Investigation Complete - Requires Binary Format Reverse Engineering
**Date:** 2025-11-03

## Overview

Xcode Instruments exports GPU performance data in a `Counters.csv` file with 246 columns of detailed hardware metrics. This document analyzes the format and outlines what's required to match it.

## CSV Structure

### Header Columns

The CSV has the following structure:

1. **Index** - Sequential row number
2. **Encoder FunctionIndex** - Index of the encoder function
3. **CommandBuffer Label** - Label of the command buffer (e.g., "Command Buffer 2 0xa74c48380")
4. **Encoder Label** - Label of the encoder (e.g., "Compute Encoder 0 0xa74c3c960")
5. **Empty Column** - (possibly reserved)
6-246. **Performance Metrics** - 241 hardware performance counter metrics

### Sample Header (First 20 columns):
```
Index,Encoder FunctionIndex,CommandBuffer Label,Encoder Label,,
"1D Texture Array Sampler Calls",
"1D Texture Sampler Calls",
"2D MSAA Texture Sampler Calls",
"2D Texture Array Sampler Calls",
"2D Texture Sampler Calls",
"2X MSAA Resolved Pixels Stored",
"3D Texture Sampler Calls",
"4X MSAA Resolved Pixels Stored",
"ALU Utilization",
"Anisotropic Sampler Calls",
"Attachment Pixels Stored",
"Average Anisotropic Level",
"Average Pixel Overdraw",
"Average Samples Per Pixel",
"Average Sparse Texture Tile Size",
...
```

## Key Performance Metrics (Mentioned in Requirement)

### Core Metrics:
- **ALU Utilization** - Percentage of ALU units actively executing instructions (0-100%)
- **Buffer L1 Miss Rate** - Percentage of buffer reads that miss L1 cache
- **Buffer Device Memory Bytes Read** - Bytes read from device memory for buffers
- **Buffer Device Memory Bytes Written** - Bytes written to device memory for buffers
- **Kernel Occupancy** - Percentage of GPU occupancy for compute kernels (0-100%)
- **Texture Cache Miss Rate** - Percentage of texture reads that miss cache

### Kernel-Specific Metrics:
- Kernel ALU Float Instructions
- Kernel ALU Half Instructions
- Kernel ALU Instructions (total)
- Kernel ALU Integer and Complex Instructions
- Kernel ALU Integer and Conditional Instructions
- Kernel ALU Performance
- Kernel Invocations
- Kernel Occupancy
- Kernel Texture Cache Miss Rate

### Memory Metrics:
- Bytes Read From Device Memory
- Bytes Written To Device Memory
- L1 Read/Write Bandwidth
- Last Level Cache Bytes Read/Written
- Texture Device Memory Bytes Read/Written
- Depth Texture Device Memory Bytes Read/Written

### Fragment/Vertex Shader Metrics (Prefixed):
- FS/VS Buffer Device Memory Bytes Read/Written
- FS/VS Bytes Read From Device Memory
- FS/VS Device Atomic Bytes Read/Written
- FS/VS Last Level Cache Bytes Read/Written
- FS/VS Texture Cache Miss Rate
- FS/VS Occupancy
- FS/VS ALU Instructions (Float/Half/Integer)

### Pipeline Metrics:
- Fragment Shader Launch Limiter/Utilization
- Vertex Shader Launch Limiter/Utilization
- Compute Shader Launch Limiter/Utilization
- Texture Filtering Limiter/Utilization
- L1 Cache Limiter/Utilization
- Last Level Cache Limiter/Utilization

## Sample Data Row

```csv
1,77,Command Buffer 2 0xa74c48380,Compute Encoder 0 0xa74c3c960,,
0.00,0.00,0.00,0.00,0.00,0.00,0.00,0.00,
0.98,    # ALU Utilization = 98%
0.00,0.00,0.00,0.00,0.00,0.00,0.00,0.00,
0.00,0.00,
25.15,   # Buffer Device Memory Bytes Read (in some unit)
19.95,   # Buffer Device Memory Bytes Written
10.57,   # Buffer L1 Miss Rate = 10.57%
...
```

## Data Source: .gpuprofiler_raw Files

The metrics in Counters.csv come from hardware performance counter data stored in `.gpuprofiler_raw` directories.

### File Structure:
```
trace.gputrace.gpuprofiler_raw/
├── Counters_f_0.raw      # Frame 0 counters (binary)
├── Counters_f_1.raw      # Frame 1 counters (binary)
├── ...
└── metadata.plist        # Metadata about counter collection
```

### Counter File Format (.raw files):

**Known Structure:**
- Records start with `0x4E 0x00 0x00 0x00` marker
- Each record contains binary performance counter data
- Variable-length records
- Contains data for one or more shader invocations

**Unknown Details** (Reverse Engineering Required):
- Exact field offsets for each of 246 metrics
- Record types and their field layouts
- How metrics are encoded (uint32, float, etc.)
- Relationship between records and shader invocations
- Mapping from record data to metric names

## Current Implementation Status

### What We Have:
✅ Record boundary detection (0x4E markers)
✅ Record counting
✅ Basic file parsing infrastructure
✅ Understanding that data comes from APS (Apple Performance Streaming)

### What We Need:
❌ Field-level binary format specification for all 246 metrics
❌ Mapping of record offsets to metric names
❌ Understanding of different record types
❌ Handling of per-encoder vs per-shader metrics
❌ CSV export matching exact Xcode column order

## Technical Challenges

### Challenge 1: Binary Format Reverse Engineering

The `.raw` files are undocumented binary formats. Options:

1. **Hexdump Analysis**: Compare counter files with known shader workloads
2. **Instruments Inspection**: Use Instruments to profile while capturing counters
3. **API Hooking**: Hook APS/AGXGPURawCounter APIs to observe data flow
4. **Correlation Analysis**: Run known workloads and correlate raw bytes with Counters.csv values

### Challenge 2: Scale (246 Metrics)

Even with format knowledge, implementing 246 metrics is substantial:
- 241 performance counter fields to extract
- Different field layouts for different shader stages (VS/FS/Compute)
- Proper units and scaling for each metric
- Validation against Xcode's output

### Challenge 3: Encoder Attribution

Metrics must be attributed to:
- Specific command buffers (by address/label)
- Specific encoders (Compute/Vertex/Fragment)
- Specific shader functions (kernel names)
- Specific invocations (for aggregation)

## Proposed Phased Implementation

### Phase 1: Core Metrics Extraction (Feasible Now)
**Goal**: Extract 10-15 most important metrics
**Metrics**:
- ALU Utilization
- Kernel Occupancy
- Buffer Device Memory Bytes Read/Written
- Kernel Invocations
- Texture Cache Miss Rate
- L1 Cache Miss Rate

**Approach**:
1. Create reference traces with known characteristics
2. Analyze `.raw` files with hexdump
3. Correlate with Counters.csv values
4. Identify field offsets for core metrics
5. Implement extraction in `perfcounters.go`

**Estimated Effort**: 2-3 days

### Phase 2: Extended Metrics (Medium Term)
**Goal**: Add shader-stage-specific metrics
**Metrics**:
- All Kernel-prefixed metrics (9 metrics)
- Memory bandwidth metrics (8 metrics)
- L1/LLC metrics (15 metrics)

**Approach**:
1. Build on Phase 1 knowledge
2. Identify record type markers
3. Handle different shader stages
4. Add metrics incrementally with validation

**Estimated Effort**: 3-5 days

### Phase 3: Complete Format Match (Long Term)
**Goal**: Match all 246 columns exactly
**Approach**:
1. Systematic field-by-field extraction
2. Comprehensive test suite
3. Diff against Xcode output

**Estimated Effort**: 1-2 weeks

### Phase 4: CSV Export (Final Step)
**Goal**: Generate Counters.csv matching Xcode format exactly

**Requirements**:
- All 246 metrics extracted
- Proper encoder/command buffer attribution
- Exact column order match
- Proper numeric formatting
- Header row match

**Estimated Effort**: 2-3 days

## Alternative Approaches

### Option A: Metal Replay with MTLCounterSampleBuffer (Recommended)

Instead of parsing binary files, replay the GPU trace and collect metrics using Metal's public API.

**Pros**:
- Uses documented public API
- Gets real-time hardware data
- Guaranteed accuracy
- No reverse engineering needed

**Cons**:
- Requires implementing full replay engine
- Slower than file parsing
- Requires Metal-capable GPU
- More complex implementation

**Estimated Effort**: 2-3 weeks for replay engine + metric collection

**See**: `docs/PROFILING_DATA_RECREATION_GUIDE.md` for replay implementation details

### Option B: Hybrid Approach

1. Parse basic metrics from `.gpuprofiler_raw` (Phase 1)
2. Use replay for advanced metrics (future)
3. Allow both modes: `--fast` (parse only) vs `--accurate` (replay)

## References

- [GPU_PROFILING_APIS_DISCOVERED.md](../GPU_PROFILING_APIS_DISCOVERED.md) - APS and AGXGPURawCounter APIs
- [PROFILING_DATA_RECREATION_GUIDE.md](./PROFILING_DATA_RECREATION_GUIDE.md) - Replay-based profiling
- [INSTRUMENTS_TIMING_ANALYSIS.md](./INSTRUMENTS_TIMING_ANALYSIS.md) - How Instruments collects data
- Metal Performance Counters: https://developer.apple.com/documentation/metal/performance_tuning

## Recommendation

**For gputrace-23 (P2 Priority)**:

Given the scope and complexity, I recommend:

1. **Implement Phase 1** (core metrics) - This provides immediate value
2. **Document format discoveries** as we learn more about the binary structure
3. **Defer full 246-column support** until we have:
   - Complete binary format specification, OR
   - Working replay engine with MTLCounterSampleBuffer

This gives users the most important metrics quickly while leaving room for expansion.

## Next Steps

1. Create test traces with known characteristics (simple compute kernels)
2. Capture both `.gputrace` and `Counters.csv` for correlation
3. Hexdump analysis to find patterns
4. Document field offsets as discovered
5. Implement incremental extraction in `perfcounters.go`
6. Add CSV export when sufficient metrics available

## Status Update for Bead gputrace-23

**Current Status**: Investigation complete, binary format reverse engineering required

**Blocking Issues**:
- Undocumented binary format in `.gpuprofiler_raw/*.raw` files
- 246 metrics is substantial scope (weeks of work)
- No sample `.gpuprofiler_raw` files currently available for analysis

**Recommended Next Action**:
- Capture test trace with GPU profiler enabled
- Begin Phase 1 implementation (10-15 core metrics)
- Consider Metal replay approach as alternative (Option A)

**Priority Assessment**:
- P2 is appropriate - This is valuable but not blocking
- Consider promoting to P1 if replay engine becomes priority
- Core metrics (Phase 1) should be sufficient for most use cases
