package main

import (
	"fmt"

	"github.com/spf13/cobra"
	"go.uber.org/zap"

	"github.com/kloia/kubevirt-migrator/internal/config"
	"github.com/kloia/kubevirt-migrator/internal/executor"
	"github.com/kloia/kubevirt-migrator/internal/replication"
	"github.com/kloia/kubevirt-migrator/internal/template"
)

func newMigrateCmd(logger *zap.Logger) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "migrate",
		Short: "Perform VM migration",
		Long:  `Completes VM migration by stopping source VM, performing final sync, and starting destination VM.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runMigrate(logger)
		},
	}

	return cmd
}

func runMigrate(logger *zap.Logger) error {
	// Load configuration
	cfg, err := config.LoadConfig()
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	logger.Info("Starting migration",
		zap.String("vm", cfg.VMName),
		zap.String("namespace", cfg.Namespace))

	// Create executor
	exec := executor.NewShellExecutor(logger)

	// Create SSH manager
	sshMgr := replication.NewSSHManager(exec, logger)

	// Create template manager
	tmplMgr := template.NewManager(exec, logger)

	// Create sync manager
	syncMgr := replication.NewSyncManager(exec, logger, sshMgr, tmplMgr)

	// Suspend cronjob first
	if err := syncMgr.SuspendCronJob(cfg); err != nil {
		return err
	}

	// Stop source VM
	if err := stopSourceVM(exec, cfg, logger); err != nil {
		return err
	}

	// Wait for source VM to stop
	if err := waitForSourceVMStop(exec, cfg, logger); err != nil {
		return err
	}

	// Now perform final sync on stopped VM
	if err := syncMgr.PerformFinalSync(cfg); err != nil {
		return err
	}

	// Start destination VM
	if err := startDestVM(exec, cfg, logger); err != nil {
		return err
	}

	// Wait for destination VM to start
	if err := waitForDestVMStart(exec, cfg, logger); err != nil {
		return err
	}

	// Cleanup resources
	if err := cleanup(exec, cfg, logger); err != nil {
		return err
	}

	logger.Info("Migration completed successfully")
	return nil
}

func stopSourceVM(exec executor.CommandExecutor, cfg *config.Config, logger *zap.Logger) error {
	logger.Info("Stopping source VM")
	_, err := exec.Execute("virtctl", "stop", cfg.VMName, "--kubeconfig", cfg.SrcKubeconfig)
	if err != nil {
		return fmt.Errorf("failed to stop source VM: %w", err)
	}
	return nil
}

func waitForSourceVMStop(exec executor.CommandExecutor, cfg *config.Config, logger *zap.Logger) error {
	logger.Info("Waiting for source VM to stop")

	for {
		status, err := exec.Execute("oc", "get", "vm", cfg.VMName, "-n", cfg.Namespace,
			"--kubeconfig", cfg.SrcKubeconfig, "--no-headers", "-o", "custom-columns=STATUS:.status.printableStatus")
		if err != nil {
			return fmt.Errorf("failed to get source VM status: %w", err)
		}

		if status == "Stopped" {
			logger.Info("Source VM stopped")
			break
		}

		logger.Info("Source VM is stopping", zap.String("status", status))
		_, err = exec.Execute("sleep", "5")
		if err != nil {
			return fmt.Errorf("failed during sleep: %w", err)
		}
	}

	return nil
}

func startDestVM(exec executor.CommandExecutor, cfg *config.Config, logger *zap.Logger) error {
	logger.Info("Starting destination VM")
	_, err := exec.Execute("virtctl", "start", cfg.VMName, "--kubeconfig", cfg.DstKubeconfig)
	if err != nil {
		return fmt.Errorf("failed to start destination VM: %w", err)
	}
	return nil
}

func waitForDestVMStart(exec executor.CommandExecutor, cfg *config.Config, logger *zap.Logger) error {
	logger.Info("Waiting for destination VM to start")

	for {
		status, err := exec.Execute("oc", "get", "vm", cfg.VMName, "-n", cfg.Namespace,
			"--kubeconfig", cfg.DstKubeconfig, "--no-headers", "-o", "custom-columns=STATUS:.status.printableStatus")
		if err != nil {
			return fmt.Errorf("failed to get destination VM status: %w", err)
		}

		if status == "Running" {
			logger.Info("Destination VM started")
			break
		}

		logger.Info("Destination VM is starting", zap.String("status", status))
		_, err = exec.Execute("sleep", "5")
		if err != nil {
			return fmt.Errorf("failed during sleep: %w", err)
		}
	}

	return nil
}

func cleanup(exec executor.CommandExecutor, cfg *config.Config, logger *zap.Logger) error {
	logger.Info("Cleaning up resources")

	// Delete final job
	_, err := exec.Execute("oc", "delete", "job", fmt.Sprintf("%s-repl-final-job", cfg.VMName),
		"-n", cfg.Namespace, "--kubeconfig", cfg.SrcKubeconfig, "--wait")
	if err != nil {
		logger.Warn("Failed to delete final job", zap.Error(err))
	}

	// Delete cronjob
	_, err = exec.Execute("oc", "delete", "cronjob", fmt.Sprintf("%s-repl-cronjob", cfg.VMName),
		"-n", cfg.Namespace, "--kubeconfig", cfg.SrcKubeconfig, "--wait")
	if err != nil {
		logger.Warn("Failed to delete cronjob", zap.Error(err))
	}

	// Delete source replicator
	_, err = exec.Execute("oc", "delete", "pod", fmt.Sprintf("%s-src-replicator", cfg.VMName),
		"-n", cfg.Namespace, "--kubeconfig", cfg.SrcKubeconfig, "--wait")
	if err != nil {
		logger.Warn("Failed to delete source replicator", zap.Error(err))
	}

	// Delete SSH keys secret
	_, err = exec.Execute("oc", "delete", "secret", fmt.Sprintf("%s-repl-ssh-keys", cfg.VMName),
		"-n", cfg.Namespace, "--kubeconfig", cfg.SrcKubeconfig, "--wait")
	if err != nil {
		logger.Warn("Failed to delete SSH keys secret", zap.Error(err))
	}

	// Delete destination replicator
	_, err = exec.Execute("oc", "delete", "pod", fmt.Sprintf("%s-dst-replicator", cfg.VMName),
		"-n", cfg.Namespace, "--kubeconfig", cfg.DstKubeconfig, "--wait")
	if err != nil {
		logger.Warn("Failed to delete destination replicator", zap.Error(err))
	}

	// Delete destination service
	_, err = exec.Execute("oc", "delete", "svc", fmt.Sprintf("%s-dst-svc", cfg.VMName),
		"-n", cfg.Namespace, "--kubeconfig", cfg.DstKubeconfig, "--wait")
	if err != nil {
		logger.Warn("Failed to delete destination service", zap.Error(err))
	}

	logger.Info("Cleanup completed")
	return nil
}
