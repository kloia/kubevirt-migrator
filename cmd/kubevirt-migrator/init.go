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

func newInitCmd(logger *zap.Logger) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize VM migration",
		Long:  `Sets up VM migration infrastructure and starts initial replication.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runInit(logger)
		},
	}

	return cmd
}

func runInit(logger *zap.Logger) error {
	// Load configuration
	cfg, err := config.LoadConfig()
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	logger.Info("Initializing migration",
		zap.String("vm", cfg.VMName),
		zap.String("namespace", cfg.Namespace))

	// Create executor
	exec := executor.NewShellExecutor(logger)

	// Create template manager
	tmplMgr := template.NewManager(exec, logger)

	// Create SSH manager
	sshMgr := replication.NewSSHManager(exec, logger)

	// Create sync manager
	syncMgr := replication.NewSyncManager(exec, logger, sshMgr, tmplMgr)

	// Check VM status
	if err := checkVMStatus(exec, cfg, logger); err != nil {
		return err
	}

	// Create destination VM if needed
	if err := createDestVM(exec, cfg, logger); err != nil {
		return err
	}

	// Create source replicator
	if err := createSourceReplicator(tmplMgr, cfg, logger); err != nil {
		return err
	}

	// Generate SSH keys
	if err := sshMgr.GenerateKeys(cfg); err != nil {
		return err
	}

	// Create destination replicator
	if err := createDestReplicator(tmplMgr, cfg, logger); err != nil {
		return err
	}

	// Setup destination SSH authorization
	if err := sshMgr.SetupDestinationAuth(cfg); err != nil {
		return err
	}

	// Perform initial sync
	if err := syncMgr.PerformInitialSync(cfg); err != nil {
		return err
	}

	// Setup CronJob for async replication
	if err := syncMgr.SetupCronJob(cfg); err != nil {
		return err
	}

	logger.Info("Migration initialization complete")
	return nil
}

func checkVMStatus(exec executor.CommandExecutor, cfg *config.Config, logger *zap.Logger) error {
	logger.Info("Checking VM status")

	// Source VM
	srcVM, err := exec.Execute("oc", "get", "vm", cfg.VMName, "-n", cfg.Namespace, "--kubeconfig", cfg.SrcKubeconfig, "--no-headers")
	if err != nil {
		return fmt.Errorf("failed to get source VM: %w", err)
	}
	logger.Info("Source VM status", zap.String("status", srcVM))

	// Destination VM
	dstOutput, dstErr := exec.Execute("oc", "get", "vm", cfg.VMName, "-n", cfg.Namespace, "--kubeconfig", cfg.DstKubeconfig, "--no-headers")
	if dstErr != nil {
		logger.Info("Destination VM not found, it will be created", zap.Error(dstErr))
	} else {
		logger.Info("Destination VM found", zap.String("vm", dstOutput))
	}

	return nil
}

func createDestVM(exec executor.CommandExecutor, cfg *config.Config, logger *zap.Logger) error {
	// Check if VM already exists
	_, err := exec.Execute("oc", "get", "vm", cfg.VMName, "-n", cfg.Namespace, "--kubeconfig", cfg.DstKubeconfig, "--no-headers")
	if err == nil {
		logger.Info("Destination VM already exists")
		return nil
	}

	logger.Info("Creating destination VM")

	// Export VM
	tmpFile := fmt.Sprintf("/tmp/%s-vm.yaml", cfg.VMName)
	_, err = exec.Execute("oc", "get", "vm", cfg.VMName, "-n", cfg.Namespace, "--kubeconfig", cfg.SrcKubeconfig, "-o", "yaml", ">", tmpFile)
	if err != nil {
		return fmt.Errorf("failed to export VM: %w", err)
	}

	// Update spec.running to false
	_, err = exec.Execute("yq", "e", "-i", ".spec.running = false", tmpFile)
	if err != nil {
		return fmt.Errorf("failed to update VM spec: %w", err)
	}

	// Handle IP preservation if requested
	if cfg.PreservePodIP {
		logger.Info("Preserving pod IP address")

		// Get pod IP
		podIP, err := exec.Execute("oc", "get", "vmi", cfg.VMName, "-n", cfg.Namespace, "--kubeconfig", cfg.SrcKubeconfig,
			"-o=jsonpath='{.status.interfaces[0].ipAddress}'")
		if err != nil {
			return fmt.Errorf("failed to get pod IP: %w", err)
		}

		// Get MAC
		podMAC, err := exec.Execute("oc", "get", "vmi", cfg.VMName, "-n", cfg.Namespace, "--kubeconfig", cfg.SrcKubeconfig,
			"-o=jsonpath='{.status.interfaces[0].mac}'")
		if err != nil {
			return fmt.Errorf("failed to get pod MAC: %w", err)
		}

		// Create annotation with IP and MAC
		ipJSON := fmt.Sprintf(`'{"default":{"ip_address":"%s/23","mac_address":"%s"}}'`, podIP, podMAC)
		_, err = exec.Execute("yq", "e", "-i", fmt.Sprintf(".spec.template.metadata.annotations[\"k8s.ovn.org/pod-networks\"] = %s", ipJSON), tmpFile)
		if err != nil {
			return fmt.Errorf("failed to set IP annotation: %w", err)
		}
	}

	// Apply to destination
	_, err = exec.Execute("oc", "apply", "--wait", "-n", cfg.Namespace, "--kubeconfig", cfg.DstKubeconfig, "-f", tmpFile)
	if err != nil {
		return fmt.Errorf("failed to create destination VM: %w", err)
	}

	logger.Info("Destination VM created")
	return nil
}

func createSourceReplicator(tmplMgr *template.Manager, cfg *config.Config, logger *zap.Logger) error {
	// Check if replicator already exists
	exec := executor.NewShellExecutor(logger)
	_, err := exec.Execute("oc", "get", "pod", fmt.Sprintf("%s-src-replicator", cfg.VMName), "-n", cfg.Namespace, "--kubeconfig", cfg.SrcKubeconfig, "--no-headers")
	if err == nil {
		logger.Info("Source replicator already exists")
		return nil
	}

	logger.Info("Creating source replicator")

	err = tmplMgr.RenderAndApply(template.SourceReplicator, template.TemplateVariables{
		VMName:    cfg.VMName,
		Namespace: cfg.Namespace,
	}, cfg.SrcKubeconfig)
	if err != nil {
		return fmt.Errorf("failed to create source replicator: %w", err)
	}

	// Wait for pod to be ready
	_, err = exec.Execute("oc", "wait", "pod", fmt.Sprintf("%s-src-replicator", cfg.VMName), "-n", cfg.Namespace,
		"--kubeconfig", cfg.SrcKubeconfig, "--for=condition=Ready", "--timeout=-1m")
	if err != nil {
		return fmt.Errorf("failed waiting for source replicator to be ready: %w", err)
	}

	logger.Info("Source replicator created")
	return nil
}

func createDestReplicator(tmplMgr *template.Manager, cfg *config.Config, logger *zap.Logger) error {
	// Check if replicator already exists
	exec := executor.NewShellExecutor(logger)
	_, err := exec.Execute("oc", "get", "pod", fmt.Sprintf("%s-dst-replicator", cfg.VMName), "-n", cfg.Namespace, "--kubeconfig", cfg.DstKubeconfig, "--no-headers")
	if err == nil {
		logger.Info("Destination replicator already exists")
		return nil
	}

	logger.Info("Creating destination replicator")

	// Create replicator pod
	err = tmplMgr.RenderAndApply(template.DestReplicator, template.TemplateVariables{
		VMName:    cfg.VMName,
		Namespace: cfg.Namespace,
	}, cfg.DstKubeconfig)
	if err != nil {
		return fmt.Errorf("failed to create destination replicator: %w", err)
	}

	// Wait for pod to be ready
	_, err = exec.Execute("oc", "wait", "pod", fmt.Sprintf("%s-dst-replicator", cfg.VMName), "-n", cfg.Namespace,
		"--kubeconfig", cfg.DstKubeconfig, "--for=condition=Ready", "--timeout=-1m")
	if err != nil {
		return fmt.Errorf("failed waiting for destination replicator to be ready: %w", err)
	}

	// Create service
	err = tmplMgr.RenderAndApply(template.DestService, template.TemplateVariables{
		VMName:     cfg.VMName,
		Namespace:  cfg.Namespace,
		Port:       cfg.SSHPort,
		TargetPort: cfg.SSHPort,
	}, cfg.DstKubeconfig)
	if err != nil {
		return fmt.Errorf("failed to create destination service: %w", err)
	}

	logger.Info("Destination replicator created")
	return nil
}
