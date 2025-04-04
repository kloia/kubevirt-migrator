package kubernetes

import (
	"testing"

	"go.uber.org/zap"

	"github.com/kloia/kubevirt-migrator/internal/executor"
	"github.com/kloia/kubevirt-migrator/internal/sync"
)

func TestClientFactory_CreateClient(t *testing.T) {
	// Create dependencies
	mockExecutor := executor.NewMockCommandExecutor()
	mockSyncTool := &sync.MockSyncCommand{
		ToolName: "mock-sync",
	}
	logger, _ := zap.NewDevelopment()

	// Create factory
	factory := NewClientFactory(mockExecutor, mockSyncTool, logger)

	tests := []struct {
		name       string
		clientType ClientType
		expectType interface{}
		expectErr  bool
	}{
		{
			name:       "create oc client",
			clientType: ClientTypeOC,
			expectType: &OCClient{},
			expectErr:  false,
		},
		{
			name:       "create kubectl client",
			clientType: ClientTypeKubectl,
			expectType: &KubectlClient{},
			expectErr:  false,
		},
		{
			name:       "unsupported client type",
			clientType: ClientType("unsupported"),
			expectType: nil,
			expectErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create client
			client, err := factory.CreateClient(tt.clientType, "test-kubeconfig.yaml")

			// Check for errors
			if tt.expectErr {
				if err == nil {
					t.Errorf("expected error but got nil")
				}
			} else {
				if err != nil {
					t.Errorf("expected no error but got: %v", err)
				}

				// Check client type
				switch tt.expectType.(type) {
				case *OCClient:
					if _, ok := client.(*OCClient); !ok {
						t.Errorf("expected OCClient but got %T", client)
					}
				case *KubectlClient:
					if _, ok := client.(*KubectlClient); !ok {
						t.Errorf("expected KubectlClient but got %T", client)
					}
				}

				// Verify the client is configured correctly
				var kubeConfig string
				var cmdName string

				// Extract the base client config based on client type
				switch c := client.(type) {
				case *OCClient:
					kubeConfig = c.BaseClient.kubeconfig
					cmdName = c.BaseClient.cmdName
				case *KubectlClient:
					kubeConfig = c.BaseClient.kubeconfig
					cmdName = c.BaseClient.cmdName
				}

				// Verify kubeconfig
				if kubeConfig != "test-kubeconfig.yaml" {
					t.Errorf("expected kubeconfig 'test-kubeconfig.yaml', got '%s'", kubeConfig)
				}

				// Verify kubeCLI
				expectedCLI := string(tt.clientType)
				if cmdName != expectedCLI {
					t.Errorf("expected cmdName '%s', got '%s'", expectedCLI, cmdName)
				}
			}
		})
	}
}
