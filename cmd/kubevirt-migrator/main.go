package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"go.uber.org/zap"

	"github.com/kloia/kubevirt-migrator/pkg/logging"
)

var rootCmd = &cobra.Command{
	Use:   "kubevirt-migrator",
	Short: "Migrate VMs between KubeVirt clusters",
	Long: `A tool for migrating virtual machines between KubeVirt clusters.
It replicates disk contents and VM definitions while preserving settings.`,
}

func init() {
	// Global flags
	rootCmd.PersistentFlags().String("vm-name", "", "Name of the virtual machine (required)")
	rootCmd.PersistentFlags().String("namespace", "", "Kubernetes namespace (required)")
	rootCmd.PersistentFlags().String("src-kubeconfig", "", "Source cluster kubeconfig file (required)")
	rootCmd.PersistentFlags().String("dst-kubeconfig", "", "Destination cluster kubeconfig file (required)")
	rootCmd.PersistentFlags().Bool("preserve-pod-ip", false, "Preserve pod IP address during migration")
	rootCmd.PersistentFlags().String("log-level", "info", "Log level (debug, info, warn, error)")
	rootCmd.PersistentFlags().Int("ssh-port", 22, "SSH port for replication")

	// Bind flags to viper
	if err := viper.BindPFlag("vm-name", rootCmd.PersistentFlags().Lookup("vm-name")); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to bind flag vm-name: %v\n", err)
		os.Exit(1)
	}
	if err := viper.BindPFlag("namespace", rootCmd.PersistentFlags().Lookup("namespace")); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to bind flag namespace: %v\n", err)
		os.Exit(1)
	}
	if err := viper.BindPFlag("src-kubeconfig", rootCmd.PersistentFlags().Lookup("src-kubeconfig")); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to bind flag src-kubeconfig: %v\n", err)
		os.Exit(1)
	}
	if err := viper.BindPFlag("dst-kubeconfig", rootCmd.PersistentFlags().Lookup("dst-kubeconfig")); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to bind flag dst-kubeconfig: %v\n", err)
		os.Exit(1)
	}
	if err := viper.BindPFlag("preserve-pod-ip", rootCmd.PersistentFlags().Lookup("preserve-pod-ip")); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to bind flag preserve-pod-ip: %v\n", err)
		os.Exit(1)
	}
	if err := viper.BindPFlag("log-level", rootCmd.PersistentFlags().Lookup("log-level")); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to bind flag log-level: %v\n", err)
		os.Exit(1)
	}
	if err := viper.BindPFlag("ssh-port", rootCmd.PersistentFlags().Lookup("ssh-port")); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to bind flag ssh-port: %v\n", err)
		os.Exit(1)
	}

	// Environment variables with prefix
	viper.SetEnvPrefix("KUBEVIRT_MIGRATOR")
	viper.AutomaticEnv()
}

func main() {
	// Setup logger
	logLevel := viper.GetString("log-level")
	if logLevel == "" {
		logLevel = "info"
	}

	logger, err := logging.NewLogger(logLevel)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize logger: %v\n", err)
		os.Exit(1)
	}
	defer func() {
		// Ignore "inappropriate ioctl for device" errors which occur when syncing to stdout
		err := logger.Sync()
		if err != nil && !strings.Contains(err.Error(), "inappropriate ioctl for device") {
			fmt.Fprintf(os.Stderr, "Failed to sync logger: %v\n", err)
		}
	}()

	// Add commands
	addCommands(logger)

	// Execute
	if err := rootCmd.Execute(); err != nil {
		logger.Error("Command execution failed", zap.Error(err))
		os.Exit(1)
	}
}

func addCommands(logger *zap.Logger) {
	// Add init and migrate commands
	rootCmd.AddCommand(newInitCmd(logger))
	rootCmd.AddCommand(newMigrateCmd(logger))
	rootCmd.AddCommand(newVersionCmd())
}
