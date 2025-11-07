# test-debug-labels

Test workload for validating GPU trace parsing of:
- Debug groups (pushDebugGroup/popDebugGroup)
- Encoder labels
- Buffer labels
- Nested debug hierarchies

## Purpose

Generates a GPU trace with comprehensive labeling to test gputrace parsing of:

1. **Command Buffer Labels**: `ForwardPass`
2. **Debug Group Hierarchy**:
   - `training_iteration`
     - `forward_pass`
       - `compute_add` (encoder-level)
     - `optimization_step`
       - `compute_multiply` (encoder-level)
       - `apply_scale_factor` (encoder-level)
3. **Encoder Labels**: `VectorAddition`, `VectorMultiply`, `ApplyScaling`
4. **Buffer Labels**:
   - `input_tensor_A`
   - `input_tensor_B`
   - `temp_computation_result`
   - `final_output`
   - `scale_factor`

## Building

```bash
# Compile Objective-C test program
clang -framework Metal -framework Foundation -o test-debug-labels main.m

# Run with GPU tracing enabled
MTL_CAPTURE_ENABLED=1 ./test-debug-labels

# Or capture in Xcode Instruments
# Product > Profile > Metal System Trace
```

## Expected Trace Structure

The generated trace should show:
- Nested debug groups in hierarchy
- Labeled encoders within groups
- Named buffers in shader bindings
- Clear operation context for profiling

## Validation

Parse the generated trace with gputrace:

```bash
# Extract timing data
../../cmd/gputrace/gputrace timing test-debug-labels.gputrace

# Verify encoder labels are parsed
../../cmd/gputrace/gputrace export-counters test-debug-labels.gputrace

# Check buffer names appear in output
../../cmd/gputrace/gputrace dump test-debug-labels.gputrace | grep -i "buffer\|label"
```
