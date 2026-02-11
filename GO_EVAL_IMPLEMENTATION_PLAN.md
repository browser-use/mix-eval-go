# Native Go Evaluation System - Complete Implementation Plan

## Overview

Build a native Go evaluation system (`go-eval`) that uses the Mix agent as the execution engine and evaluates tasks from the same dataset as manus-eval. This eliminates the need to port all evaluation logic to Go while maintaining type safety and performance.

---

## Repository Locations

### Primary Repositories

| Repository | Absolute Path | Purpose |
|------------|---------------|---------|
| **manus-eval** (Python) | `/Users/sarathmenon/Documents/startup/image_generation/browser-use-trial/manus-eval` | Reference implementation, browser providers, judges |
| **Mix Agent** (Go) | `/Users/sarathmenon/Documents/startup/image_generation/mix/mix_agent` | Execution engine to be modified |
| **Mix Go SDK** | `/Users/sarathmenon/Documents/startup/image_generation/mix-go-sdk` | Client library for Mix API |
| **mix-browser-app** (Electron) | `/Users/sarathmenon/Documents/startup/image_generation/browser-use-trial/mix-browser-app` | Reference for Mix integration patterns |
| **go-eval** (NEW) | `/Users/sarathmenon/Documents/startup/image_generation/browser-use-trial/go-eval` | New evaluation orchestrator |

### Key Reference Files

#### Manus-Eval (Reference)
- Service orchestrator: `/Users/sarathmenon/Documents/startup/image_generation/browser-use-trial/manus-eval/manus_eval/service.py`
- Browser providers: `/Users/sarathmenon/Documents/startup/image_generation/browser-use-trial/manus-eval/manus_eval/browsers.py`
- Server API client: `/Users/sarathmenon/Documents/startup/image_generation/browser-use-trial/manus-eval/manus_eval/server.py`
- Judge implementation: `/Users/sarathmenon/Documents/startup/image_generation/browser-use-trial/manus-eval/manus_eval/judges/manus_judge.py`

#### Mix Agent (To Modify)
- Browser factory: `/Users/sarathmenon/Documents/startup/image_generation/mix/mix_agent/internal/browser/factory.go`
- Browser tool: `/Users/sarathmenon/Documents/startup/image_generation/mix/mix_agent/internal/llm/tools/browser/browser.go`
- REST sessions API: `/Users/sarathmenon/Documents/startup/image_generation/mix/mix_agent/internal/http/rest_sessions.go`
- Session storage: `/Users/sarathmenon/Documents/startup/image_generation/mix/mix_agent/internal/session/session.go`
- CDP spec: `/Users/sarathmenon/Documents/startup/image_generation/mix/mix_agent/REMOTE_CDP_SUPPORT_SPEC.md`

#### Mix Go SDK (Use As-Is)
- SDK documentation: `/Users/sarathmenon/Documents/startup/image_generation/mix-go-sdk/docs/golang_sdk_reference.md`
- Import path: `github.com/recreate-run/mix-go-sdk`

---

## Architecture Overview

```
┌─────────────────────────────────────────────────────────────┐
│                     GO-EVAL ORCHESTRATOR                    │
│                  (New Go Application)                       │
├─────────────────────────────────────────────────────────────┤
│                                                             │
│  1. Fetch tasks from Convex (HTTP API)                     │
│  2. Create browser sessions (cloud providers)               │
│  3. Create Mix sessions with CDP URL                        │
│  4. Stream agent execution events (SSE)                     │
│  5. Extract execution history                               │
│  6. Evaluate with Claude judge                              │
│  7. Submit results to Convex                                │
│                                                             │
└──────────┬──────────────┬──────────────┬───────────────────┘
           │              │              │
           │              │              └─> Convex API
           │              │                  (tasks/results)
           │              │
           │              └─> Browser Providers
           │                  (Browserbase, Brightdata, etc.)
           │
           └─> Mix Agent (HTTP + SSE)
               ├─> Mix Go SDK (type-safe client)
               ├─> Remote CDP support
               └─> Tool execution + history
```

---

## Phase 1: Mix Agent CDP Support (2-3 days)

### Objective
Enable Mix Agent to accept remote CDP URLs during session creation and use go-rod to connect to cloud browsers.

### Implementation Spec
Follow the complete specification at:
**`/Users/sarathmenon/Documents/startup/image_generation/mix/mix_agent/REMOTE_CDP_SUPPORT_SPEC.md`**

**Note:** This project now uses Mix Go SDK v0.2.1

### Files to Modify

#### 1. Browser Factory (`internal/browser/factory.go`)

**Location:** `/Users/sarathmenon/Documents/startup/image_generation/mix/mix_agent/internal/browser/factory.go`

**Changes:**
- Add `ModeCDP = "cdp"` constant
- Add `CDPURL string` field to `FactoryConfig`
- Add CDP case in `NewClient()` function

```go
// Add after existing mode constants (around line 15)
const (
    ModeTunnel  = "tunnel"
    ModeService = "service"
    ModeCDP     = "cdp"  // NEW
)

// Add to FactoryConfig struct (around line 30)
type FactoryConfig struct {
    Mode                string
    CDPURL              string  // NEW: Remote CDP WebSocket URL
    ConnectionManager   ConnectionManager
    TunnelRegistry      TunnelRegistry
    SessionID           string
}

// Add case in NewClient() (around line 70)
func NewClient(config FactoryConfig) (BrowserClient, error) {
    switch config.Mode {
    case ModeTunnel:
        // ... existing code
    case ModeService:
        // ... existing code
    case ModeCDP:  // NEW
        if config.CDPURL == "" {
            return nil, fmt.Errorf("CDP URL required for cdp mode")
        }
        return newRodClient(config.CDPURL)
    default:
        return nil, fmt.Errorf("unknown browser mode: %s", config.Mode)
    }
}
```

#### 2. Rod Client Implementation (NEW FILE)

**Location:** `/Users/sarathmenon/Documents/startup/image_generation/mix/mix_agent/internal/browser/rod_client.go`

**Create new file with ~300 lines:**

```go
package browser

import (
    "context"
    "encoding/base64"
    "fmt"
    "github.com/go-rod/rod"
    "github.com/go-rod/rod/lib/proto"
)

// RodClient implements BrowserClient using go-rod for remote CDP connections
type RodClient struct {
    browser *rod.Browser
    page    *rod.Page
}

// newRodClient connects to remote browser via CDP URL
func newRodClient(cdpURL string) (*RodClient, error) {
    browser := rod.New().ControlURL(cdpURL)
    err := browser.Connect()
    if err != nil {
        return nil, fmt.Errorf("failed to connect to CDP: %w", err)
    }

    // Create initial page
    page, err := browser.Page(proto.TargetCreateTarget{})
    if err != nil {
        browser.Close()
        return nil, fmt.Errorf("failed to create page: %w", err)
    }

    return &RodClient{
        browser: browser,
        page:    page,
    }, nil
}

// Navigate implements BrowserClient.Navigate
func (c *RodClient) Navigate(url, tabID string) (*NavigateResult, error) {
    err := c.page.Navigate(url)
    if err != nil {
        return nil, err
    }

    return &NavigateResult{
        FrameID: c.page.FrameID,
    }, nil
}

// Screenshot implements BrowserClient.Screenshot
func (c *RodClient) Screenshot(params ScreenshotParams) (*ScreenshotResult, error) {
    data, err := c.page.Screenshot(true, nil)
    if err != nil {
        return nil, err
    }

    return &ScreenshotResult{
        Data: base64.StdEncoding.EncodeToString(data),
    }, nil
}

// Click implements BrowserClient.Click
func (c *RodClient) Click(index int) (string, error) {
    // Get element by index from accessibility tree
    element, err := c.getElementByIndex(index)
    if err != nil {
        return "", err
    }

    err = element.Click(proto.InputMouseButtonLeft, 1)
    if err != nil {
        return "", fmt.Errorf("click failed: %w", err)
    }

    return "Click successful", nil
}

// ReadPage implements BrowserClient.ReadPage
func (c *RodClient) ReadPage(interactiveOnly bool) (*ReadPageResult, error) {
    // Use CDP Accessibility domain to get accessibility tree
    tree, err := c.page.Accessibility()
    if err != nil {
        return nil, err
    }

    // Convert to ReadPageResult format
    // ... implementation details

    return &ReadPageResult{
        Elements: elements,
    }, nil
}

// Type implements BrowserClient.Type
func (c *RodClient) Type(index int, text string) (string, error) {
    element, err := c.getElementByIndex(index)
    if err != nil {
        return "", err
    }

    err = element.Input(text)
    if err != nil {
        return "", fmt.Errorf("type failed: %w", err)
    }

    return "Type successful", nil
}

// Close implements BrowserClient.Close
func (c *RodClient) Close() error {
    if c.browser != nil {
        return c.browser.Close()
    }
    return nil
}

// Helper: Get element from accessibility tree by index
func (c *RodClient) getElementByIndex(index int) (*rod.Element, error) {
    // Implementation to map index to element
    // ... details
    return nil, fmt.Errorf("not implemented")
}

// Implement remaining BrowserClient interface methods:
// - ClickByBackendID
// - FormInput
// - Scroll
// - Wait
// - GetText
// - Find
// - CreateTab, ListTabs, SwitchTab, CloseTab
// - GoBack, GoForward
// - etc.
```

#### 3. Session API (`internal/http/rest_sessions.go`)

**Location:** `/Users/sarathmenon/Documents/startup/image_generation/mix/mix_agent/internal/http/rest_sessions.go`

**Changes:**

```go
// Add to CreateSessionRequest struct (around line 165)
type CreateSessionRequest struct {
    Title              string  `json:"title"`
    CustomSystemPrompt string  `json:"customSystemPrompt,omitempty"`
    PromptMode         string  `json:"promptMode,omitempty"`
    CdpUrl             string  `json:"cdpUrl,omitempty"`  // NEW
    // ... existing fields
}

// Add to SessionData struct (around line 23)
type SessionData struct {
    ID      string `json:"id"`
    Title   string `json:"title"`
    CdpUrl  string `json:"cdpUrl,omitempty"`  // NEW
    // ... existing fields
}

// Add validation in HandleCreateSession (after line 229)
func (h *Handler) HandleCreateSession(w http.ResponseWriter, r *http.Request) {
    // ... existing code to parse request

    // NEW: Validate CDP URL if provided
    if req.CdpUrl != "" {
        if !strings.HasPrefix(req.CdpUrl, "ws://") && !strings.HasPrefix(req.CdpUrl, "wss://") {
            http.Error(w, "CDP URL must start with ws:// or wss://", http.StatusBadRequest)
            return
        }

        _, err := url.Parse(req.CdpUrl)
        if err != nil {
            http.Error(w, "Invalid CDP URL format", http.StatusBadRequest)
            return
        }
    }

    // ... rest of handler
}
```

#### 4. Session Storage (`internal/session/session.go`)

**Location:** `/Users/sarathmenon/Documents/startup/image_generation/mix/mix_agent/internal/session/session.go`

**Changes:**

```go
// Add to Session struct (around line 92)
type Session struct {
    ID      string
    Title   string
    CdpUrl  string  // NEW: Remote CDP WebSocket URL
    // ... existing fields
}

// Update database schema to include cdp_url column
// Create migration file in database migrations directory
```

#### 5. Browser Tool (`internal/llm/tools/browser/browser.go`)

**Location:** `/Users/sarathmenon/Documents/startup/image_generation/mix/mix_agent/internal/llm/tools/browser/browser.go`

**Changes in `getClient()` method (around line 100-123):**

```go
func (t *browserTool) getClient(ctx context.Context) (BrowserClient, error) {
    // Get session from context
    session := getSessionFromContext(ctx)

    // NEW: Determine browser mode
    var mode string
    var cdpURL string

    if session.CdpUrl != "" {
        mode = browser.ModeCDP
        cdpURL = session.CdpUrl
    } else if t.browserMode == "tunnel" {
        mode = browser.ModeTunnel
    } else {
        mode = browser.ModeService
    }

    // Create client via factory
    client, err := browser.NewClient(browser.FactoryConfig{
        Mode:              mode,
        CDPURL:            cdpURL,  // NEW
        ConnectionManager: t.connectionManager,
        TunnelRegistry:    t.tunnelRegistry,
        SessionID:         session.ID,
    })

    return client, err
}
```

### Testing

Create integration test:
**Location:** `/Users/sarathmenon/Documents/startup/image_generation/mix/mix_agent/internal/browser/rod_client_test.go`

```go
package browser_test

import (
    "testing"
    "github.com/stretchr/testify/assert"
)

func TestRodClientWithCDP(t *testing.T) {
    // Mock CDP URL
    cdpURL := "ws://localhost:9222"

    client, err := browser.NewClient(browser.FactoryConfig{
        Mode:   browser.ModeCDP,
        CDPURL: cdpURL,
    })

    assert.NoError(t, err)
    assert.NotNil(t, client)

    defer client.Close()

    // Test basic operations
    result, err := client.Navigate("https://example.com", "")
    assert.NoError(t, err)
    assert.NotEmpty(t, result.FrameID)
}
```

---

## Phase 2: Browser Providers (1 day)

### Objective
Port browser provider session creation logic from Python to Go.

### Reference Implementation
**Python source:** `/Users/sarathmenon/Documents/startup/image_generation/browser-use-trial/manus-eval/manus_eval/browsers.py`

### Implementation

**Location:** `/Users/sarathmenon/Documents/startup/image_generation/browser-use-trial/go-eval/pkg/providers/browsers.go`

```go
package providers

import (
    "bytes"
    "encoding/json"
    "fmt"
    "net/http"
)

// BrowserProvider represents a cloud browser service
type BrowserProvider string

const (
    ProviderBrowserbase   BrowserProvider = "browserbase"
    ProviderBrightdata    BrowserProvider = "brightdata"
    ProviderHyperbrowser  BrowserProvider = "hyperbrowser"
    ProviderAnchorBrowser BrowserProvider = "anchor"
)

// BrowserSession contains CDP connection info
type BrowserSession struct {
    CDPURL     string
    SessionID  string
    Provider   BrowserProvider
}

// CreateBrowserbaseSession creates a Browserbase browser session
func CreateBrowserbaseSession(apiKey, projectID string) (*BrowserSession, error) {
    reqBody := map[string]string{
        "projectId": projectID,
    }

    body, _ := json.Marshal(reqBody)
    req, _ := http.NewRequest("POST", "https://www.browserbase.com/v1/sessions", bytes.NewBuffer(body))
    req.Header.Set("x-bb-api-key", apiKey)
    req.Header.Set("Content-Type", "application/json")

    resp, err := http.DefaultClient.Do(req)
    if err != nil {
        return nil, fmt.Errorf("browserbase API error: %w", err)
    }
    defer resp.Body.Close()

    if resp.StatusCode != 200 {
        return nil, fmt.Errorf("browserbase returned status %d", resp.StatusCode)
    }

    var result struct {
        ID         string `json:"id"`
        ConnectURL string `json:"connectUrl"`
    }

    if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
        return nil, err
    }

    return &BrowserSession{
        CDPURL:    result.ConnectURL,
        SessionID: result.ID,
        Provider:  ProviderBrowserbase,
    }, nil
}

// CreateBrightdataSession creates a Brightdata CDP session
func CreateBrightdataSession(username, password string) (*BrowserSession, error) {
    cdpURL := fmt.Sprintf("wss://%s:%s@brd.superproxy.io:9222", username, password)

    return &BrowserSession{
        CDPURL:    cdpURL,
        SessionID: "brightdata-session",
        Provider:  ProviderBrightdata,
    }, nil
}

// CreateHyperbrowserSession creates a Hyperbrowser session
func CreateHyperbrowserSession(apiKey string) (*BrowserSession, error) {
    reqBody := map[string]interface{}{
        "stealth": true,
    }

    body, _ := json.Marshal(reqBody)
    req, _ := http.NewRequest("POST", "https://cloud.hyperbrowser.ai/v1/sessions", bytes.NewBuffer(body))
    req.Header.Set("Authorization", "Bearer "+apiKey)
    req.Header.Set("Content-Type", "application/json")

    resp, err := http.DefaultClient.Do(req)
    if err != nil {
        return nil, fmt.Errorf("hyperbrowser API error: %w", err)
    }
    defer resp.Body.Close()

    var result struct {
        SessionID string `json:"sessionId"`
        CdpUrl    string `json:"cdpUrl"`
    }

    if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
        return nil, err
    }

    return &BrowserSession{
        CDPURL:    result.CdpUrl,
        SessionID: result.SessionID,
        Provider:  ProviderHyperbrowser,
    }, nil
}

// CreateAnchorBrowserSession creates an Anchor Browser session
func CreateAnchorBrowserSession(apiKey string, mobile bool) (*BrowserSession, error) {
    reqBody := map[string]interface{}{
        "mobile":         mobile,
        "captchaSolving": true,
    }

    body, _ := json.Marshal(reqBody)
    req, _ := http.NewRequest("POST", "https://api.anchorbrowser.io/browser", bytes.NewBuffer(body))
    req.Header.Set("Authorization", "Bearer "+apiKey)
    req.Header.Set("Content-Type", "application/json")

    resp, err := http.DefaultClient.Do(req)
    if err != nil {
        return nil, fmt.Errorf("anchor browser API error: %w", err)
    }
    defer resp.Body.Close()

    var result struct {
        SessionID string `json:"sessionId"`
        CdpUrl    string `json:"cdpUrl"`
    }

    if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
        return nil, err
    }

    return &BrowserSession{
        CDPURL:    result.CdpUrl,
        SessionID: result.SessionID,
        Provider:  ProviderAnchorBrowser,
    }, nil
}

// CloseBrowserSession closes a browser session (if provider supports it)
func CloseBrowserSession(session *BrowserSession, apiKey string) error {
    switch session.Provider {
    case ProviderBrowserbase:
        url := fmt.Sprintf("https://www.browserbase.com/v1/sessions/%s", session.SessionID)
        req, _ := http.NewRequest("DELETE", url, nil)
        req.Header.Set("x-bb-api-key", apiKey)
        _, err := http.DefaultClient.Do(req)
        return err
    case ProviderHyperbrowser:
        url := fmt.Sprintf("https://cloud.hyperbrowser.ai/v1/sessions/%s", session.SessionID)
        req, _ := http.NewRequest("DELETE", url, nil)
        req.Header.Set("Authorization", "Bearer "+apiKey)
        _, err := http.DefaultClient.Do(req)
        return err
    default:
        return nil // No cleanup needed
    }
}
```

---

## Phase 3: Convex Client (1 day)

### Objective
Create HTTP client for Convex evaluation platform API.

### Reference Implementation
**Python source:** `/Users/sarathmenon/Documents/startup/image_generation/browser-use-trial/manus-eval/manus_eval/server.py`

### Implementation

**Location:** `/Users/sarathmenon/Documents/startup/image_generation/browser-use-trial/go-eval/pkg/convex/client.go`

```go
package convex

import (
    "bytes"
    "context"
    "encoding/json"
    "fmt"
    "io"
    "net/http"
    "time"
)

// Client handles communication with Convex evaluation platform
type Client struct {
    baseURL   string
    secretKey string
    client    *http.Client
}

// NewClient creates a Convex API client
func NewClient(baseURL, secretKey string) *Client {
    return &Client{
        baseURL:   baseURL,
        secretKey: secretKey,
        client: &http.Client{
            Timeout: 120 * time.Second,
        },
    }
}

// Task represents an evaluation task
type Task struct {
    ID              string                 `json:"taskId"`
    Text            string                 `json:"task"`
    Website         string                 `json:"website,omitempty"`
    LoginCookie     string                 `json:"loginCookie,omitempty"`
    OutputSchema    map[string]interface{} `json:"outputSchema,omitempty"`
    BrowserProvider string                 `json:"browserProvider,omitempty"`
}

// FetchTestCase fetches tasks from Convex
func (c *Client) FetchTestCase(ctx context.Context, testCaseName string) ([]Task, error) {
    url := fmt.Sprintf("%s/api/getTestCase", c.baseURL)

    payload := map[string]string{
        "name": testCaseName,
    }

    body, _ := json.Marshal(payload)
    req, _ := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(body))
    req.Header.Set("Authorization", "Bearer "+c.secretKey)
    req.Header.Set("Content-Type", "application/json")

    // Retry logic
    var resp *http.Response
    var err error
    for attempt := 0; attempt < 5; attempt++ {
        resp, err = c.client.Do(req)
        if err == nil {
            break
        }

        backoff := time.Duration(1<<attempt) * time.Second
        time.Sleep(backoff)
    }

    if err != nil {
        return nil, fmt.Errorf("fetch test case failed: %w", err)
    }
    defer resp.Body.Close()

    if resp.StatusCode != 200 {
        bodyBytes, _ := io.ReadAll(resp.Body)
        return nil, fmt.Errorf("status %d: %s", resp.StatusCode, string(bodyBytes))
    }

    var tasks []Task
    if err := json.NewDecoder(resp.Body).Decode(&tasks); err != nil {
        return nil, err
    }

    return tasks, nil
}

// TaskResult represents evaluation result
type TaskResult struct {
    RunID                string                 `json:"runId"`
    TaskID               string                 `json:"taskId"`
    Task                 string                 `json:"task"`
    ToolCalls            []ToolCall             `json:"toolCalls"`
    ScreenshotStorageIDs []string               `json:"screenshotStorageIds"`
    FinalResponse        string                 `json:"finalResultResponse"`
    Evaluation           *Evaluation            `json:"comprehensiveJudgeEvaluation"`
    CompleteHistory      []map[string]interface{} `json:"completeHistory,omitempty"`
}

// ToolCall represents a tool execution
type ToolCall struct {
    ToolName string `json:"tool_name"`
    Args     string `json:"args"`
    Result   string `json:"result"`
    IsError  bool   `json:"is_error"`
}

// Evaluation represents judge evaluation
type Evaluation struct {
    Passed     bool     `json:"passed"`
    Score      float64  `json:"score"`
    Reasoning  string   `json:"reasoning"`
    Errors     []string `json:"error_categories"`
}

// SaveTaskResult submits result to Convex
func (c *Client) SaveTaskResult(ctx context.Context, result *TaskResult) error {
    url := fmt.Sprintf("%s/api/saveTaskResult", c.baseURL)

    body, _ := json.Marshal(result)

    // Log payload size
    payloadSize := len(body)
    if payloadSize > 5_000_000 {
        fmt.Printf("Warning: Large payload %d MB\n", payloadSize/1_000_000)
    }

    req, _ := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(body))
    req.Header.Set("Authorization", "Bearer "+c.secretKey)
    req.Header.Set("Content-Type", "application/json")

    resp, err := c.client.Do(req)
    if err != nil {
        return fmt.Errorf("save task result failed: %w", err)
    }
    defer resp.Body.Close()

    if resp.StatusCode != 200 {
        bodyBytes, _ := io.ReadAll(resp.Body)
        return fmt.Errorf("status %d: %s", resp.StatusCode, string(bodyBytes))
    }

    return nil
}

// UploadScreenshots uploads screenshots to Convex storage
func (c *Client) UploadScreenshots(ctx context.Context, screenshots [][]byte) ([]string, error) {
    var storageIDs []string

    for _, screenshot := range screenshots {
        // Get upload URL
        uploadURL, err := c.getUploadURL(ctx)
        if err != nil {
            continue
        }

        // Compress image to JPEG
        compressed := compressImageToJPEG(screenshot, 85)

        // Upload to storage
        storageID, err := c.uploadToStorage(ctx, uploadURL, compressed)
        if err != nil {
            continue
        }

        storageIDs = append(storageIDs, storageID)
    }

    return storageIDs, nil
}

func (c *Client) getUploadURL(ctx context.Context) (string, error) {
    url := fmt.Sprintf("%s/api/generateUploadUrl", c.baseURL)

    req, _ := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer([]byte("{}")))
    req.Header.Set("Authorization", "Bearer "+c.secretKey)
    req.Header.Set("Content-Type", "application/json")

    resp, err := c.client.Do(req)
    if err != nil {
        return "", err
    }
    defer resp.Body.Close()

    var result struct {
        UploadURL string `json:"uploadUrl"`
    }

    if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
        return "", err
    }

    return result.UploadURL, nil
}

func (c *Client) uploadToStorage(ctx context.Context, uploadURL string, data []byte) (string, error) {
    req, _ := http.NewRequestWithContext(ctx, "POST", uploadURL, bytes.NewBuffer(data))
    req.Header.Set("Content-Type", "image/jpeg")

    resp, err := c.client.Do(req)
    if err != nil {
        return "", err
    }
    defer resp.Body.Close()

    var result struct {
        StorageID string `json:"storageId"`
    }

    if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
        return "", err
    }

    return result.StorageID, nil
}

func compressImageToJPEG(data []byte, quality int) []byte {
    // TODO: Implement JPEG compression using image/jpeg package
    return data
}
```

---

## Phase 4: Go-Eval Orchestrator (1 week)

### Objective
Main orchestration logic using Mix Go SDK.

### SDK Location
**Import:** `github.com/recreate-run/mix-go-sdk`
**Documentation:** `/Users/sarathmenon/Documents/startup/image_generation/mix-go-sdk/docs/golang_sdk_reference.md`

### Implementation

**Location:** `/Users/sarathmenon/Documents/startup/image_generation/browser-use-trial/go-eval/pkg/orchestrator/orchestrator.go`

```go
package orchestrator

import (
    "context"
    "fmt"
    "sync"
    "time"

    "github.com/recreate-run/mix-go-sdk"
    "github.com/recreate-run/mix-go-sdk/models/components"
    "github.com/recreate-run/mix-go-sdk/models/operations"

    "go-eval/pkg/convex"
    "go-eval/pkg/providers"
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
    MixURL           string
    ConvexURL        string
    ConvexSecretKey  string
    BrowserbaseKey   string
    BrightdataUser   string
    BrightdataPass   string
}

// New creates a new orchestrator
func New(config Config) *Orchestrator {
    return &Orchestrator{
        mixClient:    mix.New(config.MixURL, mix.WithTimeout(30*time.Second)),
        convexClient: convex.NewClient(config.ConvexURL, config.ConvexSecretKey),
        judge:        NewJudge(config.MixURL),
        config:       config,
    }
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

    // 2. Create Mix session with CDP URL
    sessionResp, err := o.mixClient.Sessions.CreateSession(ctx, operations.CreateSessionRequest{
        Title:  fmt.Sprintf("Eval: %s", task.ID),
        CdpUrl: cdpURL,
    })
    if err != nil {
        return nil, fmt.Errorf("session creation failed: %w", err)
    }

    sessionID := sessionResp.SessionData.ID
    defer o.mixClient.Sessions.DeleteSession(ctx, sessionID)

    fmt.Printf("Created Mix session: %s\n", sessionID)

    // 3. Start SSE event stream in background
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

    // Wait for stream to finish
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
    storageIDs, err := o.convexClient.UploadScreenshots(ctx, screenshots)
    if err != nil {
        fmt.Printf("Screenshot upload warning: %v\n", err)
    }

    // 10. Build result
    result := &convex.TaskResult{
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

        // Exit on complete
        if event.Type == components.SSEEventStreamTypeComplete {
            return
        }
    }

    if err := streamResp.SSEEventStream.Err(); err != nil {
        fmt.Printf("Stream error: %v\n", err)
    }
}

// collectEvents processes SSE events and extracts tool calls and screenshots
func (o *Orchestrator) collectEvents(eventsChan chan *components.SSEEventStream) ([]convex.ToolCall, [][]byte) {
    var toolCalls []convex.ToolCall
    var screenshots [][]byte

    for event := range eventsChan {
        switch event.Type {
        case components.SSEEventStreamTypeThinking:
            fmt.Println("💭 Agent thinking...")

        case components.SSEEventStreamTypeContent:
            if event.SSEContentEvent != nil {
                fmt.Printf("💬 %s\n", event.SSEContentEvent.Data.Content)
            }

        case components.SSEEventStreamTypeToolExecutionStart:
            if event.SSEToolExecutionStartEvent != nil {
                fmt.Printf("🔧 Tool: %s\n", event.SSEToolExecutionStartEvent.Data.ToolName)
            }

        case components.SSEEventStreamTypeToolExecutionComplete:
            if event.SSEToolExecutionCompleteEvent != nil {
                toolCalls = append(toolCalls, convex.ToolCall{
                    ToolName: event.SSEToolExecutionCompleteEvent.Data.ToolName,
                    Args:     "", // TODO: Get from parameter events
                    Result:   event.SSEToolExecutionCompleteEvent.Data.Progress,
                    IsError:  !event.SSEToolExecutionCompleteEvent.Data.Success,
                })
            }

        case components.SSEEventStreamTypeComplete:
            fmt.Println("✅ Complete")
            return toolCalls, screenshots

        case components.SSEEventStreamTypeError:
            if event.SSEErrorEvent != nil {
                fmt.Printf("❌ Error: %s\n", event.SSEErrorEvent.Data.Error)
            }
        }
    }

    return toolCalls, screenshots
}

// createBrowserSession creates a browser session based on provider
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

// closeBrowserSession closes browser session if possible
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
        sem <- struct{}{} // Acquire

        go func(t convex.Task) {
            defer wg.Done()
            defer func() { <-sem }() // Release

            result, err := o.RunTask(ctx, t)
            if err != nil {
                fmt.Printf("Task %s failed: %v\n", t.ID, err)
                return
            }

            // Submit result to Convex
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
```

---

### History Extractor

**Location:** `/Users/sarathmenon/Documents/startup/image_generation/browser-use-trial/go-eval/pkg/orchestrator/extractor.go`

```go
package orchestrator

import (
    "github.com/recreate-run/mix-go-sdk/models/components"
)

// ExecutionHistory represents agent execution trace
type ExecutionHistory struct {
    ToolCalls     []ToolCallDetail
    FinalResponse string
    Reasoning     string
    TokensUsed    int64
    Cost          float64
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
                Name:     tc.Name,
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
```

---

### Judge Implementation

**Location:** `/Users/sarathmenon/Documents/startup/image_generation/browser-use-trial/go-eval/pkg/orchestrator/judge.go`

```go
package orchestrator

import (
    "context"
    "encoding/json"
    "fmt"
    "time"

    "github.com/recreate-run/mix-go-sdk"
    "github.com/recreate-run/mix-go-sdk/models/operations"

    "go-eval/pkg/convex"
)

// Judge evaluates task completion using Claude
type Judge struct {
    mixClient *mix.Mix
}

// NewJudge creates a new judge
func NewJudge(mixURL string) *Judge {
    return &Judge{
        mixClient: mix.New(mixURL),
    }
}

// Evaluate evaluates task completion
func (j *Judge) Evaluate(ctx context.Context, task convex.Task, history ExecutionHistory) (*convex.Evaluation, error) {
    // Create temporary judge session
    sessionResp, err := j.mixClient.Sessions.CreateSession(ctx, operations.CreateSessionRequest{
        Title: "Judge Session",
        // TODO: Configure to use Claude 3.5 Haiku via preferences API
    })
    if err != nil {
        return nil, err
    }

    sessionID := sessionResp.SessionData.ID
    defer j.mixClient.Sessions.DeleteSession(ctx, sessionID)

    // Build evaluation prompt
    prompt := j.buildEvaluationPrompt(task, history)

    // Send to judge
    _, err = j.mixClient.Messages.SendMessage(ctx, sessionID, operations.SendMessageRequestBody{
        Text: prompt,
    })
    if err != nil {
        return nil, err
    }

    // Wait for processing (TODO: use streaming for faster response)
    time.Sleep(5 * time.Second)

    // Get response
    messagesResp, err := j.mixClient.Messages.GetSessionMessages(ctx, sessionID)
    if err != nil {
        return nil, err
    }

    // Parse JSON evaluation
    var eval convex.Evaluation
    for _, msg := range messagesResp.BackendMessages {
        if msg.AssistantResponse != nil {
            // Try to extract JSON from response
            if err := json.Unmarshal([]byte(*msg.AssistantResponse), &eval); err != nil {
                // If not pure JSON, try to extract JSON block
                eval = j.extractJSONFromText(*msg.AssistantResponse)
            }
            break
        }
    }

    return &eval, nil
}

// buildEvaluationPrompt creates the evaluation prompt
func (j *Judge) buildEvaluationPrompt(task convex.Task, history ExecutionHistory) string {
    return fmt.Sprintf(`Evaluate whether the agent successfully completed this task:

Task: %s

Execution Summary:
- Tool calls: %d
- Final response: %s
- Reasoning provided: %v

Please analyze if the task was completed successfully and respond with JSON:
{
  "passed": true/false,
  "score": 0.0-1.0,
  "reasoning": "detailed explanation",
  "error_categories": ["error1", "error2"]
}

Be strict in your evaluation. Only mark as passed if the task was fully completed.`,
        task.Text,
        len(history.ToolCalls),
        history.FinalResponse,
        history.Reasoning != "",
    )
}

// extractJSONFromText extracts JSON from text response
func (j *Judge) extractJSONFromText(text string) convex.Evaluation {
    // TODO: Implement JSON extraction from markdown code blocks
    return convex.Evaluation{
        Passed:    false,
        Score:     0.0,
        Reasoning: "Failed to parse evaluation",
        Errors:    []string{"parse_error"},
    }
}
```

---

### Main CLI

**Location:** `/Users/sarathmenon/Documents/startup/image_generation/browser-use-trial/go-eval/cmd/main.go`

```go
package main

import (
    "context"
    "flag"
    "fmt"
    "log"
    "os"

    "go-eval/pkg/orchestrator"
)

func main() {
    // Parse flags
    testCaseName := flag.String("test-case", "", "Test case name to run")
    runID := flag.String("run-id", "", "Run ID")
    parallelism := flag.Int("parallel", 3, "Number of parallel tasks")
    flag.Parse()

    if *testCaseName == "" {
        log.Fatal("--test-case required")
    }

    // Load configuration
    config := orchestrator.Config{
        MixURL:          getEnv("MIX_AGENT_URL", "http://localhost:8088"),
        ConvexURL:       getEnv("CONVEX_URL", ""),
        ConvexSecretKey: getEnv("CONVEX_SECRET_KEY", ""),
        BrowserbaseKey:  getEnv("BROWSERBASE_API_KEY", ""),
        BrightdataUser:  getEnv("BRIGHTDATA_USER", ""),
        BrightdataPass:  getEnv("BRIGHTDATA_PASS", ""),
    }

    // Create orchestrator
    orch := orchestrator.New(config)

    ctx := context.Background()

    // Fetch tasks
    fmt.Printf("Fetching test case: %s\n", *testCaseName)
    tasks, err := orch.convexClient.FetchTestCase(ctx, *testCaseName)
    if err != nil {
        log.Fatalf("Failed to fetch tasks: %v", err)
    }

    fmt.Printf("Found %d tasks\n", len(tasks))

    // Set run ID on all tasks
    for i := range tasks {
        tasks[i].RunID = *runID
    }

    // Run tasks in parallel
    if err := orch.RunMultipleTasks(ctx, tasks, *parallelism); err != nil {
        log.Fatalf("Execution failed: %v", err)
    }

    fmt.Println("All tasks completed")
}

func getEnv(key, defaultValue string) string {
    if value := os.Getenv(key); value != "" {
        return value
    }
    return defaultValue
}
```

---

## Project Structure

```
/Users/sarathmenon/Documents/startup/image_generation/browser-use-trial/go-eval/
├── go.mod
├── go.sum
├── README.md
├── .env.example
├── cmd/
│   └── main.go                 # CLI entry point
├── pkg/
│   ├── orchestrator/
│   │   ├── orchestrator.go     # Main pipeline
│   │   ├── extractor.go        # History extraction
│   │   └── judge.go            # Claude evaluation
│   ├── convex/
│   │   └── client.go           # Convex HTTP API
│   └── providers/
│       └── browsers.go         # Browser providers
└── scripts/
    └── run-eval.sh             # Convenience script
```

---

## Dependencies (go.mod)

```go
module go-eval

go 1.21

require (
    github.com/recreate-run/mix-go-sdk v0.2.1
)
```

---

## Environment Variables (.env.example)

```bash
# Mix Agent
MIX_AGENT_URL=http://localhost:8088

# Convex
CONVEX_URL=https://your-deployment.convex.cloud
CONVEX_SECRET_KEY=your_secret_key

# Browser Providers
BROWSERBASE_API_KEY=your_browserbase_key
BROWSERBASE_PROJECT_ID=your_project_id
BRIGHTDATA_USER=your_username
BRIGHTDATA_PASS=your_password
HYPERBROWSER_API_KEY=your_hyperbrowser_key
ANCHOR_API_KEY=your_anchor_key
```

---

## Timeline

### Week 1: Mix CDP Support
- **Days 1-2:** Implement `rod_client.go` (~300 lines)
- **Day 3:** Modify factory, session API, browser tool (~100 lines)
- **Day 4:** Testing and integration
- **Day 5:** Buffer/documentation

### Week 2: Browser Providers + Convex Client
- **Days 1-2:** Port 7 browser providers from Python (~300 lines)
- **Days 3-4:** Implement Convex HTTP client (~250 lines)
- **Day 5:** Testing and error handling

### Week 3: Orchestrator + Judge
- **Days 1-2:** Main orchestrator with Mix SDK (~300 lines)
- **Days 3-4:** History extractor + Judge (~450 lines)
- **Day 5:** Integration testing

### Week 4: Testing + Polish
- **Days 1-2:** End-to-end testing with all browser providers
- **Day 3:** Parallel execution optimization
- **Day 4:** Error handling and retry logic
- **Day 5:** Documentation and deployment scripts

**Total: 4 weeks, ~1,800 lines of Go code**

---

## Success Metrics

1. ✅ Mix Agent accepts `cdpUrl` in session creation
2. ✅ Remote browsers work via CDP (Browserbase, Brightdata, etc.)
3. ✅ go-eval fetches tasks from Convex
4. ✅ Tasks execute via Mix Agent with proper event streaming
5. ✅ History extraction captures all tool calls
6. ✅ Judge evaluation produces accurate scores
7. ✅ Results submitted to Convex successfully
8. ✅ Parallel execution works (3+ concurrent tasks)
9. ✅ Zero regressions in Mix Agent (backward compatible)
10. ✅ Complete evaluation run on full dataset

---

## Testing Strategy

### Unit Tests
- Browser provider session creation
- Convex API client methods
- History extraction logic
- JSON parsing in judge

### Integration Tests
- Mix session with CDP URL
- SSE event streaming
- Message history retrieval
- Screenshot upload

### End-to-End Tests
- Single task execution (full pipeline)
- Parallel task execution
- Error recovery and retry
- Different browser providers

---

## Deployment

### Prerequisites
1. Mix Agent running with CDP support
2. Convex deployment with API credentials
3. Browser provider API keys

### Run Evaluation
```bash
cd /Users/sarathmenon/Documents/startup/image_generation/browser-use-trial/go-eval

# Build
go build -o bin/go-eval cmd/main.go

# Run
./bin/go-eval \
  --test-case "example_tasks" \
  --run-id "run_123" \
  --parallel 3
```

---

## Advantages Over Python Implementation

1. **Type Safety:** Compile-time guarantees via Go's type system
2. **Performance:** Native concurrency with goroutines
3. **Deployment:** Single binary, no dependencies
4. **Code Reuse:** Mix Agent handles 95% of complexity
5. **Maintainability:** 1,800 lines vs 4,000+ Python lines
6. **SDK Integration:** Type-safe Mix Go SDK
7. **Browser Flexibility:** Support for 7+ cloud providers

---

## Future Enhancements

1. **Metrics:** Prometheus metrics for monitoring
2. **Retries:** Automatic retry on transient failures
3. **Caching:** Cache browser sessions for speed
4. **Reporting:** HTML/JSON reports generation
5. **Webhooks:** Real-time progress notifications
6. **Dashboard:** Web UI for results visualization

---

## Support

For issues or questions:
- Mix Agent: `/Users/sarathmenon/Documents/startup/image_generation/mix/mix_agent/README.md`
- Mix Go SDK: `/Users/sarathmenon/Documents/startup/image_generation/mix-go-sdk/docs/golang_sdk_reference.md`
- Manus-Eval Reference: `/Users/sarathmenon/Documents/startup/image_generation/browser-use-trial/manus-eval/README.md`

---

**Version:** 1.0.0
**Last Updated:** 2025-02-11
**Status:** Ready for Implementation
