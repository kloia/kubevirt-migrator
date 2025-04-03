package replication

import (
	"fmt"
	"strings"

	"go.uber.org/zap"

	"github.com/kloia/kubevirt-migrator/internal/config"
	"github.com/kloia/kubevirt-migrator/internal/executor"
	"github.com/kloia/kubevirt-migrator/internal/sync"
	"github.com/kloia/kubevirt-migrator/internal/template"
)

// SyncManager handles replication and synchronization
type SyncManager struct {
	executor executor.CommandExecutor
	logger   *zap.Logger
	sshMgr   *SSHManager
	tmplMgr  *template.Manager
	syncTool sync.SyncCommand
}

// NewSyncManager creates a new synchronization manager
func NewSyncManager(executor executor.CommandExecutor, logger *zap.Logger,
	sshMgr *SSHManager, tmplMgr *template.Manager) *SyncManager {
	return &SyncManager{
		executor: executor,
		logger:   logger,
		sshMgr:   sshMgr,
		tmplMgr:  tmplMgr,
	}
}

// SetSyncTool sets the synchronization tool
func (s *SyncManager) SetSyncTool(syncTool sync.SyncCommand) {
	s.syncTool = syncTool
}

// GetDestinationInfo returns NodePort and HostIP for the destination replicator
func (s *SyncManager) GetDestinationInfo(cfg *config.Config) (nodePort, hostIP string, err error) {
	// Get NodePort
	nodePort, err = s.executor.Execute(cfg.KubeCLI, "get", "svc", fmt.Sprintf("%s-dst-svc", cfg.VMName),
		"-n", cfg.Namespace, "--kubeconfig", cfg.DstKubeconfig,
		"-o=jsonpath='{.spec.ports[0].nodePort}'")
	if err != nil {
		return "", "", fmt.Errorf("failed to get destination NodePort: %w", err)
	}
	nodePort = strings.Trim(nodePort, "'")

	// Get Host IP
	hostIP, err = s.executor.Execute(cfg.KubeCLI, "get", "pod", fmt.Sprintf("%s-dst-replicator", cfg.VMName),
		"-n", cfg.Namespace, "--kubeconfig", cfg.DstKubeconfig,
		"-o=jsonpath='{.status.hostIP}'")
	if err != nil {
		return "", "", fmt.Errorf("failed to get destination Host IP: %w", err)
	}
	hostIP = strings.Trim(hostIP, "'")

	return nodePort, hostIP, nil
}

// PerformInitialSync performs the initial synchronization
func (s *SyncManager) PerformInitialSync(cfg *config.Config) error {
	s.logger.Info("Getting destination information for initial sync")

	// Get NodePort and Host IP
	nodePort, hostIP, err := s.GetDestinationInfo(cfg)
	if err != nil {
		return err
	}

	s.logger.Info("Starting initial volume replication",
		zap.String("hostIP", hostIP),
		zap.String("nodePort", nodePort),
		zap.String("syncTool", cfg.SyncTool))

	// Execute initial sync (initial sync is still using cp command regardless of sync tool)
	syncCmd := fmt.Sprintf("mkdir -p /data/dimg; sshfs -o StrictHostKeyChecking=no -o port=%s %s:/data/simg /data/dimg; "+
		"cp -p --sparse=always /data/simg/disk.img /data/dimg/ & progress -m", nodePort, hostIP)

	_, err = s.executor.Execute(cfg.KubeCLI, "exec", fmt.Sprintf("%s-src-replicator", cfg.VMName),
		"-n", cfg.Namespace, "--kubeconfig", cfg.SrcKubeconfig,
		"--", "bash", "-c", syncCmd)
	if err != nil {
		return fmt.Errorf("failed to perform initial sync: %w", err)
	}

	return nil
}

// CreateSyncCommand generates a synchronization command based on the sync tool
func (s *SyncManager) CreateSyncCommand(nodePort, hostIP string, cfg *config.Config) (string, error) {
	if s.syncTool == nil {
		// Fallback to default rclone command if syncTool is not set
		cmd := fmt.Sprintf("mkdir -p /data/dimg /data/dfs /data/sfs/; "+
			"sshfs -o StrictHostKeyChecking=no -o port=%s %s:/data/simg /data/dimg; "+
			"guestmount -a /data/simg/disk.img -m /dev/sda4 --ro /data/sfs; "+
			"guestmount -a /data/dimg/disk.img -m /dev/sda4 --rw /data/dfs; "+
			"rclone sync --progress /data/sfs/ /data/dfs/ --skip-links --checkers 8 "+
			"--contimeout 100s --timeout 300s --retries 3 --low-level-retries 10 "+
			"--drive-acknowledge-abuse --stats 1s --cutoff-mode=soft; sleep 20",
			nodePort, hostIP)

		s.logger.Info("Created default rclone replication command (fallback)")
		return cmd, nil
	}

	// Get sync command from the sync tool implementation
	syncToolName, syncArgs := s.syncTool.GenerateSyncCommand("/data/sfs/", "/data/dfs/", map[string]string{
		"checksum": "true",
		"checkers": "8",
	})

	// Create the full replication command
	cmd := fmt.Sprintf("mkdir -p /data/dimg /data/dfs /data/sfs/; "+
		"sshfs -o StrictHostKeyChecking=no -o port=%s %s:/data/simg /data/dimg; "+
		"guestmount -a /data/simg/disk.img -m /dev/sda4 --ro /data/sfs; "+
		"guestmount -a /data/dimg/disk.img -m /dev/sda4 --rw /data/dfs; "+
		"%s %s; sleep 20",
		nodePort, hostIP, syncToolName, strings.Join(syncArgs, " "))

	s.logger.Info("Created replication command with sync tool",
		zap.String("syncTool", s.syncTool.GetToolName()))

	return cmd, nil
}

// SetupCronJob sets up the asynchronous replication cronjob
func (s *SyncManager) SetupCronJob(cfg *config.Config) error {
	s.logger.Info("Setting up replication cronjob",
		zap.String("vm", cfg.VMName),
		zap.String("kubeCLI", cfg.KubeCLI),
		zap.String("syncTool", cfg.SyncTool))

	// Get NodePort and Host IP
	nodePort, hostIP, err := s.GetDestinationInfo(cfg)
	if err != nil {
		return err
	}

	// Create replication command
	replicationCmd, err := s.CreateSyncCommand(nodePort, hostIP, cfg)
	if err != nil {
		return fmt.Errorf("failed to create replication command: %w", err)
	}

	// Create CronJob using template
	err = s.tmplMgr.RenderAndApply(template.ReplicationJob, template.TemplateVariables{
		VMName:             cfg.VMName,
		Namespace:          cfg.Namespace,
		Schedule:           "*/15 * * * *", // Every 15 minutes
		ReplicationCommand: replicationCmd,
		SyncTool:           cfg.SyncTool,
	}, cfg.SrcKubeconfig)
	if err != nil {
		return fmt.Errorf("failed to create cronjob: %w", err)
	}

	s.logger.Info("Replication cronjob created successfully")
	return nil
}

// PerformFinalSync performs the final synchronization
func (s *SyncManager) PerformFinalSync(cfg *config.Config) error {
	s.logger.Info("Creating final replication job")

	// Create final job
	_, err := s.executor.Execute(cfg.KubeCLI, "create", "job", "--from=cronjob/"+fmt.Sprintf("%s-repl-cronjob", cfg.VMName),
		fmt.Sprintf("%s-repl-final-job", cfg.VMName), "-n", cfg.Namespace, "--kubeconfig", cfg.SrcKubeconfig)
	if err != nil {
		return fmt.Errorf("failed to create final job: %w", err)
	}

	s.logger.Info("Waiting for final replication to complete")

	// Wait for job completion
	_, err = s.executor.Execute(cfg.KubeCLI, "wait", "job", fmt.Sprintf("%s-repl-final-job", cfg.VMName),
		"-n", cfg.Namespace, "--kubeconfig", cfg.SrcKubeconfig,
		"--for=condition=complete", "--timeout=-1m")
	if err != nil {
		return fmt.Errorf("failed waiting for final job: %w", err)
	}

	return nil
}

// SuspendCronJob suspends the replication CronJob
func (s *SyncManager) SuspendCronJob(cfg *config.Config) error {
	s.logger.Info("Suspending CronJob")

	// Suspend cronjob
	_, err := s.executor.Execute(cfg.KubeCLI, "patch", "cronjob", fmt.Sprintf("%s-repl-cronjob", cfg.VMName),
		"-n", cfg.Namespace, "--kubeconfig", cfg.SrcKubeconfig,
		"-p", `{"spec" : {"suspend" : true }}`)
	if err != nil {
		return fmt.Errorf("failed to suspend cronjob: %w", err)
	}

	return nil
}
