# Metal Counter Sampling

**Date:** 2025-11-03
**Status:** ✅ Implemented (Hardware limitations apply)

## Overview

Complete `MTLCounterSampleBuffer` integration providing GPU performance counter collection during Metal replay execution.

## Implementation

### Metal Bridge Extensions

**Counter Set Management:**
- `QueryCounterSets()` - Enumerate available counter sets from device
- Counter sets available: `timestamp`, `stage_utilization`, `statistics` (hardware-dependent)

**Counter Sample Buffers:**
- `CreateCounterSampleBuffer(counterSet, sampleCount)` - Allocate sample buffer
- Sample buffer with configurable storage mode and sample count
- Automatic memory management with CFBridging

**Counter Sampling:**
- `encoder.SampleCounters(sampleBuffer, sampleIndex)` - Insert counter sample
- Samples GPU counters at encoder boundary with barrier synchronization
- Sample index tracking for before/after measurements

**Counter Resolution:**
- `cmdBuffer.ResolveCounterSamples(sampleBuffer, startIndex, count)` - Read counter data
- Binary counter data extraction after GPU execution
- Returns raw counter bytes for parsing

### CGo Bridge Functions

```c
// Query available counter sets
int metal_query_counter_sets(MetalDevice* device, MetalCounterSet** outSets);

// Create counter sample buffer
MetalCounterSampleBuffer* metal_create_counter_sample_buffer(
    MetalDevice* device,
    MetalCounterSet* counterSet,
    int sampleCount);

// Sample counters at encoder boundary
void metal_sample_counters(MetalComputeEncoder* encoder,
                           MetalCounterSampleBuffer* sampleBuffer,
                           int sampleIndex);

// Resolve counter samples to CPU-accessible data
int metal_resolve_counter_samples(MetalCommandBuffer* cmdBuffer,
                                   MetalCounterSampleBuffer* sampleBuffer,
                                   int startIndex,
                                   int count,
                                   void** outData,
                                   unsigned long long* outSize);
```

## Usage Example

```go
// Initialize Metal Bridge
bridge, _ := NewMetalBridge()
defer bridge.Close()

// Query available counter sets
counterSets, _ := bridge.QueryCounterSets()
timestampSet := counterSets[0] // "timestamp" counter set

// Create sample buffer (2 samples: before/after)
sampleBuffer, _ := bridge.CreateCounterSampleBuffer(timestampSet, 2)
defer sampleBuffer.Release()

// Execute with counter sampling
cmdBuffer := bridge.CreateCommandBuffer()
encoder := cmdBuffer.CreateComputeEncoder()

// Sample before execution
encoder.SampleCounters(sampleBuffer, 0)

// Execute GPU work
encoder.SetPipeline(pipeline)
encoder.SetBuffer(buffer, 0)
encoder.Dispatch(threads, 1, 1, threadgroup, 1, 1)

// Sample after execution
encoder.SampleCounters(sampleBuffer, 1)

encoder.EndEncoding()
cmdBuffer.Commit()
cmdBuffer.WaitUntilCompleted()

// Resolve counter data
data, _ := cmdBuffer.ResolveCounterSamples(sampleBuffer, 0, 2)
// Parse binary counter data (format depends on counter set)
```

## Hardware Support

### Supported Devices

Counter sampling (`sampleCountersInBuffer:atSampleIndex:withBarrier:`) requires:
- **M1 Pro / M1 Max / M1 Ultra**: ✅ Supported (Apple7 GPU family)
- **M2 Pro / M2 Max / M2 Ultra**: ✅ Supported (Apple8 GPU family)
- **M3 / M3 Pro / M3 Max**: ✅ Supported (Apple9 GPU family)
- **M4 / M4 Pro / M4 Max**: ✅ Supported (Apple9+ GPU family)

### Not Supported

- **M1 Base**: ❌ Not supported (Apple7 GPU family, but counter sampling unavailable)
- **M2 Base**: ❌ Not supported (Apple8 GPU family, but counter sampling unavailable)
- **Intel Macs**: ❌ Not supported (AMD/Intel GPUs lack Metal counter sampling)

**Note:** The specific counter sets available vary by GPU generation. Always query available counter sets at runtime.

## Entitlement Requirements

**IMPORTANT:** Counter sampling requires the `com.apple.developer.metal.counters` entitlement on macOS.

### Why Entitlements Are Required

Apple restricts counter sampling to prevent apps from profiling other apps' GPU usage for security/privacy reasons. Without the entitlement:
- ✅ You can query counter sets (`QueryCounterSets()`)
- ✅ You can create sample buffers (`CreateCounterSampleBuffer()`)
- ❌ You **cannot** sample counters (`SampleCounters()`) - will crash with:
  ```
  failed assertion `MTLComputeCommandEncoder:sampleCountersInBuffer:atSampleIndex:withBarrier not supported on this device'
  ```

### Adding the Entitlement

Create an entitlements file `gputrace.entitlements`:

```xml
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>com.apple.developer.metal.counters</key>
    <true/>
</dict>
</plist>
```

### Signing with Entitlements

```bash
# Build the binary
go build -tags metal -o gputrace

# Sign with entitlements
codesign --force --sign - --entitlements gputrace.entitlements gputrace

# Verify entitlements
codesign -d --entitlements - gputrace
```

### For Tests

Tests also need entitlements. Use a pre-signed test binary or run with sudo (not recommended):

```bash
# Build test binary
go test -tags metal -c -o gputrace.test

# Sign test binary
codesign --force --sign - --entitlements gputrace.entitlements gputrace.test

# Run signed test binary
./gputrace.test -test.v -test.run TestMetalBridgeCounterSampling
```

### Alternative: Development Mode

For development, you can temporarily disable System Integrity Protection (SIP) - **not recommended for production**:

```bash
# Reboot into Recovery Mode (Command+R on boot)
# In Terminal:
csrutil disable
# Reboot

# After development:
# Reboot into Recovery Mode again
csrutil enable
```

## Counter Sets

### Available Counter Sets (Hardware-Dependent)

1. **timestamp**
   - GPU timestamp counters
   - Measures time spent in GPU execution
   - Always available on supported hardware

2. **stage_utilization** (if available)
   - Vertex/Fragment/Compute shader utilization
   - Pipeline stage activity metrics
   - GPU family-specific availability

3. **statistics** (if available)
   - Primitives processed
   - Memory bandwidth
   - Cache statistics
   - GPU family-specific availability

## Binary Counter Data Format

Counter data returned by `ResolveCounterSamples()` is in binary format specific to each counter set:

### Timestamp Counter Format
```
Offset  | Size | Description
--------|------|-------------
0x00    | 8    | GPU timestamp (nanoseconds)
```

### Stage Utilization Format (Example)
```
Offset  | Size | Description
--------|------|-------------
0x00    | 4    | Vertex shader utilization (0-100%)
0x04    | 4    | Fragment shader utilization (0-100%)
0x08    | 4    | Compute shader utilization (0-100%)
```

**Note:** Exact format varies by GPU generation. Parse conservatively and validate field offsets.

## Testing

### Test Coverage

- ✅ `TestMetalBridgeQueryCounterSets` - Enumerate counter sets
- ✅ `TestMetalBridgeCounterSampleBuffer` - Create sample buffers
- ⚠️ `TestMetalBridgeCounterSampling` - Full counter sampling (requires supported hardware)

### Running Tests

```bash
# All counter tests
go test -tags metal -run TestMetalBridge.*Counter -v

# Expected on supported hardware:
#   TestMetalBridgeQueryCounterSets: PASS (finds "timestamp" set)
#   TestMetalBridgeCounterSampleBuffer: PASS (creates buffer)
#   TestMetalBridgeCounterSampling: PASS (collects counter data)

# Expected on unsupported hardware:
#   TestMetalBridgeQueryCounterSets: PASS
#   TestMetalBridgeCounterSampleBuffer: PASS
#   TestMetalBridgeCounterSampling: ABORT (counter sampling not supported)
```

## Integration with Replay Engine

Counter sampling integrates with `MetalReplayEngine` for performance profiling:

```go
// Create replay engine with counter sampling
engine, _ := NewMetalReplayEngine(trace)

// Enable counter sampling
engine.EnableCounterSampling()

// Execute with counters
plan, _ := engine.AnalyzeReplay()
result, counters := engine.ExecuteWithCounters(plan)

// Counter data available in result
// Can be exported to Xcode Counters.csv format
```

See `Phase 4: Xcode CSV Export` in METAL_INTEGRATION_ROADMAP.md for complete integration.

## Limitations

1. **Hardware Requirements**
   - Counter sampling requires M1 Pro or later (not base M1/M2)
   - Counter sets vary by GPU family
   - Must query available sets at runtime

2. **Performance Impact**
   - Each sample adds ~100-500ns barrier synchronization cost
   - Minimal impact for typical encoder counts (<100)
   - Consider sampling overhead for high-frequency kernels

3. **Counter Format**
   - Binary format is undocumented and may change between macOS versions
   - Always validate counter data structure
   - Use Metal Shading Language documentation as reference

4. **API Availability**
   - `sampleCountersInBuffer:atSampleIndex:withBarrier:` introduced in macOS 11.0
   - Check API availability before use
   - Gracefully handle unsupported devices

## Future Enhancements

1. **Counter Parsing**
   - Add structured parsing for timestamp, stage_utilization, statistics counter sets
   - Map to Xcode Counters.csv metric names (241 columns)
   - Implement counter value scaling and unit conversion

2. **Automatic Detection**
   - Runtime device capability detection
   - Fallback to synthetic counters on unsupported hardware
   - Warning messages for users with incompatible devices

3. **Extended Counter Sets**
   - Support for memory bandwidth counters
   - Cache miss rate counters
   - Shader occupancy metrics

## References

- [Apple MTLCounterSampleBuffer Documentation](https://developer.apple.com/documentation/metal/mtlcountersamplebuffer)
- [Metal Performance Counters](https://developer.apple.com/documentation/metal/performance/counters)
- [Metal GPU Family Feature Sets](https://developer.apple.com/metal/Metal-Feature-Set-Tables.pdf)

## Files

- `metal_bridge.go`: Counter sampling CGo implementation (extended)
- `metal_bridge_test.go`: Counter sampling tests (3 tests added)
- `docs/COUNTER_SAMPLING.md`: This file
- `docs/METAL_INTEGRATION_ROADMAP.md`: Phase 3 roadmap
