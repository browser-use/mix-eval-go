//go:build e2e

package e2e

import (
	"context"
	"encoding/base64"
	"image"
	"image/color"
	"image/png"
	"bytes"
	"os"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"

	"mix-eval-go/pkg/convex"
	"mix-eval-go/pkg/orchestrator"
)

// TestJudgeSuccessfulRecipeSearch tests the judge with a successful recipe search scenario
func TestJudgeSuccessfulRecipeSearch(t *testing.T) {
	// Skip if ANTHROPIC_API_KEY not set
	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	if apiKey == "" {
		t.Skip("ANTHROPIC_API_KEY not set, skipping E2E judge test")
	}

	ctx := context.Background()

	// Create judge
	judge := orchestrator.NewJudgeAnthropic(apiKey, anthropic.ModelClaudeSonnet4_5_20250929)

	// Mock task
	task := convex.Task{
		Text: "Find 3 vegan recipes on allrecipes.com",
	}

	// Mock tool calls history - simulating a successful recipe search
	toolCalls := []orchestrator.ToolCall{
		{
			ToolName: "browser_navigate",
			Arguments: map[string]interface{}{
				"url": "https://www.allrecipes.com",
			},
			Result:  "Navigated to https://www.allrecipes.com\nTitle: Allrecipes | Food, friends, and recipe inspiration",
			IsError: false,
		},
		{
			ToolName: "browser_search",
			Arguments: map[string]interface{}{
				"query": "vegan recipes",
			},
			Result:  "Search initiated for 'vegan recipes'",
			IsError: false,
		},
		{
			ToolName: "browser_state",
			Arguments: map[string]interface{}{},
			Result: `URL: https://www.allrecipes.com/search?q=vegan+recipes
Title: Vegan Recipes Search Results

DOM CONTENT:
=== Page Header ===
Allrecipes - Search Results for "vegan recipes"

=== Search Results ===
1. Vegan Chili
   Rating: 4.5 stars (2,341 reviews)
   Description: A hearty and flavorful vegan chili packed with beans and vegetables
   URL: /recipe/12345/vegan-chili

2. Tofu Scramble
   Rating: 4.7 stars (1,892 reviews)
   Description: A delicious breakfast alternative to scrambled eggs
   URL: /recipe/12346/tofu-scramble

3. Lentil Curry
   Rating: 4.6 stars (3,104 reviews)
   Description: Rich and creamy coconut lentil curry
   URL: /recipe/12347/lentil-curry

4. Vegan Brownies
   Rating: 4.8 stars (5,234 reviews)
   Description: Fudgy chocolate brownies without eggs or dairy
   URL: /recipe/12348/vegan-brownies

5. Chickpea Salad Sandwich
   Rating: 4.4 stars (987 reviews)
   Description: A quick and easy vegan lunch option
   URL: /recipe/12349/chickpea-salad-sandwich

=== Sidebar ===
Popular Categories: Breakfast | Lunch | Dinner | Desserts
Dietary Options: Vegan | Vegetarian | Gluten-Free | Keto`,
			IsError: false,
		},
		{
			ToolName: "python",
			Arguments: map[string]interface{}{
				"code": "recipes = ['Vegan Chili', 'Tofu Scramble', 'Lentil Curry']\nprint('\\n'.join(recipes[:3]))",
			},
			Result: `Vegan Chili
Tofu Scramble
Lentil Curry`,
			IsError: false,
		},
		{
			ToolName: "done",
			Arguments: map[string]interface{}{
				"response": "Found 3 vegan recipes: Vegan Chili, Tofu Scramble, Lentil Curry",
			},
			Result:  "Task completed",
			IsError: false,
		},
	}

	// Mock final response
	finalResponse := "Found 3 vegan recipes on allrecipes.com:\n1. Vegan Chili - A hearty and flavorful vegan chili\n2. Tofu Scramble - A delicious breakfast alternative\n3. Lentil Curry - Rich and creamy coconut lentil curry"

	// Mock intermediate reasoning
	intermediateReasoning := []string{
		"Navigating to allrecipes.com to search for vegan recipes",
		"Initiating search for 'vegan recipes'",
		"Found multiple vegan recipes in search results. Extracting top 3.",
		"Successfully extracted 3 recipe names",
	}

	// Generate mock screenshots (simple colored images)
	screenshots := generateMockScreenshots(t, 2)

	// No sandbox files for this scenario
	sandboxFiles := []orchestrator.SandboxFile{}

	// Evaluate
	t.Log("Calling judge to evaluate recipe search task...")
	eval, err := judge.Evaluate(
		ctx,
		task,
		toolCalls,
		sandboxFiles,
		finalResponse,
		intermediateReasoning,
		nil, // no screenshot paths
		screenshots,
	)

	if err != nil {
		t.Fatalf("Judge evaluation failed: %v", err)
	}

	// Assertions
	t.Logf("Judge verdict: passed=%v, score=%.2f", eval.Passed, eval.Score)
	t.Logf("Judge reasoning: %s", eval.Reasoning)

	if !eval.Passed {
		t.Errorf("Expected task to pass, but judge marked it as failed. Reasoning: %s", eval.Reasoning)
	}

	if eval.Score != 1.0 {
		t.Errorf("Expected score 1.0, got %.2f", eval.Score)
	}

	if eval.ImpossibleTask {
		t.Error("Judge incorrectly marked task as impossible")
	}

	if eval.ReachedCaptcha {
		t.Error("Judge incorrectly marked task as hitting CAPTCHA")
	}

	if len(eval.Errors) > 0 {
		t.Errorf("Expected no errors, got: %v", eval.Errors)
	}

	t.Logf("✓ Judge correctly evaluated successful recipe search task")
}

// generateMockScreenshots creates simple test screenshots as base64 PNGs
func generateMockScreenshots(t *testing.T, count int) []string {
	t.Helper()

	screenshots := make([]string, count)

	for i := 0; i < count; i++ {
		// Create a simple 100x100 colored image
		img := image.NewRGBA(image.Rect(0, 0, 100, 100))

		// Fill with different colors for each screenshot
		var fillColor color.RGBA
		switch i {
		case 0:
			fillColor = color.RGBA{R: 100, G: 150, B: 200, A: 255} // Blue-ish
		case 1:
			fillColor = color.RGBA{R: 150, G: 200, B: 100, A: 255} // Green-ish
		default:
			fillColor = color.RGBA{R: 200, G: 100, B: 150, A: 255} // Red-ish
		}

		for y := 0; y < 100; y++ {
			for x := 0; x < 100; x++ {
				img.Set(x, y, fillColor)
			}
		}

		// Encode to PNG
		var buf bytes.Buffer
		if err := png.Encode(&buf, img); err != nil {
			t.Fatalf("Failed to encode PNG: %v", err)
		}

		// Convert to base64
		b64 := base64.StdEncoding.EncodeToString(buf.Bytes())
		screenshots[i] = b64
	}

	return screenshots
}
