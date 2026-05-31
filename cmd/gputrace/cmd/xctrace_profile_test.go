//go:build darwin

package cmd

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestParseXctraceTOCExtractsRunsAndSchemas(t *testing.T) {
	xml := `<trace-toc>
  <run number="2"><data><table schema="metal-gpu-intervals"/><table schema="cpu-samples"/></data></run>
  <run number="1"><data><table schema="gpu-counter-value"/><table schema="metal-gpu-intervals"/></data></run>
</trace-toc>`
	path := filepath.Join(t.TempDir(), "toc.xml")
	if err := os.WriteFile(path, []byte(xml), 0o644); err != nil {
		t.Fatal(err)
	}
	runs, schemas := parseXctraceTOC(path)
	if !reflect.DeepEqual(runs, []int{1, 2}) {
		t.Fatalf("runs = %#v, want [1 2]", runs)
	}
	wantSchemas := []string{"cpu-samples", "gpu-counter-value", "metal-gpu-intervals"}
	if !reflect.DeepEqual(schemas, wantSchemas) {
		t.Fatalf("schemas = %#v, want %#v", schemas, wantSchemas)
	}
}

func TestXctraceProfileSchemasAddsDiscoveredMetalAndGPUSchemas(t *testing.T) {
	got := xctraceProfileSchemas(xctraceProfileTOCSummary{
		Schemas: []string{
			"cpu-samples",
			"metal-application-encoders-list",
			"gpu-extra-table",
			"metal-gpu-intervals",
		},
	})
	for _, want := range []string{
		"metal-gpu-intervals",
		"metal-application-encoders-list",
		"gpu-extra-table",
	} {
		if !containsString(got, want) {
			t.Fatalf("schemas %#v missing %q", got, want)
		}
	}
	if containsString(got, "cpu-samples") {
		t.Fatalf("schemas %#v unexpectedly included cpu-samples", got)
	}
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
