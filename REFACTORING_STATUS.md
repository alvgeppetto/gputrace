# Package Reorganization Status

## Completed

1. ✅ Created internal/ directory structure with 9 subpackages:
   - internal/trace - Core trace parsing
   - internal/buffer - Buffer analysis  
   - internal/shader - Shader analysis
   - internal/counter - Performance counters
   - internal/timing - Timing analysis
   - internal/replay - Trace replay
   - internal/export - Export formats
   - internal/analysis - High-level analysis
   - internal/command - Command buffer analysis

2. ✅ Moved all relevant files using `git mv` to preserve history

3. ✅ Fixed package declarations in all moved files

4. ✅ Created root `gputrace.go` with clean public API

## Current Issue: Import Cycle

We have an import cycle:
```
gputrace → trace → command → trace
```

This occurs because:
- `trace` package has methods that return `command.CommandBuffer` types
- `command` package defines methods on `*trace.Trace`

## Resolution Plan

**Option 1: Move CommandBuffer types to trace** (Recommended)
- CommandBuffer is parsed from trace data, so logically belongs in trace
- command package focuses on higher-level analysis (counting, dispatch analysis)
- This follows the principle: types belong where they're created/parsed

**Option 2: Use interfaces**
- Define interface in trace, implement in command
- More complex, less idiomatic Go

**Option 3: Merge command parsing back into trace**
- Keep command package only for analysis functions
- Simpler but less separation of concerns

## Next Steps

1. Move CommandBuffer/ComputeEncoder/DispatchCall types to internal/trace
2. Keep analysis methods in internal/command
3. Run tests
4. Update documentation

## File Mapping Reference

All files have been moved but imports need final fixes after resolving cycle.

See REFACTORING_PLAN.md for the complete reorganization design.
