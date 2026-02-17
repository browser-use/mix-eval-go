package orchestrator

import "context"

// JudgeMessage is a single turn in the judge's multi-turn conversation.
type JudgeMessage struct {
	Role    string       // "user" | "assistant"
	Content string       // text content
	Images  []JudgeImage // populated only on user turns
}

// JudgeImage holds a base64-encoded image for judge evaluation.
type JudgeImage struct {
	MIMEType string // "image/jpeg" | "image/png"
	B64Data  string // raw base64, no data-URL prefix
}

// JudgeLLM abstracts the underlying LLM provider used by the judge.
// Send takes the full conversation history and returns the model's text reply.
type JudgeLLM interface {
	Send(ctx context.Context, messages []JudgeMessage) (string, error)
}
