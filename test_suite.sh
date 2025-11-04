#!/bin/bash
# GPU Trace Test Suite
# Tests all gputrace commands with reference traces

# Don't exit on error - we want to run all tests
set +e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Test counters
TESTS_RUN=0
TESTS_PASSED=0
TESTS_FAILED=0
TESTS_SKIPPED=0

# Test traces
BASIC_TRACE="/tmp/llm-tool_1762220084.gputrace"
PERF_TRACE="/tmp/llm-tool_1762220084-perf.gputrace"
COUNTERS_CSV="/tmp/llm-tool_1762220084 Counters.csv"

# Output directory for test results
TEST_OUTPUT_DIR="/tmp/gputrace-test-results"
mkdir -p "$TEST_OUTPUT_DIR"

# Path to gputrace binary
GPUTRACE="./gputrace"

# Build gputrace if needed
if [ ! -x "$GPUTRACE" ]; then
    echo "Building gputrace..."
    go build ./cmd/gputrace
fi

# Helper functions
print_test_header() {
    echo ""
    echo "=========================================="
    echo "TEST: $1"
    echo "=========================================="
}

print_result() {
    local status=$1
    local message=$2

    case $status in
        PASS)
            echo -e "${GREEN}✓ PASS${NC}: $message"
            ((TESTS_PASSED++))
            ;;
        FAIL)
            echo -e "${RED}✗ FAIL${NC}: $message"
            ((TESTS_FAILED++))
            ;;
        SKIP)
            echo -e "${YELLOW}⊘ SKIP${NC}: $message"
            ((TESTS_SKIPPED++))
            ;;
    esac
    ((TESTS_RUN++))
}

check_file_exists() {
    local file=$1
    if [ ! -f "$file" ] && [ ! -d "$file" ]; then
        return 1
    fi
    return 0
}

check_output_not_empty() {
    local output=$1
    if [ -z "$output" ]; then
        return 1
    fi
    return 0
}

check_output_contains() {
    local output=$1
    local pattern=$2
    if echo "$output" | grep -q "$pattern"; then
        return 0
    fi
    return 1
}

# Test: stats command
test_stats() {
    print_test_header "stats command"

    if ! check_file_exists "$BASIC_TRACE"; then
        print_result SKIP "Basic trace not found: $BASIC_TRACE"
        return
    fi

    local output=$($GPUTRACE stats "$BASIC_TRACE" 2>&1)

    if check_output_contains "$output" "GPU Trace Statistics"; then
        print_result PASS "stats produces output with header"
    else
        print_result FAIL "stats output missing header"
    fi

    if check_output_contains "$output" "Kernel Statistics"; then
        print_result PASS "stats shows kernel statistics"
    else
        print_result FAIL "stats missing kernel statistics"
    fi
}

# Test: shaders command
test_shaders() {
    print_test_header "shaders command"

    if ! check_file_exists "$BASIC_TRACE"; then
        print_result SKIP "Basic trace not found"
        return
    fi

    local output=$($GPUTRACE shaders "$BASIC_TRACE" 2>&1)

    if check_output_contains "$output" "Cost"; then
        print_result PASS "shaders produces cost analysis"
    else
        print_result FAIL "shaders missing cost column"
    fi
}

# Test: timeline command (JSON format)
test_timeline_json() {
    print_test_header "timeline command (JSON)"

    if ! check_file_exists "$BASIC_TRACE"; then
        print_result SKIP "Basic trace not found"
        return
    fi

    local output_file="$TEST_OUTPUT_DIR/timeline.json"
    $GPUTRACE timeline "$BASIC_TRACE" --format json -o "$output_file" 2>&1

    if check_file_exists "$output_file"; then
        print_result PASS "timeline JSON file created"

        # Check if it's valid JSON
        if python3 -m json.tool "$output_file" > /dev/null 2>&1; then
            print_result PASS "timeline JSON is valid"
        else
            print_result FAIL "timeline JSON is invalid"
        fi
    else
        print_result FAIL "timeline JSON file not created"
    fi
}

# Test: timeline command (HTML format)
test_timeline_html() {
    print_test_header "timeline command (HTML)"

    if ! check_file_exists "$BASIC_TRACE"; then
        print_result SKIP "Basic trace not found"
        return
    fi

    local output_file="$TEST_OUTPUT_DIR/timeline.html"
    $GPUTRACE timeline "$BASIC_TRACE" --format html -o "$output_file" 2>&1

    if check_file_exists "$output_file"; then
        print_result PASS "timeline HTML file created"

        # Check if it contains expected HTML elements
        if grep -q "<!DOCTYPE html>" "$output_file"; then
            print_result PASS "timeline HTML has DOCTYPE"
        else
            print_result FAIL "timeline HTML missing DOCTYPE"
        fi
    else
        print_result FAIL "timeline HTML file not created"
    fi
}

# Test: xcode-counters command
test_xcode_counters() {
    print_test_header "xcode-counters command"

    if ! check_file_exists "$BASIC_TRACE"; then
        print_result SKIP "Basic trace not found"
        return
    fi

    if ! check_file_exists "$COUNTERS_CSV"; then
        print_result SKIP "Counters CSV not found: $COUNTERS_CSV"
        return
    fi

    local output=$($GPUTRACE xcode-counters "$BASIC_TRACE" 2>&1)

    if check_output_contains "$output" "Xcode Performance Counters"; then
        print_result PASS "xcode-counters produces output"
    else
        print_result FAIL "xcode-counters missing header"
    fi

    if check_output_contains "$output" "ALU Utilization"; then
        print_result PASS "xcode-counters shows ALU Utilization"
    else
        print_result FAIL "xcode-counters missing ALU Utilization"
    fi
}

# Test: export-counters command
test_export_counters() {
    print_test_header "export-counters command"

    if ! check_file_exists "$BASIC_TRACE"; then
        print_result SKIP "Basic trace not found"
        return
    fi

    local output_file="$TEST_OUTPUT_DIR/exported_counters.csv"
    $GPUTRACE export-counters "$BASIC_TRACE" -o "$output_file" 2>&1

    if check_file_exists "$output_file"; then
        print_result PASS "export-counters CSV created"

        # Check CSV has proper header
        if head -1 "$output_file" | grep -q "Index,Encoder FunctionIndex"; then
            print_result PASS "export-counters has correct CSV header"
        else
            print_result FAIL "export-counters CSV header incorrect"
        fi
    else
        print_result FAIL "export-counters CSV not created"
    fi
}

# Test: perfcounters command
test_perfcounters() {
    print_test_header "perfcounters command"

    if ! check_file_exists "$BASIC_TRACE"; then
        print_result SKIP "Basic trace not found"
        return
    fi

    # Check if trace has .gpuprofiler_raw directory
    local raw_dir="${BASIC_TRACE}.gpuprofiler_raw"
    if [ ! -d "$raw_dir" ]; then
        # Check inside trace
        raw_dir=$(find "$BASIC_TRACE" -name "*.gpuprofiler_raw" -type d 2>/dev/null | head -1)
        if [ -z "$raw_dir" ]; then
            print_result SKIP "No .gpuprofiler_raw directory found"
            return
        fi
    fi

    local output=$($GPUTRACE perfcounters "$BASIC_TRACE" 2>&1)

    if check_output_contains "$output" "GPU Hardware Performance Counters"; then
        print_result PASS "perfcounters produces output"
    else
        print_result FAIL "perfcounters missing header"
    fi
}

# Test: replay command
test_replay() {
    print_test_header "replay command"

    if ! check_file_exists "$BASIC_TRACE"; then
        print_result SKIP "Basic trace not found"
        return
    fi

    local output=$($GPUTRACE replay "$BASIC_TRACE" 2>&1)

    if check_output_contains "$output" "Replay Plan"; then
        print_result PASS "replay produces plan"
    else
        print_result FAIL "replay missing plan header"
    fi

    if check_output_contains "$output" "Command Buffers"; then
        print_result PASS "replay shows command buffers"
    else
        print_result FAIL "replay missing command buffers"
    fi
}

# Test: encoders command
test_encoders() {
    print_test_header "encoders command"

    if ! check_file_exists "$BASIC_TRACE"; then
        print_result SKIP "Basic trace not found"
        return
    fi

    local output=$($GPUTRACE encoders "$BASIC_TRACE" 2>&1)

    if check_output_contains "$output" "Compute Encoders"; then
        print_result PASS "encoders produces output"
    else
        print_result FAIL "encoders missing header"
    fi
}

# Test: buffers command
test_buffers() {
    print_test_header "buffers command"

    if ! check_file_exists "$BASIC_TRACE"; then
        print_result SKIP "Basic trace not found"
        return
    fi

    local output=$($GPUTRACE buffers "$BASIC_TRACE" 2>&1)

    # Check if it produces some output (may vary by trace format)
    if check_output_not_empty "$output"; then
        print_result PASS "buffers produces output"
    else
        print_result FAIL "buffers produces no output"
    fi
}

# Test: command-buffers command
test_command_buffers() {
    print_test_header "command-buffers command"

    if ! check_file_exists "$BASIC_TRACE"; then
        print_result SKIP "Basic trace not found"
        return
    fi

    local output=$($GPUTRACE command-buffers "$BASIC_TRACE" 2>&1)

    if check_output_not_empty "$output"; then
        print_result PASS "command-buffers produces output"
    else
        print_result FAIL "command-buffers produces no output"
    fi
}

# Test: insights command
test_insights() {
    print_test_header "insights command"

    if ! check_file_exists "$BASIC_TRACE"; then
        print_result SKIP "Basic trace not found"
        return
    fi

    local output=$($GPUTRACE insights "$BASIC_TRACE" 2>&1)

    if check_output_contains "$output" "Performance Insights"; then
        print_result PASS "insights produces recommendations"
    else
        print_result FAIL "insights missing header"
    fi
}

# Test: gputrace2pprof command
test_gputrace2pprof() {
    print_test_header "gputrace2pprof command"

    if ! check_file_exists "$BASIC_TRACE"; then
        print_result SKIP "Basic trace not found"
        return
    fi

    local output_file="$TEST_OUTPUT_DIR/profile.pb.gz"
    $GPUTRACE gputrace2pprof "$BASIC_TRACE" -o "$output_file" 2>&1

    if check_file_exists "$output_file"; then
        print_result PASS "gputrace2pprof creates pprof file"
    else
        print_result FAIL "gputrace2pprof failed to create file"
    fi
}

# Print test summary
print_summary() {
    echo ""
    echo "=========================================="
    echo "TEST SUMMARY"
    echo "=========================================="
    echo "Tests run:    $TESTS_RUN"
    echo -e "${GREEN}Passed:       $TESTS_PASSED${NC}"
    echo -e "${RED}Failed:       $TESTS_FAILED${NC}"
    echo -e "${YELLOW}Skipped:      $TESTS_SKIPPED${NC}"
    echo "=========================================="

    if [ $TESTS_FAILED -eq 0 ]; then
        echo -e "${GREEN}All tests passed!${NC}"
        exit 0
    else
        echo -e "${RED}Some tests failed!${NC}"
        exit 1
    fi
}

# Main test execution
main() {
    echo "GPU Trace Test Suite"
    echo "===================="
    echo "Basic trace: $BASIC_TRACE"
    echo "Perf trace:  $PERF_TRACE"
    echo "Counters CSV: $COUNTERS_CSV"
    echo ""

    # Check trace availability
    if ! check_file_exists "$BASIC_TRACE"; then
        echo -e "${YELLOW}Warning: Basic trace not found at $BASIC_TRACE${NC}"
        echo "Some tests will be skipped"
    fi

    # Run all tests
    test_stats
    test_shaders
    test_timeline_json
    test_timeline_html
    test_xcode_counters
    test_export_counters
    test_perfcounters
    test_replay
    test_encoders
    test_buffers
    test_command_buffers
    test_insights
    test_gputrace2pprof

    # Print summary
    print_summary
}

# Run main
main "$@"
