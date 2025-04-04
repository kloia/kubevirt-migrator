package replication

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/kloia/kubevirt-migrator/internal/config"
	"github.com/kloia/kubevirt-migrator/internal/executor"
)

// SSHManager handles SSH key operations
type SSHManager struct {
	executor executor.CommandExecutor
	logger   *zap.Logger
}

// NewSSHManager creates a new SSH key manager
func NewSSHManager(executor executor.CommandExecutor, logger *zap.Logger) *SSHManager {
	return &SSHManager{
		executor: executor,
		logger:   logger,
	}
}

// GenerateKeys generates SSH keys on the source replicator pod
func (s *SSHManager) GenerateKeys(cfg *config.Config) error {
	podName := fmt.Sprintf("%s-src-replicator", cfg.VMName)

	// First check if SSH keys already exist in the pod - this is expected to "fail" when keys don't exist
	output, err := s.executor.Execute(cfg.KubeCLI, "exec", podName, "-n", cfg.Namespace, "--kubeconfig", cfg.SrcKubeconfig,
		"--", "bash", "-c", "if [[ -f ~/.ssh/id_rsa && -f ~/.ssh/id_rsa.pub ]]; then echo 'EXISTS'; else echo 'NOT_EXISTS'; fi")
	if err != nil {
		// If we can't even check, assume keys don't exist
		s.logger.Warn("Failed to check for existing SSH keys, will create new ones",
			zap.String("pod", podName), zap.Error(err))
		output = "NOT_EXISTS"
	}

	keysExist := strings.TrimSpace(output) == "EXISTS"

	if keysExist {
		// SSH keys already exist, use them instead of regenerating
		s.logger.Info("SSH keys already exist in the pod, skipping generation step",
			zap.String("pod", podName))
	} else {
		s.logger.Info("SSH keys don't exist or are incomplete, will generate new ones",
			zap.String("pod", podName))

		// First remove any existing keys to prevent interactive prompts
		s.logger.Info("Ensuring no existing SSH keys are present", zap.String("pod", podName))
		_, cleanErr := s.executor.Execute(cfg.KubeCLI, "exec", podName, "-n", cfg.Namespace, "--kubeconfig", cfg.SrcKubeconfig,
			"--", "bash", "-c", "rm -f ~/.ssh/id_rsa ~/.ssh/id_rsa.pub")
		if cleanErr != nil {
			s.logger.Warn("Failed to clean existing SSH keys, but will continue",
				zap.String("pod", podName), zap.Error(cleanErr))
		}

		// Generate SSH keys with -f (force) flag to avoid any prompts
		s.logger.Info("Generating new SSH keys", zap.String("pod", podName))
		_, err := s.executor.Execute(cfg.KubeCLI, "exec", podName, "-n", cfg.Namespace, "--kubeconfig", cfg.SrcKubeconfig,
			"--", "bash", "-c", "mkdir -p ~/.ssh && ssh-keygen -t rsa -b 4096 -N '' -f ~/.ssh/id_rsa")
		if err != nil {
			return fmt.Errorf("failed to generate SSH keys: %w", err)
		}
	}

	// Create secret with SSH keys
	s.logger.Info("Copying SSH keys to create secret")

	// Get private key
	privateKey, err := s.executor.Execute(cfg.KubeCLI, "exec", podName, "-n", cfg.Namespace, "--kubeconfig", cfg.SrcKubeconfig,
		"--", "cat", "/root/.ssh/id_rsa")
	if err != nil {
		return fmt.Errorf("failed to get private key: %w", err)
	}

	// Get public key
	publicKey, err := s.executor.Execute(cfg.KubeCLI, "exec", podName, "-n", cfg.Namespace, "--kubeconfig", cfg.SrcKubeconfig,
		"--", "cat", "/root/.ssh/id_rsa.pub")
	if err != nil {
		return fmt.Errorf("failed to get public key: %w", err)
	}

	// Create temp files for keys
	privateKeyFile := filepath.Join("/tmp", fmt.Sprintf("%s-id_rsa", cfg.VMName))
	publicKeyFile := filepath.Join("/tmp", fmt.Sprintf("%s-id_rsa.pub", cfg.VMName))

	if err := s.writeKeyFile(privateKeyFile, privateKey); err != nil {
		return err
	}

	if err := s.writeKeyFile(publicKeyFile, publicKey); err != nil {
		return err
	}

	// Create secret
	secretName := fmt.Sprintf("%s-repl-ssh-keys", cfg.VMName)

	// First check if secret already exists and delete it if it does
	checkCmd := []string{"get", "secret", secretName, "-n", cfg.Namespace, "--kubeconfig", cfg.SrcKubeconfig}
	_, checkErr := s.executor.Execute(cfg.KubeCLI, checkCmd...)
	if checkErr == nil {
		// Secret exists, delete it first for clean recreation
		s.logger.Info("Secret already exists, deleting it for recreation", zap.String("secret", secretName))
		deleteCmd := []string{"delete", "secret", secretName, "-n", cfg.Namespace, "--kubeconfig", cfg.SrcKubeconfig}
		_, deleteErr := s.executor.Execute(cfg.KubeCLI, deleteCmd...)
		if deleteErr != nil {
			s.logger.Warn("Failed to delete existing secret, will try to replace it",
				zap.String("secret", secretName), zap.Error(deleteErr))
		}
	}

	// Create the secret YAML without shell redirection
	tempSecretFile := filepath.Join("/tmp", fmt.Sprintf("%s-ssh-secret.yaml", cfg.VMName))
	yamlOutput, yamlErr := s.executor.Execute(cfg.KubeCLI, "create", "secret", "generic", secretName,
		"--from-file=id_rsa="+privateKeyFile,
		"--from-file=id_rsa.pub="+publicKeyFile,
		"-n", cfg.Namespace,
		"--save-config", "--dry-run=client", "-o", "yaml")

	if yamlErr != nil {
		return fmt.Errorf("failed to generate secret YAML: %w", yamlErr)
	}

	// Write the YAML to a temporary file
	if err := os.WriteFile(tempSecretFile, []byte(yamlOutput), 0600); err != nil {
		return fmt.Errorf("failed to write secret YAML to file: %w", err)
	}

	// Apply the secret using the temporary file
	_, err = s.executor.Execute(cfg.KubeCLI, "apply", "-f", tempSecretFile,
		"-n", cfg.Namespace, "--kubeconfig", cfg.SrcKubeconfig)

	// Cleanup temp file
	if rmErr := os.Remove(tempSecretFile); rmErr != nil {
		s.logger.Warn("Failed to remove temporary secret file",
			zap.String("file", tempSecretFile), zap.Error(rmErr))
	}

	if err != nil {
		return fmt.Errorf("failed to create/apply secret: %w", err)
	}

	// Verify the secret was created with retries
	maxRetries := 3
	retryDelay := 5 * time.Second
	var lastError error

	for i := 0; i < maxRetries; i++ {
		_, verifyErr := s.executor.Execute(cfg.KubeCLI, "get", "secret", secretName,
			"-n", cfg.Namespace, "--kubeconfig", cfg.SrcKubeconfig)

		if verifyErr == nil {
			s.logger.Info("Secret verified successfully", zap.String("secret", secretName))
			return nil
		}

		lastError = verifyErr
		s.logger.Info("Waiting for secret to be available",
			zap.String("secret", secretName),
			zap.Int("retry", i+1),
			zap.Int("maxRetries", maxRetries),
			zap.Error(verifyErr))

		if i < maxRetries-1 {
			time.Sleep(retryDelay)
			// Exponential backoff
			retryDelay *= 2
		}
	}

	s.logger.Warn("Failed to verify secret after multiple attempts",
		zap.String("secret", secretName), zap.Error(lastError))

	// Continue execution even if secret verification fails
	// The process will either work (secret was created but verification had issues) or fail later with a clearer error
	return nil
}

// SetupDestinationAuth sets up SSH authorization on destination pod
func (s *SSHManager) SetupDestinationAuth(cfg *config.Config) error {
	srcPodName := fmt.Sprintf("%s-src-replicator", cfg.VMName)
	dstPodName := fmt.Sprintf("%s-dst-replicator", cfg.VMName)

	// Get public key from source pod
	publicKey, err := s.executor.Execute(cfg.KubeCLI, "exec", srcPodName, "-n", cfg.Namespace, "--kubeconfig", cfg.SrcKubeconfig,
		"--", "cat", "/root/.ssh/id_rsa.pub")
	if err != nil {
		return fmt.Errorf("failed to get public key: %w", err)
	}

	// Set up SSH auth on destination pod
	_, err = s.executor.Execute(cfg.KubeCLI, "exec", dstPodName, "-n", cfg.Namespace, "--kubeconfig", cfg.DstKubeconfig,
		"--", "bash", "-c", fmt.Sprintf("mkdir -p ~/.ssh && echo '%s' > ~/.ssh/authorized_keys && chmod 600 ~/.ssh/authorized_keys", publicKey))
	if err != nil {
		return fmt.Errorf("failed to set up SSH auth on destination: %w", err)
	}

	return nil
}

// writeKeyFile writes an SSH key to a file
func (s *SSHManager) writeKeyFile(filename, content string) error {
	cmd := fmt.Sprintf("echo '%s' > %s", content, filename)
	_, err := s.executor.Execute("bash", "-c", cmd)
	if err != nil {
		return fmt.Errorf("failed to write key file %s: %w", filename, err)
	}
	return nil
}
