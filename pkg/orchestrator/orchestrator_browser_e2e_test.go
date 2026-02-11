//go:build e2e
// +build e2e

package orchestrator

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/recreate-run/mix-go-sdk"
	"github.com/recreate-run/mix-go-sdk/models/operations"
)

// TestEndToEndBrowserAutomation tests browser automation with Wikipedia extraction
// Requires Mix Agent running on http://localhost:8088
func TestEndToEndBrowserAutomation(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping e2e test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	mixClient := mix.New("http://localhost:8088", mix.WithTimeout(60*time.Second))

	// Test 1: Simple sanity check
	t.Run("SanityCheck-SimpleMath", func(t *testing.T) {
		sessionResp, err := mixClient.Sessions.CreateSession(ctx, operations.CreateSessionRequest{
			Title:       "Test: Simple Math",
			BrowserMode: operations.BrowserModeLocalBrowserService,
		})
		if err != nil {
			t.Fatalf("Failed to create session: %v", err)
		}

		sessionID := sessionResp.SessionData.ID
		t.Logf("Created session: %s", sessionID)

		defer func() {
			cleanupCtx, _ := context.WithTimeout(context.Background(), 5*time.Second)
			_, _ = mixClient.Sessions.DeleteSession(cleanupCtx, sessionID)
		}()

		// Stream events
		var contentEvents []string
		completionReceived := false
		streamWg := setupManualStream(t, ctx, sessionID, &contentEvents, nil, &completionReceived)

		// Send message
		_, err = mixClient.Messages.SendMessage(ctx, sessionID, operations.SendMessageRequestBody{
			Text: "What is 2+2? Just answer with the number.",
		})
		if err != nil {
			t.Fatalf("Failed to send message: %v", err)
		}

		streamWg.Wait()

		// Assertions
		if !completionReceived {
			t.Error("No completion event received")
		}
		if len(contentEvents) == 0 {
			t.Error("No content received")
		}

		content := strings.Join(contentEvents, "")
		if !strings.Contains(content, "4") {
			t.Errorf("Expected answer '4', got: %s", content)
		}

		t.Logf("✓ Math test passed. Content: %s", content)
	})

	// Test 2: Browser automation - Wikipedia cats
	t.Run("BrowserAutomation-WikipediaCats", func(t *testing.T) {
		sessionResp, err := mixClient.Sessions.CreateSession(ctx, operations.CreateSessionRequest{
			Title:       "Test: Wikipedia Cats",
			BrowserMode: operations.BrowserModeLocalBrowserService,
		})
		if err != nil {
			t.Fatalf("Failed to create session: %v", err)
		}

		sessionID := sessionResp.SessionData.ID
		t.Logf("Created session: %s", sessionID)

		defer func() {
			cleanupCtx, _ := context.WithTimeout(context.Background(), 5*time.Second)
			_, _ = mixClient.Sessions.DeleteSession(cleanupCtx, sessionID)
		}()

		// Stream events
		var contentEvents []string
		var toolCalls []string
		completionReceived := false
		streamWg := setupManualStream(t, ctx, sessionID, &contentEvents, &toolCalls, &completionReceived)

		// Send browser automation task
		t.Log("Sending Wikipedia extraction task...")
		_, err = mixClient.Messages.SendMessage(ctx, sessionID, operations.SendMessageRequestBody{
			Text: "Go to the Wikipedia page on cats and extract the intro paragraph. Tell me what you found.",
		})
		if err != nil {
			t.Fatalf("Failed to send message: %v", err)
		}

		// Wait for completion (browser automation takes longer)
		streamWg.Wait()

		// Fetch message history
		messagesResp, err := mixClient.Messages.GetSessionMessages(ctx, sessionID)
		if err != nil {
			t.Fatalf("Failed to get messages: %v", err)
		}

		// Assertions
		if !completionReceived {
			t.Error("No completion event received")
		}

		if len(messagesResp.BackendMessages) < 2 {
			t.Errorf("Expected at least 2 messages, got %d", len(messagesResp.BackendMessages))
		}

		if len(contentEvents) == 0 {
			t.Error("No content events received")
		}

		// Check for tool calls (browser automation should use tools)
		t.Logf("Tool calls detected: %v", toolCalls)

		// Check if the extracted content contains "Felis catus"
		fullContent := strings.Join(contentEvents, " ")
		t.Logf("Extracted content length: %d characters", len(fullContent))
		t.Logf("Content preview: %s...", truncate(fullContent, 200))

		if !strings.Contains(fullContent, "Felis catus") {
			t.Errorf("Expected content to contain 'Felis catus', but it was not found.\nFull content: %s", fullContent)
		} else {
			t.Log("✓ Successfully found 'Felis catus' in extracted content")
		}

		// Additional checks
		if !strings.Contains(strings.ToLower(fullContent), "cat") {
			t.Error("Expected content to mention 'cat'")
		}

		t.Logf("\n=== Browser Automation Test Summary ===")
		t.Logf("Session ID: %s", sessionID)
		t.Logf("Messages: %d", len(messagesResp.BackendMessages))
		t.Logf("Tool calls: %v", toolCalls)
		t.Logf("Content length: %d chars", len(fullContent))
		t.Logf("Contains 'Felis catus': %v", strings.Contains(fullContent, "Felis catus"))
	})
}

// setupManualStream sets up manual HTTP SSE streaming and returns WaitGroup
func setupManualStream(t *testing.T, ctx context.Context, sessionID string, contentEvents *[]string, toolCalls *[]string, completionReceived *bool) *sync.WaitGroup {
	streamURL := fmt.Sprintf("http://localhost:8088/stream?sessionId=%s", sessionID)

	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		defer wg.Done()

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

		t.Log("Stream connected")

		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			line := scanner.Text()

			if strings.HasPrefix(line, "data: ") {
				data := strings.TrimPrefix(line, "data: ")

				var event map[string]interface{}
				if err := json.Unmarshal([]byte(data), &event); err != nil {
					continue
				}

				eventType, _ := event["type"].(string)

				switch eventType {
				case "content":
					if content, ok := event["content"].(string); ok && contentEvents != nil {
						*contentEvents = append(*contentEvents, content)
					}
				case "tool_execution_start":
					if toolName, ok := event["toolName"].(string); ok && toolCalls != nil {
						*toolCalls = append(*toolCalls, toolName)
						t.Logf("Tool: %s", toolName)
					}
				case "complete":
					*completionReceived = true
					t.Log("Completion received")
					return
				}
			}
		}

		if err := scanner.Err(); err != nil {
			t.Logf("Scanner error: %v", err)
		}
	}()

	// Wait for stream to connect
	time.Sleep(1 * time.Second)

	return &wg
}

// truncate truncates a string to maxLen characters
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen]
}
