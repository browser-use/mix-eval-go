package orchestrator

import (
	"context"
	"fmt"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

// AnthropicJudgeLLM implements JudgeLLM using the Anthropic SDK.
type AnthropicJudgeLLM struct {
	client anthropic.Client
	model  anthropic.Model
}

// NewAnthropicJudgeLLM creates an Anthropic-backed JudgeLLM.
func NewAnthropicJudgeLLM(apiKey string, model anthropic.Model) JudgeLLM {
	return &AnthropicJudgeLLM{
		client: anthropic.NewClient(option.WithAPIKey(apiKey)),
		model:  model,
	}
}

func (a *AnthropicJudgeLLM) Send(ctx context.Context, messages []JudgeMessage) (string, error) {
	params := make([]anthropic.MessageParam, len(messages))
	for i, m := range messages {
		if m.Role == "user" {
			blocks := []anthropic.ContentBlockParamUnion{
				anthropic.NewTextBlock(m.Content),
			}
			for _, img := range m.Images {
				blocks = append(blocks, anthropic.NewImageBlockBase64(img.MIMEType, img.B64Data))
			}
			params[i] = anthropic.NewUserMessage(blocks...)
		} else {
			params[i] = anthropic.NewAssistantMessage(anthropic.NewTextBlock(m.Content))
		}
	}

	msg, err := a.client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     a.model,
		MaxTokens: 4096,
		Messages:  params,
	})
	if err != nil {
		return "", fmt.Errorf("anthropic judge call failed: %w", err)
	}

	var sb strings.Builder
	for _, block := range msg.Content {
		if tb, ok := block.AsAny().(anthropic.TextBlock); ok {
			sb.WriteString(tb.Text)
		}
	}
	return sb.String(), nil
}
