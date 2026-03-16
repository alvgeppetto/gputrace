# test-debug-labels

Test programs for validating GPU trace parsing of debug annotations.

## Overview

Two equivalent implementations that generate GPU traces with rich debug annotations:
- **main.cpp** - C++ using MLX debug API
- **main_objc.m** - Objective-C using raw Metal API

Both programs implement the same computation graph to ensure we can parse annotations from both MLX-generated and raw Metal traces.

## Annotations Generated

### Debug Groups (Hierarchical Labels)

```
training_iteration
├── forward_pass
│   ├── linear_layer
│   └── activation
├── data_preprocessing
│   └── normalization
├── loss_computation
│   ├── mse_loss
│   └── regularization
├── backward_pass
│   ├── compute_gradients
│   └── gradient_clipping
└── optimization_step
    └── apply_updates
```

### Named Buffers

**C++ version (23 buffers):**
- input_tensor_A, input_tensor_B, bias_vector
- matmul_output, biased_output, relu_output
- activation_mean, activation_variance, normalized_activations
- target_values, prediction_error, squared_error, mean_squared_error
- l2_regularization, total_loss
- loss_gradient, weight_gradients, bias_gradients
- gradient_norm, clipped_gradients
- updated_weights, updated_bias

**Objective-C version (20 buffers):**
- Same as above but slightly fewer intermediate buffers

### Encoder Labels

Each compute operation has a descriptive encoder label:
- MatrixMultiply, AddBias, ReLUActivation
- ComputeMean, ComputeVariance, Normalize
- PredictionError, SquareError, MeanLoss
- L2Penalty, ComputeGradients, ClipGradients, UpdateWeights

## Building

```bash
make              # Build both versions
make test-debug-labels        # Build C++ version only
make test-debug-labels-objc   # Build Objective-C version only
```

## Running

### C++ MLX Version

```bash
# Run with automatic GPU capture
make run-cpp

# Or manually
MTL_CAPTURE_ENABLED=1 ./test-debug-labels
```

Generates `test_annotations.gputrace`

### Objective-C Metal Version

```bash
# Run without automatic capture
make run-objc

# Or manually
./test-debug-labels-objc

# Capture with xctrace
xctrace record --template 'Metal System Trace' --launch -- ./test-debug-labels-objc --output objc_trace.trace
```

## Validating Parsing

Use the generated gputrace files to test the gputrace parser:

```bash
# Parse and export to CSV
gputrace export-counters test_annotations.gputrace

# Check for debug labels in output
grep -i "training_iteration\|forward_pass" output.csv

# Verify buffer names appear
grep -i "input_tensor_A\|matmul_output" output.csv
```

## Expected Use

These test programs are designed to validate:
1. Hierarchical debug group parsing (push/pop labels)
2. Buffer name/label extraction
3. Encoder label extraction
4. Equivalence between MLX and raw Metal annotation formats

## See Also

- [RECORD_FORMATS.md](../../docs/research/RECORD_FORMATS.md) - Record format documentation
- [docs/BINARY_FORMAT_REFERENCE.md](../../docs/research/BINARY_FORMAT_REFERENCE.md) - Binary format reference
