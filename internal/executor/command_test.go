package executor

import (
	"errors"
	"strings"
	"testing"
)

func TestMockCommandExecutor_Execute(t *testing.T) {
	tests := []struct {
		name          string
		command       string
		args          []string
		setupMock     func(*MockCommandExecutor)
		expectedOut   string
		expectedError bool
	}{
		{
			name:    "successful command execution",
			command: "kubectl",
			args:    []string{"get", "pods"},
			setupMock: func(m *MockCommandExecutor) {
				m.AddCommandResult("kubectl get pods", "pod1 Running\npod2 Running", nil)
			},
			expectedOut:   "pod1 Running\npod2 Running",
			expectedError: false,
		},
		{
			name:    "command execution with error",
			command: "kubectl",
			args:    []string{"get", "pods", "--invalid-flag"},
			setupMock: func(m *MockCommandExecutor) {
				m.AddCommandResult(
					"kubectl get pods --invalid-flag",
					"",
					errors.New("error: unknown flag: --invalid-flag"),
				)
			},
			expectedOut:   "",
			expectedError: true,
		},
		{
			name:    "unexpected command",
			command: "kubectl",
			args:    []string{"get", "deployments"},
			setupMock: func(m *MockCommandExecutor) {
				// Not setting up this command, should return an error
			},
			expectedOut:   "",
			expectedError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock executor
			mockExecutor := NewMockCommandExecutor()

			// Setup mock
			tt.setupMock(mockExecutor)

			// Execute command
			out, err := mockExecutor.Execute(tt.command, tt.args...)

			// Check results
			if tt.expectedError {
				if err == nil {
					t.Errorf("expected error but got nil")
				}
			} else {
				if err != nil {
					t.Errorf("expected no error but got: %v", err)
				}
			}

			if out != tt.expectedOut {
				t.Errorf("expected output %q but got %q", tt.expectedOut, out)
			}

			// Verify the command was executed
			cmdString := tt.command + " " + strings.Join(tt.args, " ")
			found := false
			for _, cmd := range mockExecutor.ExecutedCommands {
				if cmd == cmdString {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("command %q was not executed", cmdString)
			}
		})
	}
}

func TestMockCommandExecutor_ExecuteWithInput(t *testing.T) {
	tests := []struct {
		name          string
		command       string
		input         string
		args          []string
		setupMock     func(*MockCommandExecutor)
		expectedOut   string
		expectedError bool
	}{
		{
			name:    "successful command execution with input",
			command: "kubectl",
			input:   "sample yaml input",
			args:    []string{"apply", "-f", "-"},
			setupMock: func(m *MockCommandExecutor) {
				m.AddCommandResult("kubectl apply -f -", "deployment.apps/test created", nil)
			},
			expectedOut:   "deployment.apps/test created",
			expectedError: false,
		},
		{
			name:    "command execution with input and error",
			command: "kubectl",
			input:   "invalid yaml",
			args:    []string{"apply", "-f", "-"},
			setupMock: func(m *MockCommandExecutor) {
				m.AddCommandResult(
					"kubectl apply -f -",
					"",
					errors.New("error: error parsing YAML: yaml: line 1: mapping values are not allowed in this context"),
				)
			},
			expectedOut:   "",
			expectedError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock executor
			mockExecutor := NewMockCommandExecutor()

			// Setup mock
			tt.setupMock(mockExecutor)

			// Execute command with input
			out, err := mockExecutor.ExecuteWithInput(tt.command, tt.input, tt.args...)

			// Check results
			if tt.expectedError {
				if err == nil {
					t.Errorf("expected error but got nil")
				}
			} else {
				if err != nil {
					t.Errorf("expected no error but got: %v", err)
				}
			}

			if out != tt.expectedOut {
				t.Errorf("expected output %q but got %q", tt.expectedOut, out)
			}

			// Verify the command was executed
			cmdString := tt.command + " " + strings.Join(tt.args, " ")
			found := false
			for _, cmd := range mockExecutor.ExecutedCommands {
				if cmd == cmdString {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("command %q was not executed", cmdString)
			}
		})
	}
}

func TestMockCommandExecutor_ExecuteWithEnv(t *testing.T) {
	tests := []struct {
		name          string
		command       string
		env           map[string]string
		args          []string
		setupMock     func(*MockCommandExecutor)
		expectedOut   string
		expectedError bool
	}{
		{
			name:    "successful command execution with env",
			command: "kubectl",
			env:     map[string]string{"KUBECONFIG": "/path/to/kubeconfig"},
			args:    []string{"get", "pods"},
			setupMock: func(m *MockCommandExecutor) {
				m.AddCommandResult("kubectl get pods", "pod1 Running\npod2 Running", nil)
			},
			expectedOut:   "pod1 Running\npod2 Running",
			expectedError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock executor
			mockExecutor := NewMockCommandExecutor()

			// Setup mock
			tt.setupMock(mockExecutor)

			// Execute command with env
			out, err := mockExecutor.ExecuteWithEnv(tt.command, tt.env, tt.args...)

			// Check results
			if tt.expectedError {
				if err == nil {
					t.Errorf("expected error but got nil")
				}
			} else {
				if err != nil {
					t.Errorf("expected no error but got: %v", err)
				}
			}

			if out != tt.expectedOut {
				t.Errorf("expected output %q but got %q", tt.expectedOut, out)
			}

			// Verify the command was executed
			cmdString := tt.command + " " + strings.Join(tt.args, " ")
			found := false
			for _, cmd := range mockExecutor.ExecutedCommands {
				if cmd == cmdString {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("command %q was not executed", cmdString)
			}
		})
	}
}
