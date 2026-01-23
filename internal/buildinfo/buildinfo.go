package buildinfo

// Populated via -ldflags at build time; defaults are for local dev.
var (
	Version = "dev"
	Commit  = "none"
	Date    = "unknown"
)
