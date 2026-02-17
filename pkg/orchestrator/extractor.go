package orchestrator

import (
	"github.com/recreate-run/mix-go-sdk/models/components"
)

// ExecutionHistory represents agent execution trace
type ExecutionHistory struct {
	ToolCalls      []ToolCallDetail
	ScreenshotURLs []string
	FinalResponse  string
	Reasoning      string
	TokensUsed     int64
	Cost           float64
}

// ToolCallDetail represents detailed tool execution
type ToolCallDetail struct {
	ID       string
	Name     string
	Input    string
	Result   string
	IsError  bool
	Finished bool
}

// extractHistory extracts execution history from messages
func extractHistory(messages []components.BackendMessage) ExecutionHistory {
	history := ExecutionHistory{}

	for _, msg := range messages {
		// Extract tool calls
		for _, tc := range msg.ToolCalls {
			detail := ToolCallDetail{
				ID:       tc.ID,
				Name:     extractToolName(tc.Name),
				Input:    tc.Input,
				Finished: tc.Finished,
			}

			if tc.Result != nil {
				detail.Result = *tc.Result
			}

			if tc.IsError != nil {
				detail.IsError = *tc.IsError
			}

			history.ToolCalls = append(history.ToolCalls, detail)
			history.ScreenshotURLs = append(history.ScreenshotURLs, tc.ScreenshotUrls...)
		}

		// Extract final response
		if msg.AssistantResponse != nil {
			history.FinalResponse = *msg.AssistantResponse
		}

		// Extract reasoning
		if msg.Reasoning != nil {
			history.Reasoning = *msg.Reasoning
		}
	}

	return history
}

// extractToolName extracts the string name from ToolName union type
func extractToolName(toolName components.ToolName) string {
	// ToolName is a discriminated union: either CoreToolName or Str
	if toolName.CoreToolName != nil {
		return string(*toolName.CoreToolName)
	}
	if toolName.Str != nil {
		return *toolName.Str
	}
	return "unknown"
}
