# GPU Profiling APIs Discovered from GPUToolsReplayService

**Investigation Date:** 2025-11-03
**Method:** Profiled GPUToolsReplayService.xpc with Instruments Time Profiler

## Executive Summary

By profiling Xcode's GPUToolsReplayService while it collected performance data, we discovered the exact APIs and mechanisms it uses to measure GPU shader timing. The key insight: **Instruments uses Apple Performance Streaming (APS) and IOReport framework to collect GPU performance counters in real-time during replay**.

## Key APIs and Frameworks Discovered

### 1. IOReport Framework (Public API)
Located in: `IOKit.framework`

**Primary Functions:**
- `IOReportCopyChannelsInCategories()` - Get available performance channels
- `IOReportCopyFilteredChannels()` - Filter channels by criteria
- `IOReportCopyChannelsForDriver()` - Get channels for specific driver
- `IOReportMergeChannels()` - Merge channel data
- `IOReportIterate()` - Iterate over channel data
- `IOReportPrune()` - Prune channel data
- `IOReportChannelGetGroup()` - Get channel group info

**Usage Pattern:**
```text
GTPerfStatsHelper::setup()
  └─ IOReportCopyChannelsInCategories()
     └─ IOReportCopyFilteredChannels()
        └─ IOReportCopyChannelsForDriver()
           └─ IOReportCopyDriverName()
```

**Time spent:** 142ms (12.2% of total profiling time)

### 2. Apple Performance Streaming (APS)
Private API for streaming GPU performance data.

**Key Classes/Functions:**
- `GTUSCSamplingStreamingManagerHelper::InitAPSStreaming()` - Initialize APS
- `GTUSCSamplingStreamingManagerHelper::StreamAPS()` - Stream performance data
- `GTUSCSamplingStreamingManagerHelper::SetupBuffersForAPSSource()` - Setup ring buffers
- `GTUSCSamplingStreamingManagerHelper::PollAndDrainSourceRingBuffer()` - Read data
- `GTUSCSamplingStreamingManagerHelper::PostProcessRawData()` - Process raw counter data

**Time spent:** 318ms (27.4% of total - largest single operation!)

### 3. AGX GPU Raw Counter API
Private API for Apple GPU hardware performance counters.

**Key Classes:**
- `AGXGPURawCounter` - Main counter interface
- `AGXGPURawCounterImpl` - Implementation
- `AGXGPURawCounterSource` - Counter data source
- `AGXGPURawCounterSourceGroup` - Group of counters
- `AGXGPURawCounterImpl::SourceAPSImpl` - APS-specific implementation

**Key Methods:**
- `AGXGPURawCounter::createInstance()` - Create counter instance
- `AGXGPURawCounterImpl::SourceImpl::setOptions()` - Configure counter options
- `AGXGPURawCounterImpl::SourceImpl::setBufferSize()` - Set ring buffer size
- `AGXGPURawCounterImpl::startSampling()` - Begin sampling
- `AGXGPURawCounterImpl::stopSampling()` - End sampling
- `AGXGPURawCounterImpl::SourceAPSImpl::RingBufferAPSImpl::drain()` - Read samples

**Time spent:** 32ms for setup, 168ms for setOptions

### 4. GTMTLReplay Framework Components

**Replay Control:**
- `GTMTLReplayController_defaultDispatchFunction_noPinning()` - Execute commands
- `GTMTLReplayController_restoreCommandBuffer()` - Restore command buffer state
- `GTMTLReplayController_restoreMTLBufferContents()` - Restore buffer data
- `GTMTLReplay_commitCommandBuffer()` - Commit command buffer for execution
- `GTMTLReplayController_prePlayForProfiling()` - Prepare for profiled replay

**Profiling-Specific:**
- `GTUSCSamplingStreamingManagerHelper::ReplayForDerivedCounters()` - Replay with counter collection
- `GTUSCSamplingStreamingManagerHelper::ReplaySingleFrameForUSCSampling()` - Frame-by-frame replay
- `GTUSCSamplingStreamingManagerHelper::StreamEncoderDerivedCounterData()` - Collect encoder counters

**Time spent:** 178ms for ReplayForDerivedCounters (15.3%)

### 5. Shader Profiler Data Collection

**Classes:**
- `GTMutableShaderProfilerStreamData` - Container for profiling data
- `GTShaderProfilerShaderFunctionInfo` - Per-shader function info

**Methods:**
- `-[GTMutableShaderProfilerStreamData addAPSData:]` - Add APS sample data
- `-[GTMutableShaderProfilerStreamData addAPSCounterData:]` - Add counter data
- `-[GTMutableShaderProfilerStreamData _copyForAddAPSData:prefix:]` - Copy/process data

## Profiling Workflow

Based on the call stacks, here's the complete workflow:

### Phase 1: Initialize Performance Counters (150ms)
```text
GTUSCSamplingStreamingManagerHelper::Init()
  └─ GTPerfStatsHelper::setup()
     └─ IOReportCopyChannelsInCategories()
        └─ IOReportCopyFilteredChannels()
           └─ _IOReportCopyChannelsForDriver()
```

### Phase 2: Setup APS Streaming (68ms)
```text
GTUSCSamplingStreamingManagerHelper::InitAPSStreaming()
  └─ SetupBuffersForAPSSource()
     └─ SetupBufferForSourceAtIndex()
  └─ SetupGPURawCounters()
     └─ AGXGPURawCounter::createInstance()
        └─ AGXGPURawCounterImpl::init()
           └─ AGXGPURawCounterImpl::SourceImpl::setBufferSize()
```

### Phase 3: Stream and Collect Data (270ms)
```text
GTUSCSamplingStreamingManagerHelper::StreamAPS()
  ├─ AGXGPURawCounterSource::setOptions()  [162ms]
  │  └─ AGXGPURawCounterImpl::SourceImpl::setBufferSize()
  │
  ├─ AGXGPURawCounterImpl::startSampling()  [8ms]
  │
  ├─ ReplaySingleFrameForUSCSampling()  [52ms]
  │  └─ GTMTLReplayController_dispatchForUSCSampling()
  │     └─ GTMTLReplayController_defaultDispatchFunction()
  │        └─ GTMTLReplay_commitCommandBuffer()
  │           └─ [GPU EXECUTES SHADERS HERE]
  │
  ├─ PollAndDrainSourceRingBuffer()  [background thread]
  │  └─ AGXGPURawCounterImpl::SourceAPSImpl::RingBufferAPSImpl::drain()
  │
  ├─ AGXGPURawCounterImpl::stopSampling()  [2ms]
  │
  └─ PostProcessRawData()  [background thread]
```

### Phase 4: Derived Counters (220ms)
```text
GTUSCSamplingStreamingManagerHelper::StreamEncoderDerivedCounterData()
  └─ ReplayForDerivedCounters()
     ├─ DispatchFunction()  [85ms]
     │  └─ [GPU EXECUTES SHADERS AGAIN]
     └─ GTMTLReplayController_restoreCommandBuffer()  [74ms]
```

### Phase 5: Finalize Data (19ms)
```text
GTMTLReplayClient_collectAPSData_block_invoke
  └─ GTMutableShaderProfilerStreamData::addAPSData()
     └─ GTMutableShaderProfilerStreamData::_copyForAddAPSData()
```

## Time Budget Breakdown

Total profiling time: **1.16 seconds**

| Operation | Time | % | Description |
|-----------|------|---|-------------|
| InitAPSStreaming | 318ms | 27.4% | Initialize APS and IOReport |
| StreamAPS | 270ms | 23.2% | Stream performance data during replay |
| StreamEncoderDerivedCounterData | 220ms | 18.9% | Collect derived counter data |
| IOReport setup | 142ms | 12.2% | Query available performance channels |
| AGX setOptions | 168ms | 14.5% | Configure GPU counters |
| Replay execution | ~85ms | 7.3% | Actual GPU command replay |
| Other | ~50ms | 4.3% | Data processing, cleanup |

**Key Insight:** Most time (70%+) is spent on setup and data collection infrastructure, not actual GPU execution!

## Ring Buffer Architecture

APS uses a ring buffer architecture for low-overhead streaming:

1. **Setup:** `setBufferSize()` allocates ring buffer in shared memory
2. **Sampling:** GPU hardware writes performance data to ring buffer
3. **Polling:** Background thread (`PollAndDrainSourceRingBuffer`) reads data
4. **Processing:** Another thread (`PostProcessRawData`) processes samples
5. **Draining:** `RingBufferAPSImpl::drain()` extracts final data

This allows continuous sampling with minimal impact on GPU execution.

## What We Can Use

### Public APIs (Available Now)

**IOReport Framework** - Can query GPU performance channels:
```objective-c
#include <IOReport.h>

// Get all GPU performance channels
CFDictionaryRef channels = IOReportCopyChannelsInCategories(
    categories,  // e.g., "GPU", "Energy"
    NULL,        // All devices
    NULL,        // All channels
    NULL         // No filtering
);

// Iterate and read channel data
IOReportIterate(channels, ^(IOReportChannelRef channel) {
    // Get channel name, value, units, etc.
    return kIOReportIterOk;
});
```

**Documentation:** Available in IOKit.framework headers

### Private APIs (Reverse Engineering Required)

**AGXGPURawCounter** - Direct GPU hardware counters:
- Located in: `/System/Library/Extensions/AGXMetalA*.bundle/`
- Requires: Reverse engineering or runtime inspection
- Risk: May break with OS updates
- Benefit: Most accurate, lowest overhead

**GTMTLReplay Framework** - Command buffer replay:
- Located in: `/System/Library/PrivateFrameworks/GPUToolsReplay.framework/`
- Could potentially link against it (unsupported)
- Provides same replay mechanism as Instruments

## Recommendations for gputrace

### Option 1: Use IOReport (Recommended)
- **Pros:** Public API, documented, stable
- **Cons:** Less detailed than APS, requires active GPU
- **Implementation:** Add IOReport-based timing to `timing_v2.go`

### Option 2: Implement Metal Performance Counters
- **Pros:** Public Metal API, well-documented
- **Cons:** Requires replay implementation
- **API:** `MTLCounterSampleBuffer`, `MTLCounterSet`
- **Implementation:** New `replay.go` module

### Option 3: Parse .gpuprofiler_raw (If Available)
- **Pros:** One-time parse, no replay needed
- **Cons:** Only available if trace captured with counters
- **Implementation:** Extend existing counter parsing code

### Option 4: Runtime Hook APS APIs (Advanced)
- **Pros:** Get exact data Instruments gets
- **Cons:** Complex, fragile, unsupported
- **Method:** Use `DYLD_INSERT_LIBRARIES` to intercept APS calls

## Sample Code: Using IOReport

```go
package gputrace

/*
#cgo LDFLAGS: -framework IOKit -framework CoreFoundation
#include <IOKit/IOKitLib.h>
#include <CoreFoundation/CoreFoundation.h>

// Forward declare IOReport functions
extern CFDictionaryRef IOReportCopyChannelsInCategories(
    CFStringRef categories,
    CFStringRef deviceID,
    CFStringRef channelID,
    CFDictionaryRef channelFilter
);

extern CFDictionaryRef IOReportCreateSubscription(
    void *a,
    CFDictionaryRef channels,
    CFMutableDictionaryRef *outSub,
    uint64_t,
    CFTypeRef
);
*/
import "C"
import "unsafe"

func GetGPUPerformanceChannels() error {
    // Create category string for GPU
    gpuCategory := C.CFStringCreateWithCString(
        nil,
        C.CString("GPU"),
        C.kCFStringEncodingUTF8,
    )
    defer C.CFRelease(C.CFTypeRef(gpuCategory))

    // Get all GPU performance channels
    channels := C.IOReportCopyChannelsInCategories(
        gpuCategory,
        nil,  // All devices
        nil,  // All channels
        nil,  // No filter
    )
    if channels == nil {
        return fmt.Errorf("failed to get GPU channels")
    }
    defer C.CFRelease(C.CFTypeRef(channels))

    // TODO: Iterate channels and extract timing data

    return nil
}
```

## Next Steps

1. **Experiment with IOReport** - Validate we can get useful GPU timing
2. **Document MTLCounterSampleBuffer** - Research Metal's official profiling API
3. **Create proof-of-concept** - Implement simple replay with counter collection
4. **Update documentation** - Add findings to main README

## References

- IOReport: `/System/Library/Frameworks/IOKit.framework/Headers/IOReport.h`
- Metal Counters: https://developer.apple.com/documentation/metal/performance_tuning
- AGX GPU: `/System/Library/Extensions/AGXMetalA*.bundle/`
- GPUToolsReplay: `/System/Library/PrivateFrameworks/GPUToolsReplay.framework/`
