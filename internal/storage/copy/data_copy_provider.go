package copy

import (
	"fmt"

	"go.uber.org/zap"

	"github.com/kloia/kubevirt-migrator/internal/config"
	"github.com/kloia/kubevirt-migrator/internal/executor"
)

// DataCopyProvider defines an interface for data transfer operations
// This allows for different data copying strategies in the future
type DataCopyProvider interface {
	// CopyData performs the data transfer operation
	CopyData(cfg *config.Config) error
}

// SimpleBlockCopyProvider implements basic block-level copying
type SimpleBlockCopyProvider struct {
	executor executor.CommandExecutor
	logger   *zap.Logger
}

// NewSimpleBlockCopyProvider creates a new block copy provider
func NewSimpleBlockCopyProvider(executor executor.CommandExecutor, logger *zap.Logger) *SimpleBlockCopyProvider {
	return &SimpleBlockCopyProvider{
		executor: executor,
		logger:   logger,
	}
}

// CopyData performs the initial data copy using basic cp command
func (p *SimpleBlockCopyProvider) CopyData(cfg *config.Config) error {
	p.logger.Info("Performing initial data copy")

	copyCmd := "cp -p --sparse=always /data/simg/disk.img /data/dimg/ & progress -m"
	output, err := p.executor.Execute(cfg.KubeCLI, "exec",
		fmt.Sprintf("%s-src-replicator", cfg.VMName),
		"-n", cfg.Namespace, "--kubeconfig", cfg.SrcKubeconfig,
		"--", "bash", "-c", copyCmd)

	if err != nil {
		p.logger.Error("Failed to perform initial data copy",
			zap.String("error", err.Error()),
			zap.String("output", output))
		return fmt.Errorf("failed to perform initial data copy: %w", err)
	}

	p.logger.Info("Data copy initiated successfully")
	return nil
}
