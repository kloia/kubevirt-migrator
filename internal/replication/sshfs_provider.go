package replication

import (
	"fmt"
	"strings"

	"go.uber.org/zap"

	"github.com/kloia/kubevirt-migrator/internal/config"
	"github.com/kloia/kubevirt-migrator/internal/executor"
)

// SSHFSProvider implements MountProvider using SSHFS technology
type SSHFSProvider struct {
	executor executor.CommandExecutor
	logger   *zap.Logger
}

// NewSSHFSProvider creates a new SSHFS-based mount provider
func NewSSHFSProvider(executor executor.CommandExecutor, logger *zap.Logger) *SSHFSProvider {
	return &SSHFSProvider{
		executor: executor,
		logger:   logger,
	}
}

// CheckConnectivity verifies basic connectivity to the destination
func (s *SSHFSProvider) CheckConnectivity(cfg *config.Config, hostIP, port string) error {
	s.logger.Info("Checking basic connectivity",
		zap.String("hostIP", hostIP),
		zap.String("port", port))

	// First try a basic TCP connection to verify network connectivity
	testCmd := fmt.Sprintf("timeout 5 bash -c '</dev/tcp/%s/%s' && echo 'Connection successful'",
		hostIP, port)

	s.logger.Debug("Running TCP connectivity test command",
		zap.String("command", testCmd),
		zap.String("pod", fmt.Sprintf("%s-src-replicator", cfg.VMName)),
		zap.String("namespace", cfg.Namespace))

	output, err := s.executor.Execute(cfg.KubeCLI, "exec",
		fmt.Sprintf("%s-src-replicator", cfg.VMName),
		"-n", cfg.Namespace, "--kubeconfig", cfg.SrcKubeconfig,
		"--", "bash", "-c", testCmd)

	if err != nil {
		s.logger.Warn("Basic connectivity test failed",
			zap.String("error", err.Error()),
			zap.String("output", output),
			zap.String("hostIP", hostIP),
			zap.String("port", port))
		return fmt.Errorf("TCP connectivity test failed: cannot connect to %s:%s: %v",
			hostIP, port, err)
	}

	s.logger.Info("Basic connectivity test successful")
	s.logger.Debug("TCP connectivity test output", zap.String("output", output))

	// Verify source directory permissions
	srcDirCmd := "mkdir -p /data/simg && touch /data/simg/test_write_perm && rm /data/simg/test_write_perm"

	s.logger.Debug("Testing source directory permissions",
		zap.String("command", srcDirCmd),
		zap.String("pod", fmt.Sprintf("%s-src-replicator", cfg.VMName)))

	output, err = s.executor.Execute(cfg.KubeCLI, "exec",
		fmt.Sprintf("%s-src-replicator", cfg.VMName),
		"-n", cfg.Namespace, "--kubeconfig", cfg.SrcKubeconfig,
		"--", "bash", "-c", srcDirCmd)

	if err != nil {
		s.logger.Error("Source directory permission check failed",
			zap.String("error", err.Error()),
			zap.String("output", output))
		return fmt.Errorf("source directory write permission check failed - cannot write to /data/simg: %w", err)
	}

	s.logger.Debug("Source directory permission check successful")

	// Check if sshfs command is available
	sshfsCheckCmd := "which sshfs || echo 'sshfs not found'"

	s.logger.Debug("Checking if SSHFS command is available",
		zap.String("command", sshfsCheckCmd),
		zap.String("pod", fmt.Sprintf("%s-src-replicator", cfg.VMName)))

	output, err = s.executor.Execute(cfg.KubeCLI, "exec",
		fmt.Sprintf("%s-src-replicator", cfg.VMName),
		"-n", cfg.Namespace, "--kubeconfig", cfg.SrcKubeconfig,
		"--", "bash", "-c", sshfsCheckCmd)

	if err != nil || strings.Contains(output, "not found") {
		errMsg := ""
		if err != nil {
			errMsg = err.Error()
		}
		s.logger.Error("SSHFS command not available",
			zap.String("error", errMsg),
			zap.String("output", output))

		if err != nil {
			return fmt.Errorf("SSHFS command not available in the replicator pod: %w", err)
		}
		return fmt.Errorf("SSHFS command not available in the replicator pod - please install sshfs package")
	}

	s.logger.Debug("SSHFS command check successful", zap.String("path", output))
	s.logger.Info("All connectivity and command checks successful")
	return nil
}

// Mount establishes the SSHFS connection
func (s *SSHFSProvider) Mount(cfg *config.Config, hostIP, port string) error {
	s.logger.Info("Mounting remote filesystem with SSHFS",
		zap.String("hostIP", hostIP),
		zap.String("port", port))

	mountCmd := fmt.Sprintf("mkdir -p /data/dimg && "+
		"sshfs -o StrictHostKeyChecking=no -o port=%s %s:/data/simg /data/dimg",
		port, hostIP)

	s.logger.Debug("Executing SSHFS mount command",
		zap.String("command", mountCmd),
		zap.String("pod", fmt.Sprintf("%s-src-replicator", cfg.VMName)),
		zap.String("hostIP", hostIP),
		zap.String("port", port))

	output, err := s.executor.Execute(cfg.KubeCLI, "exec",
		fmt.Sprintf("%s-src-replicator", cfg.VMName),
		"-n", cfg.Namespace, "--kubeconfig", cfg.SrcKubeconfig,
		"--", "bash", "-c", mountCmd)

	if err != nil {
		s.logger.Error("Failed to mount remote filesystem",
			zap.String("error", err.Error()),
			zap.String("output", output))
		return fmt.Errorf("failed to mount destination directory: %w", err)
	}

	s.logger.Debug("Mount command executed successfully", zap.String("output", output))
	s.logger.Info("SSHFS mount successful")
	return nil
}

// VerifyMount checks that the mount point is accessible
func (s *SSHFSProvider) VerifyMount(cfg *config.Config) error {
	s.logger.Info("Verifying mount point accessibility")

	verifyCmd := "ls -la /data/dimg"

	s.logger.Debug("Checking mount point accessibility",
		zap.String("command", verifyCmd),
		zap.String("pod", fmt.Sprintf("%s-src-replicator", cfg.VMName)))

	output, err := s.executor.Execute(cfg.KubeCLI, "exec",
		fmt.Sprintf("%s-src-replicator", cfg.VMName),
		"-n", cfg.Namespace, "--kubeconfig", cfg.SrcKubeconfig,
		"--", "bash", "-c", verifyCmd)

	if err != nil {
		s.logger.Error("Mount verification failed",
			zap.String("error", err.Error()),
			zap.String("output", output))
		return fmt.Errorf("mount verification failed: %w", err)
	}

	s.logger.Debug("Mount point content listing", zap.String("output", output))

	// Verify that we can see the mounted directory contents
	if !strings.Contains(output, "simg") {
		s.logger.Error("Mount verification failed: cannot find expected content",
			zap.String("output", output))
		return fmt.Errorf("mount verification failed: cannot access mounted content")
	}

	s.logger.Info("Mount verification successful")
	return nil
}

// Unmount removes the SSHFS mount
func (s *SSHFSProvider) Unmount(cfg *config.Config) error {
	s.logger.Info("Unmounting SSHFS mount point")

	unmountCmd := "umount /data/dimg"

	s.logger.Debug("Executing unmount command",
		zap.String("command", unmountCmd),
		zap.String("pod", fmt.Sprintf("%s-src-replicator", cfg.VMName)))

	output, err := s.executor.Execute(cfg.KubeCLI, "exec",
		fmt.Sprintf("%s-src-replicator", cfg.VMName),
		"-n", cfg.Namespace, "--kubeconfig", cfg.SrcKubeconfig,
		"--", "bash", "-c", unmountCmd)

	if err != nil {
		s.logger.Warn("Failed to unmount SSHFS",
			zap.String("error", err.Error()),
			zap.String("output", output))
		return fmt.Errorf("unmount operation failed: %w", err)
	}

	s.logger.Debug("Unmount command executed successfully", zap.String("output", output))
	s.logger.Info("SSHFS unmounted successfully")
	return nil
}
