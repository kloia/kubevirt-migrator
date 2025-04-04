package kubernetes

import (
	"go.uber.org/zap"

	"github.com/kloia/kubevirt-migrator/internal/executor"
	"github.com/kloia/kubevirt-migrator/internal/sync"
)

// OCClient implements KubernetesClient using the OpenShift (oc) CLI
type OCClient struct {
	*BaseClient
}

// NewOCClient creates a new OCClient instance
func NewOCClient(kubeconfig string, executor executor.CommandExecutor, syncTool sync.SyncCommand, logger *zap.Logger) *OCClient {
	return &OCClient{
		BaseClient: NewBaseClient("oc", kubeconfig, executor, syncTool, logger),
	}
}
