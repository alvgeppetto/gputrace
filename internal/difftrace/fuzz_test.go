package difftrace

import "testing"

func FuzzBuildReportRandom(f *testing.F) {
	f.Add([]byte{1, 2, 3, 4, 5, 6, 7, 8})
	f.Add([]byte{0, 0, 0, 0, 0, 0, 0, 0})
	f.Add([]byte{255, 1, 33, 64, 18, 7, 200, 17, 80})

	f.Fuzz(func(t *testing.T, data []byte) {
		mkTrace := func(seed byte) *TraceData {
			ds := make([]Dispatch, 0, len(data)/6+1)
			for i := 0; i+5 < len(data); i += 6 {
				name := ""
				if data[i]&1 == 1 {
					name = string([]byte{'k', 'n', 'l', byte('a' + (data[i+1] % 26))})
				}
				ds = append(ds, Dispatch{
					SourceIndex:  len(ds),
					FunctionName: name,
					FunctionKey:  functionKey(name, int(data[i+2]%11)),
					PipelineID:   int(data[i+2] % 11),
					EncoderIndex: int(data[i+3] % 6),
					DurationUs:   int(data[i+4]) - int(data[i+5]&0x7f),
				})
			}
			for i := range ds {
				if ds[i].DurationUs < 0 {
					ds[i].DurationUs = 0
				}
			}
			return &TraceData{Label: string([]byte{'t', seed}), Dispatches: ds}
		}

		a := mkTrace('a')
		b := mkTrace('b')
		aligned := AlignDispatches(a, b, AlignOptions{})
		report := BuildReport(a, b, aligned, ReportOptions{Limit: 50, MinDeltaUs: 0})

		if report.SchemaVersion == "" {
			t.Fatalf("empty schema version")
		}
		if report.Summary.DispatchCountA < 0 || report.Summary.DispatchCountB < 0 {
			t.Fatalf("negative dispatch counts")
		}
	})
}
