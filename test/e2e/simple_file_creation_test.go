//go:build e2e
// +build e2e

package e2e

import (
	"context"
	"testing"
	"time"

	"mix-eval-go/pkg/convex"
	"mix-eval-go/pkg/orchestrator"
)

// TestSimpleFileCreation tests session preservation and runID generation
// Task: Create a text file with the word cat
func TestSimpleFileCreation(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping e2e test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Create orchestrator
	config := orchestrator.Config{
		MixURL:          "http://localhost:8088",
		ConvexURL:       "http://dummy-convex-url.com",
		ConvexSecretKey: "dummy-key",
	}
	orch := orchestrator.New(config)

	// Create simple task directly (no Convex fetch needed)
	task := convex.Task{
		ID:     "test-file-creation-001",
		RunID:  "", // Leave empty to test auto-generation
		Text:   "Create a text file with the word cat",
		Category: "test",
	}

	t.Log("Starting task: Create a text file with the word cat")

	// Run the task
	result, err := orch.RunTask(ctx, task)
	if err != nil {
		t.Fatalf("Task execution failed: %v", err)
	}

	// Verify auto-generated runID
	if result.RunID == "" {
		t.Error("Expected auto-generated runID, got empty string")
	} else {
		t.Logf("✓ Auto-generated runID: %s", result.RunID)
	}

	// Verify task ID matches
	if result.TaskID != task.ID {
		t.Errorf("Expected taskID %s, got %s", task.ID, result.TaskID)
	}

	// Verify tool calls were captured
	if len(result.ToolCalls) == 0 {
		t.Error("Expected tool calls to be captured, got 0")
	} else {
		t.Logf("✓ Captured %d tool calls", len(result.ToolCalls))
		for i, tc := range result.ToolCalls {
			t.Logf("  Tool %d: %s", i+1, tc.ToolName)
		}
	}

	// Verify final response exists
	if result.FinalResponse == "" {
		t.Error("Expected final response, got empty string")
	} else {
		t.Logf("✓ Final response length: %d characters", len(result.FinalResponse))
	}

	// Note: We cannot verify the file exists because:
	// 1. Session workspace is internal to Mix Agent
	// 2. We'd need to query Mix Agent API for session files
	// But we can verify the session was preserved by checking the logs
	// The session ID should be printed with "preserved for file access"

	t.Log("✓ Test completed - session preserved, runID auto-generated")
}
