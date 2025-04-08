package connectivity

import (
	"fmt"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/kloia/kubevirt-migrator/internal/config"
	"github.com/kloia/kubevirt-migrator/internal/encrypt/ssh"
	"github.com/kloia/kubevirt-migrator/internal/executor"
	"github.com/kloia/kubevirt-migrator/internal/kubernetes"
	"github.com/kloia/kubevirt-migrator/internal/storage/mount"
	"github.com/kloia/kubevirt-migrator/internal/template"
)

// CheckManager handles connectivity and feasibility checks
type CheckManager struct {
	executor      executor.CommandExecutor
	logger        *zap.Logger
	tmplMgr       template.TemplateManager
	sshMgr        ssh.SSHManagerInterface
	srcClient     kubernetes.KubernetesClient
	dstClient     kubernetes.KubernetesClient
	mountProvider mount.MountProvider
	checkResults  map[string]int // Changed to int for three states: success(1), failure(0), not tested(-1)
}

// Check result constants
const (
	CheckFailed    = 0
	CheckSuccess   = 1
	CheckNotTested = -1
)

// NewCheckManager creates a new check manager
func NewCheckManager(executor executor.CommandExecutor, logger *zap.Logger,
	tmplMgr template.TemplateManager, sshMgr ssh.SSHManagerInterface,
	srcClient, dstClient kubernetes.KubernetesClient) *CheckManager {
	return &CheckManager{
		executor:      executor,
		logger:        logger,
		tmplMgr:       tmplMgr,
		sshMgr:        sshMgr,
		srcClient:     srcClient,
		dstClient:     dstClient,
		mountProvider: mount.NewSSHFSProvider(executor, logger),
		checkResults:  make(map[string]int),
	}
}

// SetMountProvider allows injection of a specific mount provider
func (c *CheckManager) SetMountProvider(provider mount.MountProvider) {
	c.mountProvider = provider
}

// GetCheckResults returns the results of the checks performed
func (c *CheckManager) GetCheckResults() map[string]int {
	return c.checkResults
}

// addCheckResult adds a check result to the map
func (c *CheckManager) addCheckResult(checkName string, success bool) {
	if success {
		c.checkResults[checkName] = CheckSuccess
	} else {
		c.checkResults[checkName] = CheckFailed
	}
}

// markRemainingNotTested marks all expected checks that haven't been evaluated yet as NOT_TESTED
func (c *CheckManager) markRemainingNotTested() {
	expectedChecks := []string{
		"Source Pod",
		"Destination Pod",
		"SSH Service",
		"SSH Auth",
		"NodePort",
		"Host IP",
		"TCP Connection",
		"Write Permissions",
		"SSHFS Available",
		"Mount",
		"Mount Verification",
		"Unmount",
	}

	for _, check := range expectedChecks {
		if _, exists := c.checkResults[check]; !exists {
			c.checkResults[check] = CheckNotTested
		}
	}
}

// CheckConnectivity verifies connectivity between source and destination clusters
func (c *CheckManager) CheckConnectivity(cfg *config.Config) error {
	// Reset check results
	c.checkResults = make(map[string]int)

	// First check if VM exists in source cluster
	c.logger.Info("Checking if VM exists in source cluster")
	vmStatus, err := c.srcClient.GetVMStatus(cfg.VMName, cfg.Namespace)
	if err != nil {
		c.logger.Error("Failed to get VM status", zap.Error(err))
		c.markRemainingNotTested()
		return fmt.Errorf("VM %s not found in namespace %s: %w", cfg.VMName, cfg.Namespace, err)
	}
	c.logger.Info("Found VM in source cluster", zap.String("status", vmStatus))

	// Check if VM exists in destination cluster
	c.logger.Info("Checking if VM exists in destination cluster")
	destVMStatus, err := c.dstClient.GetVMStatus(cfg.VMName, cfg.Namespace)
	if err != nil {
		// VM doesn't exist in destination, let's create it
		c.logger.Info("VM not found in destination cluster, creating it in halted state")

		// Export VM from source cluster
		vmDef, err := c.srcClient.ExportVM(cfg.VMName, cfg.Namespace)
		if err != nil {
			c.logger.Error("Failed to export VM from source cluster", zap.Error(err))
			c.markRemainingNotTested()
			return fmt.Errorf("failed to export VM from source cluster: %w", err)
		}

		// Import VM to destination cluster
		err = c.dstClient.ImportVM(vmDef, cfg.Namespace)
		if err != nil {
			c.logger.Error("Failed to import VM to destination cluster", zap.Error(err))
			c.markRemainingNotTested()
			return fmt.Errorf("failed to import VM to destination cluster: %w", err)
		}

		c.logger.Info("Successfully imported VM to destination cluster")
	} else {
		c.logger.Info("VM already exists in destination cluster", zap.String("status", destVMStatus))
	}

	// Ensure VM is in halted state regardless of whether it was just created or already existed
	if destVMStatus != "Stopped" {
		c.logger.Info("Ensuring VM is in halted state")
		err = c.dstClient.StopVM(cfg.VMName, cfg.Namespace)
		if err != nil {
			c.logger.Error("Failed to stop VM in destination cluster", zap.Error(err))
			c.markRemainingNotTested()
			return fmt.Errorf("failed to stop VM in destination cluster: %w", err)
		}
	}

	// Wait for VM to actually reach Stopped state
	c.logger.Info("Waiting for VM to reach Stopped state in destination cluster")
	err = c.dstClient.WaitForVMStatus(cfg.VMName, cfg.Namespace, "Stopped", 60*time.Second)
	if err != nil {
		c.logger.Error("VM failed to reach Stopped state in destination cluster", zap.Error(err))
		c.markRemainingNotTested()
		return fmt.Errorf("VM failed to reach Stopped state in destination cluster: %w", err)
	}

	c.logger.Info("Confirmed VM is in halted state in destination cluster")

	c.logger.Info("Setting up test replicator pods")

	// Create replicator pods for testing
	err = c.setupTestReplicators(cfg)
	c.addCheckResult("Source Pod", err == nil)
	c.addCheckResult("Destination Pod", err == nil)
	c.addCheckResult("SSH Service", err == nil)
	if err != nil {
		c.markRemainingNotTested()
		return fmt.Errorf("failed to setup test replicators: %w", err)
	}

	// Ensure cleanup happens
	defer func() {
		c.logger.Info("Cleaning up test artifacts")
		if cleanupErr := c.cleanupTestReplicators(cfg); cleanupErr != nil {
			c.logger.Error("Failed to clean up test replicators",
				zap.Error(cleanupErr))
		}
	}()

	// Generate SSH Keys and setup auth
	c.logger.Info("Setting up SSH authentication")
	err = c.sshMgr.GenerateKeys(cfg)
	if err != nil {
		c.addCheckResult("SSH Auth", false)
		c.markRemainingNotTested()
		return fmt.Errorf("failed to generate SSH keys: %w", err)
	}

	// Setup destination authentication
	err = c.sshMgr.SetupDestinationAuth(cfg)
	c.addCheckResult("SSH Auth", err == nil)
	if err != nil {
		c.markRemainingNotTested()
		return fmt.Errorf("failed to setup destination auth: %w", err)
	}

	// Get destination pod information
	c.logger.Info("Getting destination pod information")
	nodePort, err := c.dstClient.GetNodePort(fmt.Sprintf("%s-dst-svc", cfg.VMName), cfg.Namespace)
	c.addCheckResult("NodePort", err == nil)
	if err != nil {
		c.markRemainingNotTested()
		return fmt.Errorf("failed to get destination NodePort: %w", err)
	}

	nodePortStr := fmt.Sprintf("%d", nodePort)

	// Get host IP of the destination pod
	hostIP, err := c.dstClient.GetPodHostIP(fmt.Sprintf("%s-dst-replicator", cfg.VMName), cfg.Namespace)
	c.addCheckResult("Host IP", err == nil)
	if err != nil {
		c.markRemainingNotTested()
		return fmt.Errorf("failed to get destination pod host IP: %w", err)
	}

	// Perform connectivity check (this contains multiple sub-checks)
	c.logger.Info("Testing network connectivity",
		zap.String("hostIP", hostIP),
		zap.String("port", nodePortStr))

	err = c.mountProvider.CheckConnectivity(cfg, hostIP, nodePortStr)

	if err != nil {
		// Extract specific check failures based on error message
		tcpConnected := !strings.Contains(err.Error(), "TCP connectivity test failed")
		srcDirWritable := !strings.Contains(err.Error(), "Source directory write permission")
		sshfsAvailable := !strings.Contains(err.Error(), "SSHFS command not available")

		c.addCheckResult("TCP Connection", tcpConnected)
		c.addCheckResult("Write Permissions", srcDirWritable)
		c.addCheckResult("SSHFS Available", sshfsAvailable)

		// Mark remaining checks as not tested since we can't proceed
		c.markRemainingNotTested()
		return fmt.Errorf("connectivity check failed: %w", err)
	}

	// All connectivity sub-checks passed
	c.addCheckResult("TCP Connection", true)
	c.addCheckResult("Write Permissions", true)
	c.addCheckResult("SSHFS Available", true)

	// Test mounting
	c.logger.Info("Testing mount functionality")
	err = c.mountProvider.Mount(cfg, hostIP, nodePortStr)
	c.addCheckResult("Mount", err == nil)
	if err != nil {
		c.markRemainingNotTested()
		return fmt.Errorf("mount test failed: %w", err)
	}

	// Verify mount
	err = c.mountProvider.VerifyMount(cfg)
	c.addCheckResult("Mount Verification", err == nil)
	if err != nil {
		// Try to unmount even if verification failed
		if unmountErr := c.mountProvider.Unmount(cfg); unmountErr != nil {
			c.logger.Warn("Failed to unmount after verification failure",
				zap.Error(unmountErr))
		}
		c.markRemainingNotTested()
		return fmt.Errorf("mount verification failed: %w", err)
	}

	// Unmount
	err = c.mountProvider.Unmount(cfg)
	c.addCheckResult("Unmount", err == nil)
	if err != nil {
		c.logger.Warn("Failed to unmount after test",
			zap.Error(err))
		// Continue despite unmount failure - this is not fatal
	}

	c.logger.Info("All connectivity checks completed successfully",
		zap.String("hostIP", hostIP),
		zap.String("nodePort", nodePortStr))
	return nil
}

// setupTestReplicators creates temporary source and destination replicator pods
func (c *CheckManager) setupTestReplicators(cfg *config.Config) error {
	c.logger.Info("Setting up test replicator pods")

	// Create source replicator pod
	err := c.tmplMgr.RenderAndApply(template.SourceReplicator, template.TemplateVariables{
		VMName:    cfg.VMName,
		Namespace: cfg.Namespace,
		SyncTool:  cfg.SyncTool,
	}, cfg.SrcKubeconfig)
	if err != nil {
		return fmt.Errorf("failed to create source replicator: %w", err)
	}

	// Create destination replicator pod
	err = c.tmplMgr.RenderAndApply(template.DestReplicator, template.TemplateVariables{
		VMName:    cfg.VMName,
		Namespace: cfg.Namespace,
	}, cfg.DstKubeconfig)
	if err != nil {
		return fmt.Errorf("failed to create destination replicator: %w", err)
	}

	// Create destination service
	err = c.tmplMgr.RenderAndApply(template.DestService, template.TemplateVariables{
		VMName:     cfg.VMName,
		Namespace:  cfg.Namespace,
		Port:       cfg.SSHPort,
		TargetPort: cfg.SSHPort,
	}, cfg.DstKubeconfig)
	if err != nil {
		return fmt.Errorf("failed to create destination service: %w", err)
	}

	c.logger.Info("Waiting for replicator pods to be ready")

	// Wait for source replicator to be ready using the client
	err = c.srcClient.WaitForPod(
		fmt.Sprintf("%s-src-replicator", cfg.VMName),
		cfg.Namespace,
		"ready",
		60*time.Second,
	)
	if err != nil {
		return fmt.Errorf("timeout waiting for source replicator: %w", err)
	}

	// Wait for destination replicator to be ready using the client
	err = c.dstClient.WaitForPod(
		fmt.Sprintf("%s-dst-replicator", cfg.VMName),
		cfg.Namespace,
		"ready",
		60*time.Second,
	)
	if err != nil {
		return fmt.Errorf("timeout waiting for destination replicator: %w", err)
	}

	c.logger.Info("Test replicator pods are ready")
	return nil
}

// cleanupTestReplicators removes the temporary replicator pods
func (c *CheckManager) cleanupTestReplicators(cfg *config.Config) error {
	c.logger.Info("Cleaning up test replicator pods")

	// Use the CleanupMigrationResources from each client to handle cleanup
	if err := c.srcClient.CleanupMigrationResources(cfg.VMName, cfg.Namespace, false); err != nil {
		c.logger.Warn("Error cleaning up source resources", zap.Error(err))
		return err
	}

	if err := c.dstClient.CleanupMigrationResources(cfg.VMName, cfg.Namespace, true); err != nil {
		c.logger.Warn("Error cleaning up destination resources", zap.Error(err))
		return err
	}

	c.logger.Info("Test replicator pods cleaned up")
	return nil
}
