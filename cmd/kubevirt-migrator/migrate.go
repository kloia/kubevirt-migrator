package main

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"go.uber.org/zap"

	"github.com/kloia/kubevirt-migrator/internal/config"
	"github.com/kloia/kubevirt-migrator/internal/executor"
	"github.com/kloia/kubevirt-migrator/internal/kubernetes"
	"github.com/kloia/kubevirt-migrator/internal/replication"
	"github.com/kloia/kubevirt-migrator/internal/sync"
	"github.com/kloia/kubevirt-migrator/internal/template"
)

func newMigrateCmd(logger *zap.Logger) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "migrate",
		Short: "Perform VM migration",
		Long:  `Finalize the VM migration by performing final replication and switching VMs.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runMigrate(cmd, logger)
		},
	}

	// Add command-specific flags
	cmd.Flags().String("vm-name", "", "Name of the virtual machine (required)")
	cmd.Flags().String("namespace", "", "Kubernetes namespace (required)")
	cmd.Flags().String("src-kubeconfig", "", "Source cluster kubeconfig file (required)")
	cmd.Flags().String("dst-kubeconfig", "", "Destination cluster kubeconfig file (required)")
	cmd.Flags().Bool("preserve-pod-ip", false, "Preserve pod IP address during migration")
	cmd.Flags().Int("ssh-port", 22, "SSH port for replication")

	// Add the new kubecli and sync-tool flags
	cmd.Flags().String("kubecli", "oc", "Kubernetes CLI to use (oc, kubectl)")
	cmd.Flags().String("sync-tool", "rclone", "Synchronization tool to use (rclone, rsync)")

	// Mark required flags
	if err := cmd.MarkFlagRequired("vm-name"); err != nil {
		logger.Error("Failed to mark flag as required", zap.String("flag", "vm-name"), zap.Error(err))
	}
	if err := cmd.MarkFlagRequired("namespace"); err != nil {
		logger.Error("Failed to mark flag as required", zap.String("flag", "namespace"), zap.Error(err))
	}
	if err := cmd.MarkFlagRequired("src-kubeconfig"); err != nil {
		logger.Error("Failed to mark flag as required", zap.String("flag", "src-kubeconfig"), zap.Error(err))
	}
	if err := cmd.MarkFlagRequired("dst-kubeconfig"); err != nil {
		logger.Error("Failed to mark flag as required", zap.String("flag", "dst-kubeconfig"), zap.Error(err))
	}

	// Bind flags to viper
	if err := viper.BindPFlag("vm-name", cmd.Flags().Lookup("vm-name")); err != nil {
		logger.Error("Failed to bind flag", zap.String("flag", "vm-name"), zap.Error(err))
	}
	if err := viper.BindPFlag("namespace", cmd.Flags().Lookup("namespace")); err != nil {
		logger.Error("Failed to bind flag", zap.String("flag", "namespace"), zap.Error(err))
	}
	if err := viper.BindPFlag("src-kubeconfig", cmd.Flags().Lookup("src-kubeconfig")); err != nil {
		logger.Error("Failed to bind flag", zap.String("flag", "src-kubeconfig"), zap.Error(err))
	}
	if err := viper.BindPFlag("dst-kubeconfig", cmd.Flags().Lookup("dst-kubeconfig")); err != nil {
		logger.Error("Failed to bind flag", zap.String("flag", "dst-kubeconfig"), zap.Error(err))
	}
	if err := viper.BindPFlag("preserve-pod-ip", cmd.Flags().Lookup("preserve-pod-ip")); err != nil {
		logger.Error("Failed to bind flag", zap.String("flag", "preserve-pod-ip"), zap.Error(err))
	}
	if err := viper.BindPFlag("ssh-port", cmd.Flags().Lookup("ssh-port")); err != nil {
		logger.Error("Failed to bind flag", zap.String("flag", "ssh-port"), zap.Error(err))
	}
	if err := viper.BindPFlag("kubecli", cmd.Flags().Lookup("kubecli")); err != nil {
		logger.Error("Failed to bind flag", zap.String("flag", "kubecli"), zap.Error(err))
	}
	if err := viper.BindPFlag("sync-tool", cmd.Flags().Lookup("sync-tool")); err != nil {
		logger.Error("Failed to bind flag", zap.String("flag", "sync-tool"), zap.Error(err))
	}

	return cmd
}

func runMigrate(cmd *cobra.Command, logger *zap.Logger) error {
	// Parse config from command flags
	cfg, err := config.ParseMigrateConfig(cmd)
	if err != nil {
		return fmt.Errorf("failed to parse configuration: %w", err)
	}

	logger.Info("Starting migration",
		zap.String("vm", cfg.VMName),
		zap.String("namespace", cfg.Namespace),
		zap.String("kubecli", cfg.KubeCLI),
		zap.String("sync-tool", cfg.SyncTool))

	// Create command executor
	exec := executor.NewShellExecutor(logger)

	// Create sync tool
	syncTool, err := sync.NewSyncCommand(sync.SyncTool(cfg.SyncTool))
	if err != nil {
		return fmt.Errorf("error creating sync tool: %w", err)
	}

	// Create template manager with the appropriate kubeCLI
	tmplMgr := template.NewManager(exec, logger, cfg.KubeCLI)

	// Create client factory
	clientFactory := kubernetes.NewClientFactory(exec, syncTool, logger)

	// Create source and destination clients
	srcClient, err := clientFactory.CreateClient(kubernetes.ClientType(cfg.KubeCLI), cfg.SrcKubeconfig)
	if err != nil {
		return fmt.Errorf("error creating source client: %w", err)
	}

	dstClient, err := clientFactory.CreateClient(kubernetes.ClientType(cfg.KubeCLI), cfg.DstKubeconfig)
	if err != nil {
		return fmt.Errorf("error creating destination client: %w", err)
	}

	// Create SSH manager
	sshMgr := replication.NewSSHManager(exec, logger)

	// Create sync manager
	syncMgr := replication.NewSyncManager(exec, logger, sshMgr, tmplMgr)
	syncMgr.SetSyncTool(syncTool)

	// Perform final check
	if err := performFinalCheck(srcClient, dstClient, cfg, logger); err != nil {
		return err
	}

	// Stop source VM
	if err := srcClient.StopVM(cfg.VMName, cfg.Namespace); err != nil {
		return fmt.Errorf("failed to stop source VM: %w", err)
	}
	logger.Info("Source VM stopping")

	// Wait for source VM to be fully stopped
	if err := srcClient.WaitForVMStatus(cfg.VMName, cfg.Namespace, "Stopped", 5*time.Minute); err != nil {
		return fmt.Errorf("failed while waiting for source VM to stop: %w", err)
	}
	logger.Info("Source VM stopped")

	// Perform final sync
	if err := syncMgr.PerformFinalSync(cfg); err != nil {
		return fmt.Errorf("failed to perform final sync: %w", err)
	}
	logger.Info("Final replication complete")

	// Suspend replication CronJob
	if err := srcClient.SuspendCronJob(fmt.Sprintf("%s-repl-cronjob", cfg.VMName), cfg.Namespace); err != nil {
		logger.Warn("Failed to suspend replication cronjob", zap.Error(err))
	} else {
		logger.Info("Replication cronjob suspended")
	}

	// Start destination VM
	if err := dstClient.StartVM(cfg.VMName, cfg.Namespace); err != nil {
		return fmt.Errorf("failed to start destination VM: %w", err)
	}
	logger.Info("Destination VM starting")

	// Wait for destination VM to be fully running
	if err := dstClient.WaitForVMStatus(cfg.VMName, cfg.Namespace, "Running", 5*time.Minute); err != nil {
		return fmt.Errorf("failed while waiting for destination VM to start: %w", err)
	}
	logger.Info("Destination VM started")

	// Clean up resources using the client methods
	logger.Info("Starting cleanup of migration resources")
	var cleanupErrors []error

	// Clean up source resources
	if err := srcClient.CleanupMigrationResources(cfg.VMName, cfg.Namespace); err != nil {
		cleanupErrors = append(cleanupErrors, fmt.Errorf("source cleanup error: %w", err))
		logger.Warn("Source cleanup failed", zap.Error(err))
	}

	// Clean up destination resources
	if err := dstClient.CleanupMigrationResources(cfg.VMName, cfg.Namespace); err != nil {
		cleanupErrors = append(cleanupErrors, fmt.Errorf("destination cleanup error: %w", err))
		logger.Warn("Destination cleanup failed", zap.Error(err))
	}

	if len(cleanupErrors) > 0 {
		logger.Warn("Some cleanup operations failed", zap.Int("error_count", len(cleanupErrors)))
	} else {
		logger.Info("All migration resources cleaned up successfully")
	}

	logger.Info("Migration completed successfully")
	return nil
}

func performFinalCheck(srcClient, dstClient kubernetes.KubernetesClient, cfg *config.Config, logger *zap.Logger) error {
	// Check source VM status
	srcStatus, err := srcClient.GetVMStatus(cfg.VMName, cfg.Namespace)
	if err != nil {
		return fmt.Errorf("failed to check source VM status: %w", err)
	}
	logger.Info("Source VM status", zap.String("status", srcStatus))

	// Check destination VM status
	dstStatus, err := dstClient.GetVMStatus(cfg.VMName, cfg.Namespace)
	if err != nil {
		return fmt.Errorf("failed to check destination VM status: %w", err)
	}
	logger.Info("Destination VM status", zap.String("status", dstStatus))

	return nil
}
