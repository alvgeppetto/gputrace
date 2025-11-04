# GPU Trace Replay Engine

**Bead:** gputrace-53
**Date:** 2025-11-03
**Status:** Implementation Complete (Pending Build Fix)

## Overview

The replay engine provides analysis and structure extraction for GPU trace replay from `.gputrace` files. This is Phase 1 of the replay system, providing the foundation for actual GPU execution with Metal API bindings (Phase 2).

## Architecture

### Core Components

1. **replay_state.go** - Metal state restoration analysis
   - Discovers buffers from MTLBuffer-* files
   - Extracts functions from device resources
   - Correlates buffer addresses from capture file
   - Provides dry-run analysis without requiring Metal bindings

2. **replay.go** - Replay orchestration and command extraction
   - Parses MTSP records to extract command structure
   - Reconstructs command buffer sequences
   - Analyzes encoder organization
   - Validates replay readiness

3. **cmd/gputrace/cmd/replay.go** - CLI command interface
   - Multiple output formats (plan, validate, state, json)
   - Replay plan visualization
   - Resource requirement analysis

### Key Types

```go
// ReplayEngine - Main orchestrator
type ReplayEngine struct {
    Trace         *Trace
    State         *ReplayState
    Commands      []ReplayCommand
    Encoders      []ReplayEncoderInfo
    CommandQueue  CommandQueueInfo
}

// ReplayCommand - Reconstructed Metal command
type ReplayCommand struct {
    Type           string // "compute_dispatch", "execute_icb"
    SequenceNum    int
    PipelineAddr   uint64
    FunctionAddr   uint64
    FunctionName   string
    BufferBindings []uint64
    ICBAddr        uint64 // For indirect command buffers
}

// ReplayState - Resource restoration state
type ReplayState struct {
    Buffers        map[uint64]any
    Functions      map[uint64]any
    PipelineStates map[uint64]any
    BufferSizes    map[uint64]uint64
    BufferNames    map[uint64]string
    FunctionNames  map[uint64]string
}
```

## Features Implemented

### 1. Command Structure Extraction

Parses MTSP records to extract:
- **Ct records** → Compute dispatch commands with pipeline/function/buffer bindings
- **Ci records** → Indirect command buffer executions
- **Culul records** → Command buffer/encoder boundaries
- **CS records** → String labels for encoders and resources

### 2. State Restoration Analysis

Analyzes what would be restored:
- **Buffers**: Discovers MTLBuffer-* files, correlates addresses, reads contents
- **Functions**: Extracts from device-resources files
- **Pipelines**: Links pipeline states to functions

### 3. Replay Validation

Checks replay readiness:
- Command presence
- Buffer availability
- Function resolution
- Address correlation

### 4. Multiple Output Formats

- **plan**: Detailed execution plan with command sequence
- **validate**: Validation report with errors/warnings
- **state**: Resource restoration requirements
- **json**: Machine-readable plan export

## Usage Examples

```bash
# Show replay execution plan
gputrace replay trace.gputrace

# Validate replay readiness
gputrace replay trace.gputrace --format validate

# Show resource restoration requirements
gputrace replay trace.gputrace --format state

# Export replay plan to JSON
gputrace replay trace.gputrace --format json -o replay-plan.json
```

## Sample Output

```
=== Replay Plan ===

Trace: /tmp/fast-llm-mlx-test.gputrace

Execution Summary:
  Command Queue: ReplayQueue
  Command Buffers: 1
  Total Encoders: 1
  Total Commands: 1
    - Compute Dispatches: 1
    - ICB Executions: 0

Encoders:
  [ 0] (unlabeled)            1 commands

Command Sequence (first 20):
  Seq  Encoder  Type                 Function/Target
  ------------------------------------------------------------------------------
  0    0        compute_dispatch     func@0x...

=== Replay State Analysis ===

Resource Summary:
  Buffers:   24 (4.25 MB total)
  Functions: 10
  Pipelines: 0

Buffers:
  Name                                     Size       Address
  -----------------------------------------------------------------
  MTLBuffer-100-0                      128.00 KB         0x...
  MTLBuffer-115-1                      128.00 KB         0x...
  ...
```

## Implementation Details

### Buffer Address Correlation

Buffers in the trace directory are named by ID (`MTLBuffer-<id>-<index>`), but the capture file references them by memory address. The correlation process:

1. Scans capture file for buffer address markers (binary pattern: `0x43 0x74 0x55 ...`)
2. Extracts address (8 bytes at offset 0x14)
3. Extracts name (null-terminated string at offset 0x1c)
4. Builds address→name mapping
5. Correlates discovered buffers with addresses

### Command Sequence Reconstruction

Commands are extracted from MTSP records in execution order:

1. Parse Ct records for compute dispatches
   - Extract pipeline address, function address, buffer bindings
2. Parse Ci records for ICB executions
   - Extract ICB address and count
3. Parse Culul records for encoder boundaries
4. Associate CS labels with encoders
5. Maintain sequence numbers for execution order

### Validation Logic

The validation process checks:

- ✓ Commands found in trace
- ✓ Buffers available and correlated
- ✓ Functions resolved from device resources
- ⚠ Unresolved function references
- ⚠ Uncorrelated buffer addresses

## Test Results

```
=== RUN   TestReplayEngineBasic
    Replay plan: 1 commands, 1 encoders
    Validation: CanReplay=true, Errors=0, Warnings=1
--- PASS: TestReplayEngineBasic

=== RUN   TestReplayStateAnalysis
    State analysis: 24 buffers, 10 functions, 0 pipelines
    First buffer: MTLBuffer-100-0 (131072 bytes)
--- PASS: TestReplayStateAnalysis
```

## Phase 2 Integration Points

The replay engine provides hooks for Phase 2 (gputrace-54: Metal execution with counter sampling):

### 1. Resource Creation Hooks

```go
// In ReplayState - extend to create actual Metal objects
func (rs *ReplayState) CreateMetalBuffer(info BufferInfo) (MTLBuffer, error)
func (rs *ReplayState) CreateMetalFunction(info FunctionInfo) (MTLFunction, error)
func (rs *ReplayState) CreatePipelineState(info PipelineInfo) (MTLComputePipelineState, error)
```

### 2. Command Execution Hooks

```go
// In ReplayEngine - extend to execute commands
func (re *ReplayEngine) ExecuteCommand(cmd ReplayCommand, encoder MTLComputeCommandEncoder)
func (re *ReplayEngine) ExecuteWithCounters(plan *ReplayPlan, counterBuffer MTLCounterSampleBuffer)
```

### 3. Counter Sampling Integration

Phase 2 will add:
- MTLCounterSampleBuffer creation per encoder
- Counter samples before/after dispatches
- Counter data resolution and processing
- Metric name standardization

## Current Limitations

1. **No Metal Execution**: Phase 1 provides analysis only, no actual GPU execution
2. **CGo Bindings Required**: Actual Metal replay requires Swift/Objective-C bindings via CGo
3. **Pipeline Discovery**: Currently finds 0 pipelines (may need enhanced parsing)
4. **ICB Expansion**: Indirect command buffers not expanded to individual dispatches
5. **Thread Group Parameters**: Not yet extracted from dispatch records

## Build Status

**Status**: ✓ Package builds successfully
**Issue**: CLI command blocked by timeline.go compilation errors in session 1C1C
**Workaround**: Package API fully functional, CLI pending build fix

## Files Created

- `replay_state.go` (386 lines) - State restoration and analysis
- `replay.go` (413 lines) - Replay orchestration and validation
- `cmd/gputrace/cmd/replay.go` (147 lines) - CLI command
- `replay_test.go` (76 lines) - Test coverage
- `docs/REPLAY_ENGINE.md` (this file)

## Next Steps

1. **Immediate**: Wait for session 1C1C to fix timeline.go build issues
2. **Phase 2 (gputrace-54)**: Add MTLCounterSampleBuffer support
   - Implement CGo bindings to Metal APIs
   - Add counter sampling hooks
   - Resolve counter data
   - Export in Xcode Counters.csv format
3. **Enhancement**: Improve pipeline state discovery
4. **Enhancement**: Extract thread group parameters from dispatches
5. **Enhancement**: Expand ICBs to individual dispatches

## Dependencies

- gputrace-25: GPU Trace Profiling and Analysis Toolkit [P0]
- gputrace-44: Implement Phase 1 core metrics extraction [P1]

## Blocks

- gputrace-54: Add MTLCounterSampleBuffer performance counter sampling [P1]

## References

- [PERFCOUNTER_IMPLEMENTATION_RECOMMENDATION.md](./PERFCOUNTER_IMPLEMENTATION_RECOMMENDATION.md)
- [GPU_PROFILING_APIS_DISCOVERED.md](../GPU_PROFILING_APIS_DISCOVERED.md)
- Apple Metal Performance Counters Documentation
- MTLCounterSampleBuffer API Reference
