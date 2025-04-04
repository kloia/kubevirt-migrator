package executor

import "go.uber.org/zap"

// CommandExecutor defines the interface for executing shell commands
type CommandExecutor interface {
	// Execute runs a command and returns its output
	Execute(command string, args ...string) (string, error)

	// ExecuteWithInput runs a command with input and returns its output
	ExecuteWithInput(command string, input string, args ...string) (string, error)

	// ExecuteWithEnv runs a command with environment variables and returns its output
	ExecuteWithEnv(command string, env map[string]string, args ...string) (string, error)
}

// CommandOptions contains options for command execution
type CommandOptions struct {
	// Directory to run the command in
	Dir string

	// Environment variables to set
	Env map[string]string

	// Input to provide to the command
	Input string

	// Logger to use
	Logger *zap.Logger

	// Kubeconfig file to use for kubectl/oc commands
	Kubeconfig string
}
