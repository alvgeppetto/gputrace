# Kernel Occupancy Data Location

**Date:** 2025-11-04
**Bead:** gputrace-64
**Investigation:** Locate Kernel Occupancy in .gpuprofiler_raw binary format

## Key Finding

**Kernel Occupancy data is stored in `Profiling_f_*.raw` files, NOT in `Counters_f_*.raw` files.**

## Evidence

### Test Trace: 01-single-encoder
- **CSV Value:** Kernel Occupancy = 0.09 (9%)
- **Binary Location:** `Profiling_f_0.raw`
- **Offsets Found:**
  - 0x28450: 0.092401
  - 0x290b8: 0.092279
  - 0x334a8: 0.086229
  - 0x40e78: 0.092401
  - 0x41da8: 0.088481
  - 0x428b8: 0.092401

### Test Trace: 06-six-encoders
Expected CSV values:
- Encoder 1: 0.09 (simple_add)
- Encoder 2: 0.09 (simple_multiply)
- Encoder 3: 0.08 (simple_subtract)
- Encoder 4: 0.09 (simple_divide)
- Encoder 5: **0.47** (complex_math) ← Distinctive value
- Encoder 6: 0.15 (low_register_pressure)

**Binary Location for Encoder 5 (0.47):**
- File: `Profiling_f_4.raw`
- Offset: 0x645cc
- Value: 0.470202 (float32)

## File Structure

### .gpuprofiler_raw Directory Contents
```
trace.gputrace.gpuprofiler_raw/
├── Counters_f_0.raw       # 32 KB  - Counter samples (NOT occupancy)
├── Counters_f_1.raw       # 32 KB
├── ...
├── Counters_f_N.raw
├── Profiling_f_0.raw      # 256-606 KB - Profiling metrics (CONTAINS occupancy)
├── Profiling_f_1.raw
├── ...
├── Profiling_f_N.raw
├── Timeline_f_0.raw       # Timeline data
└── streamData             # Stream metadata
```

### Data Format
- **Encoding:** IEEE 754 float32 (little-endian)
- **Value Range:** 0.0 to 1.0 (CSV shows same range, e.g., 0.09 not 9.0)
- **Multiple Occurrences:** Occupancy value appears multiple times per encoder
  - Likely: per-sample or per-warp measurements that get averaged

## Implications for Implementation

### Current Implementation Issue
The current `counter.ParsePerfCounters()` function reads from `Counters_f_*.raw`:
```go
files, err := filepath.Glob(filepath.Join(perfDir, "Counters_f_*.raw"))
```

This is why Kernel Occupancy extraction returns 0.00% - it's looking in the wrong files.

### Required Changes

1. **Add Profiling File Parser**
   - Parse `Profiling_f_*.raw` files (not Counters files)
   - Files are larger (256KB-606KB vs 32KB)
   - Contains full profiling metrics including occupancy

2. **Search Strategy**
   - Scan for float32 values in reasonable occupancy range (0.01-1.0)
   - Multiple samples per encoder need aggregation (likely average)
   - Correlate Profiling_f_N with encoder indices

3. **Record Structure**
   - Profiling files have 0x4E record markers (same as Counters)
   - But records are much larger and variable-sized
   - Occupancy appears deep within records (e.g., offset +0x68aa in record 7)

### Validation Approach
1. Extract all float32 values in range 0.01-1.0 from Profiling files
2. Group by proximity (values close together likely same encoder)
3. Average multiple samples per encoder
4. Match count to CSV row count
5. Validate against CSV values (should match within ~0.01)

## Next Steps

1. Create `ParseProfilingFiles()` function in `internal/counter/profiling.go`
2. Implement occupancy extraction logic
3. Update `ParsePerfCounters()` to call both Counters and Profiling parsers
4. Validate against test traces:
   - `testdata/traces/01-single-encoder/01-single-encoder-run1-perf.gputrace`
   - `testdata/traces/06-six-encoders/06-six-encoders-run1-perf.gputrace`
5. Update CLI to display kernel occupancy

## Search Command Used
```bash
# Build searcher
go build -o /tmp/find-occupancy ./cmd/find-occupancy

# Search for value
/tmp/find-occupancy path/to/Profiling_f_0.raw 0.09
```

## References
- CSV Format: `docs/COUNTERS_CSV_FORMAT.md`
- Binary Format: `docs/PERFCOUNTER_BINARY_FORMAT.md`
- Counter Parsing: `internal/counter/counter.go`
