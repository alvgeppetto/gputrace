# Field Offset Analysis - Initial Findings

**Bead:** gputrace-44 / gputrace-48
**Date:** 2025-11-03
**Trace:** `/tmp/fast-llm-mlx-test-perf.gputrace` (M4 Max)

## Executive Summary

Initial hexdump and Python analysis of Counters_f_0.raw confirms the aggregation complexity documented in PERFCOUNTER_BINARY_FORMAT.md. Direct field offset extraction is **not feasible** without implementing full aggregation logic.

## Analysis Method

### Tools Used
1. **Hexdump correlation** - Searching for known CSV values in binary
2. **Python analysis script** (`/tmp/analyze_counters.py`) - Systematic record extraction
3. **Reference CSV** - Known validation targets from Instruments

### Target Values
From `/tmp/fast-llm-mlx-test Counters.csv` Row 1:
- Kernel Invocations: 1,237,392 (0x12E310)
- ALU Utilization: 0.98
- Kernel Occupancy: 0.30
- Device Memory Bandwidth: 16.44 GB/s

## Key Findings

### 1. Record Structure Confirmed

**Record Types:**
- **Metadata records**: 2,300-2,900 bytes
  - Example: Record 0 at offset 0xF85 = 2,898 bytes
  - Example: Record 2 at offset 0x1CA7 = 2,409 bytes
- **Sample records**: 464 bytes
  - Example: Record 1 at offset 0x1AD7 = 464 bytes

**Total in Counters_f_0.raw**: 1,598 records

### 2. Aggregation Ratio

```
1,598 records → 10 CSV rows = ~160 records per CSV row
```

For Kernel Invocations (Row 1 = 1,237,392):
```
Average per record: 1,237,392 / 1,598 = 774.34 invocations
```

This means each 464-byte sample record contains ~774 kernel invocations worth of data, which then gets summed across ~160 records to produce the CSV value.

### 3. Hexdump Search Results

**Searched for**: `10 e3 12 00` (Kernel Invocations = 1,237,392 in little-endian)

**Result**: ❌ No matches found

**Reason**: The value 1,237,392 is an **aggregated sum**, not stored directly in any single record. Individual records contain smaller per-sample values that must be:
1. Grouped by encoder/command buffer
2. Summed across multiple records
3. Exported as CSV row

### 4. Per-Sample Value Analysis

From Python script analysis of sample records (464 bytes):

**Record 1 interesting uint32 values:**
```
Offset  Value       Notes
0x0064  28,416      Possible invocation count?
0x00a0  8,257,536   Too large for single invocation
0x00cc  5           Small count
0x0100  12,040      Possible invocation count?
0x010c  21          Small count
0x0114  3,932,160   Large value
0x0130  1           Flag?
0x0144  3,968       Moderate count
0x0160  19          Small count
0x0178  3,678,208   Large value
0x01a0  3           Small count
```

**Candidate fields for Kernel Invocations:**
- Offset 0x0064: 28,416 (if summed across 43 records = 1,221,888 ≈ target)
- Offset 0x0100: 12,040 (if summed across 102 records = 1,228,080 ≈ target)

However, without knowing which encoder each record belongs to, cannot confirm.

### 5. Metadata Record Analysis

**Record 0 (2,898 bytes) interesting values:**
```
Offset  Value       Notes
0x0094  113,664     Large count
0x01b4  1,801       Possible encoder ID?
0x01d8  7,208,960   Very large value
0x0224  23,424      Moderate count
0x0254  684,032     Large count
0x0318  456,108     Large count
0x0338  1,052,672   Very large value
0x0358  1,687,552   Very large value
```

Metadata records likely contain:
- Encoder identification (address, index)
- Frame/timing context
- Aggregation group markers
- Summary statistics

## Implementation Implications

### What's Required for Binary Parsing

1. **Record Type Identification**
   - Distinguish metadata (2.3-2.9 KB) from samples (464 bytes)
   - Metadata: First record of each encoder group?
   - Samples: Subsequent records for that encoder

2. **Encoder Grouping**
   - Find encoder ID field in metadata records
   - Associate subsequent sample records with that encoder
   - Group all samples belonging to same encoder

3. **Field Offset Mapping** (per 464-byte sample)
   - Kernel Invocations: Offset TBD (candidates: 0x0064, 0x0100)
   - ALU Utilization: Float at offset TBD
   - Occupancy: Float at offset TBD
   - Memory bytes: Uint64 at offset TBD

4. **Aggregation Logic** (per metric type)
   - **Sum**: Kernel Invocations, bytes transferred
   - **Average**: ALU Utilization, Occupancy (percentages)
   - **Bandwidth calc**: Bytes / time
   - **Min/Max**: Duration tracking

5. **Validation**
   - Extract → group → aggregate → compare with CSV
   - Verify per-encoder values match Instruments
   - Test across multiple traces/architectures

### Estimated Complexity

**Phase 1 (5-10 core metrics):**
- Record type detection: 0.5 days
- Encoder grouping logic: 1-2 days
- Field offset discovery (trial & error): 2-3 days
- Aggregation implementation: 1-2 days
- Validation: 1 day
- **Total: 5.5-8.5 days**

**Known Challenges:**
- Field offsets may vary by GPU architecture (M1/M2/M3/M4)
- No documentation → pure reverse engineering
- Validation difficult (Instruments as only ground truth)
- Fragile (breaks with OS updates)

## Recommendations

### Option 1: Continue Binary Parsing (Current Path)

**Pros:**
- Direct access to Apple's profiler data
- No replay overhead
- Educational value (reverse engineering)

**Cons:**
- 5-8 days for Phase 1 only (10 metrics)
- Weeks for complete 241 metrics
- Fragile, undocumented format
- High maintenance cost

**Next Steps:**
1. Identify encoder ID field in metadata records
2. Implement grouping logic
3. Trial-and-error field offset discovery
4. Build aggregation framework
5. Validate against CSV

### Option 2: Pivot to Metal Replay (Recommended)

As documented in `PERFCOUNTER_IMPLEMENTATION_RECOMMENDATION.md`:

**Pros:**
- Public MTLCounterSampleBuffer API
- 3-5 days total (vs weeks for binary)
- All 241+ metrics available
- Stable, documented, maintained by Apple
- Easy validation

**Cons:**
- Requires replay implementation
- Some overhead for replay execution

**Next Steps:**
1. Implement replay engine (gputrace-53, gputrace-41)
2. Add MTLCounterSampleBuffer sampling (gputrace-54)
3. Export CSV format (gputrace-55)

## Decision Point

**Question**: Proceed with binary parsing OR pivot to Metal replay?

**Data from this analysis**:
- ✅ Confirmed aggregation complexity (160:1 ratio)
- ✅ Confirmed no simple field extraction possible
- ✅ Estimated 5-8 days for Phase 1 (vs 3-5 days for complete Metal replay)
- ❌ Field offsets still unknown (requires more investigation)
- ❌ Encoder grouping logic undefined

**My recommendation**: **Pivot to Metal Replay** (gputrace-53).

Reasons:
1. Faster time to complete solution (3-5 vs 5-8+ days)
2. More reliable and maintainable
3. Access to all metrics, not just Phase 1 subset
4. Public API with Apple support
5. This analysis confirms the implementation complexity

## Files Created

- `/tmp/analyze_counters.py` - Python analysis script (104 lines)
- `docs/FIELD_OFFSET_ANALYSIS.md` - This document

## References

- `docs/REFERENCE_TRACE.md` - Reference trace specifications
- `docs/PERFCOUNTER_BINARY_FORMAT.md` - Binary format analysis
- `docs/PERFCOUNTER_IMPLEMENTATION_RECOMMENDATION.md` - Implementation approaches
- `docs/COUNTERS_CSV_FORMAT.md` - CSV format specification

## Status

**Analysis Complete**: ✅
**Field Offsets Identified**: ❌ (requires encoder grouping first)
**Recommendation**: Pivot to Metal Replay

---

**This analysis provides data-driven confirmation of the implementation complexity and strengthens the recommendation for Metal Replay approach.**
