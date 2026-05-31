//go:build darwin

package cmd

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

var (
	xctraceStreamDataJSON        bool
	xctraceStreamDataInput       string
	xctraceStreamDataOutDir      string
	xctraceStreamDataProcessName string
	xctraceStreamDataMaxRows     int
	xctraceStreamDataTimeout     time.Duration
	xctraceStreamDataClang       string
	xctraceStreamDataMinFreeGiB  float64
	xctraceStreamDataMinMemFree  int
)

type xctraceIntervalRow struct {
	StartNs         uint64 `json:"start_ns"`
	DurationNs      uint64 `json:"duration_ns"`
	Process         string `json:"process"`
	Label           string `json:"label,omitempty"`
	CommandBufferID uint64 `json:"command_buffer_id,omitempty"`
	EncoderID       uint64 `json:"encoder_id,omitempty"`
}

type xctraceStreamDataOutput struct {
	InputXML          string                           `json:"input_xml"`
	OutputDir         string                           `json:"output_dir"`
	ResourcePreflight privateReplayerResourcePreflight `json:"resource_preflight"`
	RowsRead          int                              `json:"rows_read"`
	RowsEncoded       int                              `json:"rows_encoded"`
	ProcessName       string                           `json:"process_name,omitempty"`
	Helper            privateReplayerResult            `json:"helper"`
	StreamData        filePresence                     `json:"streamData"`
	StreamDataStats   *streamDataProbeStats            `json:"streamData_stats,omitempty"`
	TimingUsable      bool                             `json:"timing_usable"`
	CounterUsable     bool                             `json:"counter_usable"`
}

var xctraceStreamDataCmd = &cobra.Command{
	Use:   "xctrace-streamdata --input metal-gpu-intervals.xml --process name --out-dir out.gpuprofiler_raw",
	Short: "Encode real xctrace Metal GPU intervals into streamData",
	Long: `Encode target-attributed xctrace Metal GPU interval rows into a
.gpuprofiler_raw/streamData archive using Apple's GTMutableShaderProfilerStreamData.

This command does not fabricate timings: it requires non-empty exported
metal-gpu-intervals rows for the requested process. It does not make hardware
counter claims.`,
	Args: cobra.NoArgs,
	RunE: runXctraceStreamData,
}

func init() {
	rootCmd.AddCommand(xctraceStreamDataCmd)
	xctraceStreamDataCmd.Flags().BoolVar(&xctraceStreamDataJSON, "json", false, "Output in JSON format")
	xctraceStreamDataCmd.Flags().StringVar(&xctraceStreamDataInput, "input", "", "Exported xctrace metal-gpu-intervals XML")
	xctraceStreamDataCmd.Flags().StringVar(&xctraceStreamDataOutDir, "out-dir", "", "Output .gpuprofiler_raw directory")
	xctraceStreamDataCmd.Flags().StringVar(&xctraceStreamDataProcessName, "process", "", "Process name substring required for rows")
	xctraceStreamDataCmd.Flags().IntVar(&xctraceStreamDataMaxRows, "max-rows", 20000, "Maximum interval rows to encode")
	xctraceStreamDataCmd.Flags().DurationVar(&xctraceStreamDataTimeout, "timeout", 20*time.Second, "Helper compile/run timeout")
	xctraceStreamDataCmd.Flags().StringVar(&xctraceStreamDataClang, "clang", "clang", "C compiler used for the streamData helper")
	xctraceStreamDataCmd.Flags().Float64Var(&xctraceStreamDataMinFreeGiB, "min-out-dir-free-gib", 24, "Minimum free GiB required on the output volume")
	xctraceStreamDataCmd.Flags().IntVar(&xctraceStreamDataMinMemFree, "min-memory-free-percent", 10, "Minimum memory_pressure free percentage required")
}

func runXctraceStreamData(cmd *cobra.Command, args []string) error {
	if xctraceStreamDataInput == "" || xctraceStreamDataOutDir == "" {
		return fmt.Errorf("--input and --out-dir are required")
	}
	if xctraceStreamDataProcessName == "" {
		return fmt.Errorf("--process is required to avoid encoding system-wide GPU rows; use '*' only for diagnostics")
	}
	if err := preflightXctraceStreamDataResources(xctraceStreamDataOutDir); err != nil {
		return err
	}
	rows, rowsRead, err := parseXctraceGPUIntervalsXML(xctraceStreamDataInput, xctraceStreamDataProcessName, xctraceStreamDataMaxRows)
	if err != nil {
		return err
	}
	if len(rows) == 0 {
		return fmt.Errorf("no metal-gpu-intervals rows matched process %q (rows read: %d)", xctraceStreamDataProcessName, rowsRead)
	}
	if err := os.RemoveAll(xctraceStreamDataOutDir); err != nil {
		return err
	}
	if err := os.MkdirAll(xctraceStreamDataOutDir, 0o755); err != nil {
		return err
	}
	helper := encodeXctraceRowsWithHelper(rows, xctraceStreamDataOutDir)
	streamStats := summarizeEncodedStreamData(xctraceStreamDataOutDir)
	out := xctraceStreamDataOutput{
		InputXML:  xctraceStreamDataInput,
		OutputDir: xctraceStreamDataOutDir,
		ResourcePreflight: collectResourcePreflight(
			xctraceStreamDataOutDir,
			xctraceStreamDataMinFreeGiB,
			xctraceStreamDataMinMemFree,
			false,
		),
		RowsRead:        rowsRead,
		RowsEncoded:     len(rows),
		ProcessName:     xctraceStreamDataProcessName,
		Helper:          helper,
		StreamData:      presence(filepath.Join(xctraceStreamDataOutDir, "streamData")),
		StreamDataStats: streamStats,
		CounterUsable:   false,
	}
	if streamStats != nil {
		out.TimingUsable = streamStats.TimingUsable
	}
	if xctraceStreamDataJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(out)
	}
	fmt.Printf("Rows encoded: %d\n", out.RowsEncoded)
	fmt.Printf("streamData:   %v (%d bytes)\n", out.StreamData.Present, out.StreamData.Bytes)
	fmt.Printf("Timing usable: %v\n", out.TimingUsable)
	if out.ResourcePreflight.CheckedPath != "" || out.ResourcePreflight.MemoryFreePercent != 0 {
		fmt.Printf("Preflight:    output=%s free=%.1fGiB memory_free=%d%%\n",
			out.ResourcePreflight.OutputDir,
			out.ResourcePreflight.FreeGiB,
			out.ResourcePreflight.MemoryFreePercent,
		)
	}
	if helper.Signal != "" || helper.ExitCode != 0 || helper.TimedOut {
		return fmt.Errorf("xctrace streamData helper failed")
	}
	if !out.TimingUsable {
		return fmt.Errorf("encoded streamData did not contain usable timing rows")
	}
	return nil
}

func preflightXctraceStreamDataResources(output string) error {
	if xctraceStreamDataMinFreeGiB > 0 {
		freeBytes, checkedPath, err := availableBytesForPath(output)
		if err != nil {
			return fmt.Errorf("resource preflight failed for output %s: %w", output, err)
		}
		freeGiB := float64(freeBytes) / (1024 * 1024 * 1024)
		if freeGiB < xctraceStreamDataMinFreeGiB {
			return fmt.Errorf("refusing to encode streamData: output volume at %s has %.1f GiB free, below %.1f GiB threshold", checkedPath, freeGiB, xctraceStreamDataMinFreeGiB)
		}
	}
	if xctraceStreamDataMinMemFree > 0 {
		freePercent, err := currentMemoryFreePercent()
		if err != nil {
			return fmt.Errorf("resource preflight failed reading memory pressure: %w", err)
		}
		if freePercent < xctraceStreamDataMinMemFree {
			return fmt.Errorf("refusing to encode streamData: memory_pressure free percentage is %d%%, below %d%% threshold", freePercent, xctraceStreamDataMinMemFree)
		}
	}
	return nil
}

func parseXctraceGPUIntervalsXML(path, processName string, maxRows int) ([]xctraceIntervalRow, int, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, 0, err
	}
	defer file.Close()
	decoder := xml.NewDecoder(file)
	values := map[string]string{}
	rows := []xctraceIntervalRow{}
	rowsRead := 0
	for {
		token, err := decoder.Token()
		if err != nil {
			if err.Error() == "EOF" {
				break
			}
			return nil, rowsRead, err
		}
		start, ok := token.(xml.StartElement)
		if !ok || start.Name.Local != "row" {
			continue
		}
		fields, err := parseXctraceRow(decoder, values)
		if err != nil {
			return nil, rowsRead, err
		}
		rowsRead++
		row, ok := xctraceIntervalFromFields(fields)
		if !ok {
			continue
		}
		if processName != "*" && !strings.Contains(row.Process, processName) {
			continue
		}
		rows = append(rows, row)
		if maxRows > 0 && len(rows) >= maxRows {
			break
		}
	}
	sort.Slice(rows, func(i, j int) bool {
		return rows[i].StartNs < rows[j].StartNs
	})
	return rows, rowsRead, nil
}

func parseXctraceRow(decoder *xml.Decoder, values map[string]string) ([]string, error) {
	fields := []string{}
	for {
		token, err := decoder.Token()
		if err != nil {
			return nil, err
		}
		switch t := token.(type) {
		case xml.StartElement:
			value, err := parseXctraceField(decoder, t, values)
			if err != nil {
				return nil, err
			}
			fields = append(fields, value)
		case xml.EndElement:
			if t.Name.Local == "row" {
				return fields, nil
			}
		}
	}
}

func parseXctraceField(decoder *xml.Decoder, start xml.StartElement, values map[string]string) (string, error) {
	if ref := xmlAttr(start, "ref"); ref != "" {
		if err := skipXMLToEnd(decoder, start.Name.Local); err != nil {
			return "", err
		}
		return values[ref], nil
	}
	value := xmlAttr(start, "fmt")
	var text strings.Builder
	depth := 1
	hadNested := false
	for depth > 0 {
		token, err := decoder.Token()
		if err != nil {
			return "", err
		}
		switch t := token.(type) {
		case xml.CharData:
			text.Write([]byte(t))
		case xml.StartElement:
			hadNested = true
			depth++
			if nestedFmt := xmlAttr(t, "fmt"); nestedFmt != "" {
				if nestedID := xmlAttr(t, "id"); nestedID != "" {
					values[nestedID] = nestedFmt
				}
				if value == "" {
					value = nestedFmt
				}
			}
		case xml.EndElement:
			depth--
		}
	}
	textValue := strings.TrimSpace(text.String())
	if textValue != "" && !hadNested {
		value = textValue
	} else if value == "" {
		value = textValue
	}
	if id := xmlAttr(start, "id"); id != "" {
		values[id] = value
	}
	return value, nil
}

func skipXMLToEnd(decoder *xml.Decoder, name string) error {
	depth := 1
	for depth > 0 {
		token, err := decoder.Token()
		if err != nil {
			return err
		}
		switch t := token.(type) {
		case xml.StartElement:
			depth++
		case xml.EndElement:
			if t.Name.Local == name {
				depth--
			} else {
				depth--
			}
		}
	}
	return nil
}

func xctraceIntervalFromFields(fields []string) (xctraceIntervalRow, bool) {
	if len(fields) < 18 {
		return xctraceIntervalRow{}, false
	}
	startNs, ok := parseUnsignedXctraceValue(fields[0])
	if !ok {
		return xctraceIntervalRow{}, false
	}
	durationNs, ok := parseUnsignedXctraceValue(fields[1])
	if !ok || durationNs == 0 {
		return xctraceIntervalRow{}, false
	}
	cbID, _ := parseUnsignedXctraceValue(fields[15])
	encoderID, _ := parseUnsignedXctraceValue(fields[16])
	return xctraceIntervalRow{
		StartNs:         startNs,
		DurationNs:      durationNs,
		Process:         fields[10],
		Label:           fields[6],
		CommandBufferID: cbID,
		EncoderID:       encoderID,
	}, true
}

func parseUnsignedXctraceValue(value string) (uint64, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, false
	}
	base := 10
	if strings.HasPrefix(value, "0x") {
		base = 16
		value = strings.TrimPrefix(value, "0x")
	}
	clean := strings.NewReplacer("'", "", ",", "", " ", "").Replace(value)
	n, err := strconv.ParseUint(clean, base, 64)
	return n, err == nil
}

func xmlAttr(start xml.StartElement, name string) string {
	for _, attr := range start.Attr {
		if attr.Name.Local == name {
			return attr.Value
		}
	}
	return ""
}

func encodeXctraceRowsWithHelper(rows []xctraceIntervalRow, outDir string) privateReplayerResult {
	helperDir := filepath.Join(filepath.Dir(outDir), "xctrace-streamdata-helper")
	_ = os.MkdirAll(helperDir, 0o755)
	source := filepath.Join(helperDir, "xctrace_streamdata_helper.m")
	binary := filepath.Join(helperDir, "xctrace_streamdata_helper")
	inputJSON := filepath.Join(helperDir, "rows.json")
	data, _ := json.Marshal(rows)
	if err := os.WriteFile(inputJSON, data, 0o644); err != nil {
		return privateReplayerResult{Name: "xctrace_streamdata_write_rows", Signal: err.Error()}
	}
	if err := os.WriteFile(source, []byte(xctraceStreamDataHelperSource), 0o644); err != nil {
		return privateReplayerResult{Name: "xctrace_streamdata_write_helper", Signal: err.Error()}
	}
	compile := runExternalCommand(
		"xctrace_streamdata_compile_helper",
		[]string{xctraceStreamDataClang, "-fobjc-arc", "-Wall", "-Wextra", "-O0", "-g", "-framework", "Foundation", "-o", binary, source},
		"",
		xctraceStreamDataTimeout,
	)
	if compile.Signal != "" || compile.ExitCode != 0 || compile.TimedOut {
		return compile
	}
	return runExternalCommand(
		"xctrace_streamdata_encode_helper",
		[]string{binary, inputJSON, outDir},
		outDir,
		xctraceStreamDataTimeout,
	)
}

const xctraceStreamDataHelperSource = `
#import <Foundation/Foundation.h>
#import <dlfcn.h>
#import <objc/message.h>
#import <objc/runtime.h>
#include <stdint.h>
#include <stdlib.h>
#include <sys/stat.h>

typedef struct {
    uint64_t sequenceID;
    uint64_t startTimestamp;
    uint64_t endOffsetMicros;
    uint32_t labelStringIndex;
    uint32_t commandBufferIndex;
    uint32_t flags;
    uint32_t reserved;
} GTEncoderInfoRow;

typedef struct {
    uint32_t functionIndex;
    uint32_t subCommandIndex;
    uint32_t reserved0;
    uint32_t pipelineIndex;
    uint64_t endOffsetMicros;
    uint32_t encoderIndex;
    int32_t reserved1;
} GTGPUCommandInfoRow;

typedef struct {
    uint64_t sequenceID;
    uint64_t startTimestamp;
    uint64_t endOffsetMicros;
    uint32_t flags;
    uint32_t encoderCount;
} GTCommandBufferInfoRow;

static NSString *stringFromObject(id object) {
    if (!object || object == [NSNull null]) {
        return @"";
    }
    return [NSString stringWithFormat:@"%@", object];
}

static uint64_t unsignedValue(id object) {
    if ([object respondsToSelector:@selector(unsignedLongLongValue)]) {
        return [object unsignedLongLongValue];
    }
    return 0;
}

int main(int argc, char **argv) {
    @autoreleasepool {
        if (argc != 3) {
            fprintf(stderr, "usage: %s rows.json out.gpuprofiler_raw\n", argv[0]);
            return 2;
        }
        NSString *jsonPath = [NSString stringWithUTF8String:argv[1]];
        NSString *outDir = [NSString stringWithUTF8String:argv[2]];
        NSData *jsonData = [NSData dataWithContentsOfFile:jsonPath];
        if (!jsonData) {
            fprintf(stderr, "failed to read rows json\n");
            return 3;
        }
        NSError *jsonError = nil;
        NSArray *rows = [NSJSONSerialization JSONObjectWithData:jsonData options:0 error:&jsonError];
        if (![rows isKindOfClass:[NSArray class]] || [rows count] == 0) {
            fprintf(stderr, "invalid rows json: %s\n", [[jsonError description] UTF8String]);
            return 4;
        }
        void *handle = dlopen("/System/Library/PrivateFrameworks/GPUToolsReplay.framework/GPUToolsReplay", RTLD_NOW);
        fprintf(stdout, "GPUToolsReplay dlopen=%p err=%s\n", handle, dlerror());
        Class streamDataClass = NSClassFromString(@"GTMutableShaderProfilerStreamData");
        if (!streamDataClass) {
            fprintf(stderr, "GTMutableShaderProfilerStreamData missing\n");
            return 5;
        }
        id streamData = nil;
        if ([streamDataClass instancesRespondToSelector:@selector(initWithNewFileFormatV2Support:)]) {
            streamData = ((id (*)(id, SEL, BOOL))objc_msgSend)([streamDataClass alloc], @selector(initWithNewFileFormatV2Support:), YES);
        } else {
            streamData = [streamDataClass new];
        }
        if ([streamData respondsToSelector:@selector(setTraceName:)]) {
            [streamData setValue:@"xctrace-metal-gpu-intervals" forKey:@"traceName"];
        }
        NSUInteger count = [rows count];
        GTEncoderInfoRow *encoders = calloc(count, sizeof(GTEncoderInfoRow));
        GTGPUCommandInfoRow *commands = calloc(count, sizeof(GTGPUCommandInfoRow));
        if (!encoders || !commands) {
            return 6;
        }
        uint64_t firstStart = unsignedValue(rows[0][@"start_ns"]);
        uint64_t cumulativeUs = 0;
        for (NSUInteger i = 0; i < count; i++) {
            NSDictionary *row = rows[i];
            uint64_t startNs = unsignedValue(row[@"start_ns"]);
            uint64_t durationNs = unsignedValue(row[@"duration_ns"]);
            uint64_t durationUs = durationNs / 1000;
            if (durationUs == 0 && durationNs > 0) {
                durationUs = 1;
            }
            cumulativeUs += durationUs;
            encoders[i].sequenceID = (uint64_t)i + 1;
            encoders[i].startTimestamp = startNs;
            encoders[i].endOffsetMicros = cumulativeUs;
            encoders[i].labelStringIndex = 0;
            encoders[i].commandBufferIndex = 0;
            commands[i].functionIndex = 0;
            commands[i].subCommandIndex = (uint32_t)i;
            commands[i].pipelineIndex = 0;
            commands[i].endOffsetMicros = cumulativeUs;
            commands[i].encoderIndex = (uint32_t)i;
        }
        GTCommandBufferInfoRow commandBuffer = {1, firstStart, cumulativeUs, 0, (uint32_t)count};
        if ([streamData respondsToSelector:@selector(addCommandBuffers:count:)]) {
            ((void (*)(id, SEL, GTCommandBufferInfoRow *, uint64_t))objc_msgSend)(streamData, @selector(addCommandBuffers:count:), &commandBuffer, 1);
        }
        if ([streamData respondsToSelector:@selector(addEncoders:count:)]) {
            ((void (*)(id, SEL, GTEncoderInfoRow *, uint64_t))objc_msgSend)(streamData, @selector(addEncoders:count:), encoders, (uint64_t)count);
        }
        if ([streamData respondsToSelector:@selector(addGPUCommands:count:)]) {
            ((void (*)(id, SEL, GTGPUCommandInfoRow *, uint64_t))objc_msgSend)(streamData, @selector(addGPUCommands:count:), commands, (uint64_t)count);
        }
        mkdir([outDir fileSystemRepresentation], 0777);
        NSError *encodeError = nil;
        id encoded = ((id (*)(id, SEL, id, NSError **))objc_msgSend)(streamData, @selector(encode:error:), [NSURL fileURLWithPath:outDir isDirectory:YES], &encodeError);
        fprintf(stdout, "encoded=%s err=%s rows=%lu cumulative_us=%llu streamData_exists=%d\n",
                [stringFromObject(encoded) UTF8String],
                [[encodeError description] UTF8String],
                (unsigned long)count,
                (unsigned long long)cumulativeUs,
                [[NSFileManager defaultManager] fileExistsAtPath:[outDir stringByAppendingPathComponent:@"streamData"]] ? 1 : 0);
        free(encoders);
        free(commands);
        return encodeError ? 7 : 0;
    }
}
`
