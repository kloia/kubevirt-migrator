package kubernetes

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/kloia/kubevirt-migrator/internal/executor"
	"github.com/kloia/kubevirt-migrator/internal/sync"
)

func TestBaseClient_GetVMStatus(t *testing.T) {
	// Create test logger
	logger, _ := zap.NewDevelopment()
	mockSyncTool := &sync.MockSyncCommand{
		ToolName: "mock-sync",
	}

	tests := []struct {
		name          string
		vmName        string
		namespace     string
		kubeCLI       string
		kubeconfig    string
		mockResponses map[string]struct {
			Output string
			Error  error
		}
		expectedStatus string
		expectError    bool
	}{
		{
			name:       "successful VM status retrieval with kubectl",
			vmName:     "test-vm",
			namespace:  "test-namespace",
			kubeCLI:    "kubectl",
			kubeconfig: "test-kubeconfig.yaml",
			mockResponses: map[string]struct {
				Output string
				Error  error
			}{
				"kubectl get vm test-vm -n test-namespace --kubeconfig test-kubeconfig.yaml --no-headers": {
					Output: "test-vm   Running   True      1h",
					Error:  nil,
				},
			},
			expectedStatus: "Running",
			expectError:    false,
		},
		{
			name:       "successful VM status retrieval with oc",
			vmName:     "test-vm",
			namespace:  "test-namespace",
			kubeCLI:    "oc",
			kubeconfig: "test-kubeconfig.yaml",
			mockResponses: map[string]struct {
				Output string
				Error  error
			}{
				"oc get vm test-vm -n test-namespace --kubeconfig test-kubeconfig.yaml --no-headers": {
					Output: "test-vm   Stopped   False     1h",
					Error:  nil,
				},
			},
			expectedStatus: "Stopped",
			expectError:    false,
		},
		{
			name:       "command execution error",
			vmName:     "test-vm",
			namespace:  "test-namespace",
			kubeCLI:    "kubectl",
			kubeconfig: "test-kubeconfig.yaml",
			mockResponses: map[string]struct {
				Output string
				Error  error
			}{
				"kubectl get vm test-vm -n test-namespace --kubeconfig test-kubeconfig.yaml --no-headers": {
					Output: "",
					Error:  errors.New("command execution failed"),
				},
			},
			expectedStatus: "",
			expectError:    true,
		},
		{
			name:       "unexpected output format",
			vmName:     "test-vm",
			namespace:  "test-namespace",
			kubeCLI:    "kubectl",
			kubeconfig: "test-kubeconfig.yaml",
			mockResponses: map[string]struct {
				Output string
				Error  error
			}{
				"kubectl get vm test-vm -n test-namespace --kubeconfig test-kubeconfig.yaml --no-headers": {
					Output: "invalid-format",
					Error:  nil,
				},
			},
			expectedStatus: "",
			expectError:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock executor
			mockExecutor := executor.NewMockCommandExecutor()
			for cmd, result := range tt.mockResponses {
				mockExecutor.AddCommandResult(cmd, result.Output, result.Error)
			}

			// Create base client
			client := NewBaseClient(tt.kubeCLI, tt.kubeconfig, mockExecutor, mockSyncTool, logger)

			// Call the method
			status, err := client.GetVMStatus(tt.vmName, tt.namespace)

			// Check error
			if tt.expectError {
				if err == nil {
					t.Errorf("expected error but got nil")
				}
			} else {
				if err != nil {
					t.Errorf("expected no error but got: %v", err)
				}
				if status != tt.expectedStatus {
					t.Errorf("expected status '%s', got '%s'", tt.expectedStatus, status)
				}
			}

			// Verify the correct command was executed
			expectedCmd := tt.kubeCLI + " get vm " + tt.vmName + " -n " + tt.namespace + " --kubeconfig " + tt.kubeconfig + " --no-headers"
			found := false
			for _, cmd := range mockExecutor.ExecutedCommands {
				if cmd == expectedCmd {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("expected command '%s' not executed", expectedCmd)
			}
		})
	}
}

func TestBaseClient_StartVM(t *testing.T) {
	// Create test logger
	logger, _ := zap.NewDevelopment()
	mockSyncTool := &sync.MockSyncCommand{
		ToolName: "mock-sync",
	}

	tests := []struct {
		name          string
		vmName        string
		namespace     string
		kubeCLI       string
		kubeconfig    string
		mockResponses map[string]struct {
			Output string
			Error  error
		}
		expectError bool
	}{
		{
			name:       "start VM with runStrategy",
			vmName:     "test-vm",
			namespace:  "test-namespace",
			kubeCLI:    "kubectl",
			kubeconfig: "test-kubeconfig.yaml",
			mockResponses: map[string]struct {
				Output string
				Error  error
			}{
				"kubectl get vm test-vm -n test-namespace --kubeconfig test-kubeconfig.yaml -o jsonpath='{.spec.runStrategy}'": {
					Output: "'Always'",
					Error:  nil,
				},
				"kubectl patch vm test-vm -n test-namespace --kubeconfig test-kubeconfig.yaml --type merge -p {\"spec\":{\"runStrategy\":\"Always\"}}": {
					Output: "virtualmachine.kubevirt.io/test-vm patched",
					Error:  nil,
				},
			},
			expectError: false,
		},
		{
			name:       "start VM with running field",
			vmName:     "test-vm",
			namespace:  "test-namespace",
			kubeCLI:    "kubectl",
			kubeconfig: "test-kubeconfig.yaml",
			mockResponses: map[string]struct {
				Output string
				Error  error
			}{
				"kubectl get vm test-vm -n test-namespace --kubeconfig test-kubeconfig.yaml -o jsonpath='{.spec.runStrategy}'": {
					Output: "",
					Error:  errors.New("not found"),
				},
				"kubectl patch vm test-vm -n test-namespace --kubeconfig test-kubeconfig.yaml --type merge -p {\"spec\":{\"running\":true}}": {
					Output: "virtualmachine.kubevirt.io/test-vm patched",
					Error:  nil,
				},
			},
			expectError: false,
		},
		{
			name:       "error patching VM",
			vmName:     "test-vm",
			namespace:  "test-namespace",
			kubeCLI:    "kubectl",
			kubeconfig: "test-kubeconfig.yaml",
			mockResponses: map[string]struct {
				Output string
				Error  error
			}{
				"kubectl get vm test-vm -n test-namespace --kubeconfig test-kubeconfig.yaml -o jsonpath='{.spec.runStrategy}'": {
					Output: "",
					Error:  errors.New("not found"),
				},
				"kubectl patch vm test-vm -n test-namespace --kubeconfig test-kubeconfig.yaml --type merge -p {\"spec\":{\"running\":true}}": {
					Output: "",
					Error:  errors.New("failed to patch VM"),
				},
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock executor
			mockExecutor := executor.NewMockCommandExecutor()
			for cmd, result := range tt.mockResponses {
				mockExecutor.AddCommandResult(cmd, result.Output, result.Error)
			}

			// Create base client
			client := NewBaseClient(tt.kubeCLI, tt.kubeconfig, mockExecutor, mockSyncTool, logger)

			// Call the method
			err := client.StartVM(tt.vmName, tt.namespace)

			// Check error
			if tt.expectError {
				if err == nil {
					t.Errorf("expected error but got nil")
				}
			} else {
				if err != nil {
					t.Errorf("expected no error but got: %v", err)
				}
			}
		})
	}
}

func TestBaseClient_ExportAndImportVM(t *testing.T) {
	// Create test logger
	logger, _ := zap.NewDevelopment()
	mockSyncTool := &sync.MockSyncCommand{
		ToolName: "mock-sync",
	}

	vmName := "test-vm"
	namespace := "test-namespace"
	kubeconfig := "test-kubeconfig.yaml"
	kubeCLI := "kubectl"

	// Sample VM definition
	sampleVM := `apiVersion: kubevirt.io/v1
kind: VirtualMachine
metadata:
  name: test-vm
  namespace: test-namespace
spec:
  running: true
  template:
    spec:
      domain:
        devices: {}
`

	// Create mock executor
	mockExecutor := executor.NewMockCommandExecutor()
	mockExecutor.AddCommandResult(
		"kubectl get vm test-vm -n test-namespace --kubeconfig test-kubeconfig.yaml -o yaml",
		sampleVM,
		nil,
	)
	mockExecutor.AddCommandResult(
		"kubectl apply -n test-namespace --kubeconfig test-kubeconfig.yaml -f /tmp/vm-import.yaml",
		"virtualmachine.kubevirt.io/test-vm created",
		nil,
	)

	// Create base client
	client := NewBaseClient(kubeCLI, kubeconfig, mockExecutor, mockSyncTool, logger)

	// Test export VM
	vmDef, err := client.ExportVM(vmName, namespace)
	if err != nil {
		t.Errorf("expected no error on ExportVM but got: %v", err)
	}
	if string(vmDef) != sampleVM {
		t.Errorf("expected vmDef '%s', got '%s'", sampleVM, string(vmDef))
	}

	// Test import VM (this is more complex to test fully as it writes to a file)
	err = client.ImportVM(vmDef, namespace)
	if err != nil {
		t.Errorf("expected no error on ImportVM but got: %v", err)
	}
}

func TestBaseClient_GetPodStatus(t *testing.T) {
	// Create test logger
	logger, _ := zap.NewDevelopment()
	mockSyncTool := &sync.MockSyncCommand{
		ToolName: "mock-sync",
	}

	podName := "test-pod"
	namespace := "test-namespace"
	kubeconfig := "test-kubeconfig.yaml"
	kubeCLI := "kubectl"

	// Create mock executor
	mockExecutor := executor.NewMockCommandExecutor()
	mockExecutor.AddCommandResult(
		"kubectl get pod test-pod -n test-namespace --kubeconfig test-kubeconfig.yaml --no-headers",
		"test-pod   1/1     Running   0          10m",
		nil,
	)

	// Create base client
	client := NewBaseClient(kubeCLI, kubeconfig, mockExecutor, mockSyncTool, logger)

	// Test getting pod status
	status, err := client.GetPodStatus(podName, namespace)
	if err != nil {
		t.Errorf("expected no error but got: %v", err)
	}
	if status != "Running" {
		t.Errorf("expected status 'Running', got '%s'", status)
	}
}

func TestBaseClient_WaitForVMStatus(t *testing.T) {
	// Create test logger
	logger, _ := zap.NewDevelopment()
	mockSyncTool := &sync.MockSyncCommand{
		ToolName: "mock-sync",
	}

	vmName := "test-vm"
	namespace := "test-namespace"
	kubeconfig := "test-kubeconfig.yaml"
	kubeCLI := "kubectl"
	expectedStatus := "Running"
	timeout := 10 * time.Second // Shorter timeout

	// Create a custom mock executor that returns different values on subsequent calls
	mockExecutor := &customMockExecutor{
		callCounts: make(map[string]int),
	}

	// Create base client
	client := NewBaseClient(kubeCLI, kubeconfig, mockExecutor, mockSyncTool, logger)

	// Test waiting for VM status
	err := client.WaitForVMStatus(vmName, namespace, expectedStatus, timeout)
	if err != nil {
		t.Errorf("expected no error but got: %v", err)
	}
}

// customMockExecutor, Custom mock that returns different responses on each call
type customMockExecutor struct {
	callCounts map[string]int
	executor.CommandExecutor
}

func (m *customMockExecutor) Execute(command string, args ...string) (string, error) {
	cmdString := command
	for _, arg := range args {
		cmdString += " " + arg
	}

	// Custom response for VM status query
	if strings.Contains(cmdString, "get vm") && strings.Contains(cmdString, "--no-headers") {
		m.callCounts[cmdString]++
		count := m.callCounts[cmdString]

		if count == 1 {
			return "test-vm   Pending   False     0s", nil
		}
		return "test-vm   Running   True      5s", nil
	}

	return "", fmt.Errorf("unexpected command: %s", cmdString)
}

func TestBaseClient_CreateService(t *testing.T) {
	// Create test logger
	logger, _ := zap.NewDevelopment()
	mockSyncTool := &sync.MockSyncCommand{
		ToolName: "mock-sync",
	}

	namespace := "test-namespace"
	kubeconfig := "test-kubeconfig.yaml"
	kubeCLI := "kubectl"

	// Sample service definition
	sampleSvc := []byte(`apiVersion: v1
kind: Service
metadata:
  name: test-service
  namespace: test-namespace
spec:
  selector:
    app: test-app
  ports:
  - port: 80
    targetPort: 8080
  type: NodePort
`)

	// Create mock executor
	mockExecutor := executor.NewMockCommandExecutor()
	mockExecutor.AddCommandResult(
		"kubectl apply -n test-namespace --kubeconfig test-kubeconfig.yaml -f /tmp/svc-create.yaml",
		"service/test-service created",
		nil,
	)

	// Create base client
	client := NewBaseClient(kubeCLI, kubeconfig, mockExecutor, mockSyncTool, logger)

	// Test creating a service
	err := client.CreateService(sampleSvc, namespace)
	if err != nil {
		t.Errorf("expected no error but got: %v", err)
	}
}

func TestBaseClient_GetNodePort(t *testing.T) {
	// Create test logger
	logger, _ := zap.NewDevelopment()
	mockSyncTool := &sync.MockSyncCommand{
		ToolName: "mock-sync",
	}

	svcName := "test-service"
	namespace := "test-namespace"
	kubeconfig := "test-kubeconfig.yaml"
	kubeCLI := "kubectl"
	expectedPort := 30123

	// Create mock executor
	mockExecutor := executor.NewMockCommandExecutor()
	mockExecutor.AddCommandResult(
		"kubectl get svc test-service -n test-namespace --kubeconfig test-kubeconfig.yaml -o jsonpath='{.spec.ports[0].nodePort}'",
		"'30123'",
		nil,
	)

	// Create base client
	client := NewBaseClient(kubeCLI, kubeconfig, mockExecutor, mockSyncTool, logger)

	// Test getting node port
	port, err := client.GetNodePort(svcName, namespace)
	if err != nil {
		t.Errorf("expected no error but got: %v", err)
	}
	if port != expectedPort {
		t.Errorf("expected port %d, got %d", expectedPort, port)
	}
}
