package ssh

import (
	"github.com/kloia/kubevirt-migrator/internal/config"
)

// MockSSHManager implements SSHManager for testing
type MockSSHManager struct {
	GenerateKeysErrors  map[string]error
	SetupDestAuthErrors map[string]error
	GenerateKeysCalls   []string
	SetupDestAuthCalls  []string
	WriteKeyFileCalls   map[string]string
	WriteKeyFileErrors  map[string]error
	PublicKey           string
	PrivateKey          string
}

// NewMockSSHManager creates a new mock SSH manager
func NewMockSSHManager() *MockSSHManager {
	return &MockSSHManager{
		GenerateKeysErrors:  make(map[string]error),
		SetupDestAuthErrors: make(map[string]error),
		GenerateKeysCalls:   []string{},
		SetupDestAuthCalls:  []string{},
		WriteKeyFileCalls:   make(map[string]string),
		WriteKeyFileErrors:  make(map[string]error),
		PublicKey:           "mock-public-key",
		PrivateKey:          "mock-private-key",
	}
}

// SetGenerateKeysError sets an error for GenerateKeys by VM name
func (m *MockSSHManager) SetGenerateKeysError(vmName string, err error) {
	m.GenerateKeysErrors[vmName] = err
}

// SetSetupDestinationAuthError sets an error for SetupDestinationAuth by VM name
func (m *MockSSHManager) SetSetupDestinationAuthError(vmName string, err error) {
	m.SetupDestAuthErrors[vmName] = err
}

// SetWriteKeyFileError sets an error for writeKeyFile by filename
func (m *MockSSHManager) SetWriteKeyFileError(filename string, err error) {
	m.WriteKeyFileErrors[filename] = err
}

// GenerateKeys mocks the SSH key generation process
func (m *MockSSHManager) GenerateKeys(cfg *config.Config) error {
	m.GenerateKeysCalls = append(m.GenerateKeysCalls, cfg.VMName)

	if err, ok := m.GenerateKeysErrors[cfg.VMName]; ok {
		return err
	}

	return nil
}

// SetupDestinationAuth mocks setting up SSH authorization on destination pod
func (m *MockSSHManager) SetupDestinationAuth(cfg *config.Config) error {
	m.SetupDestAuthCalls = append(m.SetupDestAuthCalls, cfg.VMName)

	if err, ok := m.SetupDestAuthErrors[cfg.VMName]; ok {
		return err
	}

	return nil
}

// GetPublicKey returns the mock public key
func (m *MockSSHManager) GetPublicKey() string {
	return m.PublicKey
}

// GetPrivateKey returns the mock private key
func (m *MockSSHManager) GetPrivateKey() string {
	return m.PrivateKey
}

// SetKeys sets the mock keys
func (m *MockSSHManager) SetKeys(publicKey, privateKey string) {
	m.PublicKey = publicKey
	m.PrivateKey = privateKey
}
