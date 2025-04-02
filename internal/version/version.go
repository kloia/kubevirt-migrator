package version

import (
	"fmt"
	"runtime"
)

var (
	// Version is set during build via ldflags
	Version = "dev"
	// Commit is set during build via ldflags
	Commit = "none"
	// Date is set during build via ldflags
	Date = "unknown"
)

// BuildInfo represents the build information
type BuildInfo struct {
	Version   string `json:"version"`
	Commit    string `json:"commit"`
	Date      string `json:"date"`
	GoVersion string `json:"goVersion"`
}

// Get returns the build information
func Get() BuildInfo {
	return BuildInfo{
		Version:   Version,
		Commit:    Commit,
		Date:      Date,
		GoVersion: runtime.Version(),
	}
}

// String returns a formatted string representation of build info
func (b BuildInfo) String() string {
	return fmt.Sprintf(
		"Version: %s\nCommit: %s\nBuild Date: %s\nGo Version: %s",
		b.Version, b.Commit, b.Date, b.GoVersion,
	)
}
