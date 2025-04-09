package replication

import (
	"errors"
	"strings"
	"testing"

	"go.uber.org/zap"

	"github.com/kloia/kubevirt-migrator/internal/config"
	"github.com/kloia/kubevirt-migrator/internal/executor"
)

func TestSSHFSProvider_CheckConnectivity(t *testing.T) {
	// Create test cases
	testCases := []struct {
		name          string
		kubeCLI       string
		setupMock     func(*executor.MockCommandExecutor, string)
		expectError   bool
		errorContains string
	}{
		{
			name:    "successful connectivity check",
			kubeCLI: "kubectl",
			setupMock: func(mock *executor.MockCommandExecutor, kubeCLI string) {
				// Mock TCP connection check
				mock.AddCommandResult(
					kubeCLI+" exec test-vm-src-replicator -n test-namespace --kubeconfig src-kubeconfig.yaml -- bash -c timeout 5 bash -c '</dev/tcp/192.168.1.100/30123' && echo 'Connection successful'",
					"Connection successful",
					nil,
				)
				// Mock source directory permission check
				mock.AddCommandResult(
					kubeCLI+" exec test-vm-src-replicator -n test-namespace --kubeconfig src-kubeconfig.yaml -- bash -c mkdir -p /data/simg && touch /data/simg/test_write_perm && rm /data/simg/test_write_perm",
					"",
					nil,
				)
				// Mock SSHFS command availability check
				mock.AddCommandResult(
					kubeCLI+" exec test-vm-src-replicator -n test-namespace --kubeconfig src-kubeconfig.yaml -- bash -c which sshfs || echo 'sshfs not found'",
					"/usr/bin/sshfs",
					nil,
				)
			},
			expectError:   false,
			errorContains: "",
		},
		{
			name:    "tcp connection failure",
			kubeCLI: "kubectl",
			setupMock: func(mock *executor.MockCommandExecutor, kubeCLI string) {
				// Mock TCP connection check failure
				mock.AddCommandResult(
					kubeCLI+" exec test-vm-src-replicator -n test-namespace --kubeconfig src-kubeconfig.yaml -- bash -c timeout 5 bash -c '</dev/tcp/192.168.1.100/30123' && echo 'Connection successful'",
					"",
					errors.New("connection timeout"),
				)
			},
			expectError:   true,
			errorContains: "connectivity test failed",
		},
		{
			name:    "source directory permission failure",
			kubeCLI: "kubectl",
			setupMock: func(mock *executor.MockCommandExecutor, kubeCLI string) {
				// Mock TCP connection check success
				mock.AddCommandResult(
					kubeCLI+" exec test-vm-src-replicator -n test-namespace --kubeconfig src-kubeconfig.yaml -- bash -c timeout 5 bash -c '</dev/tcp/192.168.1.100/30123' && echo 'Connection successful'",
					"Connection successful",
					nil,
				)
				// Mock source directory permission check failure
				mock.AddCommandResult(
					kubeCLI+" exec test-vm-src-replicator -n test-namespace --kubeconfig src-kubeconfig.yaml -- bash -c mkdir -p /data/simg && touch /data/simg/test_write_perm && rm /data/simg/test_write_perm",
					"",
					errors.New("permission denied"),
				)
			},
			expectError:   true,
			errorContains: "source directory write permission check failed",
		},
		{
			name:    "sshfs command not available",
			kubeCLI: "kubectl",
			setupMock: func(mock *executor.MockCommandExecutor, kubeCLI string) {
				// Mock TCP connection check success
				mock.AddCommandResult(
					kubeCLI+" exec test-vm-src-replicator -n test-namespace --kubeconfig src-kubeconfig.yaml -- bash -c timeout 5 bash -c '</dev/tcp/192.168.1.100/30123' && echo 'Connection successful'",
					"Connection successful",
					nil,
				)
				// Mock source directory permission check success
				mock.AddCommandResult(
					kubeCLI+" exec test-vm-src-replicator -n test-namespace --kubeconfig src-kubeconfig.yaml -- bash -c mkdir -p /data/simg && touch /data/simg/test_write_perm && rm /data/simg/test_write_perm",
					"",
					nil,
				)
				// Mock SSHFS command not available
				mock.AddCommandResult(
					kubeCLI+" exec test-vm-src-replicator -n test-namespace --kubeconfig src-kubeconfig.yaml -- bash -c which sshfs || echo 'sshfs not found'",
					"sshfs not found",
					nil,
				)
			},
			expectError:   true,
			errorContains: "SSHFS command not available in the replicator pod",
		},
	}

	// Run test cases
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			mockExecutor := executor.NewMockCommandExecutor()
			tc.setupMock(mockExecutor, tc.kubeCLI)

			logger, _ := zap.NewDevelopment()
			provider := NewSSHFSProvider(mockExecutor, logger)

			cfg := &config.Config{
				VMName:        "test-vm",
				Namespace:     "test-namespace",
				SrcKubeconfig: "src-kubeconfig.yaml",
				KubeCLI:       tc.kubeCLI,
			}

			// Execute
			err := provider.CheckConnectivity(cfg, "192.168.1.100", "30123")

			// Verify
			if tc.expectError {
				if err == nil {
					t.Errorf("expected error but got nil")
				} else if tc.errorContains != "" && !contains(err.Error(), tc.errorContains) {
					t.Errorf("error message %q does not contain %q", err.Error(), tc.errorContains)
				}
			} else if err != nil {
				t.Errorf("expected no error but got: %v", err)
			}
		})
	}
}

func TestSSHFSProvider_Mount(t *testing.T) {
	testCases := []struct {
		name          string
		kubeCLI       string
		setupMock     func(*executor.MockCommandExecutor, string)
		expectError   bool
		errorContains string
	}{
		{
			name:    "successful mount",
			kubeCLI: "kubectl",
			setupMock: func(mock *executor.MockCommandExecutor, kubeCLI string) {
				mock.AddCommandResult(
					kubeCLI+" exec test-vm-src-replicator -n test-namespace --kubeconfig src-kubeconfig.yaml -- bash -c mkdir -p /data/dimg && sshfs -o StrictHostKeyChecking=no -o port=30123 192.168.1.100:/data/simg /data/dimg",
					"",
					nil,
				)
			},
			expectError:   false,
			errorContains: "",
		},
		{
			name:    "mount failure",
			kubeCLI: "kubectl",
			setupMock: func(mock *executor.MockCommandExecutor, kubeCLI string) {
				mock.AddCommandResult(
					kubeCLI+" exec test-vm-src-replicator -n test-namespace --kubeconfig src-kubeconfig.yaml -- bash -c mkdir -p /data/dimg && sshfs -o StrictHostKeyChecking=no -o port=30123 192.168.1.100:/data/simg /data/dimg",
					"",
					errors.New("mount error: connection refused"),
				)
			},
			expectError:   true,
			errorContains: "failed to mount destination directory",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			mockExecutor := executor.NewMockCommandExecutor()
			tc.setupMock(mockExecutor, tc.kubeCLI)

			logger, _ := zap.NewDevelopment()
			provider := NewSSHFSProvider(mockExecutor, logger)

			cfg := &config.Config{
				VMName:        "test-vm",
				Namespace:     "test-namespace",
				SrcKubeconfig: "src-kubeconfig.yaml",
				KubeCLI:       tc.kubeCLI,
			}

			// Execute
			err := provider.Mount(cfg, "192.168.1.100", "30123")

			// Verify
			if tc.expectError {
				if err == nil {
					t.Errorf("expected error but got nil")
				} else if tc.errorContains != "" && !contains(err.Error(), tc.errorContains) {
					t.Errorf("error message %q does not contain %q", err.Error(), tc.errorContains)
				}
			} else if err != nil {
				t.Errorf("expected no error but got: %v", err)
			}
		})
	}
}

func TestSSHFSProvider_VerifyMount(t *testing.T) {
	testCases := []struct {
		name          string
		kubeCLI       string
		setupMock     func(*executor.MockCommandExecutor, string)
		expectError   bool
		errorContains string
	}{
		{
			name:    "successful verify",
			kubeCLI: "kubectl",
			setupMock: func(mock *executor.MockCommandExecutor, kubeCLI string) {
				mock.AddCommandResult(
					kubeCLI+" exec test-vm-src-replicator -n test-namespace --kubeconfig src-kubeconfig.yaml -- bash -c ls -la /data/dimg",
					"total 4\ndrwxr-xr-x 2 root root 4096 Jan 1 00:00 simg\n",
					nil,
				)
			},
			expectError:   false,
			errorContains: "",
		},
		{
			name:    "ls command fails",
			kubeCLI: "kubectl",
			setupMock: func(mock *executor.MockCommandExecutor, kubeCLI string) {
				mock.AddCommandResult(
					kubeCLI+" exec test-vm-src-replicator -n test-namespace --kubeconfig src-kubeconfig.yaml -- bash -c ls -la /data/dimg",
					"",
					errors.New("no such file or directory"),
				)
			},
			expectError:   true,
			errorContains: "mount verification failed",
		},
		{
			name:    "expected content not found",
			kubeCLI: "kubectl",
			setupMock: func(mock *executor.MockCommandExecutor, kubeCLI string) {
				mock.AddCommandResult(
					kubeCLI+" exec test-vm-src-replicator -n test-namespace --kubeconfig src-kubeconfig.yaml -- bash -c ls -la /data/dimg",
					"total 0\n",
					nil,
				)
			},
			expectError:   true,
			errorContains: "cannot access mounted content",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			mockExecutor := executor.NewMockCommandExecutor()
			tc.setupMock(mockExecutor, tc.kubeCLI)

			logger, _ := zap.NewDevelopment()
			provider := NewSSHFSProvider(mockExecutor, logger)

			cfg := &config.Config{
				VMName:        "test-vm",
				Namespace:     "test-namespace",
				SrcKubeconfig: "src-kubeconfig.yaml",
				KubeCLI:       tc.kubeCLI,
			}

			// Execute
			err := provider.VerifyMount(cfg)

			// Verify
			if tc.expectError {
				if err == nil {
					t.Errorf("expected error but got nil")
				} else if tc.errorContains != "" && !contains(err.Error(), tc.errorContains) {
					t.Errorf("error message %q does not contain %q", err.Error(), tc.errorContains)
				}
			} else if err != nil {
				t.Errorf("expected no error but got: %v", err)
			}
		})
	}
}

func TestSSHFSProvider_Unmount(t *testing.T) {
	testCases := []struct {
		name          string
		kubeCLI       string
		setupMock     func(*executor.MockCommandExecutor, string)
		expectError   bool
		errorContains string
	}{
		{
			name:    "successful unmount",
			kubeCLI: "kubectl",
			setupMock: func(mock *executor.MockCommandExecutor, kubeCLI string) {
				mock.AddCommandResult(
					kubeCLI+" exec test-vm-src-replicator -n test-namespace --kubeconfig src-kubeconfig.yaml -- bash -c umount /data/dimg",
					"",
					nil,
				)
			},
			expectError:   false,
			errorContains: "",
		},
		{
			name:    "unmount failure",
			kubeCLI: "kubectl",
			setupMock: func(mock *executor.MockCommandExecutor, kubeCLI string) {
				mock.AddCommandResult(
					kubeCLI+" exec test-vm-src-replicator -n test-namespace --kubeconfig src-kubeconfig.yaml -- bash -c umount /data/dimg",
					"",
					errors.New("device or resource busy"),
				)
			},
			expectError:   true,
			errorContains: "unmount operation failed",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			mockExecutor := executor.NewMockCommandExecutor()
			tc.setupMock(mockExecutor, tc.kubeCLI)

			logger, _ := zap.NewDevelopment()
			provider := NewSSHFSProvider(mockExecutor, logger)

			cfg := &config.Config{
				VMName:        "test-vm",
				Namespace:     "test-namespace",
				SrcKubeconfig: "src-kubeconfig.yaml",
				KubeCLI:       tc.kubeCLI,
			}

			// Execute
			err := provider.Unmount(cfg)

			// Verify
			if tc.expectError {
				if err == nil {
					t.Errorf("expected error but got nil")
				} else if tc.errorContains != "" && !contains(err.Error(), tc.errorContains) {
					t.Errorf("error message %q does not contain %q", err.Error(), tc.errorContains)
				}
			} else if err != nil {
				t.Errorf("expected no error but got: %v", err)
			}
		})
	}
}

// Helper function to check if a string contains another string
func contains(s, substr string) bool {
	return strings.Contains(s, substr)
}
