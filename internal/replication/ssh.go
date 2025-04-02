package replication

import (
	"fmt"
	"path/filepath"

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

	// Generate SSH keys
	_, err := s.executor.Execute("oc", "exec", podName, "-n", cfg.Namespace, "--kubeconfig", cfg.SrcKubeconfig,
		"--", "bash", "-c", "ssh-keygen -t rsa -b 4096 -N '' -f ~/.ssh/id_rsa")
	if err != nil {
		return fmt.Errorf("failed to generate SSH keys: %w", err)
	}

	// Create secret with SSH keys
	s.logger.Info("Copying SSH keys to create secret")

	// Get private key
	privateKey, err := s.executor.Execute("oc", "exec", podName, "-n", cfg.Namespace, "--kubeconfig", cfg.SrcKubeconfig,
		"--", "cat", "/root/.ssh/id_rsa")
	if err != nil {
		return fmt.Errorf("failed to get private key: %w", err)
	}

	// Get public key
	publicKey, err := s.executor.Execute("oc", "exec", podName, "-n", cfg.Namespace, "--kubeconfig", cfg.SrcKubeconfig,
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
	_, err = s.executor.Execute("oc", "create", "secret", "generic", secretName,
		"--from-file=id_rsa="+privateKeyFile,
		"--from-file=id_rsa.pub="+publicKeyFile,
		"-n", cfg.Namespace,
		"--kubeconfig", cfg.SrcKubeconfig)
	if err != nil {
		return fmt.Errorf("failed to create secret: %w", err)
	}

	return nil
}

// SetupDestinationAuth sets up SSH authorization on destination pod
func (s *SSHManager) SetupDestinationAuth(cfg *config.Config) error {
	srcPodName := fmt.Sprintf("%s-src-replicator", cfg.VMName)
	dstPodName := fmt.Sprintf("%s-dst-replicator", cfg.VMName)

	// Get public key from source pod
	publicKey, err := s.executor.Execute("oc", "exec", srcPodName, "-n", cfg.Namespace, "--kubeconfig", cfg.SrcKubeconfig,
		"--", "cat", "/root/.ssh/id_rsa.pub")
	if err != nil {
		return fmt.Errorf("failed to get public key: %w", err)
	}

	// Set up SSH auth on destination pod
	_, err = s.executor.Execute("oc", "exec", dstPodName, "-n", cfg.Namespace, "--kubeconfig", cfg.DstKubeconfig,
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
