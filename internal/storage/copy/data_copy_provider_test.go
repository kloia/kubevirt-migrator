package copy

import (
	"errors"
	"strings"
	"testing"

	"go.uber.org/zap"

	"github.com/kloia/kubevirt-migrator/internal/config"
	"github.com/kloia/kubevirt-migrator/internal/executor"
)

func TestSimpleBlockCopyProvider_CopyData(t *testing.T) {
	testCases := []struct {
		name          string
		kubeCLI       string
		setupMock     func(*executor.MockCommandExecutor, string)
		expectError   bool
		errorContains string
	}{
		{
			name:    "successful copy",
			kubeCLI: "kubectl",
			setupMock: func(mock *executor.MockCommandExecutor, kubeCLI string) {
				mock.AddCommandResult(
					kubeCLI+" exec test-vm-src-replicator -n test-namespace --kubeconfig src-kubeconfig.yaml -- bash -c cp -p --sparse=always /data/simg/disk.img /data/dimg/ & progress -m",
					"Copying... 100%",
					nil,
				)
			},
			expectError:   false,
			errorContains: "",
		},
		{
			name:    "copy failure",
			kubeCLI: "kubectl",
			setupMock: func(mock *executor.MockCommandExecutor, kubeCLI string) {
				mock.AddCommandResult(
					kubeCLI+" exec test-vm-src-replicator -n test-namespace --kubeconfig src-kubeconfig.yaml -- bash -c cp -p --sparse=always /data/simg/disk.img /data/dimg/ & progress -m",
					"",
					errors.New("no space left on device"),
				)
			},
			expectError:   true,
			errorContains: "failed to perform initial data copy",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			mockExecutor := executor.NewMockCommandExecutor()
			tc.setupMock(mockExecutor, tc.kubeCLI)

			logger, _ := zap.NewDevelopment()
			provider := NewSimpleBlockCopyProvider(mockExecutor, logger)

			cfg := &config.Config{
				VMName:        "test-vm",
				Namespace:     "test-namespace",
				SrcKubeconfig: "src-kubeconfig.yaml",
				KubeCLI:       tc.kubeCLI,
			}

			// Execute
			err := provider.CopyData(cfg)

			// Verify
			if tc.expectError {
				if err == nil {
					t.Errorf("expected error but got nil")
				} else if tc.errorContains != "" && !strings.Contains(err.Error(), tc.errorContains) {
					t.Errorf("error message %q does not contain %q", err.Error(), tc.errorContains)
				}
			} else if err != nil {
				t.Errorf("expected no error but got: %v", err)
			}
		})
	}
}
