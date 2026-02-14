package orchestrator

// ToolCall represents a tool call for judge evaluation
type ToolCall struct {
	ToolName  string
	Arguments map[string]interface{}
	Result    string
	IsError   bool
}

// SandboxFile represents a file created in the agent sandbox
type SandboxFile struct {
	Path    string
	Size    int
	Content string
}

// min returns the minimum of two integers
func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// minFloat returns the minimum of two float64 values
func minFloat(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}
