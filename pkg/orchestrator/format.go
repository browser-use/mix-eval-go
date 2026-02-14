package orchestrator

import (
	"encoding/json"
	"fmt"
	"strings"
)

// truncate truncates text to max length with ellipsis
func truncate(text string, maxLen int) string {
	if text == "" {
		return ""
	}
	if len(text) <= maxLen {
		return text
	}
	return text[:maxLen] + "..."
}

// formatToolCalls formats tool calls for the judge prompt
func formatToolCalls(toolCalls []ToolCall, maxCalls, maxArgs, maxResult int) string {
	if len(toolCalls) == 0 {
		return "No tool calls recorded"
	}

	// Take last N calls (most relevant)
	var subset []ToolCall
	var skipped int

	if len(toolCalls) > maxCalls {
		subset = toolCalls[len(toolCalls)-maxCalls:]
		skipped = len(toolCalls) - len(subset)
	} else {
		subset = toolCalls
	}

	var lines []string
	if skipped > 0 {
		lines = append(lines, fmt.Sprintf("[... %d earlier tool calls omitted ...]", skipped))
	}

	for _, tc := range subset {
		argsJSON, _ := json.Marshal(tc.Arguments)
		argsStr := truncate(string(argsJSON), maxArgs)
		result := truncate(tc.Result, maxResult)
		status := "OK"
		if tc.IsError {
			status = "ERROR"
		}
		lines = append(lines, fmt.Sprintf("[%s] %s(%s) -> %s", status, tc.ToolName, argsStr, result))
	}

	return strings.Join(lines, "\n")
}

// formatFiles formats sandbox files for the judge prompt
func formatFiles(files []SandboxFile, maxFiles int) string {
	if len(files) == 0 {
		return "No files created"
	}

	var lines []string
	for i, f := range files {
		if i >= maxFiles {
			break
		}
		preview := "[binary/empty]"
		if f.Content != "" {
			preview = truncate(f.Content, 500)
		}
		lines = append(lines, fmt.Sprintf("- %s (%d bytes): %s", f.Path, f.Size, preview))
	}

	if len(files) > maxFiles {
		lines = append(lines, fmt.Sprintf("... and %d more files", len(files)-maxFiles))
	}

	return strings.Join(lines, "\n")
}

// StepMetadata contains metadata about a single execution step
type StepMetadata struct {
	Index         int
	ToolName      string
	ResultLength  int
	URL           string
	Title         string
	OutputPreview string
	IsError       bool
	Preview       string
}

// buildStepIndex builds an index of all steps with metadata
func buildStepIndex(toolCalls []ToolCall) []StepMetadata {
	steps := make([]StepMetadata, 0, len(toolCalls))

	for i, tc := range toolCalls {
		step := StepMetadata{
			Index:        i,
			ToolName:     tc.ToolName,
			ResultLength: len(tc.Result),
			IsError:      tc.IsError,
			Preview:      truncate(tc.Result, 200),
		}

		// Extract URL if it's a browser_state call
		if tc.ToolName == "browser_state" && strings.Contains(tc.Result, "URL:") {
			lines := strings.Split(tc.Result, "\n")
			for _, line := range lines[:minInt(10, len(lines))] {
				trimmed := strings.TrimSpace(line)
				if strings.HasPrefix(trimmed, "URL:") {
					step.URL = truncate(strings.TrimSpace(strings.TrimPrefix(trimmed, "URL:")), 100)
				} else if strings.HasPrefix(trimmed, "Title:") {
					step.Title = truncate(strings.TrimSpace(strings.TrimPrefix(trimmed, "Title:")), 100)
				}
			}
		}

		// For python calls, show the result preview
		if tc.ToolName == "python" {
			step.OutputPreview = truncate(tc.Result, 500)
		}

		steps = append(steps, step)
	}

	return steps
}

// formatStepIndex formats the step index for the judge prompt
func formatStepIndex(steps []StepMetadata) string {
	var lines []string

	for _, s := range steps {
		status := "OK"
		if s.IsError {
			status = "ERROR"
		}

		// Highlight extraction steps
		extractionMarker := ""
		if s.ToolName == "python" {
			extractionMarker = " ⚡EXTRACTION"
		} else if s.ToolName == "done" || s.ToolName == "done_autonomous" {
			extractionMarker = " ✓COMPLETION"
		}

		info := fmt.Sprintf("[%d] [%s] %s%s (%d chars)", s.Index, status, s.ToolName, extractionMarker, s.ResultLength)
		if s.URL != "" {
			info += fmt.Sprintf(" - %s", s.URL)
		}
		if s.Title != "" {
			info += fmt.Sprintf(" (%s)", s.Title)
		}

		// Show output preview for python extraction steps
		if s.ToolName == "python" && s.OutputPreview != "" {
			info += fmt.Sprintf("\n     └─ Output: %s", s.OutputPreview)
		}

		lines = append(lines, info)
	}

	return strings.Join(lines, "\n")
}
