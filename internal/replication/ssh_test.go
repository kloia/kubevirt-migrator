package replication

import (
	"errors"
	"fmt"
	"testing"

	"go.uber.org/zap"

	"github.com/kloia/kubevirt-migrator/internal/config"
)

// CommandResult represents a mocked command execution result
type CommandResult struct {
	Output string
	Error  error
}

func TestSSHManager_GenerateKeys(t *testing.T) {
	// Test both kubectl and oc CLI
	kubeCLIs := []string{"kubectl", "oc"}

	for _, kubeCLI := range kubeCLIs {
		t.Run(fmt.Sprintf("GenerateKeys with %s", kubeCLI), func(t *testing.T) {
			// Create a sample config
			testCfg := &config.Config{
				VMName:        "test-vm",
				Namespace:     "test-namespace",
				KubeCLI:       kubeCLI,
				SrcKubeconfig: "src-kubeconfig.yaml",
				DstKubeconfig: "dst-kubeconfig.yaml",
			}

			tests := []struct {
				name             string
				mockedResponses  map[string]CommandResult
				expectedError    bool
				expectedCommands []string
			}{
				{
					name: "successful key generation",
					mockedResponses: map[string]CommandResult{
						// Check if keys exist - return NOT_EXISTS
						fmt.Sprintf("%s exec test-vm-src-replicator -n test-namespace --kubeconfig src-kubeconfig.yaml -- bash -c if [[ -f ~/.ssh/id_rsa && -f ~/.ssh/id_rsa.pub ]]; then echo 'EXISTS'; else echo 'NOT_EXISTS'; fi", kubeCLI): {
							Output: "NOT_EXISTS",
							Error:  nil,
						},
						// Clean up existing partial keys
						fmt.Sprintf("%s exec test-vm-src-replicator -n test-namespace --kubeconfig src-kubeconfig.yaml -- bash -c rm -f ~/.ssh/id_rsa ~/.ssh/id_rsa.pub", kubeCLI): {
							Output: "",
							Error:  nil,
						},
						// Generate SSH keys
						fmt.Sprintf("%s exec test-vm-src-replicator -n test-namespace --kubeconfig src-kubeconfig.yaml -- bash -c mkdir -p ~/.ssh && ssh-keygen -t rsa -b 4096 -N '' -f ~/.ssh/id_rsa", kubeCLI): {
							Output: "Generated keys successfully",
							Error:  nil,
						},
						// Get private key
						fmt.Sprintf("%s exec test-vm-src-replicator -n test-namespace --kubeconfig src-kubeconfig.yaml -- cat /root/.ssh/id_rsa", kubeCLI): {
							Output: "-----BEGIN RSA PRIVATE KEY----- mock private key -----END RSA PRIVATE KEY-----",
							Error:  nil,
						},
						// Get public key
						fmt.Sprintf("%s exec test-vm-src-replicator -n test-namespace --kubeconfig src-kubeconfig.yaml -- cat /root/.ssh/id_rsa.pub", kubeCLI): {
							Output: "ssh-rsa AAAA... mock public key",
							Error:  nil,
						},
						// Check if secret exists
						fmt.Sprintf("%s get secret test-vm-repl-ssh-keys -n test-namespace --kubeconfig src-kubeconfig.yaml", kubeCLI): {
							Output: "",
							Error:  errors.New("secret not found"),
						},
						// Create secret YAML
						fmt.Sprintf("%s create secret generic test-vm-repl-ssh-keys --from-file=id_rsa=/tmp/test-vm-id_rsa --from-file=id_rsa.pub=/tmp/test-vm-id_rsa.pub -n test-namespace --save-config --dry-run=client -o yaml", kubeCLI): {
							Output: "apiVersion: v1\nkind: Secret\nmetadata:\n  name: test-vm-repl-ssh-keys\n  namespace: test-namespace\ndata:\n  id_rsa: base64-encoded-private-key\n  id_rsa.pub: base64-encoded-public-key\ntype: Opaque",
							Error:  nil,
						},
						// Apply secret
						fmt.Sprintf("%s apply -f /tmp/test-vm-ssh-secret.yaml -n test-namespace --kubeconfig src-kubeconfig.yaml", kubeCLI): {
							Output: "secret/test-vm-repl-ssh-keys created",
							Error:  nil,
						},
						// Verify secret creation
						fmt.Sprintf("%s get secret test-vm-repl-ssh-keys -n test-namespace --kubeconfig src-kubeconfig.yaml verify", kubeCLI): {
							Output: "NAME                    TYPE     DATA   AGE\ntest-vm-repl-ssh-keys   Opaque   2      1s",
							Error:  nil,
						},
						// Add mock for bash commands used in writeKeyFile
						"bash -c echo '-----BEGIN RSA PRIVATE KEY----- mock private key -----END RSA PRIVATE KEY-----' > /tmp/test-vm-id_rsa": {
							Output: "",
							Error:  nil,
						},
						"bash -c echo 'ssh-rsa AAAA... mock public key' > /tmp/test-vm-id_rsa.pub": {
							Output: "",
							Error:  nil,
						},
					},
					expectedError: false,
					expectedCommands: []string{
						fmt.Sprintf("%s exec test-vm-src-replicator -n test-namespace --kubeconfig src-kubeconfig.yaml -- bash -c if [[ -f ~/.ssh/id_rsa && -f ~/.ssh/id_rsa.pub ]]; then echo 'EXISTS'; else echo 'NOT_EXISTS'; fi", kubeCLI),
						fmt.Sprintf("%s exec test-vm-src-replicator -n test-namespace --kubeconfig src-kubeconfig.yaml -- bash -c rm -f ~/.ssh/id_rsa ~/.ssh/id_rsa.pub", kubeCLI),
						fmt.Sprintf("%s exec test-vm-src-replicator -n test-namespace --kubeconfig src-kubeconfig.yaml -- bash -c mkdir -p ~/.ssh && ssh-keygen -t rsa -b 4096 -N '' -f ~/.ssh/id_rsa", kubeCLI),
						fmt.Sprintf("%s exec test-vm-src-replicator -n test-namespace --kubeconfig src-kubeconfig.yaml -- cat /root/.ssh/id_rsa", kubeCLI),
						fmt.Sprintf("%s exec test-vm-src-replicator -n test-namespace --kubeconfig src-kubeconfig.yaml -- cat /root/.ssh/id_rsa.pub", kubeCLI),
					},
				},
				{
					name: "keys already exist",
					mockedResponses: map[string]CommandResult{
						// Check if keys exist - return EXISTS
						fmt.Sprintf("%s exec test-vm-src-replicator -n test-namespace --kubeconfig src-kubeconfig.yaml -- bash -c if [[ -f ~/.ssh/id_rsa && -f ~/.ssh/id_rsa.pub ]]; then echo 'EXISTS'; else echo 'NOT_EXISTS'; fi", kubeCLI): {
							Output: "EXISTS",
							Error:  nil,
						},
						// Get private key
						fmt.Sprintf("%s exec test-vm-src-replicator -n test-namespace --kubeconfig src-kubeconfig.yaml -- cat /root/.ssh/id_rsa", kubeCLI): {
							Output: "-----BEGIN RSA PRIVATE KEY----- mock private key -----END RSA PRIVATE KEY-----",
							Error:  nil,
						},
						// Get public key
						fmt.Sprintf("%s exec test-vm-src-replicator -n test-namespace --kubeconfig src-kubeconfig.yaml -- cat /root/.ssh/id_rsa.pub", kubeCLI): {
							Output: "ssh-rsa AAAA... mock public key",
							Error:  nil,
						},
						// Check if secret exists
						fmt.Sprintf("%s get secret test-vm-repl-ssh-keys -n test-namespace --kubeconfig src-kubeconfig.yaml", kubeCLI): {
							Output: "",
							Error:  errors.New("secret not found"),
						},
						// Create secret YAML
						fmt.Sprintf("%s create secret generic test-vm-repl-ssh-keys --from-file=id_rsa=/tmp/test-vm-id_rsa --from-file=id_rsa.pub=/tmp/test-vm-id_rsa.pub -n test-namespace --save-config --dry-run=client -o yaml", kubeCLI): {
							Output: "apiVersion: v1\nkind: Secret\nmetadata:\n  name: test-vm-repl-ssh-keys\n  namespace: test-namespace\ndata:\n  id_rsa: base64-encoded-private-key\n  id_rsa.pub: base64-encoded-public-key\ntype: Opaque",
							Error:  nil,
						},
						// Apply secret
						fmt.Sprintf("%s apply -f /tmp/test-vm-ssh-secret.yaml -n test-namespace --kubeconfig src-kubeconfig.yaml", kubeCLI): {
							Output: "secret/test-vm-repl-ssh-keys created",
							Error:  nil,
						},
						// Verify secret creation
						fmt.Sprintf("%s get secret test-vm-repl-ssh-keys -n test-namespace --kubeconfig src-kubeconfig.yaml", kubeCLI): {
							Output: "NAME                    TYPE     DATA   AGE\ntest-vm-repl-ssh-keys   Opaque   2      1s",
							Error:  nil,
						},
						// Add mock for bash commands used in writeKeyFile
						"bash -c echo '-----BEGIN RSA PRIVATE KEY----- mock private key -----END RSA PRIVATE KEY-----' > /tmp/test-vm-id_rsa": {
							Output: "",
							Error:  nil,
						},
						"bash -c echo 'ssh-rsa AAAA... mock public key' > /tmp/test-vm-id_rsa.pub": {
							Output: "",
							Error:  nil,
						},
					},
					expectedError: false,
					expectedCommands: []string{
						fmt.Sprintf("%s exec test-vm-src-replicator -n test-namespace --kubeconfig src-kubeconfig.yaml -- bash -c if [[ -f ~/.ssh/id_rsa && -f ~/.ssh/id_rsa.pub ]]; then echo 'EXISTS'; else echo 'NOT_EXISTS'; fi", kubeCLI),
						fmt.Sprintf("%s exec test-vm-src-replicator -n test-namespace --kubeconfig src-kubeconfig.yaml -- cat /root/.ssh/id_rsa", kubeCLI),
						fmt.Sprintf("%s exec test-vm-src-replicator -n test-namespace --kubeconfig src-kubeconfig.yaml -- cat /root/.ssh/id_rsa.pub", kubeCLI),
					},
				},
				{
					name: "error generating keys",
					mockedResponses: map[string]CommandResult{
						// Check if keys exist - return NOT_EXISTS
						fmt.Sprintf("%s exec test-vm-src-replicator -n test-namespace --kubeconfig src-kubeconfig.yaml -- bash -c if [[ -f ~/.ssh/id_rsa && -f ~/.ssh/id_rsa.pub ]]; then echo 'EXISTS'; else echo 'NOT_EXISTS'; fi", kubeCLI): {
							Output: "NOT_EXISTS",
							Error:  nil,
						},
						// Clean up existing partial keys
						fmt.Sprintf("%s exec test-vm-src-replicator -n test-namespace --kubeconfig src-kubeconfig.yaml -- bash -c rm -f ~/.ssh/id_rsa ~/.ssh/id_rsa.pub", kubeCLI): {
							Output: "",
							Error:  nil,
						},
						// Generate SSH keys - returns error
						fmt.Sprintf("%s exec test-vm-src-replicator -n test-namespace --kubeconfig src-kubeconfig.yaml -- bash -c mkdir -p ~/.ssh && ssh-keygen -t rsa -b 4096 -N '' -f ~/.ssh/id_rsa", kubeCLI): {
							Output: "",
							Error:  errors.New("failed to generate SSH keys"),
						},
					},
					expectedError: true,
					expectedCommands: []string{
						fmt.Sprintf("%s exec test-vm-src-replicator -n test-namespace --kubeconfig src-kubeconfig.yaml -- bash -c if [[ -f ~/.ssh/id_rsa && -f ~/.ssh/id_rsa.pub ]]; then echo 'EXISTS'; else echo 'NOT_EXISTS'; fi", kubeCLI),
						fmt.Sprintf("%s exec test-vm-src-replicator -n test-namespace --kubeconfig src-kubeconfig.yaml -- bash -c rm -f ~/.ssh/id_rsa ~/.ssh/id_rsa.pub", kubeCLI),
						fmt.Sprintf("%s exec test-vm-src-replicator -n test-namespace --kubeconfig src-kubeconfig.yaml -- bash -c mkdir -p ~/.ssh && ssh-keygen -t rsa -b 4096 -N '' -f ~/.ssh/id_rsa", kubeCLI),
					},
				},
				{
					name: "error writing key file",
					mockedResponses: map[string]CommandResult{
						// Check if keys exist - return NOT_EXISTS
						fmt.Sprintf("%s exec test-vm-src-replicator -n test-namespace --kubeconfig src-kubeconfig.yaml -- bash -c if [[ -f ~/.ssh/id_rsa && -f ~/.ssh/id_rsa.pub ]]; then echo 'EXISTS'; else echo 'NOT_EXISTS'; fi", kubeCLI): {
							Output: "NOT_EXISTS",
							Error:  nil,
						},
						// Clean up existing partial keys
						fmt.Sprintf("%s exec test-vm-src-replicator -n test-namespace --kubeconfig src-kubeconfig.yaml -- bash -c rm -f ~/.ssh/id_rsa ~/.ssh/id_rsa.pub", kubeCLI): {
							Output: "",
							Error:  nil,
						},
						// Generate SSH keys
						fmt.Sprintf("%s exec test-vm-src-replicator -n test-namespace --kubeconfig src-kubeconfig.yaml -- bash -c mkdir -p ~/.ssh && ssh-keygen -t rsa -b 4096 -N '' -f ~/.ssh/id_rsa", kubeCLI): {
							Output: "Generated keys successfully",
							Error:  nil,
						},
						// Get private key
						fmt.Sprintf("%s exec test-vm-src-replicator -n test-namespace --kubeconfig src-kubeconfig.yaml -- cat /root/.ssh/id_rsa", kubeCLI): {
							Output: "-----BEGIN RSA PRIVATE KEY----- mock private key -----END RSA PRIVATE KEY-----",
							Error:  nil,
						},
						// Get public key
						fmt.Sprintf("%s exec test-vm-src-replicator -n test-namespace --kubeconfig src-kubeconfig.yaml -- cat /root/.ssh/id_rsa.pub", kubeCLI): {
							Output: "ssh-rsa AAAA... mock public key",
							Error:  nil,
						},
						// Add mock for bash commands used in writeKeyFile - with error
						"bash -c echo '-----BEGIN RSA PRIVATE KEY----- mock private key -----END RSA PRIVATE KEY-----' > /tmp/test-vm-id_rsa": {
							Output: "",
							Error:  errors.New("failed to write key file"),
						},
					},
					expectedError: true,
					expectedCommands: []string{
						fmt.Sprintf("%s exec test-vm-src-replicator -n test-namespace --kubeconfig src-kubeconfig.yaml -- bash -c if [[ -f ~/.ssh/id_rsa && -f ~/.ssh/id_rsa.pub ]]; then echo 'EXISTS'; else echo 'NOT_EXISTS'; fi", kubeCLI),
						fmt.Sprintf("%s exec test-vm-src-replicator -n test-namespace --kubeconfig src-kubeconfig.yaml -- bash -c rm -f ~/.ssh/id_rsa ~/.ssh/id_rsa.pub", kubeCLI),
						fmt.Sprintf("%s exec test-vm-src-replicator -n test-namespace --kubeconfig src-kubeconfig.yaml -- bash -c mkdir -p ~/.ssh && ssh-keygen -t rsa -b 4096 -N '' -f ~/.ssh/id_rsa", kubeCLI),
						fmt.Sprintf("%s exec test-vm-src-replicator -n test-namespace --kubeconfig src-kubeconfig.yaml -- cat /root/.ssh/id_rsa", kubeCLI),
						fmt.Sprintf("%s exec test-vm-src-replicator -n test-namespace --kubeconfig src-kubeconfig.yaml -- cat /root/.ssh/id_rsa.pub", kubeCLI),
					},
				},
			}

			for _, tt := range tests {
				t.Run(tt.name, func(t *testing.T) {
					// Create mock executor
					mockExecutor := &MockExecutor{
						CommandResults: tt.mockedResponses,
					}

					// Create test logger
					logger, _ := zap.NewDevelopment()

					// Create SSH manager with mock executor
					sshManager := NewSSHManager(mockExecutor, logger)

					// Call the function
					err := sshManager.GenerateKeys(testCfg)

					// Check error
					if tt.expectedError && err == nil {
						t.Errorf("expected error but got nil")
					}
					if !tt.expectedError && err != nil {
						t.Errorf("expected no error but got: %v", err)
					}

					// Check commands executed
					for _, cmd := range tt.expectedCommands {
						found := false
						for _, executed := range mockExecutor.ExecutedCommands {
							if executed == cmd {
								found = true
								break
							}
						}
						if !found {
							t.Errorf("expected command not executed: %s", cmd)
						}
					}
				})
			}
		})
	}
}

func TestSSHManager_SetupDestinationAuth(t *testing.T) {
	// Test both kubectl and oc CLI
	kubeCLIs := []string{"kubectl", "oc"}

	for _, kubeCLI := range kubeCLIs {
		t.Run(fmt.Sprintf("SetupDestinationAuth with %s", kubeCLI), func(t *testing.T) {
			// Create a sample config
			testCfg := &config.Config{
				VMName:        "test-vm",
				Namespace:     "test-namespace",
				KubeCLI:       kubeCLI,
				SrcKubeconfig: "src-kubeconfig.yaml",
				DstKubeconfig: "dst-kubeconfig.yaml",
			}

			tests := []struct {
				name             string
				mockedResponses  map[string]CommandResult
				expectedError    bool
				expectedCommands []string
			}{
				{
					name: "successful setup",
					mockedResponses: map[string]CommandResult{
						// Get public key
						fmt.Sprintf("%s exec test-vm-src-replicator -n test-namespace --kubeconfig src-kubeconfig.yaml -- cat /root/.ssh/id_rsa.pub", kubeCLI): {
							Output: "ssh-rsa AAAA... mock public key",
							Error:  nil,
						},
						// Setup auth on destination
						fmt.Sprintf("%s exec test-vm-dst-replicator -n test-namespace --kubeconfig dst-kubeconfig.yaml -- bash -c mkdir -p ~/.ssh && echo 'ssh-rsa AAAA... mock public key' > ~/.ssh/authorized_keys && chmod 600 ~/.ssh/authorized_keys", kubeCLI): {
							Output: "",
							Error:  nil,
						},
					},
					expectedError: false,
					expectedCommands: []string{
						fmt.Sprintf("%s exec test-vm-src-replicator -n test-namespace --kubeconfig src-kubeconfig.yaml -- cat /root/.ssh/id_rsa.pub", kubeCLI),
						fmt.Sprintf("%s exec test-vm-dst-replicator -n test-namespace --kubeconfig dst-kubeconfig.yaml -- bash -c mkdir -p ~/.ssh && echo 'ssh-rsa AAAA... mock public key' > ~/.ssh/authorized_keys && chmod 600 ~/.ssh/authorized_keys", kubeCLI),
					},
				},
				{
					name: "error getting public key",
					mockedResponses: map[string]CommandResult{
						// Get public key - returns error
						fmt.Sprintf("%s exec test-vm-src-replicator -n test-namespace --kubeconfig src-kubeconfig.yaml -- cat /root/.ssh/id_rsa.pub", kubeCLI): {
							Output: "",
							Error:  errors.New("failed to get public key"),
						},
					},
					expectedError: true,
					expectedCommands: []string{
						fmt.Sprintf("%s exec test-vm-src-replicator -n test-namespace --kubeconfig src-kubeconfig.yaml -- cat /root/.ssh/id_rsa.pub", kubeCLI),
					},
				},
				{
					name: "error setting up auth on destination",
					mockedResponses: map[string]CommandResult{
						// Get public key
						fmt.Sprintf("%s exec test-vm-src-replicator -n test-namespace --kubeconfig src-kubeconfig.yaml -- cat /root/.ssh/id_rsa.pub", kubeCLI): {
							Output: "ssh-rsa AAAA... mock public key",
							Error:  nil,
						},
						// Setup auth on destination - returns error
						fmt.Sprintf("%s exec test-vm-dst-replicator -n test-namespace --kubeconfig dst-kubeconfig.yaml -- bash -c mkdir -p ~/.ssh && echo 'ssh-rsa AAAA... mock public key' > ~/.ssh/authorized_keys && chmod 600 ~/.ssh/authorized_keys", kubeCLI): {
							Output: "",
							Error:  errors.New("failed to set up auth"),
						},
					},
					expectedError: true,
					expectedCommands: []string{
						fmt.Sprintf("%s exec test-vm-src-replicator -n test-namespace --kubeconfig src-kubeconfig.yaml -- cat /root/.ssh/id_rsa.pub", kubeCLI),
						fmt.Sprintf("%s exec test-vm-dst-replicator -n test-namespace --kubeconfig dst-kubeconfig.yaml -- bash -c mkdir -p ~/.ssh && echo 'ssh-rsa AAAA... mock public key' > ~/.ssh/authorized_keys && chmod 600 ~/.ssh/authorized_keys", kubeCLI),
					},
				},
			}

			for _, tt := range tests {
				t.Run(tt.name, func(t *testing.T) {
					// Create mock executor
					mockExecutor := &MockExecutor{
						CommandResults: tt.mockedResponses,
					}

					// Create test logger
					logger, _ := zap.NewDevelopment()

					// Create SSH manager with mock executor
					sshManager := NewSSHManager(mockExecutor, logger)

					// Call the function
					err := sshManager.SetupDestinationAuth(testCfg)

					// Check error
					if tt.expectedError && err == nil {
						t.Errorf("expected error but got nil")
					}
					if !tt.expectedError && err != nil {
						t.Errorf("expected no error but got: %v", err)
					}

					// Check commands executed
					for _, cmd := range tt.expectedCommands {
						found := false
						for _, executed := range mockExecutor.ExecutedCommands {
							if executed == cmd {
								found = true
								break
							}
						}
						if !found {
							t.Errorf("expected command not executed: %s", cmd)
						}
					}
				})
			}
		})
	}
}

// MockExecutor implements a mock CommandExecutor for testing
type MockExecutor struct {
	CommandResults   map[string]CommandResult
	ExecutedCommands []string
}

// Execute mocks the command execution
func (m *MockExecutor) Execute(command string, args ...string) (string, error) {
	cmdStr := command
	for _, arg := range args {
		cmdStr += " " + arg
	}
	m.ExecutedCommands = append(m.ExecutedCommands, cmdStr)

	if result, ok := m.CommandResults[cmdStr]; ok {
		return result.Output, result.Error
	}

	return "", errors.New("command not mocked: " + cmdStr)
}

// ExecuteWithInput mocks command execution with input
func (m *MockExecutor) ExecuteWithInput(command string, input string, args ...string) (string, error) {
	cmdStr := command
	for _, arg := range args {
		cmdStr += " " + arg
	}
	m.ExecutedCommands = append(m.ExecutedCommands, cmdStr)

	if result, ok := m.CommandResults[cmdStr]; ok {
		return result.Output, result.Error
	}

	return "", errors.New("command with input not mocked: " + cmdStr)
}

// ExecuteWithEnv mocks command execution with environment variables
func (m *MockExecutor) ExecuteWithEnv(command string, env map[string]string, args ...string) (string, error) {
	cmdStr := command
	for _, arg := range args {
		cmdStr += " " + arg
	}
	m.ExecutedCommands = append(m.ExecutedCommands, cmdStr)

	if result, ok := m.CommandResults[cmdStr]; ok {
		return result.Output, result.Error
	}

	return "", errors.New("command with env not mocked: " + cmdStr)
}
