package executor

import (
	"fmt"
	"strings"
)

// MockCommandExecutor implements CommandExecutor for testing
type MockCommandExecutor struct {
	CommandResults map[string]struct {
		Output string
		Error  error
	}
	ExecutedCommands []string
}

// NewMockCommandExecutor creates a new mock executor
func NewMockCommandExecutor() *MockCommandExecutor {
	return &MockCommandExecutor{
		CommandResults: make(map[string]struct {
			Output string
			Error  error
		}),
		ExecutedCommands: []string{},
	}
}

// AddCommandResult adds an expected result for a command
func (m *MockCommandExecutor) AddCommandResult(command string, output string, err error) {
	m.CommandResults[command] = struct {
		Output string
		Error  error
	}{Output: output, Error: err}
}

// Execute implements CommandExecutor.Execute for testing
func (m *MockCommandExecutor) Execute(command string, args ...string) (string, error) {
	cmdString := buildCommandString(command, args...)
	m.ExecutedCommands = append(m.ExecutedCommands, cmdString)

	if result, ok := m.CommandResults[cmdString]; ok {
		return result.Output, result.Error
	}

	return "", fmt.Errorf("mock: unexpected command: %s", cmdString)
}

// ExecuteWithInput implements CommandExecutor.ExecuteWithInput for testing
func (m *MockCommandExecutor) ExecuteWithInput(command string, input string, args ...string) (string, error) {
	cmdString := buildCommandString(command, args...)
	m.ExecutedCommands = append(m.ExecutedCommands, cmdString)

	if result, ok := m.CommandResults[cmdString]; ok {
		return result.Output, result.Error
	}

	return "", fmt.Errorf("mock: unexpected command with input: %s", cmdString)
}

// ExecuteWithEnv implements CommandExecutor.ExecuteWithEnv for testing
func (m *MockCommandExecutor) ExecuteWithEnv(command string, env map[string]string, args ...string) (string, error) {
	cmdString := buildCommandString(command, args...)
	m.ExecutedCommands = append(m.ExecutedCommands, cmdString)

	if result, ok := m.CommandResults[cmdString]; ok {
		return result.Output, result.Error
	}

	return "", fmt.Errorf("mock: unexpected command with env: %s", cmdString)
}

// Helper to build command string
func buildCommandString(command string, args ...string) string {
	return command + " " + strings.Join(args, " ")
}
