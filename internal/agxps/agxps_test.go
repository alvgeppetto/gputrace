//go:build darwin

package agxps

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInit(t *testing.T) {
	err := Init()
	if err != nil {
		t.Skipf("Skipping test - GTShaderProfiler not available: %v", err)
	}
	defer Close()

	if !IsLoaded() {
		t.Fatal("Library not loaded after Init")
	}
}

func TestParseTimelineData(t *testing.T) {
	err := Init()
	if err != nil {
		t.Skipf("Skipping test - GTShaderProfiler not available: %v", err)
	}
	defer Close()

	// Look for Timeline_f_*.raw files in the test trace
	traceDir := "/private/tmp/mlx-lm-generate_tokens_8_to_9-perfdata.gputrace"
	profilerRawDir := filepath.Join(traceDir, "mlx-lm-generate_tokens_8_to_9.gputrace.gpuprofiler_raw")

	if _, err := os.Stat(profilerRawDir); os.IsNotExist(err) {
		t.Skipf("Test trace not found at %s", traceDir)
	}

	// Find Timeline_f_*.raw files
	matches, err := filepath.Glob(filepath.Join(profilerRawDir, "Timeline_f_*.raw"))
	if err != nil || len(matches) == 0 {
		t.Skipf("No Timeline_f_*.raw files found in %s", profilerRawDir)
	}

	t.Logf("Found %d Timeline_f_*.raw files", len(matches))

	// Try parsing the first one
	rawData, err := os.ReadFile(matches[0])
	if err != nil {
		t.Fatalf("Failed to read %s: %v", matches[0], err)
	}

	t.Logf("Read %d bytes from %s", len(rawData), filepath.Base(matches[0]))

	parser, err := NewParser()
	if err != nil {
		// Parser may not be available in GTShaderProfiler - that's OK
		// The ESL clique functions are still available
		t.Logf("Parser not available (expected with GTShaderProfiler): %v", err)
		t.Log("Note: ESL clique functions are still available for use with pre-parsed profile data")
		return
	}
	defer parser.Close()

	profileData, err := parser.Parse(rawData)
	if err != nil {
		t.Logf("Parse failed (expected for raw timeline data): %v", err)
		// This is expected - the raw files might need different handling
		return
	}

	t.Logf("Parse succeeded, profileData handle: %#x", profileData)

	// Try to get kick timings
	kickTimings, err := GetKickTimings(profileData)
	if err != nil {
		t.Logf("GetKickTimings failed: %v", err)
	} else {
		t.Logf("Got %d kick timings", len(kickTimings))
		for i, kt := range kickTimings {
			if i < 5 {
				t.Logf("  Kick %d: ID=%d start=%d end=%d dur=%d",
					kt.Index, kt.ID, kt.StartTimeNs, kt.EndTimeNs, kt.DurationNs)
			}
		}
	}

	// Try to get ESL clique timings
	eslTimings, err := GetESLCliqueTimings(profileData)
	if err != nil {
		t.Logf("GetESLCliqueTimings failed: %v", err)
	} else {
		t.Logf("Got %d ESL clique timings", len(eslTimings))
		for i, ct := range eslTimings {
			if i < 5 {
				t.Logf("  Clique %d: cliqueID=%d kickID=%d start=%d end=%d dur=%d",
					ct.Index, ct.CliqueID, ct.KickID, ct.StartTime, ct.EndTime, ct.Duration)
			}
		}

		// Get instruction trace for first clique
		if len(eslTimings) > 0 {
			trace := GetESLCliqueInstructionTrace(profileData, 0)
			if trace != 0 {
				stats := GetInstructionTraceStats(trace)
				t.Logf("  First clique instruction trace: timestamps=%d events=%d pcAdvances=%d",
					stats.NumTimestampRefs, stats.NumExecutionEvents, stats.NumPcAdvances)
			}
		}
	}
}

func TestESLCliqueFunctionsAvailable(t *testing.T) {
	err := Init()
	if err != nil {
		t.Skipf("Skipping test - GTShaderProfiler not available: %v", err)
	}
	defer Close()

	// Check that ESL clique functions were registered
	funcs := []struct {
		name string
		fn   any
	}{
		{"apsProfileDataGetEslCliquesNum", apsProfileDataGetEslCliquesNum},
		{"apsProfileDataGetEslCliqueStart", apsProfileDataGetEslCliqueStart},
		{"apsProfileDataGetEslCliqueEnd", apsProfileDataGetEslCliqueEnd},
		{"apsProfileDataGetEslCliqueCliqueID", apsProfileDataGetEslCliqueCliqueID},
		{"apsProfileDataGetEslCliqueKickID", apsProfileDataGetEslCliqueKickID},
		{"apsCliqueInstructionTraceGetTimestampReferencesNum", apsCliqueInstructionTraceGetTimestampReferencesNum},
		{"apsCliqueInstructionTraceGetExecutionEventsNum", apsCliqueInstructionTraceGetExecutionEventsNum},
	}

	for _, f := range funcs {
		if f.fn == nil {
			t.Logf("  %s: NOT available", f.name)
		} else {
			t.Logf("  %s: available", f.name)
		}
	}
}

func TestParserFunctionsAvailable(t *testing.T) {
	err := Init()
	if err != nil {
		t.Skipf("Skipping test - GTShaderProfiler not available: %v", err)
	}
	defer Close()

	// Log which functions are available
	// NOTE: The C parser API (agxps_aps_parser_*) requires specific initialization
	// that we don't fully understand. The ObjC GTShaderProfilerStreamDataProcessor
	// is the recommended way to parse streamData.
	t.Logf("apsDescriptorCreate available: %v", apsDescriptorCreate != nil)
	t.Logf("apsParserCreate available: %v", apsParserCreate != nil)
	t.Logf("apsParserParse available: %v", apsParserParse != nil)
	t.Logf("apsProfileDataIsValid available: %v", apsProfileDataIsValid != nil)

	// The C API query functions work with valid profile data handles
	// These are obtained from GTShaderProfilerStreamDataProcessor.result
	// after processing streamData via ObjC
	t.Log("Note: Use ObjC GTShaderProfilerStreamDataProcessor to parse streamData,")
	t.Log("then use C API query functions on the resulting profile data handle.")
}

func TestGPUCreation(t *testing.T) {
	err := Init()
	if err != nil {
		t.Skipf("Skipping test - GTShaderProfiler not available: %v", err)
	}
	defer Close()

	// Test GPU creation for various generations
	gpuGens := []struct {
		name    string
		gen     uint32
		variant uint32
		rev     uint32
	}{
		{"M1", 13, 0, 0},
		{"M2", 14, 0, 0},
		{"M3", 15, 0, 0},
		{"A17", 16, 0, 0},
	}

	t.Log("Testing GPU creation...")
	for _, g := range gpuGens {
		gpu, err := CreateGPU(g.gen, g.variant, g.rev)
		if err != nil {
			t.Logf("  %s (gen=%d): failed - %v", g.name, g.gen, err)
			continue
		}
		defer gpu.Destroy()

		name := gpu.Name()
		supported := gpu.IsSupported()
		t.Logf("  %s (gen=%d): created! name=%q valid=%v supported=%v",
			g.name, g.gen, name, gpu.IsValid(), supported)
	}
}

func TestParserWithGPU(t *testing.T) {
	err := Init()
	if err != nil {
		t.Skipf("Skipping test - GTShaderProfiler not available: %v", err)
	}
	defer Close()

	// Try agxps_initialize (returns error 1 outside Xcode/Metal context)
	if err := Initialize(); err != nil {
		t.Logf("Initialize returned error (expected outside Xcode): %v", err)
		t.Log("Note: agxps library requires Metal device context or Xcode runtime")
	}

	// Create GPU for M2 (gen=14) which we know works
	gpu, err := CreateGPU(14, 0, 0)
	if err != nil {
		t.Skipf("Failed to create GPU: %v", err)
	}
	defer gpu.Destroy()
	t.Logf("Created GPU: gen=%d name=%q", gpu.Gen(), gpu.Name())

	// Note: Parser creation crashes outside Xcode/Metal context
	// The agxps_aps_descriptor_create function requires proper initialization
	// which depends on Metal device context or Xcode-specific runtime.
	//
	// Known limitations (from CE95 testing):
	// - agxps_initialize returns error 1 outside Xcode
	// - agxps_aps_descriptor_create crashes (SIGSEGV at 0x28) without init
	// - Period queries return 0 without proper context
	//
	// Recommendation: Use ObjC GTShaderProfilerStreamDataProcessor to parse
	// streamData, then use C API query functions on the resulting profile data.
	t.Log("Skipping parser creation - requires Metal/Xcode context")
	t.Log("Use GTShaderProfilerStreamDataProcessor (ObjC) for parsing streamData")
}
