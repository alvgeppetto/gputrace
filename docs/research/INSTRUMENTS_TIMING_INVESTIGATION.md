# How Xcode Instruments Derives Shader Cost Percentages

**Investigation Date:** 2025-11-03
**Status:** RESOLVED

## Summary

Xcode Instruments calculates shader cost percentages (e.g., "61.40% steel_gemm, 2.24% block_softmax") by **replaying GPU commands and measuring actual execution time**, NOT by reading pre-recorded timing data from .gputrace files.

## Methodology

Used `fs_usage` to monitor filesystem access when opening and profiling a .gputrace file in Xcode Instruments:

```bash
sudo fs_usage -w -f filesys > /tmp/xcode-profile-access.log &
```

Monitored three stages:
1. Opening the .gputrace file in Instruments
2. Clicking "Replay" button
3. Collecting performance data

## Key Findings

### 1. GPUToolsReplayService Process

When collecting performance data, Xcode spawns `GPUToolsReplayService` which:
- Loads shader source code (hex-named files like `93549414277BA042`, `3858D86BD413F4F5`)
- Loads buffer data (`MTLBuffer-*` files)
- Loads heap data (`MTLHeap-*` files)
- Loads metadata and capture files

### 2. GPU Replay Mechanism

The service **re-executes GPU commands** on actual hardware:
- Reconstructs the Metal command buffers from the trace
- Dispatches the shaders to the GPU
- Measures execution time during replay
- Calculates cost percentages from measured timing

### 3. No Pre-Recorded Timing Data

The .gputrace file does NOT contain timing percentages:
- `store0` file is zlib-compressed but contains all zeros (16384 bytes decompressed)
- No timing data in MTSP records (verified by parsing)
- No `.gpuprofiler_raw` directory (would contain hardware performance counters if capture had them enabled)

### 4. File Access Pattern

```
GPUToolsReplayService accesses (per replay iteration):
  - /private/tmp/fast-llm-mlx-test.gputrace/93549414277BA042  (shader source)
  - /private/tmp/fast-llm-mlx-test.gputrace/3858D86BD413F4F5  (shader source)
  - /private/tmp/fast-llm-mlx-test.gputrace/MTLBuffer-100-0   (buffer data)
  - /private/tmp/fast-llm-mlx-test.gputrace/MTLBuffer-92-0    (buffer data)
  - ... (continues for all shaders and buffers)
```

## Implications

### For gputrace Library

1. **Cannot extract timing percentages from .gputrace files alone**
   - The data simply doesn't exist in the file
   - Instruments generates it dynamically

2. **Three approaches to get real timing:**

   **Option A: Implement Replay (Like Instruments)**
   - Load shaders and buffers from .gputrace
   - Reconstruct Metal command buffers
   - Execute on GPU and measure timing
   - Pros: Accurate, matches Instruments
   - Cons: Complex, requires Metal framework

   **Option B: Capture with kdebug/signposts**
   - Use kernel debug events during original capture
   - Parse kdebug codes for GPU timing (see existing `timing_v2.go`)
   - Pros: One-time capture, no replay needed
   - Cons: Requires special capture setup

   **Option C: Hardware Performance Counters**
   - Capture with `.gpuprofiler_raw` enabled
   - Parse counter files for execution statistics
   - Pros: Most detailed data
   - Cons: ~6GB of data, complex format

### For Current Implementation

The `shader_metrics.go` file currently uses **synthetic timing** which is appropriate since:
- Real timing requires replay or special capture
- Synthetic timing enables visualization
- Users understand it's estimated

## Recommendations

1. **Update Documentation**
   - Clarify that Instruments uses GPU replay for timing
   - Explain why our tool uses synthetic/estimated timing
   - Document how to get real timing (options A/B/C above)

2. **Consider Replay Implementation**
   - Could add `gputrace replay` command
   - Would require Metal framework integration
   - Could measure actual GPU execution time

3. **Enhance Existing Timing Extraction**
   - The `timing_v2.go` already supports kdebug/signpost parsing
   - Could improve that approach for users who can modify their capture setup
   - Document how to capture with timing enabled

## Related Files

- `shader_metrics.go` - Current synthetic timing implementation
- `timing_v2.go` - kdebug/signpost timing extraction
- `store0_parser.go` - Attempts to parse store0 (but it's empty)

## References

- Process: `GPUToolsReplayService` (part of Xcode GPU debugging tools)
- Location: `/Applications/Xcode.app/Contents/Developer/...`
- Trace format: `TRACE_FORMAT.md`
- MTSP records: `RECORD_FORMATS.md`
