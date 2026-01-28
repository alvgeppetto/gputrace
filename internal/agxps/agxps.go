//go:build darwin

// Package agxps provides purego bindings to Apple's agxps C API from GTShaderProfiler.framework.
//
// The agxps_aps_* functions provide access to GPU profiler data including:
//   - Kick (encoder) timing
//   - ESL clique (dispatch) timing
//   - Work clique timing
//   - Instruction traces
//   - Counter data
//
// # Usage
//
// There are two approaches to get parsed profile data:
//
// 1. ObjC GTShaderProfilerStreamDataProcessor (recommended):
//   - Load streamData from .gpuprofiler_raw/streamData using NSKeyedUnarchiver
//   - Create GTShaderProfilerStreamDataProcessor with the unarchived data
//   - Call processStreamData and waitUntilTimelineFinished
//   - Get result which provides profile data handles
//   - See /tmp/test_gtshaderprofiler.go for example
//
// 2. Direct C parser API (requires specific initialization):
//   - The agxps_aps_parser_* functions are available but require specific
//     initialization parameters that are not documented.
//   - Use approach 1 for production code.
//
// Once you have a profile data handle from either approach, use the query functions:
//   - GetKickTimings - encoder execution times
//   - GetESLCliqueTimings - per-dispatch execution times
//   - GetInstructionTraceStats - instruction trace statistics
//
// See internal/timing/profiler_raw.go for an alternative plist-based implementation
// that parses the streamData directly without using Apple's libraries.
package agxps

import (
	"fmt"
	"unsafe"

	"github.com/ebitengine/purego"
)

const gpuPluginPath = "/Applications/Xcode.app/Contents/Applications/Instruments.app/Contents/PlugIns/GPUPlugin.xrplugin/Contents/MacOS/GPUPlugin"

// GTShaderProfiler has more complete agxps API including clique/ESL functions
const gtShaderProfilerPath = "/Applications/Xcode.app/Contents/PlugIns/GPUDebugger.ideplugin/Contents/Frameworks/GTShaderProfiler.framework/GTShaderProfiler"

// GPU is an opaque handle for GPU configuration.
// Create with CreateGPU(gen, variant, rev) where:
//   - gen: 13 (M1), 14 (M2), 15 (M3), 16 (A17)
//   - variant: typically 0
//   - rev: typically 0
type GPU uint64

// ProfileData is an opaque handle for parsed profile data.
type ProfileData uint64

// ParserHandle is an opaque handle for a parser instance.
type ParserHandle uint64

// Descriptor configures the parser for parsing trace data.
// This struct must match the C layout expected by agxps_aps_descriptor_create.
type Descriptor struct {
	GPU                    GPU
	PulsePeriod            uint32
	EraPeriod              uint32
	CountPeriod            uint32
	ChunkSize              uint64
	CounterUarchBehaviour  int32
	ExcludeFlags           int32
	MinTimestamp           uint64
	MaxTimestamp           uint64
	CountersFilter         uintptr // char**
	CountersFilterSize     uint64
	TimestampSyncPointData uintptr
	TimestampSyncPointSize uint64
	MaxParseErrorCount     uint32
	_                      uint32 // padding
	TimebaseOffset         uint64
}

// Typed function variables for C API bindings
var (
	libHandle uintptr

	// Core initialization
	agxpsInitialize func() int32

	// GPU functions - use typed signatures for proper argument passing
	gpuCreate      func(gen, variant, rev uint32) GPU
	gpuDestroy     func(gpu GPU)
	gpuIsValid     func(gpu GPU) bool
	gpuGetGen      func(gpu GPU) uint32
	gpuGetVariant  func(gpu GPU) uint32
	gpuGetRev      func(gpu GPU) uint32
	gpuFormatName  func(gpu GPU, buf *byte, size uint64) int32
	gpuIsSupported func(gpu GPU) bool

	// Descriptor for parser configuration - takes pointer to Descriptor struct
	apsDescriptorCreate func(desc *Descriptor) uintptr

	// Parser lifecycle - typed signatures
	apsParserCreate  func(desc uintptr) ParserHandle
	apsParserDestroy func(parser ParserHandle)
	apsParserIsValid func(parser ParserHandle) bool
	apsParserParse   func(parser ParserHandle, data unsafe.Pointer, size uint64, profileDataOut *ProfileData) int32

	// Profile data - typed signatures
	apsProfileDataDestroy func(pd ProfileData)
	apsProfileDataIsValid func(pd ProfileData) bool

	// Kick (encoder execution) timing - typed signatures
	apsProfileDataGetKicksNum  func(pd ProfileData) uint64
	apsProfileDataGetKickStart func(pd ProfileData, idx uint64) uint64
	apsProfileDataGetKickEnd   func(pd ProfileData, idx uint64) uint64
	apsProfileDataGetKickID    func(pd ProfileData, idx uint64) uint64

	// Timestamp conversion
	apsSystemTimestampToNanoseconds func(...uintptr) uintptr

	// System timestamps
	apsProfileDataGetSystemTimestamps    func(...uintptr) uintptr
	apsProfileDataGetSystemTimestampsNum func(...uintptr) uintptr

	// Chunk timing
	apsProfileDataGetChunkEndTime        func(...uintptr) uintptr
	apsProfileDataGetChunkFirstTimestamp func(...uintptr) uintptr

	// Timing analyzer
	apsTimingAnalyzerCreate                    func(...uintptr) uintptr
	apsTimingAnalyzerDestroy                   func(...uintptr) uintptr
	apsTimingAnalyzerProcessUsc                func(...uintptr) uintptr
	apsTimingAnalyzerFinish                    func(...uintptr) uintptr
	apsTimingAnalyzerGetWorkCliquesAvgDuration func(...uintptr) uintptr
	apsTimingAnalyzerGetWorkCliquesMinDuration func(...uintptr) uintptr
	apsTimingAnalyzerGetWorkCliquesMaxDuration func(...uintptr) uintptr
	apsTimingAnalyzerGetNumCommands            func(...uintptr) uintptr
	apsTimingAnalyzerGetNumWorkCliques         func(...uintptr) uintptr

	// ESL Clique functions (from GTShaderProfiler)
	apsProfileDataGetEslCliquesNum             func(...uintptr) uintptr
	apsProfileDataGetEslCliqueStart            func(...uintptr) uintptr
	apsProfileDataGetEslCliqueEnd              func(...uintptr) uintptr
	apsProfileDataGetEslCliqueCliqueID         func(...uintptr) uintptr
	apsProfileDataGetEslCliqueKickID           func(...uintptr) uintptr
	apsProfileDataGetEslCliqueEslID            func(...uintptr) uintptr
	apsProfileDataGetEslCliqueMissingEnd       func(...uintptr) uintptr
	apsProfileDataGetEslCliqueInstructionTrace func(...uintptr) uintptr

	// Work Clique functions
	apsProfileDataGetWorkCliquesNum             func(...uintptr) uintptr
	apsProfileDataGetWorkCliqueStart            func(...uintptr) uintptr
	apsProfileDataGetWorkCliqueEnd              func(...uintptr) uintptr
	apsProfileDataGetWorkCliqueInstructionTrace func(...uintptr) uintptr

	// Clique instruction trace functions
	apsCliqueInstructionTraceGetTimestampReferences    func(...uintptr) uintptr
	apsCliqueInstructionTraceGetTimestampReferencesNum func(...uintptr) uintptr
	apsCliqueInstructionTraceGetExecutionEvents        func(...uintptr) uintptr
	apsCliqueInstructionTraceGetExecutionEventsNum     func(...uintptr) uintptr
	apsCliqueInstructionTraceGetInstructionStats       func(...uintptr) uintptr
	apsCliqueInstructionTraceGetPcAdvances             func(...uintptr) uintptr
	apsCliqueInstructionTraceGetPcAdvancesNum          func(...uintptr) uintptr

	// Clique time stats
	apsCliqueTimeStatsCreate func(...uintptr) uintptr
)

// Init loads the GPU profiler libraries and registers function symbols.
// Tries GTShaderProfiler first (more complete API), falls back to GPUPlugin.
// Returns an error if neither can be loaded.
func Init() error {
	var err error

	// Try GTShaderProfiler first - has more complete API including ESL clique functions
	libHandle, err = purego.Dlopen(gtShaderProfilerPath, purego.RTLD_LAZY|purego.RTLD_GLOBAL)
	if err != nil {
		// Fall back to GPUPlugin
		libHandle, err = purego.Dlopen(gpuPluginPath, purego.RTLD_LAZY|purego.RTLD_GLOBAL)
		if err != nil {
			return fmt.Errorf("failed to load GPU profiler library: %w", err)
		}
	}

	// Register core initialization
	registerFunc("agxps_initialize", &agxpsInitialize)

	// Register GPU functions
	registerFunc("agxps_gpu_create", &gpuCreate)
	registerFunc("agxps_gpu_destroy", &gpuDestroy)
	registerFunc("agxps_gpu_is_valid", &gpuIsValid)
	registerFunc("agxps_gpu_get_gen", &gpuGetGen)
	registerFunc("agxps_gpu_get_variant", &gpuGetVariant)
	registerFunc("agxps_gpu_get_rev", &gpuGetRev)
	registerFunc("agxps_gpu_format_name", &gpuFormatName)
	registerFunc("agxps_aps_gpu_is_supported", &gpuIsSupported)

	// Register descriptor and parser functions
	registerFunc("agxps_aps_descriptor_create", &apsDescriptorCreate)
	registerFunc("agxps_aps_parser_create", &apsParserCreate)
	registerFunc("agxps_aps_parser_destroy", &apsParserDestroy)
	registerFunc("agxps_aps_parser_parse", &apsParserParse)
	registerFunc("agxps_aps_parser_is_valid", &apsParserIsValid)

	// Register profile data functions
	registerFunc("agxps_aps_profile_data_destroy", &apsProfileDataDestroy)
	registerFunc("agxps_aps_profile_data_is_valid", &apsProfileDataIsValid)

	// Register kick timing functions
	registerFunc("agxps_aps_profile_data_get_kicks_num", &apsProfileDataGetKicksNum)
	registerFunc("agxps_aps_profile_data_get_kick_start", &apsProfileDataGetKickStart)
	registerFunc("agxps_aps_profile_data_get_kick_end", &apsProfileDataGetKickEnd)
	registerFunc("agxps_aps_profile_data_get_kick_id", &apsProfileDataGetKickID)

	// Register timestamp functions
	registerFunc("agxps_aps_system_timestamp_to_nanoseconds", &apsSystemTimestampToNanoseconds)
	registerFunc("agxps_aps_profile_data_get_system_timestamps", &apsProfileDataGetSystemTimestamps)
	registerFunc("agxps_aps_profile_data_get_system_timestamps_num", &apsProfileDataGetSystemTimestampsNum)

	// Register chunk timing functions
	registerFunc("agxps_aps_profile_data_get_chunk_end_time", &apsProfileDataGetChunkEndTime)
	registerFunc("agxps_aps_profile_data_get_chunk_first_timestamp", &apsProfileDataGetChunkFirstTimestamp)

	// Register timing analyzer functions
	registerFunc("agxps_aps_timing_analyzer_create", &apsTimingAnalyzerCreate)
	registerFunc("agxps_aps_timing_analyzer_destroy", &apsTimingAnalyzerDestroy)
	registerFunc("agxps_aps_timing_analyzer_process_usc", &apsTimingAnalyzerProcessUsc)
	registerFunc("agxps_aps_timing_analyzer_finish", &apsTimingAnalyzerFinish)
	registerFunc("agxps_aps_timing_analyzer_get_work_cliques_average_duration", &apsTimingAnalyzerGetWorkCliquesAvgDuration)
	registerFunc("agxps_aps_timing_analyzer_get_work_cliques_min_duration", &apsTimingAnalyzerGetWorkCliquesMinDuration)
	registerFunc("agxps_aps_timing_analyzer_get_work_cliques_max_duration", &apsTimingAnalyzerGetWorkCliquesMaxDuration)
	registerFunc("agxps_aps_timing_analyzer_get_num_commands", &apsTimingAnalyzerGetNumCommands)
	registerFunc("agxps_aps_timing_analyzer_get_num_work_cliques", &apsTimingAnalyzerGetNumWorkCliques)

	// Register ESL clique functions (GTShaderProfiler)
	registerFunc("agxps_aps_profile_data_get_esl_cliques_num", &apsProfileDataGetEslCliquesNum)
	registerFunc("agxps_aps_profile_data_get_esl_clique_start", &apsProfileDataGetEslCliqueStart)
	registerFunc("agxps_aps_profile_data_get_esl_clique_end", &apsProfileDataGetEslCliqueEnd)
	registerFunc("agxps_aps_profile_data_get_esl_clique_clique_id", &apsProfileDataGetEslCliqueCliqueID)
	registerFunc("agxps_aps_profile_data_get_esl_clique_kick_id", &apsProfileDataGetEslCliqueKickID)
	registerFunc("agxps_aps_profile_data_get_esl_clique_esl_id", &apsProfileDataGetEslCliqueEslID)
	registerFunc("agxps_aps_profile_data_get_esl_clique_missing_end", &apsProfileDataGetEslCliqueMissingEnd)
	registerFunc("agxps_aps_profile_data_get_esl_clique_instruction_trace", &apsProfileDataGetEslCliqueInstructionTrace)

	// Register Work clique functions
	registerFunc("agxps_aps_profile_data_get_work_cliques_num", &apsProfileDataGetWorkCliquesNum)
	registerFunc("agxps_aps_profile_data_get_work_clique_start", &apsProfileDataGetWorkCliqueStart)
	registerFunc("agxps_aps_profile_data_get_work_clique_end", &apsProfileDataGetWorkCliqueEnd)
	registerFunc("agxps_aps_profile_data_get_work_clique_instruction_trace", &apsProfileDataGetWorkCliqueInstructionTrace)

	// Register clique instruction trace functions
	registerFunc("agxps_aps_clique_instruction_trace_get_timestamp_references", &apsCliqueInstructionTraceGetTimestampReferences)
	registerFunc("agxps_aps_clique_instruction_trace_get_timestamp_references_num", &apsCliqueInstructionTraceGetTimestampReferencesNum)
	registerFunc("agxps_aps_clique_instruction_trace_get_execution_events", &apsCliqueInstructionTraceGetExecutionEvents)
	registerFunc("agxps_aps_clique_instruction_trace_get_execution_events_num", &apsCliqueInstructionTraceGetExecutionEventsNum)
	registerFunc("agxps_aps_clique_instruction_trace_get_instruction_stats", &apsCliqueInstructionTraceGetInstructionStats)
	registerFunc("agxps_aps_clique_instruction_trace_get_pc_advances", &apsCliqueInstructionTraceGetPcAdvances)
	registerFunc("agxps_aps_clique_instruction_trace_get_pc_advances_num", &apsCliqueInstructionTraceGetPcAdvancesNum)

	// Register clique time stats
	registerFunc("agxps_aps_clique_time_stats_create", &apsCliqueTimeStatsCreate)

	return nil
}

func registerFunc(name string, fn interface{}) {
	// Use RegisterLibFunc which properly handles library functions
	// Symbol names don't need underscore prefix with RegisterLibFunc
	purego.RegisterLibFunc(fn, libHandle, name)
}

// Close releases the library handle.
func Close() {
	if libHandle != 0 {
		purego.Dlclose(libHandle)
		libHandle = 0
	}
}

// IsLoaded returns true if the library was successfully loaded.
func IsLoaded() bool {
	return libHandle != 0
}

// Parser wraps the agxps_aps_parser for parsing timeline data.
type Parser struct {
	handle ParserHandle
}

// Initialize calls agxps_initialize to set up the library.
// This must be called after Init() but before parser operations.
//
// NOTE: agxps_initialize actually takes 4 arguments (discovered via disassembly),
// not void as the symbol name suggests. Calling with wrong/no args causes:
//   - Returns error 1 when called from Go (purego puts zeros in arg registers)
//   - SIGSEGV when called from native C/ObjC (garbage in arg registers)
//
// The proper arguments are likely resource paths or configuration pointers
// that are set up by Xcode/Instruments runtime. Without knowing the exact
// signature, this function will always fail outside of Xcode context.
func Initialize() error {
	if agxpsInitialize == nil {
		return fmt.Errorf("agxps_initialize not available")
	}
	result := agxpsInitialize()
	if result != 0 {
		return fmt.Errorf("agxps_initialize returned error: %d (requires Xcode runtime context)", result)
	}
	return nil
}

// NewParser creates a new timeline data parser.
// The parser requires a descriptor; use NewParserWithDescriptor for full control.
func NewParser() (*Parser, error) {
	return nil, fmt.Errorf("parser creation requires descriptor - use NewParserWithGPU or NewParserWithDescriptor")
}

// NewParserWithGPU creates a parser configured for the specified GPU.
func NewParserWithGPU(gpu GPU) (*Parser, error) {
	if apsDescriptorCreate == nil || apsParserCreate == nil {
		return nil, fmt.Errorf("parser functions not available")
	}

	// Allocate descriptor and let agxps initialize it with defaults
	// agxps_aps_descriptor_create initializes memory with:
	//   - GPU: invalid handle (to be set)
	//   - ChunkSize: 0x1000 (4096)
	//   - MaxParseErrorCount: 0x32 (50)
	//   - Other fields: zeroed
	desc := &Descriptor{}
	descPtr := apsDescriptorCreate(desc)
	if descPtr == 0 {
		return nil, fmt.Errorf("failed to initialize descriptor")
	}

	// Set the GPU after initialization
	desc.GPU = gpu
	desc.ChunkSize = 262144 // Default from CE95's findings

	handle := apsParserCreate(descPtr)
	if !apsParserIsValid(handle) {
		return nil, fmt.Errorf("failed to create parser")
	}
	return &Parser{handle: handle}, nil
}

// NewParserWithDescriptor creates a parser with an explicit descriptor.
func NewParserWithDescriptor(desc *Descriptor) (*Parser, error) {
	if apsDescriptorCreate == nil {
		return nil, fmt.Errorf("descriptor_create not available")
	}
	if apsParserCreate == nil {
		return nil, fmt.Errorf("parser_create not available")
	}

	descPtr := apsDescriptorCreate(desc)
	if descPtr == 0 {
		return nil, fmt.Errorf("failed to create descriptor")
	}

	handle := apsParserCreate(descPtr)
	if !apsParserIsValid(handle) {
		return nil, fmt.Errorf("failed to create parser")
	}
	return &Parser{handle: handle}, nil
}

// Close destroys the parser.
func (p *Parser) Close() {
	if p.handle != 0 && apsParserDestroy != nil {
		apsParserDestroy(p.handle)
		p.handle = 0
	}
}

// Parse parses timeline data from a byte slice.
// Returns the profile data handle on success.
func (p *Parser) Parse(data []byte) (ProfileData, error) {
	if apsParserParse == nil {
		return 0, fmt.Errorf("parser_parse not available")
	}
	if len(data) == 0 {
		return 0, fmt.Errorf("empty data")
	}

	var pd ProfileData
	result := apsParserParse(p.handle, unsafe.Pointer(&data[0]), uint64(len(data)), &pd)
	if result != 0 {
		return 0, fmt.Errorf("parse failed with code %d", result)
	}

	return pd, nil
}

// IsValid returns true if the parser is in a valid state.
func (p *Parser) IsValid() bool {
	if apsParserIsValid == nil {
		return false
	}
	return apsParserIsValid(p.handle)
}

// IsValid returns true if the profile data handle is valid.
func (pd ProfileData) IsValid() bool {
	return apsProfileDataIsValid != nil && apsProfileDataIsValid(pd)
}

// Destroy releases the profile data.
func (pd ProfileData) Destroy() {
	if apsProfileDataDestroy != nil && pd != 0 {
		apsProfileDataDestroy(pd)
	}
}

// KickTiming represents timing data for a single GPU kick (encoder execution).
type KickTiming struct {
	Index       uint64
	ID          uint64
	StartTimeNs uint64
	EndTimeNs   uint64
	DurationNs  uint64
}

// GetKickTimings extracts kick timing data from parsed profile data.
func GetKickTimings(profileData ProfileData) ([]KickTiming, error) {
	if apsProfileDataGetKicksNum == nil {
		return nil, fmt.Errorf("get_kicks_num not available")
	}

	numKicks := apsProfileDataGetKicksNum(profileData)
	if numKicks == 0 {
		return nil, nil
	}

	timings := make([]KickTiming, numKicks)
	for i := uint64(0); i < numKicks; i++ {
		var startNs, endNs uint64

		if apsProfileDataGetKickStart != nil {
			startTs := apsProfileDataGetKickStart(profileData, i)
			// TODO: convert to nanoseconds if needed
			startNs = startTs
		}

		if apsProfileDataGetKickEnd != nil {
			endTs := apsProfileDataGetKickEnd(profileData, i)
			// TODO: convert to nanoseconds if needed
			endNs = endTs
		}

		var kickID uint64
		if apsProfileDataGetKickID != nil {
			kickID = apsProfileDataGetKickID(profileData, i)
		}

		timings[i] = KickTiming{
			Index:       i,
			ID:          kickID,
			StartTimeNs: startNs,
			EndTimeNs:   endNs,
			DurationNs:  endNs - startNs,
		}
	}

	return timings, nil
}

// TimingStats represents aggregate timing statistics.
type TimingStats struct {
	NumCommands uint64
	AvgDuration float64
	MinDuration float64
	MaxDuration float64
}

// GetTimingStats extracts timing statistics from a timing analyzer.
func GetTimingStats(analyzer uintptr) TimingStats {
	var stats TimingStats

	if apsTimingAnalyzerGetNumCommands != nil {
		stats.NumCommands = uint64(apsTimingAnalyzerGetNumCommands(analyzer))
	}
	if apsTimingAnalyzerGetWorkCliquesAvgDuration != nil {
		stats.AvgDuration = uintptrToFloat64(apsTimingAnalyzerGetWorkCliquesAvgDuration(analyzer))
	}
	if apsTimingAnalyzerGetWorkCliquesMinDuration != nil {
		stats.MinDuration = uintptrToFloat64(apsTimingAnalyzerGetWorkCliquesMinDuration(analyzer))
	}
	if apsTimingAnalyzerGetWorkCliquesMaxDuration != nil {
		stats.MaxDuration = uintptrToFloat64(apsTimingAnalyzerGetWorkCliquesMaxDuration(analyzer))
	}

	return stats
}

// uintptrToFloat64 converts a uintptr (containing float64 bits) to float64.
func uintptrToFloat64(u uintptr) float64 {
	bits := uint64(u)
	return *(*float64)(unsafe.Pointer(&bits))
}

// ESLCliqueTiming represents timing data for a single ESL clique (dispatch execution).
// ESL = Execution Scheduling Layer, a clique is a group of threads scheduled together.
type ESLCliqueTiming struct {
	Index      uint64
	CliqueID   uint64
	KickID     uint64 // Parent encoder ID
	EslID      uint64
	StartTime  uint64 // Raw timestamp
	EndTime    uint64 // Raw timestamp
	Duration   uint64 // EndTime - StartTime
	MissingEnd bool   // True if end time wasn't captured
}

// GetESLCliqueTimings extracts ESL clique timing data from parsed profile data.
// This provides per-dispatch timing granularity (finer than per-encoder).
func GetESLCliqueTimings(profileData ProfileData) ([]ESLCliqueTiming, error) {
	if apsProfileDataGetEslCliquesNum == nil {
		return nil, fmt.Errorf("get_esl_cliques_num not available (need GTShaderProfiler)")
	}

	pd := uintptr(profileData)
	numCliques := uint64(apsProfileDataGetEslCliquesNum(pd))
	if numCliques == 0 {
		return nil, nil
	}

	timings := make([]ESLCliqueTiming, numCliques)
	for i := uint64(0); i < numCliques; i++ {
		var t ESLCliqueTiming
		t.Index = i
		idx := uintptr(i)

		if apsProfileDataGetEslCliqueStart != nil {
			t.StartTime = uint64(apsProfileDataGetEslCliqueStart(pd, idx))
		}
		if apsProfileDataGetEslCliqueEnd != nil {
			t.EndTime = uint64(apsProfileDataGetEslCliqueEnd(pd, idx))
		}
		if apsProfileDataGetEslCliqueCliqueID != nil {
			t.CliqueID = uint64(apsProfileDataGetEslCliqueCliqueID(pd, idx))
		}
		if apsProfileDataGetEslCliqueKickID != nil {
			t.KickID = uint64(apsProfileDataGetEslCliqueKickID(pd, idx))
		}
		if apsProfileDataGetEslCliqueEslID != nil {
			t.EslID = uint64(apsProfileDataGetEslCliqueEslID(pd, idx))
		}
		if apsProfileDataGetEslCliqueMissingEnd != nil {
			t.MissingEnd = apsProfileDataGetEslCliqueMissingEnd(pd, idx) != 0
		}

		t.Duration = t.EndTime - t.StartTime
		timings[i] = t
	}

	return timings, nil
}

// GetESLCliqueInstructionTrace returns the instruction trace handle for a clique.
func GetESLCliqueInstructionTrace(profileData ProfileData, cliqueIndex uint64) uintptr {
	if apsProfileDataGetEslCliqueInstructionTrace == nil {
		return 0
	}
	return apsProfileDataGetEslCliqueInstructionTrace(uintptr(profileData), uintptr(cliqueIndex))
}

// InstructionTraceStats represents statistics from an instruction trace.
type InstructionTraceStats struct {
	NumTimestampRefs   uint64
	NumExecutionEvents uint64
	NumPcAdvances      uint64
}

// GetInstructionTraceStats returns statistics about an instruction trace.
func GetInstructionTraceStats(trace uintptr) InstructionTraceStats {
	var stats InstructionTraceStats

	if apsCliqueInstructionTraceGetTimestampReferencesNum != nil {
		stats.NumTimestampRefs = uint64(apsCliqueInstructionTraceGetTimestampReferencesNum(trace))
	}
	if apsCliqueInstructionTraceGetExecutionEventsNum != nil {
		stats.NumExecutionEvents = uint64(apsCliqueInstructionTraceGetExecutionEventsNum(trace))
	}
	if apsCliqueInstructionTraceGetPcAdvancesNum != nil {
		stats.NumPcAdvances = uint64(apsCliqueInstructionTraceGetPcAdvancesNum(trace))
	}

	return stats
}

// CreateCliqueTimeStats creates a time stats object for a specific clique.
func CreateCliqueTimeStats(profileData ProfileData, cliqueIndex uint64) uintptr {
	if apsCliqueTimeStatsCreate == nil {
		return 0
	}
	return apsCliqueTimeStatsCreate(uintptr(profileData), uintptr(cliqueIndex))
}

// CreateGPU creates a GPU handle for the given generation, variant, and revision.
// Common values:
//   - gen: 13 (M1), 14 (M2), 15 (M3), 16 (A17)
//   - variant: typically 0
//   - rev: typically 0
func CreateGPU(gen, variant, rev uint32) (GPU, error) {
	if gpuCreate == nil {
		return 0, fmt.Errorf("gpu_create not available")
	}
	gpu := gpuCreate(gen, variant, rev)
	if !gpuIsValid(gpu) {
		return 0, fmt.Errorf("failed to create GPU for gen=%d variant=%d rev=%d", gen, variant, rev)
	}
	return gpu, nil
}

// IsValid returns true if the GPU handle is valid.
func (g GPU) IsValid() bool {
	return gpuIsValid != nil && gpuIsValid(g)
}

// Destroy releases the GPU handle.
func (g GPU) Destroy() {
	if gpuDestroy != nil && g != 0 {
		gpuDestroy(g)
	}
}

// Gen returns the GPU generation.
func (g GPU) Gen() uint32 {
	if gpuGetGen == nil {
		return 0
	}
	return gpuGetGen(g)
}

// Variant returns the GPU variant.
func (g GPU) Variant() uint32 {
	if gpuGetVariant == nil {
		return 0
	}
	return gpuGetVariant(g)
}

// Rev returns the GPU revision.
func (g GPU) Rev() uint32 {
	if gpuGetRev == nil {
		return 0
	}
	return gpuGetRev(g)
}

// Name returns the formatted GPU name.
func (g GPU) Name() string {
	if gpuFormatName == nil {
		return ""
	}
	buf := make([]byte, 256)
	gpuFormatName(g, &buf[0], 256)
	// Find null terminator
	for i, b := range buf {
		if b == 0 {
			return string(buf[:i])
		}
	}
	return string(buf)
}

// IsSupported returns true if the GPU is supported for profiling.
func (g GPU) IsSupported() bool {
	if gpuIsSupported == nil {
		return false
	}
	return gpuIsSupported(g)
}
