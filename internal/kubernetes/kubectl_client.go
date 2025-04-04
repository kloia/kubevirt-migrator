package kubernetes

import (
	"go.uber.org/zap"

	"github.com/kloia/kubevirt-migrator/internal/executor"
	"github.com/kloia/kubevirt-migrator/internal/sync"
)

// KubectlClient implements KubernetesClient using the Kubernetes (kubectl) CLI
type KubectlClient struct {
	*BaseClient
}

// NewKubectlClient creates a new KubectlClient instance
func NewKubectlClient(kubeconfig string, executor executor.CommandExecutor, syncTool sync.SyncCommand, logger *zap.Logger) *KubectlClient {
	return &KubectlClient{
		BaseClient: NewBaseClient("kubectl", kubeconfig, executor, syncTool, logger),
	}
}
