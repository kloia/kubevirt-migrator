package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"go.uber.org/zap"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"

	"github.com/kloia/kubevirt-migrator/internal/config"
	"github.com/kloia/kubevirt-migrator/internal/executor"
	"github.com/kloia/kubevirt-migrator/internal/kubernetes"
	"github.com/kloia/kubevirt-migrator/internal/replication"
	"github.com/kloia/kubevirt-migrator/internal/sync"
	"github.com/kloia/kubevirt-migrator/internal/template"
)

func newInitCmd(logger *zap.Logger) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize VM migration",
		Long:  `Sets up VM migration infrastructure and starts initial replication.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runInit(cmd, logger)
		},
	}

	// Add command-specific flags
	cmd.Flags().String("vm-name", "", "Name of the virtual machine (required)")
	cmd.Flags().String("namespace", "", "Kubernetes namespace (required)")
	cmd.Flags().String("src-kubeconfig", "", "Source cluster kubeconfig file (required)")
	cmd.Flags().String("dst-kubeconfig", "", "Destination cluster kubeconfig file (required)")
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

func runInit(cmd *cobra.Command, logger *zap.Logger) error {
	// Parse config from command flags
	cfg, err := config.ParseInitConfig(cmd)
	if err != nil {
		return fmt.Errorf("failed to parse configuration: %w", err)
	}

	logger.Info("Initializing migration",
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

	// Check VM status
	vmStatus, err := srcClient.GetVMStatus(cfg.VMName, cfg.Namespace)
	if err != nil {
		return fmt.Errorf("failed to get VM status: %w", err)
	}
	logger.Info("Source VM status", zap.String("status", vmStatus))

	// Create destination VM if needed
	if err := createDestVM(srcClient, dstClient, cfg, logger); err != nil {
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

	// Perform initial sync with appropriate sync tool
	if err := syncMgr.PerformInitialSync(cfg); err != nil {
		return err
	}

	// Setup CronJob for async replication using the SyncManager
	if err := syncMgr.SetupCronJob(cfg); err != nil {
		return err
	}

	logger.Info("Migration initialization complete")
	return nil
}

// createDestVM creates destination VM by importing from source
func createDestVM(srcClient, dstClient kubernetes.KubernetesClient, cfg *config.Config, logger *zap.Logger) error {
	// Check if VM already exists
	_, err := dstClient.GetVMStatus(cfg.VMName, cfg.Namespace)
	if err == nil {
		logger.Info("Destination VM already exists")
		return nil
	} else if strings.Contains(err.Error(), "NotFound") {
		// This is expected - VM doesn't exist yet
		logger.Info("Destination VM doesn't exist yet, will be created from source VM export")
	} else {
		// Unexpected error, return it
		return fmt.Errorf("error checking destination VM status: %w", err)
	}

	logger.Info("Creating destination VM")

	// Export VM from source
	vmDef, err := srcClient.ExportVM(cfg.VMName, cfg.Namespace)

	if err != nil {
		return fmt.Errorf("failed to export VM: %w", err)
	}

	// Ensure VM is stopped by setting running=false
	tmpFile := "/tmp/vm-def.yaml"
	if err := os.WriteFile(tmpFile, vmDef, 0600); err != nil {
		return fmt.Errorf("failed to write VM definition to file: %w", err)
	}

	exec := executor.NewShellExecutor(logger)

	// Check if runStrategy exists
	runStrategyOutput, err := exec.Execute("yq", "e", ".spec.runStrategy", tmpFile)
	runStrategyExists := err == nil && runStrategyOutput != "" && runStrategyOutput != "null"

	// Set VM to stopped state based on what's available
	if runStrategyExists {
		// Use runStrategy if it exists
		_, err = exec.Execute("yq", "e", "-i", `.spec.runStrategy = "Halted"`, tmpFile)
		if err != nil {
			if removeErr := os.Remove(tmpFile); removeErr != nil {
				logger.Warn("Failed to remove temp file", zap.String("file", tmpFile), zap.Error(removeErr))
			}
			return fmt.Errorf("failed to update VM definition to stopped state: %w", err)
		}
		logger.Info("Using runStrategy=Halted to stop VM")
	} else {
		// Use running field if runStrategy doesn't exist
		_, err = exec.Execute("yq", "e", "-i", ".spec.running = false", tmpFile)
		if err != nil {
			if removeErr := os.Remove(tmpFile); removeErr != nil {
				logger.Warn("Failed to remove temp file", zap.String("file", tmpFile), zap.Error(removeErr))
			}
			return fmt.Errorf("failed to update VM definition to stopped state: %w", err)
		}
		logger.Info("Using running=false to stop VM")
	}

	// Read back the modified definition
	vmDef, err = os.ReadFile(tmpFile)
	if err != nil {
		if removeErr := os.Remove(tmpFile); removeErr != nil {
			logger.Warn("Failed to remove temp file", zap.String("file", tmpFile), zap.Error(removeErr))
		}
		return fmt.Errorf("failed to read modified VM definition: %w", err)
	}

	// Clean up temp file
	if err := os.Remove(tmpFile); err != nil {
		logger.Warn("Failed to remove temp file", zap.String("file", tmpFile), zap.Error(err))
	}

	// Import VM to destination
	if err := dstClient.ImportVM(vmDef, cfg.Namespace); err != nil {
		return fmt.Errorf("failed to import VM: %w", err)
	}

	// Wait for destination VM to reach "Stopped" status
	logger.Info("Waiting for destination VM to be ready")
	if err := dstClient.WaitForVMStatus(cfg.VMName, cfg.Namespace, "Stopped", 5*time.Minute); err != nil {
		return fmt.Errorf("error waiting for destination VM to be ready: %w", err)
	}

	logger.Info("Destination VM created")
	return nil
}

// createPodAndWait creates a pod from a template and waits for it to be ready
func createPodAndWait(tmplMgr *template.Manager, cfg *config.Config, logger *zap.Logger, isSource bool, templateKind template.TemplateKind, templateVars template.TemplateVariables) error {
	// Set source/destination variables
	podSuffix := "src"
	kubeconfig := cfg.SrcKubeconfig
	if !isSource {
		podSuffix = "dst"
		kubeconfig = cfg.DstKubeconfig
	}

	podName := fmt.Sprintf("%s-%s-replicator", cfg.VMName, podSuffix)

	// Initialize a title caser
	caser := cases.Title(language.English)

	// Check if replicator already exists
	exec := executor.NewShellExecutor(logger)
	_, err := exec.Execute(cfg.KubeCLI, "get", "pod", podName, "-n", cfg.Namespace,
		"--kubeconfig", kubeconfig, "--no-headers")
	if err == nil {
		logger.Info(fmt.Sprintf("%s replicator already exists", caser.String(podSuffix)))
		return nil
	} else if strings.Contains(err.Error(), "NotFound") {
		// This is expected - pod doesn't exist yet
		logger.Info(fmt.Sprintf("%s replicator doesn't exist yet, will be created", caser.String(podSuffix)))
	} else {
		// Unexpected error, return it
		return fmt.Errorf("error checking %s replicator: %w", podSuffix, err)
	}

	logger.Info(fmt.Sprintf("Creating %s replicator", podSuffix))

	// Apply template
	err = tmplMgr.RenderAndApply(templateKind, templateVars, kubeconfig)
	if err != nil {
		return fmt.Errorf("failed to create %s replicator: %w", podSuffix, err)
	}

	// Wait for pod to be ready
	_, err = exec.Execute(cfg.KubeCLI, "wait", "pod", podName, "-n", cfg.Namespace,
		"--kubeconfig", kubeconfig, "--for=condition=Ready", "--timeout=-1m")
	if err != nil {
		return fmt.Errorf("failed waiting for %s replicator to be ready: %w", podSuffix, err)
	}

	logger.Info(fmt.Sprintf("%s replicator created", caser.String(podSuffix)))
	return nil
}

func createSourceReplicator(tmplMgr *template.Manager, cfg *config.Config, logger *zap.Logger) error {
	return createPodAndWait(tmplMgr, cfg, logger, true, template.SourceReplicator, template.TemplateVariables{
		VMName:    cfg.VMName,
		Namespace: cfg.Namespace,
	})
}

func createDestReplicator(tmplMgr *template.Manager, cfg *config.Config, logger *zap.Logger) error {
	// First create replicator pod
	err := createPodAndWait(tmplMgr, cfg, logger, false, template.DestReplicator, template.TemplateVariables{
		VMName:    cfg.VMName,
		Namespace: cfg.Namespace,
	})
	if err != nil {
		return err
	}

	// Create service
	logger.Info("Creating destination service")
	err = tmplMgr.RenderAndApply(template.DestService, template.TemplateVariables{
		VMName:     cfg.VMName,
		Namespace:  cfg.Namespace,
		Port:       cfg.SSHPort,
		TargetPort: cfg.SSHPort,
	}, cfg.DstKubeconfig)
	if err != nil {
		return fmt.Errorf("failed to create destination service: %w", err)
	}

	return nil
}
