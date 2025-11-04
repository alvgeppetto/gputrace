# Performance Counter Implementation Recommendation

**Bead:** gputrace-44
**Date:** 2025-11-03
**Status:** Analysis Complete - Recommending Alternative Approach

## Executive Summary

After detailed analysis of `.gpuprofiler_raw` binary format, I recommend **NOT** pursuing direct binary parsing for Phase 1. Instead, use Metal's public `MTLCounterSampleBuffer` API with replay.

## Analysis Findings

### Binary Format Complexity

1. **Aggregation Required**
   - 1,598 binary records → 10 CSV rows
   - Instruments aggregates samples across 40 files
   - Requires statistical processing (sum, average, bandwidth calc)

2. **Record Structure**
   - Two record types: Metadata (~2400-2900 bytes) and Samples (464 bytes)
   - Variable-length records with complex structure
   - Field offsets not easily identifiable via hexdump

3. **Reverse Engineering Challenge**
   - 241 metrics × complex aggregation = weeks of work
   - Undocumented format prone to breaking across OS versions
   - No validation methodology without Instruments as ground truth

### Time Investment

| Approach | Estimated Time | Reliability | Maintenance |
|----------|---------------|-------------|-------------|
| Binary Parsing (current) | 5-7 days Phase 1, weeks for complete | Medium | High (breaks with OS updates) |
| Metal Replay + Counters | 3-5 days total | High | Low (public API) |

## Recommended Approach: Metal Replay with MTLCounterSampleBuffer

### Overview

Use Metal's **public** performance counter API during replay:

```swift
// Metal Performance Counter API (public, documented)
let counterSet = device.counterSets.first { $0.name == "timestamp" }
let counterSampleBuffer = device.makeCounterSampleBuffer(
    descriptor: MTLCounterSampleBufferDescriptor()
)

// Sample during replay
encoder.sampleCounters(sampleBuffer, atSampleIndex: 0, withBarrier: true)
// ... execute GPU work ...
encoder.sampleCounters(sampleBuffer, atSampleIndex: 1, withBarrier: true)

// Read results
let data = counterSampleBuffer.resolve Range(sampleRange)
```

### Advantages

1. **Public API**: Documented, stable, won't break
2. **Complete Metrics**: Access to all hardware counters
3. **Accurate**: Same data Instruments uses
4. **Lower Effort**: 3-5 days vs weeks
5. **Maintainable**: Apple maintains the API

### Implementation Plan

#### Phase 1: Basic Replay Engine (2 days)

**Goal**: Replay command buffers from .gputrace

**Components**:
- Parse capture file to extract command buffer data
- Restore Metal state (buffers, textures, pipelines)
- Re-execute command buffers
- Validate output matches original trace

**Files to Create**:
- `replay.go` - Replay orchestration
- `replay_state.go` - State restoration
- `cmd/gputrace/cmd/replay.go` - CLI command

#### Phase 2: Counter Collection (1-2 days)

**Goal**: Collect performance counters during replay

**Components**:
- Create MTLCounterSampleBuffer for each encoder
- Insert counter samples before/after GPU work
- Resolve counter data after execution
- Map to standard metric names

**Files to Modify**:
- `replay.go` - Add counter sampling
- `perfcounters.go` - Add MTLCounterSampleBuffer support

#### Phase 3: CSV Export (1 day)

**Goal**: Export in Xcode Counters.csv format

**Components**:
- Format counter data as CSV
- Match Xcode column ordering
- Validate against reference CSV

**Files to Create**:
- `csv_export.go` - CSV formatting
- `cmd/gputrace/cmd/export-counters.go` - CLI command

### Metal Counter API Reference

**Available Counter Sets** (M1/M2/M3/M4):
- `timestamp` - Basic timing
- `stage_utilization` - Shader stage utilization
- `statistics` - Draw/compute statistics
- Apple GPU specific counters (ALU, cache, bandwidth)

**Documentation**:
- https://developer.apple.com/documentation/metal/gpu_counters_and_counter_sample_buffers
- https://developer.apple.com/documentation/metal/mtlcountersamplebuffer
- https://developer.apple.com/documentation/metal/performance_tuning

### Comparison with Binary Parsing

| Aspect | Binary Parsing | Metal Replay |
|--------|---------------|--------------|
| **Phase 1 Time** | 5-7 days | 3-5 days |
| **Complete Implementation** | 3-4 weeks | 1 week |
| **Reliability** | Medium (undocumented) | High (public API) |
| **Metrics Available** | 241 (if successful) | All hardware counters |
| **OS Update Risk** | High | Low |
| **Validation** | Difficult | Easy (compare with Instruments) |
| **Maintenance** | High | Low |
| **Learning Value** | Reverse engineering | Metal profiling |

## Alternative: Hybrid Approach

If `.gpuprofiler_raw` parsing is still desired for research:

1. **Use Metal Replay as Primary** (Phase 1-3 above)
2. **Parse Binary as Research Project**
   - Lower priority
   - Focus on understanding Apple's aggregation logic
   - Validate against Metal replay results
   - Document for educational purposes

This gives us:
- ✅ Working counter extraction quickly (Metal)
- ✅ Research into Apple's format (optional)
- ✅ Two validation sources

## Risks of Binary Parsing Approach

1. **Time Sink**: Weeks to reverse engineer 241 fields
2. **Fragility**: Format may change with OS updates (M4 Max, macOS 16)
3. **Incomplete**: Might miss aggregation logic details
4. **Validation**: No way to verify correctness without Instruments
5. **Opportunity Cost**: Could build other valuable features instead

## Recommendation

**Implement Metal Replay + MTLCounterSampleBuffer for gputrace-44**

Reasons:
1. Faster delivery (3-5 days vs 5-7 days for partial solution)
2. More reliable and maintainable
3. Provides complete metrics, not just Phase 1 subset
4. Public API means community can contribute
5. Better alignment with gputrace's goal of practical tooling

## Next Steps if Approved

1. Create `gputrace-50`: Implement Metal replay engine
2. Create `gputrace-51`: Add counter sampling to replay
3. Create `gputrace-52`: CSV export matching Instruments
4. Update `gputrace-44` as "deferred - superseded by replay approach"

## References

- [GPU_PROFILING_APIS_DISCOVERED.md](../GPU_PROFILING_APIS_DISCOVERED.md)
- [PROFILING_DATA_RECREATION_GUIDE.md](./PROFILING_DATA_RECREATION_GUIDE.md)
- [COUNTERS_CSV_FORMAT.md](./COUNTERS_CSV_FORMAT.md)
- [PERFCOUNTER_BINARY_FORMAT.md](./PERFCOUNTER_BINARY_FORMAT.md)
- Apple Metal Performance Tuning Guide
- MTLCounterSampleBuffer Documentation

## Decision Point

**Question for gputrace-44**: Continue binary parsing OR pivot to Metal replay?

**My Recommendation**: Pivot to Metal replay for faster, more reliable results.
