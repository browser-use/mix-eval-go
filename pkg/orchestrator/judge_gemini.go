package orchestrator

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"

	"google.golang.org/genai"
)

// GeminiJudgeLLM implements JudgeLLM using the Google Gemini SDK.
type GeminiJudgeLLM struct {
	client *genai.Client
	model  string
}

// NewGeminiJudgeLLM creates a Gemini-backed JudgeLLM.
func NewGeminiJudgeLLM(apiKey, model string) (JudgeLLM, error) {
	client, err := genai.NewClient(context.Background(), &genai.ClientConfig{
		APIKey:  apiKey,
		Backend: genai.BackendGeminiAPI,
	})
	if err != nil {
		return nil, fmt.Errorf("gemini client creation failed: %w", err)
	}
	return &GeminiJudgeLLM{client: client, model: model}, nil
}

func (g *GeminiJudgeLLM) Send(ctx context.Context, messages []JudgeMessage) (string, error) {
	if len(messages) == 0 {
		return "", fmt.Errorf("no messages provided")
	}

	// Build history from all messages except the last
	var history []*genai.Content
	for _, m := range messages[:len(messages)-1] {
		role := "user"
		if m.Role == "assistant" {
			role = "model"
		}
		var parts []*genai.Part
		if m.Content != "" {
			parts = append(parts, &genai.Part{Text: m.Content})
		}
		for _, img := range m.Images {
			data, err := base64.StdEncoding.DecodeString(img.B64Data)
			if err != nil {
				continue
			}
			parts = append(parts, &genai.Part{
				InlineData: &genai.Blob{MIMEType: img.MIMEType, Data: data},
			})
		}
		history = append(history, &genai.Content{Role: role, Parts: parts})
	}

	// Build the last message parts (SendMessage takes values, not pointers)
	last := messages[len(messages)-1]
	var lastParts []genai.Part
	if last.Content != "" {
		lastParts = append(lastParts, genai.Part{Text: last.Content})
	}
	for _, img := range last.Images {
		data, err := base64.StdEncoding.DecodeString(img.B64Data)
		if err != nil {
			continue
		}
		lastParts = append(lastParts, genai.Part{
			InlineData: &genai.Blob{MIMEType: img.MIMEType, Data: data},
		})
	}

	chat, err := g.client.Chats.Create(ctx, g.model, &genai.GenerateContentConfig{
		MaxOutputTokens: 4096,
	}, history)
	if err != nil {
		return "", fmt.Errorf("gemini chat creation failed: %w", err)
	}

	resp, err := chat.SendMessage(ctx, lastParts...)
	if err != nil {
		return "", fmt.Errorf("gemini send failed: %w", err)
	}

	var sb strings.Builder
	if len(resp.Candidates) > 0 && resp.Candidates[0].Content != nil {
		for _, p := range resp.Candidates[0].Content.Parts {
			sb.WriteString(p.Text)
		}
	}
	return sb.String(), nil
}
