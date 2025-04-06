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
	Run: func(cmd *cobra.Command, args []string) {
		// Print help if no subcommand is provided
		if err := cmd.Help(); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to display help: %v\n", err)
		}
	},
	SilenceUsage: true,
}

func init() {
	// Global flags - only keep truly global flags here
	rootCmd.PersistentFlags().String("log-level", "info", "Log level (debug, info, warn, error)")

	// Customize command help and errors
	rootCmd.SilenceErrors = true
	rootCmd.SilenceUsage = true

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

	// Run the root command with custom error handling
	runRootCmd(logger)
}

func runRootCmd(logger *zap.Logger) {
	// Execute
	err := rootCmd.Execute()
	if err != nil {
		// Log error at debug level to avoid stack trace in normal output
		logger.Debug("Command execution failed", zap.Error(err))

		// Print user-friendly error message, removing any trailing newlines
		errMsg := strings.TrimSpace(err.Error())
		fmt.Fprintf(os.Stderr, "Error: %s\n", errMsg)

		// If this is an unknown command, suggest help
		if strings.Contains(errMsg, "unknown command") {
			fmt.Fprintf(os.Stderr, "Run 'kubevirt-migrator --help' for usage information.\n")
		}

		os.Exit(1)
	}
}

func addCommands(logger *zap.Logger) {
	// Add init and migrate commands
	rootCmd.AddCommand(newInitCmd(logger))
	rootCmd.AddCommand(newMigrateCmd(logger))
	rootCmd.AddCommand(newCheckCmd(logger))
	rootCmd.AddCommand(newVersionCmd())

	// Remove default completion command
	rootCmd.CompletionOptions.DisableDefaultCmd = true

	// Add custom completion command without PowerShell - We don't support windows for now
	var completionCmd = &cobra.Command{
		Use:   "completion",
		Short: "Generate the autocompletion script for the specified shell",
		Long:  "Generate the autocompletion script for kubevirt-migrator for the specified shell.",
	}

	// Add bash completion
	var bashCompletionCmd = &cobra.Command{
		Use:   "bash",
		Short: "Generate the autocompletion script for bash",
		RunE: func(cmd *cobra.Command, args []string) error {
			return rootCmd.GenBashCompletion(os.Stdout)
		},
	}

	// Add zsh completion
	var zshCompletionCmd = &cobra.Command{
		Use:   "zsh",
		Short: "Generate the autocompletion script for zsh",
		RunE: func(cmd *cobra.Command, args []string) error {
			return rootCmd.GenZshCompletion(os.Stdout)
		},
	}

	// Add fish completion
	var fishCompletionCmd = &cobra.Command{
		Use:   "fish",
		Short: "Generate the autocompletion script for fish",
		RunE: func(cmd *cobra.Command, args []string) error {
			return rootCmd.GenFishCompletion(os.Stdout, true)
		},
	}

	completionCmd.AddCommand(bashCompletionCmd)
	completionCmd.AddCommand(zshCompletionCmd)
	completionCmd.AddCommand(fishCompletionCmd)
	rootCmd.AddCommand(completionCmd)
}
