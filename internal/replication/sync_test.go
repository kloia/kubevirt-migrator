package replication

import (
	"errors"
	"fmt"
	"testing"

	"go.uber.org/zap"

	"github.com/kloia/kubevirt-migrator/internal/config"
	"github.com/kloia/kubevirt-migrator/internal/encrypt/ssh"
	"github.com/kloia/kubevirt-migrator/internal/executor"
	"github.com/kloia/kubevirt-migrator/internal/kubernetes"
	"github.com/kloia/kubevirt-migrator/internal/sync"
	"github.com/kloia/kubevirt-migrator/internal/template"
)

// MockTemplateManager is a mock implementation of template manager
type MockTemplateManager struct {
	RenderCalls      int
	RenderError      error
	RenderKind       template.TemplateKind
	RenderKubeconfig string
}

func (m *MockTemplateManager) RenderAndApply(kind template.TemplateKind, vars template.TemplateVariables, kubeconfig string) error {
	m.RenderCalls++
	m.RenderKind = kind
	m.RenderKubeconfig = kubeconfig
	return m.RenderError
}

func (m *MockTemplateManager) SetKubeCLI(kubeCLI string) {
	// Not used in tests
}

func TestSyncManager_SetSyncTool(t *testing.T) {
	// Create dependencies
	mockExecutor := executor.NewMockCommandExecutor()
	logger, _ := zap.NewDevelopment()
	mockSSHManager := ssh.NewSSHManager(mockExecutor, logger)
	mockTemplateManager := &MockTemplateManager{}

	// Create mock Kubernetes clients using the existing implementation
	mockSrcClient := kubernetes.NewMockKubernetesClient()
	mockDstClient := kubernetes.NewMockKubernetesClient()

	// Create sync manager
	syncManager := NewSyncManager(mockExecutor, logger, mockSSHManager, mockTemplateManager, mockSrcClient, mockDstClient)

	// Create mock sync tool
	mockSyncTool := &sync.MockSyncCommand{
		ToolName: "mock-sync",
	}

	// Set the sync tool
	syncManager.SetSyncTool(mockSyncTool)

	// Verify the sync tool was set
	if syncManager.syncTool != mockSyncTool {
		t.Errorf("expected syncTool to be set, but it wasn't")
	}
}

func TestSyncManager_GetDestinationInfo(t *testing.T) {
	// Create dependencies
	mockExecutor := executor.NewMockCommandExecutor()
	logger, _ := zap.NewDevelopment()
	mockSSHManager := ssh.NewSSHManager(mockExecutor, logger)
	mockTemplateManager := &MockTemplateManager{}

	// Create test config
	cfg := &config.Config{
		VMName:        "test-vm",
		Namespace:     "test-namespace",
		DstKubeconfig: "test-kubeconfig.yaml",
	}

	// Create mock clients using the existing implementation
	mockSrcClient := kubernetes.NewMockKubernetesClient()
	mockDstClient := kubernetes.NewMockKubernetesClient()

	// Configure mock responses
	mockDstClient.NodePorts[fmt.Sprintf("%s/%s-dst-svc", cfg.Namespace, cfg.VMName)] = 30123
	mockDstClient.PodHostIPs[fmt.Sprintf("%s/%s-dst-replicator", cfg.Namespace, cfg.VMName)] = "192.168.1.100"

	// Create sync manager
	syncManager := NewSyncManager(mockExecutor, logger, mockSSHManager, mockTemplateManager, mockSrcClient, mockDstClient)

	// Call the method
	nodePort, hostIP, err := syncManager.GetDestinationInfo(cfg)

	// Verify results
	if err != nil {
		t.Errorf("expected no error but got: %v", err)
	}
	if nodePort != "30123" {
		t.Errorf("expected nodePort '30123' but got '%s'", nodePort)
	}
	if hostIP != "192.168.1.100" {
		t.Errorf("expected hostIP '192.168.1.100' but got '%s'", hostIP)
	}

	// Test error handling for GetNodePort - by removing the mock entry
	delete(mockDstClient.NodePorts, fmt.Sprintf("%s/%s-dst-svc", cfg.Namespace, cfg.VMName))
	_, _, err = syncManager.GetDestinationInfo(cfg)
	if err == nil {
		t.Errorf("expected error but got nil")
	}
}

func TestSyncManager_CreateSyncCommand(t *testing.T) {
	// Create dependencies
	mockExecutor := executor.NewMockCommandExecutor()
	logger, _ := zap.NewDevelopment()
	mockSSHManager := ssh.NewSSHManager(mockExecutor, logger)
	mockTemplateManager := &MockTemplateManager{}

	// Create mock Kubernetes clients
	mockSrcClient := kubernetes.NewMockKubernetesClient()
	mockDstClient := kubernetes.NewMockKubernetesClient()

	// Create test config
	cfg := &config.Config{
		VMName:        "test-vm",
		Namespace:     "test-namespace",
		SrcKubeconfig: "src-kubeconfig.yaml",
		DstKubeconfig: "dst-kubeconfig.yaml",
		KubeCLI:       "kubectl",
		SyncTool:      "rclone",
	}

	// Create sync manager
	syncManager := NewSyncManager(mockExecutor, logger, mockSSHManager, mockTemplateManager, mockSrcClient, mockDstClient)

	// Test with no sync tool set (should use default rclone)
	nodePort := "30123"
	hostIP := "192.168.1.100"

	cmd, err := syncManager.CreateSyncCommand(nodePort, hostIP, cfg)
	if err != nil {
		t.Errorf("expected no error but got: %v", err)
	}
	if cmd == "" {
		t.Errorf("expected a command string, got empty string")
	}

	// Test with a generic sync tool set - only verify we get a command back
	mockSyncTool := sync.NewMockSyncCommand("mock-sync-tool")
	syncManager.SetSyncTool(mockSyncTool)

	cmd, err = syncManager.CreateSyncCommand(nodePort, hostIP, cfg)
	if err != nil {
		t.Errorf("expected no error but got: %v", err)
	}
	if cmd == "" {
		t.Errorf("expected a command string, got empty string")
	}
}

func TestSyncManager_PerformFinalSync(t *testing.T) {
	// Create dependencies
	mockExecutor := executor.NewMockCommandExecutor()
	logger, _ := zap.NewDevelopment()
	mockSSHManager := ssh.NewSSHManager(mockExecutor, logger)
	mockTemplateManager := &MockTemplateManager{}

	// Create mock Kubernetes clients
	mockSrcClient := kubernetes.NewMockKubernetesClient()
	mockDstClient := kubernetes.NewMockKubernetesClient()

	// Create test config
	cfg := &config.Config{
		VMName:        "test-vm",
		Namespace:     "test-namespace",
		SrcKubeconfig: "src-kubeconfig.yaml",
		KubeCLI:       "kubectl",
	}

	// Create mock responses
	mockExecutor.AddCommandResult(
		"kubectl create job --from=cronjob/test-vm-repl-cronjob test-vm-repl-final-job -n test-namespace --kubeconfig src-kubeconfig.yaml",
		"job.batch/test-vm-repl-final-job created",
		nil,
	)
	mockExecutor.AddCommandResult(
		"kubectl wait job test-vm-repl-final-job -n test-namespace --kubeconfig src-kubeconfig.yaml --for=condition=complete --timeout=-1m",
		"job.batch/test-vm-repl-final-job condition met",
		nil,
	)

	// Create sync manager
	syncManager := NewSyncManager(mockExecutor, logger, mockSSHManager, mockTemplateManager, mockSrcClient, mockDstClient)

	// Call the method
	err := syncManager.PerformFinalSync(cfg)

	// Verify results
	if err != nil {
		t.Errorf("expected no error but got: %v", err)
	}
}

func TestSyncManager_SuspendCronJob(t *testing.T) {
	// Create dependencies
	mockExecutor := executor.NewMockCommandExecutor()
	logger, _ := zap.NewDevelopment()
	mockSSHManager := ssh.NewSSHManager(mockExecutor, logger)
	mockTemplateManager := &MockTemplateManager{}

	// Create mock Kubernetes clients
	mockSrcClient := kubernetes.NewMockKubernetesClient()
	mockDstClient := kubernetes.NewMockKubernetesClient()

	// Create test config
	cfg := &config.Config{
		VMName:        "test-vm",
		Namespace:     "test-namespace",
		SrcKubeconfig: "src-kubeconfig.yaml",
		KubeCLI:       "kubectl",
	}

	// Create mock responses
	mockExecutor.AddCommandResult(
		"kubectl patch cronjob test-vm-repl-cronjob -n test-namespace --kubeconfig src-kubeconfig.yaml -p {\"spec\" : {\"suspend\" : true }}",
		"cronjob.batch/test-vm-repl-cronjob patched",
		nil,
	)

	// Create sync manager
	syncManager := NewSyncManager(mockExecutor, logger, mockSSHManager, mockTemplateManager, mockSrcClient, mockDstClient)

	// Call the method
	err := syncManager.SuspendCronJob(cfg)

	// Verify results
	if err != nil {
		t.Errorf("expected no error but got: %v", err)
	}
}

func TestSyncManager_ErrorHandling(t *testing.T) {
	// Create dependencies
	mockExecutor := executor.NewMockCommandExecutor()
	logger, _ := zap.NewDevelopment()
	mockSSHManager := ssh.NewSSHManager(mockExecutor, logger)
	mockTemplateManager := &MockTemplateManager{
		RenderError: errors.New("template render error"),
	}

	// Create mock Kubernetes clients - with error responses
	mockSrcClient := kubernetes.NewMockKubernetesClient()
	mockDstClient := kubernetes.NewMockKubernetesClient()
	// We don't set nodePort entry, so GetNodePort will fail

	// Create test config
	cfg := &config.Config{
		VMName:        "test-vm",
		Namespace:     "test-namespace",
		SrcKubeconfig: "src-kubeconfig.yaml",
		DstKubeconfig: "dst-kubeconfig.yaml",
		KubeCLI:       "kubectl",
	}

	// Create sync manager
	syncManager := NewSyncManager(mockExecutor, logger, mockSSHManager, mockTemplateManager, mockSrcClient, mockDstClient)

	// Test GetDestinationInfo error handling with the mock client error
	_, _, err := syncManager.GetDestinationInfo(cfg)
	if err == nil {
		t.Errorf("expected error but got nil")
	}
}
