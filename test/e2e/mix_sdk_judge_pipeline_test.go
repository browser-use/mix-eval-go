//go:build e2e
// +build e2e

package e2e

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/recreate-run/mix-go-sdk"
	"github.com/recreate-run/mix-go-sdk/models/components"
	"github.com/recreate-run/mix-go-sdk/models/operations"

	"mix-eval-go/pkg/convex"
	"mix-eval-go/pkg/orchestrator"
)

// TestEndToEndSimpleTask tests simple task execution with SDK streaming
// Requires Mix Agent running on http://localhost:8088
func TestEndToEndSimpleTask(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping e2e test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// 1. Initialize Mix SDK client
	mixClient := mix.New("http://localhost:8088", mix.WithTimeout(30*time.Second))

	// 2. Create session
	t.Log("Creating Mix session...")
	sessionResp, err := mixClient.Sessions.CreateSession(ctx, operations.CreateSessionRequest{
		Title:       "SDK Test: Simple Math",
		BrowserMode: operations.BrowserModeLocalBrowserService,
	})
	if err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}

	sessionID := sessionResp.SessionData.ID
	t.Logf("Created session: %s", sessionID)

	// Cleanup: Delete session when test completes
	defer func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		_, err := mixClient.Sessions.DeleteSession(cleanupCtx, sessionID)
		if err != nil {
			t.Logf("Warning: Failed to delete session: %v", err)
		}
	}()

	// 3. Use manual HTTP streaming (SDK has bug)
	t.Log("Starting manual HTTP stream...")
	streamURL := fmt.Sprintf("http://localhost:8088/stream?sessionId=%s", sessionID)

	var contentEvents []string
	var errorEvents []string
	completionReceived := false
	var streamWg sync.WaitGroup
	streamWg.Add(1)

	go func() {
		defer streamWg.Done()

		req, err := http.NewRequestWithContext(ctx, "GET", streamURL, nil)
		if err != nil {
			t.Errorf("Failed to create stream request: %v", err)
			return
		}
		req.Header.Set("Accept", "text/event-stream")

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Errorf("Stream request failed: %v", err)
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("Stream returned status %d", resp.StatusCode)
			return
		}

		t.Log("Stream connected successfully")

		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			line := scanner.Text()

			// SSE format: data: {...}
			if strings.HasPrefix(line, "data: ") {
				data := strings.TrimPrefix(line, "data: ")

				var event map[string]interface{}
				if err := json.Unmarshal([]byte(data), &event); err != nil {
					continue
				}

				eventType, _ := event["type"].(string)
				t.Logf("Received event: %s", eventType)

				switch eventType {
				case "content":
					if content, ok := event["content"].(string); ok {
						contentEvents = append(contentEvents, content)
					}
				case "error":
					if errMsg, ok := event["error"].(string); ok {
						errorEvents = append(errorEvents, errMsg)
					}
				case "complete":
					completionReceived = true
					return
				}
			}
		}
	}()

	// Wait for stream to connect
	time.Sleep(1 * time.Second)

	// 4. Send simple task (same as curl test that worked)
	t.Log("Sending message...")
	taskMessage := "What is 2+2? Just answer with the number."
	_, err = mixClient.Messages.SendMessage(ctx, sessionID, operations.SendMessageRequestBody{
		Text: taskMessage,
	})
	if err != nil {
		t.Fatalf("Failed to send message: %v", err)
	}

	// 5. Wait for completion
	streamWg.Wait()
	t.Log("Event collection completed")

	// 6. Fetch message history
	t.Log("Fetching message history...")
	messagesResp, err := mixClient.Messages.GetSessionMessages(ctx, sessionID)
	if err != nil {
		t.Fatalf("Failed to get messages: %v", err)
	}

	// 7. Assertions
	t.Log("Running assertions...")

	// Assert: Completion event was received
	if !completionReceived {
		t.Error("No completion event received")
	}

	// Assert: Messages exist in history
	if messagesResp.BackendMessages == nil || len(messagesResp.BackendMessages) == 0 {
		t.Error("No messages in session history")
	} else {
		t.Logf("Total messages in history: %d", len(messagesResp.BackendMessages))
	}

	// Assert: At least 2 messages (user + assistant)
	if len(messagesResp.BackendMessages) < 2 {
		t.Errorf("Expected at least 2 messages, got %d", len(messagesResp.BackendMessages))
	}

	// Assert: No error events
	if len(errorEvents) > 0 {
		t.Errorf("Received %d error events: %v", len(errorEvents), errorEvents)
	}

	// Assert: Content was generated
	if len(contentEvents) == 0 {
		t.Error("No content events received - agent did not produce any output")
	} else {
		t.Logf("Content received: %v", strings.Join(contentEvents, ""))
	}

	// 8. Test Judge Evaluation
	t.Log("\n=== Testing Judge Evaluation ===")

	// Extract execution data from BackendMessages
	var toolCalls []orchestrator.ToolCall
	var finalResponse string

	for _, msg := range messagesResp.BackendMessages {
		// Extract tool calls
		for _, tc := range msg.ToolCalls {
			toolCall := orchestrator.ToolCall{
				ToolName:  extractToolName(tc.Name),
				Arguments: parseArguments(tc.Input),
				Result:    "",
				IsError:   false,
			}
			if tc.Result != nil {
				toolCall.Result = *tc.Result
			}
			if tc.IsError != nil {
				toolCall.IsError = *tc.IsError
			}
			toolCalls = append(toolCalls, toolCall)
		}

		// Extract final assistant response
		if msg.AssistantResponse != nil {
			finalResponse = *msg.AssistantResponse
		}
	}

	t.Logf("Extracted %d tool calls for judge evaluation", len(toolCalls))
	t.Logf("Final response for judge: %s", finalResponse)

	// Initialize judge
	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	if apiKey == "" {
		t.Skip("ANTHROPIC_API_KEY not set, skipping judge evaluation")
	}

	judge := orchestrator.NewJudgeAnthropic(apiKey, anthropic.ModelClaudeSonnet4_5_20250929)

	// Create task for judge
	task := convex.Task{
		Text: taskMessage,
	}

	// Evaluate with judge
	t.Log("Calling judge to evaluate agent execution...")
	eval, err := judge.Evaluate(
		ctx,
		task,
		toolCalls,
		[]orchestrator.SandboxFile{}, // No files for simple math task
		finalResponse,
		[]string{}, // No intermediate reasoning captured
		nil,        // No screenshot paths
		nil,        // No screenshot base64
	)

	if err != nil {
		t.Fatalf("Judge evaluation failed: %v", err)
	}

	// Assert judge verdict
	t.Logf("Judge verdict: passed=%v, score=%.2f", eval.Passed, eval.Score)
	t.Logf("Judge reasoning: %s", eval.Reasoning)

	if !eval.Passed {
		t.Errorf("Judge marked task as failed. Reasoning: %s", eval.Reasoning)
	}

	if eval.Score != 1.0 {
		t.Errorf("Expected judge score 1.0, got %.2f", eval.Score)
	}

	if eval.ImpossibleTask {
		t.Error("Judge incorrectly marked task as impossible")
	}

	if eval.ReachedCaptcha {
		t.Error("Judge incorrectly marked task as hitting CAPTCHA")
	}

	// Print summary
	t.Logf("\n=== Test Summary ===")
	t.Logf("Session ID: %s", sessionID)
	t.Logf("Agent Content: %s", strings.Join(contentEvents, ""))
	t.Logf("Messages in history: %d", len(messagesResp.BackendMessages))
	t.Logf("Completion received: %v", completionReceived)
	t.Logf("Judge Passed: %v (score: %.2f)", eval.Passed, eval.Score)
	t.Logf("✓ Full E2E test passed: Agent execution + Judge evaluation")
}

// Helper: extractToolName extracts string from ToolName union type
func extractToolName(toolName components.ToolName) string {
	if toolName.CoreToolName != nil {
		return string(*toolName.CoreToolName)
	}
	if toolName.Str != nil {
		return *toolName.Str
	}
	return "unknown"
}

// Helper: parseArguments parses JSON input string to map
func parseArguments(input string) map[string]interface{} {
	var args map[string]interface{}
	if err := json.Unmarshal([]byte(input), &args); err != nil {
		// If parsing fails, return empty map
		return map[string]interface{}{}
	}
	return args
}
