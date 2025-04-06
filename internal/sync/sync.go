package sync

import (
	"fmt"
)

// SyncTool defines the type of synchronization tool
type SyncTool string

const (
	// SyncToolRclone represents the rclone sync tool
	SyncToolRclone SyncTool = "rclone"
	// SyncToolRsync represents the rsync sync tool
	SyncToolRsync SyncTool = "rsync"
)

// SyncCommand generates the appropriate sync command based on the selected tool
type SyncCommand interface {
	// GenerateSyncCommand creates a sync command based on the tool and paths
	GenerateSyncCommand(sourcePath, destPath string, options map[string]string) (string, []string)

	// GetToolName returns the tool name
	GetToolName() string
}

// RcloneSync implements SyncCommand for rclone
type RcloneSync struct{}

// GenerateSyncCommand creates a rclone sync command with the provided options
func (r *RcloneSync) GenerateSyncCommand(sourcePath, destPath string, options map[string]string) (string, []string) {
	var args []string

	// Start with base command
	args = append(args, "sync", "--progress")

	// Add source and dest paths only if they're provided
	if sourcePath != "" && destPath != "" {
		args = append(args, sourcePath, destPath)
	}

	// Add configurable options
	if v, ok := options["checksum"]; ok && v == "true" {
		args = append(args, "--checksum")
	}

	if v, ok := options["checkers"]; ok {
		args = append(args, "--checkers", v)
	}

	// Default options from the existing command
	args = append(args, "--skip-links", "--checkers", "8", "--contimeout", "100s",
		"--timeout", "300s", "--retries", "3", "--low-level-retries", "10",
		"--drive-acknowledge-abuse", "--stats", "1s", "--cutoff-mode=soft")

	return "rclone", args
}

// GetToolName returns the name of the sync tool
func (r *RcloneSync) GetToolName() string {
	return string(SyncToolRclone)
}

// RsyncSync implements SyncCommand for rsync
type RsyncSync struct{}

// GenerateSyncCommand creates a rsync sync command with the provided options
func (r *RsyncSync) GenerateSyncCommand(sourcePath, destPath string, options map[string]string) (string, []string) {
	var args []string

	// Start with base options
	args = append(args, "-avzP")

	// Add source and dest paths only if they're provided
	if sourcePath != "" && destPath != "" {
		args = append(args, sourcePath+"/", destPath+"/")
	}

	// Add configurable options
	if v, ok := options["checksum"]; ok && v == "true" {
		args = append(args, "-c")
	}

	if _, ok := options["delete"]; ok {
		args = append(args, "--delete")
	}

	// Additional common options for reliability
	args = append(args, "--timeout=300", "--contimeout=100")

	return "rsync", args
}

// GetToolName returns the name of the sync tool
func (r *RsyncSync) GetToolName() string {
	return string(SyncToolRsync)
}

// NewSyncCommand creates appropriate SyncCommand implementations
func NewSyncCommand(tool SyncTool) (SyncCommand, error) {
	switch tool {
	case SyncToolRclone:
		return &RcloneSync{}, nil
	case SyncToolRsync:
		return &RsyncSync{}, nil
	default:
		return nil, fmt.Errorf("unsupported sync tool: %s", tool)
	}
}
