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
	// Global flags - only keep truly global flags here
	rootCmd.PersistentFlags().String("log-level", "info", "Log level (debug, info, warn, error)")

	// Disable completion command
	rootCmd.CompletionOptions.DisableDefaultCmd = true

	// Bind global flags to viper
	if err := viper.BindPFlag("log-level", rootCmd.PersistentFlags().Lookup("log-level")); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to bind flag log-level: %v\n", err)
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
