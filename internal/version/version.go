// Package version holds build-time variables injected via ldflags.
package version

var (
	Version   = "dev"
	GitCommit = "unknown"
	BuildDate = "unknown"
)
