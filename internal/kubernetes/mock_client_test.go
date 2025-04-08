package kubernetes

import (
	"testing"
	"time"
)

func TestMockKubernetesClient_VMOperations(t *testing.T) {
	// Create a mock client
	client := NewMockKubernetesClient()

	// Set up test VM
	testNS := "test-namespace"
	testVM := "test-vm"
	vmKey := testNS + "/" + testVM

	// Test getting status for non-existent VM
	_, err := client.GetVMStatus(testVM, testNS)
	if err == nil {
		t.Errorf("Expected error when getting status for non-existent VM, but got nil")
	}

	// Add VM status
	client.VMStatuses[vmKey] = "Stopped"

	// Test getting status for existing VM
	status, err := client.GetVMStatus(testVM, testNS)
	if err != nil {
		t.Errorf("Unexpected error when getting VM status: %v", err)
	}
	if status != "Stopped" {
		t.Errorf("Expected VM status to be 'Stopped', but got '%s'", status)
	}

	// Test starting VM
	err = client.StartVM(testVM, testNS)
	if err != nil {
		t.Errorf("Unexpected error when starting VM: %v", err)
	}

	// Verify VM is running
	status, _ = client.GetVMStatus(testVM, testNS)
	if status != "Running" {
		t.Errorf("Expected VM status to be 'Running' after start, but got '%s'", status)
	}

	// Test stopping VM
	err = client.StopVM(testVM, testNS)
	if err != nil {
		t.Errorf("Unexpected error when stopping VM: %v", err)
	}

	// Verify VM is stopped
	status, _ = client.GetVMStatus(testVM, testNS)
	if status != "Stopped" {
		t.Errorf("Expected VM status to be 'Stopped' after stop, but got '%s'", status)
	}

	// Test exporting a VM that doesn't have export data
	_, err = client.ExportVM(testVM, testNS)
	if err == nil {
		t.Errorf("Expected error when exporting VM without export data, but got nil")
	}

	// Add export data
	exportData := []byte("vm-export-data")
	client.ExportedVMs[vmKey] = exportData

	// Test exporting VM
	data, err := client.ExportVM(testVM, testNS)
	if err != nil {
		t.Errorf("Unexpected error when exporting VM: %v", err)
	}
	if string(data) != string(exportData) {
		t.Errorf("Expected export data '%s', but got '%s'", exportData, data)
	}

	// Test waiting for VM status - success case
	err = client.WaitForVMStatus(testVM, testNS, "Stopped", 1*time.Second)
	if err != nil {
		t.Errorf("Unexpected error when waiting for VM status: %v", err)
	}

	// Test waiting for VM status - failure case
	err = client.WaitForVMStatus(testVM, testNS, "Running", 1*time.Second)
	if err == nil {
		t.Errorf("Expected error when waiting for incorrect VM status, but got nil")
	}

	// Test importing VM
	importData := []byte("vm-import-data")
	err = client.ImportVM(importData, testNS)
	if err != nil {
		t.Errorf("Unexpected error when importing VM: %v", err)
	}
	if len(client.ImportedVMs) != 1 || string(client.ImportedVMs[0]) != string(importData) {
		t.Errorf("Import data not correctly stored in mock client")
	}
}

func TestMockKubernetesClient_PodOperations(t *testing.T) {
	// Create a mock client
	client := NewMockKubernetesClient()

	// Set up test pod
	testNS := "test-namespace"
	testPod := "test-pod"
	podKey := testNS + "/" + testPod

	// Test getting status for non-existent pod
	_, err := client.GetPodStatus(testPod, testNS)
	if err == nil {
		t.Errorf("Expected error when getting status for non-existent pod, but got nil")
	}

	// Add pod status
	client.PodStatuses[podKey] = "Running"

	// Test getting status for existing pod
	status, err := client.GetPodStatus(testPod, testNS)
	if err != nil {
		t.Errorf("Unexpected error when getting pod status: %v", err)
	}
	if status != "Running" {
		t.Errorf("Expected pod status to be 'Running', but got '%s'", status)
	}

	// Test waiting for pod condition - success case
	err = client.WaitForPod(testPod, testNS, "Running", 1*time.Second)
	if err != nil {
		t.Errorf("Unexpected error when waiting for pod condition: %v", err)
	}

	// Test waiting for pod condition - failure case
	err = client.WaitForPod(testPod, testNS, "Completed", 1*time.Second)
	if err == nil {
		t.Errorf("Expected error when waiting for incorrect pod condition, but got nil")
	}

	// Test executing command in pod
	cmdKey := testNS + "/" + testPod + ":ls -la"
	expectedOutput := "total 0\ndrwxr-xr-x. 1 root root 0 Jan 1 00:00 ."
	client.ExecutedCommands[cmdKey] = expectedOutput

	output, err := client.ExecInPod(testPod, testNS, "ls -la")
	if err != nil {
		t.Errorf("Unexpected error when executing command in pod: %v", err)
	}
	if output != expectedOutput {
		t.Errorf("Expected command output '%s', but got '%s'", expectedOutput, output)
	}

	// Test executing command that doesn't have a mock response
	_, err = client.ExecInPod(testPod, testNS, "invalid-command")
	if err == nil {
		t.Errorf("Expected error when executing command without mock response, but got nil")
	}
}

func TestMockKubernetesClient_ServiceOperations(t *testing.T) {
	// Create a mock client
	client := NewMockKubernetesClient()

	// Set up test service
	testNS := "test-namespace"
	testSvc := "test-service"
	svcKey := testNS + "/" + testSvc

	// Test getting node port for non-existent service
	_, err := client.GetNodePort(testSvc, testNS)
	if err == nil {
		t.Errorf("Expected error when getting node port for non-existent service, but got nil")
	}

	// Add node port
	expectedPort := 30001
	client.NodePorts[svcKey] = expectedPort

	// Test getting node port for existing service
	port, err := client.GetNodePort(testSvc, testNS)
	if err != nil {
		t.Errorf("Unexpected error when getting node port: %v", err)
	}
	if port != expectedPort {
		t.Errorf("Expected node port to be '%d', but got '%d'", expectedPort, port)
	}

	// Test creating service
	svcDef := []byte("service-definition")
	err = client.CreateService(svcDef, testNS)
	if err != nil {
		t.Errorf("Unexpected error when creating service: %v", err)
	}
	if len(client.CreatedServices) != 1 || string(client.CreatedServices[0]) != string(svcDef) {
		t.Errorf("Service definition not correctly stored in mock client")
	}
}

func TestMockKubernetesClient_ResourceOperations(t *testing.T) {
	// Create a mock client
	client := NewMockKubernetesClient()
	testNS := "test-namespace"

	// Test creating job
	jobDef := []byte("job-definition")
	err := client.CreateJob(jobDef, testNS)
	if err != nil {
		t.Errorf("Unexpected error when creating job: %v", err)
	}
	if len(client.CreatedJobs) != 1 || string(client.CreatedJobs[0]) != string(jobDef) {
		t.Errorf("Job definition not correctly stored in mock client")
	}

	// Test waiting for job - mock always succeeds
	err = client.WaitForJob("test-job", testNS, 1*time.Second)
	if err != nil {
		t.Errorf("Unexpected error when waiting for job: %v", err)
	}

	// Test creating secret
	secretDef := []byte("secret-definition")
	err = client.CreateSecret(secretDef, testNS)
	if err != nil {
		t.Errorf("Unexpected error when creating secret: %v", err)
	}
	if len(client.CreatedSecrets) != 1 || string(client.CreatedSecrets[0]) != string(secretDef) {
		t.Errorf("Secret definition not correctly stored in mock client")
	}

	// Test creating cronjob
	cronJobDef := []byte("cronjob-definition")
	err = client.CreateCronJob(cronJobDef, testNS)
	if err != nil {
		t.Errorf("Unexpected error when creating cronjob: %v", err)
	}
	if len(client.CreatedCronJobs) != 1 || string(client.CreatedCronJobs[0]) != string(cronJobDef) {
		t.Errorf("CronJob definition not correctly stored in mock client")
	}

	// Test suspending cronjob
	err = client.SuspendCronJob("test-cronjob", testNS)
	if err != nil {
		t.Errorf("Unexpected error when suspending cronjob: %v", err)
	}

	// Test cleanup migration resources
	testVM := "test-vm"
	err = client.CleanupMigrationResources(testVM, testNS, true)
	if err != nil {
		t.Errorf("Unexpected error when cleaning up migration resources: %v", err)
	}
	key := testNS + "/" + testVM + ":true"
	if !client.CleanupCalls[key] {
		t.Errorf("Cleanup call not correctly recorded in mock client")
	}
}
