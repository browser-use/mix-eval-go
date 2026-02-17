//go:build e2e

package e2e

import (
	"bytes"
	"context"
	"encoding/base64"
	"image"
	"image/color"
	"image/png"
	"os"
	"testing"

	"mix-eval-go/pkg/convex"
	"mix-eval-go/pkg/orchestrator"
)

// recipeSearchToolCalls returns a standard set of tool calls simulating a successful vegan recipe search.
func recipeSearchToolCalls() []orchestrator.ToolCall {
	return []orchestrator.ToolCall{
		{
			ToolName:  "browser_navigate",
			Arguments: map[string]interface{}{"url": "https://www.allrecipes.com"},
			Result:    "Navigated to https://www.allrecipes.com\nTitle: Allrecipes | Food, friends, and recipe inspiration",
			IsError:   false,
		},
		{
			ToolName:  "browser_search",
			Arguments: map[string]interface{}{"query": "vegan recipes"},
			Result:    "Search initiated for 'vegan recipes'",
			IsError:   false,
		},
		{
			ToolName:  "browser_state",
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
			Result:  "Vegan Chili\nTofu Scramble\nLentil Curry",
			IsError: false,
		},
		{
			ToolName:  "done",
			Arguments: map[string]interface{}{"response": "Found 3 vegan recipes: Vegan Chili, Tofu Scramble, Lentil Curry"},
			Result:    "Task completed",
			IsError:   false,
		},
	}
}

// runJudgeEval is a shared helper that evaluates a recipe search task with the given judge.
func runJudgeEval(t *testing.T, judge *orchestrator.Judge) {
	t.Helper()

	ctx := context.Background()
	task := convex.Task{Text: "Find 3 vegan recipes on allrecipes.com"}
	finalResponse := "Found 3 vegan recipes on allrecipes.com:\n1. Vegan Chili - A hearty and flavorful vegan chili\n2. Tofu Scramble - A delicious breakfast alternative\n3. Lentil Curry - Rich and creamy coconut lentil curry"
	intermediateReasoning := []string{
		"Navigating to allrecipes.com to search for vegan recipes",
		"Initiating search for 'vegan recipes'",
		"Found multiple vegan recipes in search results. Extracting top 3.",
		"Successfully extracted 3 recipe names",
	}
	screenshots := generateMockScreenshots(t, 2)

	t.Log("Calling judge to evaluate recipe search task...")
	eval, err := judge.Evaluate(
		ctx,
		task,
		recipeSearchToolCalls(),
		[]orchestrator.SandboxFile{},
		finalResponse,
		intermediateReasoning,
		nil, // no screenshot paths
		screenshots,
	)
	if err != nil {
		t.Fatalf("Judge evaluation failed: %v", err)
	}

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
}

// TestJudgeSuccessfulRecipeSearch tests the judge with Anthropic Claude.
func TestJudgeSuccessfulRecipeSearch(t *testing.T) {
	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	if apiKey == "" {
		t.Skip("ANTHROPIC_API_KEY not set, skipping E2E judge test")
	}

	judge := orchestrator.NewJudgeAnthropic(apiKey, orchestrator.ModelClaude45Sonnet)
	runJudgeEval(t, judge)
	t.Log("✓ Anthropic judge correctly evaluated successful recipe search task")
}

// TestJudgeSuccessfulRecipeSearchGemini tests the same scenario with a Gemini judge.
func TestJudgeSuccessfulRecipeSearchGemini(t *testing.T) {
	apiKey := os.Getenv("GEMINI_API_KEY")
	if apiKey == "" {
		t.Skip("GEMINI_API_KEY not set, skipping Gemini E2E judge test")
	}

	judge, err := orchestrator.NewJudgeGemini(apiKey, orchestrator.ModelGemini3Flash)
	if err != nil {
		t.Fatalf("Failed to create Gemini judge: %v", err)
	}

	runJudgeEval(t, judge)
	t.Log("✓ Gemini judge correctly evaluated successful recipe search task")
}

// generateMockScreenshots creates simple test screenshots as base64 PNGs.
func generateMockScreenshots(t *testing.T, count int) []string {
	t.Helper()

	screenshots := make([]string, count)

	for i := 0; i < count; i++ {
		img := image.NewRGBA(image.Rect(0, 0, 100, 100))

		var fillColor color.RGBA
		switch i {
		case 0:
			fillColor = color.RGBA{R: 100, G: 150, B: 200, A: 255}
		case 1:
			fillColor = color.RGBA{R: 150, G: 200, B: 100, A: 255}
		default:
			fillColor = color.RGBA{R: 200, G: 100, B: 150, A: 255}
		}

		for y := 0; y < 100; y++ {
			for x := 0; x < 100; x++ {
				img.Set(x, y, fillColor)
			}
		}

		var buf bytes.Buffer
		if err := png.Encode(&buf, img); err != nil {
			t.Fatalf("Failed to encode PNG: %v", err)
		}

		screenshots[i] = base64.StdEncoding.EncodeToString(buf.Bytes())
	}

	return screenshots
}
