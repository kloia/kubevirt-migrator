package kubernetes

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/kloia/kubevirt-migrator/internal/executor"
	"github.com/kloia/kubevirt-migrator/internal/sync"
)

// BaseClient provides common implementation for Kubernetes clients
type BaseClient struct {
	executor   executor.CommandExecutor
	kubeconfig string
	syncTool   sync.SyncCommand
	logger     *zap.Logger
	cmdName    string // "oc" or "kubectl"
}

// NewBaseClient creates a new BaseClient instance
func NewBaseClient(cmdName, kubeconfig string, executor executor.CommandExecutor, syncTool sync.SyncCommand, logger *zap.Logger) *BaseClient {
	return &BaseClient{
		executor:   executor,
		kubeconfig: kubeconfig,
		syncTool:   syncTool,
		logger:     logger,
		cmdName:    cmdName,
	}
}

// GetVMStatus returns the status of a virtual machine
func (c *BaseClient) GetVMStatus(vmName, namespace string) (string, error) {
	args := []string{"get", "vm", vmName, "-n", namespace, "--kubeconfig", c.kubeconfig, "--no-headers"}

	output, err := c.executor.Execute(c.cmdName, args...)
	if err != nil {
		return "", fmt.Errorf("failed to get VM status: %w", err)
	}

	parts := strings.Fields(output)
	if len(parts) < 3 {
		return "", fmt.Errorf("unexpected output format: %s", output)
	}

	return parts[2], nil
}

// StartVM starts a virtual machine
func (c *BaseClient) StartVM(vmName, namespace string) error {
	// First try to check if runStrategy exists
	checkArgs := []string{"get", "vm", vmName, "-n", namespace, "--kubeconfig", c.kubeconfig,
		"-o", "jsonpath='{.spec.runStrategy}'"}

	runStrategyOutput, err := c.executor.Execute(c.cmdName, checkArgs...)
	runStrategyExists := err == nil && runStrategyOutput != "" && runStrategyOutput != "''" && runStrategyOutput != "'null'"

	var args []string
	if runStrategyExists {
		// Use runStrategy if it exists
		args = []string{"patch", "vm", vmName, "-n", namespace, "--kubeconfig", c.kubeconfig,
			"--type", "merge", "-p", `{"spec":{"runStrategy":"Always"}}`}
		c.logger.Info("Using runStrategy=Always to start VM", zap.String("vm", vmName))
	} else {
		// Use running field if runStrategy doesn't exist
		args = []string{"patch", "vm", vmName, "-n", namespace, "--kubeconfig", c.kubeconfig,
			"--type", "merge", "-p", `{"spec":{"running":true}}`}
		c.logger.Info("Using running=true to start VM", zap.String("vm", vmName))
	}

	_, err = c.executor.Execute(c.cmdName, args...)
	if err != nil {
		return fmt.Errorf("failed to start VM: %w", err)
	}

	return nil
}

// StopVM stops a virtual machine
func (c *BaseClient) StopVM(vmName, namespace string) error {
	// First try to check if runStrategy exists
	checkArgs := []string{"get", "vm", vmName, "-n", namespace, "--kubeconfig", c.kubeconfig,
		"-o", "jsonpath='{.spec.runStrategy}'"}

	runStrategyOutput, err := c.executor.Execute(c.cmdName, checkArgs...)
	runStrategyExists := err == nil && runStrategyOutput != "" && runStrategyOutput != "''" && runStrategyOutput != "'null'"

	var args []string
	if runStrategyExists {
		// Use runStrategy if it exists
		args = []string{"patch", "vm", vmName, "-n", namespace, "--kubeconfig", c.kubeconfig,
			"--type", "merge", "-p", `{"spec":{"runStrategy":"Halted"}}`}
		c.logger.Info("Using runStrategy=Halted to stop VM", zap.String("vm", vmName))
	} else {
		// Use running field if runStrategy doesn't exist
		args = []string{"patch", "vm", vmName, "-n", namespace, "--kubeconfig", c.kubeconfig,
			"--type", "merge", "-p", `{"spec":{"running":false}}`}
		c.logger.Info("Using running=false to stop VM", zap.String("vm", vmName))
	}

	_, err = c.executor.Execute(c.cmdName, args...)
	if err != nil {
		return fmt.Errorf("failed to stop VM: %w", err)
	}

	return nil
}

// ExportVM exports a virtual machine definition to YAML
func (c *BaseClient) ExportVM(vmName, namespace string) ([]byte, error) {
	args := []string{"get", "vm", vmName, "-n", namespace, "--kubeconfig", c.kubeconfig, "-o", "yaml"}

	output, err := c.executor.Execute(c.cmdName, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to export VM: %w", err)
	}

	return []byte(output), nil
}

// ImportVM imports a virtual machine definition from YAML
func (c *BaseClient) ImportVM(vmDef []byte, namespace string) error {
	tmpFile := "/tmp/vm-import.yaml"
	args := []string{"apply", "-n", namespace, "--kubeconfig", c.kubeconfig, "-f", tmpFile}

	// Write VM definition to temporary file using Go's file operations
	err := os.WriteFile(tmpFile, vmDef, 0644)
	if err != nil {
		return fmt.Errorf("failed to write VM definition to file: %w", err)
	}

	// Apply the VM definition
	_, err = c.executor.Execute(c.cmdName, args...)
	if err != nil {
		return fmt.Errorf("failed to import VM: %w", err)
	}

	return nil
}

// GetPodStatus returns the status of a pod
func (c *BaseClient) GetPodStatus(podName, namespace string) (string, error) {
	args := []string{"get", "pod", podName, "-n", namespace, "--kubeconfig", c.kubeconfig, "--no-headers"}

	output, err := c.executor.Execute(c.cmdName, args...)
	if err != nil {
		return "", fmt.Errorf("failed to get pod status: %w", err)
	}

	parts := strings.Fields(output)
	if len(parts) < 3 {
		return "", fmt.Errorf("unexpected output format: %s", output)
	}

	return parts[2], nil
}

// WaitForPod waits for a pod to reach the specified condition
func (c *BaseClient) WaitForPod(podName, namespace, condition string, timeout time.Duration) error {
	timeoutStr := fmt.Sprintf("%dm", int(timeout.Minutes()))
	args := []string{"wait", "pod", podName, "-n", namespace, "--kubeconfig", c.kubeconfig,
		"--for", "condition=" + condition, "--timeout", timeoutStr}

	_, err := c.executor.Execute(c.cmdName, args...)
	if err != nil {
		return fmt.Errorf("failed waiting for pod to reach condition %s: %w", condition, err)
	}

	return nil
}

// ExecInPod executes a command in a pod
func (c *BaseClient) ExecInPod(podName, namespace, command string) (string, error) {
	args := []string{"exec", podName, "-n", namespace, "--kubeconfig", c.kubeconfig, "--", "sh", "-c", command}

	output, err := c.executor.Execute(c.cmdName, args...)
	if err != nil {
		return "", fmt.Errorf("failed to execute command in pod: %w", err)
	}

	return output, nil
}

// CreateService creates a service from the provided definition
func (c *BaseClient) CreateService(svcDef []byte, namespace string) error {
	tmpFile := "/tmp/svc-create.yaml"
	args := []string{"apply", "-n", namespace, "--kubeconfig", c.kubeconfig, "-f", tmpFile}

	// Write service definition to temporary file using Go's file operations
	err := os.WriteFile(tmpFile, svcDef, 0644)
	if err != nil {
		return fmt.Errorf("failed to write service definition to file: %w", err)
	}

	// Apply the service definition
	_, err = c.executor.Execute(c.cmdName, args...)
	if err != nil {
		return fmt.Errorf("failed to create service: %w", err)
	}

	return nil
}

// GetNodePort returns the node port for a service
func (c *BaseClient) GetNodePort(svcName, namespace string) (int, error) {
	args := []string{"get", "svc", svcName, "-n", namespace, "--kubeconfig", c.kubeconfig,
		"-o", "jsonpath='{.spec.ports[0].nodePort}'"}

	output, err := c.executor.Execute(c.cmdName, args...)
	if err != nil {
		return 0, fmt.Errorf("failed to get node port: %w", err)
	}

	// Remove single quotes from the output
	portStr := strings.Trim(output, "'")

	// Convert to integer
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return 0, fmt.Errorf("failed to parse node port: %w", err)
	}

	return port, nil
}

// CreateJob creates a job from the provided definition
func (c *BaseClient) CreateJob(jobDef []byte, namespace string) error {
	tmpFile := "/tmp/job-create.yaml"
	args := []string{"apply", "-n", namespace, "--kubeconfig", c.kubeconfig, "-f", tmpFile}

	// Write job definition to temporary file using Go's file operations
	err := os.WriteFile(tmpFile, jobDef, 0644)
	if err != nil {
		return fmt.Errorf("failed to write job definition to file: %w", err)
	}

	// Apply the job definition
	_, err = c.executor.Execute(c.cmdName, args...)
	if err != nil {
		return fmt.Errorf("failed to create job: %w", err)
	}

	return nil
}

// WaitForJob waits for a job to complete
func (c *BaseClient) WaitForJob(jobName, namespace string, timeout time.Duration) error {
	timeoutStr := fmt.Sprintf("%dm", int(timeout.Minutes()))
	args := []string{"wait", "job", jobName, "-n", namespace, "--kubeconfig", c.kubeconfig,
		"--for=condition=complete", "--timeout", timeoutStr}

	_, err := c.executor.Execute(c.cmdName, args...)
	if err != nil {
		return fmt.Errorf("failed waiting for job to complete: %w", err)
	}

	return nil
}

// CreateSecret creates a secret from the provided definition
func (c *BaseClient) CreateSecret(secretDef []byte, namespace string) error {
	tmpFile := "/tmp/secret-create.yaml"
	args := []string{"apply", "-n", namespace, "--kubeconfig", c.kubeconfig, "-f", tmpFile}

	// Write secret definition to temporary file using Go's file operations
	err := os.WriteFile(tmpFile, secretDef, 0644)
	if err != nil {
		return fmt.Errorf("failed to write secret definition to file: %w", err)
	}

	// Apply the secret definition
	_, err = c.executor.Execute(c.cmdName, args...)
	if err != nil {
		return fmt.Errorf("failed to create secret: %w", err)
	}

	return nil
}

// CreateCronJob creates a cronjob from the provided definition
func (c *BaseClient) CreateCronJob(cronJobDef []byte, namespace string) error {
	tmpFile := "/tmp/cronjob-create.yaml"
	args := []string{"apply", "-n", namespace, "--kubeconfig", c.kubeconfig, "-f", tmpFile}

	// Write cronjob definition to temporary file using Go's file operations
	err := os.WriteFile(tmpFile, cronJobDef, 0644)
	if err != nil {
		return fmt.Errorf("failed to write cronjob definition to file: %w", err)
	}

	// Apply the cronjob definition
	_, err = c.executor.Execute(c.cmdName, args...)
	if err != nil {
		return fmt.Errorf("failed to create cronjob: %w", err)
	}

	return nil
}

// SuspendCronJob suspends a cronjob
func (c *BaseClient) SuspendCronJob(cronJobName, namespace string) error {
	args := []string{"patch", "cronjob", cronJobName, "-n", namespace, "--kubeconfig", c.kubeconfig,
		"--type", "merge", "-p", `{"spec":{"suspend":true}}`}

	_, err := c.executor.Execute(c.cmdName, args...)
	if err != nil {
		return fmt.Errorf("failed to suspend cronjob: %w", err)
	}

	return nil
}

// ExportVMWithPreservedIP exports a virtual machine definition to YAML with IP preservation
func (c *BaseClient) ExportVMWithPreservedIP(vmName, namespace string) ([]byte, error) {
	// First export the VM definition
	vmDef, err := c.ExportVM(vmName, namespace)
	if err != nil {
		return nil, fmt.Errorf("failed to export VM for IP preservation: %w", err)
	}

	c.logger.Info("Preserving pod IP address for VM", zap.String("vm", vmName))

	// Get VMI to extract network information
	podIP, err := c.executor.Execute(c.cmdName, "get", "vmi", vmName, "-n", namespace,
		"--kubeconfig", c.kubeconfig, "-o=jsonpath='{.status.interfaces[0].ipAddress}'")
	if err != nil {
		return nil, fmt.Errorf("failed to get pod IP: %w", err)
	}
	podIP = strings.Trim(podIP, "'") + "/23"

	podMAC, err := c.executor.Execute(c.cmdName, "get", "vmi", vmName, "-n", namespace,
		"--kubeconfig", c.kubeconfig, "-o=jsonpath='{.status.interfaces[0].mac}'")
	if err != nil {
		return nil, fmt.Errorf("failed to get pod MAC: %w", err)
	}
	podMAC = strings.Trim(podMAC, "'")

	// Create a temporary file with VM definition
	tmpFile := "/tmp/vm-ip-preserve.yaml"
	if err := os.WriteFile(tmpFile, vmDef, 0600); err != nil {
		return nil, fmt.Errorf("failed to write VM definition to file: %w", err)
	}

	// Build the annotation JSON
	ipAnnotation := fmt.Sprintf(`'{"default":{"ip_address":"%s","mac_address":"%s"}}'`, podIP, podMAC)

	// Use yq to update the YAML with the annotation
	_, err = c.executor.Execute("yq", "e", "-i",
		fmt.Sprintf(`.spec.template.metadata.annotations["k8s.ovn.org/pod-networks"] = %s`, ipAnnotation),
		tmpFile)
	if err != nil {
		if err := os.Remove(tmpFile); err != nil {
			c.logger.Warn("Failed to remove temp file", zap.String("file", tmpFile), zap.Error(err))
		}
		return nil, fmt.Errorf("failed to update VM definition with IP annotation: %w", err)
	}

	// Check if runStrategy exists
	runStrategyOutput, err := c.executor.Execute("yq", "e", ".spec.runStrategy", tmpFile)
	runStrategyExists := err == nil && runStrategyOutput != "" && runStrategyOutput != "null"

	// Set VM to stopped state based on what's available
	if runStrategyExists {
		// Use runStrategy if it exists
		_, err = c.executor.Execute("yq", "e", "-i", `.spec.runStrategy = "Halted"`, tmpFile)
		if err != nil {
			if err := os.Remove(tmpFile); err != nil {
				c.logger.Warn("Failed to remove temp file", zap.String("file", tmpFile), zap.Error(err))
			}
			return nil, fmt.Errorf("failed to update VM definition to stopped state: %w", err)
		}
		c.logger.Info("Using runStrategy=Halted to stop VM")
	} else {
		// Use running field if runStrategy doesn't exist
		_, err = c.executor.Execute("yq", "e", "-i", ".spec.running = false", tmpFile)
		if err != nil {
			if err := os.Remove(tmpFile); err != nil {
				c.logger.Warn("Failed to remove temp file", zap.String("file", tmpFile), zap.Error(err))
			}
			return nil, fmt.Errorf("failed to update VM definition to stopped state: %w", err)
		}
		c.logger.Info("Using running=false to stop VM")
	}

	// Read back the modified definition
	modifiedVMDef, err := os.ReadFile(tmpFile)
	if err != nil {
		if err := os.Remove(tmpFile); err != nil {
			c.logger.Warn("Failed to remove temp file", zap.String("file", tmpFile), zap.Error(err))
		}
		return nil, fmt.Errorf("failed to read modified VM definition: %w", err)
	}

	// Clean up temp file
	if err := os.Remove(tmpFile); err != nil {
		c.logger.Warn("Failed to remove temp file", zap.String("file", tmpFile), zap.Error(err))
	}

	return modifiedVMDef, nil
}

// CleanupMigrationResources cleans up all resources created during migration
func (c *BaseClient) CleanupMigrationResources(vmName, namespace string) error {
	c.logger.Info("Cleaning up migration resources", zap.String("vm", vmName), zap.String("namespace", namespace))
	var errs []error

	// Define resource types to clean up
	type Resource struct {
		kind string
		name string
	}

	// Resources to clean up in source cluster
	srcResources := []Resource{
		{"job", fmt.Sprintf("%s-repl-final-job", vmName)},
		{"cronjob", fmt.Sprintf("%s-repl-cronjob", vmName)},
		{"pod", fmt.Sprintf("%s-src-replicator", vmName)},
		{"secret", fmt.Sprintf("%s-repl-ssh-keys", vmName)},
	}

	// Resources to clean up in destination cluster (handled by caller for different kubeconfig)
	dstResources := []Resource{
		{"pod", fmt.Sprintf("%s-dst-replicator", vmName)},
		{"svc", fmt.Sprintf("%s-dst-svc", vmName)},
	}

	// Cleanup source or destination resources based on current client
	resources := srcResources
	if strings.Contains(c.kubeconfig, "dst-kubeconfig") {
		resources = dstResources
	}

	// Delete each resource
	for _, res := range resources {
		c.logger.Info(fmt.Sprintf("Deleting %s", res.kind), zap.String("name", res.name))
		_, err := c.executor.Execute(c.cmdName, "delete", res.kind, res.name,
			"-n", namespace, "--kubeconfig", c.kubeconfig, "--wait")
		if err != nil {
			errs = append(errs, fmt.Errorf("failed to delete %s %s: %w", res.kind, res.name, err))
			c.logger.Warn(fmt.Sprintf("Failed to delete %s", res.kind),
				zap.String("name", res.name), zap.Error(err))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("one or more cleanup operations failed")
	}

	c.logger.Info("Migration resources cleaned up successfully")
	return nil
}

// WaitForVMStatus waits for a VM to reach the specified status
func (c *BaseClient) WaitForVMStatus(vmName, namespace, expectedStatus string, timeout time.Duration) error {
	c.logger.Info("Waiting for VM to reach status",
		zap.String("vm", vmName),
		zap.String("namespace", namespace),
		zap.String("expected_status", expectedStatus),
		zap.Duration("timeout", timeout))

	// Convert timeout to seconds for the deadline calculation
	deadline := time.Now().Add(timeout)

	// Poll interval
	interval := 5 * time.Second

	for {
		// Check if we've exceeded the timeout
		if time.Now().After(deadline) {
			return fmt.Errorf("timeout waiting for VM %s to reach status %s after %v",
				vmName, expectedStatus, timeout)
		}

		// Get current VM status
		status, err := c.GetVMStatus(vmName, namespace)
		if err != nil {
			c.logger.Warn("Error getting VM status",
				zap.String("vm", vmName),
				zap.Error(err))

			// If we can't get the status, wait a bit and try again
			time.Sleep(interval)
			continue
		}

		// Check if we've reached the expected status
		if status == expectedStatus {
			c.logger.Info("VM reached expected status",
				zap.String("vm", vmName),
				zap.String("status", expectedStatus))
			return nil
		}

		c.logger.Info("VM status check",
			zap.String("vm", vmName),
			zap.String("current", status),
			zap.String("expected", expectedStatus))

		// Wait before next check
		time.Sleep(interval)
	}
}
