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
	args := []string{
		"get", "vm", vmName,
		"-n", namespace,
		"--kubeconfig", c.kubeconfig,
		"-o", "jsonpath='{.status.printableStatus}'"}

	output, err := c.executor.Execute(c.cmdName, args...)
	if err != nil {
		return "", fmt.Errorf("failed to get VM status: %w", err)
	}

	// Remove surrounding quotes if present
	status := strings.Trim(output, "'")

	if status == "" {
		return "", fmt.Errorf("VM status is empty")
	}

	return status, nil
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
	err := os.WriteFile(tmpFile, vmDef, 0600)
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
	err := os.WriteFile(tmpFile, svcDef, 0600)
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
	err := os.WriteFile(tmpFile, jobDef, 0600)
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
	err := os.WriteFile(tmpFile, secretDef, 0600)
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
	err := os.WriteFile(tmpFile, cronJobDef, 0600)
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

// CleanupMigrationResources cleans up all resources created during migration
func (c *BaseClient) CleanupMigrationResources(vmName, namespace string, isDestination bool) error {
	c.logger.Info("Cleaning up migration resources",
		zap.String("vm", vmName),
		zap.String("namespace", namespace),
		zap.Bool("isDestination", isDestination))

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

	// Resources to clean up in destination cluster
	dstResources := []Resource{
		{"pod", fmt.Sprintf("%s-dst-replicator", vmName)},
		{"svc", fmt.Sprintf("%s-dst-svc", vmName)},
	}

	// Select resources based on whether this is source or destination cluster
	resources := srcResources
	if isDestination {
		resources = dstResources
		c.logger.Info("Identified as destination cluster cleanup")
	} else {
		c.logger.Info("Identified as source cluster cleanup")
	}

	// Delete each resource with idempotent approach
	for _, res := range resources {
		c.logger.Info(fmt.Sprintf("Deleting %s", res.kind), zap.String("name", res.name))

		// First check if resource exists
		checkArgs := []string{"get", res.kind, res.name, "-n", namespace, "--kubeconfig", c.kubeconfig, "--no-headers", "--ignore-not-found"}
		output, _ := c.executor.Execute(c.cmdName, checkArgs...)

		if strings.TrimSpace(output) == "" {
			c.logger.Info(fmt.Sprintf("%s %s not found, skipping delete", res.kind, res.name))
			continue
		}

		// Resource exists, delete it
		_, err := c.executor.Execute(c.cmdName, "delete", res.kind, res.name,
			"-n", namespace, "--kubeconfig", c.kubeconfig, "--wait", "--ignore-not-found")
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
			zap.String("current_status", status),
			zap.String("expected_status", expectedStatus))

		// Wait before next check
		time.Sleep(interval)
	}
}

// GetPodHostIP retrieves the host IP of a pod
func (c *BaseClient) GetPodHostIP(podName, namespace string) (string, error) {
	c.logger.Debug("Getting pod host IP",
		zap.String("pod", podName),
		zap.String("namespace", namespace))

	args := []string{
		"get", "pod", podName,
		"-n", namespace,
		"--kubeconfig", c.kubeconfig,
		"-o", "jsonpath='{.status.hostIP}'",
	}

	output, err := c.executor.Execute(c.cmdName, args...)
	if err != nil {
		return "", fmt.Errorf("failed to get pod host IP: %w", err)
	}

	// Remove surrounding quotes
	hostIP := strings.Trim(output, "'")

	c.logger.Debug("Got pod host IP",
		zap.String("pod", podName),
		zap.String("hostIP", hostIP))

	return hostIP, nil
}

// GetPVCSize retrieves the size of a PVC
func (c *BaseClient) GetPVCSize(pvcName, namespace string) (string, error) {
	args := []string{"get", "pvc", pvcName, "-n", namespace, "--kubeconfig", c.kubeconfig,
		"-o", "jsonpath='{.spec.resources.requests.storage}'"}

	output, err := c.executor.Execute(c.cmdName, args...)
	if err != nil {
		return "", fmt.Errorf("failed to get PVC size: %w", err)
	}

	// Clean the output by removing quotes
	output = strings.Trim(output, "'")
	return output, nil
}

// GetActualDiskUsage retrieves the actual used space for a VM disk
func (c *BaseClient) GetActualDiskUsage(vmName, namespace string) (int64, error) {
	// First, find the pod name with virt-launcher prefix
	podListArgs := []string{
		"get", "pods",
		"-n", namespace,
		"--kubeconfig", c.kubeconfig,
		"--field-selector=status.phase=Running",
		"-o", "name",
	}

	output, err := c.executor.Execute(c.cmdName, podListArgs...)
	if err != nil {
		return 0, fmt.Errorf("failed to list pods: %w", err)
	}

	// Get the pod list and filter by VM name
	pods := strings.Split(strings.TrimSpace(output), "\n")
	var vmPod string
	for _, pod := range pods {
		// Format: pod/virt-launcher-vmname-xxxxx
		podName := strings.TrimPrefix(pod, "pod/")
		// Check if the pod name contains the VM name with launcher prefix
		if strings.Contains(podName, "virt-launcher-"+vmName) {
			vmPod = podName
			break
		}
	}

	if vmPod == "" {
		return 0, fmt.Errorf("no VM pod found for %s", vmName)
	}

	// Measure disk usage - try apparent size with -sh
	diskUsageCmd := "du -sh /run/kubevirt-private/vmi-disks/rootdisk 2>/dev/null || " +
		"du -sh /run/kubevirt-private-vmi-disks/rootdisk 2>/dev/null || " +
		"echo 'Disk not found'"

	usageOutput, err := c.ExecInPod(vmPod, namespace, diskUsageCmd)
	if err != nil || strings.Contains(usageOutput, "Disk not found") {
		c.logger.Warn("Failed to get disk usage from VM pod",
			zap.String("vm", vmName),
			zap.String("pod", vmPod),
			zap.Error(err))
		return 0, fmt.Errorf("disk usage check failed: %w", err)
	}

	// Parse the output - format: "1.6G /path/to/disk"
	parts := strings.Fields(usageOutput)
	if len(parts) < 1 {
		return 0, fmt.Errorf("unexpected disk usage output: %s", usageOutput)
	}

	// Convert human-readable size (e.g., "1.6G") to bytes
	sizeStr := parts[0]
	bytes, err := parseHumanReadableSize(sizeStr)
	if err != nil {
		c.logger.Warn("Failed to parse human readable size, trying block size",
			zap.String("size", sizeStr),
			zap.Error(err))

		// In case of error, try block size (-sb)
		blockSizeCmd := "du -sb /run/kubevirt-private/vmi-disks/rootdisk 2>/dev/null || " +
			"du -sb /run/kubevirt-private-vmi-disks/rootdisk 2>/dev/null"
		blockOutput, blockErr := c.ExecInPod(vmPod, namespace, blockSizeCmd)
		if blockErr != nil {
			return 0, fmt.Errorf("both apparent size and block size checks failed: %w", blockErr)
		}

		// Parse block size output
		blockParts := strings.Fields(blockOutput)
		if len(blockParts) < 1 {
			return 0, fmt.Errorf("unexpected block size output: %s", blockOutput)
		}

		// Convert block size bytes to number
		bytes, err = strconv.ParseInt(blockParts[0], 10, 64)
		if err != nil {
			return 0, fmt.Errorf("failed to parse block size: %w", err)
		}

		// Block size shows full disk size, estimate actual usage as 30%
		bytes = int64(float64(bytes) * 0.3)
		c.logger.Info("Using estimated disk usage from block size",
			zap.Int64("blockSize", bytes),
			zap.Int64("estimatedSize", bytes))
	}

	c.logger.Info("Actual disk usage retrieved",
		zap.String("vm", vmName),
		zap.Int64("bytes", bytes),
		zap.String("humanReadable", formatHumanReadableSize(bytes)))

	return bytes, nil
}

// parseHumanReadableSize converts human-readable sizes like "1.6G" to bytes
func parseHumanReadableSize(size string) (int64, error) {
	size = strings.TrimSpace(size)

	// Separate numeric part and unit
	var numStr string
	var unit string

	i := len(size) - 1
	for i >= 0 {
		if (size[i] >= '0' && size[i] <= '9') || size[i] == '.' {
			break
		}
		i--
	}

	if i < 0 {
		return 0, fmt.Errorf("invalid size format: %s", size)
	}

	numStr = size[:i+1]
	unit = strings.ToUpper(strings.TrimSpace(size[i+1:]))

	// Parse the number
	num, err := strconv.ParseFloat(numStr, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid number in size: %s", numStr)
	}

	// Convert to bytes according to unit
	multiplier := float64(1)
	switch unit {
	case "B", "":
		// multiplier is already 1, no need to assign again
	case "K", "KB", "KIB":
		multiplier = 1024
	case "M", "MB", "MIB":
		multiplier = 1024 * 1024
	case "G", "GB", "GIB":
		multiplier = 1024 * 1024 * 1024
	case "T", "TB", "TIB":
		multiplier = 1024 * 1024 * 1024 * 1024
	default:
		return 0, fmt.Errorf("unknown unit: %s", unit)
	}

	return int64(num * multiplier), nil
}

// formatHumanReadableSize converts bytes to human-readable format
func formatHumanReadableSize(bytes int64) string {
	const (
		KB = 1024
		MB = KB * 1024
		GB = MB * 1024
		TB = GB * 1024
	)

	switch {
	case bytes >= TB:
		return fmt.Sprintf("%.2f TB", float64(bytes)/float64(TB))
	case bytes >= GB:
		return fmt.Sprintf("%.2f GB", float64(bytes)/float64(GB))
	case bytes >= MB:
		return fmt.Sprintf("%.2f MB", float64(bytes)/float64(MB))
	case bytes >= KB:
		return fmt.Sprintf("%.2f KB", float64(bytes)/float64(KB))
	default:
		return fmt.Sprintf("%d bytes", bytes)
	}
}
