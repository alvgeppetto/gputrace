# GPU Trace Format Documentation

This document describes the findings from reverse engineering the Apple GPU trace format used by Xcode's GPU debugger.

## File Structure

A `.gputrace` directory contains:

- `capture` - Main binary trace file containing MTSP records
- `index` - xdic index mapping function calls to offsets
- `metadata` - Trace metadata (timestamps, device info, etc.)
- `device-resources-*` - Device resource state snapshots
- `MTLBuffer-*-*` - Metal buffer contents (symlinks for aliased buffers)
- Various shader files (hex UUIDs)

## Capture File Format

### File Header
```
Magic: "MTSP" (Metal Trace Stream Protocol)
```

### Record Types

The capture file contains various record types identified by 4-byte magic numbers:

1. **CUUU** - Command buffer records (70 in test trace)
   - +0x00: "CUUU" magic (4 bytes)
   - +0x04: padding (4 bytes)
   - +0x08: timestamp (8 bytes, little-endian)
   - +0x10: UUID string (null-terminated hex)

   Each CUUU marker represents one committed Metal command buffer.

2. **Culul** - Encoder/label markers (6 in test trace)
   - These appear to mark encoder boundaries or labeled scopes
   - NOT the same as command buffers!

3. **Cul** - Unknown label records (1140 in test trace)

4. **Ct** - Unknown trace records (4624 in test trace)

5. **Cuw** - Unknown records (13 in test trace)

6. **Ci** - Unknown info records (6 in test trace)

## Index File Format (xdic)

The `index` file maps function indices to capture file offsets:

```
Header (20 bytes):
  +0x00: "xdic" magic (4 bytes)
  +0x04: version (4 bytes)
  +0x08: entry_size (4 bytes) - typically 8192 (0x2000)
  +0x0C: entry_count (4 bytes) - number of function indices
  +0x10: entry_count_copy (4 bytes) - duplicate count

Entry Array (starting at 0x20):
  Each entry is 8 bytes (two uint32s):
    [function_index]: offset1, offset2

  0xffffffff indicates no mapping for that slot
```

In the test trace:
- 3,771 total entries
- 1,138 unique function indices with mappings

## Counting Metal API Calls

### Command Buffers

**Key Discovery**: Command buffers are counted by CUUU markers, NOT by:
- Culul markers (only 6 vs 70 command buffers)
- Function call counts
- Other record types

Each `[MTLCommandBuffer commit]` call generates one CUUU record with:
- Unique timestamp
- Unique UUID identifier
- Offset in the capture stream

### Compute Encoders

Compute command encoders are identified by **Cul records with specific characteristics**:
- Type field = 1 (at offset +0x0C)
- Size/count field = 0x74 (116 decimal, at offset +0x14)

Each `[MTLCommandBuffer computeCommandEncoder]` or `[MTLCommandBuffer computeCommandEncoderWithDescriptor:]` call generates a Cul record matching these criteria.

Test results:
- Trace 1: 42 compute encoders
- Trace 2: 38 compute encoders

### Dispatch Calls

Dispatch calls (kernel launches) are identified by the **"ul@3" marker pattern**.

Each `[MTLComputeCommandEncoder dispatchThreadgroups:...]` or `[MTLComputeCommandEncoder dispatchThreads:...]` call generates this marker.

Test results:
- Trace 1: 1646 dispatch calls
- Trace 2: 1578 dispatch calls

## Implementation

See `command_buffer.go` for the Go implementation:

```go
// Command buffers (CUUU markers)
type CommandBuffer struct {
    Index     int      // 0-based index
    Timestamp uint64   // Commit timestamp
    UUID      string   // Unique identifier
    Offset    int64    // File offset
}
func (t *Trace) ParseCommandBuffers() ([]*CommandBuffer, error)
func (t *Trace) CountCommandBuffers() (int, error)

// Compute encoders (Cul records with type=1, size=0x74)
type ComputeEncoder struct {
    Index   int      // 0-based index
    Address uint64   // Encoder address/ID
    Offset  int64    // File offset
}
func (t *Trace) ParseComputeEncoders() ([]*ComputeEncoder, error)
func (t *Trace) CountComputeEncoders() (int, error)

// Dispatch calls ("ul@3" markers)
type DispatchCall struct {
    Index  int      // 0-based index
    Offset int64    // File offset
}
func (t *Trace) ParseDispatchCalls() ([]*DispatchCall, error)
func (t *Trace) CountDispatchCalls() (int, error)
```

## Usage

```bash
# Count command buffers
gputrace stats trace.gputrace

# List all command buffers with details
go test -v -run TestParseCommandBuffers
```

## References

Based on reverse engineering of:
- `/Applications/Xcode-beta.app/Contents/PlugIns/GPUDebugger.ideplugin`
- `/Applications/Xcode-beta.app/Contents/SharedFrameworks/GPUToolsCore.framework`

Key classes and symbols found:
- `GPUMTLTraceCommandBufferGroupItem`
- `GTHostMTLCommandBuffer`
- `DYCaptureSession`
- `kDYMessageGuestAppMTLCommandBuffersCaptured`
