package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/recreate-run/mix-go-sdk"
	"github.com/recreate-run/mix-go-sdk/models/operations"

	"mix-eval-go/pkg/convex"
)

// Judge evaluates task completion using Claude
type Judge struct {
	mixClient *mix.Mix
}

// NewJudge creates a new judge instance
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

	// Wait for processing
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
			json.Unmarshal([]byte(*msg.AssistantResponse), &eval)
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
