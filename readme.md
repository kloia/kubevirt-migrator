# KubeVirt Migrator

KubeVirt Migrator is a tool designed to facilitate the migration 
of virtual machines between OpenShift clusters using KubeVirt. 
It provides a streamlined process for VM replication and 
migration while ensuring data consistency.

## Features

    - Warm migration of VMs between OpenShift clusters
    - Automated handling of VM states during migration
    - Secure replication
    - Progress monitoring and status checking
    - Configurable replication schedules
    - Automatic validation of cluster configurations

## Installation

### Dependencies
- Docker (for building container images which installs dependencies for migration operations)
  - `rclone`: File synchronization
  - `sshfs`: SSH filesystem mounting
  - `guestmount`: VM disk image mounting (requires libguestfs-tools)
- Access to both source and destination Kubernetes clusters with KubeVirt installed
- **The following tools must be available in PATH:**
  - `oc` or `kubectl`: Kubernetes CLI
  - `virtctl`: KubeVirt VM management
  - `yq`: YAML processing

### Install from GitHub

You can install using Go directly:

```bash
# Install the latest version
go install github.com/kloia/kubevirt-migrator/cmd/kubevirt-migrator@latest

# Verify installation
kubevirt-migrator --help
```

### Build from Source

1. Clone the repository:
   ```bash
   git clone https://github.com/kloia/kubevirt-migrator.git
   cd kubevirt-migrator
   ```

2. Download dependencies:
   ```bash
   task download
   # or
   go mod download
   ```

3. Build the binary:
   ```bash
   task build
   # or for platform-specific builds:
   
   # For Linux (amd64)
   GOOS=linux GOARCH=amd64 go build -o bin/kubevirt-migrator ./cmd/kubevirt-migrator
   
   # For macOS (amd64)
   GOOS=darwin GOARCH=amd64 go build -o bin/kubevirt-migrator ./cmd/kubevirt-migrator
   
   # For macOS (Apple Silicon)
   GOOS=darwin GOARCH=arm64 go build -o bin/kubevirt-migrator ./cmd/kubevirt-migrator
   ```

4. Build the container images:
   ```bash
   task docker
   # or
   docker build -t kubevirt-migrator-src -f Dockerfiles/DockerfileReplicator .
   docker build -t kubevirt-migrator-dst -f Dockerfiles/DockerfileDst .
   ```

## Usage

KubeVirt Migrator provides two main commands:

### Initialize Migration

The `init` command sets up the migration infrastructure and starts initial replication:

```bash
kubevirt-migrator init \
  --vm-name <vm-name> \
  --namespace <namespace> \
  --src-kubeconfig <source-kubeconfig> \
  --dst-kubeconfig <destination-kubeconfig> \
  [--preserve-pod-ip]
```

This will:
1. Create a stopped VM on the destination cluster
2. Set up replication pods on both clusters
3. Perform initial disk replication
4. Configure incremental replication via cronjob

### Perform Migration

The `migrate` command finalizes the migration:

```bash
kubevirt-migrator migrate \
  --vm-name <vm-name> \
  --namespace <namespace> \
  --src-kubeconfig <source-kubeconfig> \
  --dst-kubeconfig <destination-kubeconfig>
```

This will:
1. Stop the VM on the source cluster
2. Perform final replication
3. Start the VM on the destination cluster
4. Clean up all migration resources

## Configuration

The tool can be configured via:

1. Command-line arguments
2. Environment variables (prefixed with `KUBEVIRT_MIGRATOR_`)

Available options:

| CLI Flag | Environment Variable | Description |
|----------|----------------------|-------------|
| `--vm-name` | `KUBEVIRT_MIGRATOR_VM_NAME` | Name of the virtual machine (required) |
| `--namespace` | `KUBEVIRT_MIGRATOR_NAMESPACE` | Kubernetes namespace (required) |
| `--src-kubeconfig` | `KUBEVIRT_MIGRATOR_SRC_KUBECONFIG` | Source cluster kubeconfig (required) |
| `--dst-kubeconfig` | `KUBEVIRT_MIGRATOR_DST_KUBECONFIG` | Destination cluster kubeconfig (required) |
| `--preserve-pod-ip` | `KUBEVIRT_MIGRATOR_PRESERVE_POD_IP` | Preserve pod IP address during migration |
| `--log-level` | `KUBEVIRT_MIGRATOR_LOG_LEVEL` | Logging level (debug, info, warn, error) |
| `--ssh-port` | `KUBEVIRT_MIGRATOR_SSH_PORT` | SSH port for replication |

## Installation of dependencies

### Platform Requirements
- OpenShift 4.x or higher
- KubeVirt v0.54.0 or higher

### CLI Tools
1. **oc** (OpenShift CLI)
   ```bash
   wget https://mirror.openshift.com/pub/openshift-v4/clients/ocp/latest/openshift-client-linux.tar.gz
   tar xvf openshift-client-linux.tar.gz
   sudo mv oc /usr/local/bin/
    ```

2. **yq** (YAML processor)

    ##### On Linux
    ```bash
    wget https://github.com/mikefarah/yq/releases/latest/download/yq_linux_amd64 -O /usr/bin/yq
    chmod +x /usr/bin/yq
    ```
    ##### On macOS
    ```bash
    brew install yq
    ```

3. **virtctl** (KubeVirt CLI tool)

    #### Download and Install virtctl
    ```bash
    export VERSION=$(curl -s https://api.github.com/repos/kubevirt/kubevirt/releases/latest | grep '"tag_name":' | sed -E 's/.*"([^"]+)".*/\1/')
    wget https://github.com/kubevirt/kubevirt/releases/download/${VERSION}/virtctl-${VERSION}-linux-amd64
    chmod +x virtctl-${VERSION}-linux-amd64
    sudo mv virtctl-${VERSION}-linux-amd64 /usr/local/bin/virtctl
    ```
### Network Requirements

Direct network connectivity between clusters

Open ports:

    443/TCP (Kubernetes API)

    6443/TCP (Kubernetes API)

    30000-32767/TCP (NodePort range)

No NAT between clusters (recommended)

Stable network connection with sufficient bandwidth

### Permissions and RBAC
Required Cluster Permissions
```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
rules:
- apiGroups: ["kubevirt.io"]
  resources: ["virtualmachines", "virtualmachineinstances"]
  verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]
- apiGroups: [""]
  resources: ["pods", "services", "persistentvolumeclaims", "secrets"]
  verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]
- apiGroups: ["batch"]
  resources: ["jobs", "cronjobs"]
  verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]
```

### OpenShift Security Context Constraints (SCC)

This tool requires privileged container permissions to mount and synchronize VM disk images. Before using the tool in OpenShift environments, you must grant the appropriate SCC permissions to your service account:

#### For the default service account:
```bash
oc adm policy add-scc-to-user privileged -z default -n <your-namespace>
```

#### For a custom service account:
```bash
# Create a custom service account (optional)
oc create serviceaccount kubevirt-migrator-sa -n <your-namespace>

# Grant privileged SCC permissions
oc adm policy add-scc-to-user privileged -z kubevirt-migrator-sa -n <your-namespace>
```

Without these permissions, replication jobs will fail with errors like:
```
Error creating: pods "vm-name-repl-cronjob-xxxxxx-" is forbidden: unable to validate against any security context constraint: [provider "anyuid": Forbidden: not usable by user or serviceaccount, provider restricted-v2: .containers[0].privileged: Invalid value: true: Privileged containers are not allowed...]
```

These permissions are required because the tool uses privileged containers to:
1. Mount VM disk images with `guestmount`
2. Use `sshfs` for secure replication
3. Perform file synchronization across clusters

## Migration Process

### How It Works

1. **Initialization Phase(Replication initialization)**:
   - Validates environment and prerequisites
   - Checks VM status in both clusters
   - Sets up replication components
   - Replication Setup
   - Creates source and destination replicators
   - Establishes secure connection between clusters

2. **Migration Phase**:
   - Final replication is performed using the cronjob
   - Source VM is stopped
   - Destination VM is started
   - Validates successful migration
   - All migration resources are cleaned up

## Troubleshooting
### Common issues and solutions:

#### VM Status Check Fails

    - Verify VM exists in the specified namespace

    - Ensure kubeconfig files are valid

    - Check cluster connectivity

#### Replication Issues

    - Verify network connectivity between clusters

    - Check storage provisioner status

    - Ensure sufficient storage capacity

#### Permission Errors

    - Verify RBAC permissions

    - Check service account configurations

    - Ensure proper cluster access

## Limitations
    - Requires KubeVirt installation on both clusters

    - Network connectivity between clusters required

    - Storage must be compatible with both clusters

    - VM must use supported disk formats



## License
MIT License

## Shell Completion

The tool provides shell completion support for bash, zsh, and fish, making it easier to use the CLI commands and flags.

### Bash Completion

```bash
# Generate the completion script and add it to your bash profile
kubevirt-migrator completion bash > ~/.bash_completion.d/kubevirt-migrator
echo 'source ~/.bash_completion.d/kubevirt-migrator' >> ~/.bashrc
```

### Zsh Completion

```bash
# For macOS/Linux with zsh
mkdir -p ~/.zsh/completion
kubevirt-migrator completion zsh > ~/.zsh/completion/_kubevirt-migrator

# Add to your ~/.zshrc if not already present
echo 'fpath=(~/.zsh/completion $fpath)' >> ~/.zshrc
echo 'autoload -U compinit && compinit' >> ~/.zshrc
source ~/.zshrc
```

### Fish Completion

```bash
# Generate the completion script for fish
kubevirt-migrator completion fish > ~/.config/fish/completions/kubevirt-migrator.fish
```