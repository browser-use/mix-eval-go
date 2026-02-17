package orchestrator

import "github.com/anthropics/anthropic-sdk-go"

// Judge model constants — API strings sourced from mix_agent/internal/llm/models/.

// Anthropic judge models.
const (
	ModelClaude45Sonnet anthropic.Model = "claude-sonnet-4-5-20250929"
	ModelClaudeOpus46   anthropic.Model = "claude-opus-4-6"
	ModelClaudeHaiku45  anthropic.Model = "claude-haiku-4-5"
)

// Gemini judge models.
const (
	ModelGemini3Flash = "gemini-3-flash-preview"
	ModelGemini3Pro   = "gemini-3-pro-preview"
)
