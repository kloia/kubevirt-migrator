package config

import (
	"fmt"
	"regexp"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// Default values
const (
	DefaultLogLevel            = "info"
	DefaultSSHPort             = 22
	DefaultKubeCLI             = "oc"
	DefaultSyncTool            = "rclone"
	DefaultReplicationSchedule = "*/15 * * * *" // Every 15 minutes
)

// Config holds the application configuration
type Config struct {
	VMName        string
	Namespace     string
	SrcKubeconfig string
	DstKubeconfig string
	LogLevel      string
	SSHPort       int

	// New fields
	KubeCLI             string // "oc", "kubectl"
	SyncTool            string // "rclone", "rsync"
	ReplicationSchedule string // Cron schedule for replication
	DryRun              bool   // If true, will set up pods but not run sync or create cronjobs
}

// Validate checks if the configuration is valid
func (c *Config) Validate() error {
	if c.VMName == "" {
		return fmt.Errorf("vm-name is required")
	}
	if c.Namespace == "" {
		return fmt.Errorf("namespace is required")
	}
	if c.SrcKubeconfig == "" {
		return fmt.Errorf("src-kubeconfig is required")
	}
	if c.DstKubeconfig == "" {
		return fmt.Errorf("dst-kubeconfig is required")
	}

	// Validate new fields
	if c.KubeCLI != "oc" && c.KubeCLI != "kubectl" {
		return fmt.Errorf("kubecli must be 'oc' or 'kubectl'")
	}
	if c.SyncTool != "rclone" && c.SyncTool != "rsync" {
		return fmt.Errorf("sync-tool must be 'rclone' or 'rsync'")
	}

	// Validate cron schedule format if provided
	if c.ReplicationSchedule != "" {
		// Simple regex for basic cron format validation
		// This checks for 5 fields separated by spaces
		cronRegex := `^(\S+\s+){4}\S+$`
		match, err := regexp.MatchString(cronRegex, c.ReplicationSchedule)
		if err != nil {
			return fmt.Errorf("error validating replication schedule: %w", err)
		}
		if !match {
			return fmt.Errorf("invalid cron schedule format: %s. Must be in cron format with 5 fields (e.g. '*/15 * * * *')", c.ReplicationSchedule)
		}
	}

	return nil
}

// setDefaults sets default values for unset fields in the Config
func (c *Config) setDefaults() {
	if c.LogLevel == "" {
		c.LogLevel = DefaultLogLevel
	}
	if c.SSHPort == 0 {
		c.SSHPort = DefaultSSHPort
	}
	if c.KubeCLI == "" {
		c.KubeCLI = DefaultKubeCLI
	}
	if c.SyncTool == "" {
		c.SyncTool = DefaultSyncTool
	}
	if c.ReplicationSchedule == "" {
		c.ReplicationSchedule = DefaultReplicationSchedule
	}
}

// LoadConfig loads configuration from viper
func LoadConfig() (*Config, error) {
	viper.SetEnvPrefix("KUBEVIRT_MIGRATOR")
	viper.AutomaticEnv()

	c := &Config{
		VMName:              viper.GetString("vm-name"),
		Namespace:           viper.GetString("namespace"),
		SrcKubeconfig:       viper.GetString("src-kubeconfig"),
		DstKubeconfig:       viper.GetString("dst-kubeconfig"),
		LogLevel:            viper.GetString("log-level"),
		SSHPort:             viper.GetInt("ssh-port"),
		KubeCLI:             viper.GetString("kubecli"),
		SyncTool:            viper.GetString("sync-tool"),
		ReplicationSchedule: viper.GetString("replication-schedule"),
	}

	c.setDefaults()
	return c, c.Validate()
}

// parseCommonFlags parses flags that are common across commands
func parseCommonFlags(cmd *cobra.Command) (*Config, error) {
	cfg := &Config{}
	var err error

	cfg.VMName, err = cmd.Flags().GetString("vm-name")
	if err != nil {
		return nil, fmt.Errorf("error getting vm-name: %w", err)
	}

	cfg.Namespace, err = cmd.Flags().GetString("namespace")
	if err != nil {
		return nil, fmt.Errorf("error getting namespace: %w", err)
	}

	cfg.SrcKubeconfig, err = cmd.Flags().GetString("src-kubeconfig")
	if err != nil {
		return nil, fmt.Errorf("error getting src-kubeconfig: %w", err)
	}

	cfg.DstKubeconfig, err = cmd.Flags().GetString("dst-kubeconfig")
	if err != nil {
		return nil, fmt.Errorf("error getting dst-kubeconfig: %w", err)
	}

	cfg.KubeCLI, err = cmd.Flags().GetString("kubecli")
	if err != nil {
		return nil, fmt.Errorf("error getting kubecli: %w", err)
	}

	cfg.SyncTool, err = cmd.Flags().GetString("sync-tool")
	if err != nil {
		return nil, fmt.Errorf("error getting sync-tool: %w", err)
	}

	return cfg, nil
}

// ParseInitConfig parses the init command flags into a Config struct
func ParseInitConfig(cmd *cobra.Command) (*Config, error) {
	cfg, err := parseCommonFlags(cmd)
	if err != nil {
		return nil, err
	}

	cfg.SSHPort, err = cmd.Flags().GetInt("ssh-port")
	if err != nil {
		return nil, fmt.Errorf("error getting ssh-port: %w", err)
	}

	cfg.ReplicationSchedule, err = cmd.Flags().GetString("replication-schedule")
	if err != nil {
		return nil, fmt.Errorf("error getting replication-schedule: %w", err)
	}

	// Parse dry-run flag
	cfg.DryRun, err = cmd.Flags().GetBool("dry-run")
	if err != nil {
		return nil, fmt.Errorf("error getting dry-run: %w", err)
	}

	cfg.setDefaults()
	return cfg, cfg.Validate()
}

// ParseMigrateConfig parses the migrate command flags into a Config struct
func ParseMigrateConfig(cmd *cobra.Command) (*Config, error) {
	cfg, err := parseCommonFlags(cmd)
	if err != nil {
		return nil, err
	}

	cfg.setDefaults()
	return cfg, cfg.Validate()
}
