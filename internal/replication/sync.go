package replication

import (
	"fmt"
	"strings"

	"go.uber.org/zap"

	"encoding/base64"

	"github.com/kloia/kubevirt-migrator/internal/config"
	"github.com/kloia/kubevirt-migrator/internal/encrypt/ssh"
	"github.com/kloia/kubevirt-migrator/internal/executor"
	"github.com/kloia/kubevirt-migrator/internal/kubernetes"
	"github.com/kloia/kubevirt-migrator/internal/resource"
	"github.com/kloia/kubevirt-migrator/internal/storage/copy"
	"github.com/kloia/kubevirt-migrator/internal/storage/mount"
	"github.com/kloia/kubevirt-migrator/internal/sync"
	"github.com/kloia/kubevirt-migrator/internal/template"
)

// SyncManager handles replication and synchronization
type SyncManager struct {
	executor      executor.CommandExecutor
	logger        *zap.Logger
	sshMgr        ssh.SSHManagerInterface
	tmplMgr       template.TemplateManager
	syncTool      sync.SyncCommand
	mountProvider mount.MountProvider
	copyProvider  copy.DataCopyProvider
	srcClient     kubernetes.KubernetesClient
	dstClient     kubernetes.KubernetesClient
}

// NewSyncManager creates a new synchronization manager
func NewSyncManager(
	executor executor.CommandExecutor,
	logger *zap.Logger,
	sshMgr ssh.SSHManagerInterface,
	tmplMgr template.TemplateManager,
	srcClient kubernetes.KubernetesClient,
	dstClient kubernetes.KubernetesClient,
) *SyncManager {
	sm := &SyncManager{
		executor:  executor,
		logger:    logger,
		sshMgr:    sshMgr,
		tmplMgr:   tmplMgr,
		srcClient: srcClient,
		dstClient: dstClient,
	}

	// Set default mount provider
	mountProvider := mount.NewSSHFSProvider(executor, logger)
	sm.mountProvider = mountProvider

	// Set default copy provider
	sm.copyProvider = copy.NewSimpleBlockCopyProvider(executor, logger)

	return sm
}

// SetSyncTool sets the synchronization tool
func (s *SyncManager) SetSyncTool(syncTool sync.SyncCommand) {
	s.syncTool = syncTool
}

// SetMountProvider sets the mount provider
func (s *SyncManager) SetMountProvider(provider mount.MountProvider) {
	s.mountProvider = provider
}

// SetCopyProvider sets the data copy provider
func (s *SyncManager) SetCopyProvider(provider copy.DataCopyProvider) {
	s.copyProvider = provider
}

// GetDestinationInfo returns NodePort and HostIP for the destination replicator
func (s *SyncManager) GetDestinationInfo(cfg *config.Config) (nodePort string, hostIP string, err error) {
	// Get NodePort using client
	nodePortInt, err := s.dstClient.GetNodePort(fmt.Sprintf("%s-dst-svc", cfg.VMName), cfg.Namespace)
	if err != nil {
		return "", "", fmt.Errorf("failed to get destination NodePort: %w", err)
	}
	nodePort = fmt.Sprintf("%d", nodePortInt)

	// Get Host IP using client
	hostIP, err = s.dstClient.GetPodHostIP(fmt.Sprintf("%s-dst-replicator", cfg.VMName), cfg.Namespace)
	if err != nil {
		return "", "", fmt.Errorf("failed to get destination Host IP: %w", err)
	}

	return nodePort, hostIP, nil
}

// PerformInitialSync performs the initial synchronization
func (s *SyncManager) PerformInitialSync(cfg *config.Config) error {
	if cfg.DryRun {
		s.logger.Info("Skipping initial sync due to dry-run mode")
		return nil
	}

	s.logger.Info("Getting destination information for initial sync")

	// Get NodePort and Host IP
	nodePort, hostIP, err := s.GetDestinationInfo(cfg)
	if err != nil {
		return err
	}

	s.logger.Info("Starting initial volume replication",
		zap.String("hostIP", hostIP),
		zap.String("nodePort", nodePort),
		zap.String("syncTool", cfg.SyncTool))

	// Check basic connectivity first
	if err := s.mountProvider.CheckConnectivity(cfg, hostIP, nodePort); err != nil {
		return fmt.Errorf("connectivity check failed: %w", err)
	}

	// Establish mount
	if err := s.mountProvider.Mount(cfg, hostIP, nodePort); err != nil {
		return err
	}

	// Verify mount
	if err := s.mountProvider.VerifyMount(cfg); err != nil {
		return err
	}

	// // Copy data using the copy provider
	if err := s.copyProvider.CopyData(cfg); err != nil {
		return err
	}

	return nil
}

// CreateSyncCommand generates a synchronization command based on the sync tool
func (s *SyncManager) CreateSyncCommand(nodePort, hostIP string, cfg *config.Config) (string, error) {
	if s.syncTool == nil {
		// Dynamic partition detection command using virt-filesystems for Linux filesystem partitions
		cmd := fmt.Sprintf(`mkdir -p /data/dimg;
sshfs -o StrictHostKeyChecking=no -o port=%s %s:/data/simg /data/dimg;
DISK_IMG="/data/simg/disk.img";

# Using virt-filesystems to list partitions
VIRT_FS_OUTPUT=$(virt-filesystems --partitions --format=raw -a ${DISK_IMG});

# Get partition information from fdisk
FDISK_OUTPUT=$(fdisk -l ${DISK_IMG});

# Extract only partitions marked as "Linux filesystem"
LINUX_FS_PARTITIONS=$(echo "$FDISK_OUTPUT" | grep "Linux filesystem" | awk '{print $1}');

# Extract partition numbers only
PART_NUMS=();
for partition in $LINUX_FS_PARTITIONS; do
    # Extract partition number (the last digit after img)
    PART_NUM=${partition##*img};
    PART_NUMS+=("$PART_NUM");
done;

# Get available device formats from virt-filesystems
AVAILABLE_DEVICES=();
for dev in $VIRT_FS_OUTPUT; do
    for num in "${PART_NUMS[@]}"; do
        # Check if the device ends with the partition number
        if [[ $dev == */[a-z]*$num ]]; then
            AVAILABLE_DEVICES+=("$dev");
        fi;
    done;
done;

# Process each detected filesystem partition
for device in "${AVAILABLE_DEVICES[@]}"; do
    # Get device name for mount point
    device_name=$(basename "$device");
    
    # Create mount points
    mkdir -p /data/sfs_${device_name} /data/dfs_${device_name};
    
    # Mount filesystems
    guestmount -a /data/simg/disk.img -m ${device} --ro /data/sfs_${device_name};
    guestmount -a /data/dimg/disk.img -m ${device} --rw /data/dfs_${device_name};
    
    # Sync files - pass source and destination as direct arguments
    rclone sync --progress /data/sfs_${device_name}/ /data/dfs_${device_name}/ --skip-links --checkers 8 --contimeout 100s --timeout 300s --retries 3 --low-level-retries 10 --drive-acknowledge-abuse --stats 1s --cutoff-mode=soft;
done;

sleep 20`, nodePort, hostIP)

		s.logger.Info("Created dynamic multi-filesystem detection and replication command with virt-filesystems")
		return cmd, nil
	}

	// Get sync options (without specifying paths - they will be dynamically generated in the script)
	syncToolName, syncArgs := s.syncTool.GenerateSyncCommand("", "", map[string]string{
		// "checksum": "true",
		"checkers": "8",
	})

	// Create the full replication command with dynamic filesystem detection using virt-filesystems
	cmd := fmt.Sprintf(`mkdir -p /data/dimg;
sshfs -o StrictHostKeyChecking=no -o port=%s %s:/data/simg /data/dimg;
DISK_IMG="/data/simg/disk.img";

# Using virt-filesystems to list partitions
VIRT_FS_OUTPUT=$(virt-filesystems --partitions --format=raw -a ${DISK_IMG});

# Get partition information from fdisk
FDISK_OUTPUT=$(fdisk -l ${DISK_IMG});

# Extract only partitions marked as "Linux filesystem"
LINUX_FS_PARTITIONS=$(echo "$FDISK_OUTPUT" | grep "Linux filesystem" | awk '{print $1}');

# Extract partition numbers only
PART_NUMS=();
for partition in $LINUX_FS_PARTITIONS; do
    # Extract partition number (the last digit after img)
    PART_NUM=${partition##*img};
    PART_NUMS+=("$PART_NUM");
done;

# Get available device formats from virt-filesystems
AVAILABLE_DEVICES=();
for dev in $VIRT_FS_OUTPUT; do
    for num in "${PART_NUMS[@]}"; do
        # Check if the device ends with the partition number
        if [[ $dev == */[a-z]*$num ]]; then
            AVAILABLE_DEVICES+=("$dev");
        fi;
    done;
done;

# Process each detected filesystem partition
for device in "${AVAILABLE_DEVICES[@]}"; do
    # Get device name for mount point
    device_name=$(basename "$device");
    
    # Create mount points
    mkdir -p /data/sfs_${device_name} /data/dfs_${device_name};
    
    # Mount filesystems
    guestmount -a /data/simg/disk.img -m ${device} --ro /data/sfs_${device_name};
    guestmount -a /data/dimg/disk.img -m ${device} --rw /data/dfs_${device_name};
    
    # Run the sync tool with dynamically generated paths
    %s %s /data/sfs_${device_name}/ /data/dfs_${device_name}/;
done;

sleep 20`, nodePort, hostIP, syncToolName, strings.Join(syncArgs, " "))

	s.logger.Info("Created dynamic multi-filesystem detection and replication command with virt-filesystems",
		zap.String("syncTool", s.syncTool.GetToolName()))

	return cmd, nil
}

// SetupCronJob sets up the asynchronous replication cronjob
func (s *SyncManager) SetupCronJob(cfg *config.Config) error {
	if cfg.DryRun {
		s.logger.Info("Skipping cronjob setup due to dry-run mode")
		return nil
	}

	s.logger.Info("Setting up replication cronjob",
		zap.String("vm", cfg.VMName),
		zap.String("kubeCLI", cfg.KubeCLI),
		zap.String("syncTool", cfg.SyncTool),
		zap.String("schedule", cfg.ReplicationSchedule))

	// Get NodePort and Host IP
	nodePort, hostIP, err := s.GetDestinationInfo(cfg)
	if err != nil {
		return err
	}

	// Create replication command
	replicationCmd, err := s.CreateSyncCommand(nodePort, hostIP, cfg)
	if err != nil {
		return fmt.Errorf("failed to create replication command: %w", err)
	}

	// Base64 encode the replication command
	encodedCmd := base64.StdEncoding.EncodeToString([]byte(replicationCmd))

	// Debug log for replication command
	s.logger.Debug("Generated replication command",
		zap.String("command", replicationCmd),
		zap.String("encoded", encodedCmd))

	// Get resource requirements from the resource calculator
	calculator := resource.NewResourceCalculator(s.logger)
	defaultResources := calculator.GetDefaultResources()

	// Try to get actual disk usage from source client
	usedBytes, err := s.srcClient.GetActualDiskUsage(cfg.VMName, cfg.Namespace)
	resources := defaultResources

	if err == nil {
		// Calculate resources based on actual disk usage
		resources, err = calculator.CalculateResourcesFromUsage(usedBytes)
		if err != nil {
			s.logger.Warn("Failed to calculate resources from disk usage for cronjob, using defaults",
				zap.Error(err))
			resources = defaultResources
		}
	} else {
		// Fall back to PVC size
		s.logger.Warn("Could not get actual disk usage for cronjob, falling back to PVC size",
			zap.Error(err))

		pvcSize, pvcErr := s.srcClient.GetPVCSize(cfg.VMName, cfg.Namespace)
		if pvcErr == nil {
			resources, pvcErr = calculator.FallbackToPVCSize(pvcSize)
			if pvcErr != nil {
				s.logger.Warn("Failed to calculate resources from PVC size for cronjob, using defaults",
					zap.Error(pvcErr))
			}
		} else {
			s.logger.Warn("Could not get PVC size for cronjob, using default resources",
				zap.Error(pvcErr))
		}
	}

	// Create CronJob using template with resource requirements
	err = s.tmplMgr.RenderAndApply(template.ReplicationJob, template.TemplateVariables{
		VMName:             cfg.VMName,
		Namespace:          cfg.Namespace,
		Schedule:           cfg.ReplicationSchedule, // Use the configured schedule
		ReplicationCommand: encodedCmd,
		SyncTool:           cfg.SyncTool,
		CPULimit:           resources.CPULimit,
		CPURequest:         resources.CPURequest,
		MemoryLimit:        resources.MemoryLimit,
		MemoryRequest:      resources.MemoryRequest,
	}, cfg.SrcKubeconfig)
	if err != nil {
		return fmt.Errorf("failed to create cronjob: %w", err)
	}

	s.logger.Info("Replication cronjob created successfully",
		zap.String("schedule", cfg.ReplicationSchedule),
		zap.String("cpuLimit", resources.CPULimit),
		zap.String("memoryLimit", resources.MemoryLimit))
	return nil
}

// PerformFinalSync performs the final synchronization
func (s *SyncManager) PerformFinalSync(cfg *config.Config) error {
	s.logger.Info("Creating final replication job")

	// Create final job
	_, err := s.executor.Execute(cfg.KubeCLI, "create", "job", "--from=cronjob/"+fmt.Sprintf("%s-repl-cronjob", cfg.VMName),
		fmt.Sprintf("%s-repl-final-job", cfg.VMName), "-n", cfg.Namespace, "--kubeconfig", cfg.SrcKubeconfig)
	if err != nil {
		return fmt.Errorf("failed to create final job: %w", err)
	}

	s.logger.Info("Waiting for final replication to complete")

	// Wait for job completion
	_, err = s.executor.Execute(cfg.KubeCLI, "wait", "job", fmt.Sprintf("%s-repl-final-job", cfg.VMName),
		"-n", cfg.Namespace, "--kubeconfig", cfg.SrcKubeconfig,
		"--for=condition=complete", "--timeout=-1m")
	if err != nil {
		return fmt.Errorf("failed waiting for final job: %w", err)
	}

	return nil
}

// SuspendCronJob suspends the replication CronJob
func (s *SyncManager) SuspendCronJob(cfg *config.Config) error {
	s.logger.Info("Suspending CronJob")

	// Suspend cronjob
	_, err := s.executor.Execute(cfg.KubeCLI, "patch", "cronjob", fmt.Sprintf("%s-repl-cronjob", cfg.VMName),
		"-n", cfg.Namespace, "--kubeconfig", cfg.SrcKubeconfig,
		"-p", `{"spec" : {"suspend" : true }}`)
	if err != nil {
		return fmt.Errorf("failed to suspend cronjob: %w", err)
	}

	return nil
}
