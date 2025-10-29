// gputrace-reader is a tool to parse and analyze .gputrace files
package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/tmc/mlx-go/pkg/gputrace"
)

func main() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s <path-to.gputrace>\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "\nAnalyzes a .gputrace bundle and extracts kernel names, labels, and metadata.\n\n")
		flag.PrintDefaults()
	}

	verbose := flag.Bool("v", false, "verbose output")
	flag.Parse()

	if flag.NArg() != 1 {
		flag.Usage()
		os.Exit(1)
	}

	tracePath := flag.Arg(0)

	// Open and parse the trace
	trace, err := gputrace.Open(tracePath)
	if err != nil {
		log.Fatalf("Failed to open trace: %v", err)
	}

	// Print metadata
	fmt.Println("=== Trace Metadata ===")
	fmt.Printf("UUID: %s\n", trace.Metadata.UUID)
	fmt.Printf("Graphics API: %d (1=Metal)\n", trace.Metadata.GraphicsAPI)
	fmt.Printf("Device ID: %d\n", trace.Metadata.DeviceID)
	fmt.Printf("Pointer Size: %d bytes\n", trace.Metadata.NativePointerSize)
	fmt.Printf("Captured Frames: %d\n", trace.Metadata.CapturedFramesCount)
	fmt.Printf("Boundary Less: %v\n", trace.Metadata.BoundaryLess)

	if len(trace.Metadata.LibraryLinkVersions) > 0 && *verbose {
		fmt.Println("\nLibrary Versions:")
		for lib, version := range trace.Metadata.LibraryLinkVersions {
			if version != -1 {
				fmt.Printf("  %s: %d\n", lib, version)
			}
		}
	}

	// Print capture info
	fmt.Println("\n=== Capture Data ===")
	fmt.Printf("Capture file size: %d bytes\n", len(trace.CaptureData))

	header, err := gputrace.ReadMTSPHeader(trace.CaptureData)
	if err == nil && *verbose {
		fmt.Printf("MTSP Version: 0x%08x\n", header.Version)
		fmt.Printf("MTSP Size: 0x%08x\n", header.Size)
	}

	// Print device resources
	if *verbose {
		fmt.Println("\n=== Device Resources ===")
		fmt.Printf("Device resource files: %d\n", len(trace.DeviceResources))
		for addr, data := range trace.DeviceResources {
			fmt.Printf("  %s: %d bytes\n", addr, len(data))
		}
	}

	// Print extracted labels - THIS IS WHAT MATCHES XCODE VIEW
	fmt.Println("\n=== Kernel Functions (from device-resources) ===")
	if len(trace.KernelNames) == 0 {
		fmt.Println("  (none found)")
	} else {
		for _, name := range trace.KernelNames {
			fmt.Printf("  • %s\n", name)
		}
	}

	fmt.Println("\n=== Encoder Labels (from capture - matches Xcode timeline) ===")
	if len(trace.EncoderLabels) == 0 {
		fmt.Println("  (none found)")
	} else {
		for _, label := range trace.EncoderLabels {
			fmt.Printf("  • %s\n", label)
		}
	}

	fmt.Println("\n=== Buffer Labels ===")
	if len(trace.BufferLabels) == 0 {
		fmt.Println("  (none found)")
	} else {
		for _, label := range trace.BufferLabels {
			fmt.Printf("  • %s\n", label)
		}
	}

	if trace.CommandQueueLabel != "" {
		fmt.Printf("\n=== Command Queue ===\n")
		fmt.Printf("  Label: %s\n", trace.CommandQueueLabel)
	}

	// Verbose: try to decompress store
	if *verbose {
		fmt.Println("\n=== Store Data ===")
		decompressed, err := trace.DecompressStore(0)
		if err != nil {
			fmt.Printf("  Could not decompress store0: %v\n", err)
		} else {
			nonZero := 0
			for _, b := range decompressed {
				if b != 0 {
					nonZero++
				}
			}
			fmt.Printf("  Decompressed size: %d bytes\n", len(decompressed))
			fmt.Printf("  Non-zero bytes: %d (%.2f%%)\n", nonZero, 100.0*float64(nonZero)/float64(len(decompressed)))
		}
	}

	fmt.Println("\n=== Summary ===")
	fmt.Printf("Total kernels discovered: %d\n", len(trace.KernelNames))
	fmt.Printf("Total encoder labels: %d\n", len(trace.EncoderLabels))
	fmt.Printf("Total buffer labels: %d\n", len(trace.BufferLabels))

	fmt.Println("\n✓ These labels should match what you see in Xcode Instruments!")
}
