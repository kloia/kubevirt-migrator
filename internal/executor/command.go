package executor

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"go.uber.org/zap"
)

// ShellExecutor implements CommandExecutor using os/exec package
type ShellExecutor struct {
	logger *zap.Logger
}

// NewShellExecutor creates a new instance of ShellExecutor
func NewShellExecutor(logger *zap.Logger) *ShellExecutor {
	return &ShellExecutor{
		logger: logger,
	}
}

// Execute runs a command and returns its output
func (e *ShellExecutor) Execute(command string, args ...string) (string, error) {
	return e.executeWithOptions(command, CommandOptions{}, args...)
}

// ExecuteWithInput runs a command with input and returns its output
func (e *ShellExecutor) ExecuteWithInput(command string, input string, args ...string) (string, error) {
	return e.executeWithOptions(command, CommandOptions{Input: input}, args...)
}

// ExecuteWithEnv runs a command with environment variables and returns its output
func (e *ShellExecutor) ExecuteWithEnv(command string, env map[string]string, args ...string) (string, error) {
	return e.executeWithOptions(command, CommandOptions{Env: env}, args...)
}

// executeWithOptions is the internal implementation that runs commands with options
func (e *ShellExecutor) executeWithOptions(command string, options CommandOptions, args ...string) (string, error) {
	cmd := exec.Command(command, args...)

	if options.Dir != "" {
		cmd.Dir = options.Dir
	}

	// Set environment variables if provided
	if len(options.Env) > 0 {
		cmd.Env = os.Environ()
		for k, v := range options.Env {
			cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
		}
	}

	// Set up input if provided
	if options.Input != "" {
		cmd.Stdin = strings.NewReader(options.Input)
	}

	// Set up output
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	// Log command execution
	if e.logger != nil {
		e.logger.Debug("Executing command",
			zap.String("command", command),
			zap.Strings("args", args),
			zap.String("dir", options.Dir),
		)
	}

	// Execute command
	err := cmd.Run()

	stdoutStr := stdout.String()
	stderrStr := stderr.String()

	// Log output on error
	if err != nil && e.logger != nil {
		e.logger.Error("Command execution failed",
			zap.String("command", command),
			zap.Strings("args", args),
			zap.String("stdout", stdoutStr),
			zap.String("stderr", stderrStr),
			zap.Error(err),
		)
		return stderrStr, fmt.Errorf("command execution failed: %w: %s", err, stderrStr)
	}

	return stdoutStr, nil
}
