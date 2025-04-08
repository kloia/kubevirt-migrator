package replication

import (
	"github.com/kloia/kubevirt-migrator/internal/config"
)

// MountProvider defines an interface for mounting and transferring data
// This allows for future replacement of SSHFS with other technologies
type MountProvider interface {
	// CheckConnectivity verifies basic connectivity to the destination
	CheckConnectivity(cfg *config.Config, hostIP, port string) error

	// Mount establishes the connection to the remote filesystem
	Mount(cfg *config.Config, hostIP, port string) error

	// VerifyMount checks that the mount point is accessible and working
	VerifyMount(cfg *config.Config) error

	// Unmount removes the mount connection (optional cleanup)
	Unmount(cfg *config.Config) error
}
