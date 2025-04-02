package config

import (
	"fmt"

	"github.com/spf13/viper"
)

// Config holds the application configuration
type Config struct {
	VMName        string
	Namespace     string
	SrcKubeconfig string
	DstKubeconfig string
	PreservePodIP bool
	LogLevel      string
	SSHPort       int
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
	return nil
}

// LoadConfig loads configuration from viper
func LoadConfig() (*Config, error) {
	viper.SetEnvPrefix("KUBEVIRT_MIGRATOR")
	viper.AutomaticEnv()

	c := &Config{
		VMName:        viper.GetString("vm-name"),
		Namespace:     viper.GetString("namespace"),
		SrcKubeconfig: viper.GetString("src-kubeconfig"),
		DstKubeconfig: viper.GetString("dst-kubeconfig"),
		PreservePodIP: viper.GetBool("preserve-pod-ip"),
		LogLevel:      viper.GetString("log-level"),
		SSHPort:       viper.GetInt("ssh-port"),
	}

	// Set defaults
	if c.LogLevel == "" {
		c.LogLevel = "info"
	}
	if c.SSHPort == 0 {
		c.SSHPort = 22
	}

	return c, c.Validate()
}
