// Package version holds build metadata, injected at build time via -ldflags.
package version

var (
	Version = "0.1.0"
	Commit  = "none"
	Date    = "unknown"
)
