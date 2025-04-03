package template

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/kloia/kubevirt-migrator/internal/executor"
)

// Manager handles the templates
type Manager struct {
	executor executor.CommandExecutor
	logger   *zap.Logger
	tempDir  string
	kubeCLI  string
}

// NewManager creates a new template manager
func NewManager(executor executor.CommandExecutor, logger *zap.Logger, kubeCLI string) *Manager {
	if kubeCLI == "" {
		kubeCLI = "oc"
	}

	return &Manager{
		executor: executor,
		logger:   logger,
		tempDir:  os.TempDir(),
		kubeCLI:  kubeCLI,
	}
}

// RenderAndApply renders a template and applies it to the cluster
func (m *Manager) RenderAndApply(kind TemplateKind, vars TemplateVariables, kubeconfig string) error {
	// 1. Validate that the template kind is valid
	if !m.isValidTemplateKind(kind) {
		return fmt.Errorf("invalid template kind: %s", kind)
	}

	// 2. Read the template file
	templateFilename := string(kind) + ".yaml"
	// Protect against path traversal attacks in filename
	if strings.Contains(templateFilename, "/") || strings.Contains(templateFilename, "\\") {
		return fmt.Errorf("invalid template filename: %s", templateFilename)
	}

	tmplPath := filepath.Join("templates", templateFilename)
	// Clean path and check it's within templates directory
	tmplPath = filepath.Clean(tmplPath)
	if !strings.HasPrefix(tmplPath, "templates/") {
		return fmt.Errorf("template path must be in templates directory: %s", tmplPath)
	}

	content, err := os.ReadFile(tmplPath)
	if err != nil {
		return fmt.Errorf("failed to read template %s: %w", kind, err)
	}

	// 3. Replace variables in the template
	rendered := m.replaceVariables(string(content), vars)

	// 4. Create temporary file
	tmpFile, err := m.createTempFile(rendered)
	if err != nil {
		return err
	}
	defer func() {
		if err := os.Remove(tmpFile); err != nil {
			m.logger.Warn("Failed to remove temp file", zap.String("file", tmpFile), zap.Error(err))
		}
	}()

	// 5. Apply to Kubernetes cluster
	return m.applyToCluster(tmpFile, vars.Namespace, kubeconfig)
}

// SetKubeCLI sets the kubernetes CLI tool to use
func (m *Manager) SetKubeCLI(kubeCLI string) {
	m.kubeCLI = kubeCLI
}

// replaceVariables replaces all template variables in content
func (m *Manager) replaceVariables(content string, vars TemplateVariables) string {
	replacements := map[string]string{
		"${VM_NAME}":             vars.VMName,
		"${NAMESPACE}":           vars.Namespace,
		"${PORT}":                fmt.Sprintf("%d", vars.Port),
		"${TARGET_PORT}":         fmt.Sprintf("%d", vars.TargetPort),
		"${SCHEDULE}":            vars.Schedule,
		"${REPLICATION_COMMAND}": vars.ReplicationCommand,
		"${SYNC_TOOL}":           vars.SyncTool,
	}

	for k, v := range replacements {
		content = strings.ReplaceAll(content, k, v)
	}
	return content
}

// createTempFile creates a temporary file with content
func (m *Manager) createTempFile(content string) (string, error) {
	tmpFile := filepath.Join(m.tempDir, "kubevirt-migrator-"+uuid.New().String()+".yaml")
	if err := os.WriteFile(tmpFile, []byte(content), 0600); err != nil {
		return "", fmt.Errorf("failed to create temp file: %w", err)
	}
	return tmpFile, nil
}

// applyToCluster applies the file to the Kubernetes cluster
func (m *Manager) applyToCluster(file, namespace, kubeconfig string) error {
	_, err := m.executor.Execute(m.kubeCLI, "apply", "-f", file, "-n", namespace, "--kubeconfig", kubeconfig)
	if err != nil {
		return fmt.Errorf("failed to apply template: %w", err)
	}
	return nil
}

// isValidTemplateKind verifies that the template kind is valid
func (m *Manager) isValidTemplateKind(kind TemplateKind) bool {
	validKinds := []TemplateKind{
		SourceReplicator,
		DestReplicator,
		DestService,
		ReplicationJob,
	}

	for _, validKind := range validKinds {
		if kind == validKind {
			return true
		}
	}
	return false
}
