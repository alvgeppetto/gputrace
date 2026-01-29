// Test workload: Debug labels and named buffers for GPU trace parsing validation
// Objective-C version using raw Metal API - should be equivalent to C++ MLX version

#import <Metal/Metal.h>
#import <Foundation/Foundation.h>

int main(int argc, const char * argv[]) {
    @autoreleasepool {
        // Get default Metal device
        id<MTLDevice> device = MTLCreateSystemDefaultDevice();
        if (!device) {
            fprintf(stderr, "Metal is not supported on this device\n");
            return 1;
        }

        printf("Running Objective-C Metal debug labels test on: %s\n", [[device name] UTF8String]);

        // Simple compute shaders that mirror the MLX operations
        NSString *shaderSource = @
            "#include <metal_stdlib>\n"
            "using namespace metal;\n"
            "\n"
            "// Matrix multiplication kernel (simplified for 256x128 * 128x64)\n"
            "kernel void matmul(\n"
            "    device float* A [[buffer(0)]],\n"
            "    device float* B [[buffer(1)]],\n"
            "    device float* C [[buffer(2)]],\n"
            "    uint2 gid [[thread_position_in_grid]])\n"
            "{\n"
            "    uint row = gid.y;\n"
            "    uint col = gid.x;\n"
            "    if (row >= 256 || col >= 64) return;\n"
            "    \n"
            "    float sum = 0.0f;\n"
            "    for (uint k = 0; k < 128; k++) {\n"
            "        sum += A[row * 128 + k] * B[k * 64 + col];\n"
            "    }\n"
            "    C[row * 64 + col] = sum;\n"
            "}\n"
            "\n"
            "// Add bias vector\n"
            "kernel void add_bias(\n"
            "    device float* input [[buffer(0)]],\n"
            "    device float* bias [[buffer(1)]],\n"
            "    device float* output [[buffer(2)]],\n"
            "    uint2 gid [[thread_position_in_grid]])\n"
            "{\n"
            "    uint row = gid.y;\n"
            "    uint col = gid.x;\n"
            "    if (row >= 256 || col >= 64) return;\n"
            "    output[row * 64 + col] = input[row * 64 + col] + bias[col];\n"
            "}\n"
            "\n"
            "// ReLU activation\n"
            "kernel void relu(\n"
            "    device float* input [[buffer(0)]],\n"
            "    device float* output [[buffer(1)]],\n"
            "    uint id [[thread_position_in_grid]])\n"
            "{\n"
            "    output[id] = max(input[id], 0.0f);\n"
            "}\n"
            "\n"
            "// Compute mean along axis 1\n"
            "kernel void compute_mean(\n"
            "    device float* input [[buffer(0)]],\n"
            "    device float* output [[buffer(1)]],\n"
            "    uint gid [[thread_position_in_grid]])\n"
            "{\n"
            "    if (gid >= 256) return;\n"
            "    float sum = 0.0f;\n"
            "    for (uint i = 0; i < 64; i++) {\n"
            "        sum += input[gid * 64 + i];\n"
            "    }\n"
            "    output[gid] = sum / 64.0f;\n"
            "}\n"
            "\n"
            "// Compute variance along axis 1\n"
            "kernel void compute_variance(\n"
            "    device float* input [[buffer(0)]],\n"
            "    device float* mean [[buffer(1)]],\n"
            "    device float* output [[buffer(2)]],\n"
            "    uint gid [[thread_position_in_grid]])\n"
            "{\n"
            "    if (gid >= 256) return;\n"
            "    float m = mean[gid];\n"
            "    float sum = 0.0f;\n"
            "    for (uint i = 0; i < 64; i++) {\n"
            "        float diff = input[gid * 64 + i] - m;\n"
            "        sum += diff * diff;\n"
            "    }\n"
            "    output[gid] = sum / 64.0f;\n"
            "}\n"
            "\n"
            "// Normalize\n"
            "kernel void normalize(\n"
            "    device float* input [[buffer(0)]],\n"
            "    device float* mean [[buffer(1)]],\n"
            "    device float* variance [[buffer(2)]],\n"
            "    device float* output [[buffer(3)]],\n"
            "    uint2 gid [[thread_position_in_grid]])\n"
            "{\n"
            "    uint row = gid.y;\n"
            "    uint col = gid.x;\n"
            "    if (row >= 256 || col >= 64) return;\n"
            "    uint idx = row * 64 + col;\n"
            "    float m = mean[row];\n"
            "    float v = variance[row];\n"
            "    output[idx] = (input[idx] - m) / sqrt(v + 1e-6f);\n"
            "}\n"
            "\n"
            "// Subtract arrays (for prediction error)\n"
            "kernel void subtract(\n"
            "    device float* a [[buffer(0)]],\n"
            "    device float* b [[buffer(1)]],\n"
            "    device float* output [[buffer(2)]],\n"
            "    uint id [[thread_position_in_grid]])\n"
            "{\n"
            "    output[id] = a[id] - b[id];\n"
            "}\n"
            "\n"
            "// Square elements\n"
            "kernel void square(\n"
            "    device float* input [[buffer(0)]],\n"
            "    device float* output [[buffer(1)]],\n"
            "    uint id [[thread_position_in_grid]])\n"
            "{\n"
            "    float val = input[id];\n"
            "    output[id] = val * val;\n"
            "}\n"
            "\n"
            "// Mean reduction\n"
            "kernel void mean_reduce(\n"
            "    device float* input [[buffer(0)]],\n"
            "    device float* output [[buffer(1)]],\n"
            "    constant uint& count [[buffer(2)]])\n"
            "{\n"
            "    // Simple single-threaded reduction for demo\n"
            "    float sum = 0.0f;\n"
            "    for (uint i = 0; i < count; i++) {\n"
            "        sum += input[i];\n"
            "    }\n"
            "    output[0] = sum / count;\n"
            "}\n"
            "\n"
            "// Scale by constant\n"
            "kernel void scale(\n"
            "    device float* input [[buffer(0)]],\n"
            "    device float* output [[buffer(1)]],\n"
            "    constant float& factor [[buffer(2)]],\n"
            "    uint id [[thread_position_in_grid]])\n"
            "{\n"
            "    output[id] = input[id] * factor;\n"
            "}\n"
            "\n"
            "// Add scalar\n"
            "kernel void add_scalar(\n"
            "    device float* a [[buffer(0)]],\n"
            "    device float* b [[buffer(1)]],\n"
            "    device float* output [[buffer(2)]],\n"
            "    uint id [[thread_position_in_grid]])\n"
            "{\n"
            "    output[id] = a[id] + b[id];\n"
            "}\n";

        // Compile shaders
        NSError *error = nil;
        id<MTLLibrary> library = [device newLibraryWithSource:shaderSource
                                                       options:nil
                                                         error:&error];
        if (!library) {
            fprintf(stderr, "Failed to compile shaders: %s\n",
                    [[error localizedDescription] UTF8String]);
            return 1;
        }

        // Create compute pipeline states
        id<MTLComputePipelineState> matmulPipeline = [device newComputePipelineStateWithFunction:[library newFunctionWithName:@"matmul"] error:&error];
        id<MTLComputePipelineState> addBiasPipeline = [device newComputePipelineStateWithFunction:[library newFunctionWithName:@"add_bias"] error:&error];
        id<MTLComputePipelineState> reluPipeline = [device newComputePipelineStateWithFunction:[library newFunctionWithName:@"relu"] error:&error];
        id<MTLComputePipelineState> meanPipeline = [device newComputePipelineStateWithFunction:[library newFunctionWithName:@"compute_mean"] error:&error];
        id<MTLComputePipelineState> variancePipeline = [device newComputePipelineStateWithFunction:[library newFunctionWithName:@"compute_variance"] error:&error];
        id<MTLComputePipelineState> normalizePipeline = [device newComputePipelineStateWithFunction:[library newFunctionWithName:@"normalize"] error:&error];
        id<MTLComputePipelineState> subtractPipeline = [device newComputePipelineStateWithFunction:[library newFunctionWithName:@"subtract"] error:&error];
        id<MTLComputePipelineState> squarePipeline = [device newComputePipelineStateWithFunction:[library newFunctionWithName:@"square"] error:&error];
        id<MTLComputePipelineState> meanReducePipeline = [device newComputePipelineStateWithFunction:[library newFunctionWithName:@"mean_reduce"] error:&error];
        id<MTLComputePipelineState> scalePipeline = [device newComputePipelineStateWithFunction:[library newFunctionWithName:@"scale"] error:&error];
        id<MTLComputePipelineState> addScalarPipeline = [device newComputePipelineStateWithFunction:[library newFunctionWithName:@"add_scalar"] error:&error];

        // Create buffers with labels matching the C++ version
        id<MTLBuffer> inputA = [device newBufferWithLength:256*128*sizeof(float) options:MTLResourceStorageModeShared];
        inputA.label = @"input_tensor_A";

        id<MTLBuffer> inputB = [device newBufferWithLength:128*64*sizeof(float) options:MTLResourceStorageModeShared];
        inputB.label = @"input_tensor_B";

        id<MTLBuffer> bias = [device newBufferWithLength:64*sizeof(float) options:MTLResourceStorageModeShared];
        bias.label = @"bias_vector";

        id<MTLBuffer> matmulOutput = [device newBufferWithLength:256*64*sizeof(float) options:MTLResourceStorageModeShared];
        matmulOutput.label = @"matmul_output";

        id<MTLBuffer> biasedOutput = [device newBufferWithLength:256*64*sizeof(float) options:MTLResourceStorageModeShared];
        biasedOutput.label = @"biased_output";

        id<MTLBuffer> reluOutput = [device newBufferWithLength:256*64*sizeof(float) options:MTLResourceStorageModeShared];
        reluOutput.label = @"relu_output";

        id<MTLBuffer> activationMean = [device newBufferWithLength:256*sizeof(float) options:MTLResourceStorageModeShared];
        activationMean.label = @"activation_mean";

        id<MTLBuffer> activationVariance = [device newBufferWithLength:256*sizeof(float) options:MTLResourceStorageModeShared];
        activationVariance.label = @"activation_variance";

        id<MTLBuffer> normalizedActivations = [device newBufferWithLength:256*64*sizeof(float) options:MTLResourceStorageModeShared];
        normalizedActivations.label = @"normalized_activations";

        id<MTLBuffer> targetValues = [device newBufferWithLength:256*64*sizeof(float) options:MTLResourceStorageModeShared];
        targetValues.label = @"target_values";

        id<MTLBuffer> predictionError = [device newBufferWithLength:256*64*sizeof(float) options:MTLResourceStorageModeShared];
        predictionError.label = @"prediction_error";

        id<MTLBuffer> squaredError = [device newBufferWithLength:256*64*sizeof(float) options:MTLResourceStorageModeShared];
        squaredError.label = @"squared_error";

        id<MTLBuffer> meanSquaredError = [device newBufferWithLength:sizeof(float) options:MTLResourceStorageModeShared];
        meanSquaredError.label = @"mean_squared_error";

        id<MTLBuffer> l2Regularization = [device newBufferWithLength:sizeof(float) options:MTLResourceStorageModeShared];
        l2Regularization.label = @"l2_regularization";

        id<MTLBuffer> totalLoss = [device newBufferWithLength:sizeof(float) options:MTLResourceStorageModeShared];
        totalLoss.label = @"total_loss";

        id<MTLBuffer> lossGradient = [device newBufferWithLength:256*64*sizeof(float) options:MTLResourceStorageModeShared];
        lossGradient.label = @"loss_gradient";

        id<MTLBuffer> weightGradients = [device newBufferWithLength:128*64*sizeof(float) options:MTLResourceStorageModeShared];
        weightGradients.label = @"weight_gradients";

        id<MTLBuffer> gradientNorm = [device newBufferWithLength:sizeof(float) options:MTLResourceStorageModeShared];
        gradientNorm.label = @"gradient_norm";

        id<MTLBuffer> clippedGradients = [device newBufferWithLength:128*64*sizeof(float) options:MTLResourceStorageModeShared];
        clippedGradients.label = @"clipped_gradients";

        id<MTLBuffer> updatedWeights = [device newBufferWithLength:128*64*sizeof(float) options:MTLResourceStorageModeShared];
        updatedWeights.label = @"updated_weights";

        // Initialize data (random-like values)
        float *dataA = (float *)[inputA contents];
        float *dataB = (float *)[inputB contents];
        float *biasData = (float *)[bias contents];
        float *targetData = (float *)[targetValues contents];

        for (int i = 0; i < 256*128; i++) {
            dataA[i] = ((float)rand() / RAND_MAX) * 2.0f - 1.0f;
        }
        for (int i = 0; i < 128*64; i++) {
            dataB[i] = ((float)rand() / RAND_MAX) * 2.0f - 1.0f;
        }
        for (int i = 0; i < 64; i++) {
            biasData[i] = 0.5f;
        }
        for (int i = 0; i < 256*64; i++) {
            targetData[i] = ((float)rand() / RAND_MAX);
        }

        // Create command queue and buffer
        id<MTLCommandQueue> queue = [device newCommandQueue];
        queue.label = @"MainComputeQueue";

        id<MTLCommandBuffer> commandBuffer = [queue commandBuffer];
        commandBuffer.label = @"TrainingIteration";

        // === Hierarchical debug groups matching C++ version ===
        [commandBuffer pushDebugGroup:@"training_iteration"];
        [commandBuffer pushDebugGroup:@"forward_pass"];
        [commandBuffer pushDebugGroup:@"linear_layer"];

        // Matmul
        id<MTLComputeCommandEncoder> encoder = [commandBuffer computeCommandEncoder];
        encoder.label = @"MatrixMultiply";
        [encoder setComputePipelineState:matmulPipeline];
        [encoder setBuffer:inputA offset:0 atIndex:0];
        [encoder setBuffer:inputB offset:0 atIndex:1];
        [encoder setBuffer:matmulOutput offset:0 atIndex:2];
        [encoder dispatchThreads:MTLSizeMake(64, 256, 1) threadsPerThreadgroup:MTLSizeMake(8, 8, 1)];
        [encoder endEncoding];

        // Add bias
        encoder = [commandBuffer computeCommandEncoder];
        encoder.label = @"AddBias";
        [encoder setComputePipelineState:addBiasPipeline];
        [encoder setBuffer:matmulOutput offset:0 atIndex:0];
        [encoder setBuffer:bias offset:0 atIndex:1];
        [encoder setBuffer:biasedOutput offset:0 atIndex:2];
        [encoder dispatchThreads:MTLSizeMake(64, 256, 1) threadsPerThreadgroup:MTLSizeMake(8, 8, 1)];
        [encoder endEncoding];

        [commandBuffer popDebugGroup]; // linear_layer
        [commandBuffer pushDebugGroup:@"activation"];

        // ReLU
        encoder = [commandBuffer computeCommandEncoder];
        encoder.label = @"ReLUActivation";
        [encoder setComputePipelineState:reluPipeline];
        [encoder setBuffer:biasedOutput offset:0 atIndex:0];
        [encoder setBuffer:reluOutput offset:0 atIndex:1];
        [encoder dispatchThreads:MTLSizeMake(256*64, 1, 1) threadsPerThreadgroup:MTLSizeMake(256, 1, 1)];
        [encoder endEncoding];

        [commandBuffer popDebugGroup]; // activation
        [commandBuffer popDebugGroup]; // forward_pass

        [commandBuffer pushDebugGroup:@"data_preprocessing"];
        [commandBuffer pushDebugGroup:@"normalization"];

        // Compute mean
        encoder = [commandBuffer computeCommandEncoder];
        encoder.label = @"ComputeMean";
        [encoder setComputePipelineState:meanPipeline];
        [encoder setBuffer:reluOutput offset:0 atIndex:0];
        [encoder setBuffer:activationMean offset:0 atIndex:1];
        [encoder dispatchThreads:MTLSizeMake(256, 1, 1) threadsPerThreadgroup:MTLSizeMake(256, 1, 1)];
        [encoder endEncoding];

        // Compute variance
        encoder = [commandBuffer computeCommandEncoder];
        encoder.label = @"ComputeVariance";
        [encoder setComputePipelineState:variancePipeline];
        [encoder setBuffer:reluOutput offset:0 atIndex:0];
        [encoder setBuffer:activationMean offset:0 atIndex:1];
        [encoder setBuffer:activationVariance offset:0 atIndex:2];
        [encoder dispatchThreads:MTLSizeMake(256, 1, 1) threadsPerThreadgroup:MTLSizeMake(256, 1, 1)];
        [encoder endEncoding];

        // Normalize
        encoder = [commandBuffer computeCommandEncoder];
        encoder.label = @"Normalize";
        [encoder setComputePipelineState:normalizePipeline];
        [encoder setBuffer:reluOutput offset:0 atIndex:0];
        [encoder setBuffer:activationMean offset:0 atIndex:1];
        [encoder setBuffer:activationVariance offset:0 atIndex:2];
        [encoder setBuffer:normalizedActivations offset:0 atIndex:3];
        [encoder dispatchThreads:MTLSizeMake(64, 256, 1) threadsPerThreadgroup:MTLSizeMake(8, 8, 1)];
        [encoder endEncoding];

        [commandBuffer popDebugGroup]; // normalization
        [commandBuffer popDebugGroup]; // data_preprocessing

        [commandBuffer pushDebugGroup:@"loss_computation"];
        [commandBuffer pushDebugGroup:@"mse_loss"];

        // Subtract for prediction error
        encoder = [commandBuffer computeCommandEncoder];
        encoder.label = @"PredictionError";
        [encoder setComputePipelineState:subtractPipeline];
        [encoder setBuffer:normalizedActivations offset:0 atIndex:0];
        [encoder setBuffer:targetValues offset:0 atIndex:1];
        [encoder setBuffer:predictionError offset:0 atIndex:2];
        [encoder dispatchThreads:MTLSizeMake(256*64, 1, 1) threadsPerThreadgroup:MTLSizeMake(256, 1, 1)];
        [encoder endEncoding];

        // Square
        encoder = [commandBuffer computeCommandEncoder];
        encoder.label = @"SquareError";
        [encoder setComputePipelineState:squarePipeline];
        [encoder setBuffer:predictionError offset:0 atIndex:0];
        [encoder setBuffer:squaredError offset:0 atIndex:1];
        [encoder dispatchThreads:MTLSizeMake(256*64, 1, 1) threadsPerThreadgroup:MTLSizeMake(256, 1, 1)];
        [encoder endEncoding];

        // Mean reduce
        encoder = [commandBuffer computeCommandEncoder];
        encoder.label = @"MeanLoss";
        [encoder setComputePipelineState:meanReducePipeline];
        [encoder setBuffer:squaredError offset:0 atIndex:0];
        [encoder setBuffer:meanSquaredError offset:0 atIndex:1];
        uint count = 256*64;
        [encoder setBytes:&count length:sizeof(uint) atIndex:2];
        [encoder dispatchThreads:MTLSizeMake(1, 1, 1) threadsPerThreadgroup:MTLSizeMake(1, 1, 1)];
        [encoder endEncoding];

        [commandBuffer popDebugGroup]; // mse_loss
        [commandBuffer pushDebugGroup:@"regularization"];

        // L2 regularization (simplified - just square and sum inputB)
        encoder = [commandBuffer computeCommandEncoder];
        encoder.label = @"L2Penalty";
        [encoder setComputePipelineState:squarePipeline];
        [encoder setBuffer:inputB offset:0 atIndex:0];
        [encoder setBuffer:weightGradients offset:0 atIndex:1]; // Reuse buffer temporarily
        [encoder dispatchThreads:MTLSizeMake(128*64, 1, 1) threadsPerThreadgroup:MTLSizeMake(256, 1, 1)];
        [encoder endEncoding];

        [commandBuffer popDebugGroup]; // regularization
        [commandBuffer popDebugGroup]; // loss_computation

        [commandBuffer pushDebugGroup:@"backward_pass"];
        [commandBuffer pushDebugGroup:@"compute_gradients"];

        // Gradient computation (simplified - just scale the error)
        encoder = [commandBuffer computeCommandEncoder];
        encoder.label = @"ComputeGradients";
        float scale_factor = 2.0f / 256.0f;
        [encoder setComputePipelineState:scalePipeline];
        [encoder setBuffer:predictionError offset:0 atIndex:0];
        [encoder setBuffer:lossGradient offset:0 atIndex:1];
        [encoder setBytes:&scale_factor length:sizeof(float) atIndex:2];
        [encoder dispatchThreads:MTLSizeMake(256*64, 1, 1) threadsPerThreadgroup:MTLSizeMake(256, 1, 1)];
        [encoder endEncoding];

        [commandBuffer popDebugGroup]; // compute_gradients
        [commandBuffer pushDebugGroup:@"gradient_clipping"];

        // Gradient clipping (simplified)
        encoder = [commandBuffer computeCommandEncoder];
        encoder.label = @"ClipGradients";
        [encoder setComputePipelineState:scalePipeline];
        [encoder setBuffer:lossGradient offset:0 atIndex:0];
        [encoder setBuffer:clippedGradients offset:0 atIndex:1];
        float clip_factor = 0.2f; // 1.0 / 5.0
        [encoder setBytes:&clip_factor length:sizeof(float) atIndex:2];
        [encoder dispatchThreads:MTLSizeMake(256*64, 1, 1) threadsPerThreadgroup:MTLSizeMake(256, 1, 1)];
        [encoder endEncoding];

        [commandBuffer popDebugGroup]; // gradient_clipping
        [commandBuffer popDebugGroup]; // backward_pass

        [commandBuffer pushDebugGroup:@"optimization_step"];
        [commandBuffer pushDebugGroup:@"apply_updates"];

        // Apply updates (simplified)
        encoder = [commandBuffer computeCommandEncoder];
        encoder.label = @"UpdateWeights";
        [encoder setComputePipelineState:scalePipeline];
        [encoder setBuffer:inputB offset:0 atIndex:0];
        [encoder setBuffer:updatedWeights offset:0 atIndex:1];
        float update_scale = 0.99f; // 1.0 - 0.01 learning rate
        [encoder setBytes:&update_scale length:sizeof(float) atIndex:2];
        [encoder dispatchThreads:MTLSizeMake(128*64, 1, 1) threadsPerThreadgroup:MTLSizeMake(256, 1, 1)];
        [encoder endEncoding];

        [commandBuffer popDebugGroup]; // apply_updates
        [commandBuffer popDebugGroup]; // optimization_step
        [commandBuffer popDebugGroup]; // training_iteration

        // Commit and wait
        [commandBuffer commit];
        [commandBuffer waitUntilCompleted];

        // Print results
        float loss = ((float *)[meanSquaredError contents])[0];
        printf("\nTest complete!\n");
        printf("\nExpected annotations in trace (matching C++ version):\n");
        printf("  Debug groups (hierarchical):\n");
        printf("    - training_iteration\n");
        printf("      - forward_pass\n");
        printf("        - linear_layer\n");
        printf("        - activation\n");
        printf("      - data_preprocessing\n");
        printf("        - normalization\n");
        printf("      - loss_computation\n");
        printf("        - mse_loss\n");
        printf("        - regularization\n");
        printf("      - backward_pass\n");
        printf("        - compute_gradients\n");
        printf("        - gradient_clipping\n");
        printf("      - optimization_step\n");
        printf("        - apply_updates\n");
        printf("\n  Buffer labels (20 buffers):\n");
        printf("    - input_tensor_A, input_tensor_B, bias_vector\n");
        printf("    - matmul_output, biased_output, relu_output\n");
        printf("    - activation_mean, activation_variance, normalized_activations\n");
        printf("    - target_values, prediction_error, squared_error, mean_squared_error\n");
        printf("    - l2_regularization, total_loss\n");
        printf("    - loss_gradient, weight_gradients\n");
        printf("    - gradient_norm, clipped_gradients, updated_weights\n");
        printf("\nFinal loss: %.4f\n", loss);

        return 0;
    }
}
