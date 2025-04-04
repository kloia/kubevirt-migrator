package kubernetes

import (
	"fmt"
	"time"
)

// MockKubernetesClient implements KubernetesClient for testing
type MockKubernetesClient struct {
	VMStatuses       map[string]string
	PodStatuses      map[string]string
	NodePorts        map[string]int
	ExportedVMs      map[string][]byte
	ImportedVMs      [][]byte
	CreatedServices  [][]byte
	CreatedJobs      [][]byte
	CreatedSecrets   [][]byte
	CreatedCronJobs  [][]byte
	ExecutedCommands map[string]string
	CleanupCalls     map[string]bool
}

// NewMockKubernetesClient creates a new mock client
func NewMockKubernetesClient() *MockKubernetesClient {
	return &MockKubernetesClient{
		VMStatuses:       make(map[string]string),
		PodStatuses:      make(map[string]string),
		NodePorts:        make(map[string]int),
		ExportedVMs:      make(map[string][]byte),
		ImportedVMs:      [][]byte{},
		CreatedServices:  [][]byte{},
		CreatedJobs:      [][]byte{},
		CreatedSecrets:   [][]byte{},
		CreatedCronJobs:  [][]byte{},
		ExecutedCommands: make(map[string]string),
		CleanupCalls:     make(map[string]bool),
	}
}

// GetVMStatus returns the status of a VM
func (m *MockKubernetesClient) GetVMStatus(vmName, namespace string) (string, error) {
	key := fmt.Sprintf("%s/%s", namespace, vmName)
	if status, ok := m.VMStatuses[key]; ok {
		return status, nil
	}
	return "", fmt.Errorf("vm %s not found in namespace %s", vmName, namespace)
}

// StartVM starts a VM
func (m *MockKubernetesClient) StartVM(vmName, namespace string) error {
	key := fmt.Sprintf("%s/%s", namespace, vmName)
	if _, ok := m.VMStatuses[key]; !ok {
		return fmt.Errorf("vm %s not found in namespace %s", vmName, namespace)
	}
	m.VMStatuses[key] = "Running"
	return nil
}

// StopVM stops a VM
func (m *MockKubernetesClient) StopVM(vmName, namespace string) error {
	key := fmt.Sprintf("%s/%s", namespace, vmName)
	if _, ok := m.VMStatuses[key]; !ok {
		return fmt.Errorf("vm %s not found in namespace %s", vmName, namespace)
	}
	m.VMStatuses[key] = "Stopped"
	return nil
}

// ExportVM exports a VM definition
func (m *MockKubernetesClient) ExportVM(vmName, namespace string) ([]byte, error) {
	key := fmt.Sprintf("%s/%s", namespace, vmName)
	if data, ok := m.ExportedVMs[key]; ok {
		return data, nil
	}
	return nil, fmt.Errorf("vm %s not found in namespace %s", vmName, namespace)
}

// ExportVMWithPreservedIP exports a VM definition with preserved IP
func (m *MockKubernetesClient) ExportVMWithPreservedIP(vmName, namespace string) ([]byte, error) {
	key := fmt.Sprintf("%s/%s", namespace, vmName)
	if data, ok := m.ExportedVMs[key]; ok {
		return data, nil
	}
	return nil, fmt.Errorf("vm %s not found in namespace %s", vmName, namespace)
}

// ImportVM imports a VM definition
func (m *MockKubernetesClient) ImportVM(vmDef []byte, namespace string) error {
	m.ImportedVMs = append(m.ImportedVMs, vmDef)
	return nil
}

// WaitForVMStatus waits for a VM to reach a specific status
func (m *MockKubernetesClient) WaitForVMStatus(vmName, namespace, expectedStatus string, timeout time.Duration) error {
	key := fmt.Sprintf("%s/%s", namespace, vmName)
	if status, ok := m.VMStatuses[key]; ok {
		if status == expectedStatus {
			return nil
		}
		return fmt.Errorf("vm %s in namespace %s has status %s, expected %s", vmName, namespace, status, expectedStatus)
	}
	return fmt.Errorf("vm %s not found in namespace %s", vmName, namespace)
}

// GetPodStatus returns the status of a pod
func (m *MockKubernetesClient) GetPodStatus(podName, namespace string) (string, error) {
	key := fmt.Sprintf("%s/%s", namespace, podName)
	if status, ok := m.PodStatuses[key]; ok {
		return status, nil
	}
	return "", fmt.Errorf("pod %s not found in namespace %s", podName, namespace)
}

// WaitForPod waits for a pod to reach a specific condition
func (m *MockKubernetesClient) WaitForPod(podName, namespace string, condition string, timeout time.Duration) error {
	key := fmt.Sprintf("%s/%s", namespace, podName)
	if status, ok := m.PodStatuses[key]; ok {
		if status == condition {
			return nil
		}
		return fmt.Errorf("pod %s in namespace %s has status %s, expected %s", podName, namespace, status, condition)
	}
	return fmt.Errorf("pod %s not found in namespace %s", podName, namespace)
}

// ExecInPod executes a command in a pod
func (m *MockKubernetesClient) ExecInPod(podName, namespace, command string) (string, error) {
	key := fmt.Sprintf("%s/%s:%s", namespace, podName, command)
	if output, ok := m.ExecutedCommands[key]; ok {
		return output, nil
	}
	return "", fmt.Errorf("no mock response for pod command %s in pod %s/%s", command, namespace, podName)
}

// CreateService creates a service
func (m *MockKubernetesClient) CreateService(svcDef []byte, namespace string) error {
	m.CreatedServices = append(m.CreatedServices, svcDef)
	return nil
}

// GetNodePort gets the node port for a service
func (m *MockKubernetesClient) GetNodePort(svcName, namespace string) (int, error) {
	key := fmt.Sprintf("%s/%s", namespace, svcName)
	if port, ok := m.NodePorts[key]; ok {
		return port, nil
	}
	return 0, fmt.Errorf("service %s not found in namespace %s", svcName, namespace)
}

// CreateJob creates a job
func (m *MockKubernetesClient) CreateJob(jobDef []byte, namespace string) error {
	m.CreatedJobs = append(m.CreatedJobs, jobDef)
	return nil
}

// WaitForJob waits for a job to complete
func (m *MockKubernetesClient) WaitForJob(jobName, namespace string, timeout time.Duration) error {
	// For simplicity, mock always succeeds
	return nil
}

// CreateSecret creates a secret
func (m *MockKubernetesClient) CreateSecret(secretDef []byte, namespace string) error {
	m.CreatedSecrets = append(m.CreatedSecrets, secretDef)
	return nil
}

// CreateCronJob creates a cronjob
func (m *MockKubernetesClient) CreateCronJob(cronJobDef []byte, namespace string) error {
	m.CreatedCronJobs = append(m.CreatedCronJobs, cronJobDef)
	return nil
}

// SuspendCronJob suspends a cronjob
func (m *MockKubernetesClient) SuspendCronJob(cronJobName, namespace string) error {
	return nil
}

// CleanupMigrationResources cleans up migration resources
func (m *MockKubernetesClient) CleanupMigrationResources(vmName, namespace string, isDestination bool) error {
	key := fmt.Sprintf("%s/%s:%v", namespace, vmName, isDestination)
	m.CleanupCalls[key] = true
	return nil
}
