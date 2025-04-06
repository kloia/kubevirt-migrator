package main

import (
	"fmt"
	"strings"

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

func newCheckCmd(logger *zap.Logger) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "check",
		Short: "Check connectivity for VM migration",
		Long:  `Validate connectivity, permissions, and system requirements for VM migration.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCheck(cmd, logger)
		},
		// Silence usage when an error occurs to avoid printing twice
		SilenceUsage: true,
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

// runCheck executes the connectivity check for VM migration
func runCheck(cmd *cobra.Command, logger *zap.Logger) error {
	// Parse configuration
	cfg, err := config.ParseInitConfig(cmd)
	if err != nil {
		return fmt.Errorf("failed to parse configuration: %w", err)
	}

	logger.Debug("Configuration parsed successfully",
		zap.String("vm", cfg.VMName),
		zap.String("namespace", cfg.Namespace),
		zap.String("srcKubeconfig", cfg.SrcKubeconfig),
		zap.String("dstKubeconfig", cfg.DstKubeconfig),
		zap.String("kubeCLI", cfg.KubeCLI),
		zap.Int("sshPort", cfg.SSHPort),
		zap.String("syncTool", cfg.SyncTool))

	// Set up command executor
	cmdExec := executor.NewShellExecutor(logger)
	logger.Debug("Command executor initialized")

	// Set up template manager
	tmplMgr := template.NewManager(cmdExec, logger, cfg.KubeCLI)
	logger.Debug("Template manager initialized", zap.String("kubeCLI", cfg.KubeCLI))

	// Set up SSH manager
	sshMgr := replication.NewSSHManager(cmdExec, logger)
	logger.Debug("SSH manager initialized")

	// Create sync tool
	syncTool, err := sync.NewSyncCommand(sync.SyncTool(cfg.SyncTool))
	if err != nil {
		return fmt.Errorf("error creating sync tool: %w", err)
	}
	logger.Debug("Sync tool initialized", zap.String("tool", cfg.SyncTool))

	// Create client factory and clients
	clientFactory := kubernetes.NewClientFactory(cmdExec, syncTool, logger)
	logger.Debug("Client factory created")

	logger.Debug("Creating source Kubernetes client", zap.String("kubeconfig", cfg.SrcKubeconfig))
	srcClient, err := clientFactory.CreateClient(kubernetes.ClientType(cfg.KubeCLI), cfg.SrcKubeconfig)
	if err != nil {
		return fmt.Errorf("error creating source client: %w", err)
	}
	logger.Debug("Source Kubernetes client created successfully")

	logger.Debug("Creating destination Kubernetes client", zap.String("kubeconfig", cfg.DstKubeconfig))
	dstClient, err := clientFactory.CreateClient(kubernetes.ClientType(cfg.KubeCLI), cfg.DstKubeconfig)
	if err != nil {
		return fmt.Errorf("error creating destination client: %w", err)
	}
	logger.Debug("Destination Kubernetes client created successfully")

	// Create check manager
	checkMgr := replication.NewCheckManager(cmdExec, logger, tmplMgr, sshMgr, srcClient, dstClient)
	logger.Debug("Check manager created successfully")

	// Run connectivity checks
	logger.Info("Starting connectivity and permission validation",
		zap.String("vm", cfg.VMName),
		zap.String("namespace", cfg.Namespace))

	logger.Debug("Beginning connectivity check process")
	if err := checkMgr.CheckConnectivity(cfg); err != nil {
		// Provide cleaner error output without stack trace
		logger.Info("❌ Connectivity check failed")
		logger.Debug("Connectivity check failure details", zap.Error(err))

		// Print connectivity check results summary
		fmt.Printf("\n%s\n", strings.Repeat("=", 60))
		fmt.Printf("🔍 CONNECTIVITY CHECK RESULTS SUMMARY\n")
		fmt.Printf("%s\n", strings.Repeat("=", 60))

		// Get all check results
		checkResults := checkMgr.GetCheckResults()

		// Display results in a nice format
		for check, result := range checkResults {
			var statusSymbol, statusText string

			switch result {
			case replication.CheckSuccess:
				statusSymbol = "✓"
				statusText = "SUCCESS"
			case replication.CheckFailed:
				statusSymbol = "✗"
				statusText = "FAILED"
			case replication.CheckNotTested:
				statusSymbol = "?"
				statusText = "NOT TESTED"
			}

			fmt.Printf("%-30s %s  %s\n", check+":", statusSymbol, statusText)
		}

		fmt.Printf("%s\n\n", strings.Repeat("=", 60))
		fmt.Printf("❗ Error details: %v\n\n", err)

		return fmt.Errorf("connectivity check failed")
	}

	logger.Debug("All connectivity checks completed successfully")

	// Print connectivity check results summary for successful run
	fmt.Printf("\n%s\n", strings.Repeat("=", 60))
	fmt.Printf("🔍 CONNECTIVITY CHECK RESULTS SUMMARY\n")
	fmt.Printf("%s\n", strings.Repeat("=", 60))

	// Get all check results
	checkResults := checkMgr.GetCheckResults()

	// Display results in a nice format
	for check, result := range checkResults {
		var statusSymbol, statusText string

		switch result {
		case replication.CheckSuccess:
			statusSymbol = "✓"
			statusText = "SUCCESS"
		case replication.CheckFailed:
			statusSymbol = "✗"
			statusText = "FAILED"
		case replication.CheckNotTested:
			statusSymbol = "?"
			statusText = "NOT TESTED"
		}

		fmt.Printf("%-30s %s  %s\n", check+":", statusSymbol, statusText)
	}

	fmt.Printf("%s\n\n", strings.Repeat("=", 60))

	logger.Info("✓ All validation checks passed successfully!")
	return nil
}
