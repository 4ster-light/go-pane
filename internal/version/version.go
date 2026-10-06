// Package version holds build metadata, injected at build time via -ldflags.
package version

// Build metadata, overridden at build time via -ldflags.
var (
	// Version is the semantic version reported by `go-pane version`.
	Version = "0.1.0"
	// Commit is the short git revision the binary was built from.
	Commit = "none"
	// Date is the UTC build timestamp.
	Date = "unknown"
)
