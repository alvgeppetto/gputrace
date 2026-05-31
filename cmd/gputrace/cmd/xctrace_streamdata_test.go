//go:build darwin

package cmd

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseXctraceGPUIntervalsXMLResolvesRefsAndFiltersProcess(t *testing.T) {
	xml := `<?xml version="1.0"?>
<trace-query-result>
<node><schema name="metal-gpu-intervals"></schema>
<row><start-time id="1" fmt="00:00.001">1000</start-time><duration id="2" fmt="4 µs">4000</duration><gpu-channel-name id="3" fmt="Compute">Compute</gpu-channel-name><gpu-frame-number id="4" fmt="Frame 1">1</gpu-frame-number><duration id="5" fmt="0">0</duration><metal-nesting-level id="6" fmt="0">0</metal-nesting-level><formatted-label id="7" fmt="kernel ( target_proc (42) ) 0xabc"><process id="8" fmt="target_proc (42)"><pid id="9" fmt="42">42</pid></process><metal-encoder-id id="10" fmt="0xabc">2748</metal-encoder-id></formatted-label><gpu-state id="11" fmt="Active">Active</gpu-state><connection-uuid64 id="12" fmt="1">1</connection-uuid64><render-buffer-depth id="13" fmt="1">1</render-buffer-depth><process ref="8"/><metal-device-name id="14" fmt="M3">M3</metal-device-name><metal-object-label id="15" fmt=""></metal-object-label><formatted-label id="16" fmt="kernel">kernel</formatted-label><size-in-bytes id="17" fmt="0 Bytes">0</size-in-bytes><metal-command-buffer-id id="18" fmt="0x123">291</metal-command-buffer-id><metal-command-buffer-id id="19" fmt="0xabc">2748</metal-command-buffer-id><uint64 id="20" fmt="99">99</uint64></row>
<row><start-time id="21" fmt="00:00.002">2000</start-time><duration id="22" fmt="5 µs">5000</duration><gpu-channel-name ref="3"/><gpu-frame-number ref="4"/><duration ref="5"/><metal-nesting-level ref="6"/><formatted-label id="23" fmt="kernel ( other_proc (77) ) 0xdef"><process id="24" fmt="other_proc (77)"><pid id="25" fmt="77">77</pid></process><metal-encoder-id id="26" fmt="0xdef">3567</metal-encoder-id></formatted-label><gpu-state ref="11"/><connection-uuid64 ref="12"/><render-buffer-depth ref="13"/><process ref="24"/><metal-device-name ref="14"/><metal-object-label ref="15"/><formatted-label ref="16"/><size-in-bytes ref="17"/><metal-command-buffer-id ref="18"/><metal-command-buffer-id id="27" fmt="0xdef">3567</metal-command-buffer-id><uint64 id="28" fmt="100">100</uint64></row>
</node></trace-query-result>`
	path := filepath.Join(t.TempDir(), "intervals.xml")
	if err := os.WriteFile(path, []byte(xml), 0o644); err != nil {
		t.Fatal(err)
	}
	rows, rowsRead, err := parseXctraceGPUIntervalsXML(path, "target_proc", 10)
	if err != nil {
		t.Fatal(err)
	}
	if rowsRead != 2 {
		t.Fatalf("rowsRead = %d, want 2", rowsRead)
	}
	if len(rows) != 1 {
		t.Fatalf("len(rows) = %d, want 1", len(rows))
	}
	if rows[0].StartNs != 1000 || rows[0].DurationNs != 4000 {
		t.Fatalf("unexpected timing row: %+v", rows[0])
	}
	if rows[0].Process != "target_proc (42)" {
		t.Fatalf("process = %q", rows[0].Process)
	}
	if rows[0].CommandBufferID != 291 || rows[0].EncoderID != 2748 {
		t.Fatalf("unexpected ids: %+v", rows[0])
	}
}
