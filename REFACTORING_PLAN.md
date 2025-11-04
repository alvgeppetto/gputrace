# Package Reorganization Plan

## Goals

Following Go best practices (Russ Cox style):
1. Clear separation of concerns
2. Internal packages for implementation details
3. Minimal API surface in root package
4. Logical grouping by functionality
5. Simple, focused subcommands

## Proposed Structure

### Root Package (`gputrace`)
Keep only the essential public API:
- `Trace` struct (the main entry point)
- `Open()` function
- Core types that users need

### Internal Packages

#### `internal/trace/` - Core Trace Parsing
- `trace.go` - Trace struct, Open(), basic operations
- `metadata.go` - Metadata plist parsing
- `records.go` - MTSP/CS record parsing
- `index.go` - Index file parsing (store0, store_parser)
- `signpost.go` - Signpost/kdebug parsing

**Purpose**: Low-level trace file format handling

#### `internal/buffer/` - Buffer Analysis
- `buffer.go` - Buffer types and core functions
- `access.go` - Buffer access pattern analysis
- `diff.go` - Buffer comparison utilities
- `timeline.go` - Buffer timeline generation

**Purpose**: Everything related to Metal buffer analysis

#### `internal/shader/` - Shader Analysis
- `shader.go` - Shader types and core functions
- `metrics.go` - Shader performance metrics
- `source.go` - Source code mapping and attribution
- `correlation.go` - Shader name/address correlation
- `costs.go` - Shader cost analysis

**Purpose**: Shader-specific analysis and metrics

#### `internal/counter/` - Performance Counters
- `counter.go` - Counter types and structures
- `sampling.go` - Counter sampling implementation
- `export.go` - CSV export functionality
- `import.go` - CSV import functionality
- `validate.go` - Counter validation

**Purpose**: Hardware performance counter handling

#### `internal/timing/` - Timing Analysis
- `timing.go` - Core timing extraction
- `enhanced.go` - Enhanced timing with kdebug
- `metrics.go` - Timing metrics calculation
- `profiler.go` - Advanced timing profiling
- `store0.go` - Store0 timing data

**Purpose**: All timing-related functionality

#### `internal/replay/` - Trace Replay
- `replay.go` - Core replay engine
- `state.go` - State tracking during replay
- `metal.go` - Metal bridge (CGo)

**Purpose**: GPU trace replay functionality

#### `internal/export/` - Export Formats
- `pprof.go` - pprof format export (all variants)
- `timeline.go` - Chrome tracing format
- `csv.go` - CSV export utilities

**Purpose**: Converting traces to various output formats

#### `internal/analysis/` - High-Level Analysis
- `stats.go` - Trace statistics
- `insights.go` - Performance insights and recommendations
- `validate.go` - Validation utilities
- `device.go` - Device resources

**Purpose**: High-level analysis and reporting

#### `internal/command/` - Command Buffers
- `buffer.go` - Command buffer parsing
- `count.go` - Dispatch counting
- `encoder.go` - Encoder analysis

**Purpose**: Command buffer and encoder analysis

### Command Structure

Reorganize `cmd/gputrace/cmd/` by category:

#### Root (`root.go`)
- Keep root command definition
- Group subcommands by category in help text

#### Basic Information
- `stats.go` - Display trace statistics
- `dump.go` - Dump raw API calls
- `encoders.go` - List encoders

#### Shader Analysis
- `shaders.go` - Shader performance metrics
- `shader_metrics.go` - Alternative metrics
- `shader_source.go` - Source mapping
- `correlate.go` - Name/address correlation
- `perfcounters.go` - Hardware counters
- `perfcounters_validate.go` - Counter validation
- `xcode_counters.go` - Xcode format counters
- `replay_counters.go` - Replay with counters

#### Timing Analysis
- `timing.go` - Basic timing
- `timing_profiler.go` - Advanced profiling

#### Buffer Analysis
- `buffers.go` - List buffers
- `buffer_access.go` - Access patterns
- `buffers_diff.go` - Compare buffers
- `buffer_timeline.go` - Buffer timeline

#### Advanced Analysis
- `command_buffers.go` - Command buffer details
- `insights.go` - Performance insights
- `replay.go` - Replay traces

#### Export/Visualization
- `timeline.go` - Chrome tracing timeline
- `gputrace2pprof.go` - pprof export
- `export_counters.go` - Export counters

## Migration Strategy

### Phase 1: Create Internal Packages
1. Create `internal/` directory structure
2. Move and consolidate related code
3. Keep root package files temporarily for reference

### Phase 2: Update Imports
1. Update all internal imports to use new paths
2. Update command files to use internal packages
3. Ensure tests still pass

### Phase 3: Clean Root Package
1. Keep only essential public API in root
2. Remove now-internal code from root
3. Update documentation

### Phase 4: Command Reorganization
1. Consider grouping commands into subdirectories:
   - `cmd/gputrace/cmd/info/` (stats, dump, encoders)
   - `cmd/gputrace/cmd/shader/` (shader commands)
   - `cmd/gputrace/cmd/buffer/` (buffer commands)
   - `cmd/gputrace/cmd/export/` (export commands)
2. Or keep flat but with clear naming conventions

## Benefits

1. **Clearer Dependencies**: Each internal package has a focused purpose
2. **Better Testing**: Can test packages in isolation
3. **Easier Maintenance**: Related code is grouped together
4. **Public API Clarity**: Root package shows what's meant to be used externally
5. **Reduced Coupling**: Internal packages can be refactored without affecting users
6. **Standard Go Style**: Follows conventions used in Go standard library and tools

## Example: Russ Cox Style

This follows patterns from Go tools like:
- `go/` package (parser, ast, types, etc. in separate packages)
- `cmd/go/internal/` (modload, work, cache, etc.)
- Each package has a clear, single purpose
- Minimal API surface
- Good separation between parsing, analysis, and presentation

## File Mapping

### `internal/trace/`
- `gputrace.go` → `trace.go` (main Trace struct and Open)
- `index_parser.go` → `index.go`
- `store_parser.go` → (merge into `index.go`)
- `mtsp_records.go` → `records.go`
- `cs_records.go` → (merge into `records.go`)
- `signpost.go` → `signpost.go`
- `kdebug.go` → (merge into `signpost.go`)

### `internal/buffer/`
- `buffer_access.go` → `access.go`
- `buffer_diff.go` → `diff.go`
- `buffer_timeline.go` → `timeline.go`
- Extract buffer types from various files → `buffer.go`

### `internal/shader/`
- `shader_metrics.go` → `metrics.go`
- `shader_source_mapper.go` + `shader_source_attribution.go` → `source.go`
- `shader_correlation.go` → `correlation.go`
- `shader_costs.go` → `costs.go`
- Extract shader types → `shader.go`

### `internal/counter/`
- `perfcounters.go` → `counter.go`
- `counter_sampling.go` → `sampling.go`
- `csv_export.go` → `export.go`
- `csv_import.go` → `import.go`
- `validation.go` (counter parts) → `validate.go`

### `internal/timing/`
- `timing.go` → `timing.go`
- `enhanced_timing.go` → `enhanced.go`
- `timing_metrics.go` → `metrics.go`
- `timing_profiler_raw.go` → `profiler.go`
- `timing_v2.go` → (merge into `timing.go`)
- `store0_timing.go` → `store0.go`
- `synthetic_timing.go` → (merge or separate file)

### `internal/replay/`
- `replay.go` → `replay.go`
- `replay_state.go` → `state.go`
- `replay_metal.go` + `metal_bridge.go` → `metal.go`

### `internal/export/`
- `pprof.go` + `pprof_v2.go` + `pprof_enhanced.go` + `pprof_with_source.go` → `pprof.go`
- Timeline generation → `timeline.go`
- CSV utilities → `csv.go`

### `internal/analysis/`
- `statistics.go` → `stats.go`
- `insights.go` → `insights.go`
- `validation.go` → `validate.go`
- `device_resources.go` → `device.go`
- `analyze_trace.go` → (merge into `stats.go` or `insights.go`)

### `internal/command/`
- `command_buffer.go` → `buffer.go`
- `command_buffer_count.go` + `dispatch_count.go` → `count.go`
- Encoder analysis → `encoder.go`

## Testing Strategy

1. Keep all `*_test.go` files with their corresponding packages
2. Run `go test ./...` after each major move
3. Ensure golden tests still pass
4. Add integration tests for command-line tools

## Documentation Updates

1. Update README.md with new package structure
2. Add godoc comments to each internal package
3. Update ARCHITECTURE.md
4. Create package-level documentation files
