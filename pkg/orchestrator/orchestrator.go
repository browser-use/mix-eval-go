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
	"github.com/recreate-run/mix-go-sdk/models/components"
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

// ANSI color codes
const (
	ANSIColorGray  = "\033[90m"
	ANSIColorReset = "\033[0m"
)

// SSEEvent represents a base SSE event structure
type SSEEvent struct {
	Type               string `json:"type"`
	AssistantMessageID string `json:"assistantMessageId,omitempty"`
}

// ToolUseStartEvent represents tool_use_start event
type ToolUseStartEvent struct {
	SSEEvent
	ID   string `json:"id"`
	Name string `json:"name"`
}

// ToolUseParameterStreamingCompleteEvent represents tool parameter completion
type ToolUseParameterStreamingCompleteEvent struct {
	SSEEvent
	ID    string      `json:"id"`
	Name  string      `json:"name"`
	Input interface{} `json:"input"`
}

// ToolExecutionStartEvent represents tool execution start
type ToolExecutionStartEvent struct {
	SSEEvent
	ToolCallID string `json:"toolCallId"`
	ToolName   string `json:"toolName"`
	Progress   string `json:"progress"`
}

// ToolExecutionCompleteEvent represents tool execution completion
type ToolExecutionCompleteEvent struct {
	SSEEvent
	ToolCallID string `json:"toolCallId"`
	ToolName   string `json:"toolName"`
	Success    bool   `json:"success"`
	Progress   string `json:"progress"`
}

// ThinkingEvent represents thinking event
type ThinkingEvent struct {
	SSEEvent
	Content string `json:"content"`
}

// ContentEvent represents content event
type ContentEvent struct {
	SSEEvent
	Content string `json:"content"`
}

// ErrorEvent represents error event
type ErrorEvent struct {
	SSEEvent
	Error string `json:"error"`
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
			if eventType, ok := event["type"].(string); ok && eventType == string(components.SSEEventStreamTypeComplete) {
				return
			}
		}
	}

	if err := scanner.Err(); err != nil {
		fmt.Printf("Scanner error: %v\n", err)
	}
}

// ToolCallInfo tracks tool call details during streaming
type ToolCallInfo struct {
	ID          string
	Name        string
	Description string
	Parameters  string // JSON string representation
}

// parseEvent parses raw event map into typed event
func parseEvent(eventData []byte, eventType string) interface{} {
	switch eventType {
	case string(components.SSEEventStreamTypeToolUseStart):
		var evt ToolUseStartEvent
		if err := json.Unmarshal(eventData, &evt); err == nil {
			return &evt
		}
	case string(components.SSEEventStreamTypeToolUseParameterStreamingComplete):
		var evt ToolUseParameterStreamingCompleteEvent
		if err := json.Unmarshal(eventData, &evt); err == nil {
			return &evt
		}
	case string(components.SSEEventStreamTypeToolExecutionStart):
		var evt ToolExecutionStartEvent
		if err := json.Unmarshal(eventData, &evt); err == nil {
			return &evt
		}
	case string(components.SSEEventStreamTypeToolExecutionComplete):
		var evt ToolExecutionCompleteEvent
		if err := json.Unmarshal(eventData, &evt); err == nil {
			return &evt
		}
	case string(components.SSEEventStreamTypeThinking):
		var evt ThinkingEvent
		if err := json.Unmarshal(eventData, &evt); err == nil {
			return &evt
		}
	case string(components.SSEEventStreamTypeContent):
		var evt ContentEvent
		if err := json.Unmarshal(eventData, &evt); err == nil {
			return &evt
		}
	case string(components.SSEEventStreamTypeError):
		var evt ErrorEvent
		if err := json.Unmarshal(eventData, &evt); err == nil {
			return &evt
		}
	}
	return nil
}

// collectEvents processes SSE events
func (o *Orchestrator) collectEvents(eventsChan chan map[string]interface{}) ([]convex.ToolCall, [][]byte) {
	var toolCalls []convex.ToolCall
	var screenshots [][]byte
	toolCallsMap := make(map[string]*ToolCallInfo)
	var thinkingActive bool

	for rawEvent := range eventsChan {
		eventType, _ := rawEvent["type"].(string)

		// Re-marshal and parse into typed struct
		eventData, err := json.Marshal(rawEvent)
		if err != nil {
			continue
		}
		typedEvent := parseEvent(eventData, eventType)
		if typedEvent == nil {
			continue
		}

		switch eventType {
		case string(components.SSEEventStreamTypeToolUseStart):
			evt := typedEvent.(*ToolUseStartEvent)
			toolID := evt.ID
			if toolID == "" {
				toolID = fmt.Sprintf("%s-%d", evt.Name, len(toolCallsMap))
			}
			toolCallsMap[toolID] = &ToolCallInfo{
				ID:          toolID,
				Name:        evt.Name,
				Description: evt.Name,
				Parameters:  "",
			}

		case string(components.SSEEventStreamTypeToolUseParameterStreamingComplete):
			evt := typedEvent.(*ToolUseParameterStreamingCompleteEvent)
			if toolInfo, exists := toolCallsMap[evt.ID]; exists {
				// Convert input to JSON string
				if evt.Input != nil {
					if inputStr, ok := evt.Input.(string); ok {
						// If already string, validate it's valid JSON
						var test interface{}
						if err := json.Unmarshal([]byte(inputStr), &test); err == nil {
							toolInfo.Parameters = inputStr
						} else {
							// Wrap in JSON object if not valid JSON
							wrapped, _ := json.Marshal(map[string]string{"input": inputStr})
							toolInfo.Parameters = string(wrapped)
						}
					} else {
						// Marshal to JSON string
						paramsJSON, _ := json.Marshal(evt.Input)
						toolInfo.Parameters = string(paramsJSON)
					}
				}
			}

		case string(components.SSEEventStreamTypeToolExecutionStart):
			evt := typedEvent.(*ToolExecutionStartEvent)
			if toolInfo, exists := toolCallsMap[evt.ToolCallID]; exists {
				fmt.Printf("\n🔧 %s\n", toolInfo.Name)
				if evt.Progress != "" && evt.Progress != toolInfo.Name {
					fmt.Printf("   %s\n", evt.Progress)
				}
				if toolInfo.Parameters != "" {
					var params map[string]interface{}
					if err := json.Unmarshal([]byte(toolInfo.Parameters), &params); err == nil {
						paramsJSON, _ := json.MarshalIndent(params, "   ", "  ")
						fmt.Printf("   Parameters: %s\n", string(paramsJSON))
					}
				}
			} else {
				// Fallback if tool not in map
				fmt.Printf("\n🔧 Tool\n")
				if evt.Progress != "" {
					fmt.Printf("   %s\n", evt.Progress)
				}
			}

		case string(components.SSEEventStreamTypeToolExecutionComplete):
			evt := typedEvent.(*ToolExecutionCompleteEvent)
			var toolName string
			if toolInfo, exists := toolCallsMap[evt.ToolCallID]; exists {
				toolName = toolInfo.Name
			}

			if toolName != "" {
				toolCalls = append(toolCalls, convex.ToolCall{
					ToolName: toolName,
					Result:   evt.Progress,
					IsError:  !evt.Success,
				})

				// Display completion status
				if evt.Success {
					fmt.Printf("   ✓ Completed\n")
				} else {
					fmt.Printf("   ✗ Failed: %s\n", evt.Progress)
				}
			}

		case string(components.SSEEventStreamTypeThinking):
			evt := typedEvent.(*ThinkingEvent)
			if evt.Content != "" {
				if !thinkingActive {
					fmt.Print("\n" + ANSIColorGray)
					thinkingActive = true
				}
				fmt.Print(evt.Content)
			}

		case string(components.SSEEventStreamTypeContent):
			evt := typedEvent.(*ContentEvent)
			if evt.Content != "" {
				if thinkingActive {
					fmt.Print(ANSIColorReset + "\n")
					thinkingActive = false
				}
				fmt.Print(evt.Content)
			}

		case string(components.SSEEventStreamTypeError):
			evt := typedEvent.(*ErrorEvent)
			if thinkingActive {
				fmt.Print(ANSIColorReset + "\n")
				thinkingActive = false
			}
			if evt.Error != "" {
				fmt.Printf("\n❌ Error: %s\n", evt.Error)
			}
		}
	}

	// Clean up any dangling gray text
	if thinkingActive {
		fmt.Print(ANSIColorReset + "\n")
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
