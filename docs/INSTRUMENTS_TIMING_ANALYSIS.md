# How Xcode Instruments Derives Shader Cost Percentages

**Investigation Date**: 2025-11-03
**Related Beads**: gputrace-35, gputrace-36
**Trace Analyzed**: `/tmp/fast-llm-mlx-test.gputrace`

## Summary

Xcode Instruments derives accurate shader cost percentages (e.g., "61.40% steel_gemm, 2.24% block_softmax") from `.gputrace` files through a **multi-stage replay and profiling process**. The percentages are NOT directly stored in the trace files but are **computed on-demand** by replaying the captured workload with GPU performance counters enabled.

## Key Findings

### 1. Instruments Uses Replay-Based Profiling

When you open a `.gputrace` file in Instruments, it:

1. **Replays the captured GPU workload** using `GTUSCSamplingStreamingManagerHelper`
2. **Enables hardware performance counters** during replay via `AGXGPURawCounterSource`
3. **Collects APS (Apple Performance Shaders) sampling data** in real-time
4. **Computes derived counters** from the raw counter data
5. **Calculates cost percentages** based on actual execution time measured during replay

### 2. The Replay Process

From profiling data analysis (see session 725E):

```
GTUSCSamplingStreamingManagerHelper::ReplayForDerivedCounters()
├─ Replays each encoder with performance counters enabled
├─ AGXGPURawCounterSource::setOptions() - Configure counters
├─ AGXGPURawCounterImpl::startSampling() - Begin sampling
├─ [Execute GPU workload]
├─ AGXGPURawCounterImpl::stopSampling() - End sampling
└─ GTMutableShaderProfilerStreamData::addAPSData() - Store results
```

**Key Functions**:
- `GTUSCSamplingStreamingManagerHelper::StreamAPS()` (270ms, 23.2% of processing time)
- `GTUSCSamplingStreamingManagerHelper::StreamEncoderDerivedCounterData()` (220ms, 18.9%)
- `GTUSCSamplingStreamingManagerHelper::ReplayForDerivedCounters()` (178ms)

### 3. Data Sources

#### Files Read by Instruments

1. **`capture`** - Contains:
   - Command buffer structure (CUUU records)
   - Encoder definitions (Cul, Culul records)
   - **CS (Command Submission) records** with encoder metadata
   - Dispatch configurations
   - Resource bindings

2. **`device-resources-*`** - Contains:
   - Buffer definitions and sizes
   - Texture metadata
   - Pipeline state information

3. **Shader files** (hex UUIDs like `FE52ED69B41ABB45`):
   - Compiled Metal shader bytecode
   - Used during replay to re-execute kernels

4. **`metadata`** - Contains:
   - Device information
   - Capture session metadata
   - Library versions

5. **`store0`** (if present):
   - May contain cached profiling data
   - In our test case: decompresses to all zeros (no pre-computed timing)

#### Performance Counter Sources

Instruments accesses GPU performance counters through:
- `AGXGPURawCounterSource` - Apple GPU (AGX) raw counter interface
- `IOReportCopyFilteredChannels` - IOKit reporting framework
- `IOReportCopyChannelsForDriver` - Driver-specific counter channels

Counter types collected:
- GPU cycle counts
- Shader execution time
- Memory bandwidth utilization
- Occupancy metrics
- Cache hit rates

### 4. CS (Command Submission) Records

The capture file contains **CS records** at regular intervals:

```
Hex Pattern: 04 00 00 00 43 53 00 00 [address] [kernel_name...]
             ^^^^^^^^^^^ ^^^^^^^^^^^
             length=4    "CS" magic
```

**Example from trace**:
```
00004fd0  43 53 00 00 00 5e c4 74  0a 00 00 00 76 73 5f 4d
          C  S        ^^^^^^^^^^^              v  s  _  M
                      address                  kernel name
```

These CS records mark encoder boundaries and associate them with:
- Pipeline state addresses
- Kernel names
- Execution metadata

During replay, Instruments uses these records to:
1. Identify which shader to execute
2. Set up performance counters for that specific encoder
3. Measure actual execution time
4. Attribute timing to the correct shader

## How Cost Percentages Are Calculated

### Formula

```
Shader Cost % = (Shader GPU Time / Total GPU Time) × 100
```

Where:
- **Shader GPU Time** = Measured during replay with `AGXGPURawCounterSource`
- **Total GPU Time** = Sum of all shader execution times during replay

### Example from Test Trace

If Instruments shows:
- `steel_gemm_fused_nn_float32`: 61.40%
- `block_softmax_float32`: 2.24%

This means:
1. During replay, `steel_gemm` took 61.40% of total measured GPU cycles
2. `block_softmax` took 2.24% of total measured GPU cycles
3. These are **actual measurements**, not estimates

## Why Our Current Approach Doesn't Work

### Problem 1: No Pre-Computed Timing

The `.gputrace` file does **NOT** contain pre-computed timing data in a directly readable format:
- `store0` file: Empty (all zeros after decompression)
- CS records: Contain metadata but no duration fields
- CUUU records: Have timestamps but not per-shader timing

### Problem 2: Replay Requires GPU Access

To replicate Instruments' methodology, we would need to:
1. Parse the capture file to extract command buffers
2. Reconstruct Metal command buffers from the trace
3. Re-execute them on an Apple GPU
4. Enable performance counters during execution
5. Collect timing data

This is complex because:
- Requires Metal runtime and GPU hardware
- Need access to `AGXGPURawCounterSource` (private API)
- Must handle resource state reconstruction
- Replay may not be deterministic

### Problem 3: API Availability

**Good News**: Metal has **PUBLIC APIs** for performance counter sampling!

**Public APIs (Available since macOS 10.15 / iOS 14.0)**:
- `MTLCounterSampleBuffer` - Counter sample buffer protocol
- `MTLCounterSet` - Collection of counters to sample
- `MTLCounter` - Individual counter descriptor
- `MTLCommonCounter` constants:
  - `MTLCommonCounterTimestamp` - GPU timestamp
  - `MTLCommonCounterTotalCycles` - Total GPU cycles
  - `MTLCommonCounterComputeKernelInvocations` - Kernel invocation count
  - `MTLCommonCounterFragmentCycles` - Fragment shader cycles
  - `MTLCommonCounterVertexCycles` - Vertex shader cycles
- `MTLDevice.counterSets` - Available counter sets on device
- `MTLDevice.newCounterSampleBufferWithDescriptor()` - Create sample buffer
- `MTLDevice.supportsCounterSampling()` - Check sampling support

**Counter Sampling Points (Public)**:
- `MTLCounterSamplingPointAtStageBoundary` - Sample at stage boundaries
- `MTLCounterSamplingPointAtDrawBoundary` - Sample at draw calls
- `MTLCounterSamplingPointAtDispatchBoundary` - **Sample at compute dispatches** ← Key for us!
- `MTLCounterSamplingPointAtTileDispatchBoundary` - Sample at tile dispatches
- `MTLCounterSamplingPointAtBlitBoundary` - Sample at blit operations

**Private APIs (Used by Instruments)**:
- `AGXGPURawCounterSource` - Lower-level AGX GPU counter access
- `GTUSCSamplingStreamingManagerHelper` - Replay orchestration
- `GTMutableShaderProfilerStreamData` - Data storage format

**Conclusion**: We CAN implement accurate timing using public Metal APIs!

## Alternative Approaches

### Approach 1: Heuristic Estimation (Current)

Our current code estimates timing based on:
- Thread configuration (threadgroups × threads per group)
- Dispatch counts
- Kernel name patterns

**Pros**: Works without GPU replay
**Cons**: Inaccurate, doesn't match Instruments

### Approach 2: Parse Existing Profiling Data

If we run Instruments and export data, we can:
1. Export shader costs to CSV/text
2. Parse the exported data
3. Correlate with trace file encoders

**Pros**: Gets accurate Instruments data
**Cons**: Requires running Instruments manually

### Approach 3: Implement Lightweight Replay with Public Metal APIs ⭐ **RECOMMENDED**

Create a replay engine that:
1. Parses command buffers from capture file
2. Reconstructs and re-executes Metal commands
3. Uses **public Metal counter APIs** (`MTLCounterSampleBuffer` with `MTLCounterSamplingPointAtDispatchBoundary`)
4. Measures actual execution time per kernel

**Example Workflow**:
```objc
// 1. Get available counter sets
NSArray<id<MTLCounterSet>>* counterSets = device.counterSets;
id<MTLCounterSet> timestampSet = /* find MTLCommonCounterSetTimestamp */;

// 2. Create counter sample buffer
MTLCounterSampleBufferDescriptor* desc = [[MTLCounterSampleBufferDescriptor alloc] init];
desc.counterSet = timestampSet;
desc.sampleCount = numDispatches * 2;  // start + end for each dispatch
desc.storageMode = MTLStorageModeShared;

id<MTLCounterSampleBuffer> sampleBuffer = [device newCounterSampleBufferWithDescriptor:desc error:&error];

// 3. Sample at dispatch boundaries
[computeEncoder sampleCountersInBuffer:sampleBuffer
                             atIndex:dispatchIndex * 2
                         withBarrier:YES];  // Start sample

[computeEncoder dispatchThreadgroups:...];

[computeEncoder sampleCountersInBuffer:sampleBuffer
                             atIndex:dispatchIndex * 2 + 1
                         withBarrier:YES];  // End sample

// 4. Resolve counter data
NSData* data = [sampleBuffer resolveCounterRange:NSMakeRange(0, numDispatches * 2)];
MTLCounterResultTimestamp* timestamps = (MTLCounterResultTimestamp*)data.bytes;

// 5. Calculate duration
uint64_t duration = timestamps[dispatchIndex * 2 + 1].timestamp -
                   timestamps[dispatchIndex * 2].timestamp;
```

**Pros**:
- Gets accurate timing matching Instruments
- Uses only public APIs (macOS 10.15+)
- Can measure per-kernel execution time
- No reverse engineering needed

**Cons**:
- Complex implementation, requires GPU
- Need to reconstruct full command buffer state
- Must handle resource dependencies

### Approach 4: Reverse Engineer Counter Data Format

Investigate if Instruments caches counter data anywhere:
- Check for `.gpuprofiler_raw` files
- Look for undocumented cache directories
- Analyze Instruments' temporary files during operation

**Pros**: May find pre-computed data
**Cons**: Uncertain if such data exists

## Recommended Next Steps

1. **Implement Approach 3 (Lightweight Replay)**:
   - Use `MTLCaptureManager` to profile replay
   - Use public Metal APIs where possible
   - Start with simple command buffer reconstruction

2. **Investigate Counter Data Formats**:
   - Monitor Instruments' file I/O with `fs_usage`
   - Check for any profiling data caches
   - Document any additional data formats found

3. **Create Export Parser (Approach 2)**:
   - Automate Instruments export
   - Parse CSV/text output
   - Integrate with existing tooling

## Code Changes Needed

### High Priority
- [ ] Parse CS records from capture file (`mtsp_records.go`)
- [ ] Implement command buffer reconstruction (`replay.go`)
- [ ] Add Metal replay with profiling (`metal_replay.go`)

### Medium Priority
- [ ] Export Instruments data programmatically
- [ ] Parse Instruments CSV output (`shader_costs.go`)

### Low Priority
- [ ] Investigate `AGXGPURawCounterSource` usage
- [ ] Explore private API alternatives

## References

### Apple Frameworks
- `GPUToolsCore.framework` - Core GPU profiling infrastructure
- `GPUDebugger.ideplugin` - Instruments integration
- `GPUToolsPlatform.framework` - Platform-specific GPU tools
- `Metal.framework` - Metal API

### Key Classes
- `GTUSCSamplingStreamingManagerHelper` - Replay coordination
- `AGXGPURawCounterSource` - GPU counter access
- `GTMutableShaderProfilerStreamData` - Profiling data storage
- `DYMTLEncoderInfo` - Encoder information structure

### Profiling Session Analysis
- Session 725E shows Instruments replay process
- Key timing: 270ms for APS streaming, 220ms for derived counters
- Total replay overhead: ~500ms for profiling data collection

## Conclusion

**Xcode Instruments derives shader cost percentages by replaying the captured GPU workload with performance counters enabled.** The percentages are not stored in the `.gputrace` file but are computed on-demand during trace analysis.

### Key Findings Summary

1. **Timing data is NOT pre-computed** in `.gputrace` files
2. **Instruments replays the workload** with GPU performance counters
3. **Public Metal APIs exist** for counter sampling (`MTLCounterSampleBuffer`, available since macOS 10.15)
4. **We can implement accurate profiling** using only public APIs

### Recommended Implementation Path

1. **Phase 1**: Parse CS records and command buffer structure (in progress)
2. **Phase 2**: Implement Metal replay engine using public counter APIs
3. **Phase 3**: Generate accurate timing profiles matching Instruments

### Why This Approach Works

The public `MTLCounterSampleBuffer` API with `MTLCounterSamplingPointAtDispatchBoundary` provides:
- Per-kernel GPU timestamp collection
- Hardware cycle counters
- Kernel invocation counts
- All data needed to calculate accurate cost percentages

This is the **same mechanism Instruments uses** (though Instruments also uses lower-level `AGXGPURawCounterSource` for additional counters).

### Next Steps

1. ✅ **Parse CS records from capture file** (gputrace-36)
2. **Implement command buffer reconstruction** (gputrace-41)
3. **Create Metal replay with counter sampling** (gputrace-41)
4. **Validate against Instruments output** (new bead needed)

## Public Metal Counter APIs Reference

### Available Since macOS 10.15 / iOS 14.0

All of the following are **public, documented APIs** available to all developers:

#### Headers

```objc
#import <Metal/MTLCounters.h>
#import <Metal/MTLDevice.h>
#import <Metal/MTLCommandBuffer.h>
#import <Metal/MTLComputeCommandEncoder.h>
```

#### Counter Sets

```objc
// Query available counter sets on device
NSArray<id<MTLCounterSet>>* counterSets = device.counterSets;

// Common counter set names (public constants)
MTLCommonCounterSetTimestamp           // Timestamp-only counters
MTLCommonCounterSetStageUtilization    // Per-stage GPU utilization
MTLCommonCounterSetStatistic           // Invocation statistics
```

#### Individual Counters

```objc
// Public counter names
MTLCommonCounterTimestamp                      // GPU timestamp
MTLCommonCounterTotalCycles                    // Total GPU cycles
MTLCommonCounterComputeKernelInvocations       // Kernel invocations
MTLCommonCounterFragmentCycles                 // Fragment shader cycles
MTLCommonCounterVertexCycles                   // Vertex shader cycles
// ... and more (see MTLCounters.h for complete list)
```

#### Sampling Points

```objc
// Check if device supports counter sampling at dispatch boundaries
if ([device supportsCounterSampling:MTLCounterSamplingPointAtDispatchBoundary]) {
    // Sampling supported - we can collect per-kernel timing
}

// Available sampling points:
MTLCounterSamplingPointAtStageBoundary         // At stage boundaries
MTLCounterSamplingPointAtDrawBoundary          // At draw calls
MTLCounterSamplingPointAtDispatchBoundary      // At compute dispatches ⭐
MTLCounterSamplingPointAtTileDispatchBoundary  // At tile dispatches
MTLCounterSamplingPointAtBlitBoundary          // At blit operations
```

#### Creating Counter Sample Buffers

```objc
// Create descriptor
MTLCounterSampleBufferDescriptor* desc = [[MTLCounterSampleBufferDescriptor alloc] init];
desc.counterSet = timestampCounterSet;
desc.sampleCount = 100;  // Number of samples to allocate
desc.storageMode = MTLStorageModeShared;  // Shared for CPU access
desc.label = @"GPU Profiling Counters";

// Create sample buffer
NSError* error = nil;
id<MTLCounterSampleBuffer> sampleBuffer =
    [device newCounterSampleBufferWithDescriptor:desc error:&error];
```

#### Sampling During Execution

```objc
// In compute command encoder
[computeEncoder sampleCountersInBuffer:sampleBuffer
                             atIndex:sampleIndex
                         withBarrier:YES];  // Barrier ensures accurate timing
```

#### Resolving Results

```objc
// Resolve counter samples to CPU-readable format
NSData* results = [sampleBuffer resolveCounterRange:NSMakeRange(0, actualSamples)];

// Cast to appropriate result structure
MTLCounterResultTimestamp* timestamps = (MTLCounterResultTimestamp*)results.bytes;

// Access individual samples
for (NSUInteger i = 0; i < actualSamples; i++) {
    uint64_t gpuTimestamp = timestamps[i].timestamp;
    // Process timestamp...
}
```

#### Result Structures

```objc
// Timestamp results
typedef struct {
    uint64_t timestamp;
} MTLCounterResultTimestamp;

// Stage utilization results
typedef struct {
    uint64_t totalCycles;
    uint64_t vertexCycles;
    uint64_t fragmentCycles;
    uint64_t renderTargetCycles;
    // ... more fields
} MTLCounterResultStageUtilization;

// Statistics results
typedef struct {
    uint64_t computeKernelInvocations;
    uint64_t vertexInvocations;
    uint64_t fragmentInvocations;
    // ... more fields
} MTLCounterResultStatistic;
```

### Documentation Links

- [Metal Performance Tuning](https://developer.apple.com/documentation/metal/performance_tuning)
- [MTLCounterSampleBuffer Protocol](https://developer.apple.com/documentation/metal/mtlcountersamplebuffer)
- [MTLCounterSet Protocol](https://developer.apple.com/documentation/metal/mtlcounterset)
- [GPU Profiling with Metal](https://developer.apple.com/documentation/metal/debugging_tools)

### Example: Complete Profiling Workflow

```objc
// 1. Setup
id<MTLDevice> device = MTLCreateSystemDefaultDevice();
id<MTLCommandQueue> queue = [device newCommandQueue];

// Find timestamp counter set
id<MTLCounterSet> timestampSet = nil;
for (id<MTLCounterSet> set in device.counterSets) {
    if ([set.name isEqualToString:MTLCommonCounterSetTimestamp]) {
        timestampSet = set;
        break;
    }
}

// 2. Create sample buffer
MTLCounterSampleBufferDescriptor* desc = [[MTLCounterSampleBufferDescriptor alloc] init];
desc.counterSet = timestampSet;
desc.sampleCount = numKernels * 2;  // start + end for each kernel
desc.storageMode = MTLStorageModeShared;

id<MTLCounterSampleBuffer> sampleBuffer =
    [device newCounterSampleBufferWithDescriptor:desc error:nil];

// 3. Execute with sampling
id<MTLCommandBuffer> commandBuffer = [queue commandBuffer];
id<MTLComputeCommandEncoder> encoder = [commandBuffer computeCommandEncoder];

for (NSUInteger i = 0; i < numKernels; i++) {
    // Sample before dispatch
    [encoder sampleCountersInBuffer:sampleBuffer
                          atIndex:i * 2
                      withBarrier:YES];

    // Execute kernel
    [encoder setComputePipelineState:pso];
    [encoder dispatchThreadgroups:...];

    // Sample after dispatch
    [encoder sampleCountersInBuffer:sampleBuffer
                          atIndex:i * 2 + 1
                      withBarrier:YES];
}

[encoder endEncoding];
[commandBuffer commit];
[commandBuffer waitUntilCompleted];

// 4. Resolve and analyze
NSData* data = [sampleBuffer resolveCounterRange:NSMakeRange(0, numKernels * 2)];
MTLCounterResultTimestamp* timestamps = (MTLCounterResultTimestamp*)data.bytes;

for (NSUInteger i = 0; i < numKernels; i++) {
    uint64_t startTime = timestamps[i * 2].timestamp;
    uint64_t endTime = timestamps[i * 2 + 1].timestamp;
    uint64_t duration = endTime - startTime;

    NSLog(@"Kernel %lu: %llu GPU ticks", (unsigned long)i, duration);
}
```

This is the **exact mechanism** we can use to get Instruments-quality timing data using only public APIs!
