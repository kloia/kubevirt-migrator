package connectivity

import (
	"errors"
	"strings"
	"testing"

	"go.uber.org/zap"

	"github.com/kloia/kubevirt-migrator/internal/config"
	"github.com/kloia/kubevirt-migrator/internal/executor"
	"github.com/kloia/kubevirt-migrator/internal/kubernetes"
	"github.com/kloia/kubevirt-migrator/internal/template"
)

// Mock implementation of TemplateManager
type mockTemplateManager struct {
	renderError error
	renderCalls int
}

func (m *mockTemplateManager) RenderAndApply(kind template.TemplateKind, vars template.TemplateVariables, kubeconfig string) error {
	m.renderCalls++
	return m.renderError
}

func (m *mockTemplateManager) SetKubeCLI(kubeCLI string) {
	// No-op for test
}

// Mock implementation of SSHManagerInterface
type mockSSHManager struct {
	generateKeysError         error
	setupDestinationAuthError error
	generateKeysCalls         int
	setupDestinationAuthCalls int
}

func (m *mockSSHManager) GenerateKeys(cfg *config.Config) error {
	m.generateKeysCalls++
	return m.generateKeysError
}

func (m *mockSSHManager) SetupDestinationAuth(cfg *config.Config) error {
	m.setupDestinationAuthCalls++
	return m.setupDestinationAuthError
}

// Mock implementation of MountProvider
type mockMountProvider struct {
	checkConnectivityError error
	mountError             error
	verifyMountError       error
	unmountError           error
	checkConnectivityCalls int
	mountCalls             int
	verifyMountCalls       int
	unmountCalls           int
}

func (m *mockMountProvider) CheckConnectivity(cfg *config.Config, hostIP, port string) error {
	m.checkConnectivityCalls++
	return m.checkConnectivityError
}

func (m *mockMountProvider) Mount(cfg *config.Config, hostIP, port string) error {
	m.mountCalls++
	return m.mountError
}

func (m *mockMountProvider) VerifyMount(cfg *config.Config) error {
	m.verifyMountCalls++
	return m.verifyMountError
}

func (m *mockMountProvider) Unmount(cfg *config.Config) error {
	m.unmountCalls++
	return m.unmountError
}

func TestCheckManager_CheckConnectivity(t *testing.T) {
	testCases := []struct {
		name          string
		kubeCLI       string
		setupMocks    func(*mockTemplateManager, *mockSSHManager, *mockMountProvider, *kubernetes.MockKubernetesClient, *kubernetes.MockKubernetesClient)
		expectError   bool
		errorContains string
	}{
		{
			name:    "successful connectivity check",
			kubeCLI: "kubectl",
			setupMocks: func(tmplMgr *mockTemplateManager, sshMgr *mockSSHManager, mountProvider *mockMountProvider, srcClient *kubernetes.MockKubernetesClient, dstClient *kubernetes.MockKubernetesClient) {
				// Setup template manager
				tmplMgr.renderError = nil

				// Setup SSH manager
				sshMgr.generateKeysError = nil
				sshMgr.setupDestinationAuthError = nil

				// Setup mount provider
				mountProvider.checkConnectivityError = nil
				mountProvider.mountError = nil
				mountProvider.verifyMountError = nil
				mountProvider.unmountError = nil

				// Setup VM statuses
				srcClient.VMStatuses["test-namespace/test-vm"] = "Running"
				dstClient.VMStatuses["test-namespace/test-vm"] = "Stopped"

				// Setup Kubernetes clients
				dstClient.NodePorts["test-namespace/test-vm-dst-svc"] = 30123
				dstClient.PodHostIPs["test-namespace/test-vm-dst-replicator"] = "192.168.1.100"

				// Setup pod statuses
				srcClient.PodStatuses["test-namespace/test-vm-src-replicator"] = "ready"
				dstClient.PodStatuses["test-namespace/test-vm-dst-replicator"] = "ready"
			},
			expectError:   false,
			errorContains: "",
		},
		{
			name:    "template rendering failure",
			kubeCLI: "kubectl",
			setupMocks: func(tmplMgr *mockTemplateManager, sshMgr *mockSSHManager, mountProvider *mockMountProvider, srcClient *kubernetes.MockKubernetesClient, dstClient *kubernetes.MockKubernetesClient) {
				// Setup VM statuses
				srcClient.VMStatuses["test-namespace/test-vm"] = "Running"
				dstClient.VMStatuses["test-namespace/test-vm"] = "Stopped"

				tmplMgr.renderError = errors.New("template error")
			},
			expectError:   true,
			errorContains: "failed to setup test replicators",
		},
		{
			name:    "ssh key generation failure",
			kubeCLI: "kubectl",
			setupMocks: func(tmplMgr *mockTemplateManager, sshMgr *mockSSHManager, mountProvider *mockMountProvider, srcClient *kubernetes.MockKubernetesClient, dstClient *kubernetes.MockKubernetesClient) {
				// Setup VM statuses
				srcClient.VMStatuses["test-namespace/test-vm"] = "Running"
				dstClient.VMStatuses["test-namespace/test-vm"] = "Stopped"

				tmplMgr.renderError = nil
				sshMgr.generateKeysError = errors.New("ssh key error")

				// Setup pod statuses
				srcClient.PodStatuses["test-namespace/test-vm-src-replicator"] = "ready"
				dstClient.PodStatuses["test-namespace/test-vm-dst-replicator"] = "ready"
			},
			expectError:   true,
			errorContains: "failed to generate SSH keys",
		},
		{
			name:    "destination auth setup failure",
			kubeCLI: "kubectl",
			setupMocks: func(tmplMgr *mockTemplateManager, sshMgr *mockSSHManager, mountProvider *mockMountProvider, srcClient *kubernetes.MockKubernetesClient, dstClient *kubernetes.MockKubernetesClient) {
				// Setup VM statuses
				srcClient.VMStatuses["test-namespace/test-vm"] = "Running"
				dstClient.VMStatuses["test-namespace/test-vm"] = "Stopped"

				tmplMgr.renderError = nil
				sshMgr.generateKeysError = nil
				sshMgr.setupDestinationAuthError = errors.New("auth error")

				// Setup pod statuses
				srcClient.PodStatuses["test-namespace/test-vm-src-replicator"] = "ready"
				dstClient.PodStatuses["test-namespace/test-vm-dst-replicator"] = "ready"
			},
			expectError:   true,
			errorContains: "failed to setup destination auth",
		},
		{
			name:    "node port lookup failure",
			kubeCLI: "kubectl",
			setupMocks: func(tmplMgr *mockTemplateManager, sshMgr *mockSSHManager, mountProvider *mockMountProvider, srcClient *kubernetes.MockKubernetesClient, dstClient *kubernetes.MockKubernetesClient) {
				// Setup VM statuses
				srcClient.VMStatuses["test-namespace/test-vm"] = "Running"
				dstClient.VMStatuses["test-namespace/test-vm"] = "Stopped"

				tmplMgr.renderError = nil
				sshMgr.generateKeysError = nil
				sshMgr.setupDestinationAuthError = nil
				// Don't add NodePort entry

				// Setup pod statuses
				srcClient.PodStatuses["test-namespace/test-vm-src-replicator"] = "ready"
				dstClient.PodStatuses["test-namespace/test-vm-dst-replicator"] = "ready"
			},
			expectError:   true,
			errorContains: "failed to get destination NodePort",
		},
		{
			name:    "connectivity check failure",
			kubeCLI: "kubectl",
			setupMocks: func(tmplMgr *mockTemplateManager, sshMgr *mockSSHManager, mountProvider *mockMountProvider, srcClient *kubernetes.MockKubernetesClient, dstClient *kubernetes.MockKubernetesClient) {
				// Setup VM statuses
				srcClient.VMStatuses["test-namespace/test-vm"] = "Running"
				dstClient.VMStatuses["test-namespace/test-vm"] = "Stopped"

				tmplMgr.renderError = nil
				sshMgr.generateKeysError = nil
				sshMgr.setupDestinationAuthError = nil
				mountProvider.checkConnectivityError = errors.New("connectivity error")
				dstClient.NodePorts["test-namespace/test-vm-dst-svc"] = 30123
				dstClient.PodHostIPs["test-namespace/test-vm-dst-replicator"] = "192.168.1.100"

				// Setup pod statuses
				srcClient.PodStatuses["test-namespace/test-vm-src-replicator"] = "ready"
				dstClient.PodStatuses["test-namespace/test-vm-dst-replicator"] = "ready"
			},
			expectError:   true,
			errorContains: "connectivity check failed",
		},
		{
			name:    "mount failure",
			kubeCLI: "kubectl",
			setupMocks: func(tmplMgr *mockTemplateManager, sshMgr *mockSSHManager, mountProvider *mockMountProvider, srcClient *kubernetes.MockKubernetesClient, dstClient *kubernetes.MockKubernetesClient) {
				// Setup VM statuses
				srcClient.VMStatuses["test-namespace/test-vm"] = "Running"
				dstClient.VMStatuses["test-namespace/test-vm"] = "Stopped"

				tmplMgr.renderError = nil
				sshMgr.generateKeysError = nil
				sshMgr.setupDestinationAuthError = nil
				mountProvider.checkConnectivityError = nil
				mountProvider.mountError = errors.New("mount error")
				dstClient.NodePorts["test-namespace/test-vm-dst-svc"] = 30123
				dstClient.PodHostIPs["test-namespace/test-vm-dst-replicator"] = "192.168.1.100"

				// Setup pod statuses
				srcClient.PodStatuses["test-namespace/test-vm-src-replicator"] = "ready"
				dstClient.PodStatuses["test-namespace/test-vm-dst-replicator"] = "ready"
			},
			expectError:   true,
			errorContains: "mount test failed",
		},
		{
			name:    "mount verification failure",
			kubeCLI: "kubectl",
			setupMocks: func(tmplMgr *mockTemplateManager, sshMgr *mockSSHManager, mountProvider *mockMountProvider, srcClient *kubernetes.MockKubernetesClient, dstClient *kubernetes.MockKubernetesClient) {
				// Setup VM statuses
				srcClient.VMStatuses["test-namespace/test-vm"] = "Running"
				dstClient.VMStatuses["test-namespace/test-vm"] = "Stopped"

				tmplMgr.renderError = nil
				sshMgr.generateKeysError = nil
				sshMgr.setupDestinationAuthError = nil
				mountProvider.checkConnectivityError = nil
				mountProvider.mountError = nil
				mountProvider.verifyMountError = errors.New("verify error")
				dstClient.NodePorts["test-namespace/test-vm-dst-svc"] = 30123
				dstClient.PodHostIPs["test-namespace/test-vm-dst-replicator"] = "192.168.1.100"

				// Setup pod statuses
				srcClient.PodStatuses["test-namespace/test-vm-src-replicator"] = "ready"
				dstClient.PodStatuses["test-namespace/test-vm-dst-replicator"] = "ready"
			},
			expectError:   true,
			errorContains: "mount verification failed",
		},
		{
			name:    "unmount failure (ignored)",
			kubeCLI: "kubectl",
			setupMocks: func(tmplMgr *mockTemplateManager, sshMgr *mockSSHManager, mountProvider *mockMountProvider, srcClient *kubernetes.MockKubernetesClient, dstClient *kubernetes.MockKubernetesClient) {
				// Setup VM statuses
				srcClient.VMStatuses["test-namespace/test-vm"] = "Running"
				dstClient.VMStatuses["test-namespace/test-vm"] = "Stopped"

				tmplMgr.renderError = nil
				sshMgr.generateKeysError = nil
				sshMgr.setupDestinationAuthError = nil
				mountProvider.checkConnectivityError = nil
				mountProvider.mountError = nil
				mountProvider.verifyMountError = nil
				mountProvider.unmountError = errors.New("unmount error")
				dstClient.NodePorts["test-namespace/test-vm-dst-svc"] = 30123
				dstClient.PodHostIPs["test-namespace/test-vm-dst-replicator"] = "192.168.1.100"

				// Setup pod statuses
				srcClient.PodStatuses["test-namespace/test-vm-src-replicator"] = "ready"
				dstClient.PodStatuses["test-namespace/test-vm-dst-replicator"] = "ready"
			},
			expectError:   false, // Unmount errors are ignored
			errorContains: "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create mocks
			mockExecutor := executor.NewMockCommandExecutor()
			logger, _ := zap.NewDevelopment()
			mockTmplMgr := &mockTemplateManager{}
			mockSSHMgr := &mockSSHManager{}
			mockMountProvider := &mockMountProvider{}
			mockSrcClient := kubernetes.NewMockKubernetesClient()
			mockDstClient := kubernetes.NewMockKubernetesClient()

			// Setup mocks
			tc.setupMocks(mockTmplMgr, mockSSHMgr, mockMountProvider, mockSrcClient, mockDstClient)

			// Create CheckManager
			checkMgr := NewCheckManager(mockExecutor, logger, mockTmplMgr, mockSSHMgr, mockSrcClient, mockDstClient)
			checkMgr.SetMountProvider(mockMountProvider)

			// Create test config
			cfg := &config.Config{
				VMName:        "test-vm",
				Namespace:     "test-namespace",
				SrcKubeconfig: "src-kubeconfig.yaml",
				DstKubeconfig: "dst-kubeconfig.yaml",
				KubeCLI:       tc.kubeCLI,
			}

			// Execute
			err := checkMgr.CheckConnectivity(cfg)

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

func TestCheckManager_SetupTestReplicators(t *testing.T) {
	// Test cases for different KubeCLI options
	testCases := []struct {
		name     string
		kubeCLI  string
		mockFunc func(*mockTemplateManager, *kubernetes.MockKubernetesClient, *kubernetes.MockKubernetesClient)
		wantErr  bool
	}{
		{
			name:    "successful setup with kubectl",
			kubeCLI: "kubectl",
			mockFunc: func(mockTmplMgr *mockTemplateManager, mockSrcClient *kubernetes.MockKubernetesClient, mockDstClient *kubernetes.MockKubernetesClient) {
				mockTmplMgr.renderError = nil
				mockSrcClient.VMStatuses["test-namespace/test-vm"] = "Running"
				mockDstClient.VMStatuses["test-namespace/test-vm"] = "Stopped"
				mockSrcClient.PodStatuses["test-namespace/test-vm-src-replicator"] = "ready"
				mockDstClient.PodStatuses["test-namespace/test-vm-dst-replicator"] = "ready"
			},
			wantErr: false,
		},
		{
			name:    "error case",
			kubeCLI: "kubectl",
			mockFunc: func(mockTmplMgr *mockTemplateManager, mockSrcClient *kubernetes.MockKubernetesClient, mockDstClient *kubernetes.MockKubernetesClient) {
				mockTmplMgr.renderError = errors.New("render error")
				mockSrcClient.VMStatuses["test-namespace/test-vm"] = "Running"
				mockDstClient.VMStatuses["test-namespace/test-vm"] = "Stopped"
			},
			wantErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create mocks
			mockExecutor := executor.NewMockCommandExecutor()
			logger, _ := zap.NewDevelopment()
			mockTmplMgr := &mockTemplateManager{}
			mockSSHMgr := &mockSSHManager{}
			mockSrcClient := kubernetes.NewMockKubernetesClient()
			mockDstClient := kubernetes.NewMockKubernetesClient()

			// Setup mocks
			tc.mockFunc(mockTmplMgr, mockSrcClient, mockDstClient)

			// Create CheckManager
			checkMgr := NewCheckManager(mockExecutor, logger, mockTmplMgr, mockSSHMgr, mockSrcClient, mockDstClient)

			// Set up config
			cfg := &config.Config{
				VMName:        "test-vm",
				Namespace:     "test-namespace",
				SrcKubeconfig: "src-kubeconfig.yaml",
				DstKubeconfig: "dst-kubeconfig.yaml",
				KubeCLI:       tc.kubeCLI,
				SSHPort:       2222,
			}

			// Call setupTestReplicators method
			err := checkMgr.setupTestReplicators(cfg)

			// Verify
			if tc.wantErr {
				if err == nil {
					t.Errorf("expected error but got nil")
				}
			} else if err != nil {
				t.Errorf("expected no error but got: %v", err)
			}

			// Verify template manager was called if no error
			if !tc.wantErr && mockTmplMgr.renderCalls != 3 {
				t.Errorf("expected 3 render calls but got %d", mockTmplMgr.renderCalls)
			}
		})
	}
}

func TestCheckManager_CleanupTestReplicators(t *testing.T) {
	// Create mocks
	mockExecutor := executor.NewMockCommandExecutor()
	logger, _ := zap.NewDevelopment()
	mockTmplMgr := &mockTemplateManager{}
	mockSSHMgr := &mockSSHManager{}
	mockSrcClient := kubernetes.NewMockKubernetesClient()
	mockDstClient := kubernetes.NewMockKubernetesClient()

	// Create CheckManager
	checkMgr := NewCheckManager(mockExecutor, logger, mockTmplMgr, mockSSHMgr, mockSrcClient, mockDstClient)

	// Set up config
	cfg := &config.Config{
		VMName:        "test-vm",
		Namespace:     "test-namespace",
		SrcKubeconfig: "src-kubeconfig.yaml",
		DstKubeconfig: "dst-kubeconfig.yaml",
		KubeCLI:       "kubectl",
	}

	// Call cleanupTestReplicators method
	err := checkMgr.cleanupTestReplicators(cfg)

	// Verify
	if err != nil {
		t.Errorf("expected no error but got: %v", err)
	}

	// Verify cleanup was called with correct parameters
	srcCleanupKey := "test-namespace/test-vm:false"
	dstCleanupKey := "test-namespace/test-vm:true"

	if _, ok := mockSrcClient.CleanupCalls[srcCleanupKey]; !ok {
		t.Errorf("source cleanup not called with key %s", srcCleanupKey)
	}

	if _, ok := mockDstClient.CleanupCalls[dstCleanupKey]; !ok {
		t.Errorf("destination cleanup not called with key %s", dstCleanupKey)
	}
}
