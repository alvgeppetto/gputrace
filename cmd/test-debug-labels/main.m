// Test workload: Debug labels and named buffers for GPU trace parsing validation
// Tests PushDebugGroup/PopDebugGroup and buffer label parsing
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

        printf("Running debug labels test on: %s\n", [[device name] UTF8String]);

        // Simple compute shader source
        NSString *shaderSource = @
            "#include <metal_stdlib>\n"
            "using namespace metal;\n"
            "\n"
            "kernel void vector_add(\n"
            "    device float* inputA [[buffer(0)]],\n"
            "    device float* inputB [[buffer(1)]],\n"
            "    device float* output [[buffer(2)]],\n"
            "    uint id [[thread_position_in_grid]])\n"
            "{\n"
            "    output[id] = inputA[id] + inputB[id];\n"
            "}\n"
            "\n"
            "kernel void vector_mul(\n"
            "    device float* inputA [[buffer(0)]],\n"
            "    device float* inputB [[buffer(1)]],\n"
            "    device float* output [[buffer(2)]],\n"
            "    uint id [[thread_position_in_grid]])\n"
            "{\n"
            "    output[id] = inputA[id] * inputB[id];\n"
            "}\n"
            "\n"
            "kernel void vector_scale(\n"
            "    device float* input [[buffer(0)]],\n"
            "    device float* output [[buffer(1)]],\n"
            "    constant float& scale [[buffer(2)]],\n"
            "    uint id [[thread_position_in_grid]])\n"
            "{\n"
            "    output[id] = input[id] * scale;\n"
            "}\n";

        // Compile shader
        NSError *error = nil;
        id<MTLLibrary> library = [device newLibraryWithSource:shaderSource
                                                       options:nil
                                                         error:&error];
        if (!library) {
            fprintf(stderr, "Failed to compile shader: %s\n",
                    [[error localizedDescription] UTF8String]);
            return 1;
        }

        id<MTLFunction> addFunc = [library newFunctionWithName:@"vector_add"];
        id<MTLFunction> mulFunc = [library newFunctionWithName:@"vector_mul"];
        id<MTLFunction> scaleFunc = [library newFunctionWithName:@"vector_scale"];

        id<MTLComputePipelineState> addPipeline = [device newComputePipelineStateWithFunction:addFunc error:&error];
        id<MTLComputePipelineState> mulPipeline = [device newComputePipelineStateWithFunction:mulFunc error:&error];
        id<MTLComputePipelineState> scalePipeline = [device newComputePipelineStateWithFunction:scaleFunc error:&error];

        if (!addPipeline || !mulPipeline || !scalePipeline) {
            fprintf(stderr, "Failed to create pipelines\n");
            return 1;
        }

        // Create buffers with meaningful labels
        NSUInteger bufferSize = 1024 * sizeof(float);
        id<MTLBuffer> inputBufferA = [device newBufferWithLength:bufferSize
                                                          options:MTLResourceStorageModeShared];
        inputBufferA.label = @"input_tensor_A";

        id<MTLBuffer> inputBufferB = [device newBufferWithLength:bufferSize
                                                          options:MTLResourceStorageModeShared];
        inputBufferB.label = @"input_tensor_B";

        id<MTLBuffer> tempBuffer = [device newBufferWithLength:bufferSize
                                                        options:MTLResourceStorageModeShared];
        tempBuffer.label = @"temp_computation_result";

        id<MTLBuffer> outputBuffer = [device newBufferWithLength:bufferSize
                                                          options:MTLResourceStorageModeShared];
        outputBuffer.label = @"final_output";

        id<MTLBuffer> scaleBuffer = [device newBufferWithLength:sizeof(float)
                                                         options:MTLResourceStorageModeShared];
        scaleBuffer.label = @"scale_factor";

        // Initialize data
        float *dataA = (float *)[inputBufferA contents];
        float *dataB = (float *)[inputBufferB contents];
        float *scaleData = (float *)[scaleBuffer contents];
        *scaleData = 2.5f;

        for (int i = 0; i < 1024; i++) {
            dataA[i] = i * 0.1f;
            dataB[i] = i * 0.2f;
        }

        // Create command queue and buffer with labels
        id<MTLCommandQueue> queue = [device newCommandQueue];
        queue.label = @"MainComputeQueue";

        id<MTLCommandBuffer> commandBuffer = [queue commandBuffer];
        commandBuffer.label = @"ForwardPass";

        // === Test 1: Nested Debug Groups ===
        [commandBuffer pushDebugGroup:@"training_iteration"];
        [commandBuffer pushDebugGroup:@"forward_pass"];

        // First operation: vector_add
        id<MTLComputeCommandEncoder> addEncoder = [commandBuffer computeCommandEncoder];
        addEncoder.label = @"VectorAddition";
        [addEncoder pushDebugGroup:@"compute_add"];

        [addEncoder setComputePipelineState:addPipeline];
        [addEncoder setBuffer:inputBufferA offset:0 atIndex:0];
        [addEncoder setBuffer:inputBufferB offset:0 atIndex:1];
        [addEncoder setBuffer:tempBuffer offset:0 atIndex:2];

        MTLSize gridSize = MTLSizeMake(1024, 1, 1);
        NSUInteger threadGroupSize = addPipeline.maxTotalThreadsPerThreadgroup;
        if (threadGroupSize > 1024) threadGroupSize = 1024;
        MTLSize threadgroupSize = MTLSizeMake(threadGroupSize, 1, 1);

        [addEncoder dispatchThreads:gridSize threadsPerThreadgroup:threadgroupSize];
        [addEncoder popDebugGroup];
        [addEncoder endEncoding];

        [commandBuffer popDebugGroup]; // forward_pass

        // === Test 2: Nested operation in different group ===
        [commandBuffer pushDebugGroup:@"optimization_step"];

        // Second operation: vector_mul
        id<MTLComputeCommandEncoder> mulEncoder = [commandBuffer computeCommandEncoder];
        mulEncoder.label = @"VectorMultiply";
        [mulEncoder pushDebugGroup:@"compute_multiply"];

        [mulEncoder setComputePipelineState:mulPipeline];
        [mulEncoder setBuffer:tempBuffer offset:0 atIndex:0];
        [mulEncoder setBuffer:inputBufferB offset:0 atIndex:1];
        [mulEncoder setBuffer:tempBuffer offset:0 atIndex:2];

        [mulEncoder dispatchThreads:gridSize threadsPerThreadgroup:threadgroupSize];
        [mulEncoder popDebugGroup];
        [mulEncoder endEncoding];

        // Third operation: vector_scale
        id<MTLComputeCommandEncoder> scaleEncoder = [commandBuffer computeCommandEncoder];
        scaleEncoder.label = @"ApplyScaling";
        [scaleEncoder pushDebugGroup:@"apply_scale_factor"];

        [scaleEncoder setComputePipelineState:scalePipeline];
        [scaleEncoder setBuffer:tempBuffer offset:0 atIndex:0];
        [scaleEncoder setBuffer:outputBuffer offset:0 atIndex:1];
        [scaleEncoder setBuffer:scaleBuffer offset:0 atIndex:2];

        threadGroupSize = scalePipeline.maxTotalThreadsPerThreadgroup;
        if (threadGroupSize > 1024) threadGroupSize = 1024;
        threadgroupSize = MTLSizeMake(threadGroupSize, 1, 1);

        [scaleEncoder dispatchThreads:gridSize threadsPerThreadgroup:threadgroupSize];
        [scaleEncoder popDebugGroup];
        [scaleEncoder endEncoding];

        [commandBuffer popDebugGroup]; // optimization_step
        [commandBuffer popDebugGroup]; // training_iteration

        // Commit and wait
        [commandBuffer commit];
        [commandBuffer waitUntilCompleted];

        // Verify results
        float *output = (float *)[outputBuffer contents];
        printf("Sample results:\n");
        for (int i = 0; i < 5; i++) {
            float expected = ((i * 0.1f + i * 0.2f) * (i * 0.2f)) * 2.5f;
            printf("  output[%d] = %.2f (expected: %.2f)\n", i, output[i], expected);
        }

        printf("\nTest complete! GPU trace should contain:\n");
        printf("  - Command buffer label: 'ForwardPass'\n");
        printf("  - Debug groups: 'training_iteration', 'forward_pass', 'optimization_step'\n");
        printf("  - Encoder labels: 'VectorAddition', 'VectorMultiply', 'ApplyScaling'\n");
        printf("  - Encoder debug groups: 'compute_add', 'compute_multiply', 'apply_scale_factor'\n");
        printf("  - Buffer labels: 'input_tensor_A', 'input_tensor_B', 'temp_computation_result', 'final_output', 'scale_factor'\n");

        return 0;
    }
}
