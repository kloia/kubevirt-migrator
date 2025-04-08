package kubernetes

import (
	"fmt"

	"go.uber.org/zap"

	"github.com/kloia/kubevirt-migrator/internal/executor"
	"github.com/kloia/kubevirt-migrator/internal/sync"
)

// ClientType defines the type of Kubernetes client
type ClientType string

const (
	// ClientTypeOC represents the OpenShift CLI client
	ClientTypeOC ClientType = "oc"
	// ClientTypeKubectl represents the Kubernetes CLI client
	ClientTypeKubectl ClientType = "kubectl"
)

// ClientFactory creates KubernetesClient instances
type ClientFactory struct {
	executor executor.CommandExecutor
	syncTool sync.SyncCommand
	logger   *zap.Logger
}

// NewClientFactory creates a new ClientFactory instance
func NewClientFactory(executor executor.CommandExecutor, syncTool sync.SyncCommand, logger *zap.Logger) *ClientFactory {
	return &ClientFactory{
		executor: executor,
		syncTool: syncTool,
		logger:   logger,
	}
}

// CreateClient creates a KubernetesClient instance based on the client type
func (f *ClientFactory) CreateClient(clientType ClientType, kubeconfig string) (KubernetesClient, error) {
	switch clientType {
	case ClientTypeOC:
		return NewOCClient(kubeconfig, f.executor, f.syncTool, f.logger), nil
	case ClientTypeKubectl:
		return NewKubectlClient(kubeconfig, f.executor, f.syncTool, f.logger), nil
	default:
		return nil, fmt.Errorf("unsupported client type: %s", clientType)
	}
}
