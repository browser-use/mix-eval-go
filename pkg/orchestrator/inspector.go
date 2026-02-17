package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"
)

// inspectStep is a sub-judge that inspects a specific step with full, untruncated content.
// Returns a summary relevant to the main judge's query.
func inspectStep(
	ctx context.Context,
	stepIndex int,
	query string,
	toolCalls []ToolCall,
	task string,
	llm JudgeLLM,
) (string, error) {
	if stepIndex < 0 || stepIndex >= len(toolCalls) {
		return fmt.Sprintf("Error: Step index %d is out of range (0-%d)", stepIndex, len(toolCalls)-1), nil
	}

	tc := toolCalls[stepIndex]
	argsJSON, _ := json.Marshal(tc.Arguments)

	subJudgePrompt := fmt.Sprintf(`You are a sub-judge helping evaluate whether an AI agent completed a task.

## Original Task
%s

## Your Job
The main judge needs to verify specific information from step %d of the agent's execution.
You have access to the COMPLETE, UNTRUNCATED content from this step - exactly what the agent saw.

## Main Judge's Query
%s

## Step %d Details

**Tool:** %s
**Status:** %s
**Arguments:** %s

**Full Result (UNTRUNCATED - this is exactly what the agent saw):**
%s

## Instructions
1. Carefully read the full result above
2. Answer the main judge's query based on what you find
3. Be specific - quote exact text when relevant
4. If the queried information IS present, say so clearly and quote it
5. If the queried information is NOT present, say so clearly
6. If you find CONTRADICTORY information, highlight it

Provide a concise but complete summary that answers the main judge's query.`,
		task,
		stepIndex,
		query,
		stepIndex,
		tc.ToolName,
		map[bool]string{true: "ERROR", false: "OK"}[tc.IsError],
		string(argsJSON),
		tc.Result,
	)

	response, err := llm.Send(ctx, []JudgeMessage{
		{Role: "user", Content: subJudgePrompt},
	})
	if err != nil {
		return fmt.Sprintf("Error inspecting step: %v", err), nil
	}
	if response == "" {
		return "No response from sub-judge", nil
	}
	return response, nil
}
