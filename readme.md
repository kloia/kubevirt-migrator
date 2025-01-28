# KubeVirt Migrator

KubeVirt Migrator is a tool designed to facilitate the migration of virtual machines between OpenShift clusters using KubeVirt. It provides a streamlined process for VM replication and migration while ensuring data consistency.

## Features

    - Warm migration of VMs between OpenShift clusters
    - Automated handling of VM states during migration
    - Secure replication
    - Progress monitoring and status checking
    - Configurable replication schedules
    - Automatic validation of cluster configurations

## Prerequisites

### Platform Requirements
- OpenShift 4.x or higher
- KubeVirt v0.54.0 or higher


### CLI Tools
1. **oc** (OpenShift CLI)
   ```bash
   wget https://mirror.openshift.com/pub/openshift-v4/clients/ocp/latest/openshift-client-linux.tar.gz
   tar xvf openshift-client-linux.tar.gz
   sudo mv oc /usr/local/bin/


2. **yq** (YAML processor)

    ##### On Linux
    ```bash
    wget https://github.com/mikefarah/yq/releases/latest/download/yq_linux_amd64 -O /usr/bin/yq
    chmod +x /usr/bin/yq
    ```
    ##### On macOS
    ```
    brew install yq
    ```

3. **virtctl** (KubeVirt CLI tool)

    #### Download and Install virtctl
    ```
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

# Installation

1. Clone the repository:
```bash
git clone https://github.com/kubevirt-migrator.git
cd kubevirt-migrator
```


Make the scripts executable:
```bash
chmod +x migrate.sh init.sh
```
# Usage

Use the init script to initialize the replication:
```bash
./init.sh \
  --vm-name <vm-name> \
  --namespace <namespace> \
  --src-kubeconfig <source-kubeconfig-path> \
  --dst-kubeconfig <destination-kubeconfig-path> \
  [--verbose]

```

Execute the migration script to migrate VM from source to destination OpenShift cluster:
```bash
./migrate.sh \
  --vm-name <vm-name> \
  --namespace <namespace> \
  --src-kubeconfig <source-kubeconfig-path> \
  --dst-kubeconfig <destination-kubeconfig-path>
```

## Command Line Arguments

    --vm-name: Name of the virtual machine to migrate

    --namespace: Kubernetes namespace containing the VM

    --src-kubeconfig: Path to source cluster's kubeconfig file

    --dst-kubeconfig: Path to destination cluster's kubeconfig file

    --verbose: Enable detailed logging (optional)

    --help: Display usage information

## Migration Process
### Replication Initialization

    - Validates environment and prerequisites

    - Checks VM status in both clusters

    - Sets up replication components

    - Replication Setup

    - Creates source and destination replicators

    - Establishes secure connection between clusters

### Migration

    - Stops the source VM

    - Performs final data synchronization

    - Starts the VM in destination cluster

    - Validates successful migration

## Directory Structure
```bash
kubevirt-migrator/
├── migrate.sh           # Main migration script
├── init.sh             # Initialization script
├── manifests/          # Kubernetes manifest templates
│   ├── src-repl.yaml   # Source replicator configuration
│   ├── dst-repl.yaml   # Destination replicator configuration
│   └── dst-repl-svc.yaml # Destination service configuration
│   └── src-cronjob.yaml # Source default cronjob configuration
└── README.md           # This file
```

## Troubleshooting
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

