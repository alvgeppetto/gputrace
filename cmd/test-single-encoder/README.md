# Single Encoder Test - Binary Format Analysis

**Purpose:** Create minimal trace with exactly 1 compute encoder to validate binary format hypotheses.

## Hypothesis to Test

**Question:** Does a 1-encoder workload produce fewer counter files than a 6-encoder workload?

**Current observation:** 6-encoder LLM trace produces 40 counter files
**Expected:** 1-encoder trace should produce fewer files (proportional to encoder count)
**Alternative:** File count is constant regardless of encoder count

## Building

```bash
cd experiments/gputrace
clang -framework Metal -framework Foundation -o test-single-encoder cmd/test-single-encoder/main.m
```

## Running

```bash
# Run directly
./test-single-encoder

# Expected output:
# Running on: Apple M4 Max
# ✓ Single encoder test completed successfully
#   Dispatched: 16 threadgroups × 64 threads = 1024 threads
#   Expected in trace: 1 compute encoder
```

## Capturing Trace

### Option 1: Xcode Instruments

```bash
# 1. Open Xcode Instruments
# 2. Choose "GPU Counters" template
# 3. Select test-single-encoder as target
# 4. Record
# 5. Stop after completion
# 6. File > Export > Save as .gputrace

# Expected file: test-single-encoder.gputrace
```

### Option 2: Command Line (if available)

```bash
xcrun xctrace record \
    --template 'GPU Counters' \
    --launch ./test-single-encoder \
    --output test-single-encoder.trace

# Convert to .gputrace format
```

## Analysis

### Step 1: Count Counter Files

```bash
trace_dir="test-single-encoder.gputrace/test-single-encoder.gputrace.gpuprofiler_raw"
file_count=$(ls -1 "$trace_dir"/Counters_f_*.raw 2>/dev/null | wc -l)
echo "Counter files: $file_count"

# Compare with existing 6-encoder trace:
# - 6 encoders → 40 files
# - 1 encoder  → ? files
```

### Step 2: Export CSV

```bash
# In Instruments:
# File > Export > Counters > CSV
# Save as: test-single-encoder.csv

# Count CSV rows (should be 1 encoder)
csv_rows=$(tail -n +2 test-single-encoder.csv | wc -l)
echo "CSV encoder rows: $csv_rows"
```

### Step 3: Compare Patterns

```python
# Use existing analysis scripts
python3 /tmp/analyze_all_files.py \
    --trace test-single-encoder.gputrace \
    --csv test-single-encoder.csv
```

## Expected Outcomes

### Scenario A: File count proportional to encoder count
```
1 encoder → ~7 files (40 / 6 ≈ 7)
Implication: Can map file groups to encoders
```

### Scenario B: File count constant
```
1 encoder → 40 files (same as 6 encoders)
Implication: Files represent something else (samples, time periods, etc.)
```

### Scenario C: Different pattern entirely
```
1 encoder → X files (neither proportional nor constant)
Implication: Relationship is more complex
```

## Decision Matrix

| Outcome | File Count | Next Action |
|---------|------------|-------------|
| **Scenario A** | ~7 files | ✅ Promising - collect 2, 3, 4 encoder traces |
| **Scenario B** | 40 files | ⚠️ Complex - investigate metadata records |
| **Scenario C** | Other | 🤔 Reassess hypothesis, need more data |

## Workload Characteristics

**Simplicity by design:**
- Single kernel function (`simple_add`)
- Single compute encoder
- Fixed data size (1024 floats)
- No branches or complex logic
- Minimal ALU operations (1 add per thread)
- No memory pressure

**Expected metrics:**
- Kernel Invocations: 1 (single dispatch)
- Threads: 1024
- Threadgroups: 16
- ALU Utilization: Low (~5-10%, simple add operation)
- Occupancy: High (minimal register usage)

## Comparison with LLM Trace

| Metric | LLM Trace (6 enc) | Single Encoder | Ratio |
|--------|-------------------|----------------|-------|
| Encoders | 6 | 1 | 6:1 |
| Counter Files | 40 | ? | ? |
| Total Invocations | 4,877,700 | 1 | ~4.9M:1 |
| Complexity | High (ML ops) | Low (add) | - |

## Follow-up Tests

If initial test is promising:

### Test 2: Two Encoders
```metal
// Two sequential dispatches
encoder1: simple_add (1024 threads)
encoder2: simple_multiply (1024 threads)
```

### Test 3: Three Encoders
```metal
// Three sequential dispatches
encoder1: add
encoder2: multiply
encoder3: subtract
```

### Test 4: Known Invocation Count
```metal
// Dispatch with exact known invocation count
// Makes it easy to search for value in binary
dispatch(threadgroups: 100, threads_per_group: 10)
// Expected invocations: 1000
```

## Timeline

- **Build & test:** 10 minutes
- **Capture trace:** 5 minutes
- **Initial analysis:** 15 minutes
- **Compare with existing:** 30 minutes
- **Total:** ~1 hour for initial validation

## Success Criteria

✅ **Minimum success:** Understand file-to-encoder relationship
✅ **Ideal success:** Find pattern enabling file-to-encoder mapping
✅ **Decision made:** Continue full investigation or defer to P3

---

**Status:** Ready to execute
**Next step:** Build, run, capture trace, analyze
