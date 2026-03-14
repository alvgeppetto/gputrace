// Package buildinfo exposes build metadata for release binaries.
package buildinfo

import "runtime/debug"

var (
	// Version is set at link time for release builds.
	Version = "dev"
	// Commit is set at link time for release builds.
	Commit = "unknown"
	// Date is set at link time for release builds.
	Date = "unknown"
)

// EffectiveVersion returns the best available version string.
func EffectiveVersion() string {
	if Version != "" && Version != "dev" {
		return Version
	}
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" && info.Main.Version != "(devel)" {
		return info.Main.Version
	}
	return Version
}
