// Test workload: Single encoder dispatch for binary format analysis
// Deliberately simple to produce minimal counter files
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

        printf("Running on: %s\n", [[device name] UTF8String]);

        // Simple compute shader source
        NSString *shaderSource = @
            "#include <metal_stdlib>\n"
            "using namespace metal;\n"
            "\n"
            "kernel void simple_add(\n"
            "    device float* inputA [[buffer(0)]],\n"
            "    device float* inputB [[buffer(1)]],\n"
            "    device float* output [[buffer(2)]],\n"
            "    uint id [[thread_position_in_grid]])\n"
            "{\n"
            "    output[id] = inputA[id] + inputB[id];\n"
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

        id<MTLFunction> function = [library newFunctionWithName:@"simple_add"];
        id<MTLComputePipelineState> pipeline = [device newComputePipelineStateWithFunction:function
                                                                                      error:&error];
        if (!pipeline) {
            fprintf(stderr, "Failed to create pipeline: %s\n",
                    [[error localizedDescription] UTF8String]);
            return 1;
        }

        // Create buffers
        NSUInteger bufferSize = 1024 * sizeof(float);
        id<MTLBuffer> bufferA = [device newBufferWithLength:bufferSize
                                                     options:MTLResourceStorageModeShared];
        id<MTLBuffer> bufferB = [device newBufferWithLength:bufferSize
                                                     options:MTLResourceStorageModeShared];
        id<MTLBuffer> bufferC = [device newBufferWithLength:bufferSize
                                                     options:MTLResourceStorageModeShared];

        // Initialize data
        float *dataA = (float *)[bufferA contents];
        float *dataB = (float *)[bufferB contents];
        for (int i = 0; i < 1024; i++) {
            dataA[i] = i;
            dataB[i] = i * 2;
        }

        // Create command queue and buffer
        id<MTLCommandQueue> queue = [device newCommandQueue];
        id<MTLCommandBuffer> commandBuffer = [queue commandBuffer];
        commandBuffer.label = @"SingleEncoderTest";

        // Create compute encoder
        id<MTLComputeCommandEncoder> encoder = [commandBuffer computeCommandEncoder];
        encoder.label = @"SimpleAdd";

        [encoder setComputePipelineState:pipeline];
        [encoder setBuffer:bufferA offset:0 atIndex:0];
        [encoder setBuffer:bufferB offset:0 atIndex:1];
        [encoder setBuffer:bufferC offset:0 atIndex:2];

        // Dispatch: 1024 threads, 64 threads per group = 16 thread groups
        MTLSize gridSize = MTLSizeMake(1024, 1, 1);
        MTLSize threadGroupSize = MTLSizeMake(64, 1, 1);
        [encoder dispatchThreads:gridSize threadsPerThreadgroup:threadGroupSize];

        [encoder endEncoding];

        // Commit and wait
        [commandBuffer commit];
        [commandBuffer waitUntilCompleted];

        // Verify result
        float *dataC = (float *)[bufferC contents];
        bool success = true;
        for (int i = 0; i < 1024; i++) {
            float expected = dataA[i] + dataB[i];
            if (fabs(dataC[i] - expected) > 0.001) {
                printf("Mismatch at index %d: expected %.2f, got %.2f\n",
                       i, expected, dataC[i]);
                success = false;
                break;
            }
        }

        if (success) {
            printf("✓ Single encoder test completed successfully\n");
            printf("  Dispatched: 16 threadgroups × 64 threads = 1024 threads\n");
            printf("  Expected in trace: 1 compute encoder\n");
        } else {
            printf("✗ Test failed\n");
            return 1;
        }
    }
    return 0;
}
