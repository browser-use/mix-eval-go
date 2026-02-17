package orchestrator

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"

	"mix-eval-go/pkg/convex"
)

const (
	maxImages       = 5
	maxTask         = 2000
	maxToolCalls    = 50
	maxReasoning    = 2000
	maxInspectCalls = 10
	maxJSONRetries  = 3
)

// Judge evaluates task completion using any JudgeLLM provider.
type Judge struct {
	llm JudgeLLM
}

// NewJudge creates a judge backed by the given JudgeLLM implementation.
func NewJudge(llm JudgeLLM) *Judge {
	return &Judge{llm: llm}
}

// NewJudgeAnthropic is a convenience constructor for the Anthropic-backed judge.
func NewJudgeAnthropic(apiKey string, model anthropic.Model) *Judge {
	return NewJudge(NewAnthropicJudgeLLM(apiKey, model))
}

// mustJudge panics if err is non-nil; used for judge construction at startup.
func mustJudge(j *Judge, err error) *Judge {
	if err != nil {
		panic("judge creation failed: " + err.Error())
	}
	return j
}

// NewJudgeGemini is a convenience constructor for the Gemini-backed judge.
func NewJudgeGemini(apiKey, model string) (*Judge, error) {
	llm, err := NewGeminiJudgeLLM(apiKey, model)
	if err != nil {
		return nil, err
	}
	return NewJudge(llm), nil
}

// Evaluate evaluates task completion with multi-turn conversation and inspect_step tool.
func (j *Judge) Evaluate(
	ctx context.Context,
	task convex.Task,
	toolCalls []ToolCall,
	sandboxFiles []SandboxFile,
	finalResponse string,
	intermediateReasoning []string,
	screenshotPaths []string,
	screenshotsB64 []string,
) (*convex.Evaluation, error) {
	// Format inputs
	taskText := truncate(task.Text, maxTask)

	// Build step index
	stepIndex := buildStepIndex(toolCalls)
	stepIndexSubset := stepIndex
	if len(stepIndex) > maxToolCalls {
		stepIndexSubset = stepIndex[len(stepIndex)-maxToolCalls:]
	}
	stepIndexText := formatStepIndex(stepIndexSubset)
	if len(stepIndex) > maxToolCalls {
		stepIndexText = fmt.Sprintf("[... %d earlier steps omitted ...]\n%s", len(stepIndex)-maxToolCalls, stepIndexText)
	}

	// Format files
	filesText := formatFiles(sandboxFiles, 5)

	// Format reasoning
	reasoningText := ""
	if len(intermediateReasoning) > 0 {
		lastReasoning := intermediateReasoning
		if len(intermediateReasoning) > 5 {
			lastReasoning = intermediateReasoning[len(intermediateReasoning)-5:]
		}
		var reasoningParts []string
		for _, r := range lastReasoning {
			reasoningParts = append(reasoningParts, truncate(r, 400))
		}
		reasoningText = strings.Join(reasoningParts, "\n---\n")
		reasoningText = truncate(reasoningText, maxReasoning)
	}

	// Check completion signals
	doneCall := false
	errorCount := 0
	for _, tc := range toolCalls {
		if tc.ToolName == "done" || tc.ToolName == "done_autonomous" {
			doneCall = true
		}
		if tc.IsError {
			errorCount++
		}
	}

	// Build comprehensive prompt
	prompt := buildEvaluationPrompt(
		taskText,
		stepIndexText,
		filesText,
		reasoningText,
		finalResponse,
		doneCall,
		errorCount,
		len(toolCalls),
		maxInspectCalls,
	)

	// Collect and limit screenshots
	imageURLs := collectImageURLs(screenshotPaths, screenshotsB64)
	if len(imageURLs) == 0 {
		log.Printf("Warning: no screenshots available for judge evaluation - verdict will rely solely on tool call history and final response")
	} else if len(imageURLs) > maxImages {
		log.Printf("Limiting screenshots from %d to %d", len(imageURLs), maxImages)
		imageURLs = imageURLs[len(imageURLs)-maxImages:]
	}

	// Extract images from data URLs
	var images []JudgeImage
	for _, url := range imageURLs {
		switch {
		case strings.HasPrefix(url, "data:image/jpeg;base64,"):
			images = append(images, JudgeImage{
				MIMEType: "image/jpeg",
				B64Data:  strings.TrimPrefix(url, "data:image/jpeg;base64,"),
			})
		case strings.HasPrefix(url, "data:image/png;base64,"):
			images = append(images, JudgeImage{
				MIMEType: "image/png",
				B64Data:  strings.TrimPrefix(url, "data:image/png;base64,"),
			})
		}
	}

	// Build initial user message content
	content := prompt
	if len(images) > 0 {
		content += fmt.Sprintf(
			"\n\n[%d screenshot(s) from agent execution attached below - NOTE: These may be incomplete or partial views; the agent may have seen more information than what is visible in these screenshots]",
			len(images),
		)
	}

	messages := []JudgeMessage{
		{Role: "user", Content: content, Images: images},
	}

	inspectCount := 0
	jsonRetryCount := 0

	for {
		responseText, err := j.llm.Send(ctx, messages)
		if err != nil {
			return nil, fmt.Errorf("judge API call failed: %w", err)
		}

		// Parse JSON from response
		parsedObjects := parseJSONObjects(responseText)

		if len(parsedObjects) == 0 {
			// No JSON found
			jsonRetryCount++
			if jsonRetryCount >= maxJSONRetries {
				log.Printf("Judge failed to produce valid JSON after %d retries", maxJSONRetries)
				return &convex.Evaluation{
					Passed:         false,
					Score:          0.0,
					Reasoning:      fmt.Sprintf("Judge failed to produce valid JSON after %d retries. Last response: %s", maxJSONRetries, truncate(responseText, 500)),
					Errors:         []string{"judge_error"},
					ImpossibleTask: false,
					ReachedCaptcha: false,
				}, nil
			}

			log.Printf("No JSON in judge response (retry %d/%d), retrying", jsonRetryCount, maxJSONRetries)
			messages = append(messages, JudgeMessage{Role: "assistant", Content: responseText})
			messages = append(messages, JudgeMessage{Role: "user", Content: "Please respond with a single valid JSON object only. Either an inspect_step call or your final verdict."})
			continue
		}

		// Reset retry counter on successful parse
		jsonRetryCount = 0

		// Prioritize inspect_step over verdict
		var inspectObj, verdictObj map[string]interface{}
		for _, obj := range parsedObjects {
			if tool, ok := obj["tool"].(string); ok && tool == "inspect_step" && inspectObj == nil {
				inspectObj = obj
			}
			if _, hasVerdict := obj["verdict"]; hasVerdict && verdictObj == nil {
				verdictObj = obj
			}
		}

		// Use inspect_step if present, otherwise use verdict
		var result map[string]interface{}
		if inspectObj != nil && inspectCount < maxInspectCalls {
			result = inspectObj
			if verdictObj != nil {
				log.Println("Judge returned both inspect_step and verdict; processing inspect_step first")
			}
		} else if verdictObj != nil {
			result = verdictObj
		} else {
			result = parsedObjects[0]
		}

		// Check if this is a tool call or final verdict
		if tool, ok := result["tool"].(string); ok && tool == "inspect_step" && inspectCount < maxInspectCalls {
			// Handle inspect_step tool call
			stepIdx, _ := result["step_index"].(float64) // JSON numbers are float64
			query, _ := result["query"].(string)

			log.Printf("Judge inspecting step %d: %s", int(stepIdx), truncate(query, 100))
			inspectResult, err := inspectStep(
				ctx,
				int(stepIdx),
				query,
				toolCalls,
				task.Text,
				j.llm,
			)
			if err != nil {
				return nil, fmt.Errorf("inspect_step failed: %w", err)
			}

			inspectCount++
			remaining := maxInspectCalls - inspectCount

			// Add exchange to messages and continue
			messages = append(messages, JudgeMessage{Role: "assistant", Content: responseText})
			messages = append(messages, JudgeMessage{Role: "user", Content: fmt.Sprintf(`## inspect_step Result for Step %d

%s

---
You have %d inspect_step calls remaining. You can inspect more steps or provide your final verdict.`, int(stepIdx), inspectResult, remaining)})
			continue

		} else if tool, ok := result["tool"].(string); ok && tool == "inspect_step" && inspectCount >= maxInspectCalls {
			// Max inspections reached
			messages = append(messages, JudgeMessage{Role: "assistant", Content: responseText})
			messages = append(messages, JudgeMessage{Role: "user", Content: fmt.Sprintf("You have used all %d inspect_step calls. Please provide your final verdict now.", maxInspectCalls)})
			continue

		} else if _, hasVerdict := result["verdict"]; hasVerdict {
			// Final verdict
			verdict, _ := result["verdict"].(bool)
			reasoning, _ := result["reasoning"].(string)
			impossibleTask, _ := result["impossible_task"].(bool)
			reachedCaptcha, _ := result["reached_captcha"].(bool)

			// Enforcement: verdict=false requires inspect_step
			if !verdict && inspectCount == 0 {
				log.Println("Judge attempted verdict=false without using inspect_step - forcing verification")
				messages = append(messages, JudgeMessage{Role: "assistant", Content: responseText})
				messages = append(messages, JudgeMessage{Role: "user", Content: fmt.Sprintf(`**REJECTED: You cannot return verdict=false without first using inspect_step.**

You are about to fail this task, but you have not verified your concerns by inspecting the actual tool call results.

The step index shows truncated previews. Screenshots show partial views. You MUST use inspect_step to see the COMPLETE data the agent saw before claiming:
- Data is fabricated
- Information doesn't match
- Results are missing
- Extraction is wrong

Please use inspect_step now to verify your concerns. Look at the steps where the agent claims to have extracted the information. Check if that information actually exists in the full tool result.

Example: If the agent claims to have found 4 recipe titles, use inspect_step on the browser_state call where the search results appeared to see if those titles are in the DOM.

You have %d inspect_step calls remaining.`, maxInspectCalls-inspectCount)})
				continue
			}

			// Score: 1.0 for success, 0.0 for failure
			score := 0.0
			if verdict {
				score = 1.0
			}

			// Add note about inspections used
			if inspectCount > 0 {
				reasoning = fmt.Sprintf("[Used %d step inspection(s)] %s", inspectCount, reasoning)
			}

			// Build error categories
			var errors []string
			if !verdict {
				errors = []string{"task_incomplete"}
			}

			return &convex.Evaluation{
				Passed:         verdict,
				Score:          score,
				Reasoning:      reasoning,
				Errors:         errors,
				ImpossibleTask: impossibleTask,
				ReachedCaptcha: reachedCaptcha,
				ComprehensiveEval: map[string]interface{}{
					"task_summary":      fmt.Sprintf("Task %s", map[bool]string{true: "completed successfully", false: "not completed"}[verdict]),
					"reasoning":         reasoning,
					"passed":            verdict,
					"final_score":       int(score * 100),
					"error_categories":  errors,
					"improvement_tips":  map[bool][]string{true: {}, false: {reasoning}}[verdict],
				},
			}, nil

		} else {
			// Unknown response format
			messages = append(messages, JudgeMessage{Role: "assistant", Content: responseText})
			messages = append(messages, JudgeMessage{Role: "user", Content: "Please respond with either an inspect_step tool call or your final verdict in valid JSON format."})
			continue
		}
	}
}
