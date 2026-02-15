package orchestrator

import (
	"bufio"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/recreate-run/mix-go-sdk"
	"github.com/recreate-run/mix-go-sdk/models/operations"

	"mix-eval-go/pkg/convex"
	"mix-eval-go/pkg/providers"
)

// Orchestrator manages the evaluation pipeline
type Orchestrator struct {
	mixClient    *mix.Mix
	convexClient *convex.Client
	judge        *JudgeAnthropic
	config       Config
}

// Config holds orchestrator configuration
type Config struct {
	MixURL          string
	ConvexURL       string
	ConvexSecretKey string
	BrowserbaseKey  string
	BrightdataUser  string
	BrightdataPass  string
	AnthropicAPIKey string
	AnthropicModel  anthropic.Model
}

// New creates a new orchestrator instance
func New(config Config) *Orchestrator {
	return &Orchestrator{
		mixClient:    mix.New(config.MixURL, mix.WithTimeout(30*time.Second)),
		convexClient: convex.NewClient(config.ConvexURL, config.ConvexSecretKey),
		judge:        NewJudgeAnthropic(config.AnthropicAPIKey, config.AnthropicModel),
		config:       config,
	}
}

// FetchTasks fetches tasks from Convex
func (o *Orchestrator) FetchTasks(ctx context.Context, testCaseName string) ([]convex.Task, error) {
	return o.convexClient.FetchTestCase(ctx, testCaseName)
}

// RunTask executes a single evaluation task
func (o *Orchestrator) RunTask(ctx context.Context, task convex.Task) (*convex.TaskResult, error) {
	// Auto-generate runID if not provided
	if task.RunID == "" {
		task.RunID = fmt.Sprintf("run-%d", time.Now().Unix())
		fmt.Printf("Auto-generated run ID: %s\n", task.RunID)
	}

	fmt.Printf("Starting task: %s\n", task.ID)

	// 1. Create browser session if needed
	var cdpURL string
	var browserSession *providers.BrowserSession

	if task.BrowserProvider != "" {
		var err error
		browserSession, err = o.createBrowserSession(task.BrowserProvider)
		if err != nil {
			return nil, fmt.Errorf("browser session creation failed: %w", err)
		}
		defer o.closeBrowserSession(browserSession)

		cdpURL = browserSession.CDPURL
		fmt.Printf("Created browser session: %s\n", cdpURL)
	}

	// 2. Create Mix session with CDP URL (if cloud browser was created)
	browserMode := operations.BrowserModeLocalBrowserService
	var cdpURLPtr *string
	if cdpURL != "" {
		browserMode = operations.BrowserModeRemoteCdpWebsocket
		cdpURLPtr = &cdpURL
	}

	sessionResp, err := o.mixClient.Sessions.CreateSession(ctx, operations.CreateSessionRequest{
		Title:       fmt.Sprintf("Eval: %s", task.ID),
		BrowserMode: browserMode,
		CdpURL:      cdpURLPtr,
	})
	if err != nil {
		return nil, fmt.Errorf("session creation failed: %w", err)
	}

	sessionID := sessionResp.SessionData.ID

	fmt.Printf("Created Mix session: %s (preserved for file access)\n", sessionID)

	// 3. Start SSE event stream (manual HTTP, SDK has bug)
	eventsChan := make(chan map[string]interface{}, 100)
	var streamWg sync.WaitGroup
	streamWg.Add(1)

	go func() {
		defer streamWg.Done()
		o.streamEvents(ctx, sessionID, eventsChan)
	}()

	// Wait for stream to connect
	time.Sleep(1 * time.Second)

	// 4. Send task message
	_, err = o.mixClient.Messages.SendMessage(ctx, sessionID, operations.SendMessageRequestBody{
		Text: task.Text,
	})
	if err != nil {
		return nil, fmt.Errorf("send message failed: %w", err)
	}

	// 5. Collect events until completion
	toolCalls, screenshots := o.collectEvents(eventsChan)
	streamWg.Wait()

	fmt.Printf("Collected %d tool calls, %d screenshots\n", len(toolCalls), len(screenshots))

	// 6. Get complete message history
	messagesResp, err := o.mixClient.Messages.GetSessionMessages(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("get messages failed: %w", err)
	}

	// 7. Extract and format history
	history := extractHistory(messagesResp.BackendMessages)

	// 8. Convert data for new judge format
	judgeToolCalls := convertToJudgeToolCalls(history.ToolCalls)
	screenshotsB64 := convertScreenshotsToBase64(screenshots)
	var intermediateReasoning []string
	if history.Reasoning != "" {
		intermediateReasoning = []string{history.Reasoning}
	}

	// 9. Judge evaluation
	evaluation, err := o.judge.Evaluate(
		ctx,
		task,
		judgeToolCalls,
		[]SandboxFile{}, // No sandbox files from Mix yet
		history.FinalResponse,
		intermediateReasoning,
		nil, // No screenshot file paths
		screenshotsB64,
	)
	if err != nil {
		return nil, fmt.Errorf("evaluation failed: %w", err)
	}

	fmt.Printf("Evaluation: Score=%.2f, Passed=%v\n", evaluation.Score, evaluation.Passed)

	// 10. Upload screenshots
	storageIDs, _ := o.convexClient.UploadScreenshots(ctx, screenshots)

	// 11. Build result
	result := &convex.TaskResult{
		RunID:                task.RunID,
		TaskID:               task.ID,
		Task:                 task.Text,
		ToolCalls:            toolCalls,
		ScreenshotStorageIDs: storageIDs,
		FinalResponse:        history.FinalResponse,
		Evaluation:           evaluation,
	}

	return result, nil
}

// streamEvents handles SSE event streaming using manual HTTP (SDK has bug)
func (o *Orchestrator) streamEvents(ctx context.Context, sessionID string, ch chan map[string]interface{}) {
	defer close(ch)

	streamURL := fmt.Sprintf("%s/stream?sessionId=%s", o.config.MixURL, sessionID)

	req, err := http.NewRequestWithContext(ctx, "GET", streamURL, nil)
	if err != nil {
		fmt.Printf("Stream request creation failed: %v\n", err)
		return
	}
	req.Header.Set("Accept", "text/event-stream")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		fmt.Printf("Stream request failed: %v\n", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		fmt.Printf("Stream returned status %d\n", resp.StatusCode)
		return
	}

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

			ch <- event

			// Check for completion event
			if eventType, ok := event["type"].(string); ok && eventType == "complete" {
				return
			}
		}
	}

	if err := scanner.Err(); err != nil {
		fmt.Printf("Scanner error: %v\n", err)
	}
}

// toolCallInfo tracks tool call details during streaming
type toolCallInfo struct {
	ID          string
	Name        string
	Description string
	Parameters  map[string]interface{}
}

// collectEvents processes SSE events
func (o *Orchestrator) collectEvents(eventsChan chan map[string]interface{}) ([]convex.ToolCall, [][]byte) {
	var toolCalls []convex.ToolCall
	var screenshots [][]byte
	toolCallsMap := make(map[string]*toolCallInfo)
	var thinkingActive bool

	for event := range eventsChan {
		eventType, _ := event["type"].(string)

		switch eventType {
		case "tool_use_start":
			// Capture initial tool call metadata (data is directly in event, not nested)
			toolID, _ := event["id"].(string)
			toolName, _ := event["name"].(string)
			if toolID == "" {
				toolID = fmt.Sprintf("%s-%d", toolName, len(toolCallsMap))
			}
			toolCallsMap[toolID] = &toolCallInfo{
				ID:          toolID,
				Name:        toolName,
				Description: toolName,
				Parameters:  make(map[string]interface{}),
			}

		case "tool_use_parameter_streaming_complete":
			// Capture full parameters (data is directly in event)
			toolID, _ := event["id"].(string)
			input := event["input"]

			if toolInfo, exists := toolCallsMap[toolID]; exists {
				// Parse input as JSON if it's a string
				if inputStr, ok := input.(string); ok {
					var params map[string]interface{}
					if err := json.Unmarshal([]byte(inputStr), &params); err == nil {
						toolInfo.Parameters = params
					} else {
						toolInfo.Parameters = map[string]interface{}{"input": inputStr}
					}
				} else if params, ok := input.(map[string]interface{}); ok {
					toolInfo.Parameters = params
				}
			}

		case "tool_execution_start":
			// Display tool call with full details (data is directly in event)
			toolID, _ := event["toolCallId"].(string)
			progress, _ := event["progress"].(string)

			if toolInfo, exists := toolCallsMap[toolID]; exists {
				fmt.Printf("\n🔧 %s\n", toolInfo.Name)
				if progress != "" && progress != toolInfo.Name {
					fmt.Printf("   %s\n", progress)
				}
				if len(toolInfo.Parameters) > 0 {
					paramsJSON, _ := json.MarshalIndent(toolInfo.Parameters, "   ", "  ")
					fmt.Printf("   Parameters: %s\n", string(paramsJSON))
				}
			} else {
				// Fallback if tool not in map
				fmt.Printf("\n🔧 Tool\n")
				if progress != "" {
					fmt.Printf("   %s\n", progress)
				}
			}

		case "tool_execution_complete":
			// Handle tool completion (data is directly in event)
			toolID, _ := event["toolCallId"].(string)
			success, _ := event["success"].(bool)
			progress, _ := event["progress"].(string)

			var toolName string
			if toolInfo, exists := toolCallsMap[toolID]; exists {
				toolName = toolInfo.Name
			}

			if toolName != "" {
				toolCalls = append(toolCalls, convex.ToolCall{
					ToolName: toolName,
					Result:   progress,
					IsError:  !success,
				})

				// Display completion status
				if success {
					fmt.Printf("   ✓ Completed\n")
				} else {
					fmt.Printf("   ✗ Failed: %s\n", progress)
				}
			}

		case "thinking":
			// Thinking content is directly in event["content"]
			content, _ := event["content"].(string)
			if content != "" {
				if !thinkingActive {
					fmt.Print("\n\033[90m") // Start gray text
					thinkingActive = true
				}
				fmt.Print(content)
			}

		case "content":
			// Content is directly in event["content"]
			content, _ := event["content"].(string)
			if content != "" {
				if thinkingActive {
					fmt.Print("\033[0m\n") // End gray text
					thinkingActive = false
				}
				fmt.Print(content)
			}

		case "error":
			if thinkingActive {
				fmt.Print("\033[0m\n") // End gray text
				thinkingActive = false
			}
			errMsg, _ := event["error"].(string)
			if errMsg != "" {
				fmt.Printf("\n❌ Error: %s\n", errMsg)
			}
		}
	}

	// Clean up any dangling gray text
	if thinkingActive {
		fmt.Print("\033[0m\n")
	}

	return toolCalls, screenshots
}

// createBrowserSession creates browser session based on provider
func (o *Orchestrator) createBrowserSession(provider string) (*providers.BrowserSession, error) {
	switch provider {
	case "browserbase":
		return providers.CreateBrowserbaseSession(o.config.BrowserbaseKey, "default-project")
	case "brightdata":
		return providers.CreateBrightdataSession(o.config.BrightdataUser, o.config.BrightdataPass)
	default:
		return nil, fmt.Errorf("unknown browser provider: %s", provider)
	}
}

// closeBrowserSession closes browser session
func (o *Orchestrator) closeBrowserSession(session *providers.BrowserSession) {
	switch session.Provider {
	case providers.ProviderBrowserbase:
		providers.CloseBrowserSession(session, o.config.BrowserbaseKey)
	}
}

// RunMultipleTasks runs multiple tasks in parallel
func (o *Orchestrator) RunMultipleTasks(ctx context.Context, tasks []convex.Task, parallelism int) error {
	var wg sync.WaitGroup
	sem := make(chan struct{}, parallelism)

	for _, task := range tasks {
		wg.Add(1)
		sem <- struct{}{}

		go func(t convex.Task) {
			defer wg.Done()
			defer func() { <-sem }()

			result, err := o.RunTask(ctx, t)
			if err != nil {
				fmt.Printf("Task %s failed: %v\n", t.ID, err)
				return
			}

			if err := o.convexClient.SaveTaskResult(ctx, result); err != nil {
				fmt.Printf("Failed to save result for %s: %v\n", t.ID, err)
			} else {
				fmt.Printf("Task %s completed: Score=%.2f\n", t.ID, result.Evaluation.Score)
			}
		}(task)
	}

	wg.Wait()
	return nil
}

// convertToJudgeToolCalls converts ToolCallDetail to ToolCall for the judge
func convertToJudgeToolCalls(details []ToolCallDetail) []ToolCall {
	toolCalls := make([]ToolCall, len(details))
	for i, detail := range details {
		// Parse input as arguments
		var args map[string]interface{}
		if err := json.Unmarshal([]byte(detail.Input), &args); err != nil {
			// If input is not JSON, store as raw string
			args = map[string]interface{}{"input": detail.Input}
		}

		toolCalls[i] = ToolCall{
			ToolName:  detail.Name,
			Arguments: args,
			Result:    detail.Result,
			IsError:   detail.IsError,
		}
	}
	return toolCalls
}

// convertScreenshotsToBase64 converts screenshot bytes to base64 strings
func convertScreenshotsToBase64(screenshots [][]byte) []string {
	b64Strings := make([]string, len(screenshots))
	for i, screenshot := range screenshots {
		// Encode as base64 (screenshots are typically PNG format from browser)
		b64 := base64.StdEncoding.EncodeToString(screenshot)
		b64Strings[i] = b64
	}
	return b64Strings
}
