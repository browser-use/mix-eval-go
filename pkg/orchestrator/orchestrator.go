package orchestrator

import (
	"context"
	"fmt"
	"sync"
	"time"

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
	judge        *Judge
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
}

// New creates a new orchestrator instance
func New(config Config) *Orchestrator {
	return &Orchestrator{
		mixClient:    mix.New(config.MixURL, mix.WithTimeout(30*time.Second)),
		convexClient: convex.NewClient(config.ConvexURL, config.ConvexSecretKey),
		judge:        NewJudge(config.MixURL),
		config:       config,
	}
}

// FetchTasks fetches tasks from Convex
func (o *Orchestrator) FetchTasks(ctx context.Context, testCaseName string) ([]convex.Task, error) {
	return o.convexClient.FetchTestCase(ctx, testCaseName)
}

// RunTask executes a single evaluation task
func (o *Orchestrator) RunTask(ctx context.Context, task convex.Task) (*convex.TaskResult, error) {
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
	defer o.mixClient.Sessions.DeleteSession(ctx, sessionID)

	fmt.Printf("Created Mix session: %s\n", sessionID)

	// 3. Start SSE event stream
	eventsChan := make(chan *components.SSEEventStream, 100)
	var streamWg sync.WaitGroup
	streamWg.Add(1)

	go func() {
		defer streamWg.Done()
		o.streamEvents(ctx, sessionID, eventsChan)
	}()

	// Brief delay to ensure stream is connected
	time.Sleep(500 * time.Millisecond)

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

	// 8. Judge evaluation
	evaluation, err := o.judge.Evaluate(ctx, task, history)
	if err != nil {
		return nil, fmt.Errorf("evaluation failed: %w", err)
	}

	fmt.Printf("Evaluation: Score=%.2f, Passed=%v\n", evaluation.Score, evaluation.Passed)

	// 9. Upload screenshots
	storageIDs, _ := o.convexClient.UploadScreenshots(ctx, screenshots)

	// 10. Build result
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

// streamEvents handles SSE event streaming
func (o *Orchestrator) streamEvents(ctx context.Context, sessionID string, ch chan *components.SSEEventStream) {
	defer close(ch)

	streamResp, err := o.mixClient.Streaming.StreamEvents(ctx, sessionID, nil)
	if err != nil {
		fmt.Printf("Stream error: %v\n", err)
		return
	}
	defer streamResp.SSEEventStream.Close()

	for streamResp.SSEEventStream.Next() {
		event := streamResp.SSEEventStream.Value()
		if event == nil {
			continue
		}

		ch <- event

		// Check for completion event
		if event.Type == components.SSEEventStreamTypeSSECompleteEvent {
			return
		}
	}
}

// collectEvents processes SSE events
func (o *Orchestrator) collectEvents(eventsChan chan *components.SSEEventStream) ([]convex.ToolCall, [][]byte) {
	var toolCalls []convex.ToolCall
	var screenshots [][]byte

	for event := range eventsChan {
		switch event.Type {
		case components.SSEEventStreamTypeSSEToolExecutionCompleteEvent:
			if event.SSEToolExecutionCompleteEvent != nil {
				toolCalls = append(toolCalls, convex.ToolCall{
					ToolName: extractToolName(event.SSEToolExecutionCompleteEvent.Data.ToolName),
					Result:   event.SSEToolExecutionCompleteEvent.Data.Progress,
					IsError:  !event.SSEToolExecutionCompleteEvent.Data.Success,
				})
			}
		case components.SSEEventStreamTypeSSEThinkingEvent:
			fmt.Println("💭 Agent thinking...")
		case components.SSEEventStreamTypeSSEContentEvent:
			if event.SSEContentEvent != nil {
				fmt.Printf("💬 %s\n", event.SSEContentEvent.Data.Content)
			}
		case components.SSEEventStreamTypeSSEToolExecutionStartEvent:
			if event.SSEToolExecutionStartEvent != nil {
				fmt.Printf("🔧 Tool: %s\n", extractToolName(event.SSEToolExecutionStartEvent.Data.ToolName))
			}
		case components.SSEEventStreamTypeSSEErrorEvent:
			if event.SSEErrorEvent != nil {
				fmt.Printf("❌ Error: %s\n", event.SSEErrorEvent.Data.Error)
			}
		}
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
