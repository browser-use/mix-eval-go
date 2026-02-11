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
	RunID           string                 `json:"runId,omitempty"`
	Text            string                 `json:"task"`
	Website         string                 `json:"website,omitempty"`
	LoginCookie     string                 `json:"loginCookie,omitempty"`
	OutputSchema    map[string]interface{} `json:"outputSchema,omitempty"`
	BrowserProvider string                 `json:"browserProvider,omitempty"`
}

// TaskResult represents evaluation result
type TaskResult struct {
	RunID                string                   `json:"runId"`
	TaskID               string                   `json:"taskId"`
	Task                 string                   `json:"task"`
	ToolCalls            []ToolCall               `json:"toolCalls"`
	ScreenshotStorageIDs []string                 `json:"screenshotStorageIds"`
	FinalResponse        string                   `json:"finalResultResponse"`
	Evaluation           *Evaluation              `json:"comprehensiveJudgeEvaluation"`
	CompleteHistory      []map[string]interface{} `json:"completeHistory,omitempty"`
}

// ToolCall represents a tool execution
type ToolCall struct {
	ToolName string `json:"tool_name"`
	Args     string `json:"args,omitempty"`
	Result   string `json:"result"`
	IsError  bool   `json:"is_error"`
}

// Evaluation represents judge evaluation
type Evaluation struct {
	Passed    bool     `json:"passed"`
	Score     float64  `json:"score"`
	Reasoning string   `json:"reasoning"`
	Errors    []string `json:"error_categories,omitempty"`
}

// FetchTestCase fetches tasks from Convex
func (c *Client) FetchTestCase(ctx context.Context, testCaseName string) ([]Task, error) {
	url := fmt.Sprintf("%s/api/getTestCase", c.baseURL)

	payload := map[string]string{"name": testCaseName}
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
		time.Sleep(time.Duration(1<<attempt) * time.Second)
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

// SaveTaskResult submits result to Convex
func (c *Client) SaveTaskResult(ctx context.Context, result *TaskResult) error {
	url := fmt.Sprintf("%s/api/saveTaskResult", c.baseURL)

	body, _ := json.Marshal(result)

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
		uploadURL, err := c.getUploadURL(ctx)
		if err != nil {
			continue
		}

		storageID, err := c.uploadToStorage(ctx, uploadURL, screenshot)
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
