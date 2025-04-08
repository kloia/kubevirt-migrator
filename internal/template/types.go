package template

// TemplateKind represents different template types
type TemplateKind string

const (
	// SourceReplicator template for source replicator pod
	SourceReplicator TemplateKind = "src-repl"
	// DestReplicator template for destination replicator pod
	DestReplicator TemplateKind = "dst-repl"
	// DestService template for destination service
	DestService TemplateKind = "dst-repl-svc"
	// ReplicationJob template for replication cronjob
	ReplicationJob TemplateKind = "src-cronjob"
)

// TemplateVariables represents placeholders in templates
type TemplateVariables struct {
	// VMName is the name of the virtual machine
	VMName string
	// Namespace is the Kubernetes namespace
	Namespace string
	// Port is the service port
	Port int
	// TargetPort is the service target port
	TargetPort int
	// Schedule is the cron schedule for replication job
	Schedule string
	// ReplicationCommand is the command for replication
	ReplicationCommand string
	// SyncTool is the replication tool to use (rclone, rsync)
	SyncTool string
	// CPU and memory resource fields for dynamic resource allocation
	CPULimit      string
	CPURequest    string
	MemoryLimit   string
	MemoryRequest string
}
