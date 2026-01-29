// Test workload: Debug labels and named buffers for GPU trace parsing validation
// Tests MLX debug labeling API with hierarchical labels and buffer names

#include <iostream>
#include "mlx/mlx.h"
#include "mlx/debug.h"
#include "mlx/backend/metal/debug.h"

namespace mx = mlx::core;

int main() {
  std::cout << "Running MLX debug labels test" << std::endl;

  // Enable debug labeling
  mx::metal::enable_debug_labels(true);
  mx::metal::start_capture("test_annotations.gputrace");

  // Create test data with meaningful names
  auto input_a = mx::random::uniform(-1.0f, 1.0f, {256, 128});
  auto input_b = mx::random::uniform(-1.0f, 1.0f, {128, 64});
  auto bias = mx::ones({64}) * 0.5f;

  mx::debug::set_buffer_label(input_a, "input_tensor_A");
  mx::debug::set_buffer_label(input_b, "input_tensor_B");
  mx::debug::set_buffer_label(bias, "bias_vector");

  // Test 1: Nested debug groups for forward pass
  mx::debug::push_label("training_iteration");
  mx::debug::push_label("forward_pass");
  mx::debug::push_label("linear_layer");

  auto matmul_result = mx::matmul(input_a, input_b);
  mx::debug::set_buffer_label(matmul_result, "matmul_output");
  mx::async_eval(matmul_result);

  auto biased = matmul_result + bias;
  mx::debug::set_buffer_label(biased, "biased_output");
  mx::async_eval(biased);

  mx::debug::pop_label(); // linear_layer
  mx::debug::push_label("activation");

  // ReLU activation
  auto activated = mx::maximum(biased, mx::array(0.0f));
  mx::debug::set_buffer_label(activated, "relu_output");
  mx::async_eval(activated);

  mx::debug::pop_label(); // activation
  mx::debug::pop_label(); // forward_pass

  // Test 2: Data preprocessing operations
  mx::debug::push_label("data_preprocessing");
  mx::debug::push_label("normalization");

  auto mean = mx::mean(activated, 1, /* keepdims= */ true);
  auto variance = mx::var(activated, 1, /* keepdims= */ true);
  auto normalized = (activated - mean) / mx::sqrt(variance + 1e-6f);

  mx::debug::set_buffer_label(mean, "activation_mean");
  mx::debug::set_buffer_label(variance, "activation_variance");
  mx::debug::set_buffer_label(normalized, "normalized_activations");

  mx::async_eval(normalized);

  mx::debug::pop_label(); // normalization
  mx::debug::pop_label(); // data_preprocessing

  // Test 3: Loss computation
  mx::debug::push_label("loss_computation");
  mx::debug::push_label("mse_loss");

  auto target = mx::random::uniform(0.0f, 1.0f, {256, 64});
  auto diff = normalized - target;
  auto squared = diff * diff;
  auto loss = mx::mean(squared);

  mx::debug::set_buffer_label(target, "target_values");
  mx::debug::set_buffer_label(diff, "prediction_error");
  mx::debug::set_buffer_label(squared, "squared_error");
  mx::debug::set_buffer_label(loss, "mean_squared_error");

  mx::async_eval(loss);

  mx::debug::pop_label(); // mse_loss
  mx::debug::push_label("regularization");

  // L2 regularization
  auto l2_penalty = 0.01f * mx::sum(input_b * input_b);
  auto total_loss = loss + l2_penalty;

  mx::debug::set_buffer_label(l2_penalty, "l2_regularization");
  mx::debug::set_buffer_label(total_loss, "total_loss");

  mx::eval(total_loss);

  mx::debug::pop_label(); // regularization
  mx::debug::pop_label(); // loss_computation

  // Test 4: Gradient computation
  mx::debug::push_label("backward_pass");
  mx::debug::push_label("compute_gradients");

  auto grad_output = 2.0f * diff / 256.0f;
  auto grad_weights = mx::matmul(mx::transpose(input_a), grad_output);
  auto grad_bias = mx::sum(grad_output, 0);

  mx::debug::set_buffer_label(grad_output, "loss_gradient");
  mx::debug::set_buffer_label(grad_weights, "weight_gradients");
  mx::debug::set_buffer_label(grad_bias, "bias_gradients");

  mx::async_eval({grad_weights, grad_bias});

  mx::debug::pop_label(); // compute_gradients
  mx::debug::push_label("gradient_clipping");

  auto grad_norm = mx::sqrt(mx::sum(grad_weights * grad_weights));
  auto clipped_grads = grad_weights / mx::maximum(grad_norm / 5.0f, mx::array(1.0f));

  mx::debug::set_buffer_label(grad_norm, "gradient_norm");
  mx::debug::set_buffer_label(clipped_grads, "clipped_gradients");

  mx::async_eval(clipped_grads);

  mx::debug::pop_label(); // gradient_clipping
  mx::debug::pop_label(); // backward_pass

  // Test 5: Optimizer step
  mx::debug::push_label("optimization_step");
  mx::debug::push_label("apply_updates");

  auto weight_update = input_b - 0.01f * clipped_grads;
  auto bias_update = bias - 0.01f * grad_bias;

  mx::debug::set_buffer_label(weight_update, "updated_weights");
  mx::debug::set_buffer_label(bias_update, "updated_bias");

  mx::eval({weight_update, bias_update});

  mx::debug::pop_label(); // apply_updates
  mx::debug::pop_label(); // optimization_step
  mx::debug::pop_label(); // training_iteration

  mx::metal::stop_capture();

  std::cout << "\nTest complete! GPU trace saved to test_annotations.gputrace\n";
  std::cout << "\nExpected annotations in trace:\n";
  std::cout << "  Debug groups (hierarchical):\n";
  std::cout << "    - training_iteration\n";
  std::cout << "      - forward_pass\n";
  std::cout << "        - linear_layer\n";
  std::cout << "        - activation\n";
  std::cout << "      - data_preprocessing\n";
  std::cout << "        - normalization\n";
  std::cout << "      - loss_computation\n";
  std::cout << "        - mse_loss\n";
  std::cout << "        - regularization\n";
  std::cout << "      - backward_pass\n";
  std::cout << "        - compute_gradients\n";
  std::cout << "        - gradient_clipping\n";
  std::cout << "      - optimization_step\n";
  std::cout << "        - apply_updates\n";
  std::cout << "\n  Buffer labels (23 total):\n";
  std::cout << "    - input_tensor_A, input_tensor_B, bias_vector\n";
  std::cout << "    - matmul_output, biased_output, relu_output\n";
  std::cout << "    - activation_mean, activation_variance, normalized_activations\n";
  std::cout << "    - target_values, prediction_error, squared_error, mean_squared_error\n";
  std::cout << "    - l2_regularization, total_loss\n";
  std::cout << "    - loss_gradient, weight_gradients, bias_gradients\n";
  std::cout << "    - gradient_norm, clipped_gradients\n";
  std::cout << "    - updated_weights, updated_bias\n";
  std::cout << "\nFinal loss: " << total_loss.item<float>() << "\n";
  std::cout << "\nOpen trace with: open test_annotations.gputrace\n";

  return 0;
}
