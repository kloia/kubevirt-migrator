package kubernetes

import (
	"time"
)

// KubernetesClient defines a common interface for interacting with Kubernetes/OpenShift clusters
type KubernetesClient interface {
	// VM Management
	GetVMStatus(vmName, namespace string) (string, error)
	StartVM(vmName, namespace string) error
	StopVM(vmName, namespace string) error
	ExportVM(vmName, namespace string) ([]byte, error)
	ImportVM(vmDef []byte, namespace string) error
	WaitForVMStatus(vmName, namespace, expectedStatus string, timeout time.Duration) error

	// Pod Management
	GetPodStatus(podName, namespace string) (string, error)
	WaitForPod(podName, namespace string, condition string, timeout time.Duration) error
	ExecInPod(podName, namespace, command string) (string, error)
	GetPodHostIP(podName, namespace string) (string, error)

	// Service Management
	CreateService(svcDef []byte, namespace string) error
	GetNodePort(svcName, namespace string) (int, error)

	// Job Management
	CreateJob(jobDef []byte, namespace string) error
	WaitForJob(jobName, namespace string, timeout time.Duration) error

	// Secret Management
	CreateSecret(secretDef []byte, namespace string) error

	// CronJob Management
	CreateCronJob(cronJobDef []byte, namespace string) error
	SuspendCronJob(cronJobName, namespace string) error

	// Cleanup Management
	CleanupMigrationResources(vmName, namespace string, isDestination bool) error

	// Resource Management
	GetPVCSize(pvcName, namespace string) (string, error)
	GetActualDiskUsage(vmName, namespace string) (int64, error)
}
