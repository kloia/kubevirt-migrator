package sync

// MockSyncCommand implements SyncCommand for testing
type MockSyncCommand struct {
	ToolName          string
	CommandToGenerate string
	ArgsToGenerate    []string
	CalledWithSource  string
	CalledWithDest    string
	CalledWithOptions map[string]string
	GenerateCallCount int
	GetToolNameCount  int
}

// NewMockSyncCommand creates a new mock sync command
func NewMockSyncCommand(toolName string) *MockSyncCommand {
	return &MockSyncCommand{
		ToolName:          toolName,
		ArgsToGenerate:    []string{},
		CalledWithOptions: make(map[string]string),
	}
}

// GenerateSyncCommand implements SyncCommand.GenerateSyncCommand
func (m *MockSyncCommand) GenerateSyncCommand(sourcePath, destPath string, options map[string]string) (string, []string) {
	m.GenerateCallCount++
	m.CalledWithSource = sourcePath
	m.CalledWithDest = destPath

	// Save the options
	for k, v := range options {
		m.CalledWithOptions[k] = v
	}

	return m.CommandToGenerate, m.ArgsToGenerate
}

// GetToolName implements SyncCommand.GetToolName
func (m *MockSyncCommand) GetToolName() string {
	m.GetToolNameCount++
	return m.ToolName
}

// SetCommandToGenerate sets the command and args to return
func (m *MockSyncCommand) SetCommandToGenerate(command string, args []string) {
	m.CommandToGenerate = command
	m.ArgsToGenerate = args
}
