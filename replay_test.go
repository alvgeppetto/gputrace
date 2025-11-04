package gputrace

import (
	"testing"
)

func TestReplayEngineBasic(t *testing.T) {
	// This test uses a real trace file if available
	tracePath := "/tmp/fast-llm-mlx-test.gputrace"

	trace, err := Open(tracePath)
	if err != nil {
		t.Skipf("Skipping test: trace file not available: %v", err)
	}

	engine := NewReplayEngine(trace)
	if engine == nil {
		t.Fatal("NewReplayEngine returned nil")
	}

	// Test replay analysis
	plan, err := engine.AnalyzeReplay()
	if err != nil {
		t.Fatalf("AnalyzeReplay failed: %v", err)
	}

	if plan == nil {
		t.Fatal("AnalyzeReplay returned nil plan")
	}

	t.Logf("Replay plan: %d commands, %d encoders", plan.TotalCommands, plan.TotalEncoders)

	// Test validation
	validation, err := engine.ValidateReplay()
	if err != nil {
		t.Fatalf("ValidateReplay failed: %v", err)
	}

	if validation == nil {
		t.Fatal("ValidateReplay returned nil")
	}

	t.Logf("Validation: CanReplay=%v, Errors=%d, Warnings=%d",
		validation.CanReplay, len(validation.Errors), len(validation.Warnings))
}

func TestReplayStateAnalysis(t *testing.T) {
	tracePath := "/tmp/fast-llm-mlx-test.gputrace"

	trace, err := Open(tracePath)
	if err != nil {
		t.Skipf("Skipping test: trace file not available: %v", err)
	}

	state := NewReplayState(trace)
	if state == nil {
		t.Fatal("NewReplayState returned nil")
	}

	// Test state restoration analysis
	analysis, err := state.RestoreState()
	if err != nil {
		t.Fatalf("RestoreState failed: %v", err)
	}

	if analysis == nil {
		t.Fatal("RestoreState returned nil")
	}

	t.Logf("State analysis: %d buffers, %d functions, %d pipelines",
		analysis.BufferCount, analysis.FunctionCount, analysis.PipelineCount)

	if analysis.BufferCount > 0 {
		t.Logf("First buffer: %s (%d bytes)", analysis.Buffers[0].Name, analysis.Buffers[0].Size)
	}
}
