package sync

import (
	"testing"
)

func TestNewSyncCommand(t *testing.T) {
	tests := []struct {
		name       string
		toolName   SyncTool
		expectType interface{}
		expectErr  bool
	}{
		{
			name:       "rclone tool",
			toolName:   SyncToolRclone,
			expectType: &RcloneSync{},
			expectErr:  false,
		},
		{
			name:       "rsync tool",
			toolName:   SyncToolRsync,
			expectType: &RsyncSync{},
			expectErr:  false,
		},
		{
			name:       "unsupported tool",
			toolName:   SyncTool("unsupported"),
			expectType: nil,
			expectErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd, err := NewSyncCommand(tt.toolName)

			// Check error expectation
			if tt.expectErr {
				if err == nil {
					t.Errorf("expected error but got nil")
				}
			} else {
				if err != nil {
					t.Errorf("expected no error but got: %v", err)
				}

				// Check type
				if cmd == nil {
					t.Errorf("expected sync command but got nil")
				} else {
					switch tt.expectType.(type) {
					case *RcloneSync:
						if _, ok := cmd.(*RcloneSync); !ok {
							t.Errorf("expected RcloneSync but got %T", cmd)
						}
					case *RsyncSync:
						if _, ok := cmd.(*RsyncSync); !ok {
							t.Errorf("expected RsyncSync but got %T", cmd)
						}
					}
				}
			}
		})
	}
}

func TestRcloneSync_GenerateSyncCommand(t *testing.T) {
	rclone := &RcloneSync{}

	tests := []struct {
		name          string
		source        string
		dest          string
		options       map[string]string
		expectedCmd   string
		expectedArgs  []string
		expectedCount int
	}{
		{
			name:         "basic command",
			source:       "/source/path/",
			dest:         "/dest/path/",
			options:      map[string]string{},
			expectedCmd:  "rclone",
			expectedArgs: []string{"sync", "--progress", "/source/path/", "/dest/path/", "--skip-links", "--checkers", "8", "--contimeout", "100s", "--timeout", "300s", "--retries", "3", "--low-level-retries", "10", "--drive-acknowledge-abuse", "--stats", "1s", "--cutoff-mode=soft"},
		},
		{
			name:         "with checksum option",
			source:       "/source/path/",
			dest:         "/dest/path/",
			options:      map[string]string{"checksum": "true"},
			expectedCmd:  "rclone",
			expectedArgs: []string{"sync", "--progress", "/source/path/", "/dest/path/", "--checksum", "--skip-links", "--checkers", "8", "--contimeout", "100s", "--timeout", "300s", "--retries", "3", "--low-level-retries", "10", "--drive-acknowledge-abuse", "--stats", "1s", "--cutoff-mode=soft"},
		},
		{
			name:         "with custom checkers",
			source:       "/source/path/",
			dest:         "/dest/path/",
			options:      map[string]string{"checkers": "16"},
			expectedCmd:  "rclone",
			expectedArgs: []string{"sync", "--progress", "/source/path/", "/dest/path/", "--checkers", "16", "--skip-links", "--checkers", "8", "--contimeout", "100s", "--timeout", "300s", "--retries", "3", "--low-level-retries", "10", "--drive-acknowledge-abuse", "--stats", "1s", "--cutoff-mode=soft"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd, args := rclone.GenerateSyncCommand(tt.source, tt.dest, tt.options)

			if cmd != tt.expectedCmd {
				t.Errorf("expected command %q but got %q", tt.expectedCmd, cmd)
			}

			// Check that all expected args are included
			for _, expectedArg := range tt.expectedArgs {
				found := false
				for _, arg := range args {
					if arg == expectedArg {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("expected argument %q not found in args: %v", expectedArg, args)
				}
			}
		})
	}
}

func TestRsyncSync_GenerateSyncCommand(t *testing.T) {
	rsync := &RsyncSync{}

	tests := []struct {
		name         string
		source       string
		dest         string
		options      map[string]string
		expectedCmd  string
		expectedArgs []string
	}{
		{
			name:         "basic command",
			source:       "/source/path",
			dest:         "/dest/path",
			options:      map[string]string{},
			expectedCmd:  "rsync",
			expectedArgs: []string{"-avzP", "/source/path/", "/dest/path/", "--timeout=300", "--contimeout=100"},
		},
		{
			name:         "with checksum option",
			source:       "/source/path",
			dest:         "/dest/path",
			options:      map[string]string{"checksum": "true"},
			expectedCmd:  "rsync",
			expectedArgs: []string{"-avzP", "/source/path/", "/dest/path/", "-c", "--timeout=300", "--contimeout=100"},
		},
		{
			name:         "with delete option",
			source:       "/source/path",
			dest:         "/dest/path",
			options:      map[string]string{"delete": "true"},
			expectedCmd:  "rsync",
			expectedArgs: []string{"-avzP", "/source/path/", "/dest/path/", "--delete", "--timeout=300", "--contimeout=100"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd, args := rsync.GenerateSyncCommand(tt.source, tt.dest, tt.options)

			if cmd != tt.expectedCmd {
				t.Errorf("expected command %q but got %q", tt.expectedCmd, cmd)
			}

			// Check that all expected args are included
			for _, expectedArg := range tt.expectedArgs {
				found := false
				for _, arg := range args {
					if arg == expectedArg {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("expected argument %q not found in args: %v", expectedArg, args)
				}
			}
		})
	}
}

func TestMockSyncCommand(t *testing.T) {
	// Create a mock sync command
	mockSync := NewMockSyncCommand("mock-tool")
	mockSync.SetCommandToGenerate("mock-cmd", []string{"--flag1", "--flag2"})

	// Test GenerateSyncCommand
	sourcePath := "/source/path"
	destPath := "/dest/path"
	options := map[string]string{"option1": "value1", "option2": "value2"}

	cmd, args := mockSync.GenerateSyncCommand(sourcePath, destPath, options)

	// Check command and args
	if cmd != "mock-cmd" {
		t.Errorf("expected command to be 'mock-cmd', got %q", cmd)
	}

	if len(args) != 2 || args[0] != "--flag1" || args[1] != "--flag2" {
		t.Errorf("unexpected args: %v", args)
	}

	// Check that the mock recorded the calls correctly
	if mockSync.GenerateCallCount != 1 {
		t.Errorf("expected GenerateSyncCommand to be called once, got %d", mockSync.GenerateCallCount)
	}

	if mockSync.CalledWithSource != sourcePath {
		t.Errorf("expected source path to be %q, got %q", sourcePath, mockSync.CalledWithSource)
	}

	if mockSync.CalledWithDest != destPath {
		t.Errorf("expected dest path to be %q, got %q", destPath, mockSync.CalledWithDest)
	}

	// Check options
	for k, v := range options {
		if mockSync.CalledWithOptions[k] != v {
			t.Errorf("expected option %q to be %q, got %q", k, v, mockSync.CalledWithOptions[k])
		}
	}

	// Test GetToolName
	toolName := mockSync.GetToolName()
	if toolName != "mock-tool" {
		t.Errorf("expected tool name to be 'mock-tool', got %q", toolName)
	}

	if mockSync.GetToolNameCount != 1 {
		t.Errorf("expected GetToolName to be called once, got %d", mockSync.GetToolNameCount)
	}
}
